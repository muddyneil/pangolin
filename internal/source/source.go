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
			if err == nil {
				err = fmt.Errorf("未找到代理节点")
			}
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
	var last error
	remaining := len(urls)
	for remaining > 0 {
		select {
		case item := <-results:
			remaining--
			if item.err != nil {
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
				last = fmt.Errorf("未找到代理节点")
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
	if last == nil {
		last = fmt.Errorf("未找到代理节点")
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

func (e httpStatusError) Error() string { return fmt.Sprintf("源返回 HTTP %d", e.Status) }

func retryableStatus(status int) bool {
	return status == http.StatusRequestTimeout || status == http.StatusTooEarly || status == http.StatusTooManyRequests || status >= 500
}

func fetch(ctx context.Context, raw string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, raw, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "pangolin/0.01.00")
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
		return nil, fmt.Errorf("读取源响应失败: %w", err)
	}
	return data, nil
}

func parse(data []byte) ([]map[string]any, error) {
	if int64(len(data)) > MaxResponseBytes {
		return nil, fmt.Errorf("响应超过 %d 字节", MaxResponseBytes)
	}
	var value any
	if json.Unmarshal(data, &value) == nil {
		if proxies, err := proxyList(value); err == nil && len(proxies) > 0 {
			return proxies, nil
		}
	}
	compact := strings.Join(strings.Fields(string(data)), "")
	decoded, err := decodeBase64(compact)
	if err == nil && json.Unmarshal(decoded, &value) == nil {
		if proxies, parseErr := proxyList(value); parseErr == nil && len(proxies) > 0 {
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
		if strings.HasPrefix(line, "ss://") || strings.HasPrefix(line, "trojan://") || strings.HasPrefix(line, "vless://") || strings.HasPrefix(line, "vmess://") || strings.HasPrefix(line, "hysteria2://") || strings.HasPrefix(line, "tuic://") || strings.HasPrefix(line, "socks5://") || strings.HasPrefix(line, "http://") || strings.HasPrefix(line, "https://") {
			if proxy, err := parseURI(line); err == nil {
				result = append(result, proxy)
			}
		}
	}
	if len(result) == 0 {
		return nil, fmt.Errorf("内容不是支持的代理列表")
	}
	return result, nil
}

func parseURI(raw string) (map[string]any, error) {
	if strings.HasPrefix(raw, "vmess://") {
		encoded := strings.TrimPrefix(strings.SplitN(raw, "#", 2)[0], "vmess://")
		decoded, err := decodeBase64(encoded)
		if err != nil {
			return nil, fmt.Errorf("VMess 编码无效")
		}
		var input map[string]any
		if err := json.Unmarshal(decoded, &input); err != nil {
			return nil, fmt.Errorf("VMess JSON 无效")
		}
		server, _ := input["add"].(string)
		port := number(input["port"])
		uuid, _ := input["id"].(string)
		if server == "" || port < 1 || uuid == "" {
			return nil, fmt.Errorf("VMess 必填字段缺失")
		}
		result := map[string]any{"name": server, "type": "vmess", "server": server, "port": port, "uuid": uuid}
		for from, to := range map[string]string{"aid": "alterId", "net": "network", "host": "host", "path": "path", "tls": "tls", "sni": "servername"} {
			if value, ok := input[from]; ok {
				result[to] = value
			}
		}
		if name, ok := input["ps"].(string); ok && name != "" {
			result["name"] = name
		}
		return result, nil
	}
	if strings.HasPrefix(raw, "ss://") {
		parts := strings.SplitN(strings.TrimPrefix(strings.SplitN(raw, "#", 2)[0], "ss://"), "@", 2)
		if len(parts) != 2 {
			return nil, fmt.Errorf("SS 内容无效")
		}
		decoded, err := base64.RawStdEncoding.DecodeString(parts[0])
		if err != nil {
			decoded, err = base64.StdEncoding.DecodeString(parts[0])
		}
		if err != nil {
			return nil, fmt.Errorf("SS 编码无效")
		}
		auth := strings.SplitN(string(decoded), "@", 2)
		if len(auth) != 2 {
			return nil, fmt.Errorf("SS 内容无效")
		}
		host, portText, err := net.SplitHostPort(auth[1])
		if err != nil {
			return nil, fmt.Errorf("SS 地址无效")
		}
		port, err := strconv.Atoi(portText)
		if err != nil {
			return nil, fmt.Errorf("代理端口无效")
		}
		credentials := strings.SplitN(auth[0], ":", 2)
		result := map[string]any{"name": host, "type": "ss", "server": host, "port": port}
		if len(credentials) == 2 {
			result["cipher"], result["password"] = credentials[0], credentials[1]
		}
		if parts := strings.SplitN(raw, "#", 2); len(parts) == 2 && parts[1] != "" {
			result["name"] = parts[1]
		}
		return result, nil
	}
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Hostname() == "" {
		return nil, fmt.Errorf("代理 URI 无效")
	}
	port, err := strconv.Atoi(u.Port())
	if err != nil {
		return nil, fmt.Errorf("代理端口无效")
	}
	result := map[string]any{"name": u.Hostname(), "type": u.Scheme, "server": u.Hostname(), "port": port}
	if u.Scheme == "https" {
		result["type"], result["tls"] = "http", true
	}
	if u.User != nil {
		username := u.User.Username()
		if username != "" {
			if u.Scheme == "vless" {
				result["uuid"] = username
			} else {
				result["password"] = username
			}
		}
		if password, ok := u.User.Password(); ok {
			result["password"] = password
		}
	}
	query := u.Query()
	if query.Get("security") == "tls" || query.Get("security") == "reality" {
		result["tls"] = true
	}
	if sni := query.Get("sni"); sni != "" {
		result["servername"] = sni
	}
	if network := query.Get("type"); network != "" {
		result["network"] = network
	}
	if flow := query.Get("flow"); flow != "" {
		result["flow"] = flow
	}
	if u.Fragment != "" {
		result["name"] = u.Fragment
	}
	return result, nil
}
func decodeBase64(value string) ([]byte, error) {
	value = strings.TrimSpace(value)
	if decoded, err := base64.RawStdEncoding.DecodeString(value); err == nil {
		return decoded, nil
	}
	return base64.StdEncoding.DecodeString(value)
}
func proxyList(value any) ([]map[string]any, error) {
	if m, ok := value.(map[string]any); ok {
		value = m["proxies"]
	}
	items, ok := value.([]any)
	if !ok {
		return nil, fmt.Errorf("内容不是代理列表或包含 proxies 列表")
	}
	out := make([]map[string]any, 0, len(items))
	for _, item := range items {
		if m, ok := item.(map[string]any); ok {
			out = append(out, m)
		}
	}
	return out, nil
}
