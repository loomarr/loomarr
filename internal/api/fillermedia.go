package api

import (
	"errors"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/mantonx/loomarr/internal/store"
)

// Serving a clip's own bytes, so the operator can watch one before deciding about it (§10 V35).
//
// The redesigned Filler page puts a ▶ on every clip card, every ask in the Incoming queue, and
// every segment of a split proposal. All three are the same question — "is this actually the
// advert it says it is?" — and none of them can be answered from a thumbnail.
//
// ⚠ **Named `media`, deliberately NOT `preview`**, for the reason `thumb` is not: "preview"
// already means a pod listing in `PodAdapter.Preview` and two channel-scoped operations. A third
// meaning on a route name is how an endpoint ends up called by the wrong handler a year later.
//
// A plain mux handler rather than a Huma op, like `thumb` and the backup download: Huma models
// typed JSON and this is a byte stream.

// mediaTypes is the allowlist of what this route will serve, keyed by lowercase extension.
//
// ⚠ **An allowlist, not a lookup, and that is a security boundary rather than tidiness.**
// `FILLER_DIR` is an operator-writable folder — that is its whole purpose — so a file in it is
// untrusted input. Serving whatever `mime.TypeByExtension` returns would let an `.html` dropped
// in that folder come back as `text/html` **from Loomarr's own origin**, which is stored XSS
// against the session cookie of anyone who clicks it. The thumbnail route never had this problem
// because it hardcodes `image/jpeg`; this one has to enumerate.
//
// The list is what the scan already accepts plus the audio containers ffmpeg produces, and it is
// deliberately short: an extension that is not here is not served, so adding a format is a
// decision someone makes on purpose.
var mediaTypes = map[string]string{
	".mp4":  "video/mp4",
	".m4v":  "video/mp4",
	".mov":  "video/quicktime",
	".mkv":  "video/x-matroska",
	".webm": "video/webm",
	".avi":  "video/x-msvideo",
	".mpg":  "video/mpeg",
	".mpeg": "video/mpeg",
	".ts":   "video/mp2t",
	".m4a":  "audio/mp4",
	".mp3":  "audio/mpeg",
}

