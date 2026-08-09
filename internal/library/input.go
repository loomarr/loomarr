package library

import (
	"context"
	"net/http"
	"net/url"
	"os"
	"strings"
)

// Resolving a library item to the input ffmpeg should read (§9.1 direct play, V47).
//
// DIRECT PLAY is the default: read the FILE and copy it, not the media server's HTTP stream.
// The file lives on a disk the media server reports by ITS OWN path (`/data/tv/…`); Loomarr
// reads it at whatever prefix the same mount has locally (`/cifs/fictionalserver/tv/…`). The
// `library.path_map` setting (§15) is the prefix substitution between the two — the standard
// "path mapping" every media server exposes. When no mapping resolves a readable local file
// (a media server on another host, no shared mount), playout falls back to the HTTP stream, so
// a zero-config install still works.

// InputKind distinguishes a local file from an HTTP stream, so the encoder can pick the right
// input flags (HTTP needs reconnect flags; a file does not — see playout.ProgramArgs).
type InputKind int

const (
	// InputFile is a readable local path — the direct-play default.
	InputFile InputKind = iota
	// InputHTTP is the media server's stream URL — the fallback.
	InputHTTP
)

// InputSource is what ffmpeg should read for one library item, plus which kind it is.
type InputSource struct {
	// URL is the ffmpeg input: an absolute file path (InputFile) or an http(s) URL (InputHTTP).
	URL  string
	Kind InputKind
}

// PathMap is an ordered list of prefix-substitution rules parsed from `library.path_map`.
type PathMap []pathRule

type pathRule struct {
	from string // the media server's prefix, e.g. "/data"
	to   string // the local prefix, e.g. "/cifs/fictionalserver"
}

// ParsePathMap parses the `library.path_map` setting into rules.
//
// Format: one or more `from=>to` rules, separated by commas or newlines. Whitespace around each
// part is trimmed. Malformed entries (no `=>`, empty side) are skipped rather than failing — a
// bad mapping should degrade to "no mapping for this prefix" (→ HTTP fallback), never take a
// channel down. Rules are tried in order, so a more specific prefix can precede a broader one.
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

// itemPathResponse is the slice of /Items we read to resolve a file path.
type itemPathResponse struct {
	Items []struct {
		Path string `json:"Path"`
	} `json:"Items"`
}

// ItemPath fetches the media server's reported filesystem path for one item.
//
//	GET /Items?Ids=<id>&Fields=Path&Limit=1
//
// Empty string (not an error) when the item has no path or the server does not return one — the
// caller treats "no path" as "cannot direct-play, use the stream URL", the same degradation as an
// unmapped path.
func (c *Client) ItemPath(ctx context.Context, itemID string) (string, error) {
	if itemID == "" {
		return "", nil
	}
	q := url.Values{}
	q.Set("Ids", itemID)
	q.Set("Fields", "Path")
	q.Set("Limit", "1")

	req, err := c.newRequest(ctx, http.MethodGet, "/Items?"+q.Encode(), nil)
	if err != nil {
		return "", err
	}
	c.flavor.applyTokenAuth(req, c.token(), c.deviceID)

	var out itemPathResponse
	if err := c.do(req, &out); err != nil {
		return "", err
	}
	if len(out.Items) == 0 {
		return "", nil
	}
	return out.Items[0].Path, nil
}

// ResolveInput decides what ffmpeg should read for an item: the local FILE (direct play) when a
// path mapping resolves a readable file, else the media server's HTTP STREAM (fallback).
//
// `pathMap` is the parsed `library.path_map`. `statFn` reports whether a mapped path is a readable
// file — injected so the decision is testable without touching a real filesystem (production passes
// a wrapper over os.Stat). Best-effort throughout: any failure to resolve a local file falls back
// to the stream URL, never an error — a channel must never go dark because a path could not be
// mapped.
func (c *Client) ResolveInput(ctx context.Context, itemID string, pathMap PathMap, statFn func(string) bool) InputSource {
	stream := c.StreamURL(itemID)
	if len(pathMap) == 0 {
		return InputSource{URL: stream, Kind: InputHTTP} // no mapping configured → stream
	}
	serverPath, err := c.ItemPath(ctx, itemID)
	if err != nil || serverPath == "" {
		return InputSource{URL: stream, Kind: InputHTTP}
	}
	local, ok := pathMap.Apply(serverPath)
	if !ok || statFn == nil || !statFn(local) {
		return InputSource{URL: stream, Kind: InputHTTP}
	}
	return InputSource{URL: local, Kind: InputFile}
}

// StatReadableFile reports whether path is a readable regular file — the production statFn for
// ResolveInput. A directory, a missing path, or an unreadable file all report false, so
// ResolveInput falls back to the stream.
func StatReadableFile(path string) bool {
	info, err := os.Stat(path)
	if err != nil || info.IsDir() {
		return false
	}
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	_ = f.Close()
	return true
}
