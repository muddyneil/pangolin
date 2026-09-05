package subscription

import "time"

func Build(nodes []Proxy) Output {
	if len(nodes) == 0 {
		nodes = []Proxy{{Name: "DIRECT-FALLBACK", Type: "socks5", Server: "127.0.0.1", Port: 1}}
	}
	all := names(nodes)
	ai := all
	if len(ai) > 10 {
		ai = ai[:10]
	}
	groups := []Group{{Name: "AUTO-FAST", Type: "url-test", Proxies: all, URL: "https://www.gstatic.com/generate_204", Interval: 300}}
	for _, region := range []string{"HK", "JP", "US"} {
		members := namesByRegion(nodes, region)
		if len(members) == 0 {
			members = all
		}
		groups = append(groups, Group{Name: region + "-POOL", Type: "url-test", Proxies: members, URL: "https://www.gstatic.com/generate_204", Interval: 300})
	}
	groups = append(groups, Group{Name: "AI-POOL", Type: "url-test", Proxies: ai, URL: "https://www.gstatic.com/generate_204", Interval: 300}, Group{Name: "ALL", Type: "select", Proxies: all}, Group{Name: "FALLBACK", Type: "fallback", Proxies: []string{"AUTO-FAST", "HK-POOL", "JP-POOL", "US-POOL", "ALL"}}, Group{Name: "PROXY", Type: "select", Proxies: []string{"AUTO-FAST", "FALLBACK", "ALL"}})
	return Output{MixedPort: 7890, Mode: "rule", UnifiedDelay: true, TCPConcurrent: true, Proxies: nodes, Groups: groups, Rules: []string{"DOMAIN-SUFFIX,openai.com,AI-POOL", "DOMAIN-SUFFIX,chatgpt.com,AI-POOL", "DOMAIN-SUFFIX,anthropic.com,AI-POOL", "GEOIP,CN,DIRECT", "MATCH,PROXY"}, Metadata: map[string]string{"generated-by": "pangolin 0.01.00", "generated-at": time.Now().UTC().Format(time.RFC3339)}}
}
