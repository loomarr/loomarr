package api_test

import (
	"context"
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

// thumbHash is the content hash the byte-route tests address the seeded clip by. ⚠ Since V45a the
// byte routes take the clip HASH and resolve the disk path via the store, so the test must seed a
// clip whose hash maps to the nested path whose thumbnail exists on disk.
const thumbHash = "thumbhash000000000000000000000000000000000000000000000000000000"

// nothumbHash is a catalogued clip whose thumbnail file was NEVER written — it maps to
// `90s/cereal/frosted.mp4`, for which seedThumbClip generates no .jpg. Addressing it exercises
// the "clip exists, thumbnail missing" 404 (the common case), distinct from an uncatalogued hash.
const nothumbHash = "nothumbhash00000000000000000000000000000000000000000000000000000"

// seedThumbClip registers `thumbHash` → `80s/toys/intro.mp4` (thumbnail present) and `nothumbHash`
// → `90s/cereal/frosted.mp4` (thumbnail absent) so the byte handlers can resolve each hash to its
// on-disk location.
func seedThumbClip(t *testing.T, st store.Store) {
	t.Helper()
	c := store.Clip{}
	c.Hash = thumbHash
	c.Path = "80s/toys/intro.mp4"
	c.Name = "intro"
	c.Kind = filler.Commercial
	c.DurationMs = 30000
	if err := st.UpsertClip(context.Background(), c); err != nil {
		t.Fatal(err)
	}
	nc := store.Clip{}
	nc.Hash = nothumbHash
	nc.Path = "90s/cereal/frosted.mp4"
	nc.Name = "frosted"
	nc.Kind = filler.Commercial
	nc.DurationMs = 30000
	if err := st.UpsertClip(context.Background(), nc); err != nil {
		t.Fatal(err)
	}
}

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

	st, err := store.Open(context.Background(), "sqlite://"+filepath.Join(root, "thumb.db"), true)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	seedThumbClip(t, st)

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

// The clip is addressed by its content HASH (V45a); the handler resolves it to the nested disk path
// whose thumbnail exists. A nested clip's structure is preserved on disk (V28), and the hash lookup
// finds it regardless of nesting.
func TestServeFillerThumb_ServesANestedClipsThumbnail(t *testing.T) {
	srv, _ := newThumbServer(t)

	res := getThumb(t, srv, "/v1/filler/thumb/"+thumbHash, memberToken)

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

// Traversal is UNEXPRESSIBLE since V45a: the wire identity is a clip's content HASH, not a path.
// A caller cannot address an arbitrary disk location — the handler only serves a path it looked up
// in the store via `clipPathByHash`, and that path was written by the scanner, not the client. So
// the strongest attack a client can even spell is "supply a value that isn't a catalogued hash",
// which resolves to nothing and 404s.
//
// This pins that outcome: no traversal-shaped value, and no unknown hash, returns bytes. The old
// `../../secret.txt` spellings are kept below not because they could reach the file (they can't —
// there is no path in the URL anymore) but to prove that even a caller who tries the classic
// attack gets a 404, never the secret.
func TestServeFillerThumb_TraversalNeverReturnsBytes(t *testing.T) {
	srv, root := newThumbServer(t)

	// Sanity: the file a traversal would have reached for really is readable, or the cases below
	// would pass for the boring reason that nothing is there.
	if _, err := os.ReadFile(filepath.Join(root, "secret.txt")); err != nil {
		t.Fatalf("fixture unreadable, the traversal cases would prove nothing: %v", err)
	}

	for _, target := range []string{
		"/v1/filler/thumb/../../secret.txt",
		"/v1/filler/thumb/%2e%2e%2f%2e%2e%2fsecret.txt",
		"/v1/filler/thumb/sub/%2e%2e/%2e%2e/%2e%2e/secret.txt",
		// A hash-shaped but uncatalogued value: resolves to nothing in the store.
		"/v1/filler/thumb/deadbeef000000000000000000000000000000000000000000000000000000",
	} {
		res := getThumb(t, srv, target, memberToken)
		if res.StatusCode == http.StatusOK {
			t.Errorf("%s returned 200 — a caller reached bytes it should not have; only a catalogued clip's hash resolves", target)
		}
	}
}

// A clip whose thumbnail was never generated (ffmpeg missing, extraction failed) is the
// COMMON case, not an error. V28 counts those failures at scan time, which is where an
// operator is told about them. Since V45a the byte route resolves the hash to a disk path
// via the store; a catalogued clip whose thumbnail file is absent still 404s — the seeded
// `nothumbHash` clip maps to `90s/cereal/frosted.mp4`, for which no .jpg was written.
func TestServeFillerThumb_MissingThumbnailIs404(t *testing.T) {
	srv, _ := newThumbServer(t)

	res := getThumb(t, srv, "/v1/filler/thumb/"+nothumbHash, memberToken)

	if res.StatusCode != http.StatusNotFound {
		t.Errorf("status = %d, want 404 for a clip with no extracted frame", res.StatusCode)
	}
}

// Member-visible, matching list-filler: the catalog listing is visible to any authenticated
// user and a thumbnail is strictly less information than the row it decorates. But NOT public
// — unlike the channel icon, which is deliberately open so Tunarr can fetch it.
func TestServeFillerThumb_RequiresAuth(t *testing.T) {
	srv, _ := newThumbServer(t)

	anon := getThumb(t, srv, "/v1/filler/thumb/"+thumbHash, "")
	if anon.StatusCode == http.StatusOK {
		t.Error("served a thumbnail to an anonymous caller")
	}

	member := getThumb(t, srv, "/v1/filler/thumb/"+thumbHash, memberToken)
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

	res := getThumb(t, srv, "/v1/filler/thumb/"+thumbHash, memberToken)
	if res.StatusCode != http.StatusNotFound {
		t.Errorf("status = %d, want 404 when filler.dir is unset", res.StatusCode)
	}
}
