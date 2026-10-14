package httpx

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/metrics"
	"github.com/loomarr/loomarr/internal/testkit/httpfixture"
)

func retryResponse(status int, body string) *http.Response {
	return &http.Response{
		StatusCode: status,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}

type retryRecorder struct {
	waits   []time.Duration
	reasons []metrics.OutboundRetryReason
}

func deterministicRetry(next http.RoundTripper) (*retryTransport, *retryRecorder) {
	record := &retryRecorder{}
	retry := newRetryTransport(next, func(reason metrics.OutboundRetryReason) {
		record.reasons = append(record.reasons, reason)
	})
	retry.jitter = func(ceiling time.Duration) time.Duration { return ceiling }
	retry.wait = func(_ context.Context, delay time.Duration) error {
		record.waits = append(record.waits, delay)
		return nil
	}
	return retry, record
}

func TestRetryTransportRetriesBoundedStatuses(t *testing.T) {
	tests := []struct {
		status int
		reason metrics.OutboundRetryReason
	}{
		{408, metrics.OutboundRetryStatus408},
		{429, metrics.OutboundRetryStatus429},
		{500, metrics.OutboundRetryStatus500},
		{502, metrics.OutboundRetryStatus502},
		{503, metrics.OutboundRetryStatus503},
		{504, metrics.OutboundRetryStatus504},
	}
	for _, tc := range tests {
		t.Run(fmt.Sprint(tc.status), func(t *testing.T) {
			script := httpfixture.NewScriptedTransport(
				httpfixture.Step{Response: retryResponse(tc.status, "discard")},
				httpfixture.Step{Response: retryResponse(http.StatusOK, "ok")},
			)
			retry, record := deterministicRetry(script)
			req, _ := http.NewRequest(http.MethodGet, "http://service/", nil)
			resp, err := retry.RoundTrip(req)
			if err != nil {
				t.Fatal(err)
			}
			_ = resp.Body.Close()
			if script.Calls() != 2 || len(record.waits) != 1 {
				t.Fatalf("calls=%d waits=%v, want two calls and one wait", script.Calls(), record.waits)
			}
			if len(record.reasons) != 1 || record.reasons[0] != tc.reason {
				t.Fatalf("reasons = %v, want [%v]", record.reasons, tc.reason)
			}
		})
	}
}

func TestRetryTransportDoesNotRetryOtherStatuses(t *testing.T) {
	for _, status := range []int{200, 302, 400, 401, 403, 404, 409, 501} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			script := httpfixture.NewScriptedTransport(httpfixture.Step{
				Response: retryResponse(status, "kept"),
			})
			retry, record := deterministicRetry(script)
			req, _ := http.NewRequest(http.MethodGet, "http://service/", nil)
			resp, err := retry.RoundTrip(req)
			if err != nil {
				t.Fatal(err)
			}
			body, _ := io.ReadAll(resp.Body)
			_ = resp.Body.Close()
			if string(body) != "kept" || script.Calls() != 1 || len(record.waits) != 0 || len(record.reasons) != 0 {
				t.Fatalf("body=%q calls=%d waits=%v reasons=%v", body, script.Calls(), record.waits, record.reasons)
			}
		})
	}
}

func TestRetryTransportNeverRetriesWrites(t *testing.T) {
	for _, method := range []string{http.MethodHead, http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete} {
		t.Run(method, func(t *testing.T) {
			script := httpfixture.NewScriptedTransport(httpfixture.Step{
				Response: retryResponse(http.StatusServiceUnavailable, "kept"),
			})
			retry, _ := deterministicRetry(script)
			req, _ := http.NewRequest(method, "http://service/", nil)
			resp, err := retry.RoundTrip(req)
			if err != nil {
				t.Fatal(err)
			}
			_ = resp.Body.Close()
			if script.Calls() != 1 {
				t.Fatalf("calls = %d, want 1", script.Calls())
			}
		})
	}
}

