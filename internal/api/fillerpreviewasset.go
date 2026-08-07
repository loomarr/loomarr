package api

import (
	"errors"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"

	"github.com/mantonx/loomarr/internal/filler"
)

// Serving a clip's animated hover preview (V39).
//
// The sibling of `thumb`, and built on the same bones: both serve regenerable ffmpeg output out
// of Loomarr's own cache directory inside FILLER_DIR, both are member-visible, and both treat a
// missing file as an ordinary 404 rather than an error.
//
// ⚠ **The route is `/v1/filler/hover/{path...}`, NOT `/preview`.** "Preview" is already taken
// twice in this API — `PodAdapter.Preview` and the two channel-scoped pod-preview operations both
// mean "the pool a channel would get", a JSON listing rather than media. `fillerthumb.go` records
// that collision as the reason it is not called `preview` either, and adding a third meaning to
// the word now would make the warning it left behind pointless. `hover` says what the asset is
// FOR, which is the one thing that distinguishes it from every other clip-shaped byte stream.
//
// ⚠ **A plain mux handler, and NOT because "Huma only does JSON" — it does not only do JSON.**
// Huma can serve arbitrary content types. What it cannot do without dropping to the raw
// ResponseWriter anyway is `http.ServeContent`, which is what gives Range, If-Modified-Since and
// 206 handling in one call — the machinery that lets a client seek instead of refetching. Going
// through an op would therefore buy an openapi.yaml entry describing an opaque byte body, at the
// cost of the escape hatch, so this matches `thumb` and `media` instead.
//
// The real price of that choice, stated so it is not a surprise: this route is NOT in the spec,
// so its client URL (`clipHoverURL`) is hand-written rather than generated, and hand-maintained
// lists in this repo have drifted from generated output three times. The URL rule is shared with
// the other two helpers in one place for exactly that reason.

// serveFillerHover streams a clip's animated preview.
//
// Member-visible, matching `thumb` and `list-filler`: this is a lower-fidelity, silent excerpt of
// a clip whose row is already visible to any authenticated user, and whose full bytes that same
// user can already stream from `media`. NOT public — unlike the channel icon, nothing
// machine-to-machine needs it.
func (s *Server) serveFillerHover(w http.ResponseWriter, r *http.Request) {
	if !s.requireRole(w, r, RoleMember) {
		return
	}

	// Read live rather than captured at wiring, so changing filler.dir in Settings applies to the
	// next request (config-design §3 hot-apply) — the same treatment scan, sync and thumb give it.
	// `liveConfig` is nil in unit tests that build a bare Server.
	if s.liveConfig == nil {
		http.NotFound(w, r)
		return
	}
	dir := s.liveConfig("filler.dir")
	if dir == "" {
		// No drop-folder configured: there are no previews, not a broken one.
		http.NotFound(w, r)
		return
	}

	// The clip is addressed by its content HASH (V45a); resolve it to the disk path to locate the
	// hover preview.
	clipPath, ok := s.clipPathByHash(r.Context(), r.PathValue("hash"))
	if !ok {
		http.NotFound(w, r)
		return
	}

	rel := filler.PreviewPathFor(clipPath)
	if rel == "" {
		http.NotFound(w, r)
		return
	}

	// ⚠ The SAME containment helper as the still, because the previews live in the SAME cache
	// directory (PreviewDirName is ThumbDirName). This is the security boundary: the clip id
	// comes from the URL and preserves directory structure, so it legitimately contains
	// separators and cannot be checked as a bare filename.
	//
	// ⚠ Note what this does NOT do: consult the store first, unlike `media`. That asymmetry is
	// deliberate rather than an oversight. `media` serves out of the OPERATOR'S folder, where a
	// caller-supplied string reaching the filesystem could name any allowlisted file the operator
	// happens to keep there; this serves out of Loomarr's own derived cache, which contains
	// nothing but files this code wrote. `thumb` makes the same call for the same reason.
	full, ok := safeThumbPath(dir, rel)
	if !ok {
		// A traversal attempt is a 404, not a 403: a distinct error would confirm which paths
		// exist outside the cache directory.
		http.NotFound(w, r)
		return
	}

	f, err := os.Open(full) //nolint:gosec // path confined to the thumb dir by safeThumbPath
	if err != nil {
		if !errors.Is(err, fs.ErrNotExist) {
			// ⚠ A clip with no preview is the COMMON case, not a fault: every clip catalogued
			// before V39 has none until the next sync renders one, and an audio-only or
			// undecodable file never will. Those stay a quiet 404 — GeneratePreviews counts the
			// failures at scan time, which is where an operator is actually told.
			s.log.Error("open clip preview", "path", clipPath, "err", err)
		}
		http.NotFound(w, r)
		return
	}
	defer func() { _ = f.Close() }()

	info, err := f.Stat()
	if err != nil {
		s.log.Error("stat clip preview", "path", clipPath, "err", err)
		http.NotFound(w, r)
		return
	}

	// Hardcoded, like the thumbnail's image/jpeg: this route serves exactly one format, written by
	// our own ffmpeg call. There is no extension to look up and therefore no allowlist to get
	// wrong — the reason `media` needs one is that it serves the operator's own files.
	w.Header().Set("Content-Type", "image/webp")
	// nosniff so a browser cannot re-interpret the bytes as HTML/script. These are our own output,
	// but the drop-folder is operator-writable and this costs nothing.
	w.Header().Set("X-Content-Type-Options", "nosniff")
	// Private, not public: this sits behind auth, so a shared cache must not hand one user's
	// request to another. A re-render replaces the file in place — the path does not change when
	// the clip does — so revalidation is what keeps a replaced preview from sticking.
	w.Header().Set("Cache-Control", "private, max-age=3600, must-revalidate")
	// ServeContent handles Last-Modified and conditional requests, so a catalog scrolled past
	// twice re-fetches almost nothing.
	http.ServeContent(w, r, filepath.Base(full), info.ModTime(), f)
}
