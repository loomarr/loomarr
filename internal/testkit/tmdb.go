package testkit

import (
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/loomarr/loomarr/internal/library/tmdb/tmdbstandin"
)

// TMDB is the shared TMDB test double (AGENTS.md: one mock per service): the one stand-in in
// internal/library/tmdb/tmdbstandin, which the demo library also serves, behind an httptest server that
// records requests. /search/multi returns matches by name substring; /movie|tv/{id} 200s iff the
// id is in the catalogue, else 404, which is what the suggester's validation drops (§8). The
// catalogue setters (AddMovie, SetRating, …) are the embedded Catalog's.
type TMDB struct {
	*httptest.Server
	*tmdbstandin.Catalog
	mu       sync.Mutex
	requests []TMDBRequest
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
	m.SetRecommendations(graph)
	return m
}

// NewTMDB starts a mock TMDB with a fixed small catalog (three movies, one
// series) — enough to exercise search + exists + the fabricated-id path. The
// titles come from the invented demo catalogue (internal/demolibrary) and the
// ids sit above its ProviderIDBase, so none names a real title (#1592). Search
// also returns a person row, which the client must filter out.
func NewTMDB(t testing.TB) *TMDB {
	t.Helper()
	m := &TMDB{Catalog: tmdbstandin.New().WithPersonDecoy()}
	m.PutMovie(tmdbstandin.Title{ID: 90000100, Name: "Quiet Engines", Year: 1994, Date: "1994-06-10", GenreIDs: []int{28, 53}, Overview: "A mechanic must keep a runaway tram moving or the bridge gives way."})
	m.PutMovie(tmdbstandin.Title{ID: 90000101, Name: "Lemon Tree Heist", Year: 1996, Date: "1996-06-07", GenreIDs: []int{28, 12, 53}, Overview: "Retirees storm a walled orchard to steal a prize-winning lemon tree."})
	m.PutMovie(tmdbstandin.Title{ID: 90000603, Name: "Marigold Protocol", Year: 1999, Date: "1999-03-31", GenreIDs: []int{28, 878}, Overview: "A florist learns her orders are coded messages and joins a rebellion."})
	m.PutSeries(tmdbstandin.Title{ID: 90001396, Name: "Copperline Foundry", Year: 2008, Date: "2008-01-20", GenreIDs: []int{18, 80}, Overview: "Three generations run a family foundry in a town that has stopped buying iron."})
	handler := m.Handler()
	m.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		m.mu.Lock()
		m.requests = append(m.requests, TMDBRequest{Path: r.URL.Path, RawQuery: r.URL.RawQuery, Authorization: r.Header.Get("Authorization")})
		m.mu.Unlock()
		handler.ServeHTTP(w, r)
	}))
	t.Cleanup(m.Close)
	return m
}
