package subscription

import (
	"time"

	"github.com/pangolin/pangolin/internal/version"
)

func Build(nodes []Proxy) Output {
	if len(nodes) == 0 {
		nodes = []Proxy{{Name: "DIRECT-FALLBACK", Type: "socks5", Server: "127.0.0.1", Port: 1}}
	}
	all := names(nodes)
	groups := []Group{{Name: "AUTO-FAST", Type: "url-test", Proxies: all, URL: "https://www.gstatic.com/generate_204", Interval: 300}, {Name: "ALL", Type: "select", Proxies: all}, {Name: "FALLBACK", Type: "fallback", Proxies: []string{"AUTO-FAST", "ALL"}}, {Name: "PROXY", Type: "select", Proxies: []string{"AUTO-FAST", "FALLBACK", "ALL"}}}
	return Output{MixedPort: 7890, Mode: "rule", UnifiedDelay: true, TCPConcurrent: true, Proxies: nodes, Groups: groups, Rules: []string{"GEOIP,CN,DIRECT", "MATCH,PROXY"}, Metadata: map[string]string{"generated-by": "pangolin " + version.Version, "generated-at": time.Now().UTC().Format(time.RFC3339)}}
}
