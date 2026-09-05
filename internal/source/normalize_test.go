package source

import "testing"

func TestNormalizeAssignsName(t *testing.T) {
	nodes := normalize([]map[string]any{{"type": "http", "server": "example.com", "port": 80}})
	if len(nodes) != 1 || nodes[0].Name != "node-1" {
		t.Fatalf("unnamed node was not retained: %#v", nodes)
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
		text string
		want string
	}{
		{"hk-01.example.com", "HK"},
		{"HKG - example", "OTHER"}, // \bhk\b must not match inside HKG
		{"zhuhai.uk", "OTHER"},     // hk inside zhuhai must not match
		{"us-west-1", "US"},
		{"usa-01", "US"},
		{"house.example.com", "OTHER"}, // us inside house must not match
		{"🇭🇰 香港 | HKG", "HK"},
		{"🇯🇵 日本 | JPN", "JP"},
		{"🇺🇸 美国 | USA", "US"},
		{"japan-tokyo", "JP"},
		{"hong kong - hk", "HK"},
		{"america.us", "US"},
		{"unrelated.example.com", "OTHER"},
	}
	for _, tc := range cases {
		if got := region(tc.text); got != tc.want {
			t.Fatalf("region(%q) = %q, want %q", tc.text, got, tc.want)
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
