package tunarr

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// fakeTunarrMediaAPI models Tunarr's media-source endpoints (the shapes verified
// against real Tunarr 1.3.8 + Emby 4.10): list/create sources, list libraries,
// enable a library, scan it. State is in-memory so idempotency can be asserted.
type fakeTunarrMediaAPI struct {
	sources []map[string]any // {id,type,uri}
	libs    map[string][]map[string]any
	scans   int
	posted  map[string]any // last POST /media-sources body (assert token passthrough)
	nextID  int
	puts    []string // ids of sources updated via PUT /media-sources/{id}
}

// token returns the access token Tunarr currently holds for a source.
func (f *fakeTunarrMediaAPI) token(id string) any {
	for _, s := range f.sources {
		if s["id"] == id {
			return s["accessToken"]
		}
	}
	return nil
}

func newFakeTunarr() *fakeTunarrMediaAPI {
	return &fakeTunarrMediaAPI{libs: map[string][]map[string]any{}}
}

func (f *fakeTunarrMediaAPI) server(t *testing.T) *httptest.Server {
	mux := http.NewServeMux()
	// Reads redact the access token, as Tunarr does.
	redacted := func(s map[string]any) map[string]any {
		out := map[string]any{}
		for k, v := range s {
			out[k] = v
		}
		out["accessToken"] = "REDACTED_TOKEN"
		return out
	}
	mux.HandleFunc("GET /api/media-sources", func(w http.ResponseWriter, _ *http.Request) {
		out := []map[string]any{}
		for _, s := range f.sources {
			out = append(out, redacted(s))
		}
		_ = json.NewEncoder(w).Encode(out)
	})
	mux.HandleFunc("GET /api/media-sources/{id}", func(w http.ResponseWriter, r *http.Request) {
		for _, s := range f.sources {
			if s["id"] == r.PathValue("id") {
				_ = json.NewEncoder(w).Encode(redacted(s))
				return
			}
		}
		http.NotFound(w, r)
	})
	mux.HandleFunc("POST /api/media-sources", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&f.posted)
		f.nextID++
		id := "src-" + string(rune('0'+f.nextID))
		src := map[string]any{"id": id}
		for k, v := range f.posted {
			src[k] = v
		}
		f.sources = append(f.sources, src)
		// A fresh source enumerates the media server's libraries (disabled).
		f.libs[id] = []map[string]any{
			{"id": "lib-mov", "name": "Movies", "mediaType": "movies", "enabled": false},
			{"id": "lib-tv", "name": "TV shows", "mediaType": "shows", "enabled": false},
			{"id": "lib-music", "name": "Music", "mediaType": "music", "enabled": false},
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"id": id})
	})
	mux.HandleFunc("PUT /api/media-sources/{id}", func(w http.ResponseWriter, r *http.Request) {
		// Like Tunarr's zod schema for an Emby source, the body must be the complete
		// record: a missing required field is a 400, and the stored record is replaced.
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		for _, field := range []string{"type", "name", "uri", "accessToken", "userId", "username", "pathReplacements"} {
			if _, ok := body[field]; !ok {
				http.Error(w, `{"error":"missing `+field+`"}`, http.StatusBadRequest)
				return
			}
		}
		f.puts = append(f.puts, r.PathValue("id"))
		for i, s := range f.sources {
			if s["id"] == r.PathValue("id") {
				body["id"] = s["id"]
				f.sources[i] = body
				_ = json.NewEncoder(w).Encode(redacted(body))
				return
			}
		}
		http.NotFound(w, r)
	})
	mux.HandleFunc("GET /api/media-sources/{id}/libraries", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(f.libs[r.PathValue("id")])
	})
	mux.HandleFunc("PUT /api/media-sources/{id}/libraries/{lib}", func(w http.ResponseWriter, r *http.Request) {
		for _, l := range f.libs[r.PathValue("id")] {
			if l["id"] == r.PathValue("lib") {
				l["enabled"] = true
			}
		}
		w.WriteHeader(http.StatusOK)
	})
	mux.HandleFunc("POST /api/media-sources/{id}/libraries/{lib}/scan", func(w http.ResponseWriter, _ *http.Request) {
		f.scans++
		w.WriteHeader(http.StatusAccepted)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func newTunarrAt(url string) *Tunarr {
	return NewDynamic(func() Config { return Config{BaseURL: url} })
}

func TestEnsureEmbySource_CreatesThenReuses(t *testing.T) {
	f := newFakeTunarr()
	tn := newTunarrAt(f.server(t).URL)

	id1, err := tn.EnsureEmbySource(context.Background(), "emby", "http://emby:8096", "admin-token", "u1")
	if err != nil {
		t.Fatalf("EnsureEmbySource: %v", err)
	}
	if f.posted["accessToken"] != "admin-token" {
		t.Errorf("source created with accessToken=%v, want the admin token (no user login)", f.posted["accessToken"])
	}
	// Second call for the same URL must REUSE, not create a duplicate (idempotent).
	id2, err := tn.EnsureEmbySource(context.Background(), "emby", "http://emby:8096/", "admin-token", "u1")
	if err != nil {
		t.Fatalf("EnsureEmbySource (2nd): %v", err)
	}
	if id1 != id2 {
		t.Errorf("2nd EnsureEmbySource made a new source (%s vs %s) — not idempotent", id1, id2)
	}
	if len(f.sources) != 1 {
		t.Errorf("want exactly 1 source after two ensures, got %d", len(f.sources))
	}
}

// A rotated library token must reach the source Loomarr registered: Tunarr streams the
// library with the token it holds, so a stale one breaks playback silently.
func TestEnsureEmbySource_RotatedTokenUpdatesOwnedSource(t *testing.T) {
	f := newFakeTunarr()
	tn := newTunarrAt(f.server(t).URL)
	ctx := context.Background()

	id1, err := tn.EnsureEmbySource(ctx, "emby", "http://emby:8096", "old-token", "u1")
	if err != nil {
		t.Fatalf("EnsureEmbySource: %v", err)
	}
	id2, err := tn.EnsureEmbySource(ctx, "emby", "http://emby:8096", "new-token", "u1")
	if err != nil {
		t.Fatalf("EnsureEmbySource after rotation: %v", err)
	}
	if id1 != id2 || len(f.sources) != 1 {
		t.Fatalf("rotation changed source identity: ids %s/%s, %d sources", id1, id2, len(f.sources))
	}
	if got := f.token(id1); got != "new-token" {
		t.Errorf("Tunarr source token = %v after rotation, want new-token", got)
	}
}

// Path replacements the operator configured on Loomarr's source survive a rotation:
// Tunarr's PUT replaces the record, and dropping a mapping trashes programs under it.
func TestEnsureEmbySource_RotationKeepsConfiguredFields(t *testing.T) {
	f := newFakeTunarr()
	tn := newTunarrAt(f.server(t).URL)
	ctx := context.Background()

	id, err := tn.EnsureEmbySource(ctx, "emby", "http://emby:8096", "old-token", "u1")
	if err != nil {
		t.Fatalf("EnsureEmbySource: %v", err)
	}
	mapping := []any{map[string]any{"serverPath": "/media", "localPath": "/mnt/media"}}
	f.sources[0]["pathReplacements"] = mapping // the operator configured this in Tunarr

	if _, err := tn.EnsureEmbySource(ctx, "emby", "http://emby:8096", "new-token", "u2"); err != nil {
		t.Fatalf("EnsureEmbySource after rotation: %v", err)
	}
	got, _ := json.Marshal(f.sources[0]["pathReplacements"])
	want, _ := json.Marshal(mapping)
	if string(got) != string(want) {
		t.Errorf("pathReplacements = %s after rotation, want %s", got, want)
	}
	if f.token(id) != "new-token" {
		t.Errorf("token = %v, want new-token", f.token(id))
	}
	if f.sources[0]["userId"] != "u1" || f.sources[0]["username"] != "loomarr" {
		t.Errorf("userId/username rewritten: %v/%v, want the stored values kept", f.sources[0]["userId"], f.sources[0]["username"])
	}
}

// Sources Loomarr did not create are never modified, even when they share the URL.
func TestEnsureEmbySource_LeavesForeignSourcesUntouched(t *testing.T) {
	f := newFakeTunarr()
	f.sources = []map[string]any{
		{"id": "mine-elsewhere", "type": "emby", "name": "Family Emby", "uri": "http://emby:8096", "accessToken": "theirs"},
		{"id": "other-server", "type": "jellyfin", "name": "Loomarr jellyfin", "uri": "http://jelly:8096", "accessToken": "other"},
	}
	tn := newTunarrAt(f.server(t).URL)

	id, err := tn.EnsureEmbySource(context.Background(), "emby", "http://emby:8096", "new-token", "u1")
	if err != nil {
		t.Fatalf("EnsureEmbySource: %v", err)
	}
	if id != "mine-elsewhere" {
		t.Errorf("source id = %q, want the existing same-URL source reused", id)
	}
	if len(f.puts) != 0 {
		t.Errorf("updated sources %v, want none: Loomarr did not create them", f.puts)
	}
	if f.token("mine-elsewhere") != "theirs" || f.token("other-server") != "other" {
		t.Error("a source Loomarr does not own had its token rewritten")
	}
}

// An update failure is returned, not swallowed, so the caller can report the stale token.
func TestEnsureEmbySource_UpdateFailureSurfaces(t *testing.T) {
	f := newFakeTunarr()
	srv := f.server(t)
	tn := newTunarrAt(srv.URL)
	ctx := context.Background()
	if _, err := tn.EnsureEmbySource(ctx, "emby", "http://emby:8096", "old-token", "u1"); err != nil {
		t.Fatalf("EnsureEmbySource: %v", err)
	}
	broken := newTunarrAt(failingPut(t, srv.URL).URL)
	if _, err := broken.EnsureEmbySource(ctx, "emby", "http://emby:8096", "new-token", "u1"); err == nil {
		t.Fatal("EnsureEmbySource returned nil although Tunarr rejected the token update")
	}
}

// failingPut proxies upstream but answers every PUT with 500.
func failingPut(t *testing.T, upstream string) *httptest.Server {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPut {
			http.Error(w, `{"error":"boom"}`, http.StatusInternalServerError)
			return
		}
		req, _ := http.NewRequestWithContext(r.Context(), r.Method, upstream+r.URL.Path, r.Body)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}
		defer func() { _ = resp.Body.Close() }()
		w.WriteHeader(resp.StatusCode)
		_, _ = io.Copy(w, resp.Body)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestConnectLibraries_EnablesMovieAndShowOnly_Idempotent(t *testing.T) {
	f := newFakeTunarr()
	tn := newTunarrAt(f.server(t).URL)
	id, _ := tn.EnsureEmbySource(context.Background(), "emby", "http://emby:8096", "tok", "u1")

	n, err := tn.ConnectLibraries(context.Background(), id)
	if err != nil {
		t.Fatalf("ConnectLibraries: %v", err)
	}
	if n != 2 {
		t.Errorf("enabled %d libraries, want 2 (movies + shows, not music)", n)
	}
	if f.scans != 2 {
		t.Errorf("triggered %d scans, want 2", f.scans)
	}
	if ready, _ := tn.MediaLibrariesReady(context.Background()); !ready {
		t.Error("MediaLibrariesReady=false after enabling movies+shows")
	}
	// Re-running must NOT re-scan already-enabled libraries (idempotent).
	if _, err := tn.ConnectLibraries(context.Background(), id); err != nil {
		t.Fatalf("ConnectLibraries (2nd): %v", err)
	}
	if f.scans != 2 {
		t.Errorf("2nd ConnectLibraries re-scanned (scans=%d) — not idempotent", f.scans)
	}
}

func TestMediaLibrariesReady_FalseUntilWired(t *testing.T) {
	f := newFakeTunarr()
	tn := newTunarrAt(f.server(t).URL)
	if ready, err := tn.MediaLibrariesReady(context.Background()); err != nil || ready {
		t.Errorf("MediaLibrariesReady=%v (err %v) with no source, want false", ready, err)
	}
	// Source exists but libraries not enabled yet → still not ready.
	id, _ := tn.EnsureEmbySource(context.Background(), "emby", "http://emby:8096", "tok", "u1")
	if ready, _ := tn.MediaLibrariesReady(context.Background()); ready {
		t.Error("MediaLibrariesReady=true before any library enabled")
	}
	_, _ = tn.ConnectLibraries(context.Background(), id)
	if !strings.HasPrefix(id, "src-") {
		t.Errorf("unexpected source id %q", id)
	}
}
