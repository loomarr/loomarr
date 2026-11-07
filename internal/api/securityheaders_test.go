package api_test

import (
	"net/http"
	"testing"
)

// Every response carries the security headers: the SPA shell, an API answer and an API error alike,
// because each is a page a browser may be pointed at directly.
func TestEveryResponseCarriesTheSecurityHeaders(t *testing.T) {
	srv := newAuthFlowHarness(t).Server
	for _, path := range []string{"/", "/v1/healthz", "/v1/auth/me", "/v1/no-such-route"} {
		resp, err := http.Get(srv.URL + path)
		if err != nil {
			t.Fatal(err)
		}
		_ = resp.Body.Close()
		if got := resp.Header.Get("X-Content-Type-Options"); got != "nosniff" {
			t.Errorf("%s: X-Content-Type-Options = %q, want nosniff", path, got)
		}
		if got := resp.Header.Get("Referrer-Policy"); got != "same-origin" {
			t.Errorf("%s: Referrer-Policy = %q, want same-origin", path, got)
		}
	}
}
