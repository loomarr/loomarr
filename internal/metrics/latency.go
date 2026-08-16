package metrics

import (
	"net/http"
	"strconv"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// The latency tranche of §17: client-side RED for every outbound call (Tunarr,
// library, LLM, Seerr, TMDB) via one instrumented transport, plus the
// reconcile-loop timing + channel-reconcile counter. Both share the "wrap the
// boundary" pattern, distinct from the state-gauge/event-counter tranche.
//
// Still deferred after this (§17): LLM token/cost (needs the provider to surface
// usage), filler pod fallback-ladder depth, and slot-drift substitutions — each
// a domain-specific counter rather than a latency, a later tranche.

var (
	// outboundRequests counts outbound HTTP calls by target service and status.
	// A transport-level failure (no response) records code="error" — that's the
	// §17 "Tunarr API errors" signal, generalised to every dependency.
	outboundRequests = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: "loomarr", Subsystem: "outbound", Name: "requests_total",
		Help: "Outbound HTTP requests by target service and status code (error = no response).",
	}, []string{"target", "code"})

	// outboundDuration is the client-side latency histogram per target — the
	// §17 library-lookup / Tunarr-API / LLM latency, in one series filtered by
	// target. Buckets span a fast LAN library lookup to a 120s LLM generation.
	outboundDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Namespace: "loomarr", Subsystem: "outbound", Name: "request_duration_seconds",
		Help:    "Outbound HTTP client latency in seconds, by target service.",
		Buckets: []float64{0.01, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10, 30, 60, 120},
	}, []string{"target"})

	// outboundRetries counts actual additional attempts made by the shared
	// outbound HTTP client. The reason label is deliberately a closed set; do
	// not put raw errors or arbitrary status codes into it.
	outboundRetries = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: "loomarr", Subsystem: "outbound", Name: "retries_total",
		Help: "Outbound HTTP retry attempts by target service and bounded reason.",
	}, []string{"target", "reason"})

	// channelReconciles counts channel reconciles by outcome (§17).
	channelReconciles = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: "loomarr", Subsystem: "channel", Name: "reconciles_total",
		Help: "Channel reconciles by result.",
	}, []string{"result"})

	// reconcileDuration is the per-reconcile wall-clock (§17 reconcile-loop
	// latency): recompute + drift check + the Tunarr diff/push for one channel.
	reconcileDuration = promauto.NewHistogram(prometheus.HistogramOpts{
		Namespace: "loomarr", Subsystem: "channel", Name: "reconcile_duration_seconds",
		Help:    "Wall-clock of a single channel reconcile in seconds.",
		Buckets: []float64{0.05, 0.1, 0.5, 1, 2.5, 5, 10, 30, 60},
	})
)

// OutboundRetryReason is the bounded reason for an actual additional outbound
// HTTP attempt. It is an enum rather than a string so callers cannot turn a
// transport error or unexpected response into an unbounded Prometheus label.
type OutboundRetryReason uint8

const (
	OutboundRetryTransport OutboundRetryReason = iota
	OutboundRetryStatus408
	OutboundRetryStatus429
	OutboundRetryStatus500
	OutboundRetryStatus502
	OutboundRetryStatus503
	OutboundRetryStatus504
)

var outboundRetryReasonLabels = [...]string{
	"transport",
	"408",
	"429",
	"500",
	"502",
	"503",
	"504",
}

// InstrumentTransport wraps next (nil ⇒ http.DefaultTransport) so every request
// it carries records latency + result under the given target label. Used by
// httpx.NewNamed so instrumentation lives in one place, not per adapter.
func InstrumentTransport(target string, next http.RoundTripper) http.RoundTripper {
	if next == nil {
		next = http.DefaultTransport
	}
	return &instrumentedTransport{target: target, next: next}
}

// OutboundRetried records one actual additional attempt made for target. The
// shared HTTP factory calls this after a retry wait completes, so a retry that
// is considered but abandoned due to cancellation or deadline pressure is not
// counted. Unknown enum values stay in one bounded diagnostic bucket.
func OutboundRetried(target string, reason OutboundRetryReason) {
	label := "unknown"
	if int(reason) < len(outboundRetryReasonLabels) {
		label = outboundRetryReasonLabels[reason]
	}
	outboundRetries.WithLabelValues(target, label).Inc()
}

type instrumentedTransport struct {
	target string
	next   http.RoundTripper
}

func (t *instrumentedTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	// Attribute this call to the inbound request that caused it, if any (fanout.go). Done here
	// rather than per-adapter for the same reason the counters below are: one boundary, so a new
	// adapter is instrumented by construction instead of by remembering.
	countOutbound(req.Context())

	start := time.Now()
	resp, err := t.next.RoundTrip(req)
	outboundDuration.WithLabelValues(t.target).Observe(time.Since(start).Seconds())

	code := "error"
	if err == nil {
		code = strconv.Itoa(resp.StatusCode)
	}
	outboundRequests.WithLabelValues(t.target, code).Inc()
	return resp, err
}

// ReconcileObserved records one channel reconcile's duration and outcome (§17).
func ReconcileObserved(d time.Duration, ok bool) {
	result := "error"
	if ok {
		result = "success"
	}
	channelReconciles.WithLabelValues(result).Inc()
	reconcileDuration.Observe(d.Seconds())
}
