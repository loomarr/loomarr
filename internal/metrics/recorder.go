package metrics

import (
	"database/sql"
	"net/http"
	"strconv"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/loomarr/loomarr/internal/images/rustgen"
)

// Options identifies one application generation to its metrics Recorder.
// Store, pool, and clock inputs join this configuration at their vertical slices;
// callers do not receive the underlying Prometheus registry.
type Options struct {
	Version       string
	Revision      string
	Database      string
	Store         StoreCounts
	Now           func() time.Time
	DatabaseStats func() sql.DBStats
}

// Recorder owns the complete Prometheus surface for one application generation.
// Its registry is deliberately private: semantic methods below are the only way
// application code can create or label a series.
type Recorder struct {
	registry   *prometheus.Registry
	handler    http.Handler
	authLogins *prometheus.CounterVec
	http       recorderHTTP
	images     recorderImages
	outbound   recorderOutbound
}

type recorderHTTP struct {
	requests *prometheus.CounterVec
	duration *prometheus.HistogramVec
	inFlight prometheus.Gauge
	fanout   *prometheus.HistogramVec
}

type recorderImages struct {
	operations  *prometheus.CounterVec
	duration    *prometheus.HistogramVec
	inputBytes  *prometheus.HistogramVec
	outputBytes *prometheus.HistogramVec
	peakRSS     *prometheus.HistogramVec
	queueWait   *prometheus.HistogramVec
	inFlight    prometheus.Gauge
}

type recorderOutbound struct {
	requests *prometheus.CounterVec
	duration *prometheus.HistogramVec
	retries  *prometheus.CounterVec
}

