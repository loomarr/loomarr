package split

import (
	"path/filepath"
	"sort"
	"strings"
)

// The helpers below are shared with the filler root, which cannot be imported from here; each is
// defined once, in this package.

// IsRelativePath reports whether path is non-empty, relative and stays beneath its base once
// cleaned.
func IsRelativePath(path string) bool {
	if path == "" || filepath.IsAbs(path) {
		return false
	}
	clean := filepath.Clean(path)
	return clean != "." && clean != ".." && !strings.HasPrefix(clean, ".."+string(filepath.Separator))
}

// PathContains reports whether child is parent itself or lies beneath it. A watch folder with
// that relationship to the clip root would walk already-filed clips as arrivals; duplicate
// handling would then remove each file because its computed destination is itself.
func PathContains(parent, child string) bool {
	rel, err := filepath.Rel(parent, child)
	if err != nil || filepath.IsAbs(rel) {
		return false
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

// IsContentHash reports whether value is a 64-character lowercase hex content hash.
func IsContentHash(value string) bool {
	return len(value) == 64 && IsLowerHex(value)
}

// IsLowerHex reports whether value is a non-empty string of lowercase hex digits.
func IsLowerHex(value string) bool {
	if value == "" {
		return false
	}
	for _, r := range value {
		if !strings.ContainsRune("0123456789abcdef", r) {
			return false
		}
	}
	return true
}

// UniqueStrings trims every value, drops blanks and case-insensitive repeats, and returns the
// rest sorted case-insensitively.
func UniqueStrings(in []string) []string {
	out := make([]string, 0, len(in))
	seen := map[string]bool{}
	for _, value := range in {
		value = strings.TrimSpace(value)
		key := strings.ToLower(value)
		if value != "" && !seen[key] {
			seen[key] = true
			out = append(out, value)
		}
	}
	sort.Slice(out, func(a, b int) bool { return strings.ToLower(out[a]) < strings.ToLower(out[b]) })
	return out
}
