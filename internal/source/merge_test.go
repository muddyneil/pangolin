package source

import "testing"

func TestMergeRenameSkipsTakenSuffixes(t *testing.T) {
	items := []FetchResult{
		{Proxies: []Proxy{{Name: "x", Type: "ss", Server: "a", Port: 1}, {Name: "x-2", Type: "ss", Server: "b", Port: 2}}},
		{Proxies: []Proxy{{Name: "x", Type: "ss", Server: "c", Port: 3}}},
	}
	merged := Merge(items)
	if len(merged) != 3 {
		t.Fatalf("unexpected merged nodes: %#v", merged)
	}
	seen := map[string]bool{}
	for _, node := range merged {
		if seen[node.Name] {
			t.Fatalf("duplicate node name %q: %#v", node.Name, merged)
		}
		seen[node.Name] = true
	}
	if merged[0].Name != "x" {
		t.Fatalf("first node renamed unnecessarily: %#v", merged)
	}
}

func TestUniqueNameKeepsDistinctBasesSeparate(t *testing.T) {
	taken := map[string]bool{"a": true}
	if name := uniqueName("a", taken); name != "a-2" {
		t.Fatalf("uniqueName(a) = %q, want a-2", name)
	}
	if name := uniqueName("b", taken); name != "b" {
		t.Fatalf("uniqueName(b) = %q, want b", name)
	}
}
