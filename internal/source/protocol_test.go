package source

import (
	"encoding/base64"
	"strings"
	"testing"
)

func TestParseSSLegacyURI(t *testing.T) {
	encoded := base64.RawStdEncoding.EncodeToString([]byte("aes-256-gcm:secret@example.com:8388"))
	proxy, err := parseURI("ss://" + encoded + "#legacy")
	if err != nil || proxy["server"] != "example.com" || proxy["port"] != 8388 || proxy["name"] != "legacy" {
		t.Fatalf("unexpected legacy SS proxy: %v %#v", err, proxy)
	}
}

func TestParseSSURIWithoutName(t *testing.T) {
	proxy, err := parseURI("ss://YWVzLTI1Ni1nY206cGFzc0BleGFtcGxlLmNvbTo4Mzg4@ignored")
	if err != nil || proxy["server"] != "example.com" {
		t.Fatalf("unexpected SS proxy: %v %#v", err, proxy)
	}
}

func TestParseSSRIPv6URI(t *testing.T) {
	core := "[2001:db8::1]:443:auth_aes128_md5:aes-256-cfb:http_simple:" + base64.RawStdEncoding.EncodeToString([]byte("secret"))
	proxy, err := parseURI("ssr://" + base64.RawURLEncoding.EncodeToString([]byte(core)))
	if err != nil || proxy["server"] != "2001:db8::1" || proxy["port"] != 443 {
		t.Fatalf("unexpected SSR IPv6 proxy: %v %#v", err, proxy)
	}
}

