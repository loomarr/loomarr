package testkit

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TMDB is the shared TMDB test double (CLAUDE.md: one mock per service). It
// serves a small in-memory catalog for /search/multi and answers /movie/{id} +
// /tv/{id} exists-checks, so grounding tests can distinguish a real id from a
// fabricated one (the id-not-in-catalog case is the LLM-hallucination path §8).
type TMDB struct {
	*httptest.Server
	// movies/series are the "real" ids the mock knows. /search/multi returns
	// matches by name substring; Exists (GET /movie|tv/{id}) 200s iff the id is
	// here, else 404 — that 404 is what the suggester's validation drops.
	movies map[int]tmdbTitle
	series map[int]tmdbTitle
}

type tmdbTitle struct {
	ID   int
	Name string
	Year int
	Date string
}

// NewTMDB starts a mock TMDB with a fixed small catalog (Speed/The Rock movies,
// one series) — enough to exercise search + exists + the fabricated-id path.
func NewTMDB(t testing.TB) *TMDB {
	t.Helper()
	m := &TMDB{
		movies: map[int]tmdbTitle{
			100: {100, "Speed", 1994, "1994-06-10"},
			101: {101, "The Rock", 1996, "1996-06-07"},
			603: {603, "The Matrix", 1999, "1999-03-31"},
		},
		series: map[int]tmdbTitle{
			1396: {1396, "Breaking Bad", 2008, "2008-01-20"},
		},
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /search/multi", func(w http.ResponseWriter, r *http.Request) {
		q := strings.ToLower(r.URL.Query().Get("query"))
		var results []map[string]any
		for _, mv := range m.movies {
			if strings.Contains(strings.ToLower(mv.Name), q) {
				results = append(results, map[string]any{
					"id": mv.ID, "media_type": "movie", "title": mv.Name, "release_date": mv.Date,
				})
			}
		}
		for _, s := range m.series {
			if strings.Contains(strings.ToLower(s.Name), q) {
				results = append(results, map[string]any{
					"id": s.ID, "media_type": "tv", "name": s.Name, "first_air_date": s.Date,
				})
			}
		}
		// Include a person result to prove it's filtered out.
		results = append(results, map[string]any{"id": 9999, "media_type": "person", "name": "Some Actor"})
		_ = json.NewEncoder(w).Encode(map[string]any{"results": results})
	})
	mux.HandleFunc("GET /movie/{id}", func(w http.ResponseWriter, r *http.Request) {
		m.existsHandler(w, r, m.movies)
	})
	mux.HandleFunc("GET /tv/{id}", func(w http.ResponseWriter, r *http.Request) {
		m.existsHandler(w, r, m.series)
	})
	m.Server = httptest.NewServer(mux)
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
