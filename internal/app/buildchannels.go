package app

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/loomarr/loomarr/internal/api"
	channelsapi "github.com/loomarr/loomarr/internal/api/channels"
	playoutapi "github.com/loomarr/loomarr/internal/api/playout"
	"github.com/loomarr/loomarr/internal/app/playoutresolve"
	"github.com/loomarr/loomarr/internal/channels"
	"github.com/loomarr/loomarr/internal/channels/approval"
	"github.com/loomarr/loomarr/internal/channels/schedule"
	"github.com/loomarr/loomarr/internal/filler/cliplibrary"
	"github.com/loomarr/loomarr/internal/library"
	"github.com/loomarr/loomarr/internal/platform/activity"
	"github.com/loomarr/loomarr/internal/platform/diagnostics"
	"github.com/loomarr/loomarr/internal/platform/events"
	"github.com/loomarr/loomarr/internal/platform/holidayvocab"
	"github.com/loomarr/loomarr/internal/platform/jobs"
	"github.com/loomarr/loomarr/internal/platform/metrics"
	"github.com/loomarr/loomarr/internal/platform/settings"
	"github.com/loomarr/loomarr/internal/platform/storagegovernor"
	"github.com/loomarr/loomarr/internal/platform/store"
	"github.com/loomarr/loomarr/internal/playout"
	"github.com/loomarr/loomarr/internal/playout/backendtransition"
	"github.com/loomarr/loomarr/internal/playout/livetvpublish"
	"github.com/loomarr/loomarr/internal/playout/tunarr"
	"github.com/loomarr/loomarr/internal/proposals/quality"
)

type channelBuild struct {
	channelService         api.ChannelService
	liveTV                 api.LiveTVService
	tunerRescanner         channelsapi.TunerRescanner
	tunarrConnector        api.TunarrConnector
	backendController      *backendtransition.Controller
	refreshBackendSettings func(context.Context) error
	desiredBackend         func(context.Context) (string, error)
	appliedBackend         func(context.Context) (string, error)
	checkpoint             func(context.Context) (backendtransition.Snapshot, error)
	playoutObserver        playoutapi.PlayoutObserver
	playoutCapability      func() playout.Capacity
	playout                playoutapi.Playout
	playoutResolverService playoutapi.PlayoutResolver
	resourceBudget         *playout.ResourceBudget // nil without internal playout
	playoutGuide           playoutapi.PlayoutGuide
	playoutResolver        *playoutresolve.Resolver
	setResidentVRAM        func(func(context.Context) (float64, string))
	channelNumbers         approval.NumberSource
}

