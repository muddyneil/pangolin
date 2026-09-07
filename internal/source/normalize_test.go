package source

import "testing"

func TestNormalizeRejectsLocalServers(t *testing.T) {
	for _, server := range []string{"127.0.0.1", "10.0.0.1", "::1", "localhost"} {
		nodes := normalize([]map[string]any{{"name": "local", "type": "http", "server": server, "port": 80}})
		if len(nodes) != 0 {
			t.Fatalf("local server %q was accepted: %#v", server, nodes)
		}
	}
}
func TestNormalizeRejectsFractionalPort(t *testing.T) {
	nodes := normalize([]map[string]any{{"type": "http", "server": "example.com", "port": 443.5}})
	if len(nodes) != 0 {
		t.Fatalf("fractional port was accepted: %#v", nodes)
	}
}
func TestNormalizeRejectsInvalidNestedOptions(t *testing.T) {
	nodes := normalize([]map[string]any{{
		"name":    "invalid",
		"type":    "vless",
		"server":  "example.com",
		"port":    443,
		"uuid":    "id",
		"h2-opts": "not-a-map",
	}})
	if len(nodes) != 1 {
		t.Fatalf("invalid nested options should be removed, got %#v", nodes)
	}
	if _, ok := nodes[0].Fields["h2-opts"]; ok {
		t.Fatalf("invalid h2-opts was retained: %#v", nodes[0].Fields)
	}
}
func TestNormalizeAssignsName(t *testing.T) {
	nodes := normalize([]map[string]any{{"type": "http", "server": "example.com", "port": 80}})
	if len(nodes) != 1 || nodes[0].Name != "node-1" {
		t.Fatalf("unnamed node was not retained: %#v", nodes)
	}
}

func TestNormalizeTypeCaseInsensitive(t *testing.T) {
	nodes := normalize([]map[string]any{{"name": "upper", "type": "Vless", "server": "example.com", "port": 443, "uuid": "id"}})
	if len(nodes) != 1 || nodes[0].Type != "vless" {
		t.Fatalf("capitalized type was not normalized: %#v", nodes)
	}
}

func TestNormalizeHysteria2AcceptsAuthCredential(t *testing.T) {
	nodes := normalize([]map[string]any{{"name": "auth-node", "type": "hysteria2", "server": "example.com", "port": 443, "auth": "token"}})
	if len(nodes) != 1 {
		t.Fatalf("hysteria2 auth node was rejected: %#v", nodes)
	}
	if nodes[0].Fields["auth"] != "token" {
		t.Fatalf("unexpected auth field: %#v", nodes[0].Fields)
	}
}

func TestNormalizeKeepsDistinctCredentialsAtSameEndpoint(t *testing.T) {
	nodes := normalize([]map[string]any{
		{"name": "one", "type": "vless", "server": "example.com", "port": 443, "uuid": "first"},
		{"name": "two", "type": "vless", "server": "example.com", "port": 443, "uuid": "second"},
	})
	if len(nodes) != 2 {
		t.Fatalf("distinct nodes at one endpoint were merged: %#v", nodes)
	}
}

func TestNormalizeMapsFlatWSFieldsToWSOpts(t *testing.T) {
	nodes := normalize([]map[string]any{
		{"name": "flat", "type": "vmess", "server": "example.com", "port": 443, "uuid": "id", "network": "ws", "host": "cdn.example.com", "path": "/ws", "tls": true},
	})
	if len(nodes) != 1 {
		t.Fatalf("unexpected nodes: %#v", nodes)
	}
	fields := nodes[0].Fields
	opts, ok := fields["ws-opts"].(map[string]any)
	if !ok {
		t.Fatalf("missing ws-opts, got %#v", fields)
	}
	if opts["path"] != "/ws" {
		t.Fatalf("unexpected ws-opts path: %#v", opts)
	}
	headers, ok := opts["headers"].(map[string]any)
	if !ok || headers["Host"] != "cdn.example.com" {
		t.Fatalf("unexpected ws-opts headers: %#v", opts)
	}
	if _, exists := fields["host"]; exists {
		t.Fatalf("flat host was not removed: %#v", fields)
	}
	if _, exists := fields["path"]; exists {
		t.Fatalf("flat path was not removed: %#v", fields)
	}
}

func TestNormalizePrefersExistingWSOpts(t *testing.T) {
	nodes := normalize([]map[string]any{
		{"name": "canonical", "type": "vless", "server": "example.com", "port": 443, "uuid": "id", "network": "ws", "ws-opts": map[string]any{"path": "/canonical"}, "host": "legacy.example.com"},
	})
	if len(nodes) != 1 {
		t.Fatalf("unexpected nodes: %#v", nodes)
	}
	fields := nodes[0].Fields
	opts, ok := fields["ws-opts"].(map[string]any)
	if !ok || opts["path"] != "/canonical" {
		t.Fatalf("canonical ws-opts lost: %#v", fields)
	}
	if _, exists := fields["host"]; exists {
		t.Fatalf("legacy host kept alongside ws-opts: %#v", fields)
	}
}

