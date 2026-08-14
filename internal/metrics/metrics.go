// Package metrics is Loomarr's Prometheus surface (design §7 /metrics, §18).
//
// Scope note (honest, per AGENTS.md "no silent caps"): this package currently
// exports the RED basics — HTTP request rate, errors, and duration — plus the
// Go runtime + process collectors client_golang registers by default
// (goroutines, GC, heap, open fds, CPU). That is the RED + USE foundation a
// dashboard needs first. The §18 *domain* series (records by state, reconcile
// latency, Tunarr API latency/errors, LLM latency/tokens, filler pod-ladder
// depth, logins, active sessions, job-queue depth, janitor purges) are NOT yet
// wired; several need store count-by-state methods (and thus a change to the
// one-suite-two-backends conformance gate), so they land as follow-up
// instrumentation. docs/help/runbook records exactly what is and isn't exported.
//
// The metric vars are package-level singletons registered once at init via
// promauto, so Router() can be built many times in a process (tests) without a
// duplicate-registration panic.
package metrics

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var (
	// httpRequests counts inbound HTTP requests by method, matched route
	// pattern, and status class. The route label is the Go 1.22 ServeMux
	// pattern (e.g. "/v1/channels/{id}"), never the raw path, so a wildcard
	// segment can't explode label cardinality.
	httpRequests = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: "loomarr", Subsystem: "http", Name: "requests_total",
		Help: "Inbound HTTP requests by method, matched route, and status code.",
	}, []string{"method", "route", "code"})

	// httpDuration is the server-side handling latency histogram, labelled the
	// same way minus code (code lives on the counter). Default buckets span the
	// sub-millisecond LAN calls up to the multi-second LLM proposal path.
	httpDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Namespace: "loomarr", Subsystem: "http", Name: "request_duration_seconds",
		Help:    "Server-side HTTP handling latency in seconds, by method and matched route.",
		Buckets: []float64{0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10, 30},
	}, []string{"method", "route"})

	// httpInFlight is the count of requests currently being served — the
	// saturation signal that pairs with the runtime goroutine gauge.
	httpInFlight = promauto.NewGauge(prometheus.GaugeOpts{
		Namespace: "loomarr", Subsystem: "http", Name: "requests_in_flight",
		Help: "HTTP requests currently being served.",
	})
)

// Handler serves the Prometheus text exposition format from the default
// registry (which carries the Go + process collectors). Mounted unauthenticated
// at /metrics on the LAN (§7).
func Handler() http.Handler { return promhttp.Handler() }

// Middleware records request count, duration, and in-flight for every request.
// It must sit OUTSIDE the router so it observes total handling time, but it
// reads r.Pattern *after* next.ServeHTTP has run — that is when the ServeMux has
// populated the matched pattern on the same request pointer.
func Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		httpInFlight.Inc()
		defer httpInFlight.Dec()

		rec := &statusRecorder{ResponseWriter: w, code: http.StatusOK}
		start := time.Now()
		next.ServeHTTP(rec, r)
		elapsed := time.Since(start).Seconds()

		route := routeLabel(r.Pattern)
		httpDuration.WithLabelValues(r.Method, route).Observe(elapsed)
		httpRequests.WithLabelValues(r.Method, route, statusCode(rec.code)).Inc()
	})
}

// routeLabel normalises a ServeMux pattern into a low-cardinality label. A
// Go 1.22 pattern is "[METHOD ][HOST]/path"; we keep only the path so the label
// doesn't duplicate the method (already its own label) and stays bounded by the
// registered route set. Unmatched requests (no pattern) bucket as "other".
func routeLabel(pattern string) string {
	if pattern == "" {
		return "other"
	}
	// Drop a leading "METHOD " token if present.
	if i := strings.IndexByte(pattern, ' '); i >= 0 {
		pattern = pattern[i+1:]
	}
	if pattern == "" {
		return "other"
	}
	return pattern
}

// statusCode renders the numeric status as its own string. Kept as a helper so
// the label is always the exact code (Grafana groups to 2xx/4xx/5xx cheaply).
func statusCode(code int) string {
	if code == 0 {
		code = http.StatusOK
	}
	return strconv.Itoa(code)
}

// statusRecorder captures the response status without swallowing the streaming
// interfaces the SSE handler relies on. It forwards Flush so w.(http.Flusher)
// still succeeds through the wrapper, and exposes Unwrap for
// http.ResponseController.
type statusRecorder struct {
	http.ResponseWriter
	code    int
	written bool
}

func (s *statusRecorder) WriteHeader(code int) {
	if !s.written {
		s.code = code
		s.written = true
	}
	s.ResponseWriter.WriteHeader(code)
}

func (s *statusRecorder) Write(b []byte) (int, error) {
	if !s.written {
		s.written = true // an implicit 200 on first write
	}
	return s.ResponseWriter.Write(b)
}

// Flush forwards to the underlying writer when it supports flushing, so SSE
// (§7 /v1/events) keeps streaming through this wrapper.
func (s *statusRecorder) Flush() {
	if f, ok := s.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

// Unwrap lets http.ResponseController reach the underlying writer for any
// interface (Hijacker, etc.) this wrapper doesn't forward explicitly.
func (s *statusRecorder) Unwrap() http.ResponseWriter { return s.ResponseWriter }