func buildChannels(
	rootCtx context.Context, st store.Store, set resolved, ov Overrides,
	owner *generationLifecycle, capturePlayoutResolver func(*playoutresolve.Resolver),
	libraryClient *library.Client, secrets *settings.Secrets,
	readGeneratedSecret func(context.Context, settings.GeneratedSecret) (string, error),
	eventBus *events.Bus, emitter *eventEmitter, activityRec *activity.Recorder,
	jobReg *jobs.Registry, fillerLayout cliplibrary.Layout,
	log *slog.Logger, processDiagnostics *diagnostics.ProcessManager,
	storageGovernor *storagegovernor.Governor,
	metricRecorder *metrics.Recorder, watermarks *channelWatermarks, calendar func() schedule.Calendar,
) (channelBuild, error) {
	// Scheduler + Tunarr (§9, Phase 10): the channel reconcile engine + periodic
	// sweep, plus the Live TV wiring connector (guide-refresh poker). Wired when a
	// store, Tunarr, and the media-server library are configured.
	var channelSvc api.ChannelService
	var liveTVSvc api.LiveTVService
	var tunerRescanner channelsapi.TunerRescanner
	var tunarrConnectSvc api.TunarrConnector
	var mediaSource *livetvpublish.MediaSourceConnector
	var channelEngine *channels.Engine
	var liveTVConnector *livetvpublish.LiveTVConnector
	var backendController *backendtransition.Controller
	var backendView backendtransition.CheckpointView
	// A replica may enter the transition lock after another replica saved a newer
	// target or changed who owns it. Refreshing the settings snapshot inside that lock
	// makes durable values and provenance, rather than whichever process happened to
	// enqueue first, authoritative.
	refreshBackendSettings := func(ctx context.Context) error {
		if set.svc != nil {
			if err := set.svc.Refresh(ctx); err != nil {
				return fmt.Errorf("refresh settings before backend transition: %w", err)
			}
		}
		return nil
	}
	desiredBackend := func(context.Context) (string, error) {
		return settings.PlayoutBackend.Get(set), nil
	}
	resolveDesiredBackend := func(ctx context.Context) (string, error) {
		if err := refreshBackendSettings(ctx); err != nil {
			return "", err
		}
		return desiredBackend(ctx)
	}
	checkpointSnapshot := func(ctx context.Context) (backendtransition.Snapshot, error) {
		if backendView == nil {
			return backendtransition.Snapshot{}, fmt.Errorf("backend transition checkpoint is unavailable")
		}
		return backendView.Snapshot(ctx)
	}
	appliedBackendContext := func(ctx context.Context) (string, error) {
		snapshot, err := checkpointSnapshot(ctx)
		return snapshot.Applied, err
	}
	reconcileBackendContext := func(ctx context.Context) (string, error) {
		snapshot, err := checkpointSnapshot(ctx)
		return snapshot.ReconcileBackend(), err
	}
	transportBackendContext := func(ctx context.Context) (string, error) {
		snapshot, err := checkpointSnapshot(ctx)
		if err != nil {
			return "", err
		}
		if snapshot.PublishedInternal {
			return backendtransition.BackendInternal, nil
		}
		return snapshot.Applied, nil
	}
	// Internal playout (§9.1). Nil until wired below, which keeps the routes reporting "not
	// running" rather than half-serving when there is no store or no media server.
	var playoutObserver playoutapi.PlayoutObserver
	var playoutCapability func() playout.Capacity
	// The in-app HLS repackager (§9.1 Watch, V46). Built beside the session manager below; nil
	// until then so the /playout/hls routes report "not running" on an unwired install.
	var playoutSvc playoutapi.Playout
	var playoutResolverSvc playoutapi.PlayoutResolver
	// The one admission ledger for hardware and software transcodes (#1505, #1562), built beside
	// the resolver that owns capability detection.
	var resourceBudget *playout.ResourceBudget
	// The XMLTV guide (§9.1, V6b). Satisfied by the SAME *playoutresolve.Resolver as above — one
	// source for "what airs when", so the guide cannot advertise something the encoder does
	// not play.
	var playoutGuideSvc playoutapi.PlayoutGuide
	// Declared out here so the pod assembler can be attached further down: the resolver is
	// built alongside the channel engine, while the pod adapter needs the filler catalog that
	// is wired later. Both halves are required before a break can play a real commercial.
	var playoutRes *playoutresolve.Resolver
	var setResidentVRAM func(func(context.Context) (float64, string))
	// Which channel numbers Tunarr already uses (§9 V54). Function scope for the same reason as
	// `residentVRAM` above: it is SET where the programmer is built, and READ far below where the
	// binder is assembled. Nil until assigned, which numbering treats as "Loomarr's store only".
	var chanNumbers approval.NumberSource
	if st != nil {
		prog := tunarr.NewDynamicObserved(set.tunarrConfig(), metricRecorder)
		// Every production caller supplies an explicit URL snapshot from the durable checkpoint.
		// The connector's fixed fallback is empty so accidentally using a compatibility helper
		// fails closed instead of publishing a process-local target.
		liveTVConnector = livetvpublish.NewLiveTVConnector(
			func() library.LiveTV { return libraryClient.Snapshot() }, tunarr.LiveTVURLs{},
		).WithTunerPublishingDisabled(ov.TunerPublishingDisabled, log)
		liveTVSvc = liveTVAdapter{
			c: liveTVConnector,
			urls: func(ctx context.Context) (tunarr.LiveTVURLs, error) {
				if set.svc != nil {
					if err := set.svc.Refresh(ctx); err != nil {
						return tunarr.LiveTVURLs{}, fmt.Errorf("refresh settings before live TV status: %w", err)
					}
				}
				backend, err := appliedBackendContext(ctx)
				if err != nil {
					return tunarr.LiveTVURLs{}, err
				}
				tok, err := readGeneratedSecret(ctx, settings.SecretPlayout)
				if err != nil {
					return tunarr.LiveTVURLs{}, fmt.Errorf("read playout token for live TV status: %w", err)
				}
				return liveTVURLsFor(prog, backend, settings.ServerPublicURL.Get(set), tok, log), nil
			},
		}
		transportFreshness := transportTunerRescanner{
			c: liveTVConnector,
			urls: func(ctx context.Context) (tunarr.LiveTVURLs, error) {
				backend, err := transportBackendContext(ctx)
				if err != nil {
					return tunarr.LiveTVURLs{}, err
				}
				tok, err := readGeneratedSecret(ctx, settings.SecretPlayout)
				if err != nil {
					return tunarr.LiveTVURLs{}, fmt.Errorf("read playout token for tuner refresh: %w", err)
				}
				return liveTVURLsFor(prog, backend, settings.ServerPublicURL.Get(set), tok, log), nil
			},
		}
		tunerRescanner = transportFreshness

		// Wire the media server as Tunarr's media source (§6, POST /v1/setup/tunarr-connect):
		// uses the concrete Tunarr client's media-source methods (prog), or the injected
		// double in tests (tunarr.Adapter satisfies livetvpublish.MediaSourceProgrammer structurally,
		// so no type-assertion is needed). Library flavor/url/token resolved live.
		var msProg livetvpublish.MediaSourceProgrammer = prog
		if ov.Programmer != nil {
			msProg = ov.Programmer
		}
		mediaSource = livetvpublish.NewMediaSourceConnector(
			func() livetvpublish.MediaSourceLibrary { return libraryClient.Snapshot() }, msProg)
		tunarrConnectSvc = mediaSource

		// Give the scheduler a resolver so program slots carry the Library's runtime before the
		// Tunarr push, and an episode resolver so a series entry expands into the Library's
		// episodes (§9, #2151 step 16).
		meta := playoutresolve.ItemMetadata{Facts: st}
		avail := channels.NewStoreAvailability(rootCtx, st, meta.ItemDurationMs, channels.LibraryEpisodeResolver(st))
		// Bulk duration resolution, so a cycle layout costs ONE Library read instead of one per
		// movie (§9 N+1).
		avail = channels.WithBulkDurations(avail, bulkDurations(meta))
		// Tests inject an in-process Tunarr double here; production uses the real
		// URL-built programmer (prog). Either way the engine is a *channels.Engine.
		pusher := tunarr.Programmer(prog)
		if ov.Programmer != nil {
			pusher = ov.Programmer
		}
		// ⚠ Taken from `pusher`, NOT `prog`: tests inject an in-process Tunarr double here, and
		// numbering that consulted the real client while the reconcile used the double would
		// disagree about which numbers exist. Both must read the same Tunarr.
		chanNumbers = tunarrNumbers{pusher}
		channelEngine = channels.New(st, pusher, avail, transportFreshness, channels.Config{
			// Pending-slot policy defaults to pod-fill (§9); the interstitial-card
			// alternative is future design work; backfill is stable today
			// placement, handled inside the engine, not the placeholder kind.
			Policy:               schedule.PodFill,
			ReconcileTTL:         settings.ChannelReconcileEvery.Get(set),
			BreaksPerHour:        settings.FillerBreaksPerHour.Get(set),
			BreakDuration:        settings.FillerBreakDuration.Get(set),
			DefaultWindow:        settings.SchedWindowHours.Get(set),
			ResolveReconcileTTL:  func() time.Duration { return settings.ChannelReconcileEvery.Get(set) },
			ResolveBreaksPerHour: func() int { return settings.FillerBreaksPerHour.Get(set) },
			ResolveBreakDuration: func() time.Duration { return settings.FillerBreakDuration.Get(set) },
			ResolveDefaultWindow: func() time.Duration { return settings.SchedWindowHours.Get(set) },
			ResolveCalendar:      calendar,
			// Backend selection is durable and per-channel-aware inside the engine: this closure
			// supplies the durable in-progress target when one exists, otherwise the applied
			// global fallback, while schedule.PlaysInternally applies a channel's policy override.
			// This keeps ordinary reconcile aligned with the fleet barrier during a transition.
			ResolvePlayoutBackendContext: reconcileBackendContext,
			// Mid-roll breaks at measured scene fades (§10). The resolver is built after the
			// engine (below), so this reads it at pass time; nil until then means none.
			NaturalBreaks: func(ctx context.Context) schedule.NaturalBreakSource {
				return playoutRes.NaturalBreakSource(ctx)
			},
			// Measure ahead (#1518): the next two days' programmes go to the one measure worker.
			Premeasure: func(ctx context.Context, libraryItemIDs []string) {
				playoutRes.Premeasure(ctx, libraryItemIDs)
			},
		}, time.Now, log).WithMetrics(metricRecorder).WithQualityRecorder(quality.NewSchedulingRecorder(st, log))
		// Heal an entry that reached the scheduler unrated once its title is in the
		// library (§389 amendment): without this a fail-closed audience ceiling drops
		// it and the channel plays nothing (§9). Uses the same library client the
		// availability resolver does.
		channelEngine.WithRatings(libraryRatings{titles: st})
		// Stamp media-server collection membership so scope.collections enforces with no
		// library I/O on the scheduling path (programming-design §2.2). Shares the library
		// client; the reverse index is built once and cached behind a TTL.
		channelEngine.WithBoxSets(&libraryBoxSets{own: st, ttl: 15 * time.Minute})
		// Emit a `channel` SSE frame after each reconcile so the UI updates live — the
		// "no manual rebuild" model (§9). The emitter already fans to the event bus.
		channelEngine.WithNotifier(emitter)
		// A reconcile that has to MOVE a channel because Tunarr already occupies its number is an
		// operator-facing fact that must outlive a log line (§9 V54) — the number is what a viewer
		// tunes to, and the log that recorded the original strand had already scrolled away by the
		// time anyone asked what happened.
		if activityRec != nil {
			channelEngine.WithActivity(activityRec)
		}
		channelSvc = channelEngine

		// Now that the scheduler engine exists, give the emitter its backfill
		// handler: provisioning availability events (webhook + reconciler) fan to
		// OnAvailability, so an acquisition that lands `available` reconciles the
		// channels referencing it immediately — instead of waiting up to a full
		// sweep interval. (#10: the emitter was nil before this wire.)
		emitter.setEngine(channelEngine)

		backendView = backendtransition.NewDurableView(st)
		playoutBuilt, err := buildPlayout(playoutDeps{
			rootCtx: rootCtx, store: st, settings: set, owner: owner,
			captureResolver: capturePlayoutResolver, secrets: secrets,
			readSecret: readGeneratedSecret, events: eventBus, layout: fillerLayout,
			channels: channelEngine, liveTVConnector: liveTVConnector, backendView: backendView,
			resolveDesiredBackend: resolveDesiredBackend, log: log, programmer: prog,
			processDiagnostics: processDiagnostics,
			storageGovernor:    storageGovernor,
			metrics:            metricRecorder,
			capacityProbe:      ov.CapacityProbe,
			startup:            ov.Startup,
			watermarks:         watermarks,
		})
		if err != nil {
			return channelBuild{}, err
		}
		playoutObserver = playoutBuilt.observer
		playoutCapability = playoutBuilt.capability
		playoutSvc, playoutResolverSvc = playoutBuilt.service, playoutBuilt.resolverService
		playoutGuideSvc = playoutBuilt.guide
		resourceBudget = playoutBuilt.budget
		playoutRes, backendController = playoutBuilt.resolver, playoutBuilt.backendController
		setResidentVRAM = playoutBuilt.setResidentVRAM
		chEvery := settings.ChannelReconcileEvery.Get(set)
		// The channel sweep is a scheduler job now (§18.1) — same desired-vs-actual Sweep
		// (with its own ClaimDueChannels lease), driven by River. The Runner's lease/batch are
		// constructed here; it has no standalone loop.
		chSweep := channels.NewRunner(channelEngine, st, chEvery, 2*chEvery, 50, time.Now, log)
		jobReg.Add(channelMaintenanceJob(chSweep, func(ctx context.Context) error {
			return backendController.ApplyCurrent(ctx, resolveDesiredBackend)
		}, func(ctx context.Context) error {
			// Same gate as the auto-wire after a save: no Tunarr configured, nothing to wire.
			if settings.TunarrURL.Get(set) == "" {
				return nil
			}
			return mediaSource.Converge(ctx)
		}))
		log.Info("channel scheduler registered", "tunarr", settings.TunarrURL.Get(set), "sweep_every", chEvery)
	}
	return channelBuild{
		channelService: channelSvc, liveTV: liveTVSvc, tunerRescanner: tunerRescanner,
		tunarrConnector: tunarrConnectSvc, backendController: backendController,
		refreshBackendSettings: refreshBackendSettings, desiredBackend: desiredBackend,
		appliedBackend: appliedBackendContext, checkpoint: checkpointSnapshot,
		playoutObserver:        playoutObserver,
		playoutCapability:      playoutCapability,
		playout:                playoutSvc,
		playoutResolverService: playoutResolverSvc, resourceBudget: resourceBudget,
		playoutGuide: playoutGuideSvc, playoutResolver: playoutRes,
		setResidentVRAM: setResidentVRAM,
		channelNumbers:  chanNumbers,
	}, nil
}

