package testkit

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/loomarr/loomarr/internal/provision"
)

// TMDB is the shared TMDB test double (AGENTS.md: one mock per service). It
// serves a small in-memory catalog for /search/multi and answers /movie/{id} +
// /tv/{id} exists-checks, so grounding tests can distinguish a real id from a
// fabricated one (the id-not-in-catalog case is the LLM-hallucination path §8).
type TMDB struct {
	*httptest.Server
	mu       sync.Mutex
	requests []TMDBRequest
	// movies/series are the "real" ids the mock knows. /search/multi returns
	// matches by name substring; Exists (GET /movie|tv/{id}) 200s iff the id is
	// here, else 404 — that 404 is what the suggester's validation drops.
	movies map[int]tmdbTitle
	series map[int]tmdbTitle
	// recommends is the adjacency graph /{movie,tv}/{id}/recommendations serves
	// (programming-design §8.3): seed tmdb id → the ids it recommends. Empty for a
	// seed the test didn't wire, which is the real API's behaviour for an obscure
	// title — an unproductive seed, not an error.
	recommends map[int][]int
}

// TMDBRequest is one request observed by the shared TMDB adapter. It records
// only the fields adapter contract tests need and deliberately keeps secrets
// out of URLs by exposing the query separately from Authorization.
type TMDBRequest struct {
	Path          string
	RawQuery      string
	Authorization string
}

// Requests returns a stable copy of all requests received so far.
func (m *TMDB) Requests() []TMDBRequest {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]TMDBRequest(nil), m.requests...)
}

// RequestCount returns the number of requests received so far.
func (m *TMDB) RequestCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.requests)
}

// WithRecommendations wires the adjacency graph a test wants /recommendations to serve.
// Seeds absent from the map return an empty result set, so a test can assert the
// best-effort skip without standing up a failing server.
func (m *TMDB) WithRecommendations(graph map[int][]int) *TMDB {
	m.recommends = graph
	return m
}

type tmdbTitle struct {
	ID       int
	Name     string
	Year     int
	Date     string
	GenreIDs []int  // §8 enrichment: TMDB genre ids (28=Action, 878=Sci-Fi, …)
	Overview string // short synopsis the model reasons about
	// USRating is the US content rating the /content_ratings (tv) or /release_dates
	// (movie) endpoint reports (§389 acquisition enrichment). Empty ⇒ no US rating,
	// which is the common sparse-coverage case a test may assert is handled.
	USRating string
}

// SetRating scripts a title's US content rating so a test can drive the §389
// acquisition-enrichment path. Adds the id to the relevant catalog if absent.
func (m *TMDB) SetRating(mt provision.MediaType, tmdbID int, rating string) {
	cat := m.movies
	if mt == provision.Series {
		cat = m.series
	}
	t := cat[tmdbID]
	t.ID, t.USRating = tmdbID, rating
	cat[tmdbID] = t
}