// New constructs an isolated generation registry with Loomarr identity and the
// standard Go/process collectors. Building another Recorder cannot retain or
// mutate any series in this one.
func New(options Options) *Recorder {
	registry := prometheus.NewRegistry()
	registry.MustRegister(
		collectors.NewGoCollector(),
		collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}),
	)

	build := prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: "loomarr",
		Subsystem: "build",
		Name:      "info",
		Help:      "Loomarr build and database backend information.",
	}, []string{"version", "revision", "database"})
	build.WithLabelValues(options.Version, options.Revision, options.Database).Set(1)

	authLogins := prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: "loomarr",
		Subsystem: "auth",
		Name:      "logins_total",
		Help:      "Login attempts by result.",
	}, []string{"result"})
	for _, result := range []string{"success", "failure"} {
		authLogins.WithLabelValues(result)
	}
	storeErrors := prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: "loomarr", Subsystem: "metrics", Name: "scrape_errors_total",
		Help: "Store queries that failed during a /metrics scrape, by source.",
	}, []string{"source"})
	for _, source := range []string{
		"titles", "jobs", "sessions", "proposal_job_age",
		"proposal_job_attempts", "proposal_job_failures",
	} {
		storeErrors.WithLabelValues(source)
	}

	httpMetrics := recorderHTTP{
		requests: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: "loomarr", Subsystem: "http", Name: "requests_total",
			Help: "Inbound HTTP requests by method, matched route, and status code.",
		}, []string{"method", "route", "code"}),
		duration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Namespace: "loomarr", Subsystem: "http", Name: "request_duration_seconds",
			Help:    "Server-side HTTP handling latency in seconds, by method and matched route.",
			Buckets: []float64{0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10, 30},
		}, []string{"method", "route"}),
		inFlight: prometheus.NewGauge(prometheus.GaugeOpts{
			Namespace: "loomarr", Subsystem: "http", Name: "requests_in_flight",
			Help: "HTTP requests currently being served.",
		}),
		fanout: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Namespace: "loomarr", Subsystem: "http", Name: "outbound_fanout",
			Help:    "Downstream HTTP calls made while serving one inbound request, by route.",
			Buckets: []float64{0, 1, 2, 4, 8, 16, 32, 64, 128},
		}, []string{"method", "route"}),
	}
	imageMetrics := recorderImages{
		operations: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: "loomarr", Subsystem: "image_worker", Name: "operations_total",
			Help: "Rust image worker invocations by operation kind and stable result class.",
		}, []string{"kind", "result"}),
		duration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Namespace: "loomarr", Subsystem: "image_worker", Name: "duration_seconds",
			Help:    "Wall-clock duration of one Rust image worker process.",
			Buckets: []float64{0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10, 30},
		}, []string{"kind"}),
		inputBytes: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Namespace: "loomarr", Subsystem: "image_worker", Name: "input_bytes",
			Help:    "Compressed source bytes read by one Rust image worker process.",
			Buckets: prometheus.ExponentialBuckets(1024, 4, 9),
		}, []string{"kind"}),
		outputBytes: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Namespace: "loomarr", Subsystem: "image_worker", Name: "output_bytes",
			Help:    "Encoded output bytes returned by one Rust image worker process.",
			Buckets: prometheus.ExponentialBuckets(1024, 4, 9),
		}, []string{"kind"}),
		peakRSS: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Namespace: "loomarr", Subsystem: "image_worker", Name: "peak_rss_bytes",
			Help:    "Peak resident bytes of one Rust image worker process where the host reports it.",
			Buckets: prometheus.ExponentialBuckets(16<<20, 2, 7),
		}, []string{"kind"}),
		queueWait: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Namespace: "loomarr", Subsystem: "image_worker", Name: "queue_wait_seconds",
			Help:    "Time spent waiting for image worker capacity by admission class.",
			Buckets: []float64{0.0001, 0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 1, 5, 30},
		}, []string{"class"}),
		inFlight: prometheus.NewGauge(prometheus.GaugeOpts{
			Namespace: "loomarr", Subsystem: "image_worker", Name: "in_flight",
			Help: "Rust image worker processes currently holding a global image slot.",
		}),
	}
	outboundMetrics := recorderOutbound{
		requests: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: "loomarr", Subsystem: "outbound", Name: "requests_total",
			Help: "Outbound HTTP requests by target service and status code (error = no response).",
		}, []string{"target", "code"}),
		duration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Namespace: "loomarr", Subsystem: "outbound", Name: "request_duration_seconds",
			Help:    "Outbound HTTP client latency in seconds, by target service.",
			Buckets: []float64{0.01, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10, 30, 60, 120},
		}, []string{"target"}),
		retries: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: "loomarr", Subsystem: "outbound", Name: "retries_total",
			Help: "Outbound HTTP retry attempts by target service and bounded reason.",
		}, []string{"target", "reason"}),
	}
	for _, target := range []string{
		"tunarr", "tmdb", "library", "seerr", "arr", "llm",
		"filler_review", "filler_bakeoff", "other",
	} {
		for _, reason := range []string{"transport", "408", "429", "500", "502", "503", "504", "other"} {
			outboundMetrics.retries.WithLabelValues(target, reason)
		}
	}
	for _, kind := range []string{"inspect", "render", "other"} {
		for _, result := range []string{"success", "refused", "canceled", "process_error", "protocol_error", "other"} {
			imageMetrics.operations.WithLabelValues(kind, result)
		}
	}
	registry.MustRegister(build, authLogins, storeErrors, httpMetrics.requests, httpMetrics.duration,
		httpMetrics.inFlight, httpMetrics.fanout, imageMetrics.operations, imageMetrics.duration,
		imageMetrics.inputBytes, imageMetrics.outputBytes, imageMetrics.peakRSS,
		imageMetrics.queueWait, imageMetrics.inFlight, outboundMetrics.requests,
		outboundMetrics.duration, outboundMetrics.retries)
	if options.Store != nil {
		now := options.Now
		if now == nil {
			now = time.Now
		}
		registry.MustRegister(newStoreCollectorWithErrors(options.Store, now, storeErrors))
	}
	if options.DatabaseStats != nil {
		registry.MustRegister(newDatabaseCollector(options.DatabaseStats))
	}

	return &Recorder{
		registry:   registry,
		handler:    promhttp.HandlerFor(registry, promhttp.HandlerOpts{}),
		authLogins: authLogins,
		http:       httpMetrics,
		images:     imageMetrics,
		outbound:   outboundMetrics,
	}
}

// Handler serves this generation's Prometheus text exposition.
func (r *Recorder) Handler() http.Handler { return r.handler }

// LoginResult records whether a credential attempt issued a session.
func (r *Recorder) LoginResult(success bool) {
	result := "failure"
	if success {
		result = "success"
	}
	r.authLogins.WithLabelValues(result).Inc()
}

