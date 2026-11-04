package testkit

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync"
	"testing"
)

// ArrLibraryAPIKey is the key the arr library doubles accept.
const ArrLibraryAPIKey = "arr-test-key"

// ArrLibrary is the shared Sonarr/Radarr library double: the read side of the v3 API that the
// metadata providers use, served from the pinned fixtures under fixtures/sonarr and
// fixtures/radarr. Every title in them is invented and every id is above 90,000,000.
//
// It checks X-Api-Key, filters the list endpoints by the query parameters the real apps honour,
// records each request path and query, and can be told to fail one path.
type ArrLibrary struct {
	*httptest.Server
	mu       sync.Mutex
	requests []string
	fail     map[string]int
}

// NewSonarrLibrary serves /api/v3/series (?tvdbId=), /api/v3/episode?seriesId= and
// /api/v3/episodefile?seriesId= from fixtures/sonarr.
func NewSonarrLibrary(t testing.TB) *ArrLibrary {
	t.Helper()
	a := &ArrLibrary{fail: map[string]int{}}
	series := decodeFixtureList(t, "sonarr/series.json")
	episodes := decodeFixtureList(t, "sonarr/episode.json")
	files := decodeFixtureList(t, "sonarr/episodefile.json")
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v3/series", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, filterList(series, "tvdbId", r.URL.Query().Get("tvdbId")))
	})
	mux.HandleFunc("GET /api/v3/episode", func(w http.ResponseWriter, r *http.Request) {
		id := r.URL.Query().Get("seriesId")
		if id == "" {
			http.Error(w, `{"message":"seriesId or episodeIds must be provided"}`, http.StatusBadRequest)
			return
		}
		writeJSON(w, filterList(episodes, "seriesId", id))
	})
	mux.HandleFunc("GET /api/v3/episodefile", func(w http.ResponseWriter, r *http.Request) {
		id := r.URL.Query().Get("seriesId")
		if id == "" {
			http.Error(w, `{"message":"seriesId or episodeFileIds must be provided"}`, http.StatusBadRequest)
			return
		}
		writeJSON(w, filterList(files, "seriesId", id))
	})
	a.Server = httptest.NewServer(a.guard(mux))
	t.Cleanup(a.Close)
	return a
}

// NewRadarrLibrary serves /api/v3/movie (?tmdbId=) and /api/v3/config/metadata from
// fixtures/radarr.
func NewRadarrLibrary(t testing.TB) *ArrLibrary {
	t.Helper()
	a := &ArrLibrary{fail: map[string]int{}}
	movies := decodeFixtureList(t, "radarr/movie.json")
	metadata := Fixture(t, "radarr/config_metadata.json")
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v3/movie", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, filterList(movies, "tmdbId", r.URL.Query().Get("tmdbId")))
	})
	mux.HandleFunc("GET /api/v3/config/metadata", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(metadata)
	})
	a.Server = httptest.NewServer(a.guard(mux))
	t.Cleanup(a.Close)
	return a
}

// Fail makes every request to path answer status. path may carry a query, such as
// "/api/v3/episode?seriesId=2", to fail only that exact request.
func (a *ArrLibrary) Fail(path string, status int) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.fail[path] = status
}

// Requests returns each request's path and query, in order.
func (a *ArrLibrary) Requests() []string {
	a.mu.Lock()
	defer a.mu.Unlock()
	return append([]string(nil), a.requests...)
}

func (a *ArrLibrary) guard(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		a.mu.Lock()
		a.requests = append(a.requests, r.URL.RequestURI())
		status, failing := a.fail[r.URL.RequestURI()]
		if !failing {
			status, failing = a.fail[r.URL.Path]
		}
		a.mu.Unlock()
		if r.Header.Get("X-Api-Key") != ArrLibraryAPIKey {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		if failing {
			w.WriteHeader(status)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func decodeFixtureList(t testing.TB, rel string) []map[string]any {
	t.Helper()
	var out []map[string]any
	if err := json.Unmarshal(Fixture(t, rel), &out); err != nil {
		t.Fatalf("decode fixture %q: %v", rel, err)
	}
	return out
}

// filterList keeps the items whose numeric field equals want; an empty want keeps all.
func filterList(items []map[string]any, field, want string) []map[string]any {
	out := []map[string]any{}
	n, err := strconv.ParseFloat(want, 64)
	for _, item := range items {
		if want == "" {
			out = append(out, item)
			continue
		}
		if v, ok := item[field].(float64); ok && err == nil && v == n {
			out = append(out, item)
		}
	}
	return out
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}