// NewTMDB starts a mock TMDB with a fixed small catalog (Speed/The Rock movies,
// one series) — enough to exercise search + exists + the fabricated-id path.
func NewTMDB(t testing.TB) *TMDB {
	t.Helper()
	m := &TMDB{
		movies: map[int]tmdbTitle{
			100: {ID: 100, Name: "Speed", Year: 1994, Date: "1994-06-10", GenreIDs: []int{28, 53}, Overview: "A cop must keep a bus above 50mph or a bomb detonates."},
			101: {ID: 101, Name: "The Rock", Year: 1996, Date: "1996-06-07", GenreIDs: []int{28, 12, 53}, Overview: "A chemist and an ex-con storm Alcatraz to stop a rogue general."},
			603: {ID: 603, Name: "The Matrix", Year: 1999, Date: "1999-03-31", GenreIDs: []int{28, 878}, Overview: "A hacker learns reality is a simulation and joins a rebellion."},
		},
		series: map[int]tmdbTitle{
			1396: {ID: 1396, Name: "Breaking Bad", Year: 2008, Date: "2008-01-20", GenreIDs: []int{18, 80}, Overview: "A chemistry teacher turns to making meth."},
		},
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /search/multi", func(w http.ResponseWriter, r *http.Request) {
		q := strings.ToLower(r.URL.Query().Get("query"))
		var results []map[string]any
		for _, mv := range m.movies {
			if strings.Contains(strings.ToLower(mv.Name), q) {
				results = append(results, movieRow(mv))
			}
		}
		for _, s := range m.series {
			if strings.Contains(strings.ToLower(s.Name), q) {
				results = append(results, tvRow(s))
			}
		}
		// Include a person result to prove it's filtered out.
		results = append(results, map[string]any{"id": 9999, "media_type": "person", "name": "Some Actor"})
		_ = json.NewEncoder(w).Encode(map[string]any{"results": results})
	})
	// /discover/{movie,tv}: filter the catalog by with_genres (§8 discovery path).
	// A comma/pipe-separated with_genres matches any listed genre id; empty = all.
	mux.HandleFunc("GET /discover/movie", func(w http.ResponseWriter, r *http.Request) {
		want := parseGenreParam(r.URL.Query().Get("with_genres"))
		var results []map[string]any
		for _, mv := range m.movies {
			if genreMatch(mv.GenreIDs, want) {
				results = append(results, movieRow(mv))
			}
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"results": results})
	})
	mux.HandleFunc("GET /discover/tv", func(w http.ResponseWriter, r *http.Request) {
		want := parseGenreParam(r.URL.Query().Get("with_genres"))
		var results []map[string]any
		for _, s := range m.series {
			if genreMatch(s.GenreIDs, want) {
				results = append(results, tvRow(s))
			}
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"results": results})
	})
	// /{movie,tv}/{id}/recommendations: the §8.3 adjacency graph. Registered BEFORE the
	// bare /{id} routes so the more specific pattern wins.
	mux.HandleFunc("GET /movie/{id}/recommendations", func(w http.ResponseWriter, r *http.Request) {
		m.recommendHandler(w, r, m.movies)
	})
	mux.HandleFunc("GET /tv/{id}/recommendations", func(w http.ResponseWriter, r *http.Request) {
		m.recommendHandler(w, r, m.series)
	})
	mux.HandleFunc("GET /movie/{id}", func(w http.ResponseWriter, r *http.Request) {
		m.existsHandler(w, r, m.movies)
	})
	mux.HandleFunc("GET /tv/{id}", func(w http.ResponseWriter, r *http.Request) {
		m.existsHandler(w, r, m.series)
	})
	// Content ratings (§389 acquisition enrichment). A US entry iff the title scripts
	// a USRating; an empty results array is the realistic sparse-coverage answer.
	mux.HandleFunc("GET /tv/{id}/content_ratings", func(w http.ResponseWriter, r *http.Request) {
		results := []map[string]any{}
		if t, ok := m.series[atoiPath(r.PathValue("id"))]; ok && t.USRating != "" {
			results = append(results, map[string]any{"iso_3166_1": "US", "rating": t.USRating})
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"results": results})
	})
	mux.HandleFunc("GET /movie/{id}/release_dates", func(w http.ResponseWriter, r *http.Request) {
		results := []map[string]any{}
		if t, ok := m.movies[atoiPath(r.PathValue("id"))]; ok && t.USRating != "" {
			results = append(results, map[string]any{
				"iso_3166_1":    "US",
				"release_dates": []map[string]any{{"certification": t.USRating}},
			})
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"results": results})
	})
	m.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		request := TMDBRequest{
			Path:          r.URL.Path,
			RawQuery:      r.URL.RawQuery,
			Authorization: r.Header.Get("Authorization"),
		}
		m.mu.Lock()
		m.requests = append(m.requests, request)
		m.mu.Unlock()
		mux.ServeHTTP(w, r)
	}))
	t.Cleanup(m.Close)
	return m
}

func (m *TMDB) existsHandler(w http.ResponseWriter, r *http.Request, cat map[int]tmdbTitle) {
	id := atoiPath(r.PathValue("id"))
	if t, ok := cat[id]; ok {
		_ = json.NewEncoder(w).Encode(map[string]any{"id": t.ID, "title": t.Name})
		return
	}
	w.WriteHeader(http.StatusNotFound) // fabricated id → 404 (grounding drops it)
}

func atoiPath(s string) int {
	n := 0
	for _, r := range s {
		if r < '0' || r > '9' {
			return 0
		}
		n = n*10 + int(r-'0')
	}
	return n
}

// movieRow / tvRow render a title as a /search or /discover result row, now
// carrying genre_ids + overview (§8 enrichment).
func movieRow(mv tmdbTitle) map[string]any {
	return map[string]any{
		"id": mv.ID, "media_type": "movie", "title": mv.Name, "release_date": mv.Date,
		"genre_ids": mv.GenreIDs, "overview": mv.Overview,
	}
}

func tvRow(s tmdbTitle) map[string]any {
	return map[string]any{
		"id": s.ID, "media_type": "tv", "name": s.Name, "first_air_date": s.Date,
		"genre_ids": s.GenreIDs, "overview": s.Overview,
	}
}

// parseGenreParam splits TMDB's with_genres (comma = OR, pipe = OR here) into ids.
func parseGenreParam(v string) []int {
	if v == "" {
		return nil
	}
	var out []int
	for _, part := range strings.FieldsFunc(v, func(r rune) bool { return r == ',' || r == '|' }) {
		if id := atoiPath(strings.TrimSpace(part)); id != 0 {
			out = append(out, id)
		}
	}
	return out
}

// genreMatch reports whether have shares any id with want (empty want = match all).
func genreMatch(have, want []int) bool {
	if len(want) == 0 {
		return true
	}
	for _, w := range want {
		for _, h := range have {
			if h == w {
				return true
			}
		}
	}
	return false
}

// recommendHandler serves the §8.3 adjacency graph for one seed. An unwired seed returns
// an empty result list (200), matching TMDB's answer for a title with no neighbours —
// the case the catalog walk must skip rather than fail on.
func (m *TMDB) recommendHandler(w http.ResponseWriter, r *http.Request, from map[int]tmdbTitle) {
	id, _ := strconv.Atoi(r.PathValue("id"))
	var results []map[string]any
	for _, recID := range m.recommends[id] {
		if t, ok := from[recID]; ok {
			if _, isSeries := m.series[recID]; isSeries {
				results = append(results, tvRow(t))
			} else {
				results = append(results, movieRow(t))
			}
		}
	}
	_ = json.NewEncoder(w).Encode(map[string]any{"results": results})
}
