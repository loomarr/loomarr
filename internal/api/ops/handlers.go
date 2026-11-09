// Package ops serves the operator and household status surfaces: liveness, readiness, Prometheus
// and the API reference; the server-sent event stream; diagnostics, startup reports and client
// crash reports; the live playout sessions and the activity and services panels; and the
// discovery feedback and quality ledgers.
package ops

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/danielgtaylor/huma/v2"

	"github.com/loomarr/loomarr/internal/api/httpcore"
	"github.com/loomarr/loomarr/internal/platform/metrics"
	"github.com/loomarr/loomarr/internal/platform/store"
	"github.com/loomarr/loomarr/internal/platform/store/identitystore"
	"github.com/loomarr/loomarr/internal/playout"
)

// Store is the slice of the aggregate store these routes read and write.
type Store interface {
	store.ActivityStore
	store.DiscoveryFeedbackStore
	store.DiscoveryQualityStore
}

// DeviceLister lists a person's paired devices, so a crash report can name the device it came from.
type DeviceLister interface {
	ListFor(ctx context.Context, userID string) ([]identitystore.DeviceToken, error)
}

// SessionObserver reports the live playout sessions and the encoder capacity.
type SessionObserver interface {
	// Stats snapshots every live encoder.
	Stats(now time.Time) []playout.SessionStat
	// Capacity is how many channels the host can stream at once.
	Capacity() int
}

// Deps is what the ops routes need from the rest of the server. Every field is optional: a nil
// service leaves its routes answering 501 or reading empty, as the API's nil-semantics describe.
type Deps struct {
	Log   *slog.Logger
	Store Store
	// Ready reports readiness for /v1/readyz; nil reads as ready.
	Ready func() (ready bool, detail string)
	// Metrics serves the Prometheus exposition; nil answers 404.
	Metrics *metrics.Recorder
	// MetricsToken is the Prometheus scrape bearer; empty refuses every scrape.
	MetricsToken string
	// Pprof mounts the profiler routes.
	Pprof bool
	// Events is the subscribe surface the SSE stream needs; Shutdown closes long-lived streams
	// before the HTTP drain.
	Events   EventSource
	Shutdown <-chan struct{}
	// The diagnostics and crash-report services.
	StartupReports      StartupReportService
	DiagnosticEvents    DiagnosticEventService
	DiagnosticProcesses DiagnosticProcessService
	DiagnosticBundles   DiagnosticBundleService
	DiagnosticCapture   DiagnosticCaptureService
	ClientDiagnostics   ClientDiagnosticService
	ClientCrashes       ClientCrashService
	// Devices names the device a crash report came from.
	Devices DeviceLister
	// PlayoutObserver supplies the live playout sessions.
	PlayoutObserver SessionObserver
	// ConnectionChecks runs the connection probes the services panel shows.
	ConnectionChecks func(ctx context.Context) []httpcore.SetupCheck
	// StoreFacts reports the schema version and backend name, or is nil on a store-less boot.
	StoreFacts func() (schemaVersion int64, backend string)
}

// Handlers serves the ops routes.
type Handlers struct {
	log                 *slog.Logger
	store               Store
	ready               func() (bool, string)
	metrics             *metrics.Recorder
	metricsToken        string
	pprof               bool
	events              EventSource
	shutdown            <-chan struct{}
	startupReports      StartupReportService
	diagnosticEvents    DiagnosticEventService
	diagnosticProcesses DiagnosticProcessService
	diagnosticBundles   DiagnosticBundleService
	diagnosticCapture   DiagnosticCaptureService
	clientDiagnostics   ClientDiagnosticService
	clientCrashes       ClientCrashService
	devices             DeviceLister
	playoutObserver     SessionObserver
	connectionChecks    func(ctx context.Context) []httpcore.SetupCheck
	storeFacts          func() (int64, string)

	// schemaOnly is set only by the OpenAPI exporter: it mounts every route whatever is configured,
	// so the generated spec lists them all.
	schemaOnly bool
}

// New returns the ops handlers over d. schemaOnly mounts every route regardless of which services
// d carries; only the OpenAPI exporter sets it.
func New(d Deps, schemaOnly bool) *Handlers {
	return &Handlers{
		log: d.Log, store: d.Store, ready: d.Ready, metrics: d.Metrics, metricsToken: d.MetricsToken,
		pprof: d.Pprof, events: d.Events, shutdown: d.Shutdown, startupReports: d.StartupReports,
		diagnosticEvents: d.DiagnosticEvents, diagnosticProcesses: d.DiagnosticProcesses,
		diagnosticBundles: d.DiagnosticBundles, diagnosticCapture: d.DiagnosticCapture,
		clientDiagnostics: d.ClientDiagnostics, clientCrashes: d.ClientCrashes, devices: d.Devices,
		playoutObserver: d.PlayoutObserver, connectionChecks: d.ConnectionChecks, storeFacts: d.StoreFacts, schemaOnly: schemaOnly,
	}
}

// Register mounts every ops route on api.
func (s *Handlers) Register(api huma.API) {
	s.registerOps(api, s.pprof, s.metricsToken)
	s.registerEvents(api)
	s.registerDiagnostics(api) // also mounts the client crash routes when they are configured
	s.registerDashboard(api)
	s.registerDashboardPanels(api)
	s.registerDiscoveryFeedback(api)
	s.registerDiscoveryQuality(api)
}

func (s *Handlers) runConnectionChecks(ctx context.Context) []httpcore.SetupCheck {
	if s.connectionChecks == nil {
		return []httpcore.SetupCheck{}
	}
	return s.connectionChecks(ctx)
}

func (s *Handlers) writeProblem(w http.ResponseWriter, r *http.Request, status int, title, detail string) {
	httpcore.WriteProblem(s.log, w, r, status, title, detail)
}
