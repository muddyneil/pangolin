package source

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/pangolin/pangolin/internal/version"
)

const MaxResponseBytes int64 = 8 << 20
const MaxProxyBytes = 64 << 10
const MaxCandidates = 500

type Source struct {
	Name, Primary string
	Fallbacks     []string
}
type Proxy struct {
	Name, Type, Server string
	Port               int
	Fingerprint        string
	Region             string
	Fields             map[string]any
}

type FetchResult struct {
	Source  string
	Proxies []Proxy
	Err     error
}

func FetchAll(ctx context.Context, sources []Source) []FetchResult {
	results := make([]FetchResult, len(sources))
	type indexedResult struct {
		index  int
		result FetchResult
	}
	ch := make(chan indexedResult, len(sources))
	var wg sync.WaitGroup
	for i, src := range sources {
		i, src := i, src
		wg.Add(1)
		go func(index int, item Source) { defer wg.Done(); ch <- indexedResult{index, fetchOne(ctx, item)} }(i, src)
	}
	wg.Wait()
	close(ch)
	for item := range ch {
		results[item.index] = item.result
	}
	return results
}

func fetchOne(ctx context.Context, src Source) FetchResult {
	urls := append([]string{src.Primary}, src.Fallbacks...)
	if len(urls) == 1 {
		data, err := fetchWithRetry(ctx, urls[0])
		if err != nil {
			return FetchResult{Source: src.Name, Err: err}
		}
		proxies, err := parse(data)
		if err != nil || len(proxies) == 0 {
			err = fmt.Errorf("no proxy nodes found")
			return FetchResult{Source: src.Name, Err: err}
		}
		return FetchResult{Source: src.Name, Proxies: normalize(proxies)}
	}

	type result struct {
		index   int
		proxies []map[string]any
		err     error
	}
	requestsCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	results := make(chan result, len(urls))
	for index, raw := range urls {
		go func(index int, raw string) {
			data, err := fetchWithRetry(requestsCtx, raw)
			if err != nil {
				results <- result{index: index, err: err}
				return
			}
			proxies, err := parse(data)
			results <- result{index: index, proxies: proxies, err: err}
		}(index, raw)
	}

	// Prefer the primary for a short priority window, but never lose a good
	// fallback: once the primary has failed (or the window has expired) use the
	// first successful fallback immediately instead of waiting for the timer.
	primaryTimer := time.NewTimer(3 * time.Second)
	defer primaryTimer.Stop()
	preferPrimary := true
	var firstFallback []map[string]any
	var primaryErr error
	var last error
	remaining := len(urls)
	for remaining > 0 {
		select {
		case item := <-results:
			remaining--
			if item.err != nil {
				if item.index == 0 {
					primaryErr = item.err
				}
				last = item.err
				if item.index == 0 {
					// Primary is known dead; a fallback already in hand can be
					// used right away instead of waiting out the window.
					preferPrimary = false
					if firstFallback != nil {
						cancel()
						return FetchResult{Source: src.Name, Proxies: normalize(firstFallback)}
					}
				}
				continue
			}
			if len(item.proxies) == 0 {
				if item.index == 0 {
					primaryErr = fmt.Errorf("no proxy nodes found")
				}
				last = fmt.Errorf("no proxy nodes found")
				if item.index == 0 {
					preferPrimary = false
				}
				continue
			}
			if item.index == 0 || !preferPrimary {
				cancel()
				return FetchResult{Source: src.Name, Proxies: normalize(item.proxies)}
			}
			if firstFallback == nil {
				firstFallback = item.proxies
			}
		case <-primaryTimer.C:
			preferPrimary = false
			if firstFallback != nil {
				cancel()
				return FetchResult{Source: src.Name, Proxies: normalize(firstFallback)}
			}
		}
	}
	if primaryErr != nil {
		return FetchResult{Source: src.Name, Err: primaryErr}
	}
	if last == nil {
		last = fmt.Errorf("no proxy nodes found")
	}
	return FetchResult{Source: src.Name, Err: last}
}

