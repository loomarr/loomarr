package filler

import "sort"

// unionLeaves merges asserted taxonomy leaves without promoting derived rollups. Vision and split
// materialization are additive evidence sources, so neither may erase an operator or earlier pass.
func unionLeaves(existing, fresh []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(existing)+len(fresh))
	for _, slug := range append(append([]string(nil), existing...), fresh...) {
		if slug == "" || seen[slug] {
			continue
		}
		seen[slug] = true
		out = append(out, slug)
	}
	sort.Strings(out)
	return out
}
