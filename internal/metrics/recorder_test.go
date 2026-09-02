package metrics_test

import (
	"context"
	"database/sql"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/images/rustgen"
	"github.com/loomarr/loomarr/internal/metrics"
	"github.com/loomarr/loomarr/internal/provision"
)

type changingStoreCounts struct {
	mu        sync.Mutex
	titles    map[provision.State]int
	jobs      map[string]int
	oldest    map[string]time.Time
	titleErr  error
	oldestErr error
}

func (c *changingStoreCounts) CountTitlesByState(context.Context) (map[provision.State]int, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.titles, c.titleErr
}

func (c *changingStoreCounts) CountJobsByStatus(context.Context) (map[string]int, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.jobs, nil
}

func (c *changingStoreCounts) OldestProposalJobsByStatus(context.Context) (map[string]time.Time, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.oldest, c.oldestErr
}

func (*changingStoreCounts) CountProposalJobAttemptsByStatus(context.Context) (map[string]int, error) {
	return nil, nil
}

func (*changingStoreCounts) CountFailedProposalJobsByCode(context.Context) (map[string]int, error) {
	return nil, nil
}

func (*changingStoreCounts) CountActiveSessions(context.Context, time.Time) (int, error) {
	return 0, nil
}

func (c *changingStoreCounts) failTitlesAndSetJobs(jobs map[string]int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.titleErr = errors.New("database unavailable")
	c.jobs = jobs
}

func (c *changingStoreCounts) failOldestProposalJobs() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.oldestErr = errors.New("database unavailable")
}

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

func TestRecorderScrapePreservesLastSuccessfulSourceWhileOthersRefresh(t *testing.T) {
	counts := &changingStoreCounts{
		titles: map[provision.State]int{provision.Requested: 2},
		jobs:   map[string]int{"queued": 3},
	}
	recorder := metrics.New(metrics.Options{Store: counts, Now: func() time.Time {
		return time.Unix(1_800_000_000, 0).UTC()
	}})

	first := scrape(t, recorder)
	for _, want := range []string{
		`loomarr_titles{state="requested"} 2`,
		`loomarr_jobs{status="queued"} 3`,
	} {
		if !strings.Contains(first, want) {
			t.Fatalf("first scrape does not contain %q", want)
		}
	}

	counts.failTitlesAndSetJobs(map[string]int{"queued": 5})
	second := scrape(t, recorder)
	for _, want := range []string{
		`loomarr_titles{state="requested"} 2`,
		`loomarr_jobs{status="queued"} 5`,
		`loomarr_metrics_scrape_errors_total{source="titles"} 1`,
	} {
		if !strings.Contains(second, want) {
			t.Errorf("second scrape does not contain %q", want)
		}
	}
	if strings.Contains(second, `loomarr_jobs{status="queued"} 3`) {
		t.Error("second scrape retained stale Jobs after its source succeeded")
	}
}

func TestRecorderScrapePreservesExactProposalAgeWhenSourceFails(t *testing.T) {
	start := time.Unix(1_800_000_000, 0).UTC()
	now := start
	counts := &changingStoreCounts{
		oldest: map[string]time.Time{"queued": start.Add(-90 * time.Second)},
	}
	recorder := metrics.New(metrics.Options{Store: counts, Now: func() time.Time { return now }})

	first := scrape(t, recorder)
	if want := `loomarr_proposal_job_oldest_age_seconds{status="queued"} 90`; !strings.Contains(first, want) {
		t.Fatalf("first scrape does not contain %q", want)
	}

	now = start.Add(30 * time.Second)
	counts.failOldestProposalJobs()
	second := scrape(t, recorder)
	for _, want := range []string{
		`loomarr_proposal_job_oldest_age_seconds{status="queued"} 90`,
		`loomarr_metrics_scrape_errors_total{source="proposal_job_age"} 1`,
	} {
		if !strings.Contains(second, want) {
			t.Errorf("second scrape does not contain %q", want)
		}
	}
	if strings.Contains(second, `loomarr_proposal_job_oldest_age_seconds{status="queued"} 120`) {
		t.Error("second scrape recomputed proposal age from a stale timestamp")
	}
}

