package api_test

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/mantonx/loomarr/internal/api"
	"github.com/mantonx/loomarr/internal/filler"
	"github.com/mantonx/loomarr/internal/store"
)

// The content hashes the byte-route tests address the seeded clips by. ⚠ Since V45a the byte
// routes take the clip HASH (a plain, slash-free segment) and resolve the disk path via the store,
// so each hash is mapped to a nested on-disk path whose file exists.
const (
	// A catalogued MP4, nested on disk (V28 preserves directory structure).
	mediaHash = "mediahash000000000000000000000000000000000000000000000000000000"
	// A catalogued but hostile row: an .html that must never be served as a document.
	htmlHash = "htmlhash0000000000000000000000000000000000000000000000000000000"
)

// A drop-folder holding one catalogued clip, one file that exists on disk but is NOT a catalog
// row, and a secret outside the folder entirely. All three are load-bearing: the secret is what
// makes a traversal assertion mean something, and the uncatalogued file is what proves the
// catalog check is a real gate rather than a lookup.
func newMediaServer(t *testing.T) (*httptest.Server, string, store.Store) {
	t.Helper()
	ctx := context.Background()
	root := t.TempDir()
	fillerDir := filepath.Join(root, "clips")
	if err := os.MkdirAll(filepath.Join(fillerDir, "80s", "toys"), 0o755); err != nil {
		t.Fatal(err)
	}
	write := func(rel, body string) {
		if err := os.WriteFile(filepath.Join(fillerDir, rel), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	write(filepath.Join("80s", "toys", "intro.mp4"), "MP4BYTES")
	// On disk, never scanned into the catalog.
	write("stray.mp4", "STRAYBYTES")
	// An operator's own file that a browser would happily execute if we served its type.
	write("notes.html", "<script>alert(1)</script>")
	if err := os.WriteFile(filepath.Join(root, "secret.txt"), []byte("TOP SECRET"), 0o600); err != nil {
		t.Fatal(err)
	}

	st := openTestStore(t, t.TempDir()+"/m.db")
	t.Cleanup(func() { _ = st.Close() })
	// Hash → disk path. The wire identity is the hash (V45a); the route looks a clip up by hash
	// then serves the row's path. A path is never addressable directly, only a catalogued hash.
	for hash, p := range map[string]string{mediaHash: "80s/toys/intro.mp4", htmlHash: "notes.html"} {
		if err := st.UpsertClip(ctx, store.Clip{
			Clip:      filler.Clip{Hash: hash, Path: p, Name: p, Kind: filler.Commercial, DurationMs: 30_000},
			UpdatedAt: time.Now().UTC(),
		}); err != nil {
			t.Fatal(err)
		}
	}
	layout, err := filler.NewLayout(fillerDir, "")
	if err != nil {
		t.Fatalf("filler.NewLayout: %v", err)
	}

	srv := httptest.NewServer(api.Router(slog.New(slog.DiscardHandler), api.Options{
		Store:        st,
		Auth:         testAuthorizer{},
		Log:          slog.New(slog.DiscardHandler),
		FillerLayout: layout,
		// A saved desired value may differ until restart. Serving the clip successfully proves the
		// byte route uses the applied layout above, not this live settings seam.
		LiveConfig: func(key string) string {
			if key == "filler.dir" {
				return filepath.Join(root, "desired-after-restart")
			}
			return ""
		},
	}))
	t.Cleanup(srv.Close)
	return srv, root, st
}

func getMedia(t *testing.T, srv *httptest.Server, path, token string) *http.Response {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, srv.URL+path, nil)
	if err != nil {
		t.Fatal(err)
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = res.Body.Close() })
	return res
}

// The clip is addressed by its content HASH (V45a); the handler resolves it to the nested disk
// path whose file exists. Nesting is preserved on disk (V28) and the hash lookup finds it
// regardless — the slash-in-URL hazard the old `{path...}` route carried is gone.
func TestServeFillerMedia_ServesANestedCataloguedClip(t *testing.T) {
	srv, _, _ := newMediaServer(t)

	res := getMedia(t, srv, "/v1/filler/media/"+mediaHash, memberToken)

	if res.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", res.StatusCode)
	}
	if ct := res.Header.Get("Content-Type"); ct != "video/mp4" {
		t.Errorf("Content-Type = %q, want video/mp4", ct)
	}
	if res.Header.Get("X-Content-Type-Options") != "nosniff" {
		t.Error("missing nosniff — a browser could re-interpret the bytes")
	}
	// Range is what lets a <video> seek instead of downloading the whole clip to play the
	// last five seconds. ServeContent advertises it; losing it would be silent.
	if res.Header.Get("Accept-Ranges") != "bytes" {
		t.Error("no Accept-Ranges — the player cannot seek")
	}
}

