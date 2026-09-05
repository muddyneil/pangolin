package source

import "testing"

func TestNormalizeRejectsOversizedProxy(t *testing.T) {
	large := make(map[string]any)
	large["name"], large["type"], large["server"], large["port"] = "large", "ss", "example.com", 443
	large["password"] = string(make([]byte, MaxProxyBytes))
	if nodes := normalize([]map[string]any{large}); len(nodes) != 0 {
		t.Fatalf("oversized proxy was retained: %d", len(nodes))
	}
}
