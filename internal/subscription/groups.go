package subscription

func names(nodes []Proxy) []string {
	result := make([]string, 0, len(nodes))
	for _, node := range nodes {
		result = append(result, node.Name)
	}
	return result
}

func namesByRegion(nodes []Proxy, target string) []string {
	result := make([]string, 0)
	for _, node := range nodes {
		if node.Region == target {
			result = append(result, node.Name)
		}
	}
	return result
}