// liveTVURLsFor picks the Live TV URL pair for the backend that will actually stream (§9.1).
// `tunarr` reads the adapter's own URLs; anything else, including an unrecognised value, falls
// back to the declared default (internal playout, Loomarr's own endpoints) and logs a warning
// naming the value: internal is the default backend and Tunarr is optional, so a stale or
// corrupted setting must not silently serve a backend the household doesn't run.
func liveTVURLsFor(prog tunarr.Adapter, backend, publicURL, deviceToken string, log *slog.Logger) tunarr.LiveTVURLs {
	normalized := schedule.NormalizePlayoutBackend(backend)
	if normalized == schedule.PlayoutBackendTunarr {
		return prog.LiveTVURLs()
	}
	if normalized != schedule.PlayoutBackendInternal {
		log.Warn("unrecognised playout.backend value; Live TV URLs fall back to internal playout", "backend", backend)
	}
	return livetvpublish.InternalPlayoutURLs(publicURL, deviceToken)
}

// householdCalendar is the calendar every channel arrangement and on-screen logo reads (#2030):
// the window grid's wall clock (windowZone), the household's country (filler.home_country) and
// the holiday definitions. Each read resolves the settings live.
func householdCalendar(set resolved, holidays holidayvocab.Source, log *slog.Logger) func() schedule.Calendar {
	zone := windowZone(func() string { return settings.GuideTimezone.Get(set) }, log)
	return func() schedule.Calendar {
		defs := holidays.Current()
		return schedule.Calendar{Zone: zone(), Country: settings.FillerHomeCountry.Get(set), Holidays: &defs}
	}
}

// windowZone resolves the wall clock the rolling-window grid is laid on (#1675): guide.timezone,
// the zone the guide is read in, else the container's own zone, so a daily window turns at local
// midnight. The setting hot-applies; the last lookup is kept so a reconcile does not re-read the
// zone database. A name that will not load falls back to the container's zone, logged when the
// setting changes.
func windowZone(name func() string, log *slog.Logger) func() *time.Location {
	var (
		mu     sync.Mutex
		loaded string
		zone   = time.Local
	)
	return func() *time.Location {
		want := strings.TrimSpace(name())
		mu.Lock()
		defer mu.Unlock()
		if want == loaded {
			return zone
		}
		loaded, zone = want, time.Local
		if want != "" {
			if loc, err := time.LoadLocation(want); err == nil {
				zone = loc
			} else if log != nil {
				log.Warn("guide.timezone does not load; rolling windows turn on the container's zone", "timezone", want, "err", err)
			}
		}
		return zone
	}
}
