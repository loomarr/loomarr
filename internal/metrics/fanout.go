package metrics

import (
	"context"
	"net/http"
	"sync/atomic"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// Per-request outbound FAN-OUT: how many downstream calls did serving this one request make?
//
// # Why this exists, when outbound_requests_total already counts calls
//
// That counter is process-global. It answers "how many library calls has this process made",
// which is the right shape for alerting and useless for attribution: under any concurrency you
// cannot tell whether 200 library calls were one request making 200 or 200 requests making one.
// The only way to read it per-request is to serialise traffic and diff the counter around a
// single call — which works exactly once, by hand, on a dev box, and is how the N+1 behind
// GET /v1/guide's cold path was eventually found (25 calls for 25 movies).
//
// This records the same thing as a HISTOGRAM KEYED BY ROUTE, so the question becomes a query
// rather than an investigation: `loomarr_http_outbound_fanout_bucket{route="/v1/guide"}` shows
// the distribution directly, and a p99 far above the median is an N+1 that only fires on some
// inputs — precisely the shape that hides from a mean.
//
// # Why a histogram and not a gauge or a log line
//
// Fan-out is per-request and long-tailed: the interesting case is the 99th-percentile request
// that made 25 calls while the median made 1. A gauge would only hold the last value, and a log
// line would need someone to already suspect the endpoint. A histogram makes "which endpoint has
// the worst fan-out" answerable without knowing the answer first.
//
// # Why a CPU profile cannot replace this
//
// An N+1 against a remote service is I/O: the goroutine is parked on a socket and produces no
// CPU samples at all. A CPU profile of the request that spent 375ms on 25 sequential HTTP calls
// showed those 375ms nowhere — it attributed the visible time to the scheduler's arrangement,
// which was genuinely 53% of the CPU and 60µs of the wall clock. Fan-out is the signal that
// distinguishes "this endpoint does too much work" from "this endpoint does the same small work
// too many times", and those have completely different fixes.

// outboundFanout is the count of downstream HTTP calls made while serving one inbound request.
//
// Buckets are powers of two rather than a linear spread: the meaningful distinction is 1 vs 2 vs
// "tens", not 12 vs 13. The top bucket is deliberately reachable — a route that lands in it is
// the N+1 this metric exists to surface.
var outboundFanout = promauto.NewHistogramVec(prometheus.HistogramOpts{
	Namespace: "loomarr", Subsystem: "http", Name: "outbound_fanout",
	Help:    "Downstream HTTP calls made while serving one inbound request, by route.",
	Buckets: []float64{0, 1, 2, 4, 8, 16, 32, 64, 128},
}, []string{"method", "route"})

// fanoutKey is the context key carrying the current request's outbound counter.
//
// Unexported and of a private type, so nothing outside this package can collide with it or
// substitute a counter — the value is written by the transport and read by the middleware, and
// neither is a public contract.
type fanoutKeyType struct{}

var fanoutKey fanoutKeyType

// withFanoutCounter attaches a fresh counter to the request context.
//
// The counter is a POINTER shared with every outbound call the request makes, including those on
// goroutines it spawns: GET /v1/guide resolves its channels concurrently, so a per-goroutine
// counter would attribute each channel's calls to nothing. atomic because those goroutines
// increment it in parallel.
func withFanoutCounter(ctx context.Context) (context.Context, *atomic.Int64) {
	n := &atomic.Int64{}
	return context.WithValue(ctx, fanoutKey, n), n
}

// countOutbound increments the calling request's fan-out counter, if there is one.
//
// A request with no counter (a background job, a test calling an adapter directly) is silently
// ignored rather than an error: outbound calls legitimately happen outside an inbound request,
// and instrumentation must never be the thing that breaks them.
func countOutbound(ctx context.Context) {
	if n, ok := ctx.Value(fanoutKey).(*atomic.Int64); ok {
		n.Add(1)
	}
}

// FanoutMiddleware attaches a per-request outbound counter and records the total on completion.
//
// Separate from Middleware so the ordering is explicit at the call site: this must be OUTSIDE
// any handler that makes outbound calls, and the context it installs has to reach them — which
// is why every adapter must pass the request's ctx down rather than context.Background(). An
// adapter that drops the context silently reports zero fan-out, which reads as "this endpoint
// makes no downstream calls" rather than as a missing wire.
// ⚠ The route label must be read off the request the MUX SAW, not the one handed to this
// closure. r.WithContext returns a COPY, and ServeMux populates Pattern on the copy it routes —
// so labelling from the outer `r` reports every route as "other", which looks like working
// instrumentation until you read the output. Middleware above gets away with reading its own `r`
// precisely because it does not copy the request. Same trap, opposite conclusion.
func FanoutMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx, n := withFanoutCounter(r.Context())
		routed := r.WithContext(ctx)
		next.ServeHTTP(w, routed)
		outboundFanout.WithLabelValues(routed.Method, routeLabel(routed.Pattern)).Observe(float64(n.Load()))
	})
}