func TestRecorderConcurrentScrapesSynchronizeSuccessfulSnapshots(t *testing.T) {
	counts := &changingStoreCounts{
		titles: map[provision.State]int{provision.Available: 4},
		jobs:   map[string]int{"running": 2},
	}
	recorder := metrics.New(metrics.Options{Store: counts})

	const scrapes = 16
	start := make(chan struct{})
	results := make(chan string, scrapes)
	var wg sync.WaitGroup
	for range scrapes {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			response := httptest.NewRecorder()
			recorder.Handler().ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/metrics", nil))
			results <- response.Body.String()
		}()
	}
	close(start)
	wg.Wait()
	close(results)

	for body := range results {
		for _, want := range []string{
			`loomarr_titles{state="available"} 4`,
			`loomarr_jobs{status="running"} 2`,
		} {
			if !strings.Contains(body, want) {
				t.Errorf("concurrent scrape does not contain %q", want)
			}
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

func TestRecorderUsesBoundedNotificationOutboundTarget(t *testing.T) {
	recorder := metrics.New(metrics.Options{})
	transport := recorder.InstrumentTransport("notification-provider", roundTripFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusNoContent,
			Body:       io.NopCloser(strings.NewReader("")),
			Header:     make(http.Header),
		}, nil
	}))
	response, err := transport.RoundTrip(httptest.NewRequest(http.MethodPost, "http://loomarr.invalid/", nil))
	if err != nil {
		t.Fatal(err)
	}
	_ = response.Body.Close()

	body := scrape(t, recorder)
	if !strings.Contains(body, `loomarr_outbound_requests_total{code="204",target="notifications"} 1`) {
		t.Fatalf("scrape does not contain bounded notifications target:\n%s", body)
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

func TestRecorderRecordsLLMTokensWithoutProviderOrModelLabels(t *testing.T) {
	recorder := metrics.New(metrics.Options{})
	recorder.LLMTokens(21, 8)

	body := scrape(t, recorder)
	for _, want := range []string{
		`loomarr_llm_tokens_total{kind="prompt"} 21`,
		`loomarr_llm_tokens_total{kind="completion"} 8`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("scrape does not contain %q", want)
		}
	}
}

func TestRecorderHTTPUsesMatchedRouteAndCountsOutboundFanout(t *testing.T) {
	recorder := metrics.New(metrics.Options{})
	transport := recorder.InstrumentTransport("library", roundTripFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader("ok"))}, nil
	}))
	mux := http.NewServeMux()
	mux.HandleFunc("GET /v1/items/{id}", func(w http.ResponseWriter, request *http.Request) {
		for range 2 {
			response, err := transport.RoundTrip(request.Clone(request.Context()))
			if err != nil {
				t.Fatal(err)
			}
			_ = response.Body.Close()
		}
		w.WriteHeader(http.StatusNoContent)
	})
	handler := recorder.FanoutMiddleware(recorder.Middleware(mux))
	handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/v1/items/private-id", nil))

	body := scrape(t, recorder)
	if strings.Contains(body, "private-id") {
		t.Fatalf("scrape leaked raw request path:\n%s", body)
	}
	for _, want := range []string{
		`loomarr_http_requests_total{code="204",method="GET",route="/v1/items/{id}"} 1`,
		`loomarr_http_outbound_fanout_sum{method="GET",route="/v1/items/{id}"} 2`,
		`loomarr_outbound_requests_total{code="200",target="library"} 2`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("scrape does not contain %q", want)
		}
	}
}

func TestRecorderBoundsImageWorkerDimensions(t *testing.T) {
	recorder := metrics.New(metrics.Options{})
	recorder.ImageWorkerObserved(rustgen.Observation{
		Kind: "private-operation", Result: "secret failure", Duration: time.Millisecond,
	})
	recorder.ImageWorkerQueueWait("private-class", time.Millisecond)

	body := scrape(t, recorder)
	if strings.Contains(body, "private-operation") || strings.Contains(body, "secret failure") || strings.Contains(body, "private-class") {
		t.Fatalf("scrape leaked arbitrary image-worker labels:\n%s", body)
	}
	for _, want := range []string{
		`loomarr_image_worker_operations_total{kind="other",result="other"} 1`,
		`loomarr_image_worker_queue_wait_seconds_count{class="other"} 1`,
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
