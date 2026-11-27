package api_test

import (
	"net/http"
	"testing"
)

// A disabled admin's session reads nothing from the Ignored list (design §19).
func TestDisabledAdminLosesTheIgnoredList(t *testing.T) {
	h := newLibrarySessionHarness(t)
	srv := h.Server
	admin := login(t, srv, "boss", "pw")
	patch := func(body string) {
		resp := authed(t, http.MethodPatch, srv.URL+"/v1/users/u-kid", admin, body)
		_ = resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("patch %s = %d", body, resp.StatusCode)
		}
	}
	patch(`{"role":"admin"}`)
	second := login(t, srv, "kid", "pw")
	for _, step := range []struct {
		body string
		want int
	}{{"", http.StatusOK}, {`{"disabled":true}`, http.StatusUnauthorized}} {
		if step.body != "" {
			patch(step.body)
		}
		resp := authed(t, http.MethodGet, srv.URL+"/v1/library/ignored", second, "")
		_ = resp.Body.Close()
		if resp.StatusCode != step.want {
			t.Fatalf("second admin = %d, want %d", resp.StatusCode, step.want)
		}
	}
}
