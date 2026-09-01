package app

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/loomarr/loomarr/internal/activity"
	"github.com/loomarr/loomarr/internal/buildinfo"
	"github.com/loomarr/loomarr/internal/diagnostics"
	"github.com/loomarr/loomarr/internal/events"
	"github.com/loomarr/loomarr/internal/filler"
	"github.com/loomarr/loomarr/internal/library"
	"github.com/loomarr/loomarr/internal/metrics"
	"github.com/loomarr/loomarr/internal/scheduler"
	"github.com/loomarr/loomarr/internal/settings"
	"github.com/loomarr/loomarr/internal/store"
	"github.com/loomarr/loomarr/internal/tmdb"
)

type foundationBuild struct {
	ready                  func() (bool, string)
	set                    resolved
	desiredSet             resolved
	appliedRestartSettings map[string]string
	fillerLayout           filler.Layout
	secrets                *settings.Secrets
	log                    *slog.Logger
	libraryClient          *library.Client
	tmdbClient             *tmdb.Client
	refreshSecretRedactor  func()
	readGeneratedSecret    func(context.Context, settings.GeneratedSecret) (string, error)
	eventBus               *events.Bus
	emitter                *eventEmitter
	jobs                   *scheduler.Registry
	activity               *activity.Recorder
	diagnostics            *diagnostics.Recorder
	diagnosticEvents       *diagnostics.EventLog
	diagnosticProcesses    *diagnostics.ProcessLog
	diagnosticBundles      *diagnostics.BundleService
	clientDiagnostics      *diagnostics.ClientIngestor
	processDiagnostics     *diagnostics.ProcessManager
	startup                *diagnostics.Startup
	startupReports         *diagnostics.StartupReports
	instanceID             string
	metrics                *metrics.Recorder
}