func TestNormalizeMapsFlatGRPCFieldsToGRPCOpts(t *testing.T) {
	nodes := normalize([]map[string]any{
		{"name": "grpc", "type": "vless", "server": "example.com", "port": 443, "uuid": "id", "network": "grpc", "serviceName": "svc"},
	})
	if len(nodes) != 1 {
		t.Fatalf("unexpected nodes: %#v", nodes)
	}
	fields := nodes[0].Fields
	opts, ok := fields["grpc-opts"].(map[string]any)
	if !ok || opts["grpc-service-name"] != "svc" {
		t.Fatalf("unexpected grpc-opts: %#v", fields)
	}
}

func TestRegionWordBoundaries(t *testing.T) {
	cases := []struct {
		name, server string
		want         string
	}{
		{name: "hk-01.example.com", want: "HK"},
		{name: "HKG - example", want: "OTHER"}, // \bhk\b must not match inside HKG
		{name: "zhuhai.uk", want: "OTHER"},     // hk inside zhuhai must not match
		{name: "us-west-1", want: "US"},
		{name: "usa-01", want: "US"},
		{name: "house.example.com", want: "OTHER"}, // us inside house must not match
		{name: "🇭🇰 香港 | HKG", want: "HK"},
		{name: "🇯🇵 日本 | JPN", want: "JP"},
		{name: "🇺🇸 美国 | USA", want: "US"},
		{name: "japan-tokyo", want: "JP"},
		{name: "hong kong - hk", want: "HK"},
		{name: "america.us", want: "US"},
		{name: "unrelated.example.com", want: "OTHER"},
		// An unambiguous server location wins over an ambiguous name, and
		// ties keep declaration order (HK, JP, US).
		{name: "香港-USA", server: "us-lax-01.example.com", want: "US"},
		{name: "日本 - hk", server: "jp-tokyo.example.com", want: "JP"},
	}
	for _, tc := range cases {
		if got := region(tc.name, tc.server); got != tc.want {
			t.Fatalf("region(%q, %q) = %q, want %q", tc.name, tc.server, got, tc.want)
		}
	}
}

func TestNormalizeCompletesVMessDefaults(t *testing.T) {
	nodes := normalize([]map[string]any{
		{"name": "plain", "type": "vmess", "server": "example.com", "port": 443, "uuid": "id1"},
		{"name": "explicit", "type": "vmess", "server": "example.com", "port": 443, "uuid": "id2", "alterId": 1, "cipher": "aes-128-gcm"},
	})
	if len(nodes) != 2 {
		t.Fatalf("unexpected nodes: %#v", nodes)
	}
	if nodes[0].Fields["alterId"] != 0 || nodes[0].Fields["cipher"] != "auto" {
		t.Fatalf("vmess defaults not applied: %#v", nodes[0].Fields)
	}
	if nodes[1].Fields["alterId"] != 1 || nodes[1].Fields["cipher"] != "aes-128-gcm" {
		t.Fatalf("explicit vmess fields overridden: %#v", nodes[1].Fields)
	}
}

func TestNormalizeHysteriaRequiresAuth(t *testing.T) {
	accepted := normalize([]map[string]any{{"name": "hy-auth", "type": "hysteria", "server": "example.com", "port": 443, "auth": "secret"}})
	if len(accepted) != 1 {
		t.Fatalf("hysteria auth node was rejected: %#v", accepted)
	}
	accepted = normalize([]map[string]any{{"name": "hy-auth-str", "type": "hysteria", "server": "example.com", "port": 443, "auth-str": "secret"}})
	if len(accepted) != 1 {
		t.Fatalf("hysteria auth-str node was rejected: %#v", accepted)
	}
	rejected := normalize([]map[string]any{{"name": "hy-bare", "type": "hysteria", "server": "example.com", "port": 443}})
	if len(rejected) != 0 {
		t.Fatalf("hysteria node without a credential was accepted: %#v", rejected)
	}
}

func TestNormalizeMapsFlatHTTPFieldsToHTTPOpts(t *testing.T) {
	nodes := normalize([]map[string]any{
		{"name": "flat-http", "type": "vmess", "server": "example.com", "port": 443, "uuid": "id", "network": "http", "host": "cdn.example.com", "path": "/api"},
	})
	if len(nodes) != 1 {
		t.Fatalf("unexpected nodes: %#v", nodes)
	}
	fields := nodes[0].Fields
	opts, ok := fields["http-opts"].(map[string]any)
	if !ok {
		t.Fatalf("missing http-opts, got %#v", fields)
	}
	if opts["path"] != "/api" {
		t.Fatalf("unexpected http-opts path: %#v", opts)
	}
	headers, ok := opts["headers"].(map[string]any)
	if !ok || headers["Host"] != "cdn.example.com" {
		t.Fatalf("unexpected http-opts headers: %#v", opts)
	}
	if _, exists := fields["host"]; exists {
		t.Fatalf("flat host was not removed: %#v", fields)
	}
	if _, exists := fields["path"]; exists {
		t.Fatalf("flat path was not removed: %#v", fields)
	}
}
