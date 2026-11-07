package llm

import "strings"

// ExtractJSONObject returns the outermost {...} span in s, or s unchanged if there is no balanced
// object. It unwraps a markdown code fence (```json … ```) or strips stray prose around the JSON
// without touching bare JSON (which is already a single {...} span). It scans for balanced braces
// while respecting string literals + escapes, so a brace inside a string value cannot end the
// object early.
//
// ⚠ This lives in `llm` because it is a property of MODEL OUTPUT, not of any one caller. The
// suggester needed it first (a grounded model wraps its JSON in a fence often enough to matter),
// and the V44 vision tier needs it for the same reason — a live test found llava:7b returns its
// answer inside ```json fences, which json.Unmarshal rejects on the leading backtick. Both callers
// share ONE implementation here rather than each carrying a copy that can drift; the suggester's
// package-private extractJSONObject now delegates to this.
func ExtractJSONObject(s string) string {
	start := strings.IndexByte(s, '{')
	if start < 0 {
		return s
	}
	depth := 0
	inStr := false
	escaped := false
	for i := start; i < len(s); i++ {
		c := s[i]
		if inStr {
			switch {
			case escaped:
				escaped = false
			case c == '\\':
				escaped = true
			case c == '"':
				inStr = false
			}
			continue
		}
		switch c {
		case '"':
			inStr = true
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return s[start : i+1]
			}
		}
	}
	return s // unbalanced — let json.Unmarshal report the real error
}
