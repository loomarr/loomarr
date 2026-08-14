package testkit

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
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
	// Processing is what GET /api/v1/media?filter=processing returns: tmdbId → MediaStatus
	// enum (3=processing, 4=partially available). Drives the Seerr coarse queue-status path
	// (§18.1). Empty ⇒ the endpoint returns no results.
	Processing map[int]int
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
	// returns {results:[{tmdbId,status}]} from s.Processing. Empty ⇒ no results.
	mux.HandleFunc("GET /api/v1/media", func(w http.ResponseWriter, _ *http.Request) {
		type item struct {
			TMDBID int `json:"tmdbId"`
			Status int `json:"status"`
		}
		results := make([]item, 0, len(s.Processing))
		for id, st := range s.Processing {
			results = append(results, item{TMDBID: id, Status: st})
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"results": results})
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
