package app

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/mantonx/loomarr/internal/activity"
	"github.com/mantonx/loomarr/internal/events"
	"github.com/mantonx/loomarr/internal/filler"
	"github.com/mantonx/loomarr/internal/library"
	"github.com/mantonx/loomarr/internal/metrics"
	"github.com/mantonx/loomarr/internal/scheduler"
	"github.com/mantonx/loomarr/internal/settings"
	"github.com/mantonx/loomarr/internal/store"
	"github.com/mantonx/loomarr/internal/tmdb"
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
}

// buildFoundation creates the shared roots consumed by later subsystem builders. The returned
// value is immutable composition output, not a builder carried and mutated across phases.
func buildFoundation(
	rootCtx context.Context,
	st store.Store,
	log *slog.Logger,
	overrides Overrides,
) (foundationBuild, error) {
	result := foundationBuild{}
	result.ready = func() (bool, string) {
		if st == nil {
			return false, "no store configured"
		}
		if _, err := st.GetSetting(context.Background(), "healthcheck_probe"); err != nil && err != store.ErrNotFound {
			return false, "store unreachable: " + err.Error()
		}
		return true, "ok"
	}
	if st != nil {
		if err := metrics.RegisterStoreCollector(st, time.Now); err != nil {
			log.Warn("metrics: store collector not registered", "err", err)
		}

		set, secrets, redactor, redactedLog, err := bootSettings(context.Background(), st, log)
		if err != nil {
			return foundationBuild{}, err
		}
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
		result.libraryClient = library.NewDynamic(result.set.libraryConn(), instanceDeviceID(rootCtx, st))
		key := func() string { return result.set.str("tmdb.api_key") }
		if overrides.TMDBBaseURL != "" {
			result.tmdbClient = tmdb.NewDynamicWithBase(overrides.TMDBBaseURL, key)
		} else {
			result.tmdbClient = tmdb.NewDynamic(key)
		}
		result.refreshSecretRedactor = func() {
			redactor.Set(collectSecrets(settings.NewRegistry(), result.set.svc, secrets))
		}
		slog.SetDefault(redactedLog)
	} else {
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
	result.jobs = scheduler.NewRegistry()
	if st != nil {
		result.activity = activity.New(st, result.log).WithNotifier(result.emitter)
	}
	return result, nil
}
