package api

import (
	"errors"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/mantonx/loomarr/internal/filler"
)

// Serving clip thumbnail bytes (V30, plan §6.2).
//
// V28 extracts a frame per clip and stores the path on the row, and `ClipDTO.thumbnail` has
// been crossing the wire ever since — but nothing served it, so the field was a path into a
// directory no HTTP client could reach. This is the route that makes it fetchable; V17b is the
// ClipCard that renders it.
//
// ⚠ **Named `thumb`, deliberately NOT `preview`.** "Preview" already means something else here
// and is shipped twice over: `PodAdapter.Preview` and the two channel-scoped pod-preview
// operations assemble *the pool a channel would get* — a JSON listing, not media. A third
// meaning on a route name is how an endpoint ends up called by the wrong handler a year later.
//
// A plain mux handler rather than a Huma op, for the same reason as the backup download and the
// channel icon: Huma models typed JSON, and this is image bytes.

// serveFillerThumb streams a clip's thumbnail JPEG.
//
// Member-visible, matching `list-filler`: the catalog listing is visible to any authenticated
// user, and a thumbnail is strictly less information than the row it decorates. NOT public like
// the channel icon — that one is deliberately open so Tunarr can fetch it machine-to-machine,
// which is a reason that does not apply here.
func (s *Server) serveFillerThumb(w http.ResponseWriter, r *http.Request) {
	if !s.requireRole(w, r, RoleMember) {
		return
	}

	// Read live rather than captured at wiring, so changing filler.dir in Settings applies to
	// the next request (config-design §3 hot-apply) — the same treatment the scan and sync
	// give it. `liveConfig` is nil in unit tests that build a bare Server.
	if s.liveConfig == nil {
		http.NotFound(w, r)
		return
	}
	dir := s.liveConfig("filler.dir")
	if dir == "" {
		// No drop-folder configured: there are no thumbnails, not a broken one.
		http.NotFound(w, r)
		return
	}

	// The clip id is its path relative to FILLER_DIR, so it arrives with slashes and must be
	// matched by a wildcard segment rather than a plain {id}.
	clipPath := r.PathValue("path")
	if clipPath == "" {
		http.NotFound(w, r)
		return
	}

	rel := filler.ThumbPathFor(clipPath)
	if rel == "" {
		http.NotFound(w, r)
		return
	}

	full, ok := safeThumbPath(dir, rel)
	if !ok {
		// A traversal attempt is a 404, not a 403: a distinct error would confirm which
		// paths exist outside the thumbnail directory.
		http.NotFound(w, r)
		return
	}

	f, err := os.Open(full) //nolint:gosec // path confined to the thumb dir by safeThumbPath
	if err != nil {
		if !errors.Is(err, fs.ErrNotExist) {
			// A clip whose thumbnail was never generated (ffmpeg missing, extraction
			// failed) is the COMMON case and must stay a quiet 404 — V28 counts those
			// failures at scan time, which is where an operator is told about them.
			s.log.Error("open clip thumbnail", "path", clipPath, "err", err)
		}
		http.NotFound(w, r)
		return
	}
	defer func() { _ = f.Close() }()

	info, err := f.Stat()
	if err != nil {
		s.log.Error("stat clip thumbnail", "path", clipPath, "err", err)
		http.NotFound(w, r)
		return
	}

	w.Header().Set("Content-Type", "image/jpeg")
	// nosniff so a browser cannot re-interpret the bytes as HTML/script. These files are
	// produced by our own ffmpeg call, but the drop-folder is operator-writable and this
	// costs nothing.
	w.Header().Set("X-Content-Type-Options", "nosniff")
	// Private, not public: unlike a channel icon this sits behind auth, and a shared cache
	// must not hand one user's request to another. Short max-age with revalidation because a
	// re-scan can replace the frame in place — the path does not change when the clip does.
	w.Header().Set("Cache-Control", "private, max-age=3600, must-revalidate")
	// ServeContent handles Last-Modified, Range and conditional requests, so a catalog
	// scrolling past a few hundred thumbnails re-fetches almost none of them.
	http.ServeContent(w, r, filepath.Base(full), info.ModTime(), f)
}

// safeThumbPath resolves `rel` inside the thumbnail directory, returning ok=false if it would
// escape.
//
// ⚠ The clip id comes from the URL, and V28 stores paths that PRESERVE directory structure
// (`80s/toys/intro.jpg`), so this cannot be a filename-only check — the legitimate input
// contains separators. Without this, `../../` in a clip path reads any file the process can,
// and `filler.dir` is operator-configured so the base is not a constant either.
//
// The check is done on the CLEANED, absolute forms: comparing the raw strings would miss
// `thumbs/../../etc/passwd`, which has the right prefix before cleaning and the wrong one after.
func safeThumbPath(fillerDir, rel string) (string, bool) {
	if strings.Contains(rel, "\x00") {
		return "", false
	}
	base, err := filepath.Abs(filepath.Join(fillerDir, filler.ThumbDirName))
	if err != nil {
		return "", false
	}
	full, err := filepath.Abs(filepath.Join(base, filepath.FromSlash(rel)))
	if err != nil {
		return "", false
	}
	// filepath.Rel is the reliable containment test: a `..` component in the result means
	// `full` sits outside `base`, however the input was spelled.
	within, err := filepath.Rel(base, full)
	if err != nil || within == ".." || strings.HasPrefix(within, ".."+string(filepath.Separator)) {
		return "", false
	}
	return full, true
}