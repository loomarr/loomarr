package api_test

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/loomarr/loomarr/internal/api"
	"github.com/loomarr/loomarr/internal/metrics"
)

// /metrics is unauthenticated on the LAN (§7) and exposes both the Go runtime
// collectors and Loomarr's own HTTP series (§18).
func TestMetricsExposed(t *testing.T) {
	srv, _ := newServer(t)

	// Drive one request so the labelled HTTP vecs emit a series (Prometheus
	// counter/histogram vecs produce no lines until a label set is observed).
	if r := do(t, srv, http.MethodGet, "/v1/healthz", "", ""); r != nil {
		_ = r.Body.Close()
	}

	resp := do(t, srv, http.MethodGet, "/v1/metrics", "", "")
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /v1/metrics without a token = %d, want 200 (unauthenticated ops)", resp.StatusCode)
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
	if r := do(t, srv, http.MethodGet, "/v1/healthz", "", ""); r != nil {
		_ = r.Body.Close()
	}

	resp := do(t, srv, http.MethodGet, "/v1/metrics", "", "")
	defer func() { _ = resp.Body.Close() }()
	body, _ := io.ReadAll(resp.Body)
	text := string(body)

	// The counter should carry the route pattern with the method stripped off.
	//
	// ⚠ `/v1/healthz`, because the probes moved under /v1. The label comes from r.Pattern, so a
	// caller using the bare alias records `route="/healthz"` — two labels for one endpoint. That
	// is a deliberate consequence of keeping the alias, and it is honest: it shows which callers
	// have not been migrated yet.
	if !strings.Contains(text, `route="/v1/healthz"`) {
		t.Errorf("expected a request recorded with route=\"/v1/healthz\"; got:\n%s",
			grepLines(text, "loomarr_http_requests_total"))
	}
	// And the method must be its own label, not folded into the route.
	if !strings.Contains(text, `method="GET"`) {
		t.Error("expected method=\"GET\" label on the request counter")
	}
}

func TestRouterUsesItsGenerationRecorderForTrafficAndScrapes(t *testing.T) {
	recorder := metrics.New(metrics.Options{
		Version: "v9.8.7", Revision: "generation-a", Database: "postgres",
	})
	srv := httptest.NewServer(api.Router(slog.New(slog.DiscardHandler), api.Options{
		Metrics: recorder,
	}))
	t.Cleanup(srv.Close)

	health, err := http.Get(srv.URL + "/v1/healthz")
	if err != nil {
		t.Fatalf("GET /v1/healthz: %v", err)
	}
	_ = health.Body.Close()

	response, err := http.Get(srv.URL + "/v1/metrics")
	if err != nil {
		t.Fatalf("GET /v1/metrics: %v", err)
	}
	defer func() { _ = response.Body.Close() }()
	body, _ := io.ReadAll(response.Body)
	text := string(body)
	for _, want := range []string{
		`loomarr_build_info{database="postgres",revision="generation-a",version="v9.8.7"} 1`,
		`loomarr_http_requests_total{code="200",method="GET",route="/v1/healthz"} 1`,
	} {
		if !strings.Contains(text, want) {
			t.Errorf("generation scrape does not contain %q\n%s", want, grepLines(text, "loomarr_"))
		}
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
