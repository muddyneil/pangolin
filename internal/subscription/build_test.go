package subscription

import (
	"strings"
	"testing"
)

func TestFallbackOutputIsValid(t *testing.T) {
	out := Build(nil)
	data, err := Marshal(out)
	if err != nil {
		t.Fatal(err)
	}
	if len(data) == 0 || len(out.Proxies) != 1 || out.Proxies[0].Name != "DIRECT-FALLBACK" {
		t.Fatalf("invalid fallback output: %s", data)
	}
}

func TestProxyFieldsAreSerialized(t *testing.T) {
	out := Build([]Proxy{{Name: "node", Type: "vmess", Server: "example.com", Port: 443, Fields: map[string]any{"uuid": "id", "tls": true}}})
	data, err := Marshal(out)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) == "" || !containsAll(string(data), "\"uuid\": \"id\"", "\"tls\": true") {
		t.Fatalf("missing proxy fields: %s", data)
	}
}

func containsAll(text string, values ...string) bool {
	for _, value := range values {
		if !strings.Contains(text, value) {
			return false
		}
	}
	return true
}

func TestAutoFastContainsAllPublishedNodes(t *testing.T) {
	nodes := make([]Proxy, 11)
	for i := range nodes {
		nodes[i] = Proxy{Name: "node-" + string(rune('a'+i)), Type: "ss", Server: "example.com", Port: i + 1}
	}
	out := Build(nodes)
	var auto, all Group
	for _, group := range out.Groups {
		switch group.Name {
		case "AUTO-FAST":
			auto = group
		case "ALL":
			all = group
		}
	}
	if auto.Type != "url-test" || all.Type != "select" || strings.Join(auto.Proxies, "\x00") != strings.Join(all.Proxies, "\x00") {
		t.Fatalf("AUTO-FAST and ALL differ: %#v %#v", auto, all)
	}
}

func TestRejectsDanglingReference(t *testing.T) {
	out := Build(nil)
	out.Groups[0].Proxies = []string{"missing"}
	if err := Validate(out); err == nil {
		t.Fatal("expected dangling reference error")
	}
}
