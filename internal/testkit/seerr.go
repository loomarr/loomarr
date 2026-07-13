package testkit

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// Seerr is the shared Seerr test double (CLAUDE.md: one mock per service). It
// records requests and lets a test choose the status it returns, so the §6
// idempotency contract (201 and 409 both = success) can be exercised.
type Seerr struct {
	*httptest.Server
	// Status is the code /api/v1/request returns (default 201). Set to 409 to
	// exercise the conflict-is-success path, or 500 for a failure.
	Status int
	// LastAPIKey / Requests capture what the requester sent.
	LastAPIKey string
	Requests   int
}

// NewSeerr starts a mock Seerr.
func NewSeerr(t testing.TB) *Seerr {
	t.Helper()
	s := &Seerr{Status: http.StatusCreated}
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/v1/request", func(w http.ResponseWriter, r *http.Request) {
		s.Requests++
		s.LastAPIKey = r.Header.Get("X-Api-Key")
		w.WriteHeader(s.Status)
		// The real body carries the media record (Phase 0); a minimal stub suffices.
		_, _ = w.Write([]byte(`{"type":"movie","media":{"status":5}}`))
	})
	s.Server = httptest.NewServer(mux)
	return s
}