// serveFillerMedia streams a catalog clip's own file.
//
// Member-visible, matching `list-filler` and `thumb`: the catalog listing is already visible to
// any authenticated user, and these are the same commercials the household's channels play at
// them. NOT public — unlike the channel icon, nothing machine-to-machine needs this.
func (s *Server) serveFillerMedia(w http.ResponseWriter, r *http.Request) {
	if !s.requireRole(w, r, RoleMember) {
		return
	}

	// Read live rather than captured at wiring, so changing filler.dir in Settings applies to
	// the next request (config-design §3 hot-apply) — the same treatment scan, sync and thumb
	// give it. `liveConfig` is nil in unit tests that build a bare Server.
	if s.liveConfig == nil {
		http.NotFound(w, r)
		return
	}
	dir := s.liveConfig("filler.dir")
	if dir == "" {
		http.NotFound(w, r)
		return
	}

	// The clip id IS its path relative to FILLER_DIR, so it arrives with slashes and needs a
	// wildcard segment rather than a plain {id}.
	clipPath := r.PathValue("path")
	if clipPath == "" {
		http.NotFound(w, r)
		return
	}

	// ⚠ Guarded even though a nil store is currently unreachable here: `liveConfig` is nil
	// exactly when the store is (both are built only inside `if st != nil`), so the check above
	// already 404s a store-less boot. That coupling is true by construction today and stated
	// nowhere — every sibling handler guards the store directly, and relying on a second
	// variable's nilness is how the next refactor turns a 404 into a panic.
	if s.store == nil {
		http.NotFound(w, r)
		return
	}

	// ⚠ The catalog is consulted BEFORE the filesystem, and it is a second gate rather than a
	// convenience. Without it this route serves any allowlisted file anywhere under a folder
	// the operator also uses for their own media; with it, the only reachable files are rows
	// the scan created. A path that is not a clip is not a clip, whatever is on disk.
	// ⚠ The URL carries the clip's IDENTITY (its hash since V38c), and the file served is the
	// row's `Path`. Before V38c those were the same string, so this route resolved the URL
	// segment directly against the filesystem — which is why the lookup's result was discarded.
	// It cannot be now: a caller-supplied string must never reach the filesystem, and the row is
	// the only thing that says where a clip actually lives.
	clip, err := s.store.GetClip(r.Context(), clipPath)
	if err != nil {
		if !errors.Is(err, store.ErrNotFound) {
			s.log.Error("look up clip for media", "id", clipPath, "err", err)
		}
		http.NotFound(w, r)
		return
	}

	ctype, ok := mediaTypes[strings.ToLower(filepath.Ext(clip.Path))]
	if !ok {
		http.NotFound(w, r)
		return
	}

	full, ok := safeFillerPath(dir, clip.Path)
	if !ok {
		// A traversal attempt is a 404, not a 403: a distinct error would confirm which paths
		// exist outside the drop-folder.
		http.NotFound(w, r)
		return
	}

	f, err := os.Open(full) //nolint:gosec // path confined to the drop-folder by safeFillerPath
	if err != nil {
		if !errors.Is(err, fs.ErrNotExist) {
			s.log.Error("open clip media", "path", clipPath, "err", err)
		}
		http.NotFound(w, r)
		return
	}
	defer func() { _ = f.Close() }()

	info, err := f.Stat()
	if err != nil {
		s.log.Error("stat clip media", "path", clipPath, "err", err)
		http.NotFound(w, r)
		return
	}
	// A directory that happens to be named like a clip would otherwise reach ServeContent,
	// which renders a listing.
	if info.IsDir() {
		http.NotFound(w, r)
		return
	}

	w.Header().Set("Content-Type", ctype)
	// nosniff belongs here even with the allowlist above: it is what stops a browser
	// re-interpreting the BYTES as HTML when it disagrees with our declared type.
	w.Header().Set("X-Content-Type-Options", "nosniff")
	// Never render inline as a document. A `<video src>` is unaffected — the attachment
	// disposition governs navigation, not media loading — but following the URL directly
	// downloads rather than opens.
	w.Header().Set("Content-Disposition", "attachment")
	// Private: this sits behind auth, so a shared cache must not hand one user's request to
	// another. Clip bytes are immutable for a given path — a re-scan that changes the file
	// changes its mtime, which ServeContent turns into a new validator.
	w.Header().Set("Cache-Control", "private, max-age=3600, must-revalidate")
	// ServeContent gives us Range and conditional requests, which is what lets a <video>
	// element seek instead of downloading a whole clip to play the last five seconds.
	http.ServeContent(w, r, filepath.Base(full), info.ModTime(), f)
}

// safeFillerPath resolves a clip path inside the drop-folder, returning ok=false if it escapes.
//
// The same containment test as `safeThumbPath` against a different base — and it is a separate
// function rather than a parameter because the two bases are not interchangeable: the thumb
// directory is Loomarr's own cache, while this one is the operator's folder.
//
// Note what this function deliberately does NOT do: it does not exclude the thumbnail cache
// under `.loomarr-thumbs`. It does not need to, and adding the check would imply the containment
// test is the only thing standing between a caller and those files. Two other gates already
// refuse them — a thumbnail is not a catalog row, and `.jpg` is not in `mediaTypes`.
func safeFillerPath(fillerDir, rel string) (string, bool) {
	if rel == "" || strings.Contains(rel, "\x00") {
		return "", false
	}
	base, err := filepath.Abs(fillerDir)
	if err != nil {
		return "", false
	}
	full, err := filepath.Abs(filepath.Join(base, filepath.FromSlash(rel)))
	if err != nil {
		return "", false
	}
	// filepath.Rel is the reliable containment test: a `..` component in the result means
	// `full` sits outside `base`, however the input was spelled. Comparing raw strings would
	// miss `clips/../../etc/passwd`, which has the right prefix before cleaning.
	within, err := filepath.Rel(base, full)
	if err != nil || within == ".." || strings.HasPrefix(within, ".."+string(filepath.Separator)) {
		return "", false
	}
	return full, true
}