// The catalog gate. `stray.mp4` is a real, allowlisted, contained file — everything except a
// clip. Since V45a the route resolves a HASH via the store, so an uncatalogued file simply has no
// addressable hash: the only value a caller could supply for it is one the store does not know,
// which resolves to nothing and 404s. The drop-folder is not a public share.
func TestServeFillerMedia_RefusesAFileThatIsNotACatalogRow(t *testing.T) {
	srv, _, _ := newMediaServer(t)

	// Sanity: a catalogued clip really is served, or this passes for the boring reason.
	if res := getMedia(t, srv, "/v1/filler/media/"+mediaHash, memberToken); res.StatusCode != http.StatusOK {
		t.Fatalf("fixture broken: a catalogued clip returned %d", res.StatusCode)
	}

	// `stray.mp4` exists on disk but was never catalogued, so no hash resolves to it.
	uncatalogued := "strayhash000000000000000000000000000000000000000000000000000000"
	if res := getMedia(t, srv, "/v1/filler/media/"+uncatalogued, memberToken); res.StatusCode == http.StatusOK {
		t.Error("served a file that is not in the catalog — the drop-folder is not a public share")
	}
}

// The allowlist gate, exercised through the route: `notes.html` IS a catalog row here (a
// deliberately hostile fixture), exists on disk, and is contained. Only the extension check
// stands between it and being served as a document from our own origin.
func TestServeFillerMedia_RefusesNonMediaEvenWhenCatalogued(t *testing.T) {
	srv, _, _ := newMediaServer(t)

	res := getMedia(t, srv, "/v1/filler/media/"+htmlHash, memberToken)

	if res.StatusCode == http.StatusOK {
		t.Errorf("served notes.html as %q — an operator-writable folder must never yield a "+
			"document from Loomarr's origin", res.Header.Get("Content-Type"))
	}
}

// Traversal is UNEXPRESSIBLE since V45a: the wire identity is a clip's content HASH, not a path,
// so a caller cannot address an arbitrary disk location — the handler only serves a path it looked
// up in the store from the supplied hash, and that path was written by the scanner, not the client.
// The strongest attack a client can spell is "a value that isn't a catalogued hash", which
// resolves to nothing and 404s.
//
// This pins that outcome: no traversal-shaped value, and no unknown hash, returns bytes. The old
// `../../secret.txt` spellings are kept not because they could reach the file (they can't — there
// is no path in the URL anymore) but to prove the classic attack yields a 404, never the secret.
func TestServeFillerMedia_TraversalNeverReturnsBytes(t *testing.T) {
	srv, root, _ := newMediaServer(t)

	if _, err := os.ReadFile(filepath.Join(root, "secret.txt")); err != nil {
		t.Fatalf("fixture unreadable, the traversal cases would prove nothing: %v", err)
	}

	for _, target := range []string{
		"/v1/filler/media/../../secret.txt",
		"/v1/filler/media/%2e%2e%2f%2e%2e%2fsecret.txt",
		"/v1/filler/media/sub/%2e%2e/%2e%2e/%2e%2e/secret.txt",
		// A hash-shaped but uncatalogued value: resolves to nothing in the store.
		"/v1/filler/media/deadbeef000000000000000000000000000000000000000000000000000000",
	} {
		if res := getMedia(t, srv, target, memberToken); res.StatusCode == http.StatusOK {
			t.Errorf("%s returned 200 — a caller reached bytes it should not have; only a catalogued clip's hash resolves", target)
		}
	}
}

// §19 requires the negative. Unauthenticated must not reach clip bytes.
func TestServeFillerMedia_RequiresASession(t *testing.T) {
	srv, _, _ := newMediaServer(t)

	if res := getMedia(t, srv, "/v1/filler/media/"+mediaHash, ""); res.StatusCode == http.StatusOK {
		t.Error("served clip bytes with no credential")
	}
}
