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

// scrapeToken is the configured Prometheus scrape credential in these tests.
const scrapeToken = "test-scrape-token-0123456789"

// newMetricsHarness builds the production router with the scrape token configured (or empty
// for the fail-closed case) and returns a log buffer so a test can prove no credential is logged.
func newMetricsHarness(t *testing.T, token string) (*apiHarness, *strings.Builder) {
	t.Helper()
	logs := &strings.Builder{}
	h := startAPIHarness(t, func(defaults apiHarnessDefaults) http.Handler {
		return api.Router(slog.New(slog.NewTextHandler(logs, &slog.HandlerOptions{Level: slog.LevelDebug})), api.Options{
			Store: defaults.Store, Auth: defaults.Auth, MetricsToken: token,
		})
	})
	return h, logs
}

// scrape issues GET path with an optional bearer credential and extra headers.
func scrape(t *testing.T, h *apiHarness, path, bearer string, extra map[string]string) (int, string, http.Header) {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, h.Server.URL+path, nil)
	if err != nil {
		t.Fatal(err)
	}
	if bearer != "" {
		req.Header.Set("Authorization", "Bearer "+bearer)
	}
	for k, v := range extra {
		req.Header.Set(k, v)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	body, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(body), resp.Header
}

// /metrics requires the scrape token (§7, #1408): a public listener must not hand request
// volumes, route latency and process details to anyone who can reach it. Fail closed when no
// token is configured, and never let a Loomarr credential (admin API token, member, session)
// stand in for the scrape token.
func TestMetricsRequiresScrapeToken(t *testing.T) {
	for _, path := range []string{"/v1/metrics", "/metrics"} {
		t.Run(path, func(t *testing.T) {
			h, _ := newMetricsHarness(t, scrapeToken)

			if code, _, hdr := scrape(t, h, path, "", nil); code != http.StatusUnauthorized {
				t.Errorf("no credential = %d, want 401", code)
			} else if !strings.HasPrefix(hdr.Get("WWW-Authenticate"), "Bearer") {
				t.Errorf("401 lacks a Bearer challenge: %q", hdr.Get("WWW-Authenticate"))
			}
			for name, bearer := range map[string]string{
				"wrong token":             "not-the-token",
				"prefix of the token":     scrapeToken[:len(scrapeToken)-1],
				"token with extra suffix": scrapeToken + "x",
				"admin API token":         adminToken,
				"member token":            memberToken,
			} {
				if code, body, _ := scrape(t, h, path, bearer, nil); code != http.StatusUnauthorized {
					t.Errorf("%s = %d, want 401; body: %s", name, code, body)
				}
			}
			// A session cookie alone does not unlock it — a scrape job holds no session and a
			// member/admin login is a different credential.
			if code, _, _ := scrape(t, h, path, "", map[string]string{"Cookie": "loomarr_session=anything"}); code != http.StatusUnauthorized {
				t.Errorf("session cookie alone = %d, want 401", code)
			}
			// The correct token yields the exposition.
			code, body, _ := scrape(t, h, path, scrapeToken, nil)
			if code != http.StatusOK {
				t.Fatalf("correct token = %d, want 200; body: %s", code, body)
			}
			if !strings.Contains(body, "go_goroutines") {
				t.Error("authorized scrape missing the Go runtime collectors")
			}
		})
	}
}

// No token configured ⇒ refused for everyone, including a caller presenting a Loomarr
// credential, with a message that names the setting to fix.
func TestMetricsFailsClosedWithoutConfiguredToken(t *testing.T) {
	h, _ := newMetricsHarness(t, "")
	for _, bearer := range []string{"", adminToken, memberToken, "anything", " "} {
		code, body, _ := scrape(t, h, "/v1/metrics", bearer, nil)
		if code != http.StatusForbidden {
			t.Errorf("bearer %q with no token configured = %d, want 403", bearer, code)
		}
		if !strings.Contains(body, "LOOMARR_METRICS_TOKEN") {
			t.Errorf("refusal does not name LOOMARR_METRICS_TOKEN: %q", body)
		}
		if strings.Contains(body, "go_goroutines") {
			t.Error("refused response leaked metrics")
		}
	}
}

// The scrape token (and any guess at it) must never reach the log.
func TestMetricsTokenNeverLogged(t *testing.T) {
	h, logs := newMetricsHarness(t, scrapeToken)
	scrape(t, h, "/v1/metrics", "wrong-guess-secret", nil)
	scrape(t, h, "/v1/metrics", scrapeToken, nil)
	if out := logs.String(); strings.Contains(out, scrapeToken) || strings.Contains(out, "wrong-guess-secret") {
		t.Errorf("log output contains a bearer credential:\n%s", out)
	}
}

// /metrics is authenticated by the scrape token (§7) and exposes both the Go runtime
// collectors and Loomarr's own HTTP series (§18).
func TestMetricsExposed(t *testing.T) {
	harness, _ := newMetricsHarness(t, scrapeToken)

	// Drive one request so the labelled HTTP vecs emit a series (Prometheus
	// counter/histogram vecs produce no lines until a label set is observed).
	if r := harness.Do(http.MethodGet, "/v1/healthz", "", ""); r != nil {
		_ = r.Body.Close()
	}

	resp := harness.Do(http.MethodGet, "/v1/metrics", scrapeToken, "")
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /v1/metrics with the scrape token = %d, want 200", resp.StatusCode)
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
	harness, _ := newMetricsHarness(t, scrapeToken)

	// Drive a request through a known, low-cardinality route.
	if r := harness.Do(http.MethodGet, "/v1/healthz", "", ""); r != nil {
		_ = r.Body.Close()
	}

	resp := harness.Do(http.MethodGet, "/v1/metrics", scrapeToken, "")
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
		Metrics: recorder, MetricsToken: scrapeToken,
	}))
	t.Cleanup(srv.Close)

	health, err := http.Get(srv.URL + "/v1/healthz")
	if err != nil {
		t.Fatalf("GET /v1/healthz: %v", err)
	}
	_ = health.Body.Close()

	req, _ := http.NewRequest(http.MethodGet, srv.URL+"/v1/metrics", nil)
	req.Header.Set("Authorization", "Bearer "+scrapeToken)
	response, err := http.DefaultClient.Do(req)
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
