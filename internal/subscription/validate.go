package subscription

import (
	"encoding/json"
	"fmt"
)

func Validate(out Output) error {
	if len(out.Proxies) == 0 {
		return fmt.Errorf("subscription must contain at least one node")
	}
	names := map[string]bool{}
	for _, proxy := range out.Proxies {
		if proxy.Name == "" || names[proxy.Name] {
			return fmt.Errorf("node name is empty or duplicated: %q", proxy.Name)
		}
		names[proxy.Name] = true
	}
	groupSet := map[string]bool{}
	validTypes := map[string]bool{"select": true, "url-test": true, "fallback": true, "load-balance": true}
	for _, group := range out.Groups {
		if group.Name == "" || groupSet[group.Name] {
			return fmt.Errorf("proxy group name is empty or duplicated: %q", group.Name)
		}
		if !validTypes[group.Type] {
			return fmt.Errorf("proxy group %q has an invalid type: %q", group.Name, group.Type)
		}
		groupSet[group.Name] = true
	}
	for _, group := range out.Groups {
		if len(group.Proxies) == 0 {
			return fmt.Errorf("proxy group %q is empty", group.Name)
		}
		for _, ref := range group.Proxies {
			if !names[ref] && !groupSet[ref] {
				return fmt.Errorf("proxy group %q references a missing node or proxy group %q", group.Name, ref)
			}
		}
	}
	if len(out.Rules) == 0 || out.Rules[len(out.Rules)-1] != "MATCH,PROXY" {
		return fmt.Errorf("rules must end with MATCH,PROXY")
	}
	return nil
}

func Marshal(out Output) ([]byte, error) {
	if err := Validate(out); err != nil {
		return nil, err
	}
	return json.MarshalIndent(out, "", "  ")
}
