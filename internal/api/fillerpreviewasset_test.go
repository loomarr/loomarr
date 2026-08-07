package api_test

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/mantonx/loomarr/internal/api"
	"github.com/mantonx/loomarr/internal/filler"
	"github.com/mantonx/loomarr/internal/store"
)

// GET /v1/filler/hover/{hash} — a clip's animated preview (V39; hash-keyed since V45a).
//
// The sibling of the thumbnail route and tested the same way: since V45a the byte routes take the
// clip HASH and resolve the disk path via the store, so the test seeds a clip whose hash maps to
// the nested path whose preview exists on disk.

// hoverHash is the content hash the byte-route tests address the seeded clip by; noPreviewHash is a
// catalogued clip whose preview file was never rendered (the ORDINARY pre-V39 case).
const (
	hoverHash     = "hoverhash000000000000000000000000000000000000000000000000000000"
	noPreviewHash = "nopreviewhash00000000000000000000000000000000000000000000000000"
)

// A drop-folder with one clip's preview already rendered, plus a secret outside it that a
// traversal would reach. The secret is the point: a containment test that only checks for a 404
// proves nothing about whether the file was readable in the first place.
func newHoverServer(t *testing.T) (*httptest.Server, string) {
	t.Helper()
	ctx := context.Background()
	root := t.TempDir()
	fillerDir := filepath.Join(root, "clips")
	cache := filepath.Join(fillerDir, filler.PreviewDirName)
	if err := os.MkdirAll(filepath.Join(cache, "80s", "toys"), 0o755); err != nil {
		t.Fatal(err)
	}
	// Nested, because the generator preserves directory structure rather than flattening.
	if err := os.WriteFile(filepath.Join(cache, "80s", "toys", "intro.webp"), []byte("RIFF....WEBP"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "secret.txt"), []byte("TOP SECRET"), 0o600); err != nil {
		t.Fatal(err)
	}

	st, err := store.Open(ctx, "sqlite://"+filepath.Join(root, "hover.db"), true)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	// Hash → disk path. `hoverHash` maps to the clip whose .webp exists; `noPreviewHash` to one
	// whose preview was never rendered. A path is never addressable directly, only a catalogued hash.
	for hash, p := range map[string]string{
		hoverHash:     "80s/toys/intro.mp4",
		noPreviewHash: "90s/cereal/frosted.mp4",
	} {
		if err := st.UpsertClip(ctx, store.Clip{
			Clip: filler.Clip{Hash: hash, Path: p, Name: p, Kind: filler.Commercial, DurationMs: 30000},
		}); err != nil {
			t.Fatal(err)
		}
	}

	srv := httptest.NewServer(api.Router(slog.New(slog.DiscardHandler), api.Options{
		Auth:  testAuthorizer{},
		Log:   slog.New(slog.DiscardHandler),
		Store: st,
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

// The clip is addressed by its content HASH (V45a); the handler resolves it to the nested disk
// path whose preview exists. Nesting is preserved on disk and the hash lookup finds it regardless.
func TestServeFillerHover_ServesANestedClipsPreview(t *testing.T) {
	srv, _ := newHoverServer(t)

	res := getThumb(t, srv, "/v1/filler/hover/"+hoverHash, memberToken)

	if res.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", res.StatusCode)
	}
	// ⚠ The content type is what makes the bytes an animation to a browser. Served as
	// octet-stream an <img> renders nothing at all, with no error anywhere.
	if got := res.Header.Get("Content-Type"); got != "image/webp" {
		t.Errorf("Content-Type = %q, want image/webp", got)
	}
	if got := res.Header.Get("X-Content-Type-Options"); got != "nosniff" {
		t.Errorf("X-Content-Type-Options = %q, want nosniff", got)
	}
	body, _ := io.ReadAll(res.Body)
	if string(body) != "RIFF....WEBP" {
		t.Errorf("body = %q, want the preview's bytes", body)
	}
}

// ⚠ **A clip with no preview is the ORDINARY case, not an error.** Every clip catalogued before
// V39 has none until the next sync renders one, and an audio-only file never will. A 404 is the
// honest answer and the card falls back to its still.
func TestServeFillerHover_MissingPreviewIsAQuiet404(t *testing.T) {
	srv, _ := newHoverServer(t)

	res := getThumb(t, srv, "/v1/filler/hover/"+noPreviewHash, memberToken)

	if res.StatusCode != http.StatusNotFound {
		t.Errorf("status = %d, want 404 for a clip whose preview was never rendered", res.StatusCode)
	}
}

// ⚠ The containment boundary, reframed for V45a. The wire identity is a clip's content HASH, not a
// path, so a caller cannot address an arbitrary disk location — the handler only serves a path it
// looked up in the store from the supplied hash, written by the generator, not the client. The
// strongest attack a client can spell is "a value that isn't a catalogued hash", which resolves to
// nothing and 404s. Only a catalogued clip's hash resolves.
func TestServeFillerHover_RefusesTraversal(t *testing.T) {
	srv, root := newHoverServer(t)

	// Proof the target a traversal would reach is genuinely readable, or the 404s below prove nothing.
	if _, err := os.Stat(filepath.Join(root, "secret.txt")); err != nil {
		t.Fatalf("the traversal target must exist for this test to mean anything: %v", err)
	}

	for _, path := range []string{
		"/v1/filler/hover/../../secret.txt",
		"/v1/filler/hover/%2e%2e%2f%2e%2e%2fsecret.txt",
		"/v1/filler/hover/sub/%2e%2e/%2e%2e/%2e%2e/secret.txt",
		// A hash-shaped but uncatalogued value: resolves to nothing in the store.
		"/v1/filler/hover/deadbeef000000000000000000000000000000000000000000000000000000",
	} {
		res := getThumb(t, srv, path, memberToken)
		if res.StatusCode == http.StatusOK {
			body, _ := io.ReadAll(res.Body)
			t.Errorf("%s served %q — a caller reached bytes it should not have; only a catalogued clip's hash resolves", path, body)
		}
	}
}

// Member-visible like `thumb` and the catalog listing, but never anonymous: this sits behind auth
// and nothing machine-to-machine needs it (unlike the channel icon, which is deliberately open).
func TestServeFillerHover_RequiresAMember(t *testing.T) {
	srv, _ := newHoverServer(t)

	if anon := getThumb(t, srv, "/v1/filler/hover/"+hoverHash, ""); anon.StatusCode == http.StatusOK {
		t.Error("an anonymous caller got the preview; this route is behind auth")
	}
	if member := getThumb(t, srv, "/v1/filler/hover/"+hoverHash, memberToken); member.StatusCode != http.StatusOK {
		t.Errorf("member got %d, want 200 — a member can already see the row and stream the clip",
			member.StatusCode)
	}
}

// No drop-folder configured means there are no previews, not a broken one.
func TestServeFillerHover_NoFillerDirIs404(t *testing.T) {
	srv := httptest.NewServer(api.Router(slog.New(slog.DiscardHandler), api.Options{
		Auth:       testAuthorizer{},
		Log:        slog.New(slog.DiscardHandler),
		LiveConfig: func(string) string { return "" },
	}))
	t.Cleanup(srv.Close)

	res := getThumb(t, srv, "/v1/filler/hover/"+hoverHash, memberToken)
	if res.StatusCode != http.StatusNotFound {
		t.Errorf("status = %d, want 404 with no filler.dir", res.StatusCode)
	}
}
