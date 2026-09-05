package source

import "testing"

func TestSimpleYAMLTopLevelList(t *testing.T) {
	items, err := parseSimpleYAML("- name: node\n  type: vless\n  server: example.com\n  port: 443\n  uuid: id\n")
	if err != nil || len(items) != 1 || items[0]["name"] != "node" {
		t.Fatalf("unexpected top-level list: %v %#v", err, items)
	}
}

func TestSimpleYAMLMultilineList(t *testing.T) {
	items, err := parseSimpleYAML("proxies:\n  - name: node\n    type: vless\n    server: example.com\n    port: 443\n    uuid: id\n    alpn:\n    - h2\n    - http/1.1\n")
	if err != nil || len(items) != 1 {
		t.Fatalf("unexpected parse result: %v %#v", err, items)
	}
	list, ok := items[0]["alpn"].([]interface{})
	if !ok || len(list) != 2 || list[1] != "http/1.1" {
		t.Fatalf("unexpected alpn: %#v", items[0]["alpn"])
	}
}
