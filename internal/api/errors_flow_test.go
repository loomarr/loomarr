package api_test

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
)

// The error-copy + correlation contract (§7), asserted over real HTTP so it exercises the
// request-id middleware AND the NewErrorWithContext hook, not just the handler return.

// decodeProblem reads an RFC 7807 problem body.
type problem struct {
	Title    string `json:"title"`
	Detail   string `json:"detail"`
	Status   int    `json:"status"`
	Instance string `json:"instance"`
}

func decodeProblem(t *testing.T, r io.Reader) problem {
	t.Helper()
	var p problem
	if err := json.NewDecoder(r).Decode(&p); err != nil {
		t.Fatalf("decode problem: %v", err)
	}
	return p
}

// A failed login returns a FRIENDLY problem: a Title-Case title and a full-sentence detail
// (not huma's "Unauthorized" / "invalid username or password"), plus a correlation id — and
// it must NOT reveal whether the username or the password was wrong (§11).
func TestLoginError_IsFriendlyAndCorrelated(t *testing.T) {
	srv, _, _ := authServer(t)

	body, _ := json.Marshal(map[string]string{"username": "boss", "password": "wrong"})
	req, _ := http.NewRequest(http.MethodPost, srv.URL+"/v1/auth/login", strings.NewReader(string(body)))
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", resp.StatusCode)
	}
	// Every response carries the correlation id header…
	if resp.Header.Get("X-Request-Id") == "" {
		t.Error("missing X-Request-Id header")
	}
	p := decodeProblem(t, resp.Body)
	// …and the problem body echoes it as the RFC 7807 instance.
	if p.Instance == "" || p.Instance != resp.Header.Get("X-Request-Id") {
		t.Errorf("instance %q should equal X-Request-Id %q", p.Instance, resp.Header.Get("X-Request-Id"))
	}
	// Title is a Title-Case human summary, not the bare status label.
	if p.Title == "" || p.Title == "Unauthorized" {
		t.Errorf("title = %q, want a friendly summary (not the status label)", p.Title)
	}
	// Detail is a full sentence (ends with a period), and must NOT name which field was wrong.
	if !strings.HasSuffix(p.Detail, ".") {
		t.Errorf("detail = %q, want a full sentence ending in a period", p.Detail)
	}
	low := strings.ToLower(p.Detail + " " + p.Title)
	if strings.Contains(low, "no such user") || strings.Contains(low, "unknown user") ||
		strings.Contains(low, "wrong password") || strings.Contains(low, "user not found") {
		t.Errorf("login error leaks which field was wrong: %q / %q", p.Title, p.Detail)
	}
}

// An inbound X-Request-Id from a trusted proxy is propagated (so the id spans hops), while a
// hostile one (control chars) is rejected and replaced — never echoed into logs/response.
func TestRequestID_PropagatesGoodRejectsBad(t *testing.T) {
	srv, _, _ := authServer(t)

	// A clean inbound id is honored end to end.
	req, _ := http.NewRequest(http.MethodGet, srv.URL+"/v1/auth/me", nil)
	req.Header.Set("X-Request-Id", "req_proxy_abc123")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if got := resp.Header.Get("X-Request-Id"); got != "req_proxy_abc123" {
		t.Errorf("clean inbound id = %q, want it propagated", got)
	}

	// A malformed id (disallowed chars — a log-injection vector) is rejected and replaced
	// with a fresh generated one. (Newlines can't even be sent by net/http, so we use other
	// out-of-charset bytes the sanitizer must still reject: spaces + angle brackets.)
	req2, _ := http.NewRequest(http.MethodGet, srv.URL+"/v1/auth/me", nil)
	req2.Header.Set("X-Request-Id", "bad id <script>")
	resp2, err := http.DefaultClient.Do(req2)
	if err != nil {
		t.Fatal(err)
	}
	_ = resp2.Body.Close()
	got := resp2.Header.Get("X-Request-Id")
	if got == "bad id <script>" || got == "" {
		t.Errorf("malformed id = %q, want it replaced with a generated one", got)
	}
	if !strings.HasPrefix(got, "req_") {
		t.Errorf("replacement id = %q, want a generated req_ id", got)
	}
}