func fetchWithRetry(ctx context.Context, raw string) ([]byte, error) {
	var last error
retryLoop:
	for attempt := 0; attempt < 3; attempt++ {
		requestCtx, cancel := context.WithTimeout(ctx, 25*time.Second)
		data, err := fetch(requestCtx, raw)
		cancel()
		if err == nil {
			return data, nil
		}
		last = err
		if ctx.Err() != nil {
			break
		}
		// Deterministic client errors (404, 403, ...) do not heal with retries;
		// transient statuses such as 408 and 429 are retried before failing.
		var statusErr httpStatusError
		if errors.As(err, &statusErr) && !retryableStatus(statusErr.Status) {
			break
		}
		select {
		case <-time.After(time.Duration(attempt+1) * time.Second):
		case <-ctx.Done():
			break retryLoop
		}
	}
	return nil, last
}

// httpStatusError carries the HTTP status of a failed fetch so retry logic can
// distinguish deterministic client errors from transient failures.
type httpStatusError struct{ Status int }

func (e httpStatusError) Error() string { return fmt.Sprintf("source returned HTTP %d", e.Status) }

func retryableStatus(status int) bool {
	return status == http.StatusRequestTimeout || status == http.StatusTooEarly || status == http.StatusTooManyRequests || status >= 500
}

func fetch(ctx context.Context, raw string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, raw, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", fmt.Sprintf("pangolin/%s", version.Version))
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, httpStatusError{Status: resp.StatusCode}
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, MaxResponseBytes+1))
	if err != nil {
		return nil, fmt.Errorf("failed to read source response: %w", err)
	}
	return data, nil
}

func parse(data []byte) ([]map[string]any, error) {
	if int64(len(data)) > MaxResponseBytes {
		return nil, fmt.Errorf("source response exceeds %d bytes", MaxResponseBytes)
	}
	var value any
	if json.Unmarshal(data, &value) == nil {
		if proxies, err := proxyList(value); err == nil && len(proxies) > 0 {
			return proxies, nil
		}
	}
	// Only bother compacting the body when it could actually be Base64; a
	// large YAML/HTML source would otherwise double its peak memory just to
	// be rejected here.
	var decoded []byte
	var err error
	if looksLikeBase64(data) {
		compact := strings.Join(strings.Fields(string(data)), "")
		decoded, err = decodeBase64(compact)
	}
	if err == nil && len(decoded) > 0 {
		if json.Unmarshal(decoded, &value) == nil {
			if proxies, parseErr := proxyList(value); parseErr == nil && len(proxies) > 0 {
				return proxies, nil
			}
		}
		if parsed, parseErr := parseSimpleYAML(string(decoded)); parseErr == nil && len(parsed) > 0 {
			return parsed, nil
		}
		if proxies, parseErr := parseURIs(string(decoded)); parseErr == nil && len(proxies) > 0 {
			return proxies, nil
		}
	}
	parsed, err := parseSimpleYAML(string(data))
	if err == nil && len(parsed) > 0 {
		return parsed, nil
	}
	if fallback := extractProxyBlock(string(data)); len(fallback) > 0 {
		return fallback, nil
	}
	return parseURIs(string(data))
}

func extractProxyBlock(text string) []map[string]any {
	lines := strings.Split(text, "\n")
	start := -1
	for index, line := range lines {
		if strings.TrimSpace(line) == "proxies:" {
			start = index
			break
		}
	}
	if start < 0 {
		return nil
	}
	block := []string{}
	for _, line := range lines[start+1:] {
		trimmed := strings.TrimSpace(line)
		if trimmed != "" && !strings.HasPrefix(line, " ") && !strings.HasPrefix(line, "\t") && strings.Contains(trimmed, ":") {
			break
		}
		block = append(block, line)
	}
	parsed, err := parseSimpleYAML("proxies:\n" + strings.Join(block, "\n"))
	if err != nil {
		return nil
	}
	return parsed
}

func parseURIs(text string) ([]map[string]any, error) {
	var result []map[string]any
	for _, line := range strings.Fields(text) {
		if proxy, err := parseURI(line); err == nil {
			result = append(result, proxy)
		}
	}
	if len(result) == 0 {
		return nil, fmt.Errorf("content is not a supported proxy list")
	}
	return result, nil
}

