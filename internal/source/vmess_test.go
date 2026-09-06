package source

import (
	"encoding/base64"
	"testing"
)

func TestParseMultilineBase64JSON(t *testing.T) {
	encoded := base64.StdEncoding.EncodeToString([]byte(`[{"name":"node","type":"http","server":"example.com","port":80}]`))
	encoded = encoded[:8] + "\n" + encoded[8:]
	items, err := parse([]byte(encoded))
	if err != nil || len(items) != 1 {
		t.Fatalf("unexpected multiline base64 result: %v %#v", err, items)
	}
}

func TestParseVMessURI(t *testing.T) {
	proxy, err := parseURI("vmess://eyJ2IjoiMiIsInBzIjoiZGVtbyIsImFkZCI6ImV4YW1wbGUuY29tIiwicG9ydCI6NDQzLCJpZCI6InV1aWQiLCJhaWQiOjAsIm5ldCI6IndzIiwidGxzIjoidGxzIiwic25pIjoiY2RuLmV4YW1wbGUuY29tIn0=")
	if err != nil {
		t.Fatal(err)
	}
	if proxy["type"] != "vmess" || proxy["name"] != "demo" || proxy["uuid"] != "uuid" || proxy["network"] != "ws" || proxy["servername"] != "cdn.example.com" {
		t.Fatalf("unexpected VMess proxy: %#v", proxy)
	}
}