// buildFoundation creates the shared roots consumed by later subsystem builders. The returned
// value is immutable composition output, not a builder carried and mutated across phases.
func buildFoundation(
	rootCtx context.Context,
	st store.Store,
	log *slog.Logger,
	overrides Overrides,
	owner *generationLifecycle,
) (foundationBuild, error) {
	result := foundationBuild{}
	info := buildinfo.Get()
	database := string(store.DialectOf(st))
	if database == "" {
		database = "unavailable"
	}
	result.metrics = metrics.New(metrics.Options{
		Version: info.Version, Revision: info.Commit, Database: database,
		Store: st, Now: time.Now,
	})
	instanceID := ""
	fallbackLog := log
	var secretRedactor *settings.Redactor
	result.startup = overrides.Startup
	if result.startup == nil {
		result.startup = diagnostics.NewStartup(time.Now(), 1, "dev", []diagnostics.StartupCheck{
			{Key: "database", Label: "Database", Required: true,
				Mode: diagnostics.HealthCheckContinuous, FreshFor: 3 * time.Minute},
		}, time.Now)
		if st == nil {
			result.startup.Complete("database", diagnostics.StartupFailed, "no store configured", "/settings/system/database", "")
		} else if _, err := st.GetSetting(context.Background(), "healthcheck_probe"); err != nil && err != store.ErrNotFound {
			result.startup.Complete("database", diagnostics.StartupFailed, "store unreachable", "/settings/system/database", "")
		} else {
			result.startup.Complete("database", diagnostics.StartupPassed, "ready", "", "")
		}
	}
	result.ready = result.startup.Ready
	result.log = log
	if st != nil {
		// Startup retention is available as soon as the migrated store is available. Building it
		// before settings means a later settings/generated-secret failure is still retained.
		instanceID = instanceDeviceID(rootCtx, st)
		result.instanceID = instanceID
		result.diagnostics = diagnostics.New(st, diagnostics.Options{
			OnFailure: func(err error, count int) {
				fallbackLog.Error("diagnostics: persistence failed", "err", err, "records", count)
			},
		})
		owner.addStop(result.diagnostics.Close)
		result.diagnosticEvents = diagnostics.NewEventLog(st, time.Now).WithDropped(result.diagnostics.Dropped)
		result.clientDiagnostics = diagnostics.NewClientIngestor(result.diagnostics, time.Now)
		result.startup.AttachPersistence(result.diagnostics, instanceID)
		result.startupReports = diagnostics.NewStartupReports(result.startup, st, time.Now)
	}
	if st != nil {
		set, secrets, redactor, redactedLog, err := bootSettings(context.Background(), st, log)
		if err != nil {
			result.startup.Complete(diagnostics.StartupCheckGeneratedSecrets, diagnostics.StartupFailed,
				"settings and generated secrets could not be initialized", "/settings/system/diagnostics", "")
			return foundationBuild{}, err
		}
		result.startup.Complete(diagnostics.StartupCheckGeneratedSecrets, diagnostics.StartupPassed,
			"generated secrets are available", "", "")
		result.desiredSet = set
		result.set, result.appliedRestartSettings = set.freeze(settings.NewRegistry().RestartKeys()...)
		result.fillerLayout, err = filler.NewLayout(
			result.set.str("filler.dir"), result.set.str("filler.watch_dir"),
		)
		if err != nil {
			return foundationBuild{}, fmt.Errorf("resolve filler storage layout: %w", err)
		}
		canonicalFillerRestartBaseline(result.appliedRestartSettings, result.fillerLayout)
		result.secrets = secrets
		result.log = redactedLog
		fallbackLog = redactedLog
		secretRedactor = redactor
		result.libraryClient = library.NewDynamic(result.set.libraryConn(), instanceID)
		key := func() string { return result.set.str("tmdb.api_key") }
		if overrides.TMDBBaseURL != "" {
			result.tmdbClient = tmdb.NewDynamicWithBase(overrides.TMDBBaseURL, key)
		} else {
			result.tmdbClient = tmdb.NewDynamic(key)
		}
		result.refreshSecretRedactor = func() {
			redactor.Set(collectSecrets(settings.NewRegistry(), result.set.svc, secrets))
		}
	} else {
		result.startup.Complete(diagnostics.StartupCheckGeneratedSecrets, diagnostics.StartupSkipped,
			"database unavailable", "/settings/system/diagnostics", "")
		result.log = log
		result.refreshSecretRedactor = func() {}
	}
	result.readGeneratedSecret = func(ctx context.Context, secret settings.GeneratedSecret) (string, error) {
		return currentGeneratedSecret(
			ctx, store.DialectOf(st), result.secrets, secret, result.refreshSecretRedactor,
		)
	}
	result.eventBus = events.NewBus()
	result.emitter = &eventEmitter{bus: result.eventBus}
	result.startup.SetHealthNotifier(result.emitter.HealthChanged)
	result.jobs = scheduler.NewRegistry()
	if st != nil {
		// Dynamic secret redaction wraps the fan-out rather than only stdout. Both durable events
		// and JSON output therefore receive the same scrubbed record, while the recorder failure
		// hook above retains its permanently stdout-only logger.
		result.log = slog.New(secretRedactor.Handler(diagnostics.NewSlogHandler(log.Handler(), result.diagnostics)))
		slog.SetDefault(result.log)
		result.activity = activity.New(st, result.log).WithNotifier(result.emitter)
		result.processDiagnostics = diagnostics.NewProcessManager(st, result.diagnostics, diagnostics.ProcessOptions{
			OutputDir: result.set.str("diagnostics.dir"), InstanceID: instanceID,
			OnFailure: func(err error) { fallbackLog.Error("diagnostics: process recorder failed", "err", err) },
		})
		result.diagnosticProcesses = diagnostics.NewProcessLog(st, diagnostics.ProcessReadOptions{
			OutputDir: result.set.str("diagnostics.dir"), Now: time.Now,
		})
		owner.addStop(result.processDiagnostics.Close)
	} else {
		slog.SetDefault(result.log)
	}
	if result.startupReports == nil {
		result.startupReports = diagnostics.NewStartupReports(result.startup, nil, time.Now)
	}
	if result.diagnosticEvents != nil && result.diagnosticProcesses != nil {
		result.diagnosticBundles = diagnostics.NewBundleService(diagnostics.BundleOptions{
			Events: result.diagnosticEvents, Processes: result.diagnosticProcesses,
			Health: result.startupReports.Health, Build: func() diagnostics.BundleBuild {
				info := buildinfo.Get()
				return diagnostics.BundleBuild{Version: info.Version, Commit: info.Commit, BuiltAt: info.BuiltAt, Dirty: info.Dirty}
			}, Now: time.Now,
		})
	}
	return result, nil
}
