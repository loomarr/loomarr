package testkit

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
)

// Seerr is the shared Seerr test double (AGENTS.md: one mock per service). It
// records requests and lets a test choose the status it returns, so the §6
// idempotency contract (201 and 409 both = success) can be exercised.
type Seerr struct {
	*httptest.Server
	// Status is the code /api/v1/request returns (default 201). Set to 409 to
	// exercise the conflict-is-success path, or 500 for a failure.
	Status int
	// LastAPIKey / LastBody / Requests capture what the requester sent. LastBody is the
	// raw POST /request body, so a test can assert the exact payload (e.g. that a series
	// always sends `seasons`, "all" or an array — Jellyseerr 500s if it's omitted).
	LastAPIKey string
	LastBody   []byte
	Requests   int
	// Processing is what GET /api/v1/media?filter=processing returns, in order. Drives the
	// Seerr coarse queue-status path (§18.1). Empty ⇒ the endpoint returns no results.
	Processing []SeerrMedia
}

// SeerrMedia is one /api/v1/media record. TMDB numbers movies and TV separately, so the
// same TMDBID can appear once per MediaType.
type SeerrMedia struct {
	TMDBID    int
	MediaType string // "movie" | "tv"
	Status    int    // MediaStatus enum: 3=processing, 4=partially available
}

// NewSeerr starts a mock Seerr.
func NewSeerr(t testing.TB) *Seerr {
	t.Helper()
	s := &Seerr{Status: http.StatusCreated}
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/v1/request", func(w http.ResponseWriter, r *http.Request) {
		s.Requests++
		s.LastAPIKey = r.Header.Get("X-Api-Key")
		s.LastBody, _ = io.ReadAll(r.Body)
		w.WriteHeader(s.Status)
		// The real body carries the media record (Phase 0); a minimal stub suffices.
		_, _ = w.Write([]byte(`{"type":"movie","media":{"status":5}}`))
	})
	// Processing set for the coarse queue-status path (§18.1): GET /media?filter=processing
	// returns {pageInfo, results:[{tmdbId,mediaType,status}]} from s.Processing. It pages as Seerr
	// does: `take` rows (20 when absent) starting at `skip`, and pageInfo.results is the whole
	// count, so a client that reads only one page misses the rest. Empty ⇒ no results.
	mux.HandleFunc("GET /api/v1/media", func(w http.ResponseWriter, r *http.Request) {
		type item struct {
			TMDBID    int    `json:"tmdbId"`
			MediaType string `json:"mediaType"`
			Status    int    `json:"status"`
		}
		take, err := strconv.Atoi(r.URL.Query().Get("take"))
		if err != nil || take <= 0 {
			take = 20
		}
		skip, err := strconv.Atoi(r.URL.Query().Get("skip"))
		if err != nil || skip < 0 {
			skip = 0
		}
		start, end := min(skip, len(s.Processing)), min(skip+take, len(s.Processing))
		results := make([]item, 0, end-start)
		for _, m := range s.Processing[start:end] {
			results = append(results, item(m))
		}
		pages := (len(s.Processing) + take - 1) / take
		_ = json.NewEncoder(w).Encode(map[string]any{
			"pageInfo": map[string]int{"pages": pages, "pageSize": take, "results": len(s.Processing), "page": skip/take + 1},
			"results":  results,
		})
	})
	// Admin-authed endpoint the "test my Seerr" probe hits (Seerr.Reachable): 200 with
	// a key, 403 without — so the connection test validates the key, not just reachability.
	mux.HandleFunc("GET /api/v1/settings/main", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Api-Key") == "" {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		_, _ = w.Write([]byte(`{"apiKey":"ok"}`))
	})
	s.Server = httptest.NewServer(mux)
	return s
}
