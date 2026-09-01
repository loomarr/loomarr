package metrics_test

import (
	"database/sql"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

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

func TestRecorderBoundsOutboundTargetAndRetryReason(t *testing.T) {
	recorder := metrics.New(metrics.Options{})
	hostile := "https://user@example.invalid/private/title/123"
	transport := recorder.InstrumentTransport(hostile, roundTripFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusServiceUnavailable,
			Body:       io.NopCloser(strings.NewReader("unavailable")),
			Header:     make(http.Header),
		}, nil
	}))
	request := httptest.NewRequest(http.MethodGet, "http://loomarr.invalid/", nil)
	response, err := transport.RoundTrip(request)
	if err != nil {
		t.Fatal(err)
	}
	_ = response.Body.Close()
	recorder.OutboundRetried(hostile, metrics.OutboundRetryReason(255))

	body := scrape(t, recorder)
	if strings.Contains(body, hostile) || strings.Contains(body, "example.invalid") {
		t.Fatalf("scrape leaked hostile outbound target:\n%s", body)
	}
	for _, want := range []string{
		`loomarr_outbound_requests_total{code="503",target="other"} 1`,
		`loomarr_outbound_retries_total{reason="other",target="other"} 1`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("scrape does not contain %q", want)
		}
	}
}

func TestRecorderBoundsFillerMatchLevelAndRecordsRotationState(t *testing.T) {
	recorder := metrics.New(metrics.Options{})
	hostile := `exact",channel="private-channel`
	recorder.FillerPodAssembled(hostile)
	recorder.FillerRotationAired(true, true, false)

	body := scrape(t, recorder)
	if strings.Contains(body, hostile) || strings.Contains(body, "private-channel") {
		t.Fatalf("scrape leaked hostile filler match level:\n%s", body)
	}
	for _, want := range []string{
		`loomarr_filler_pods_total{match_level="other"} 1`,
		`loomarr_filler_rotation_airings_total{cooldown="relaxed",repeat="repeat"} 1`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("scrape does not contain %q", want)
		}
	}
}

func TestRecorderScrapeReportsLiveDatabasePoolStats(t *testing.T) {
	recorder := metrics.New(metrics.Options{
		DatabaseStats: func() sql.DBStats {
			return sql.DBStats{
				MaxOpenConnections: 10,
				OpenConnections:    7,
				InUse:              6,
				Idle:               1,
				WaitCount:          5,
				WaitDuration:       2 * time.Second,
				MaxIdleClosed:      3,
				MaxIdleTimeClosed:  4,
				MaxLifetimeClosed:  8,
			}
		},
	})

	body := scrape(t, recorder)
	for _, want := range []string{
		`loomarr_database_connections{state="idle"} 1`,
		`loomarr_database_connections{state="in_use"} 6`,
		`loomarr_database_connections{state="open"} 7`,
		`loomarr_database_max_open_connections 10`,
		`loomarr_database_connection_waits_total 5`,
		`loomarr_database_connection_wait_duration_seconds_total 2`,
		`loomarr_database_connections_closed_total{reason="idle_limit"} 3`,
		`loomarr_database_connections_closed_total{reason="idle_time"} 4`,
		`loomarr_database_connections_closed_total{reason="lifetime"} 8`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("scrape does not contain %q", want)
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

type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return fn(request)
}
