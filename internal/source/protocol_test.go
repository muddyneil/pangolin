package source

import "testing"

func TestParseSSURIWithoutName(t *testing.T) {
	proxy, err := parseURI("ss://YWVzLTI1Ni1nY206cGFzc0BleGFtcGxlLmNvbTo4Mzg4@ignored")
	if err != nil || proxy["server"] != "example.com" {
		t.Fatalf("unexpected SS proxy: %v %#v", err, proxy)
	}
}

func TestParseVLESSURI(t *testing.T) {
	proxy, err := parseURI("vless://uuid@example.com:443?security=tls&type=ws&sni=cdn.example.com#vless")
	if err != nil {
		t.Fatal(err)
	}
	if proxy["type"] != "vless" || proxy["uuid"] != "uuid" || proxy["tls"] != true || proxy["network"] != "ws" || proxy["servername"] != "cdn.example.com" {
		t.Fatalf("unexpected vless proxy: %#v", proxy)
	}
}

func TestParseTrojanURI(t *testing.T) {
	proxy, err := parseURI("trojan://secret@example.com:443?security=tls&sni=example.com#trojan")
	if err != nil {
		t.Fatal(err)
	}
	if proxy["type"] != "trojan" || proxy["password"] != "secret" || proxy["tls"] != true || proxy["servername"] != "example.com" {
		t.Fatalf("unexpected trojan proxy: %#v", proxy)
	}
}