func parseURI(raw string) (map[string]any, error) {
	raw = strings.TrimSpace(raw)
	separator := strings.Index(raw, "://")
	if separator <= 0 {
		return nil, fmt.Errorf("invalid proxy URI")
	}
	scheme := strings.ToLower(raw[:separator])
	switch scheme {
	case "vmess":
		return parseVMessURI(raw[separator+3:])
	case "ss":
		return parseSSURI(raw)
	case "ssr":
		return parseSSRURI(raw)
	case "trojan", "vless", "hysteria", "hysteria2", "hy2", "tuic", "socks5", "http", "https":
	default:
		return nil, fmt.Errorf("unsupported proxy protocol: %s", scheme)
	}

	u, err := url.Parse(raw)
	if err != nil || u.Hostname() == "" {
		return nil, fmt.Errorf("invalid proxy URI")
	}
	port, err := portOf(u)
	if err != nil {
		return nil, err
	}
	result := map[string]any{"name": u.Hostname(), "type": scheme, "server": u.Hostname(), "port": port}
	if scheme == "https" {
		result["type"], result["tls"] = "http", true
	}
	if u.User != nil {
		username := u.User.Username()
		if username != "" {
			switch scheme {
			case "vless", "tuic":
				result["uuid"] = username
			case "http", "https", "socks5":
				result["username"] = username
			default:
				result["password"] = username
			}
		}
		if password, ok := u.User.Password(); ok {
			result["password"] = password
		}
	}
	applyURIQuery(result, scheme, u.Query())
	if u.Fragment != "" {
		result["name"] = u.Fragment
	}
	return result, nil
}

// defaultPorts supplies the conventional port when a generic proxy URI omits
// one. An absent port makes the node unusable anyway, so a conventional value
// is strictly more useful than rejecting the URI outright.
var defaultPorts = map[string]int{"http": 80, "https": 443, "socks5": 1080, "trojan": 443, "vless": 443, "hysteria": 443, "hysteria2": 443, "tuic": 443}

func portOf(u *url.URL) (int, error) {
	if text := u.Port(); text != "" {
		port, err := strconv.Atoi(text)
		if err != nil {
			return 0, fmt.Errorf("invalid proxy port")
		}
		return port, nil
	}
	port, ok := defaultPorts[strings.ToLower(u.Scheme)]
	if !ok {
		return 0, fmt.Errorf("invalid proxy port")
	}
	return port, nil
}

func parseVMessURI(encoded string) (map[string]any, error) {
	encoded = strings.SplitN(encoded, "#", 2)[0]
	decoded, err := decodeBase64(encoded)
	if err != nil {
		return nil, fmt.Errorf("invalid VMess encoding")
	}
	var input map[string]any
	if err := json.Unmarshal(decoded, &input); err != nil {
		return nil, fmt.Errorf("invalid VMess JSON")
	}
	server, _ := input["add"].(string)
	port := number(input["port"])
	uuid, _ := input["id"].(string)
	if server == "" || port < 1 || uuid == "" {
		return nil, fmt.Errorf("missing required VMess fields")
	}
	result := map[string]any{"name": server, "type": "vmess", "server": server, "port": port, "uuid": uuid}
	for from, to := range map[string]string{"aid": "alterId", "scy": "cipher", "net": "network", "host": "host", "path": "path", "tls": "tls", "sni": "servername", "alpn": "alpn", "fp": "client-fingerprint"} {
		if value, ok := input[from]; ok {
			result[to] = value
		}
	}
	if insecure, ok := input["allowInsecure"]; ok {
		result["skip-cert-verify"] = insecure
	}
	if name, ok := input["ps"].(string); ok && name != "" {
		result["name"] = name
	}
	return result, nil
}

