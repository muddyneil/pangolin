package source

import "fmt"

// Merge interleaves source queues while removing duplicate fingerprints and
// renaming colliding node names to unique names.
func Merge(results []FetchResult) []Proxy {
	queues := make([][]Proxy, len(results))
	for i, result := range results {
		queues[i] = append([]Proxy(nil), result.Proxies...)
	}
	var out []Proxy
	seen := map[string]bool{}
	taken := map[string]bool{}
	maxItems := 0
	for _, queue := range queues {
		if len(queue) > maxItems {
			maxItems = len(queue)
		}
	}
	for round := 0; round < maxItems; round++ {
		for i := range queues {
			if len(queues[i]) == 0 {
				continue
			}
			p := queues[i][0]
			queues[i] = queues[i][1:]
			if p.Fingerprint == "" {
				p.Fingerprint = fingerprint(p)
			}
			if seen[p.Fingerprint] {
				continue
			}
			seen[p.Fingerprint] = true
			p.Name = uniqueName(p.Name, taken)
			out = append(out, p)
			if len(out) == MaxCandidates {
				return out
			}
		}
	}
	return out
}

// uniqueName returns name if it is not taken, otherwise name-2, name-3, ...
// skipping suffixes that are already taken, and marks the result as taken.
func uniqueName(name string, taken map[string]bool) string {
	base := name
	for n := 2; taken[name]; n++ {
		name = fmt.Sprintf("%s-%d", base, n)
	}
	taken[name] = true
	return name
}