// Middleware records inbound HTTP RED signals against matched route templates.
func (r *Recorder) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		r.http.inFlight.Inc()
		defer r.http.inFlight.Dec()

		response := &statusRecorder{ResponseWriter: w, code: http.StatusOK}
		start := time.Now()
		next.ServeHTTP(response, request)
		method := methodLabel(request.Method)
		route := routeLabel(request.Pattern)
		r.http.duration.WithLabelValues(method, route).Observe(time.Since(start).Seconds())
		r.http.requests.WithLabelValues(method, route, statusCode(response.code)).Inc()
	})
}

// FanoutMiddleware records how many outbound calls one inbound request caused.
func (r *Recorder) FanoutMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		ctx, count := withFanoutCounter(request.Context())
		routed := request.WithContext(ctx)
		next.ServeHTTP(w, routed)
		r.http.fanout.WithLabelValues(methodLabel(routed.Method), routeLabel(routed.Pattern)).
			Observe(float64(count.Load()))
	})
}

func methodLabel(method string) string {
	switch method {
	case http.MethodGet, http.MethodHead, http.MethodPost, http.MethodPut,
		http.MethodPatch, http.MethodDelete, http.MethodConnect, http.MethodOptions, http.MethodTrace:
		return method
	default:
		return "other"
	}
}

// ImageWorkerObserved records one bounded worker process observation.
func (r *Recorder) ImageWorkerObserved(observation rustgen.Observation) {
	kind := closedLabel(observation.Kind, "inspect", "render")
	result := closedLabel(observation.Result,
		"success", "refused", "canceled", "process_error", "protocol_error")
	r.images.operations.WithLabelValues(kind, result).Inc()
	r.images.duration.WithLabelValues(kind).Observe(observation.Duration.Seconds())
	r.images.inputBytes.WithLabelValues(kind).Observe(float64(observation.InputBytes))
	r.images.outputBytes.WithLabelValues(kind).Observe(float64(observation.OutputBytes))
	if observation.PeakRSSBytes > 0 {
		r.images.peakRSS.WithLabelValues(kind).Observe(float64(observation.PeakRSSBytes))
	}
}

// ImageWorkerQueueWait records admission delay under a bounded worker class.
func (r *Recorder) ImageWorkerQueueWait(class string, wait time.Duration) {
	r.images.queueWait.WithLabelValues(closedLabel(class, "interactive", "background")).Observe(wait.Seconds())
}

// ImageWorkerInFlight applies a balanced worker-capacity transition.
func (r *Recorder) ImageWorkerInFlight(delta int) {
	r.images.inFlight.Add(float64(delta))
}

func closedLabel(value string, allowed ...string) string {
	for _, candidate := range allowed {
		if value == candidate {
			return value
		}
	}
	return "other"
}

// InstrumentTransport wraps one logical outbound request with bounded RED metrics.
func (r *Recorder) InstrumentTransport(target string, next http.RoundTripper) http.RoundTripper {
	if next == nil {
		next = http.DefaultTransport
	}
	return &recorderTransport{recorder: r, target: outboundTargetLabel(target), next: next}
}

// OutboundRetried records one actual additional attempt after its wait completed.
func (r *Recorder) OutboundRetried(target string, reason OutboundRetryReason) {
	reasonLabel := "other"
	if int(reason) < len(outboundRetryReasonLabels) {
		reasonLabel = outboundRetryReasonLabels[reason]
	}
	r.outbound.retries.WithLabelValues(outboundTargetLabel(target), reasonLabel).Inc()
}

type recorderTransport struct {
	recorder *Recorder
	target   string
	next     http.RoundTripper
}

func (t *recorderTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	countOutbound(request.Context())
	started := time.Now()
	response, err := t.next.RoundTrip(request)
	t.recorder.outbound.duration.WithLabelValues(t.target).Observe(time.Since(started).Seconds())
	code := "error"
	if err == nil && response != nil && response.StatusCode >= 100 && response.StatusCode <= 599 {
		code = strconv.Itoa(response.StatusCode)
	} else if err == nil {
		code = "other"
	}
	t.recorder.outbound.requests.WithLabelValues(t.target, code).Inc()
	return response, err
}

func outboundTargetLabel(target string) string {
	switch target {
	case "tunarr", "tunarr-bulk":
		return "tunarr"
	case "tmdb", "library", "seerr", "arr", "llm":
		return target
	case "llm-video":
		return "llm"
	case "filler-review-ollama", "filler-review-openrouter":
		return "filler_review"
	case "filler-bakeoff-ollama", "filler-bakeoff-openrouter", "filler-openrouter-snapshot":
		return "filler_bakeoff"
	default:
		return "other"
	}
}