func TestRetryTransportRecoversFromTransportErrors(t *testing.T) {
	boom := errors.New("connection refused")
	script := httpfixture.NewScriptedTransport(
		httpfixture.Step{Err: boom},
		httpfixture.Step{Err: boom},
		httpfixture.Step{Err: boom},
		httpfixture.Step{Response: retryResponse(http.StatusOK, "ok")},
	)
	retry, record := deterministicRetry(script)
	req, _ := http.NewRequest(http.MethodGet, "http://service/", nil)
	resp, err := retry.RoundTrip(req)
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	wantWaits := []time.Duration{200 * time.Millisecond, 400 * time.Millisecond, 800 * time.Millisecond}
	if fmt.Sprint(record.waits) != fmt.Sprint(wantWaits) {
		t.Fatalf("waits = %v, want %v", record.waits, wantWaits)
	}
	if len(record.reasons) != 3 {
		t.Fatalf("reasons = %v, want three", record.reasons)
	}
	for _, reason := range record.reasons {
		if reason != metrics.OutboundRetryTransport {
			t.Fatalf("reason = %v, want transport", reason)
		}
	}
}

func TestRetryTransportReturnsFourthTransportError(t *testing.T) {
	errs := []error{
		errors.New("first"),
		errors.New("second"),
		errors.New("third"),
		errors.New("fourth"),
	}
	steps := make([]httpfixture.Step, len(errs))
	for i := range errs {
		steps[i].Err = errs[i]
	}
	script := httpfixture.NewScriptedTransport(steps...)
	retry, _ := deterministicRetry(script)
	req, _ := http.NewRequest(http.MethodGet, "http://service/", nil)
	if _, err := retry.RoundTrip(req); !errors.Is(err, errs[3]) {
		t.Fatalf("error = %v, want fourth error", err)
	}
	if script.Calls() != retryAttempts {
		t.Fatalf("calls = %d, want %d", script.Calls(), retryAttempts)
	}
}

func TestRetryTransportReturnsLastResponseAtAttemptLimit(t *testing.T) {
	steps := make([]httpfixture.Step, retryAttempts)
	for i := range steps {
		steps[i] = httpfixture.Step{Response: retryResponse(http.StatusServiceUnavailable, fmt.Sprint(i))}
	}
	script := httpfixture.NewScriptedTransport(steps...)
	retry, _ := deterministicRetry(script)
	req, _ := http.NewRequest(http.MethodGet, "http://service/", nil)
	resp, err := retry.RoundTrip(req)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if string(body) != "3" || script.Calls() != retryAttempts {
		t.Fatalf("body=%q calls=%d", body, script.Calls())
	}
}

func TestRetryAfter(t *testing.T) {
	base := time.Now().UTC().Truncate(time.Second)
	tests := []struct {
		name      string
		value     string
		wantWait  time.Duration
		wantCalls int
	}{
		{name: "seconds", value: "1", wantWait: time.Second, wantCalls: 2},
		{name: "at cap", value: "2", wantWait: 2 * time.Second, wantCalls: 2},
		{name: "http date", value: base.Add(time.Second).Format(http.TimeFormat), wantWait: time.Second, wantCalls: 2},
		{name: "invalid", value: "soon", wantWait: 200 * time.Millisecond, wantCalls: 2},
		{name: "past", value: base.Add(-time.Second).Format(http.TimeFormat), wantWait: 200 * time.Millisecond, wantCalls: 2},
		{name: "over cap", value: "3", wantCalls: 1},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			first := retryResponse(http.StatusTooManyRequests, "busy")
			first.Header.Set("Retry-After", tc.value)
			script := httpfixture.NewScriptedTransport(
				httpfixture.Step{Response: first},
				httpfixture.Step{Response: retryResponse(http.StatusOK, "ok")},
			)
			retry, record := deterministicRetry(script)
			retry.now = func() time.Time { return base }
			req, _ := http.NewRequest(http.MethodGet, "http://service/", nil)
			resp, err := retry.RoundTrip(req)
			if err != nil {
				t.Fatal(err)
			}
			body, readErr := io.ReadAll(resp.Body)
			_ = resp.Body.Close()
			if readErr != nil {
				t.Fatal(readErr)
			}
			if script.Calls() != tc.wantCalls {
				t.Fatalf("calls = %d, want %d", script.Calls(), tc.wantCalls)
			}
			if tc.wantCalls == 1 && string(body) != "busy" {
				t.Fatalf("over-cap response body = %q, want untouched busy", body)
			}
			if tc.wantCalls == 2 && (len(record.waits) != 1 || record.waits[0] != tc.wantWait) {
				t.Fatalf("waits = %v, want [%v]", record.waits, tc.wantWait)
			}
		})
	}
}

