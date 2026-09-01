// Package metrics owns Loomarr's generation-scoped Prometheus surface (design §7 /metrics, §17).
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
	scheduler  recorderScheduler
	playout    recorderPlayout
	channels   recorderChannels
	filler     recorderFiller
	llm        recorderLLM
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

type recorderScheduler struct {
	executions  *prometheus.CounterVec
	duration    *prometheus.HistogramVec
	running     *prometheus.GaugeVec
	lastSuccess *prometheus.GaugeVec
}

type recorderPlayout struct {
	sessionsActive prometheus.Gauge
	sessionStarts  *prometheus.CounterVec
	processFailure *prometheus.CounterVec
	fallbacks      *prometheus.CounterVec
}

type recorderChannels struct {
	reconciles    *prometheus.CounterVec
	duration      prometheus.Histogram
	substitutions prometheus.Counter
}

type recorderFiller struct {
	pods    *prometheus.CounterVec
	airings *prometheus.CounterVec
}

type recorderLLM struct {
	tokens *prometheus.CounterVec
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
	schedulerMetrics := recorderScheduler{
		executions: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: "loomarr", Subsystem: "scheduler", Name: "job_executions_total",
			Help: "Named Job executions by bounded result and trigger.",
		}, []string{"job", "result", "trigger"}),
		duration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Namespace: "loomarr", Subsystem: "scheduler", Name: "job_duration_seconds",
			Help:    "Named Job execution wall-clock duration in seconds.",
			Buckets: []float64{0.01, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10, 30, 60, 300, 1800},
		}, []string{"job"}),
		running: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: "loomarr", Subsystem: "scheduler", Name: "jobs_running",
			Help: "Named Jobs currently executing in this application generation.",
		}, []string{"job"}),
		lastSuccess: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: "loomarr", Subsystem: "scheduler", Name: "job_last_success_timestamp_seconds",
			Help: "Unix timestamp of the last successful named Job execution.",
		}, []string{"job"}),
	}
	playoutMetrics := recorderPlayout{
		sessionsActive: prometheus.NewGauge(prometheus.GaugeOpts{
			Namespace: "loomarr", Subsystem: "playout", Name: "sessions_active",
			Help: "Internal Playout sessions active in this application generation.",
		}),
		sessionStarts: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: "loomarr", Subsystem: "playout", Name: "session_starts_total",
			Help: "Internal Playout session starts by bounded result.",
		}, []string{"result"}),
		processFailure: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: "loomarr", Subsystem: "playout", Name: "process_failures_total",
			Help: "Internal Playout process failures by bounded stage.",
		}, []string{"stage"}),
		fallbacks: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: "loomarr", Subsystem: "playout", Name: "fallbacks_total",
			Help: "Internal Playout fallback transitions by bounded reason.",
		}, []string{"reason"}),
	}
	channelMetrics := recorderChannels{
		reconciles: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: "loomarr", Subsystem: "channel", Name: "reconciles_total",
			Help: "Channel reconciles by result.",
		}, []string{"result"}),
		duration: prometheus.NewHistogram(prometheus.HistogramOpts{
			Namespace: "loomarr", Subsystem: "channel", Name: "reconcile_duration_seconds",
			Help:    "Wall-clock of a single Channel reconcile in seconds.",
			Buckets: []float64{0.05, 0.1, 0.5, 1, 2.5, 5, 10, 30, 60},
		}),
		substitutions: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: "loomarr", Subsystem: "channel", Name: "slot_substitutions_total",
			Help: "Scheduled programs demoted to a placeholder because the Title vanished.",
		}),
	}
	fillerMetrics := recorderFiller{
		pods: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: "loomarr", Subsystem: "filler", Name: "pods_total",
			Help: "Assembled filler pods by fallback-ladder match level.",
		}, []string{"match_level"}),
		airings: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: "loomarr", Subsystem: "filler", Name: "rotation_airings_total",
			Help: "Actual filler clip airings by repeat state and cooldown pressure.",
		}, []string{"repeat", "cooldown"}),
	}
	llmMetrics := recorderLLM{
		tokens: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: "loomarr", Subsystem: "llm", Name: "tokens_total",
			Help: "LLM tokens consumed by grounded generation, by kind.",
		}, []string{"kind"}),
	}
	for _, result := range []string{"success", "error"} {
		channelMetrics.reconciles.WithLabelValues(result)
	}
	for _, result := range []string{"success", "capacity", "spawn_error", "canceled", "other"} {
		playoutMetrics.sessionStarts.WithLabelValues(result)
	}
	for _, stage := range []string{"parent", "program", "filler", "hls", "probe", "other"} {
		playoutMetrics.processFailure.WithLabelValues(stage)
	}
	for _, reason := range []string{"hardware_to_software", "prepared_to_live", "file_to_stream", "other"} {
		playoutMetrics.fallbacks.WithLabelValues(reason)
	}
	for _, level := range []string{"exact", "widened", "audience", "bumper_card", "other"} {
		fillerMetrics.pods.WithLabelValues(level)
	}
	for _, kind := range []string{"prompt", "completion"} {
		llmMetrics.tokens.WithLabelValues(kind)
	}
	for _, repeat := range []string{"fresh", "repeat"} {
		for _, cooldown := range []string{"ready", "relaxed", "override"} {
			fillerMetrics.airings.WithLabelValues(repeat, cooldown)
		}
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
	registry.MustRegister(build, authLogins, httpMetrics.requests, httpMetrics.duration,
		httpMetrics.inFlight, httpMetrics.fanout, imageMetrics.operations, imageMetrics.duration,
		imageMetrics.inputBytes, imageMetrics.outputBytes, imageMetrics.peakRSS,
		imageMetrics.queueWait, imageMetrics.inFlight, outboundMetrics.requests,
		outboundMetrics.duration, outboundMetrics.retries, schedulerMetrics.executions,
		schedulerMetrics.duration, schedulerMetrics.running, schedulerMetrics.lastSuccess,
		playoutMetrics.sessionsActive, playoutMetrics.sessionStarts,
		playoutMetrics.processFailure, playoutMetrics.fallbacks,
		channelMetrics.reconciles, channelMetrics.duration, channelMetrics.substitutions,
		fillerMetrics.pods, fillerMetrics.airings, llmMetrics.tokens)
	if options.Store != nil {
		now := options.Now
		if now == nil {
			now = time.Now
		}
		registry.MustRegister(newStoreCollectorWithErrors(options.Store, now, storeErrors))
	} else {
		registry.MustRegister(storeErrors)
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
		scheduler:  schedulerMetrics,
		playout:    playoutMetrics,
		channels:   channelMetrics,
		filler:     fillerMetrics,
		llm:        llmMetrics,
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

// SchedulerJobs initializes the sealed code-defined Job set to explicit zeroes.
func (r *Recorder) SchedulerJobs(jobs []string) {
	for _, job := range jobs {
		for _, result := range []string{"success", "error", "timeout", "panic"} {
			for _, trigger := range []string{"scheduled", "manual"} {
				r.scheduler.executions.WithLabelValues(job, result, trigger)
			}
		}
		r.scheduler.running.WithLabelValues(job).Set(0)
		r.scheduler.lastSuccess.WithLabelValues(job).Set(0)
	}
}

// SchedulerJobStarted records one Job entering its execution boundary.
func (r *Recorder) SchedulerJobStarted(job string) {
	r.scheduler.running.WithLabelValues(job).Inc()
}

// SchedulerJobFinished records one bounded outcome and balances the running gauge.
func (r *Recorder) SchedulerJobFinished(
	job, result, trigger string,
	duration time.Duration,
	finishedAt time.Time,
) {
	result = closedLabel(result, "success", "error", "timeout", "panic")
	trigger = closedLabel(trigger, "scheduled", "manual")
	r.scheduler.running.WithLabelValues(job).Dec()
	r.scheduler.executions.WithLabelValues(job, result, trigger).Inc()
	r.scheduler.duration.WithLabelValues(job).Observe(max(0, duration.Seconds()))
	if result == "success" {
		r.scheduler.lastSuccess.WithLabelValues(job).Set(float64(finishedAt.Unix()))
	}
}

// PlayoutSessionStarted records one bounded session-start outcome.
func (r *Recorder) PlayoutSessionStarted(result string) {
	result = closedLabel(result, "success", "capacity", "spawn_error", "canceled")
	r.playout.sessionStarts.WithLabelValues(result).Inc()
}

// PlayoutSessionActive applies a balanced live-session transition.
func (r *Recorder) PlayoutSessionActive(delta int) {
	r.playout.sessionsActive.Add(float64(delta))
}

// PlayoutProcessFailure records one bounded process stage failure.
func (r *Recorder) PlayoutProcessFailure(stage string) {
	stage = closedLabel(stage, "parent", "program", "filler", "hls", "probe")
	r.playout.processFailure.WithLabelValues(stage).Inc()
}

// PlayoutFallback records one bounded degradation transition.
func (r *Recorder) PlayoutFallback(reason string) {
	reason = closedLabel(reason, "hardware_to_software", "prepared_to_live", "file_to_stream")
	r.playout.fallbacks.WithLabelValues(reason).Inc()
}

// ChannelReconciled records one complete reconcile outcome and duration.
func (r *Recorder) ChannelReconciled(duration time.Duration, success bool) {
	result := "error"
	if success {
		result = "success"
	}
	r.channels.reconciles.WithLabelValues(result).Inc()
	r.channels.duration.Observe(max(0, duration.Seconds()))
}

// ChannelSlotSubstitutions records Titles demoted after Library drift.
func (r *Recorder) ChannelSlotSubstitutions(count int) {
	if count > 0 {
		r.channels.substitutions.Add(float64(count))
	}
}

// FillerPodAssembled records one bounded fallback-ladder rung reached by an attached pod.
func (r *Recorder) FillerPodAssembled(matchLevel string) {
	matchLevel = closedLabel(matchLevel, "exact", "widened", "audience", "bumper_card")
	r.filler.pods.WithLabelValues(matchLevel).Inc()
}

// FillerRotationAired records one actual internal-playout clip start.
func (r *Recorder) FillerRotationAired(repeated, relaxed, pinned bool) {
	repeat := "fresh"
	if repeated {
		repeat = "repeat"
	}
	cooldown := "ready"
	if pinned && repeated {
		cooldown = "override"
	} else if relaxed {
		cooldown = "relaxed"
	}
	r.filler.airings.WithLabelValues(repeat, cooldown).Inc()
}

// LLMTokens records the provider-reported aggregate token categories.
func (r *Recorder) LLMTokens(promptTokens, completionTokens int) {
	if promptTokens > 0 {
		r.llm.tokens.WithLabelValues("prompt").Add(float64(promptTokens))
	}
	if completionTokens > 0 {
		r.llm.tokens.WithLabelValues("completion").Add(float64(completionTokens))
	}
}
