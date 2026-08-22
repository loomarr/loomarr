// Package app is the composition root: it wires every subsystem from an open
// store into the API handler that cmd/loomarr serves and the integration tests
// drive. Keeping it importable (not package main) is what lets tests exercise
// the REAL wiring end to end (§21), and keeps cmd/loomarr a thin entrypoint.
package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"github.com/mantonx/loomarr/internal/api"
	"github.com/mantonx/loomarr/internal/auth"
	"github.com/mantonx/loomarr/internal/backendtransition"
	"github.com/mantonx/loomarr/internal/binder"
	"github.com/mantonx/loomarr/internal/channels"
	"github.com/mantonx/loomarr/internal/config"
	"github.com/mantonx/loomarr/internal/events"
	"github.com/mantonx/loomarr/internal/library"
	"github.com/mantonx/loomarr/internal/llm"
	"github.com/mantonx/loomarr/internal/media"
	"github.com/mantonx/loomarr/internal/playout"
	"github.com/mantonx/loomarr/internal/prepared"
	"github.com/mantonx/loomarr/internal/programmer"
	"github.com/mantonx/loomarr/internal/schedule"
	"github.com/mantonx/loomarr/internal/scheduler"
	"github.com/mantonx/loomarr/internal/settings"
	"github.com/mantonx/loomarr/internal/setup"
	"github.com/mantonx/loomarr/internal/store"
	"github.com/mantonx/loomarr/internal/suggest"
)

// Overrides injects the two in-process boundaries (the Tunarr push target and the
// LLM provider) for tests. Both nil ⇒ the real URL-built adapters (production).
type Overrides struct {
	Programmer programmer.Programmer // nil ⇒ programmer.NewDynamic(live Tunarr config)
	LLM        llm.Provider          // nil ⇒ the Swappable from buildLLM
	// TMDBBaseURL redirects the real dynamic TMDB adapter to an in-process external
	// service double. The credential still comes from live settings; accepting a
	// prebuilt client here used to freeze its key at boot and bypass composition.
	TMDBBaseURL string // empty ⇒ https://api.themoviedb.org/3
	// DevLogin mounts POST /v1/auth/dev-login (§11). run() sets it from
	// LOOMARR_DEV_LOGIN; it rides Overrides so the §19 negative tests can build a
	// handler BOTH ways through the real composition root, rather than asserting
	// against a hand-rolled router that could drift from what production mounts.
	DevLogin bool
	// Pprof mounts /debug/pprof/* (§7). Rides Overrides alongside DevLogin for the same
	// reason: run() sets it from LOOMARR_PPROF, and a test can build a handler either way
	// through the real composition root.
	Pprof bool
	// Restart asks the process to rebuild itself in place (§9.2). nil ⇒ the restart
	// routes report 501 rather than pretending: an embedded or test-built handler has no
	// loop behind it, and a button that silently does nothing is worse than an absent one.
	//
	// A func rather than a channel so the API package never learns how restarting works —
	// only main owns the generation loop. It must NOT block: the handler calls it while
	// responding, and the drain it triggers cannot begin until that response is written.
	Restart func()
	// DatabaseMigration asks the process-owned generation loop to drain the current
	// handler and perform a SQLite→Postgres migration after every old-generation
	// writer has stopped. nil keeps the database service fail-closed in embedded tests.
	DatabaseMigration func(string) error
	// DatabaseMigrationError carries a failed attempt into the replacement SQLite
	// generation, where the database status endpoint can explain why it stayed put.
	DatabaseMigrationError string
	// ImageWorkerExecutable selects the required Rust image worker in tests and embedded builds.
	// Empty resolves LOOMARR_IMAGE_WORKER, then the sibling/local/PATH-installed release binary.
	ImageWorkerExecutable string
}

// Build wires every subsystem from the already-open store + logger and returns one
// lifecycle-owned application generation. It is the seam run() (production) and the integration
// harness (tests) both call, so tests drive the real composition rather than a copy. It does not
// open/close the store, read process config, listen, or handle signals; those stay in run().
//
// # Why this is one long function
//
// Decomposing it into methods on a shared builder struct was tried on paper and rejected: the
// sections are sequential and genuinely interdependent (three back-patches — emitter.setEngine,
// playoutRes.activeChannels, playoutRes.pods), so splitting them would convert ~70 locals into
// fields on a mutable carrier. That WIDENS their scope rather than narrowing it, and trades
// compile-time use-before-assignment errors for runtime nils.
//
// ⚠ **It said "~630 lines and stays that way". Measured 2026-08-10 it is 1,457** — 94 branches
// and 15 separate `if st != nil` blocks. "A composition root is allowed to be
// long; it is not allowed to be unnavigable" was the right test and it now fails: the section
// map below makes it possible to FIND a heading, not to hold the whole.
//
// The sanctioned decomposition (§14.1) is per-subsystem FUNCTIONS returning values —
// `buildFiller(deps) (…, error)` — each owning its own `if st != nil` guard. That is a
// different shape from the rejected builder: nothing widens to a mutable field, and the
// use-before-assignment errors stay. The three back-patches stay explicit here in the root.
// Measured coupling, which is what makes this tractable: the 434-line filler section reads
// only EIGHT locals from earlier sections. Section size does not predict extraction cost.
//
// # The nil-store path is deliberate
//
// Most sections sit inside `if st != nil`. That guard is not defensive habit: a container
// started without DATABASE_URL must still build a handler and answer /readyz with the reason,
// rather than crash-looping past the probe that would explain the problem. See
// TestBuildWithoutStoreServesReadinessInsteadOfPanicking — the nesting is the price of
// that behaviour.
//
// # Section map (in dependency order — later sections read earlier ones)
//
//	readiness + metrics      /readyz truth, store gauges
//	event bus + emitter      §7 SSE, §4 terminal-transition fan-out
//	job registry             §18.1 — every recurring job registers here, started at the end
//	provisioning reconciler  §7 acquisition loop + session sweep
//	scheduler + Tunarr       §9 channel reconcile engine
//	internal playout         §9.1 sessions, resolver, XMLTV guide (one resolver, both uses)
//	channel binder           §7 the ONE approved-proposal → channel path
//	suggester + search       §8 catalog boundary, LLM, grounding
//	filler + commercials     §10 catalog sync, pod assembly
//	playout device token     §11 rotation-safe (a func, never a captured value)
//	scheduler start          §18.1 — after every subsystem has registered its jobs
//	api.Options              the single assembly point
const replicaSettingsRefreshInterval = 30 * time.Second

// internalPlayoutNeedsPublicURL reports whether the boot path should warn that internal
// playout has no reachable base URL. Callers invoke it only inside the internal-playout branch,
// so the sole question here is whether server.public_url is effectively empty — whitespace is
// treated as unset because a blank env value round-trips to "  " through some shells and is not
// a usable URL. (beta-readiness D-4.)
func internalPlayoutNeedsPublicURL(publicURL string) bool {
	return strings.TrimSpace(publicURL) == ""
}

// startReplicaSettingsRefresh owns the one periodic settings reader for a Postgres
// application generation. SQLite is single-process by contract and returns without
// starting a goroutine or issuing any additional store reads.
func startReplicaSettingsRefresh(
	ctx context.Context,
	dialect store.Dialect,
	svc *settings.Service,
	interval time.Duration,
	after func(),
) bool {
	if dialect != store.DialectPostgres || svc == nil {
		return false
	}
	go svc.RefreshEvery(ctx, interval, after)
	return true
}

