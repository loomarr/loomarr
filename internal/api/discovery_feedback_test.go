package api_test

import (
	"net/http"
	"testing"
)

func TestDiscoveryFeedbackAuthorizationAndRoundTrip(t *testing.T) {
	srv, _ := newServer(t)
	body := `{"scope":"household","targetKey":"movie:tmdb:603","action":"keep","reason":"family favorite"}`

	if resp := do(t, srv, http.MethodPost, "/v1/discovery/feedback", "", body); resp.StatusCode != http.StatusUnauthorized {
		_ = resp.Body.Close()
		t.Fatalf("anonymous write = %d, want 401", resp.StatusCode)
	} else {
		_ = resp.Body.Close()
	}
	if resp := do(t, srv, http.MethodPost, "/v1/discovery/feedback", memberToken, body); resp.StatusCode != http.StatusForbidden {
		_ = resp.Body.Close()
		t.Fatalf("member write = %d, want 403", resp.StatusCode)
	} else {
		_ = resp.Body.Close()
	}
	if resp := do(t, srv, http.MethodPost, "/v1/discovery/feedback", adminToken, body); resp.StatusCode != http.StatusOK {
		_ = resp.Body.Close()
		t.Fatalf("admin write = %d, want 200", resp.StatusCode)
	} else {
		_ = resp.Body.Close()
	}
	if resp := do(t, srv, http.MethodGet, "/v1/discovery/feedback?scope=household", memberToken, ""); resp.StatusCode != http.StatusOK {
		_ = resp.Body.Close()
		t.Fatalf("member read = %d, want 200", resp.StatusCode)
	} else {
		_ = resp.Body.Close()
	}
}