func TestRetryAfterBeyondDeadlineReturnsUntouchedResponse(t *testing.T) {
	base := time.Now()
	ctx, cancel := context.WithDeadline(context.Background(), base.Add(5*time.Second))
	defer cancel()

	first := retryResponse(http.StatusTooManyRequests, "still readable")
	first.Header.Set("Retry-After", "1")
	script := httpfixture.NewScriptedTransport(
		httpfixture.Step{Response: first},
		httpfixture.Step{Response: retryResponse(http.StatusOK, "must not happen")},
	)
	retry, record := deterministicRetry(script)
	retry.now = func() time.Time { return base.Add(4500 * time.Millisecond) }
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, "http://service/", nil)
	resp, err := retry.RoundTrip(req)
	if err != nil {
		t.Fatal(err)
	}
	body, readErr := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if readErr != nil || string(body) != "still readable" {
		t.Fatalf("returned body=%q err=%v", body, readErr)
	}
	if script.Calls() != 1 || len(record.waits) != 0 || len(record.reasons) != 0 {
		t.Fatalf("calls=%d waits=%v reasons=%v", script.Calls(), record.waits, record.reasons)
	}
}

func TestRetryTransportCancellationDuringWait(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	script := httpfixture.NewScriptedTransport(
		httpfixture.Step{Response: retryResponse(http.StatusServiceUnavailable, "busy")},
		httpfixture.Step{Response: retryResponse(http.StatusOK, "must not happen")},
	)
	retry, record := deterministicRetry(script)
	retry.wait = func(context.Context, time.Duration) error {
		cancel()
		return ctx.Err()
	}
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, "http://service/", nil)
	if _, err := retry.RoundTrip(req); !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v, want canceled", err)
	}
	if script.Calls() != 1 || len(record.reasons) != 0 {
		t.Fatalf("calls=%d reasons=%v", script.Calls(), record.reasons)
	}
}

func TestRetryTransportAlreadyCanceledMakesNoAttempt(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	calls := 0
	retry, _ := deterministicRetry(httpfixture.RoundTripperFunc(func(*http.Request) (*http.Response, error) {
		calls++
		return retryResponse(http.StatusOK, "must not happen"), nil
	}))
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, "http://service/", nil)
	if _, err := retry.RoundTrip(req); !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v, want canceled", err)
	}
	if calls != 0 {
		t.Fatalf("calls = %d, want zero", calls)
	}
}

func TestRetryTransportReplaysGETBody(t *testing.T) {
	script := httpfixture.NewScriptedTransport(
		httpfixture.Step{Response: retryResponse(http.StatusServiceUnavailable, "busy")},
		httpfixture.Step{Response: retryResponse(http.StatusOK, "ok")},
	)
	retry, _ := deterministicRetry(script)
	req, _ := http.NewRequest(http.MethodGet, "http://service/", bytes.NewBufferString("query"))
	resp, err := retry.RoundTrip(req)
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	requests := script.Requests()
	if len(requests) != 2 || string(requests[0].Body) != "query" || string(requests[1].Body) != "query" {
		t.Fatalf("requests = %#v", requests)
	}
}

func TestRetryTransportDoesNotReplayUnrepeatableGETBody(t *testing.T) {
	script := httpfixture.NewScriptedTransport(httpfixture.Step{
		Response: retryResponse(http.StatusServiceUnavailable, "kept"),
	})
	retry, _ := deterministicRetry(script)
	req, _ := http.NewRequest(http.MethodGet, "http://service/", io.NopCloser(strings.NewReader("query")))
	resp, err := retry.RoundTrip(req)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if script.Calls() != 1 || string(body) != "kept" {
		t.Fatalf("calls=%d body=%q", script.Calls(), body)
	}
}

func TestRetryTransportClosesResponseReturnedWithError(t *testing.T) {
	badBody := &trackingBody{Reader: strings.NewReader("discard")}
	script := httpfixture.NewScriptedTransport(
		httpfixture.Step{
			Response: &http.Response{StatusCode: http.StatusServiceUnavailable, Body: badBody},
			Err:      errors.New("broken transport"),
		},
		httpfixture.Step{Response: retryResponse(http.StatusOK, "ok")},
	)
	retry, _ := deterministicRetry(script)
	req, _ := http.NewRequest(http.MethodGet, "http://service/", nil)
	resp, err := retry.RoundTrip(req)
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if !badBody.closed {
		t.Fatal("response body returned alongside an error was not closed")
	}
}