func parseSSURI(raw string) (map[string]any, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return nil, fmt.Errorf("invalid SS content")
	}
	var server, portText string
	var credentials []string
	if u.User != nil {
		decoded, decodeErr := decodeBase64(u.User.Username())
		if decodeErr != nil {
			return nil, fmt.Errorf("invalid SS encoding")
		}
		server, portText, credentials, err = parseSSPayload(string(decoded), u.Hostname(), u.Port())
	} else {
		payload := strings.TrimPrefix(strings.TrimSpace(raw), "ss://")
		if fragment := strings.IndexByte(payload, '#'); fragment >= 0 {
			payload = payload[:fragment]
		}
		decoded, decodeErr := decodeBase64(payload)
		if decodeErr != nil {
			return nil, fmt.Errorf("invalid SS encoding")
		}
		server, portText, credentials, err = parseSSPayload(string(decoded), "", "")
	}
	if err != nil || server == "" || len(credentials) != 2 {
		return nil, fmt.Errorf("invalid SS content")
	}
	port, err := strconv.Atoi(portText)
	if err != nil {
		return nil, fmt.Errorf("invalid proxy port")
	}
	result := map[string]any{"name": server, "type": "ss", "server": server, "port": port, "cipher": credentials[0], "password": credentials[1]}
	if plugin := u.Query().Get("plugin"); plugin != "" {
		name, opts := parsePlugin(plugin)
		result["plugin"] = name
		if len(opts) > 0 {
			result["plugin-opts"] = opts
		}
	}
	if u.Fragment != "" {
		result["name"] = u.Fragment
	}
	return result, nil
}

func parseSSPayload(payload, fallbackServer, fallbackPort string) (string, string, []string, error) {
	server, portText := fallbackServer, fallbackPort
	credentials := strings.SplitN(payload, ":", 2)
	if at := strings.LastIndex(payload, "@"); at >= 0 {
		credentials = strings.SplitN(payload[:at], ":", 2)
		server, portText, _ = net.SplitHostPort(payload[at+1:])
	}
	if len(credentials) != 2 {
		return "", "", nil, fmt.Errorf("invalid SS content")
	}
	return server, portText, credentials, nil
}

// parseSSRURI decodes the standard ssr:// form:
// ssr://base64url(server:port:protocol:method:obfs:base64(password)/?params)
// where params like obfsparam/protoparam/remarks are base64url- or
// percent-encoded values.
func parseSSRURI(raw string) (map[string]any, error) {
	encoded := strings.TrimPrefix(strings.TrimSpace(raw), "ssr://")
	decoded, err := decodeBase64(encoded)
	if err != nil {
		return nil, fmt.Errorf("invalid SSR encoding")
	}
	text := string(decoded)
	var params string
	if index := strings.Index(text, "?"); index >= 0 {
		params = text[index+1:]
		text = text[:index]
	}
	text = strings.TrimSuffix(text, "/")
	core, err := splitSSRCore(text)
	if err != nil {
		return nil, err
	}
	password, err := decodeBase64(core[5])
	if err != nil {
		return nil, fmt.Errorf("invalid SSR password encoding")
	}
	port, err := strconv.Atoi(core[1])
	if err != nil {
		return nil, fmt.Errorf("invalid proxy port")
	}
	result := map[string]any{"name": core[0], "type": "ssr", "server": core[0], "port": port, "cipher": core[3], "password": string(password), "protocol": core[2], "obfs": core[4]}
	if query, err := url.ParseQuery(params); err == nil {
		if remarks := decodeParam(query.Get("remarks")); remarks != "" {
			result["name"] = remarks
		}
		for from, to := range map[string]string{"obfsparam": "obfs-param", "protoparam": "protocol-param"} {
			if value := decodeParam(query.Get(from)); value != "" {
				result[to] = value
			}
		}
	}
	return result, nil
}

func splitSSRCore(text string) ([]string, error) {
	if strings.HasPrefix(text, "[") {
		end := strings.IndexByte(text, ']')
		if end < 0 || end+1 >= len(text) || text[end+1] != ':' {
			return nil, fmt.Errorf("invalid SSR content")
		}
		server := text[1:end]
		rest := strings.SplitN(text[end+2:], ":", 5)
		if len(rest) != 5 || server == "" {
			return nil, fmt.Errorf("invalid SSR content")
		}
		return append([]string{server}, rest...), nil
	}
	core := strings.SplitN(text, ":", 6)
	if len(core) != 6 || core[0] == "" {
		return nil, fmt.Errorf("invalid SSR content")
	}
	return core, nil
}
func decodeParam(value string) string {
	if value == "" {
		return ""
	}
	if decoded, err := decodeBase64(value); err == nil && isPrintable(decoded) {
		return string(decoded)
	}
	if decoded, err := url.QueryUnescape(value); err == nil {
		return decoded
	}
	return value
}

