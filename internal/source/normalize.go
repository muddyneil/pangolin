package source

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

var allowed = map[string]bool{"ss": true, "ssr": true, "vmess": true, "vless": true, "trojan": true, "hysteria": true, "hysteria2": true, "hy2": true, "tuic": true, "socks5": true, "http": true}

var allowedFields = map[string]bool{"cipher": true, "password": true, "uuid": true, "alterId": true, "network": true, "host": true, "path": true, "tls": true, "servername": true, "sni": true, "alpn": true, "udp": true, "udp-over-tcp": true, "tfo": true, "mptcp": true, "ip-version": true, "fingerprint": true, "fp": true, "encryption": true, "packet-encoding": true, "ports": true, "token": true, "serviceName": true, "pbk": true, "sid": true, "spx": true, "plugin": true, "plugin-opts": true, "protocol": true, "protocol-param": true, "obfs": true, "obfs-param": true, "obfs-password": true, "auth": true, "auth-str": true, "up": true, "down": true, "ca": true, "ca-str": true, "recv-window-conn": true, "recv-window": true, "disable-mtu-discovery": true, "fast-open": true, "hop-interval": true, "congestion-controller": true, "congestion_control": true, "udp-relay-mode": true, "reduce-rtt": true, "ws-opts": true, "http-opts": true, "h2-opts": true, "grpc-opts": true, "reality-opts": true, "flow": true, "client-fingerprint": true, "auth-name": true, "auth-password": true}

func fields(m map[string]any) map[string]any {
	result := map[string]any{}
	for key, value := range m {
		if !allowedFields[key] {
			continue
		}
		if key == "ws-opts" || key == "grpc-opts" || key == "reality-opts" {
			if _, ok := value.(map[string]any); !ok {
				continue
			}
		}
		if key == "tls" || key == "udp" {
			if text, ok := value.(string); ok {
				value = strings.EqualFold(text, "tls") || strings.EqualFold(text, "true") || strings.EqualFold(text, "yes") || strings.EqualFold(text, "1")
			}
		}
		result[key] = value
	}
	return result
}

var requiredFields = map[string][]string{"ss": {"password", "cipher"}, "ssr": {"password", "cipher"}, "vmess": {"uuid"}, "vless": {"uuid"}, "trojan": {"password"}, "hysteria2": {"password"}, "tuic": {"uuid", "password"}}

func hasRequired(m map[string]any, typ string) bool {
	for _, key := range requiredFields[typ] {
		value, ok := m[key]
		if !ok || value == nil || fmt.Sprint(value) == "" {
			return false
		}
	}
	return true
}

func normalize(items []map[string]any) []Proxy {
	out := make([]Proxy, 0, len(items))
	seen := map[string]bool{}
	for _, m := range items {
		raw, err := json.Marshal(m)
		if err != nil || len(raw) > MaxProxyBytes {
			continue
		}
		name, _ := m["name"].(string)
		typ, _ := m["type"].(string)
		server, _ := m["server"].(string)
		port := number(m["port"])
		if typ == "hy2" {
			typ = "hysteria2"
		}
		if name == "" {
			name = fmt.Sprintf("node-%d", len(out)+1)
		}
		if !allowed[typ] || server == "" || port < 1 || port > 65535 {
			continue
		}
		if !hasRequired(m, typ) {
			continue
		}
		if typ == "vmess" {
			// Mihomo's -t rejects vmess nodes without alterId and cipher.
			if _, ok := m["alterId"]; !ok {
				m["alterId"] = 0
			}
			if _, ok := m["cipher"]; !ok {
				m["cipher"] = "auto"
			}
		}
		normalizeTransport(m)
		p := Proxy{Name: name, Type: typ, Server: server, Port: port, Fields: fields(m)}
		p.Fingerprint = fingerprint(p)
		p.Region = region(name + " " + server)
		if seen[p.Fingerprint] {
			continue
		}
		seen[p.Fingerprint] = true
		out = append(out, p)
	}
	return out
}

// normalizeTransport converts flat v2ray-style transport fields into the
// ws-opts/h2-opts/grpc-opts maps that Mihomo consumes. If the upstream already
// declares the canonical opts map, the legacy top-level fields are dropped so
// the canonical form wins. This keeps flat and canonical forms of the same node
// deduping to the same fingerprint.
func normalizeTransport(m map[string]any) {
	network, _ := m["network"].(string)
	switch network {
	case "ws":
		if _, ok := m["ws-opts"].(map[string]any); ok {
			delete(m, "host")
			delete(m, "path")
			return
		}
		host, _ := m["host"].(string)
		path, _ := m["path"].(string)
		if host == "" && path == "" {
			return
		}
		opts := map[string]any{}
		if path != "" {
			opts["path"] = path
		}
		if host != "" {
			opts["headers"] = map[string]any{"Host": host}
		}
		m["ws-opts"] = opts
		delete(m, "host")
		delete(m, "path")
	case "h2":
		if _, ok := m["h2-opts"].(map[string]any); ok {
			delete(m, "host")
			delete(m, "path")
			return
		}
		host, _ := m["host"].(string)
		path, _ := m["path"].(string)
		if host == "" && path == "" {
			return
		}
		opts := map[string]any{}
		if path != "" {
			opts["path"] = path
		}
		if host != "" {
			opts["host"] = []string{host}
		}
		m["h2-opts"] = opts
		delete(m, "host")
		delete(m, "path")
	case "grpc":
		if _, ok := m["grpc-opts"].(map[string]any); ok {
			delete(m, "serviceName")
			delete(m, "service-name")
			return
		}
		name, _ := m["serviceName"].(string)
		if name == "" {
			name, _ = m["service-name"].(string)
		}
		if name != "" {
			m["grpc-opts"] = map[string]any{"grpc-service-name": name}
		}
		delete(m, "serviceName")
		delete(m, "service-name")
	}
}

func number(v any) int {
	switch n := v.(type) {
	case int:
		return n
	case int64:
		return int(n)
	case float64:
		return int(n)
	case string:
		i, _ := strconv.Atoi(n)
		return i
	}
	return 0
}
func fingerprint(p Proxy) string {
	identity := make(map[string]any, len(p.Fields)+3)
	for key, value := range p.Fields {
		identity[key] = value
	}
	identity["type"] = p.Type
	identity["server"] = p.Server
	identity["port"] = p.Port
	payload, _ := json.Marshal(identity)
	return fmt.Sprintf("%x", sha256.Sum256(payload))
}

// region maps a node's name/server text to a country pool using word-boundary
// patterns plus common CJK and flag-emoji markers (mirroring the reference
// generator's detect_region). Word boundaries keep substrings like "us" in
// "house" or "hk" in "zhuhai" from misclassifying nodes.
var regionPatterns = []struct {
	name    string
	pattern *regexp.Regexp
}{
	{"HK", regexp.MustCompile(`(?i)\bhk\b|\bhong\s*kong\b|香港|🇭🇰`)},
	{"JP", regexp.MustCompile(`(?i)\bjp\b|japan|日本|🇯🇵`)},
	{"US", regexp.MustCompile(`(?i)\bus\b|\busa\b|united states|america|美国|美國|🇺🇸`)},
}

func region(s string) string {
	for _, group := range regionPatterns {
		if group.pattern.MatchString(s) {
			return group.name
		}
	}
	return "OTHER"
}