func TestRetryTransportBoundedDrainsAndClosesDiscardedResponse(t *testing.T) {
	discarded := &trackingBody{Reader: strings.NewReader(strings.Repeat("x", retryDrainLimit*2))}
	script := httpfixture.NewScriptedTransport(
		httpfixture.Step{Response: &http.Response{
			StatusCode: http.StatusServiceUnavailable,
			Header:     make(http.Header),
			Body:       discarded,
		}},
		httpfixture.Step{Response: retryResponse(http.StatusOK, "ok")},
	)
	retry, _ := deterministicRetry(script)
	req, _ := http.NewRequest(http.MethodGet, "http://service/", nil)
	resp, err := retry.RoundTrip(req)
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if !discarded.closed {
		t.Fatal("discarded body was not closed")
	}
	if discarded.read != retryDrainLimit {
		t.Fatalf("discarded bytes = %d, want bounded %d", discarded.read, retryDrainLimit)
	}
}

func TestRetryTransportIsSafeForConcurrentRequests(t *testing.T) {
	var mu sync.Mutex
	calls := map[string]int{}
	next := httpfixture.RoundTripperFunc(func(req *http.Request) (*http.Response, error) {
		mu.Lock()
		calls[req.URL.Path]++
		attempt := calls[req.URL.Path]
		mu.Unlock()
		if attempt == 1 {
			return retryResponse(http.StatusServiceUnavailable, "busy"), nil
		}
		return retryResponse(http.StatusOK, "ok"), nil
	})
	retry := newRetryTransport(next, nil)
	retry.jitter = func(time.Duration) time.Duration { return 0 }
	retry.wait = func(context.Context, time.Duration) error { return nil }

	const requests = 20
	var wg sync.WaitGroup
	errs := make(chan error, requests)
	for i := range requests {
		wg.Add(1)
		go func() {
			defer wg.Done()
			req, _ := http.NewRequest(http.MethodGet, fmt.Sprintf("http://service/%d", i), nil)
			resp, err := retry.RoundTrip(req)
			if err == nil {
				_ = resp.Body.Close()
			}
			errs <- err
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	mu.Lock()
	defer mu.Unlock()
	for path, count := range calls {
		if count != 2 {
			t.Fatalf("path %s calls = %d, want 2", path, count)
		}
	}
}

func TestInstrumentedRetryCountsOneLogicalRequest(t *testing.T) {
	const target = "httpx-retry-composition-test"
	script := httpfixture.NewScriptedTransport(
		httpfixture.Step{Response: retryResponse(http.StatusServiceUnavailable, "busy")},
		httpfixture.Step{Response: retryResponse(http.StatusOK, "ok")},
	)
	retry, _ := deterministicRetry(script)
	recorder := metrics.New(metrics.Options{})
	instrumented := recorder.InstrumentTransport(target, retry)
	req, _ := http.NewRequest(http.MethodGet, "http://service/", nil)
	resp, err := instrumented.RoundTrip(req)
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if script.Calls() != 2 {
		t.Fatalf("attempts = %d, want 2", script.Calls())
	}
	scrape := httptest.NewRecorder()
	recorder.Handler().ServeHTTP(scrape, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	body := scrape.Body.String()
	if !strings.Contains(body, `loomarr_outbound_requests_total{code="200",target="other"} 1`) {
		t.Fatalf("logical 200 request missing from scrape:\n%s", body)
	}
	if strings.Contains(body, `loomarr_outbound_requests_total{code="503",target="other"}`) {
		t.Fatalf("intermediate 503 was counted as a logical request:\n%s", body)
	}
}

func TestRetryTransportDoesNotRetryMidBodyFailure(t *testing.T) {
	calls := 0
	next := httpfixture.RoundTripperFunc(func(*http.Request) (*http.Response, error) {
		calls++
		return &http.Response{StatusCode: http.StatusOK, Body: failingBody{}}, nil
	})
	retry, _ := deterministicRetry(next)
	req, _ := http.NewRequest(http.MethodGet, "http://service/", nil)
	resp, err := retry.RoundTrip(req)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := io.ReadAll(resp.Body); err == nil {
		t.Fatal("expected body error")
	}
	_ = resp.Body.Close()
	if calls != 1 {
		t.Fatalf("calls = %d, want 1", calls)
	}
}

type failingBody struct{}

func (failingBody) Read([]byte) (int, error) { return 0, errors.New("body failed") }
func (failingBody) Close() error             { return nil }

type trackingBody struct {
	io.Reader
	closed bool
	read   int
}

func (b *trackingBody) Read(p []byte) (int, error) {
	n, err := b.Reader.Read(p)
	b.read += n
	return n, err
}

func (b *trackingBody) Close() error {
	b.closed = true
	return nil
}
