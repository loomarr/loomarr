package metrics_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/loomarr/loomarr/internal/metrics"
)

func TestRecorderScrapeIdentifiesBuildBackendAndRuntime(t *testing.T) {
	recorder := metrics.New(metrics.Options{
		Version:  "v1.2.3",
		Revision: "abc123",
		Database: "sqlite",
	})

	response := httptest.NewRecorder()
	recorder.Handler().ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/metrics", nil))

	if response.Code != http.StatusOK {
		t.Fatalf("GET /metrics status = %d, want 200", response.Code)
	}
	body := response.Body.String()
	for _, want := range []string{
		`loomarr_build_info{database="sqlite",revision="abc123",version="v1.2.3"} 1`,
		"# TYPE go_goroutines gauge",
		"# TYPE process_cpu_seconds_total counter",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("scrape does not contain %q", want)
		}
	}
}

func TestRecorderScrapeInitializesAndIsolatesKnownLoginOutcomes(t *testing.T) {
	first := metrics.New(metrics.Options{})
	first.LoginResult(true)

	firstBody := scrape(t, first)
	for _, want := range []string{
		`loomarr_auth_logins_total{result="failure"} 0`,
		`loomarr_auth_logins_total{result="success"} 1`,
	} {
		if !strings.Contains(firstBody, want) {
			t.Errorf("first scrape does not contain %q", want)
		}
	}

	secondBody := scrape(t, metrics.New(metrics.Options{}))
	for _, want := range []string{
		`loomarr_auth_logins_total{result="failure"} 0`,
		`loomarr_auth_logins_total{result="success"} 0`,
	} {
		if !strings.Contains(secondBody, want) {
			t.Errorf("fresh generation scrape does not contain %q", want)
		}
	}
}

func scrape(t *testing.T, recorder *metrics.Recorder) string {
	t.Helper()
	response := httptest.NewRecorder()
	recorder.Handler().ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	if response.Code != http.StatusOK {
		t.Fatalf("GET /metrics status = %d, want 200", response.Code)
	}
	return response.Body.String()
}
