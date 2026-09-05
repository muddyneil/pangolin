package subscription

import "encoding/json"

type Output struct {
	MixedPort     int               `json:"mixed-port"`
	AllowLAN      bool              `json:"allow-lan"`
	Mode          string            `json:"mode"`
	IPv6          bool              `json:"ipv6"`
	UnifiedDelay  bool              `json:"unified-delay"`
	TCPConcurrent bool              `json:"tcp-concurrent"`
	Proxies       []Proxy           `json:"proxies"`
	Groups        []Group           `json:"proxy-groups"`
	Rules         []string          `json:"rules"`
	Metadata      map[string]string `json:"metadata"`
}
type Proxy struct {
	Name   string         `json:"name"`
	Type   string         `json:"type"`
	Server string         `json:"server"`
	Port   int            `json:"port"`
	Fields map[string]any `json:"-"`
	Region string         `json:"-"`
}
type Group struct {
	Name     string   `json:"name"`
	Type     string   `json:"type"`
	Proxies  []string `json:"proxies"`
	URL      string   `json:"url,omitempty"`
	Interval int      `json:"interval,omitempty"`
}

func (p Proxy) MarshalJSON() ([]byte, error) {
	value := make(map[string]any, len(p.Fields)+4)
	for key, field := range p.Fields {
		value[key] = field
	}
	value["name"], value["type"], value["server"], value["port"] = p.Name, p.Type, p.Server, p.Port
	return json.Marshal(value)
}