func trackReplicaSettingsRefresh(
	owner *generationLifecycle,
	dialect store.Dialect,
	svc *settings.Service,
	interval time.Duration,
	after func(),
) bool {
	if dialect != store.DialectPostgres || svc == nil {
		return false
	}
	owner.goRun(func(ctx context.Context) { svc.RefreshEvery(ctx, interval, after) })
	return true
}

func buildHandler(
	rootCtx context.Context,
	st store.Store,
	log *slog.Logger,
	ov Overrides,
	owner *generationLifecycle,
	capturePlayoutResolver func(*playoutResolver),
) (http.Handler, error) {
	foundation, err := buildFoundation(rootCtx, st, log, ov)
	if err != nil {
		return nil, err
	}
	ready := foundation.ready
	set, desiredSet := foundation.set, foundation.desiredSet
	appliedRestartSettings, fillerLayout := foundation.appliedRestartSettings, foundation.fillerLayout
	secrets, log := foundation.secrets, foundation.log
	libraryClient, tmdbClient := foundation.libraryClient, foundation.tmdbClient
	refreshSecretRedactor, readGeneratedSecret := foundation.refreshSecretRedactor, foundation.readGeneratedSecret
	eventBus, emitter := foundation.eventBus, foundation.emitter
	jobReg, activityRec := foundation.jobs, foundation.activity

	episodeRefresh := buildProvisioning(st, set, libraryClient, emitter, jobReg, activityRec, log)

	// Scheduler + Tunarr (§9, Phase 10): the channel reconcile engine + periodic
	// sweep, plus the Live TV wiring connector (guide-refresh poker). Wired when a
	// store, Tunarr, and the media-server library are configured.
	var channelSvc api.ChannelService
	var liveTVSvc api.LiveTVService
	var tunerRescanner api.TunerRescanner
	var tunarrConnectSvc api.TunarrConnector
	var channelEngine *channels.Engine
	var liveTVConnector *setup.LiveTVConnector
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
		return set.str("playout.backend"), nil
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
	var playoutObserver api.PlayoutObserver
	// Prepared readiness is observed through the planner itself; the API only snapshots it.
	var preparedObserver api.PreparedObserver
	// The in-app HLS repackager (§9.1 Watch, V46). Built beside the session manager below; nil
	// until then so the /playout/hls routes report "not running" on an unwired install.
	var playoutSvc api.Playout
	var playoutResolverSvc api.PlayoutResolver
	// hwEncodeSlots reports the box's concurrent hardware-transcode capacity for the admission gate
	// (§9.1 V47). Nil until playout is wired below — nil leaves the gate off (hardware unbounded).
	var hwEncodeSlots func(context.Context) int
	// One host-wide pool arbitrates the measured hardware slots between live playout and prepared
	// media. It is created beside the resolver that owns capability detection, then handed to both
	// consumers; neither may maintain a private GPU counter.
	var encodePool *media.EncodePool
	// The XMLTV guide (§9.1, V6b). Satisfied by the SAME *playoutResolver as above — one
	// source for "what airs when", so the guide cannot advertise something the encoder does
	// not play.
	var playoutGuideSvc api.PlayoutGuide
	// Declared out here so the pod assembler can be attached further down: the resolver is
	// built alongside the channel engine, while the pod adapter needs the filler catalog that
	// is wired later. Both halves are required before a break can play a real commercial.
	var playoutRes *playoutResolver
	// residentVRAM — the admission budget's late-bound "VRAM a resident LLM holds now" hook (§9.1
	// V49). Declared at FUNCTION scope (not inside the playout block) because it is SET far below,
	// where the LLM residency getter is built, yet READ by the budget closure inside the block. A no-op
	// (nil) until assigned, which the budget treats as "no contention".
	var residentVRAM func(context.Context) (gib float64, model string)
	// Which channel numbers Tunarr already uses (§9 V54). Function scope for the same reason as
	// `residentVRAM` above: it is SET where the programmer is built, and READ far below where the
	// binder is assembled. Nil until assigned, which numbering treats as "Loomarr's store only".
	var chanNumbers binder.NumberSource
	if st != nil {
		lib := libraryClient
		prog := programmer.NewDynamic(set.tunarrConfig())
		// Every production caller supplies an explicit URL snapshot from the durable checkpoint.
		// The connector's fixed fallback is empty so accidentally using a compatibility helper
		// fails closed instead of publishing a process-local target.
		liveTVConnector = setup.NewLiveTVConnector(
			func() library.LiveTV { return libraryClient.Snapshot() }, setup.LiveTVURLs{})
		liveTVSvc = liveTVAdapter{
			c: liveTVConnector,
			urls: func(ctx context.Context) (setup.LiveTVURLs, error) {
				if set.svc != nil {
					if err := set.svc.Refresh(ctx); err != nil {
						return setup.LiveTVURLs{}, fmt.Errorf("refresh settings before live TV status: %w", err)
					}
				}
				backend, err := appliedBackendContext(ctx)
				if err != nil {
					return setup.LiveTVURLs{}, err
				}
				tok, err := readGeneratedSecret(ctx, settings.SecretPlayout)
				if err != nil {
					return setup.LiveTVURLs{}, fmt.Errorf("read playout token for live TV status: %w", err)
				}
				return setup.LiveTVURLsFor(
					backend, set.str("tunarr.url"), set.str("server.public_url"), tok,
				), nil
			},
		}
		transportFreshness := transportTunerRescanner{
			c: liveTVConnector,
			urls: func(ctx context.Context) (setup.LiveTVURLs, error) {
				backend, err := transportBackendContext(ctx)
				if err != nil {
					return setup.LiveTVURLs{}, err
				}
				tok, err := readGeneratedSecret(ctx, settings.SecretPlayout)
				if err != nil {
					return setup.LiveTVURLs{}, fmt.Errorf("read playout token for tuner refresh: %w", err)
				}
				return setup.LiveTVURLsFor(
					backend, set.str("tunarr.url"), set.str("server.public_url"), tok,
				), nil
			},
		}
		tunerRescanner = transportFreshness

		// Wire the media server as Tunarr's media source (§6, POST /v1/setup/tunarr-connect):
		// uses the concrete Tunarr client's media-source methods (prog), or the injected
		// double in tests when it implements them. Library flavor/url/token resolved live.
		var msProg setup.MediaSourceProgrammer = prog
		if ov.Programmer != nil {
			if msp, ok := ov.Programmer.(setup.MediaSourceProgrammer); ok {
				msProg = msp
			}
		}
		tunarrConnectSvc = setup.NewMediaSourceConnector(
			func() setup.MediaSourceLibrary { return libraryClient.Snapshot() }, msProg)

		// Program duration comes from the media server (§9/§10): give the scheduler
		// a resolver so program slots carry a real runtime before the Tunarr push,
		// and an episode resolver so a series entry expands into its episodes (§9).
		avail := channels.NewStoreAvailability(rootCtx, st, lib.ItemDurationMs, episodeResolver(lib))
		// Bulk duration resolution, so a cycle layout costs ONE media-server call instead of one
		// per movie (§9 N+1). ItemMetadataByID already asks for RunTimeTicks and already batches
		// by id list — the bulk answer simply was not wired to the scheduler's duration path, so
		// a 25-movie channel paid 25 sequential round trips on every cold guide request (~375ms
		// of GET /v1/guide, measured against the dev Emby).
		avail = channels.WithBulkDurations(avail, bulkDurations(lib))
		// Tests inject an in-process Tunarr double here; production uses the real
		// URL-built programmer (prog). Either way the engine is a *channels.Engine.
		pusher := programmer.Programmer(prog)
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
			ReconcileTTL:         set.dur("channel.reconcile_every"),
			BreaksPerHour:        set.intv("filler.breaks_per_hour"),
			BreakDuration:        set.dur("filler.break_duration"),
			DefaultWindow:        set.dur("sched.window_hours"),
			ResolveReconcileTTL:  func() time.Duration { return set.dur("channel.reconcile_every") },
			ResolveBreaksPerHour: func() int { return set.intv("filler.breaks_per_hour") },
			ResolveBreakDuration: func() time.Duration { return set.dur("filler.break_duration") },
			ResolveDefaultWindow: func() time.Duration { return set.dur("sched.window_hours") },
			// Backend selection is durable and per-channel-aware inside the engine: this closure
			// supplies the durable in-progress target when one exists, otherwise the applied
			// global fallback, while schedule.PlaysInternally applies a channel's policy override.
			// This keeps ordinary reconcile aligned with the fleet barrier during a transition.
			ResolvePlayoutBackendContext: reconcileBackendContext,
		}, time.Now, log)
		// Heal an entry that reached the scheduler unrated once its title is in the
		// library (§389 amendment): without this a fail-closed audience ceiling drops
		// it and the channel plays nothing (§9). Uses the same library client the
		// availability resolver does.
		channelEngine.WithRatings(libraryRatings{lib: lib})
		// Stamp media-server collection membership so scope.collections enforces with no
		// library I/O on the scheduling path (programming-design §2.2). Shares the library
		// client; the reverse index is built once and cached behind a TTL.
		channelEngine.WithBoxSets(&libraryBoxSets{lib: lib, ttl: 15 * time.Minute})
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

		// Internal playout (§9.1): Loomarr serves its own channels. Wired here because this is
		// where BOTH halves already exist — the engine that answers "what airs when" and the
		// library client that resolves an item to a streamable URL.
		//
		// ⚠ **The manager is built FIRST, and the resolver second.** The profile depends on
		// how many channels are encoding (the load-aware ladder), so the resolver needs
		// `playoutMgr.ActiveCount`.
		//
		// This used to read "the cycle is broken with a func … assigned after the manager
		// exists", and there was no cycle: the manager never references the resolver. The
		// resolver was simply constructed 45 lines too early, and the field was back-patched
		// to compensate. Since `activeChannels` is called UNGUARDED, forgetting that patch
		// was a nil-func panic when a viewer tuned in — and nothing tested it. Building in
		// dependency order puts the field in the literal, where an omission is visible.
		// Nil-guarded like every other secrets read in this file: the parent's playlist URL is
		// built at SPAWN time, so an unguarded read here would panic when a viewer tunes in
		// rather than at boot — the worst place to find out.
		playoutTokenFn := func() string {
			if secrets == nil {
				return ""
			}
			token, err := readGeneratedSecret(context.Background(), settings.SecretPlayout)
			if err != nil {
				return "" // fail child URL generation closed while durable auth is unavailable.
			}
			return token
		}
		// residentVRAM (declared at function scope above) is the late-bound hook to "how much GPU VRAM
		// a resident LLM holds right now" (§9.1 V49) — the real getter is assigned far below, after the
		// LLM wiring. The budget closure reads it through that pointer; nil ⇒ assume no contention.
		//
		// playoutBudget is the DYNAMIC admission budget: concurrent VIDEO transcodes this box can
		// sustain right now (§9.1 V49). Composed from three live sources, re-read on every admission:
		//   1. MEASURED capacity — what Detect's lazy encoder trial found this box sustains
		//      (playoutRes.maxChannels). The source of truth for "how many encodes fit"; until the first
		//      trial completes, EffectiveCapacity deliberately permits one conservative transcode.
		//   2. OPERATOR SAFETY CAP — playout.max_channels, applied as a HARD CAP (min): an operator may
		//      only LOWER below the measurement (a safety throttle), never claim more than the hardware
		//      proved. 0/unset ⇒ no cap, use the measurement.
		//   3. VRAM SHADING — a resident LLM steals VRAM each hardware encode needs for its device
		//      context; the original black screen was an encoder that could not allocate under a
		//      resident model. So when a model is resident, shade the budget down by the encodes that
		//      VRAM can no longer host (~1 hardware encode per few GiB held). Reactive to the model
		//      loading/unloading, so headroom grows back when it evicts.
		playoutBudget := func() int {
			measured := 0
			if playoutRes != nil {
				measured = int(playoutRes.maxChannels.Load()) // published once by the lazy Detect trial
			}
			residentGiB := 0.0
			if residentVRAM != nil {
				residentGiB, _ = residentVRAM(rootCtx)
			}
			return playout.EffectiveCapacity(measured, set.intv("playout.max_channels"), residentGiB)
		}
		playoutMgr := playout.NewManager(
			playoutSpawner(set.str("playout.ffmpeg_path"),
				func() string { return set.str("server.public_url") },
				playoutTokenFn, log),
			playoutBudget,
			playout.DefaultGrace,
			log,
		)
		// A committed internal Desired-cycle change must retire the encoder reading the
		// previous cycle. The next tune starts from the new wall-clock position; peer
		// Postgres replicas receive the same cutover through durable invalidations.
		channelEngine.WithScheduleInvalidator(playoutMgr)
		playoutRes = &playoutResolver{
			engine: channelEngine, lib: lib, now: time.Now,
			detectContext: rootCtx,
			// The store, narrowed to GetTitle — the grid's provenance line reads acquisition
			// state and must not be able to change it.
			titles: st,
			// Same store, narrowed to the one write the resolver legitimately makes: counting
			// a filler clip as having aired (V28).
			clipPlays: st,
			// Airing history (§5, programming-design §3.1): the same store, narrowed to the one
			// write. Recording what actually aired is what lets placement prefer titles that
			// have NOT been on recently — the memory the scheduler previously lacked.
			airings: st,
			// The arranged-cycle cache and the channel read it fingerprints (cyclecache.go): the
			// guide re-arranges every channel on every poll, which profiled as 53% of the
			// request's CPU. GUIDE PATHS ONLY — AiringNow stays on the live computation, so a
			// cache bug degrades a grid rather than a broadcast.
			channels: st,
			// The store, narrowed to the single derived-column write ComputeChannelCodec makes
			// (§9.1 V50): persist the majority broadcast codec measured from the channel's content.
			codecs:   st,
			cycles:   newCycleCache(time.Now),
			tier:     func() string { return set.str("playout.quality_tier") },
			encoder:  func() string { return set.str("playout.encoder") },
			capacity: playoutBudget,
			// fillerDir belongs to the immutable generation layout. Changing storage roots
			// is applied only after the generation drains and rebuilds (§10); `pods` is
			// assigned after the pod adapter is built further down.
			fillerDir: fillerLayout.ClipDir(),
			// The capability probe runs lazily on the first program that needs it, when
			// playout.encoder is unset — so a box with a working GPU uses it instead of
			// silently falling back to software.
			ffmpegPath: func() string { return set.str("playout.ffmpeg_path") },
			// GPU name for the encoder chooser's vendor-native hint (Detect). Read via the LLM
			// package's thin nvidia-smi wrapper — the same GPU signal the rest of the app probes —
			// so playout picks NVENC on an NVIDIA card rather than young cross-vendor Vulkan. Called
			// once, lazily, inside the memoised capability probe; "" (unknown GPU) is a fine default.
			gpuName: func() string { return llm.GPUName(rootCtx) },
			// Preferred audio language (§9.1), read live so a Settings change applies to the
			// next programme rather than the next restart. The prober derives ffprobe from the
			// ffmpeg path — the two ship together, so an operator who moved one moved both.
			audioLanguage: func() string { return set.str("playout.audio_language") },
			probeAudio:    playout.FFprobeAudioNextTo(set.str("playout.ffmpeg_path")),
			probeTracks:   playout.FFprobeTracksNextTo(set.str("playout.ffmpeg_path")),
			probeFormat:   playout.FFprobeFormatNextTo(set.str("playout.ffmpeg_path")),
			// Live read of `library.path_map` (§15, V47), parsed each call so a mapping edit
			// applies without a restart — the same hot-apply posture as audioLanguage.
			pathMap: func() library.PathMap { return library.ParsePathMap(set.str("library.path_map")) },
			log:     log,
			// ⚠ Set HERE, in the literal, rather than back-patched after the manager exists.
			// It is called UNGUARDED (playout.Resolve → r.activeChannels()), so a missing
			// assignment is a nil-func panic on the quality-ladder path — i.e. when a viewer
			// tunes in, which is the worst place to discover it. Verified: dropping the old
			// back-patch broke no test.
			//
			// There was never a construction cycle to break: the manager does not reference
			// the resolver, so the resolver simply had to be built AFTER it.
			activeChannels: playoutMgr.ActiveCount,
		}
		// A channel starting or stopping is a STRUCTURAL change the dashboard should see
		// immediately, so it rides the SSE bus (§8: the frame is the latency path, GET
		// /v1/playout/sessions is truth). Deliberately NOT fired per ffmpeg progress sample —
		// those arrive ~1/second per stream, and republishing each would push a handful of
		// frames per second at every open browser for numbers that move by fractions.
		//
		// The payload is the CHANNEL COUNT, not the full snapshot: this layer holds the bus
		// but not the API's telemetry shape, and a frame that says "something changed" is
		// enough to make the dashboard re-read the endpoint that owns the shape.
		playoutMgr.OnChange(func() {
			eventBus.Publish(events.Event{
				Type:    "playout",
				Payload: api.PlayoutEvent{Active: playoutMgr.ActiveCount()},
			})
		})
		playoutObserver = playoutMgr

		// The in-app HLS repackager shares the session manager's encoder (§9.1 Watch, V46): it
		// attaches to a channel like any other viewer and stream-copies the bytes into HLS. A
		// failure to create its scratch root is not fatal — the media-server streams work without
		// it — so log and leave the /playout/hls routes reporting "not running" rather than
		// refusing to boot.
		var liveHLS *playout.HLSManager
		if hlsMgr, herr := playout.NewHLSManager(
			playoutMgr, set.str("playout.ffmpeg_path"), set.str("playout.hls_dir"),
			playout.DefaultGrace, log,
		); herr != nil {
			log.Warn("internal playout: in-app HLS unavailable — browser playback disabled",
				"err", herr)
		} else {
			liveHLS = hlsMgr
			owner.addStop(func(context.Context) error {
				hlsMgr.Stop()
				return nil
			})
		}
		playoutResolverSvc = playoutRes

		// One-time broadcast-codec backfill (§9.1 V50). The migration defaults every existing
		// channel to h264 — a data migration runs in the store with no library access, so it
		// cannot probe. Channels bound AFTER this upgrade get their codec at bind; the ones that
		// predate it would otherwise stay h264 (and needlessly transcode an HEVC library down)
		// until their next re-curation. This pass recomputes each once, off the boot path.
		//
		// Async and best-effort: channels still play (as h264) while it runs, so a slow or
		// failing probe delays convergence, never startup. Idempotent — ComputeChannelCodec
		// writes the measured majority every time — so it needs no "already backfilled" marker
		// and re-running on a later boot is harmless bounded work.
		if st != nil {
			owner.goRun(func(ctx context.Context) {
				chans, lerr := st.ListChannels(ctx)
				if lerr != nil {
					log.Warn("playout: broadcast-codec backfill skipped (channel list failed)", "err", lerr)
					return
				}
				for _, ch := range chans {
					if ctx.Err() != nil {
						return // shutting down mid-backfill
					}
					if _, cerr := playoutRes.ComputeChannelCodec(ctx, ch.ID); cerr != nil {
						log.Debug("playout: broadcast-codec backfill for a channel failed",
							"channel", ch.ID, "err", cerr)
					}
				}
				log.Info("playout: broadcast-codec backfill complete", "channels", len(chans))
			})
		}

		// Wrap the resolver's capacity so the chosen HW-encode slot count is logged once, when the
		// admission gate first reads it — the operator-facing counterpart to "encoder probed".
		hwEncodeSlots = func(ctx context.Context) int {
			n := playoutRes.HWEncodeSlots(ctx)
			log.Info("playout: hardware encode admission", "hw_slots", n)
			return n
		}
		encodePool = media.NewEncodePool(func() int {
			if playout.Encoder(set.str("playout.encoder")) == playout.EncoderSoftware {
				return 0 // an explicit software choice must not start hardware preparation.
			}
			return hwEncodeSlots(rootCtx)
		})

		// Prepared playout is persistent control-plane work feeding the SAME Origin as the live
		// fallback. Construction may fail on an unwritable volume without taking live TV down; the
		// task remains visible with the exact reason instead of silently disappearing.
		var preparedOrigin *playout.PreparedOrigin
		preparedLibrary, preparedErr := prepared.NewLibrary(set.str("playout.prepared_dir"))
		if preparedErr != nil {
			reason := "the prepared media directory is unavailable: " + preparedErr.Error()
			log.Warn("playout: prepared media unavailable — live fallback remains active", "err", preparedErr)
			planner := prepared.NewPlanner(prepared.PlannerDependencies{
				Pool: encodePool, Now: time.Now, Log: log, UnavailableReason: reason,
			})
			preparedObserver = planner
			jobReg.Add(preparedPlayoutJob(planner, reason))
		} else {
			readiness, readinessErr := prepared.OpenReadiness(preparedLibrary)
			if readinessErr != nil {
				log.Warn("playout: prepared readiness index unavailable — live fallback remains active", "err", readinessErr)
			}
			packager := prepared.NewFFmpegPackager(
				set.str("playout.ffmpeg_path"),
				func(contract prepared.RenditionContract) (prepared.VideoPlan, error) {
					encoder := playout.Encoder(set.str("playout.encoder"))
					if encoder == "" {
						encoder = playoutRes.detectedEncoder(rootCtx)
					}
					return playout.PreparedVideoArgs(encoder, contract)
				},
			)
			preparer := prepared.NewPreparer(prepared.PreparerDependencies{
				Library: preparedLibrary, Packager: packager, Readiness: readiness,
			})
			preparedRuntime := newPreparedRuntimeResolver(preparedRuntimeDependencies{
				Channels: st, Timeline: playoutRes, Inputs: lib, Lookup: preparer,
				Now: time.Now, Readiness: readiness,
				PathMap: func() library.PathMap { return library.ParsePathMap(set.str("library.path_map")) },
				Policy: func() string {
					return preparedSourcePolicy(
						set.str("playout.quality_tier"),
						set.str("playout.audio_language"),
						set.str("library.path_map"),
					)
				},
				GlobalBackendContext:    appliedBackendContext,
				TransportBackendContext: transportBackendContext,
				Rendition: func() prepared.RenditionContract {
					return playout.CanonicalPreparedRendition(
						playout.TierFor(set.str("playout.quality_tier")),
					)
				},
			})
			planner := prepared.NewPlanner(prepared.PlannerDependencies{
				Resolver: preparedRuntime, Preparation: preparer, Pool: encodePool,
				Retainer: preparedLibrary,
				BudgetBytes: func() int64 {
					return preparedBudgetBytes(set.intv("playout.prepared_budget_gb"))
				},
				Now: time.Now, Log: log,
			})
			preparedObserver = planner
			jobReg.Add(preparedPlayoutJob(planner, ""))
			preparedOrigin = playout.NewPreparedOrigin(preparedLibrary, preparedRuntime)
		}
		var lifecycleGate *playoutAdmissionGate
		// Every transport hop uses one durable eligibility decision, including SQLite's raw
		// playlist/program chain. Postgres additionally closes the process-wide listener gate
		// whenever notification continuity cannot be proved.
		backendView = backendtransition.NewDurableView(st)
		durablePlayoutEligibility := func(ctx context.Context, channelID string) (bool, error) {
			return durableInternalTransportPlayable(ctx, st, backendView, channelID)
		}
		if store.DialectOf(st) == store.DialectPostgres {
			lifecycleGate = &playoutAdmissionGate{}
		}
		origin := playout.NewOrigin(playout.OriginDependencies{
			Prepared: preparedOrigin, LiveSessions: playoutMgr, LiveHLS: liveHLS,
			Available: func() bool {
				return lifecycleGate == nil || lifecycleGate.Available()
			},
			Eligible: durablePlayoutEligibility,
		})
		playoutSvc = origin

		// The durable backend checkpoint is initialized synchronously before any request can
		// observe its runtime gates. Initialize performs store I/O only; fleet and media-server
		// work is retried by settings writes and channel maintenance.
		backendURLs := func(ctx context.Context, target string) (setup.LiveTVURLs, error) {
			tok, err := readGeneratedSecret(ctx, settings.SecretPlayout)
			if err != nil {
				return setup.LiveTVURLs{}, fmt.Errorf("read playout token for backend publication: %w", err)
			}
			return setup.LiveTVURLsFor(
				target, set.str("tunarr.url"), set.str("server.public_url"), tok,
			), nil
		}
		builtBackendController, err := buildBackendTransition(rootCtx, backendTransitionDependencies{
			store: st, fleet: channelEngine,
			publisher: &backendPublisher{connector: liveTVConnector, urls: backendURLs},
			cutover:   inheritedInternalCutover{channels: st, playout: playoutSvc},
			desired:   resolveDesiredBackend,
		})
		if err != nil {
			return nil, err
		}
		backendController = builtBackendController
		if lifecycleGate != nil {
			lifecycle := &postgresPlayoutLifecycle{
				store: st, checkpoint: backendView, origin: origin, gate: lifecycleGate, log: log,
			}
			if err := lifecycle.StartTracked(rootCtx, owner); err != nil {
				return nil, fmt.Errorf("start postgres playout lifecycle: %w", err)
			}
		}
		// The ladder inputs (tier/encoder/capacity/activeChannels) are called UNGUARDED by
		// Profile, so leaving one unset is a panic when a viewer tunes in. `Profile` is
		// invoked by the spawner rather than over HTTP. Build captures the concrete resolver
		// on the returned generation, which lets package tests assert the real wiring without
		// mutable package state crossing concurrent builds.
		capturePlayoutResolver(playoutRes)
		playoutGuideSvc = playoutRes
		// A live encoder never exits on its own (playout/process.go), so shutdown MUST tear
		// them down explicitly or they outlive the process that started them.
		owner.addStop(func(context.Context) error {
			playoutMgr.Stop()
			return nil
		})
		log.Info("internal playout registered",
			"ffmpeg", set.str("playout.ffmpeg_path"), "max_channels_cap", set.intv("playout.max_channels"))

		// server.public_url is the base a media client dials for HLS segments. Internal playout
		// is the default backend, and an unset public URL makes channels appear in the guide but
		// fail at tune time (playout.go returns 503). The first-run wizard requires this (#387),
		// but an env-only, restored-DB, or API-driven install bypasses the wizard entirely and
		// otherwise gets no signal until a viewer hits the dead channel. Warn at boot so the
		// operator sees it in the logs, not in a support ticket. (§9.1; beta-readiness D-4.)
		if internalPlayoutNeedsPublicURL(set.str("server.public_url")) {
			log.Warn("internal playout: server.public_url is unset — channels will appear in the guide but fail at tune time; set SERVER_PUBLIC_URL (or server.public_url) to this instance's reachable base URL")
		}

		chEvery := set.dur("channel.reconcile_every")
		// The channel sweep is a scheduler job now (§18.1) — same desired-vs-actual Sweep
		// (with its own ClaimDueChannels lease), driven by the shared heartbeat. The Runner's
		// lease/batch are still constructed; only its standalone loop is gone.
		chSweep := channels.NewRunner(channelEngine, st, chEvery, 2*chEvery, 50, time.Now, log)
		jobReg.Add(channelMaintenanceJob(chSweep, episodeRefresh, func(ctx context.Context) error {
			return backendController.ApplyCurrent(ctx, resolveDesiredBackend)
		}))
		log.Info("channel scheduler registered", "tunarr", set.str("tunarr.url"), "sweep_every", chEvery)
	}

	// The channel binder (§7) owns the approval coordinator's read-only channel plan
	// and post-commit runtime consequences. The coordinator below is the ONE public
	// approval operation shared by HTTP and automatic paths; callers cannot split
	// acquisition persistence from local channel materialization. The binder needs
	// only the store, so it's constructed whenever one is open;
	// channelSvc (if the scheduler/Tunarr block above wired it) satisfies
	// binder.Reconciler directly — same Reconcile(ctx, channelID) signature. If
	// channels isn't configured, channelSvc is a nil interface and the binder's
	// nil-guard just skips the immediate reconcile push (same best-effort
	// behavior createChannel/approveProposal always had).
	var chBinder *binder.Binder
	var proposalApprover *suggest.Approver
	if st != nil {
		var rec binder.Reconciler
		if channelSvc != nil {
			rec = channelSvc
		}
		// The playout resolver doubles as the CodecComputer (§9.1 V50): it owns library
		// resolution + ffprobe, so it measures + stores the channel's broadcast codec after a
		// bind. Assigned only when playout is wired — passing a typed-nil *playoutResolver as a
		// non-nil interface would defeat the binder's nil-guard and panic on the first bind, so
		// it stays a nil interface unless playoutRes actually exists.
		var codec binder.CodecComputer
		if playoutRes != nil {
			codec = playoutRes
		}
		chBinder = binder.New(st, rec, codec, log)
		// A failed FIRST reconcile leaves a channel that exists, has a lineup, and shows a full
		// schedule in the guide but has never reached Tunarr (§9 V54). Give that failure a durable
		// home rather than one log line: the recorder is the mechanism that already survives a
		// restart and surfaces on the Dashboard. Typed-nil guard, exactly as `codec` above needs.
		if activityRec != nil {
			chBinder = chBinder.WithActivity(activityRec)
		}
		// Numbering must see the numbers TUNARR already uses, not just Loomarr's own (§9 V54):
		// Tunarr accumulates channels from earlier installs and reset databases, and picking one
		// of those numbers makes the create fail forever.
		if chanNumbers != nil {
			chBinder = chBinder.WithChannelNumbers(chanNumbers)
		}
		// One deep gate owns proposal edits, acquisition records, and the intent-bound
		// channel commit. HTTP, per-user auto-approval, and auto-curate all receive this
		// same instance, so composition cannot accidentally split local materialization.
		proposalApprover = suggest.NewApprover(st, chBinder, time.Now)
	}

	suggestions, err := buildSuggestions(
		rootCtx, st, set, ov, eventBus, emitter, jobReg, fillerLayout, activityRec, log,
		libraryClient, tmdbClient, channelSvc, proposalApprover, owner,
	)
	if err != nil {
		return nil, err
	}
	suggestSvc, proposalWorkflow := suggestions.suggest, suggestions.workflow
	searchSvc, collectionsSvc := suggestions.search, suggestions.collections
	systemLLM, iconSvc := suggestions.systemLLM, suggestions.icons
	imageSvc := suggestions.images
	timelineThumbs := suggestions.timelineThumbs

	fillers := buildFillerSubsystem(
		st, set, fillerLayout, log, libraryClient, eventBus, emitter, jobReg, playoutRes, channelSvc,
	)
	fillerSvc, podPreview := fillers.service, fillers.preview

	// Scheduled backups with rotation (§16, §18.1, V12) — the consumer `backup.schedule`
	// and `backup.retain` were declared without in V4. Writes one snapshot into
	// `backup.dir`, then prunes to the newest `backup.retain`.
	//
	// ⚠ On Postgres this registers as a DISABLED job rather than not registering at all.
	// `WriteBackup` is SQLite-only by design (Postgres has mature backup tooling and an
	// operator running it already has a strategy) — but an omitted row is indistinguishable
	// on the Tasks page from a job that runs fine and has never failed, and for backup that
	// ambiguity means believing you are covered when you are not. So the row is present and
	// states why it cannot run. It is never scheduled and Run-now 409s (§18.1).
	var backupsSvc api.BackupsService
	if w := store.BackupWriter(st); w != nil {
		bs := &backupsService{
			w:      w,
			dir:    func() string { return set.str("backup.dir") },
			retain: func() int { return set.intv("backup.retain") },
			sched:  func() string { return set.str("backup.schedule") },
		}
		backupsSvc = bs
		jobReg.Add(bs.Job(log))
	} else if st != nil {
		// No writer, but there IS a store — i.e. Postgres. (A store-less boot registers
		// nothing at all, since there is no scheduler to list it on.) The job is still
		// LISTED, carrying why it cannot run — see unavailableBackupJob.
		jobReg.Add(unavailableBackupJob(
			"Loomarr does not back up PostgreSQL itself — use pg_dump on your usual schedule."))
	}

	// Restart control (§9.2, V13). Wired only when main passed a restart func — a handler
	// built by a test or the integration harness has no generation loop behind it, and
	// 501 is the honest answer there rather than a button that silently does nothing.
	var restartSvc api.RestartService
	if ov.Restart != nil {
		restartSvc = restartAdapter{fn: ov.Restart}
	}
	// What this generation booted with, for the RestartRequired derivation. A load
	// failure leaves it nil, which reports no drift — the safe direction, since a false
	// "restart required" points the operator at an action that cannot help.
	bootCfg, cfgErr := config.Load()
	if cfgErr != nil {
		log.Warn("could not read boot config for restart-required detection", "err", cfgErr)
		bootCfg = nil
	}

	// API server (§7): titles + ops + OpenAPI/docs + backup + auth/users (§11).
	// Session auth (cookie) with API_TOKEN Bearer break-glass; SQLite gets a
	// backup streamer, Postgres passes nil (→ 501).
	var backup api.BackupStreamer
	if st != nil {
		if b := store.SQLiteBackuper(st); b != nil {
			backup = b
		}
	}
	apiToken := ""
	if secrets != nil {
		apiToken = secrets.Value(settings.SecretAPI)
	}
	authorizer := api.Authorizer(api.NewTokenAuthorizer(apiToken))
	var loginSvc api.LoginService
	var ssoSvc api.SSOService
	var sessMgr api.SessionManager
	var userSync api.UserSyncer
	var provisionSvc api.Provisioner
	var passwordSvc api.PasswordService
	var deviceMgr *auth.DeviceManager
	var deviceLimiter *auth.RateLimiter
	if st != nil {
		// Auth wires on the STORE alone (§11 rework): identity is Loomarr-owned, so
		// bootstrap + local login work with zero media-server config. The media-server
		// verifier and user lister stay present while unconfigured; the dynamic client
		// fails closed until the complete live triple exists.
		lib := libraryClient
		mgr := auth.NewManager(st, set.dur("session.ttl"), time.Now)
		limiter := auth.NewRateLimiter(0.2, 5) // ~5 attempts, refill 1/5s (§11)
		loginSvc = auth.NewLoginService(lib, st, mgr, limiter, time.Now)
		userSync = auth.NewUserSync(lib, st, time.Now)
		provisionSvc = auth.NewProvisioner(st, lib, newID, time.Now)
		// SSO: the third credential path (§11, D-F, V8). Config is read PER CALL so a saved
		// change applies without a restart (config-design §3).
		//
		// ⚠ The redirect URL is DERIVED from `server.public_url` rather than configured
		// separately. Two places to state one address is two places for it to disagree, and
		// a mismatched redirect is the most common OIDC misconfiguration — the provider
		// refuses and neither side says which value it expected.
		ssoSvc = auth.NewSSOService(func() auth.SSOConfig {
			base := strings.TrimRight(set.str("server.public_url"), "/")
			redirect := ""
			if base != "" {
				redirect = base + "/v1/auth/sso/callback"
			}
			return auth.SSOConfig{
				Enabled:      set.boolv("auth.sso.enabled"),
				Issuer:       set.str("auth.sso.issuer"),
				ClientID:     set.str("auth.sso.client_id"),
				ClientSecret: set.str("auth.sso.client_secret"),
				RedirectURL:  redirect,
			}
		}, st, mgr, time.Now, log)

		// Local account management (§11) needs no media server — a local user is
		// verified entirely in-app, exactly like the bootstrap admin. So it is wired
		// unconditionally alongside the store, not inside the `lib != nil` branch.
		passwordSvc = auth.NewPasswordService(st, newID, time.Now)
		sessMgr = mgr
		// Device pairing (§11, Shield P1) — the credential path for a client with no keyboard.
		// Wired alongside local accounts and for the same reason: it needs no media server, since a
		// device inherits the role of the Loomarr user who approved it.
		deviceMgr = auth.NewDeviceManager(st, time.Now)
		// Deliberately tighter than the login limiter: a login attempt is a human retyping a
		// password they know, while a pairing-code attempt is a guess at a short code. Five tries
		// refilling at one per twenty seconds leaves normal use untouched and makes grinding the
		// keyspace hopeless within the ten-minute window a code lives for.
		deviceLimiter = auth.NewRateLimiter(0.05, 5)
		authorizer = api.NewSessionAuthorizerCurrent(mgr, deviceMgr, func(ctx context.Context) (string, error) {
			return readGeneratedSecret(ctx, settings.SecretAPI)
		})
	}

	// The playout device token (§11). A FUNC, not a captured value, so a REGENERATED token
	// takes effect immediately — rotation is an operator action the UI offers, and a value read
	// once at boot would keep authorizing the old token until restart.
	//
	// Nil when secrets are unavailable (no store), which makes the playout routes fail CLOSED:
	// serving streams unauthenticated because a secret could not be minted would silently
	// remove the only auth those routes have.
	var playoutSecret func() string
	var playoutSecretCurrent func(context.Context) (string, error)
	if secrets != nil {
		playoutSecret = func() string { return secrets.Value(settings.SecretPlayout) }
		playoutSecretCurrent = func(ctx context.Context) (string, error) {
			return readGeneratedSecret(ctx, settings.SecretPlayout)
		}
	}

	// The settings API surface (config-design §8): wired when the store (hence the
	// settings service) is up. Connection Test probes reuse the live adapters.
	// The guide reader powers /v1/channels/now-next and …/{id}/upcoming.
	//
	// ⚠ ROUTED PER CHANNEL, by the backend that actually streams it (§9.1). Reading Tunarr's
	// guide for a channel Loomarr plays internally is a real bug and was a shipped one: the
	// channel keeps its `tunarr_id`, Tunarr keeps generating listings, and the card confidently
	// reports a DIFFERENT programme than the grid and the XMLTV the television reads — measured
	// ~30 minutes apart on the dev install. See nowNextRouter.
	//
	// The Tunarr half still reads LIVE config at the start of each operation, so saving a new
	// Tunarr URL takes effect without a restart (config-design §3 hot-apply). A 2h window is
	// comfortably longer than any single program, so "next" is always present.
	var guideSvc api.GuideReader
	if st != nil {
		// Always construct the dynamic adapter, even when Tunarr is unconfigured at boot.
		// Its config resolves per operation, so adding tunarr.url later hot-applies; while
		// empty, reads fail softly through nowNextRouter's existing no-guide behaviour.
		var tunarrGuide tunarrGuideReader = guideAdapter{
			tunarr: programmer.NewDynamic(set.tunarrConfig()),
			window: 2 * time.Hour,
		}
		// playoutRes is nil when internal playout is not wired; the router then has no reader
		// for internal channels and gives them no entry — never a fallback to Tunarr's guide,
		// which is precisely the defect this replaces.
		var internalGuide broadcastReader
		if playoutRes != nil {
			internalGuide = playoutRes
		}
		guideSvc = nowNextRouter{
			tunarr:   tunarrGuide,
			internal: internalGuide,
			channels: st,
			// The same nil-means-inherit precedence playoutChannels uses (§15): a channel's own
			// policy.playout.backend wins, else the durable applied global. A prepared target
			// cannot leak into the next render before publication.
			appliedBackend: appliedBackendContext,
			window:         2 * time.Hour,
		}
	}

	var settingsSvc api.SettingsService
	if st != nil && secrets != nil {
		settingsSvc = settingsAdapter{
			svc: set.svc, secrets: secrets, store: st, log: log,
			tests: connectionTests(desiredSet, libraryClient, tmdbClient), refreshRedactor: refreshSecretRedactor,
			readSecret: readGeneratedSecret,
		}
	}

	// liveConfig lets the always-constructed feature routes gate on the CURRENT
	// config (config-design §3 hot-apply): a saved connection enables the route with
	// no restart (§8.1). Only when the settings service exists (a store is open).
	var liveConfig func(key string) string
	var libraryConfigured func() bool
	if st != nil {
		liveConfig = set.str
		libraryConfigured = set.libraryConfigured
	}

	// Build + start the job scheduler once every subsystem has registered its jobs (§18.1).
	// Each job's CRON schedule resolves from its settings key (falling back to the job's
	// default cron), so a changed schedule hot-applies on the next tick. The notifier emits a
	// `job` SSE frame on each state change so the Tasks page updates live. Only with a store.
	var jobsSvc api.JobService
	if st != nil {
		sched := scheduler.New(st, jobReg, func(key, def string) string {
			if c := set.str(key); c != "" {
				return c
			}
			return def
		}, time.Now, log).WithNotifier(emitter)
		// Seed the state rows the Tasks page reads, then hand execution to River (§14, §18.1).
		sched.SeedRegistry(rootCtx)
		if pool := store.PoolOf(st); pool != nil {
			stop, err := sched.StartRiver(rootCtx, st, pool, log)
			if err != nil {
				// ⚠ A failure here is fatal to the FEATURE, not to the process: without the
				// queue nothing runs on a schedule, and the Tasks page must say so rather
				// than list jobs whose next-run time is fiction. Logged loudly; the page
				// still renders the rows with their real (stale) state.
				log.Error("scheduler: River did not start — no job will run on its schedule", "err", err)
			} else {
				owner.addStop(stop)
				// ⚠ StartRiver already stops the client when rootCtx is cancelled; this waits
				// for that to FINISH before Application.Shutdown lets the store close. River's shutdown issues
				// queries, so a pool closed underneath it strands its goroutines — measured at
				// 4 leaked per generation, which the §9.2 restart loop's goleak test caught.
			}
		} else {
			log.Error("scheduler: store exposes no pool — no job will run on its schedule")
		}
		jobsSvc = jobsAdapter{s: sched}
	}

	// Database migration stepper (§18, V11). Keep the status service on Postgres so the
	// browser polling across an atomic cutover can observe the new backend. Mutations
	// fail closed there, while backup.dir remains hot-applied for SQLite.
	var databaseSvc api.DatabaseService
	if st != nil {
		dataDir := ""
		if store.DialectOf(st) == store.DialectSQLite {
			dataDir = filepath.Dir(store.SQLitePath(st))
		}
		databaseSvc = newDatabaseService(st, dataDir,
			func() string { return set.str("backup.dir") }, eventBus).
			WithMigrationRequest(ov.DatabaseMigration).
			WithLastError(ov.DatabaseMigrationError)
	}

	// residentLLMVRAMFn — the TRUE resident-VRAM reading (§9.1 V47): ask Ollama /api/ps what is
	// actually loaded now, rather than estimating from on-disk size. Extracted to a named var (not an
	// inline opts field) because TWO consumers need it: the doctor's GPU header (below) AND the
	// admission budget's VRAM shading (V49) — the manager was built earlier with a late-bound hook
	// (residentVRAM), which we now point at this. Local ollama only; a hosted provider holds no local
	// VRAM → (0, ""). Read LIVE so a provider/URL change hot-applies.
	residentLLMVRAMFn := func(ctx context.Context) (float64, string) {
		if set.str("llm.provider") != "ollama" {
			return 0, ""
		}
		url := set.str("llm.url")
		if url == "" {
			return 0, ""
		}
		resident, err := llm.NewOllama(url, set.str("llm.model")).ListResident(ctx)
		if err != nil {
			log.Debug("playout doctor: /api/ps residency probe failed", "err", err)
			return 0, ""
		}
		var total float64
		var largest llm.ResidentModel
		for _, m := range resident {
			total += m.VRAMGiB
			if m.VRAMGiB > largest.VRAMGiB {
				largest = m
			}
		}
		return total, largest.Name
	}
	// Point the admission budget's late-bound VRAM hook at the real getter now that it exists, so a
	// resident model shades the transcode budget down (V49). Guarded: playout may be unwired.
	if playoutRes != nil {
		residentVRAM = residentLLMVRAMFn
	}

	handler := api.Router(log, api.Options{
		Store:            st,
		Auth:             authorizer,
		Log:              log,
		BackupSQLite:     backup,
		Ready:            ready,
		Login:            loginSvc,
		Sessions:         sessMgr,
		Passwords:        passwordSvc,
		UserSync:         userSync,
		Devices:          deviceMgr,
		DeviceLimiter:    deviceLimiter,
		CookieSecure:     set.str("cookie.secure"),
		TrustProxy:       set.boolv("security.trust_proxy"),
		DevLogin:         ov.DevLogin,
		Pprof:            ov.Pprof,
		Channels:         channelSvc,
		LiveTV:           liveTVSvc,
		TunerRescanner:   tunerRescanner,
		TunarrConnect:    tunarrConnectSvc,
		Suggest:          suggestSvc,
		ProposalWorkflow: proposalWorkflow,
		Search:           searchSvc,
		Collections:      collectionsSvc,
		Icons:            iconSvc,
		Images:           imageService(imageSvc),
		Events:           eventBus,
		Shutdown:         rootCtx.Done(),
		Filler:           fillerSvc,
		Pods:             podPreview,
		SystemLLM:        systemLLM,
		Database:         databaseSvc,
		Backups:          backupsSvc,
		SSO:              ssoSvc,
		Restart:          restartSvc,
		Activity:         activityRec,
		// The baseline for "has a restart-scoped setting changed?" is what THIS
		// generation booted with, captured here rather than per call (config-design §3).
		RestartDrift: restartDrift(bootCfg, appliedRestartSettings, canonicalRestartCurrent(desiredSet)),
		Jobs:         jobsSvc,
		Settings:     settingsSvc,
		BackendTransition: currentBackendTransition{
			controller: backendController, refresh: refreshBackendSettings, desired: desiredBackend,
		},
		Guide:             guideSvc,
		Provision:         provisionSvc,
		Approver:          proposalApprover,
		Binder:            chBinder,
		FillerLayout:      fillerLayout,
		LiveConfig:        liveConfig,
		LibraryConfigured: libraryConfigured,
		BackendCheckpoint: func(ctx context.Context) (api.BackendCheckpoint, error) {
			snapshot, err := checkpointSnapshot(ctx)
			return api.BackendCheckpoint{
				Applied: snapshot.Applied, Prepared: snapshot.Prepared,
				PublishedInternal: snapshot.PublishedInternal,
			}, err
		},
		LiveConfigInt: set.intv,
		// boolOn (not boolv): the API reads bool keys that are ON by default, where an
		// unanswerable read must fail open — see Options.LiveConfigBoolOn.
		LiveConfigBoolOn: set.boolOn,
		// Internal playout (§9.1). PlayoutSecret is a FUNC so a regenerated token takes
		// effect without a restart (§11 rotation).
		PlayoutObserver:  playoutObserver,
		PreparedObserver: preparedObserver,
		// The in-app HLS repackager for the Watch surface (§9.1, V46). Nil ⇒ /playout/hls 501s.
		Playout:         playoutSvc,
		PlayoutResolver: playoutResolverSvc,
		// The XMLTV guide reads the same resolver, so listings cannot drift from playout.
		PlayoutGuide:         playoutGuideSvc,
		TimelineThumbs:       timelineThumbs,
		PlayoutSecret:        playoutSecret,
		PlayoutSecretCurrent: playoutSecretCurrent,
		// Bound to the ffmpeg path once, like probeAudio above: the answer depends on the
		// BUILD, so it cannot change without the binary changing. Memoised inside, so the
		// `-filters` exec happens on the first offline card and never again.
		PlayoutFont:    playout.CardFontFor(set.str("playout.ffmpeg_path")),
		PlayoutTonemap: playout.TonemapperFor(set.str("playout.ffmpeg_path")),
		// Free GPU memory for the hardware encoders by evicting the resident local LLM (§8.2, §9.1
		// V47). Built on demand and read LIVE, so a provider/model change hot-applies and eviction
		// always targets whatever model is currently resident. Only the local ollama provider holds
		// VRAM on this box — a hosted provider has nothing local to reclaim, so it is a no-op there
		// (the ladder then goes straight to the software fallback).
		ReclaimVRAM: func(ctx context.Context) {
			if set.str("llm.provider") != "ollama" {
				return // hosted: nothing local to evict
			}
			url := set.str("llm.url")
			if url == "" {
				return
			}
			p := llm.NewProvider("ollama", url, set.str("llm.model"), "")
			ev, ok := p.(llm.Evictor)
			if !ok {
				return
			}
			if err := ev.Evict(ctx); err != nil && !errors.Is(err, llm.ErrNothingToEvict) {
				log.Debug("playout: LLM eviction failed (encode will fall back to software)", "err", err)
			}
		},
		// The doctor's TRUE resident-VRAM reading (§9.1 V47), extracted to residentLLMVRAMFn above so
		// the admission budget's VRAM shading (V49) shares the exact same source.
		ResidentLLMVRAM: residentLLMVRAMFn,
		// The same pool is consumed by live playout here and by the prepared-media planner. Live work
		// has foreground priority; preparation is cancellable and cannot consume the final slot.
		EncodePool: encodePool,
		PlayoutEncoder: func(
			ctx context.Context, args []string, onProgress func(playout.Progress),
		) (*playout.Process, error) {
			// `onProgress` was nil here since the supervisor was written, so ffmpeg's parsed
			// progress samples were discarded every time. Passing it through is what makes the
			// dashboard's encoder speed measured rather than invented (V16).
			return playout.Start(ctx, set.str("playout.ffmpeg_path"), args, log, onProgress)
		},
	})
	// SQLite is single-process by contract, so its settings snapshot changes only
	// through this process's writes and needs no polling reads. Postgres permits
	// ordinary replicas; one cancellable refresher per application generation keeps
	// their complete runtime snapshot within the documented ~30-second bound.
	trackReplicaSettingsRefresh(owner, store.DialectOf(st), set.svc,
		replicaSettingsRefreshInterval, refreshSecretRedactor)
	return handler, nil
}