func isPrintable(data []byte) bool {
	if len(data) == 0 || !utf8.Valid(data) {
		return false
	}
	for _, r := range string(data) {
		if !unicode.IsPrint(r) {
			return false
		}
	}
	return true
}

func parsePlugin(value string) (string, map[string]any) {
	parts := strings.Split(value, ";")
	if len(parts) == 0 {
		return "", nil
	}
	opts := make(map[string]any)
	for _, part := range parts[1:] {
		key, val, ok := strings.Cut(part, "=")
		if ok && key != "" {
			opts[key] = val
		}
	}
	return parts[0], opts
}

func applyURIQuery(result map[string]any, scheme string, query url.Values) {
	security := strings.ToLower(query.Get("security"))
	if security == "tls" || security == "reality" || strings.EqualFold(query.Get("tls"), "true") {
		result["tls"] = true
	}
	if insecure := query.Get("allowInsecure"); insecure != "" {
		result["skip-cert-verify"] = parseBool(insecure)
	} else if insecure := query.Get("insecure"); insecure != "" {
		result["skip-cert-verify"] = parseBool(insecure)
	}
	if value := query.Get("sni"); value != "" {
		result["servername"] = value
	}
	if value := query.Get("alpn"); value != "" {
		result["alpn"] = strings.Split(value, ",")
	}

	switch scheme {
	case "vless", "trojan":
		for from, to := range map[string]string{"type": "network", "host": "host", "path": "path", "serviceName": "serviceName", "flow": "flow"} {
			if value := query.Get(from); value != "" {
				result[to] = value
			}
		}
		if value := query.Get("fp"); value != "" {
			result["client-fingerprint"] = value
		}
		if security == "reality" {
			opts := map[string]any{}
			for from, to := range map[string]string{"pbk": "public-key", "sid": "short-id", "spx": "spider-x"} {
				if value := query.Get(from); value != "" {
					opts[to] = value
				}
			}
			if len(opts) > 0 {
				result["reality-opts"] = opts
			}
		}
	case "hysteria", "hysteria2", "hy2":
		for from, to := range map[string]string{"obfs": "obfs", "obfs-password": "obfs-password", "up": "up", "down": "down", "auth": "auth"} {
			if value := query.Get(from); value != "" {
				result[to] = value
			}
		}
	case "tuic":
		for from, to := range map[string]string{"congestion_control": "congestion-controller", "udp_relay_mode": "udp-relay-mode"} {
			if value := query.Get(from); value != "" {
				result[to] = value
			}
		}
	}
}

func parseBool(value string) bool {
	return strings.EqualFold(value, "true") || value == "1" || strings.EqualFold(value, "yes")
}
func decodeBase64(value string) ([]byte, error) {
	value = strings.TrimSpace(value)
	for _, encoding := range []*base64.Encoding{base64.RawStdEncoding, base64.StdEncoding, base64.RawURLEncoding, base64.URLEncoding} {
		if decoded, err := encoding.DecodeString(value); err == nil {
			return decoded, nil
		}
	}
	return nil, fmt.Errorf("invalid Base64 encoding")
}

// looksLikeBase64 reports whether the body only contains Base64 alphabet
// characters and whitespace, so content that cannot be Base64 (YAML, HTML,
// plain URI lists) skips the compaction entirely.
func looksLikeBase64(data []byte) bool {
	for _, b := range data {
		switch {
		case b == ' ' || b == '\t' || b == '\r' || b == '\n':
			continue
		case b >= 'A' && b <= 'Z', b >= 'a' && b <= 'z', b >= '0' && b <= '9', b == '+', b == '/', b == '=':
			continue
		default:
			return false
		}
	}
	return len(data) > 0
}
func proxyList(value any) ([]map[string]any, error) {
	if m, ok := value.(map[string]any); ok {
		value = m["proxies"]
	}
	items, ok := value.([]any)
	if !ok {
		return nil, fmt.Errorf("content is not a proxy list or does not contain a proxies list")
	}
	out := make([]map[string]any, 0, len(items))
	for _, item := range items {
		if m, ok := item.(map[string]any); ok {
			out = append(out, m)
		}
	}
	return out, nil
}
