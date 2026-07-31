package api_test

import (
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/mantonx/loomarr/internal/api"
	"github.com/mantonx/loomarr/internal/filler"
)

// A drop-folder with one clip's thumbnail already extracted, plus a secret outside it that a
// traversal would reach. The secret is the point: a containment test that only checks for a
// 404 proves nothing about whether the file was readable in the first place.
func newThumbServer(t *testing.T) (*httptest.Server, string) {
	t.Helper()
	root := t.TempDir()
	fillerDir := filepath.Join(root, "clips")
	thumbs := filepath.Join(fillerDir, filler.ThumbDirName)
	if err := os.MkdirAll(filepath.Join(thumbs, "80s", "toys"), 0o755); err != nil {
		t.Fatal(err)
	}
	// Nested, because V28 preserves directory structure rather than flattening — two clips
	// named intro.mp4 in different era folders are different clips.
	if err := os.WriteFile(filepath.Join(thumbs, "80s", "toys", "intro.jpg"), []byte("JPEGBYTES"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "secret.txt"), []byte("TOP SECRET"), 0o600); err != nil {
		t.Fatal(err)
	}

	srv := httptest.NewServer(api.Router(slog.New(slog.DiscardHandler), api.Options{
		Auth: testAuthorizer{},
		Log:  slog.New(slog.DiscardHandler),
		LiveConfig: func(key string) string {
			if key == "filler.dir" {
				return fillerDir
			}
			return ""
		},
	}))
	t.Cleanup(srv.Close)
	return srv, root
}

func getThumb(t *testing.T, srv *httptest.Server, path, token string) *http.Response {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, srv.URL+path, nil)
	if err != nil {
		t.Fatal(err)
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	// No redirect following: a 30x here would hide which handler actually answered.
	res, err := srv.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = res.Body.Close() })
	return res
}

// The clip id is a PATH with slashes, so the route needs a `{path...}` wildcard. A plain
// {path} silently 404s every nested clip, which is most of a real catalog.
func TestServeFillerThumb_ServesANestedClipsThumbnail(t *testing.T) {
	srv, _ := newThumbServer(t)

	res := getThumb(t, srv, "/v1/filler/thumb/80s/toys/intro.mp4", memberToken)

	if res.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", res.StatusCode)
	}
	if ct := res.Header.Get("Content-Type"); ct != "image/jpeg" {
		t.Errorf("Content-Type = %q, want image/jpeg", ct)
	}
	if res.Header.Get("X-Content-Type-Options") != "nosniff" {
		t.Error("missing nosniff — a browser could re-interpret the bytes")
	}
}

// Traversal over HTTP, asserted for what it actually is: the request never gets far enough to
// matter.
//
// ⚠ **This test does NOT exercise the containment check, and saying so is the point.** Its first
// version sent `../../secret.txt` and passed — then kept passing with `safeThumbPath` deleted,
// because two layers strip the attack first: net/http's client rewrites the URL before sending,
// and Go 1.22+ ServeMux refuses to decode `%2f` into a separator when matching `{path...}`, so
// an encoded attempt is rejected by the MUX with its own 404. A test that cannot fail is worse
// than no test; the real assertion for the guard is `TestSafeThumbPath`, in-package.
//
// What this pins is the outcome an operator cares about — no spelling of the attack returns
// bytes — plus the fact that the layers above us behave as assumed. If a future Go relaxes the
// mux, or the route is remounted behind something that pre-decodes, this starts failing and the
// in-package guard is what stops it being a breach.
func TestServeFillerThumb_TraversalNeverReturnsBytes(t *testing.T) {
	srv, root := newThumbServer(t)

	// Sanity: the file being reached for really is readable, or every case below would pass
	// for the boring reason that nothing is there.
	if _, err := os.ReadFile(filepath.Join(root, "secret.txt")); err != nil {
		t.Fatalf("fixture unreadable, the traversal cases would prove nothing: %v", err)
	}

	for _, target := range []string{
		"/v1/filler/thumb/../../secret.txt",
		"/v1/filler/thumb/%2e%2e%2f%2e%2e%2fsecret.txt",
		"/v1/filler/thumb/sub/%2e%2e/%2e%2e/%2e%2e/secret.txt",
	} {
		res := getThumb(t, srv, target, memberToken)
		if res.StatusCode == http.StatusOK {
			t.Errorf("%s returned 200 — traversal reached a file outside the thumbnail dir", target)
		}
	}
}

// A clip whose thumbnail was never generated (ffmpeg missing, extraction failed) is the
// COMMON case, not an error. V28 counts those failures at scan time, which is where an
// operator is told about them.
func TestServeFillerThumb_MissingThumbnailIs404(t *testing.T) {
	srv, _ := newThumbServer(t)

	res := getThumb(t, srv, "/v1/filler/thumb/90s/cereal/frosted.mp4", memberToken)

	if res.StatusCode != http.StatusNotFound {
		t.Errorf("status = %d, want 404 for a clip with no extracted frame", res.StatusCode)
	}
}

// Member-visible, matching list-filler: the catalog listing is visible to any authenticated
// user and a thumbnail is strictly less information than the row it decorates. But NOT public
// — unlike the channel icon, which is deliberately open so Tunarr can fetch it.
func TestServeFillerThumb_RequiresAuth(t *testing.T) {
	srv, _ := newThumbServer(t)

	anon := getThumb(t, srv, "/v1/filler/thumb/80s/toys/intro.mp4", "")
	if anon.StatusCode == http.StatusOK {
		t.Error("served a thumbnail to an anonymous caller")
	}

	member := getThumb(t, srv, "/v1/filler/thumb/80s/toys/intro.mp4", memberToken)
	if member.StatusCode != http.StatusOK {
		t.Errorf("member got %d, want 200 — this is member-visible like list-filler", member.StatusCode)
	}
}

// An install with no drop-folder has no thumbnails; that is a 404, not a 500. Guards the nil
// and empty liveConfig paths, which a bare Server in other unit tests will hit.
func TestServeFillerThumb_NoFillerDirIs404(t *testing.T) {
	srv := httptest.NewServer(api.Router(slog.New(slog.DiscardHandler), api.Options{
		Auth:       testAuthorizer{},
		Log:        slog.New(slog.DiscardHandler),
		LiveConfig: func(string) string { return "" },
	}))
	t.Cleanup(srv.Close)

	res := getThumb(t, srv, "/v1/filler/thumb/80s/toys/intro.mp4", memberToken)
	if res.StatusCode != http.StatusNotFound {
		t.Errorf("status = %d, want 404 when filler.dir is unset", res.StatusCode)
	}
}
