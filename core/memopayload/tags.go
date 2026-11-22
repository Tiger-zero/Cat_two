package memopayload

import "strings"

// CountTagBranches counts one memo once per tag path and ancestor. Expanding
// the paths also handles older payloads that contain only the literal tags.
func CountTagBranches(counts map[string]int32, tags []string) {
	seen := make(map[string]struct{}, len(tags))
	for _, path := range tags {
		for path != "" {
			if _, exists := seen[path]; exists {
				break
			}
			seen[path] = struct{}{}
			counts[path]++
			separator := strings.LastIndexByte(path, '/')
			if separator < 0 {
				break
			}
			path = path[:separator]
		}
	}
}
