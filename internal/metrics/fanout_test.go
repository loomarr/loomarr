package metrics

import (
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
)

// rtCount is a RoundTripper that reports how many calls it carried, so a test can assert on the
// COUNTER rather than on the histogram (which Prometheus does not expose per-observation).
//
// atomic because one of these tests drives it from several goroutines at once — the same reason
// the production counter is atomic, and the -race detector catches a plain int here immediately.
type rtCount struct{ n atomic.Int64 }

func (r *rtCount) RoundTrip(*http.Request) (*http.Response, error) {
	r.n.Add(1)
	return &http.Response{StatusCode: 200, Body: http.NoBody}, nil
}

// fanoutOf drives one request through the middleware and returns what the counter saw.
//
// It reads the counter directly instead of scraping /metrics: the histogram is the production
// surface, but a test that parses exposition text asserts on Prometheus's formatting as much as
// on our logic.
func fanoutOf(t *testing.T, handler func(*http.Request)) int64 {
	t.Helper()
	var got int64
	h := FanoutMiddleware(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		handler(r)
		if n, ok := r.Context().Value(fanoutKey).(interface{ Load() int64 }); ok {
			got = n.Load()
		}
	}))
	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/v1/guide", nil))
	return got
}

// THE GAP THIS CLOSES: the process-global outbound counter cannot attribute calls to a request.
// Finding the guide's N+1 meant serialising traffic and diffing that counter by hand — a
// technique that works once, on a dev box, and not at all under concurrency.
func TestFanout_CountsThisRequestsOutboundCalls(t *testing.T) {
	rt := &rtCount{}
	client := &http.Client{Transport: InstrumentTransport("library", rt)}

	got := fanoutOf(t, func(r *http.Request) {
		// Twenty-five calls, as a 25-movie channel's cycle layout made.
		for range 25 {
			req, _ := http.NewRequestWithContext(r.Context(), http.MethodGet, "http://x/Items", nil)
			resp, err := client.Do(req)
			if err != nil {
				t.Errorf("outbound: %v", err)
				return
			}
			_ = resp.Body.Close()
		}
	})

	if got != 25 {
		t.Fatalf("fan-out = %d, want 25 (the request's own calls are not being attributed)", got)
	}
	if got := rt.n.Load(); got != 25 {
		t.Fatalf("transport carried %d calls, want 25", got)
	}
}

// Calls made on goroutines the handler SPAWNS must count too. GET /v1/guide resolves its channels
// concurrently, so a counter that only saw the handler's own goroutine would miss the very
// fan-out it exists to measure — and would report a comfortable 0 for the worst endpoint.
func TestFanout_CountsCallsFromSpawnedGoroutines(t *testing.T) {
	rt := &rtCount{}
	client := &http.Client{Transport: InstrumentTransport("library", rt)}

	got := fanoutOf(t, func(r *http.Request) {
		var wg sync.WaitGroup
		for range 4 { // four channels, as the dev install has
			wg.Add(1)
			go func() {
				defer wg.Done()
				for range 3 {
					req, _ := http.NewRequestWithContext(r.Context(), http.MethodGet, "http://x/Items", nil)
					resp, err := client.Do(req)
					if err != nil {
						return
					}
					_ = resp.Body.Close()
				}
			}()
		}
		wg.Wait()
	})

	if got != 12 {
		t.Fatalf("fan-out = %d, want 12 (concurrent channel resolves are not being attributed)", got)
	}
}

// An outbound call with no inbound request — a background job, a scheduler sweep — must not
// panic or be attributed to whatever request happens to be in flight. Instrumentation is never
// allowed to be the thing that breaks a working call path.
func TestFanout_OutboundOutsideARequestIsIgnored(t *testing.T) {
	rt := &rtCount{}
	client := &http.Client{Transport: InstrumentTransport("library", rt)}

	req, _ := http.NewRequest(http.MethodGet, "http://x/Items", nil)
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("an outbound call outside a request must still work: %v", err)
	}
	_ = resp.Body.Close()

	if got := rt.n.Load(); got != 1 {
		t.Fatalf("transport carried %d calls, want 1", got)
	}
}

// A request that makes NO outbound calls records zero rather than nothing. The distinction
// matters: an endpoint absent from the histogram is indistinguishable from one that was never
// exercised, and "this route makes no downstream calls" is a real, useful answer.
func TestFanout_ZeroIsRecordedNotSkipped(t *testing.T) {
	if got := fanoutOf(t, func(*http.Request) {}); got != 0 {
		t.Fatalf("fan-out = %d, want 0", got)
	}
}

// THE GAP THIS CLOSES: r.WithContext returns a COPY, and ServeMux populates Pattern on whichever
// request it actually routed. Labelling from the outer request reports every route as "other" —
// instrumentation that exports cleanly, scrapes cleanly, and answers no question at all. Caught
// only by looking at the output, so it is pinned here instead.
func TestFanout_LabelsTheMatchedRouteNotOther(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /v1/guide", func(http.ResponseWriter, *http.Request) {})

	var seen string
	h := FanoutMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mux.ServeHTTP(w, r)
		seen = routeLabel(r.Pattern) // the request the mux routed
	}))
	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/v1/guide", nil))

	if seen != "/v1/guide" {
		t.Fatalf("route label = %q, want %q (Pattern is being read off the wrong request)", seen, "/v1/guide")
	}
}
