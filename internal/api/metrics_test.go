package api_test

import (
	"io"
	"net/http"
	"strings"
	"testing"
)

// /metrics is unauthenticated on the LAN (§7) and exposes both the Go runtime
// collectors and Loomarr's own HTTP series (§18).
func TestMetricsExposed(t *testing.T) {
	srv, _ := newServer(t)

	// Drive one request so the labelled HTTP vecs emit a series (Prometheus
	// counter/histogram vecs produce no lines until a label set is observed).
	if r := do(t, srv, http.MethodGet, "/healthz", "", ""); r != nil {
		_ = r.Body.Close()
	}

	resp := do(t, srv, http.MethodGet, "/metrics", "", "")
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /metrics without a token = %d, want 200 (unauthenticated ops)", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	text := string(body)

	// Runtime collectors come free from the default registry.
	if !strings.Contains(text, "go_goroutines") {
		t.Error("/metrics missing the Go runtime collectors (go_goroutines)")
	}
	// Loomarr's own HTTP series: the unlabelled gauge is always present; the
	// labelled counter + histogram appear once traffic has been recorded.
	for _, want := range []string{
		"loomarr_http_requests_total",
		"loomarr_http_request_duration_seconds",
		"loomarr_http_requests_in_flight",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("/metrics missing series %q", want)
		}
	}
}

// A served request is recorded against its matched route pattern, not the raw
// path — the label that keeps cardinality bounded (§18).
func TestMetricsRecordsRoute(t *testing.T) {
	srv, _ := newServer(t)

	// Drive a request through a known, low-cardinality route.
	if r := do(t, srv, http.MethodGet, "/healthz", "", ""); r != nil {
		_ = r.Body.Close()
	}

	resp := do(t, srv, http.MethodGet, "/metrics", "", "")
	defer func() { _ = resp.Body.Close() }()
	body, _ := io.ReadAll(resp.Body)
	text := string(body)

	// The counter should carry the route pattern with the method stripped off.
	if !strings.Contains(text, `route="/healthz"`) {
		t.Errorf("expected a request recorded with route=\"/healthz\"; got:\n%s",
			grepLines(text, "loomarr_http_requests_total"))
	}
	// And the method must be its own label, not folded into the route.
	if !strings.Contains(text, `method="GET"`) {
		t.Error("expected method=\"GET\" label on the request counter")
	}
}

func grepLines(s, substr string) string {
	var out []string
	for _, line := range strings.Split(s, "\n") {
		if strings.Contains(line, substr) {
			out = append(out, line)
		}
	}
	return strings.Join(out, "\n")
}