func TestParseSSRPlainTextParam(t *testing.T) {
	core := "example.com:443:auth_aes128_md5:aes-256-cfb:http_simple:" + base64.RawStdEncoding.EncodeToString([]byte("secret"))
	inner := core + "/?remarks=test"
	proxy, err := parseURI("ssr://" + base64.RawURLEncoding.EncodeToString([]byte(inner)))
	if err != nil || proxy["name"] != "test" {
		t.Fatalf("unexpected SSR plain parameter: %v %#v", err, proxy)
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
func TestParseTUICURI(t *testing.T) {
	proxy, err := parseURI("tuic://uuid:secret@example.com:443?sni=example.com")
	if err != nil {
		t.Fatal(err)
	}
	if proxy["uuid"] != "uuid" || proxy["password"] != "secret" {
		t.Fatalf("unexpected TUIC credentials: %#v", proxy)
	}
	if nodes := normalize([]map[string]any{proxy}); len(nodes) != 1 {
		t.Fatalf("TUIC node was rejected during normalization: %#v", nodes)
	}
}

func TestParseHTTPAndSOCKS5Credentials(t *testing.T) {
	for _, raw := range []string{"http://user:secret@example.com:8080", "socks5://user:secret@example.com:1080", "https://user:secret@example.com:443"} {
		proxy, err := parseURI(raw)
		if err != nil {
			t.Fatal(err)
		}
		if proxy["username"] != "user" || proxy["password"] != "secret" {
			t.Fatalf("unexpected credentials for %s: %#v", raw, proxy)
		}
	}
}

func TestParseSSRURI(t *testing.T) {
	// ssr://base64url(server:port:protocol:method:obfs:base64(password)/?params)
	core := "example.com:443:auth_aes128_md5:aes-256-cfb:http_simple:" + base64.RawStdEncoding.EncodeToString([]byte("secret"))
	inner := core + "/?obfsparam=" + base64.RawURLEncoding.EncodeToString([]byte("obfs-secret")) + "&remarks=" + base64.RawURLEncoding.EncodeToString([]byte("ssr-node"))
	proxy, err := parseURI("ssr://" + base64.RawURLEncoding.EncodeToString([]byte(inner)))
	if err != nil {
		t.Fatal(err)
	}
	if proxy["type"] != "ssr" || proxy["server"] != "example.com" || proxy["port"] != 443 || proxy["cipher"] != "aes-256-cfb" || proxy["password"] != "secret" || proxy["protocol"] != "auth_aes128_md5" || proxy["obfs"] != "http_simple" {
		t.Fatalf("unexpected SSR proxy: %#v", proxy)
	}
	if proxy["name"] != "ssr-node" || proxy["obfs-param"] != "obfs-secret" {
		t.Fatalf("unexpected SSR params: %#v", proxy)
	}
	if nodes := normalize([]map[string]any{proxy}); len(nodes) != 1 {
		t.Fatalf("SSR node was rejected during normalization: %#v", nodes)
	}
}

func TestParseSSRPasswordEndingInSlash(t *testing.T) {
	// A password whose RAW base64 ends with '/' (e.g. bytes {0xFF,0xFF,0xFF}
	// encode to "////") produces a double slash in the canonical link: one
	// from the base64 itself and one structural '/' before the params.
	// parseSSRURI must keep all four slashes of the password and trim only
	// the structural one.
	password := string([]byte{0xFF, 0xFF, 0xFF})
	core := "example.com:443:auth_aes128_md5:aes-256-cfb:http_simple:" + base64.RawStdEncoding.EncodeToString([]byte(password)) + "/?remarks=" + base64.RawURLEncoding.EncodeToString([]byte("slash-node"))
	proxy, err := parseURI("ssr://" + base64.RawURLEncoding.EncodeToString([]byte(core)))
	if err != nil {
		t.Fatal(err)
	}
	if proxy["password"] != password {
		t.Fatalf("trailing slash of the password was trimmed: %q != %q", proxy["password"], password)
	}
	if proxy["name"] != "slash-node" {
		t.Fatalf("unexpected SSR name: %#v", proxy)
	}
}

func TestParseURIAppliesConventionalPorts(t *testing.T) {
	proxy, err := parseURI("https://example.com")
	if err != nil || proxy["port"] != 443 || proxy["type"] != "http" || proxy["tls"] != true {
		t.Fatalf("unexpected https default port: %#v %v", proxy, err)
	}
	proxy, err = parseURI("socks5://user:secret@example.com")
	if err != nil || proxy["port"] != 1080 || proxy["username"] != "user" || proxy["password"] != "secret" {
		t.Fatalf("unexpected socks5 default port: %#v %v", proxy, err)
	}
}

func TestParseSSPluginURI(t *testing.T) {
	encoded := base64.RawStdEncoding.EncodeToString([]byte("aes-256-gcm:secret"))
	proxy, err := parseURI("ss://" + encoded + "@example.com:443/?plugin=v2ray-plugin%3Bmode%3Dwebsocket%3Bhost%3Dcdn.example.com#ss-node")
	if err != nil {
		t.Fatal(err)
	}
	if proxy["plugin"] != "v2ray-plugin" || proxy["name"] != "ss-node" {
		t.Fatalf("unexpected SS plugin: %#v", proxy)
	}
	opts, ok := proxy["plugin-opts"].(map[string]any)
	if !ok || opts["mode"] != "websocket" || opts["host"] != "cdn.example.com" {
		t.Fatalf("unexpected plugin options: %#v", proxy["plugin-opts"])
	}
}

func TestParseVLESSWSRealityURI(t *testing.T) {
	proxy, err := parseURI("vless://uuid@example.com:443?type=ws&host=cdn.example.com&path=%2Fws&security=reality&sni=cdn.example.com&pbk=public&sid=short&spx=%2F&fp=chrome")
	if err != nil {
		t.Fatal(err)
	}
	if proxy["network"] != "ws" || proxy["host"] != "cdn.example.com" || proxy["path"] != "/ws" || proxy["client-fingerprint"] != "chrome" {
		t.Fatalf("unexpected VLESS fields: %#v", proxy)
	}
	opts, ok := proxy["reality-opts"].(map[string]any)
	if !ok || opts["public-key"] != "public" || opts["short-id"] != "short" || opts["spider-x"] != "/" {
		t.Fatalf("unexpected Reality options: %#v", proxy["reality-opts"])
	}
	nodes := normalize([]map[string]any{proxy})
	if len(nodes) != 1 {
		t.Fatalf("VLESS node was rejected during normalization: %#v", nodes)
	}
	if _, ok := nodes[0].Fields["ws-opts"].(map[string]any); !ok {
		t.Fatalf("VLESS ws-opts were not generated: %#v", nodes[0].Fields)
	}
}

func TestParseBase64URIList(t *testing.T) {
	content := "trojan://secret@example.com:443?security=tls\n"
	encoded := base64.StdEncoding.EncodeToString([]byte(content))
	proxies, err := parse([]byte(encoded))
	if err != nil || len(proxies) != 1 || proxies[0]["type"] != "trojan" {
		t.Fatalf("unexpected Base64 URI list: %v %#v", err, proxies)
	}
}

func TestParseVMessCipher(t *testing.T) {
	payload := `{"add":"example.com","port":443,"id":"uuid","aid":0,"scy":"chacha20-poly1305","net":"ws"}`
	encoded := base64.RawStdEncoding.EncodeToString([]byte(payload))
	proxy, err := parseURI("vmess://" + encoded)
	if err != nil || proxy["cipher"] != "chacha20-poly1305" {
		t.Fatalf("unexpected VMess cipher: %v %#v", err, proxy)
	}
}

func TestParseHy2Alias(t *testing.T) {
	proxies, err := parseURIs("hy2://secret@example.com:443?sni=example.com")
	if err != nil || len(proxies) != 1 || proxies[0]["type"] != "hy2" {
		t.Fatalf("unexpected hy2 result: %v %#v", err, proxies)
	}
}

func TestParseURIsIgnoresMalformedEntries(t *testing.T) {
	proxies, err := parseURIs(strings.Join([]string{"not-a-uri", "trojan://secret@example.com:443"}, "\n"))
	if err != nil || len(proxies) != 1 {
		t.Fatalf("unexpected URI result: %v %#v", err, proxies)
	}
}
