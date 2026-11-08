package library

import "strings"

// The media server reports each of its folders by ITS OWN path (`/data/tv`); Loomarr reads the
// same mount at whatever prefix it has locally (`/cifs/fictionalserver/tv`). The
// `library.path_map` setting (§15) is the prefix substitution between the two, the "path mapping"
// every media server exposes. Folder discovery uses it to offer a media server's folders as
// library roots; playout never does, since it reads the files Loomarr's own scan found
// (#2151 step 16).

// PathMap is an ordered list of prefix-substitution rules parsed from `library.path_map`.
type PathMap []pathRule

type pathRule struct {
	from string // the media server's prefix, e.g. "/data"
	to   string // the local prefix, e.g. "/cifs/fictionalserver"
}

// ParsePathMap parses the `library.path_map` setting into rules.
//
// Format: one or more `from=>to` rules, separated by commas or newlines. Whitespace around each
// part is trimmed. Malformed entries (no `=>`, empty side) are skipped rather than failing: a
// bad mapping degrades to "no mapping for this prefix". Rules are tried in order, so a more
// specific prefix can precede a broader one.
func ParsePathMap(raw string) PathMap {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	// Split on commas AND newlines so both `a=>b, c=>d` and a multi-line textarea work.
	fields := strings.FieldsFunc(raw, func(r rune) bool { return r == ',' || r == '\n' || r == '\r' })
	out := make(PathMap, 0, len(fields))
	for _, f := range fields {
		from, to, ok := strings.Cut(f, "=>")
		from, to = strings.TrimSpace(from), strings.TrimSpace(to)
		if !ok || from == "" || to == "" {
			continue
		}
		out = append(out, pathRule{from: from, to: to})
	}
	return out
}

// Apply translates a media-server path to its local equivalent using the first matching rule,
// or returns ("", false) when no rule's `from` prefixes the path. A rule matches on a path-segment
// boundary: `/data` maps `/data/tv/x` but NOT `/database/x`, so a broad prefix cannot accidentally
// rewrite an unrelated path.
func (m PathMap) Apply(serverPath string) (string, bool) {
	for _, r := range m {
		if serverPath == r.from {
			return r.to, true
		}
		// Boundary-aware prefix: the char after `from` must be a separator, so `/data` does not
		// match `/database`. Both `/` and `\` are checked for cross-platform paths.
		if strings.HasPrefix(serverPath, r.from) {
			rest := serverPath[len(r.from):]
			if rest[0] == '/' || rest[0] == '\\' {
				return r.to + rest, true
			}
		}
	}
	return "", false
}
