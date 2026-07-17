package programmer

import (
	"context"
	"encoding/json"
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
}

func newFakeTunarr() *fakeTunarrMediaAPI {
	return &fakeTunarrMediaAPI{libs: map[string][]map[string]any{}}
}

func (f *fakeTunarrMediaAPI) server(t *testing.T) *httptest.Server {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/media-sources", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(f.sources)
	})
	mux.HandleFunc("POST /api/media-sources", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&f.posted)
		f.nextID++
		id := "src-" + string(rune('0'+f.nextID))
		f.sources = append(f.sources, map[string]any{"id": id, "type": f.posted["type"], "uri": f.posted["uri"]})
		// A fresh source enumerates the media server's libraries (disabled).
		f.libs[id] = []map[string]any{
			{"id": "lib-mov", "name": "Movies", "mediaType": "movies", "enabled": false},
			{"id": "lib-tv", "name": "TV shows", "mediaType": "shows", "enabled": false},
			{"id": "lib-music", "name": "Music", "mediaType": "music", "enabled": false},
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"id": id})
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
	return NewDynamic(func() (string, string) { return url, "" }, "")
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
