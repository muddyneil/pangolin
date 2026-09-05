package subscription

import (
	"encoding/json"
	"fmt"
)

func Validate(out Output) error {
	if len(out.Proxies) == 0 {
		return fmt.Errorf("订阅必须至少包含一个节点")
	}
	names := map[string]bool{}
	for _, proxy := range out.Proxies {
		if proxy.Name == "" || names[proxy.Name] {
			return fmt.Errorf("节点名称为空或重复: %q", proxy.Name)
		}
		names[proxy.Name] = true
	}
	groupSet := map[string]bool{}
	validTypes := map[string]bool{"select": true, "url-test": true, "fallback": true, "load-balance": true}
	for _, group := range out.Groups {
		if group.Name == "" || groupSet[group.Name] {
			return fmt.Errorf("代理组名称为空或重复: %q", group.Name)
		}
		if !validTypes[group.Type] {
			return fmt.Errorf("代理组 %q 类型无效: %q", group.Name, group.Type)
		}
		groupSet[group.Name] = true
	}
	for _, group := range out.Groups {
		if len(group.Proxies) == 0 {
			return fmt.Errorf("代理组 %q 为空", group.Name)
		}
		for _, ref := range group.Proxies {
			if !names[ref] && !groupSet[ref] {
				return fmt.Errorf("代理组 %q 引用了不存在的节点或代理组 %q", group.Name, ref)
			}
		}
	}
	if len(out.Rules) == 0 || out.Rules[len(out.Rules)-1] != "MATCH,PROXY" {
		return fmt.Errorf("规则必须以 MATCH,PROXY 结尾")
	}
	return nil
}

func Marshal(out Output) ([]byte, error) {
	if err := Validate(out); err != nil {
		return nil, err
	}
	return json.MarshalIndent(out, "", "  ")
}
