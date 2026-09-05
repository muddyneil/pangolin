package source

import "gopkg.in/yaml.v3"

func parseSimpleYAML(text string) ([]map[string]any, error) {
	var document any
	if err := yaml.Unmarshal([]byte(text), &document); err != nil {
		return nil, err
	}
	if m, ok := document.(map[string]any); ok {
		document = m["proxies"]
	}
	items, ok := document.([]any)
	if !ok {
		return nil, nil
	}
	out := make([]map[string]any, 0, len(items))
	for _, item := range items {
		if m, ok := item.(map[string]any); ok {
			out = append(out, m)
		}
	}
	return out, nil
}
