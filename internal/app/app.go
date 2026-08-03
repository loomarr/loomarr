// Package app is the composition root: it wires every subsystem from an open
// store into the API handler that cmd/loomarr serves and the integration tests
// drive. Keeping it importable (not package main) is what lets tests exercise
// the REAL wiring end to end (§21), and keeps cmd/loomarr a thin entrypoint.
package app

import (
	"context"
	"io/fs"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/mantonx/loomarr/internal/activity"
	"github.com/mantonx/loomarr/internal/api"
	"github.com/mantonx/loomarr/internal/auth"
	"github.com/mantonx/loomarr/internal/binder"
	"github.com/mantonx/loomarr/internal/catalog"
	"github.com/mantonx/loomarr/internal/channels"
	"github.com/mantonx/loomarr/internal/clipfetch"
	"github.com/mantonx/loomarr/internal/config"
	"github.com/mantonx/loomarr/internal/events"
	"github.com/mantonx/loomarr/internal/filler"
	"github.com/mantonx/loomarr/internal/library"
	"github.com/mantonx/loomarr/internal/llm"
	"github.com/mantonx/loomarr/internal/metrics"
	"github.com/mantonx/loomarr/internal/playout"
	"github.com/mantonx/loomarr/internal/programmer"
	"github.com/mantonx/loomarr/internal/reconcile"
	"github.com/mantonx/loomarr/internal/recurate"
	"github.com/mantonx/loomarr/internal/retention"
	"github.com/mantonx/loomarr/internal/schedule"
	"github.com/mantonx/loomarr/internal/scheduler"
	"github.com/mantonx/loomarr/internal/settings"
	"github.com/mantonx/loomarr/internal/setup"
	"github.com/mantonx/loomarr/internal/store"
	"github.com/mantonx/loomarr/internal/suggest"
	"github.com/mantonx/loomarr/internal/tmdb"
)

// Overrides injects the two in-process boundaries (the Tunarr push target and the
// LLM provider) for tests. Both nil ⇒ the real URL-built adapters (production).
type Overrides struct {
	Programmer programmer.Programmer // nil ⇒ programmer.NewDynamic(tunarr.url)
	LLM        llm.Provider          // nil ⇒ the Swappable from buildLLM
	// TMDB overrides the grounding/validation client. tmdb.New uses a fixed base
	// (api.themoviedb.org), so unlike library/seerr it isn't settings-routable to a
	// test double; tests pass tmdb.NewWithBase(double.URL, key) here to stay offline.
	TMDB *tmdb.Client // nil ⇒ tmdb.New(tmdb.api_key)
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
}

// flavorOrDefault resolves the media-server flavor, defaulting to Emby when unset.
// The feature services are always-constructed (so a saved connection enables their
// routes live, §8.1) and the library adapter fixes its flavor at construction — so
// an unconfigured install defaults to Emby's auth shape. CHANGING the flavor (e.g.
// to Jellyfin) still needs a restart; url/token/etc. hot-apply. (Follow-up: make
// the library flavor a live closure to drop that last caveat.)
func flavorOrDefault(set resolved) library.Flavor {
	if f, err := library.ParseFlavor(set.str("library.flavor")); err == nil {
		return f
	}
	return library.Emby
}

// BuildHandler wires every subsystem from the already-open store + logger and
// returns the fully-configured API handler (all api.Options). Background goroutines
// start under ctx. It is the seam run() (production) and the integration harness
// (tests) both call — so tests drive the REAL composition, not a copy. It does not
// open/close the store, read env, listen, or handle signals; those stay in run().
//
// # Why this is one long function
//
// It is ~630 lines and stays that way deliberately. Decomposing it into methods on a shared
// builder struct was tried on paper and rejected: the sections are sequential and genuinely
// interdependent (three back-patches — emitter.setEngine, playoutRes.activeChannels,
// playoutRes.pods), so splitting them would convert ~70 locals into fields on a mutable
// carrier. That WIDENS their scope rather than narrowing it, and trades compile-time
// use-before-assignment errors for runtime nils. A composition root is allowed to be long; it
// is not allowed to be unnavigable, which is what the section map below is for.
//
// # The nil-store path is deliberate
//
// Most sections sit inside `if st != nil`. That guard is not defensive habit: a container
// started without DATABASE_URL must still build a handler and answer /readyz with the reason,
// rather than crash-looping past the probe that would explain the problem. See
// TestBuildHandlerWithoutStoreServesReadinessInsteadOfPanicking — the nesting is the price of
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
//
// lastPlayoutResolver records the resolver the most recent BuildHandler constructed, for
// the package's own tests. Never read by production code — see the note at its assignment.
var lastPlayoutResolver *playoutResolver

func BuildHandler(rootCtx context.Context, st store.Store, log *slog.Logger, ov Overrides) (http.Handler, error) {
	// Readiness is true only once the store is connected + migrated (§17).
	ready := func() (bool, string) {
		if st == nil {
			return false, "no store configured"
		}
		if _, err := st.GetSetting(context.Background(), "healthcheck_probe"); err != nil && err != store.ErrNotFound {
			return false, "store unreachable: " + err.Error()
		}
		return true, "ok"
	}

	// State gauges for /metrics (§17): read from the store on scrape. Non-fatal —
	// a registration failure just omits these gauges, it never blocks boot.
	if st != nil {
		if err := metrics.RegisterStoreCollector(st, time.Now); err != nil {
			log.Warn("metrics: store collector not registered", "err", err)
		}
	}

	// Settings subsystem (config-design §3): once the store is open, build the
	// registry + resolution service (env pins validated → boot error), the
	// generated secrets, and the redactor. Every subsystem below reads config
	// through `set` (env > db > default, hot-applying) instead of raw cfg fields,
	// and connection adapters take snapshot-backed closures so a saved URL/token
	// takes effect on the next call with no restart. Without a store we can't
	// resolve DB overrides, so fall back to env-only defaults via a store-less
	// service is out of scope here — the app already isn't ready without a store.
	var set resolved
	var secrets *settings.Secrets
	if st != nil {
		r, sec, _, slog2, serr := bootSettings(context.Background(), st, log)
		if serr != nil {
			return nil, serr // invalid env pin / ambiguous <VAR>+<VAR>_FILE — fail fast (§3)
		}
		set, secrets, log = r, sec, slog2
		slog.SetDefault(log)
	}

	// The event bus (§7 SSE) and the domain-event emitter (§4 inv. 2) are built up
	// front so both the webhook ingest handler and the provisioning reconciler can
	// fan their terminal transitions to the SAME sink. The emitter routes each
	// event to (a) the channel scheduler's backfill (OnAvailability → recompute +
	// push the affected channels) and (b) the bus (→ SSE frame for the UI). The
	// scheduler engine doesn't exist yet at this point, so the emitter takes it
	// later via setEngine; until then (and if the scheduler is unconfigured) events
	// still reach the bus. Events are a latency optimization — the channel sweep
	// reconciles regardless — so a nil engine is safe, never a correctness gap (§9).
	eventBus := events.NewBus()
	emitter := &eventEmitter{bus: eventBus}

	// The background-job scheduler (§18.1): all recurring work — reconcile, the channel
	// sweep, filler sync, session cleanup, and (later phases) the availability pollers —
	// registers here as a named job with a settings-driven interval, then runs under ONE
	// heartbeat loop that is also on-demand-triggerable ("Run now"). Jobs are appended to
	// this registry as each subsystem is wired below; the scheduler is built + started once
	// at the end. This replaces the previous scattering of bespoke time.Ticker goroutines.
	jobReg := scheduler.NewRegistry()

	// Dashboard activity feed (§12, V32). One recorder, passed to every subsystem that
	// records a line — nil-safe, so a store-less boot records nothing rather than needing a
	// guard at each write point.
	var activityRec *activity.Recorder
	if st != nil {
		activityRec = activity.New(st, log).WithNotifier(emitter)
	}

	// Provisioning reconciler (§7), registered as scheduler jobs (§18.1). Always
	// constructed given a store so acquisitions process the moment a requester is
	// configured — no restart (§8.1). Its dynamic seerr/library connections are empty
	// until configured; with nothing `wanted` it's a no-op. Session cleanup is its own
	// scheduler job now (was the janitor piggybacking the reconcile ticker).
	if st != nil {
		lib := library.NewDynamic(flavorOrDefault(set), set.libraryConn(), instanceDeviceID(rootCtx, st))
		req := set.requesterFor() // Seerr or direct Sonarr/Radarr, per requester.provider (§6)
		rec := reconcile.New(st, req, lib, emitter, reconcile.Config{
			RequestTTL: set.dur("request.ttl"), DownloadingTTL: set.dur("downloading.ttl"),
		}, time.Now, log)
		// The reconcile tick is now a scheduler job (§18.1) — same Tick logic, driven by the
		// shared heartbeat on a cron schedule instead of its own ticker.
		jobReg.Add(rec.Job())
		// Session cleanup is its own scheduler job now (was piggybacking the reconcile
		// ticker via the janitor).
		// Retention for the accumulating tables (§5, §18.1). The purge POLICY — what may be
		// deleted, in what order, after how long — lives in internal/retention; the store owns
		// only the SQL. Windows are read live, so a settings change applies on the next run.
		jobReg.AddAll(retention.New(st, retention.Windows{
			Proposals: func() time.Duration { return set.dur("proposals.retention") },
			Jobs:      func() time.Duration { return set.dur("jobs.retention") },
			Activity:  func() time.Duration { return set.dur("activity.retention") },
		}, time.Now, log).Jobs())

		jobReg.Add(auth.NewSessionSweeper(st).Job(time.Now))

		// Poll-based availability (§4, §18.1): the PRIMARY path a requested title reaches
		// `available`. The scan lists what the media server recently added and confirms any
		// in-flight title now present — the same LibraryConfirmed transition the reconciler's
		// deadline backstop applies, but continuous. `lib` (a *library.Client) satisfies
		// LibraryScanner. Incremental runs often (recent window); Full is the daily safety net.
		libScan := reconcile.NewLibraryScan(st, lib, emitter, set.dur("job.library_scan.lookback"), time.Now, log).
			WithActivity(activityRec)
		jobReg.AddAll(libScan.Jobs())

		// Keeps the cached series episode lists current (§5, §18.1) so the guide never
		// enumerates a show on the request path. Deliberately NOT folded into library-scan:
		// that job only correlates in-flight acquisitions and returns early when there are
		// none, so it would never revisit an already-available show — the exact set this
		// refreshes. Bounded to the shows channel lineups actually reference.
		epRefresh := reconcile.NewEpisodeRefresh(st, episodeResolver(lib), func() time.Duration {
			return set.dur("episodes.max_age")
		}, time.Now, log)
		jobReg.Add(epRefresh.Job())

		// Queue poller (§18.1) — one poll job, whichever requester is active. The direct arr
		// reads Sonarr/Radarr queues for real byte-progress; Seerr reads /media for a COARSE
		// status (Downloading / Partly available, no percentage). Both promote grabbed titles
		// requested→downloading and persist what they know; availability stays the library
		// scan's job. The two branches are mutually exclusive (a provider is arr XOR seerr), so
		// their distinct job names never collide in the registry.
		if arr := set.arrRequester(); arr != nil {
			queuePoll := reconcile.NewQueuePoll(st, arr, emitter, set.dur("downloading.ttl"), time.Now, log)
			jobReg.Add(queuePoll.Job("arr-queue-poll", "Poll Sonarr/Radarr downloads",
				"Asks Sonarr and Radarr how your in-progress downloads are doing, so the queue shows real progress and stalled grabs are visible."))
		} else if seerr := set.seerrRequester(); seerr != nil {
			queuePoll := reconcile.NewQueuePoll(st, seerr, emitter, set.dur("downloading.ttl"), time.Now, log)
			jobReg.Add(queuePoll.Job("seerr-queue-poll", "Poll Seerr acquisition status",
				"Asks Seerr which requested titles are still being fetched. Seerr reports a coarse status rather than a percentage."))
		}
	}

	// Scheduler + Tunarr (§9, Phase 10): the channel reconcile engine + periodic
	// sweep, plus the Live TV wiring connector (guide-refresh poker). Wired when a
	// store, Tunarr, and the media-server library are configured.
	var channelSvc api.ChannelService
	var liveTVSvc api.LiveTVService
	var tunarrConnectSvc api.TunarrConnector
	// Internal playout (§9.1). Nil until wired below, which keeps the routes reporting "not
	// running" rather than half-serving when there is no store or no media server.
	var playoutSessions api.PlayoutSessions
	var playoutResolverSvc api.PlayoutResolver
	// The XMLTV guide (§9.1, V6b). Satisfied by the SAME *playoutResolver as above — one
	// source for "what airs when", so the guide cannot advertise something the encoder does
	// not play.
	var playoutGuideSvc api.PlayoutGuide
	// Declared out here so the pod assembler can be attached further down: the resolver is
	// built alongside the channel engine, while the pod adapter needs the filler catalog that
	// is wired later. Both halves are required before a break can play a real commercial.
	var playoutRes *playoutResolver
	if st != nil {
		lib := library.NewDynamic(flavorOrDefault(set), set.libraryConn(), instanceDeviceID(rootCtx, st))
		prog := programmer.NewDynamic(set.tunarrConn(), set.str("tunarr.transcode_config_id")).WithFillerPolicy(set.intv("filler.weight"), set.intv("filler.cooldown_seconds"))
		// Live TV points at whichever backend actually STREAMS (§9.1). `playout.backend`
		// defaults to `internal`, and wiring Tunarr's URLs unconditionally pointed the media
		// server at a backend that was not serving those channels — the channels appear in the
		// guide and refuse to play. Resolved per call so a backend switch applies without a
		// restart; the playout secret is read the same lazy way it is on the spawn path.
		connector := setup.NewLiveTVConnector(lib, func() setup.TunarrURLs {
			tok := ""
			if secrets != nil {
				tok = secrets.Value(settings.SecretPlayout)
			}
			return setup.LiveTVURLsFor(
				set.str("playout.backend"), set.str("tunarr.url"), set.str("server.public_url"), tok)
		})
		liveTVSvc = liveTVAdapter{connector}

		// Wire the media server as Tunarr's media source (§6, POST /v1/setup/tunarr-connect):
		// uses the concrete Tunarr client's media-source methods (prog), or the injected
		// double in tests when it implements them. Library flavor/url/token resolved live.
		var msProg setup.MediaSourceProgrammer = prog
		if ov.Programmer != nil {
			if msp, ok := ov.Programmer.(setup.MediaSourceProgrammer); ok {
				msProg = msp
			}
		}
		tunarrConnectSvc = setup.NewMediaSourceConnector(lib, msProg, func() (string, string, string) {
			u, tk := set.libraryConn()()
			fl := set.str("library.flavor")
			if fl == "" {
				fl = "emby"
			}
			return fl, u, tk
		})

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
		engine := channels.New(st, pusher, avail, connector, channels.Config{
			// Pending-slot policy defaults to pod-fill (§9); the interstitial-card
			// alternative is future config. SCHED_BACKFILL gates reshuffle-vs-stable
			// placement, handled inside the engine, not the placeholder kind.
			Policy: schedule.PodFill, ReconcileTTL: set.dur("channel.reconcile_every"),
			BreaksPerHour: set.intv("filler.breaks_per_hour"), // §10 commercial-break density
			DefaultWindow: set.dur("sched.window_hours"),      // §6.5 rolling-window horizon
		}, time.Now, log)
		// Heal an entry that reached the scheduler unrated once its title is in the
		// library (§389 amendment): without this a fail-closed audience ceiling drops
		// it and the channel plays nothing (§9). Uses the same library client the
		// availability resolver does.
		engine.WithRatings(libraryRatings{lib: lib})
		// Stamp media-server collection membership so scope.collections enforces with no
		// library I/O on the scheduling path (programming-design §2.2). Shares the library
		// client; the reverse index is built once and cached behind a TTL.
		engine.WithBoxSets(&libraryBoxSets{lib: lib, ttl: 15 * time.Minute})
		// Emit a `channel` SSE frame after each reconcile so the UI updates live — the
		// "no manual rebuild" model (§9). The emitter already fans to the event bus.
		engine.WithNotifier(emitter)
		channelSvc = engine

		// Now that the scheduler engine exists, give the emitter its backfill
		// handler: provisioning availability events (webhook + reconciler) fan to
		// OnAvailability, so an acquisition that lands `available` reconciles the
		// channels referencing it immediately — instead of waiting up to a full
		// sweep interval. (#10: the emitter was nil before this wire.)
		emitter.setEngine(engine)

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
			return secrets.Value(settings.SecretPlayout)
		}
		playoutMgr := playout.NewManager(
			playoutSpawner(set.str("playout.ffmpeg_path"),
				func() string { return set.str("server.public_url") },
				playoutTokenFn, log),
			set.intv("playout.max_channels"),
			playout.DefaultGrace,
			log,
		)
		playoutRes = &playoutResolver{
			engine: engine, lib: lib, now: time.Now,
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
			cycles:   newCycleCache(time.Now),
			tier:     func() string { return set.str("playout.quality_tier") },
			encoder:  func() string { return set.str("playout.encoder") },
			capacity: func() int { return set.intv("playout.max_channels") },
			// fillerDir is read live like every other setting; `pods` is assigned after the
			// pod adapter is built further down (it needs the filler catalog, which is wired
			// later) — see "playoutRes.pods" below.
			fillerDir: func() string { return set.str("filler.dir") },
			// The capability probe runs lazily on the first program that needs it, when
			// playout.encoder is unset — so a box with a working GPU uses it instead of
			// silently falling back to software.
			ffmpegPath: func() string { return set.str("playout.ffmpeg_path") },
			// Preferred audio language (§9.1), read live so a Settings change applies to the
			// next programme rather than the next restart. The prober derives ffprobe from the
			// ffmpeg path — the two ship together, so an operator who moved one moved both.
			audioLanguage: func() string { return set.str("playout.audio_language") },
			probeAudio:    playout.FFprobeAudioNextTo(set.str("playout.ffmpeg_path")),
			log:           log,
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
				Payload: map[string]int{"active": playoutMgr.ActiveCount()},
			})
		})
		playoutSessions = playoutMgr
		playoutResolverSvc = playoutRes
		// ⚠ A test-only observation point, unexported and write-only from here.
		//
		// The ladder inputs (tier/encoder/capacity/activeChannels) are called UNGUARDED by
		// Profile, so leaving one unset is a panic when a viewer tunes in. `Profile` is
		// invoked by the spawner rather than over HTTP, so no route reaches it and no
		// end-to-end test can observe the wiring. This lets the package's own test assert
		// that BuildHandler wired them — the assertion that was missing when the old
		// back-patch could be deleted with every test still green.
		lastPlayoutResolver = playoutRes
		playoutGuideSvc = playoutRes
		// A live encoder never exits on its own (playout/process.go), so shutdown MUST tear
		// them down explicitly or they outlive the process that started them.
		go func() {
			<-rootCtx.Done()
			playoutMgr.Stop()
		}()
		log.Info("internal playout registered",
			"ffmpeg", set.str("playout.ffmpeg_path"), "max_channels", set.intv("playout.max_channels"))

		chEvery := set.dur("channel.reconcile_every")
		// The channel sweep is a scheduler job now (§18.1) — same desired-vs-actual Sweep
		// (with its own ClaimDueChannels lease), driven by the shared heartbeat. The Runner's
		// lease/batch are still constructed; only its standalone loop is gone.
		chSweep := channels.NewRunner(engine, st, chEvery, 2*chEvery, 50, time.Now, log)
		jobReg.Add(chSweep.Job())
		log.Info("channel scheduler registered", "tunarr", set.str("tunarr.url"), "sweep_every", chEvery)
	}

	// The channel binder (§7): the ONE "materialize an approved proposal onto a
	// channel" implementation, shared by the manual-approve HTTP handler and the
	// suggest worker's auto-approve path (§8/§11) — closes the gap where a
	// per-user auto-approved refine enqueued acquisitions but never rebound its
	// channel. Needs only the store, so it's constructed whenever one is open;
	// channelSvc (if the scheduler/Tunarr block above wired it) satisfies
	// binder.Reconciler directly — same Reconcile(ctx, channelID) signature. If
	// channels isn't configured, channelSvc is a nil interface and the binder's
	// nil-guard just skips the immediate reconcile push (same best-effort
	// behavior createChannel/approveProposal always had).
	var chBinder *binder.Binder
	if st != nil {
		var rec binder.Reconciler
		if channelSvc != nil {
			rec = channelSvc
		}
		chBinder = binder.New(st, rec, log)
	}

	// Suggester + search (§8, Phase 11): the catalog boundary (library + TMDB),
	// the LLM provider, the grounded suggester, and the worker pool. Wired when a
	// store, the library, and the LLM are configured. The catalog + search share
	// one impl (§7.2).
	var suggestSvc api.SuggestService
	var searchSvc api.SearchService
	var collectionsSvc api.CollectionService
	var systemLLM api.SystemLLMService
	var iconSvc api.IconService
	if st != nil {
		lib := library.NewDynamic(flavorOrDefault(set), set.libraryConn(), instanceDeviceID(rootCtx, st))
		var tmdbClient *tmdb.Client
		if k := set.str("tmdb.api_key"); k != "" {
			tmdbClient = tmdb.New(k)
		}
		if ov.TMDB != nil { // tests point TMDB at an in-process double (offline)
			tmdbClient = ov.TMDB
		}
		// Franchise ordering (§5): teach the channel engine to heal each movie's TMDB
		// collection id at reconcile, so a franchise's films play together in release order.
		// Only when TMDB is configured; the engine was created in the earlier st-block, so
		// reach it through channelSvc (the same type-assert the pods wiring uses).
		if tmdbClient != nil {
			if engine, ok := channelSvc.(*channels.Engine); ok {
				engine.WithFranchises(tmdbFranchises{tmdb: tmdbClient})
			}
		}
		// catalog.New accepts nil corpora; a nil *tmdb.Client would be a non-nil
		// interface, so pass the concrete nil only when configured.
		var tmdbSearcher catalog.TMDBSearcher
		var validator suggest.Validator
		if tmdbClient != nil {
			tmdbSearcher = tmdbClient
			validator = tmdbClient
		} else {
			validator = noopValidator{} // no TMDB: acquisitions can't be re-validated, so treat as exists
		}
		// clip corpus wired in Phase 12. WithPresence lets discovery mark titles the
		// library already owns as in-library (a lineup pick, not an acquisition).
		cat := catalog.New(lib, tmdbSearcher).WithPresence(libraryPresence{lib})
		searchSvc = searchAdapter{cat}
		// The scope.collections picker (§2.2). Gated on the library alone — unlike search it
		// needs no TMDB, because a collection is the operator's own shelf and never a TMDB
		// lookup. Shares this block's library client.
		collectionsSvc = libraryCollections{lib: lib}

		// Channel icon suggestions (§icon P2): gated on TMDB, same as search/suggest above —
		// posters are TMDB-only, so no TMDB key means no suggestions, and the route 501s.
		if tmdbClient != nil {
			iconSvc = iconAdapter{store: st, tmdb: tmdbClient, log: log}
		}

		// The model is a config choice (§8/§14): ollama (local default) or the
		// OpenAI-compatible client (hosted OR Ollama's own /v1). LLM_PROVIDER selects.
		// For LOCAL ollama the active model is runtime-swappable (§8.1): an in-app
		// selection persisted to the settings store overrides LLM_MODEL and can be
		// changed without a restart. A hosted openai provider names its model in
		// config and isn't wrapped (nothing local to swap/probe/pull).
		provider, systemLLMSvc := buildLLM(rootCtx, set, st, eventBus, log)
		// Tests inject a scripted in-process LLM for the suggester; the systemLLM
		// picker still probes llm.url (point that at the Ollama HTTP double).
		if ov.LLM != nil {
			provider = ov.LLM
		}
		sug := suggest.New(provider, cat, validator, set.intv("suggest.max_acquisitions"))
		// Enrich a not-yet-owned acquisition's rating from TMDB (§389), so an audience
		// ceiling doesn't drop it before it can even show as a pending slot. Only when
		// TMDB is configured; sparse coverage is fine (the reconciler heals from the
		// library once it lands).
		if tmdbClient != nil {
			sug.WithRatings(tmdbClient)
		}
		svc := suggest.NewService(st, sug, suggest.Config{
			Workers: set.intv("job.workers"), Timeout: set.dur("job.timeout"), CacheTTL: 24 * time.Hour,
		}, newID, time.Now, log).WithProgressEmitter(emitter) // §8 SSE type=suggestion frames

		// The §11 auto-approve grant, hard-gated by the pending-acquisition cap. The
		// default limit is read PER CALL so raising suggest.max_acquisitions takes
		// effect without a restart, like every other setting (config-design §3).
		svc = svc.WithAutoApprove(suggest.NewAutoApprover(
			st,
			func(context.Context) int { return set.intv("suggest.max_acquisitions") },
			time.Now,
			log,
		))
		// Same *binder.Binder the API's approve handler uses (wired into api.Options
		// below) — so an auto-approved proposal ALSO rebinds its channel, closing the
		// gap where auto-approve enqueued acquisitions but left the channel stale.
		svc = svc.WithChannelBinder(chBinder)

		// Self-updating channels (§8.2): the channel-scoped auto-curate grant + the
		// scheduled re-curation job. The Curator approves a re-curation proposal because
		// its CHANNEL opted in (schedule.AutoCurate), bounded by the quality bar + title
		// cap (global settings, per-channel overridable) — through the SAME suggest.Approve
		// gate, audit "auto-curate". The Runner (registered below) triggers the refresh
		// refine that produces the proposal the Curator considers.
		curator := recurate.NewCurator(st, recurateThresholds{set}, time.Now, log)
		svc = svc.WithAutoCurate(curator)
		// The §8.3 adjacency corpus rides the SAME catalog the suggester grounds against,
		// so an adjacency candidate is the same shape (and gets the same in-library
		// backfill) as one the model found itself. Nil-safe: a catalog without a TMDB
		// client yields no adjacency and re-curation runs the LLM corpus alone.
		recurateRunner := recurate.NewRunner(st, svc, log).WithAdjacency(cat)
		jobReg.Add(recurateRunner.Job())

		suggestSvc = svc // *suggest.Service satisfies api.SuggestService directly
		systemLLM = systemLLMSvc
		go svc.Run(rootCtx)
		log.Info("suggester started", "provider", provider.Name(), "workers", set.intv("job.workers"), "tmdb", tmdbClient != nil)
	}

	// Filler & commercials (§10, Phase 12; redesign — Loomarr-owned via Tunarr).
	// Filler is a pure Loomarr↔Tunarr concern now: the catalog syncs from a Tunarr
	// `local` source over the FILLER_DIR drop-folder (the media server is out of the
	// filler path), plus the AI-tagging job and the pod assembler wired into the
	// scheduler. Wired when a store, Tunarr, and FILLER_DIR are configured.
	var fillerSvc api.FillerService
	var podPreview api.PodPreviewer
	if st != nil {
		fillerProg := programmer.NewDynamic(set.tunarrConn(), set.str("tunarr.transcode_config_id")).WithFillerPolicy(set.intv("filler.weight"), set.intv("filler.cooldown_seconds"))
		// The catalog comes from OUR OWN scan of FILLER_DIR (§9.1), with Tunarr consulted only
		// to annotate each clip with its program uuid for Tunarr-backed filler-lists. That
		// ordering is the fix: previously Tunarr's scan DEFINED the catalog, so an install
		// running internal playout with no Tunarr had no commercials at all.
		//
		// Tunarr is attached only when configured — with no tunarr.url there is nothing to
		// annotate with, and the scan alone is a complete catalog.
		// ⚠ CREATE the drop-folder if it is missing. `filler.dir` defaults to /data/filler
		// (like database.url and backup.dir), and the scanner treats a missing ROOT as a
		// FATAL error on purpose — that check exists so a typo'd path cannot present as an
		// empty catalog. Without this, shipping a default would swap an honest "not
		// configured" for a scan error on every fresh install: configured, and broken.
		//
		// Best-effort: a read-only or unwritable /data is the operator's to fix, and it
		// must not stop the rest of the app booting. The scan then reports the real problem.
		if dir := set.str("filler.dir"); dir != "" {
			if err := os.MkdirAll(dir, 0o750); err != nil {
				log.Warn("could not create the filler drop-folder; the catalog scan will report it",
					"dir", dir, "err", err)
			}
		}
		fillerSource := filler.DirSource{
			Dir:   func() string { return set.str("filler.dir") },
			Probe: filler.FFprobeNextTo(set.str("playout.ffmpeg_path")),
			// ⚠ **Artwork was relying on its nil default, which ignored `playout.ffmpeg_path`
			// entirely** and shelled out to whatever `ffmpeg` PATH resolved to. An operator who
			// points that setting at a custom build (the whole reason it exists — see the
			// hardware-encode notes) got their frames from a DIFFERENT binary than playout uses,
			// silently, and the setting appeared to do nothing here.
			Artwork: filler.FFmpegArtwork(set.str("playout.ffmpeg_path")),
			// The quality gate's floor (§10 V40). Read live, like `Dir`, so a settings change
			// applies on the next sync rather than needing a restart.
			MinDuration: func() time.Duration { return set.dur("filler.min_duration") },
			// ⚠ **Log was never assigned either**, so the "some thumbnails could not be generated"
			// warning has never once been emitted. That count exists precisely because extraction
			// is best-effort and failures are skipped — the shape that already produced one
			// silently-empty catalog in this repo's history (see FFprobeNextTo). A generator that
			// counts failures into a logger nobody wired is the same silence with extra steps.
			Log: log.Warn,
		}
		if set.str("tunarr.url") != "" {
			fillerSource.Tunarr = fillerSourceAdapter{fillerProg}
		}
		// ⚠ The switch is read on every sync, not captured here: `filler.source.folder.enabled`
		// hot-applies (config-design §3), so an operator who switches the drop-folder off
		// expects the next scheduled pass to stop rather than a restart to be required.
		syncer := filler.NewSyncer(fillerSource, fillerStoreAdapter{st}, set.str("filler.dir"), time.Now, log).
			WithEnabled(func() bool { return set.boolOn("filler.source.folder.enabled") }).
			// Read live for the same reason (§10 V38c). An empty value resolves to
			// `<filler.dir>/_watch`, so the watch folder is configured on every install
			// whether or not the operator has ever set it.
			WithWatchDir(func() string { return set.str("filler.watch_dir") })

		// Registered folders and libraries (§10 V38c). ⚠ The library scanner is nil when no media
		// server is configured, and that is a supported install rather than a degraded one:
		// folder rows still drain and library rows simply do no work. Wiring a non-nil scanner
		// over an absent media server would turn an optional service back into a precondition —
		// the dependency §9.1 removed.
		var libScanner *filler.LibraryScanner
		if set.str("library.url") != "" {
			libScanner = filler.NewLibraryScanner(
				fillerLibraryAdapter{library.NewDynamic(
					flavorOrDefault(set), set.libraryConn(), instanceDeviceID(rootCtx, st))},
				func(msg string, args ...any) { log.Warn(msg, args...) },
			)
		}
		syncer = syncer.WithScanSources(fillerScanSourceAdapter{st}, libScanner)

		var tagger *filler.Tagger
		if set.boolv("filler.ai_tagging") && set.str("llm.url") != "" {
			provider := llm.NewProvider(set.str("llm.provider"), set.str("llm.url"), set.str("llm.model"), set.str("llm.api_key"))
			// The drop-folder as an fs.FS so tagging can read the info-JSON sidecars
			// ingest writes beside each clip (§10). An unset FILLER_DIR yields a nil FS
			// and tagging falls back to filenames — the same result as a drop-folder
			// clip that never had a sidecar.
			var drop fs.FS
			if dir := set.str("filler.dir"); dir != "" {
				drop = os.DirFS(dir)
			}
			tagger = filler.NewTagger(fillerTagStoreAdapter{st}, provider, drop, time.Now, log).
				// Auto-filing (§10 V38): a held clip whose grounding-capped score clears the
				// threshold is filed without a human. Closures, not captured values, so a
				// changed threshold applies on the next run rather than the next restart.
				//
				// ⚠ `boolv`, NOT `boolOn`. The two differ only when the settings service cannot
				// answer, and here that difference is the whole safety property: `boolOn` fails
				// OPEN (returns true), which would publish unreviewed clips to live channels
				// exactly when the install is degraded. Holding is the safe failure.
				WithAutoFile(filler.AutoFilePolicy{
					Enabled:       func() bool { return set.boolv("filler.autofile.enabled") },
					MinConfidence: func() int { return set.intv("filler.autofile.min_confidence") },
				})
		}
		// Ingest tooling ships in the single image (§16); the loomarr:filler variant (retired-ok) no longer
		// exists. Absent paths are
		// the NORMAL state on loomarr:latest, so a nil fetcher is expected, not an error
		// — the `ingest` feature gate reports it and the UI explains the image variant.
		// ⚠ **TWO downloaders, resolved INDEPENDENTLY** (§10 V38b, wiring fixed V38c.8). archive.org
		// is fetched over plain HTTP and needs only ffmpeg; yt-dlp is for YouTube and shells out to
		// ffmpeg itself.
		//
		// This required BOTH paths to be set, which was the V38b defect surviving in the wiring
		// after the feature GATE was split. The result was worse than the original bug: `features
		// .ingest` reported true (correctly — ffmpeg is present), the Sources rows offered
		// "Fetch now", and then every archive fetch failed at the point of use with "ingest
		// tooling not present in this image". Two claims that cannot both be true, which is the
		// exact shape that started V38b.
		//
		// ⚠ An UNSET path falls back to a PATH lookup, matching `settings.toolRunnable` — §15 has
		// always described these as defaulting to the vendored binaries, and only the Docker image
		// set them, so a source build had ingest off with the tools installed.
		ytPath := resolveTool(set.str("ingest.ytdlp_path"), "yt-dlp")
		ffPath := resolveTool(set.str("ingest.ffmpeg_path"), "ffmpeg")
		var fetcher *clipfetch.Ingestor
		if ffPath != "" {
			// ⚠ A nil YouTube downloader is FINE — `downloaderFor` returns nil per kind and the
			// Ingestor counts that source as failed rather than dying. So a box with ffmpeg and no
			// yt-dlp fetches archive collections (the seeded ones) and reports honestly on
			// playlists, instead of refusing everything.
			var ytDL clipfetch.Downloader
			if ytPath != "" {
				ytDL = clipfetch.NewYtDlpDownloader(ytPath, ffPath)
			}
			fetcher = clipfetch.New(ytDL, clipfetch.NewArchiveDownloader(false), set.str("filler.dir"), log)
			log.Info("filler ingest available", "ytdlp", orNone(ytPath), "ffmpeg", ffPath)
		}
		// Compilation splitting (§10, V34). Needs the drop-folder (clip paths are
		// relative to it, so without one there is nothing to cut). ffmpeg/ffprobe
		// come from playout.ffmpeg_path — a core runtime dep on the single image —
		// NOT the ingest pair, because splitting works on files already on disk and
		// must not die just because yt-dlp is absent. whisper is optional: without
		// it, over-long segments come back Unsplittable rather than guessed (§15).
		// The LLM provider wires whenever one is configured — splitting's rescue and
		// classification are operator-invoked, not the batch job filler.ai_tagging
		// gates.
		var splitter *filler.Splitter
		if dir := set.str("filler.dir"); dir != "" {
			var splitProvider llm.Provider
			if set.str("llm.url") != "" {
				splitProvider = llm.NewProvider(set.str("llm.provider"), set.str("llm.url"), set.str("llm.model"), set.str("llm.api_key"))
			}
			ffmpegPath := set.str("playout.ffmpeg_path")
			tools := filler.NewFFmpegTools(ffmpegPath, filler.FFprobePathNextTo(ffmpegPath),
				set.str("ingest.whisper_path"), set.str("ingest.whisper_model"), "")
			splitter = filler.NewSplitter(fillerSplitStoreAdapter{st}, tools, splitProvider, dir, newID, time.Now, log)
		}
		fillerSvc = fillerServiceAdapter{
			syncer: syncer, tagger: tagger, fetcher: fetcher,
			bus: eventBus, newID: newID, timeout: set.dur("ingest.timeout"),
			sources: st, now: time.Now,
			splitter: splitter, splitClips: fillerSplitStoreAdapter{st},
		}

		// Pod assembler → scheduler (§10). If the channel engine is up, teach it to
		// build a matched filler-list per channel (attached to Tunarr on reconcile).
		// The SAME adapter instance backs the §12 preview endpoint, so preview and
		// reconcile share one assembler and one policy — they cannot drift.
		podAdapter := filler.NewPodAdapter(clipCatalogAdapter{st}, filler.Policy{
			PodMax: set.intv("filler.pod_max"),
			// V17c: 0 (the default) leaves selection exactly as it was before the floor
			// existed — see the warning on Policy.MinQualityHeight.
			MinQualityHeight: set.intv("filler.min_quality"),
		}, log)
		podPreview = podPreviewAdapter{store: st, pods: podAdapter}
		// Commercial breaks for internal playout (§10): the SAME pod assembler the API preview
		// and the reconciler use, so the ad that plays is the one the channel page promised.
		// Assigned here rather than at construction because the resolver is built with the
		// channel engine (above) while the pod adapter needs the filler catalog (here).
		if playoutRes != nil {
			playoutRes.pods = podPreview
		}
		if engine, ok := channelSvc.(*channels.Engine); ok {
			engine.WithPods(podAdapter)
			log.Info("filler pod assembler wired into the scheduler")
		}

		// Filler catalog sync is a scheduler job now (§18.1) — same Syncer.Sync, on the
		// shared heartbeat. Interval key: filler.sync_every.
		jobReg.Add(fillerSyncJob(syncer))
		log.Info("filler catalog sync registered", "dir", set.str("filler.dir"), "every", set.dur("filler.sync_every"), "ai_tagging", set.boolv("filler.ai_tagging"))

		// Auto-fetch (§10 V38b): registered sources are polled and new clips download unattended.
		// Every limit is a closure so it hot-applies — an operator lowering a ceiling expects the
		// next run to honour it, not the next restart.
		//
		// ⚠ `filler.fetch.every == 0` disables the whole job. It is checked inside Run rather
		// than by skipping registration, so the Tasks page still shows the row: an omitted row is
		// indistinguishable from a job that runs fine and has never failed — the same call V12
		// made for backups on Postgres.
		autoFetch := filler.NewFetcher(
			fetchStoreAdapter{st: st, fetchEvery: func() time.Duration { return set.dur("filler.fetch.every") }},
			archiveDiscoverAdapter{}, fillerSvc, set.str("filler.dir"),
			filler.FetchLimits{
				MaxPerRun:       func() int { return set.intv("filler.fetch.max_per_run") },
				MaxCatalogClips: func() int { return set.intv("filler.fetch.max_catalog_clips") },
				MaxDiskGB:       func() int { return set.intv("filler.fetch.max_disk_gb") },
			}, log,
		).WithEnabled(func() bool { return set.dur("filler.fetch.every") > 0 })
		jobReg.Add(fillerFetchJob(autoFetch))
		log.Info("filler auto-fetch registered", "every", set.dur("filler.fetch.every"), "max_per_run", set.intv("filler.fetch.max_per_run"))
	}

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
	if st != nil {
		// Auth wires on the STORE alone (§11 rework): identity is Loomarr-owned, so
		// bootstrap + local login work with zero media-server config. The library
		// (media-server verifier + user lister) is optional — nil ⇒ import is
		// unavailable and only local users can sign in.
		var lib *library.Client
		if set.str("library.flavor") != "" {
			flavor, _ := library.ParseFlavor(set.str("library.flavor"))
			lib = library.NewDynamic(flavor, set.libraryConn(), instanceDeviceID(rootCtx, st))
		}
		mgr := auth.NewManager(st, set.dur("session.ttl"), time.Now)
		limiter := auth.NewRateLimiter(0.2, 5) // ~5 attempts, refill 1/5s (§11)
		// NewLoginService takes the Authenticator interface; a nil *library.Client
		// would be a non-nil interface, so pass nil explicitly when unconfigured.
		if lib != nil {
			loginSvc = auth.NewLoginService(lib, st, mgr, limiter, time.Now)
			userSync = auth.NewUserSync(lib, st, time.Now)
			provisionSvc = auth.NewProvisioner(st, lib, newID, time.Now)
		} else {
			loginSvc = auth.NewLoginService(nil, st, mgr, limiter, time.Now)
			provisionSvc = auth.NewProvisioner(st, nil, newID, time.Now)
		}
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
		authorizer = api.NewSessionAuthorizer(mgr, apiToken)
	}

	// The playout device token (§11). A FUNC, not a captured value, so a REGENERATED token
	// takes effect immediately — rotation is an operator action the UI offers, and a value read
	// once at boot would keep authorizing the old token until restart.
	//
	// Nil when secrets are unavailable (no store), which makes the playout routes fail CLOSED:
	// serving streams unauthenticated because a secret could not be minted would silently
	// remove the only auth those routes have.
	var playoutSecret func() string
	if secrets != nil {
		playoutSecret = func() string { return secrets.Value(settings.SecretPlayout) }
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
	// The Tunarr half still reads the LIVE connection through the settings snapshot, so saving a
	// new Tunarr URL takes effect without a restart (config-design §3 hot-apply). A 2h window is
	// comfortably longer than any single program, so "next" is always present.
	var guideSvc api.GuideReader
	if st != nil {
		var tunarrGuide api.GuideReader
		if set.str("tunarr.url") != "" {
			tunarrGuide = guideAdapter{
				tunarr: programmer.NewDynamic(set.tunarrConn(), set.str("tunarr.transcode_config_id")),
				window: 2 * time.Hour,
			}
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
			// policy.playout.backend wins, else the global. Resolved live so a backend switch
			// applies to the next render.
			internalFor: func(ch store.Channel) bool {
				return schedule.PlaysInternally(ch.Policy, set.str("playout.backend"))
			},
			window: 2 * time.Hour,
		}
	}

	var settingsSvc api.SettingsService
	if st != nil && secrets != nil {
		settingsSvc = settingsAdapter{
			svc:     set.svc,
			secrets: secrets,
			store:   st,
			log:     log,
			tests:   connectionTests(set),
		}
	}

	// liveConfig lets the always-constructed feature routes gate on the CURRENT
	// config (config-design §3 hot-apply): a saved connection enables the route with
	// no restart (§8.1). Only when the settings service exists (a store is open).
	var liveConfig func(key string) string
	if st != nil {
		liveConfig = set.str
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
				// ⚠ BuildHandler returns only a handler — there is no teardown seam to hang a
				// Stop on — so River is stopped when rootCtx ends. This matters for the §9.2
				// restart loop, which rebuilds the app IN-PROCESS: without an explicit stop,
				// each generation leaves its client's goroutines running. The goleak test
				// caught exactly that on the first attempt here.
				//
				// ⚠ StartRiver already stops the client when rootCtx is cancelled; this waits
				// for that to FINISH before letting the store close. River's shutdown issues
				// queries, so a pool closed underneath it strands its goroutines — measured at
				// 4 leaked per generation, which the §9.2 restart loop's goleak test caught.
				//
				// Hung off the store's own teardown rather than returned from BuildHandler:
				// the ordering constraint is precisely "not before the pool closes", so the
				// wait belongs with the thing being protected, and every caller
				// (main, tests, the integration harness) gets it without changing signature.
				store.OnClose(st, func() {
					waitCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
					defer cancel()
					if err := stop(waitCtx); err != nil {
						log.Warn("scheduler: timed out waiting for River to stop", "err", err)
					}
				})
			}
		} else {
			log.Error("scheduler: store exposes no pool — no job will run on its schedule")
		}
		jobsSvc = jobsAdapter{s: sched}
	}

	// Database migration stepper (§18, V11). Wired only for a SQLite install: the
	// service reports CanMigrate=false on Postgres, but there is no reason to hold the
	// state machine at all there. backup.dir is read at call time so a hot-applied
	// change takes effect without a restart.
	var databaseSvc api.DatabaseService
	if st != nil && store.DialectOf(st) == store.DialectSQLite {
		databaseSvc = newDatabaseService(st, filepath.Dir(store.SQLitePath(st)),
			func() string { return set.str("backup.dir") }, eventBus)
	}

	return api.Router(log, api.Options{
		Store:         st,
		Auth:          authorizer,
		Log:           log,
		BackupSQLite:  backup,
		Ready:         ready,
		Login:         loginSvc,
		Sessions:      sessMgr,
		Passwords:     passwordSvc,
		UserSync:      userSync,
		CookieSecure:  set.str("cookie.secure"),
		DevLogin:      ov.DevLogin,
		Pprof:         ov.Pprof,
		Channels:      channelSvc,
		LiveTV:        liveTVSvc,
		TunarrConnect: tunarrConnectSvc,
		Suggest:       suggestSvc,
		Search:        searchSvc,
		Collections:   collectionsSvc,
		Icons:         iconSvc,
		Events:        eventBus,
		Filler:        fillerSvc,
		Pods:          podPreview,
		SystemLLM:     systemLLM,
		Database:      databaseSvc,
		Backups:       backupsSvc,
		SSO:           ssoSvc,
		Restart:       restartSvc,
		Activity:      activityRec,
		// The baseline for "has a boot-time setting changed?" is what THIS generation
		// booted with, captured here rather than per call (config-design §3).
		BootstrapDrift: bootstrapDrift(bootCfg),
		Jobs:           jobsSvc,
		Settings:       settingsSvc,
		Guide:          guideSvc,
		Provision:      provisionSvc,
		Binder:         chBinder,
		LiveConfig:     liveConfig,
		LiveConfigInt:  set.intv,
		// boolOn (not boolv): the API reads bool keys that are ON by default, where an
		// unanswerable read must fail open — see Options.LiveConfigBoolOn.
		LiveConfigBoolOn: set.boolOn,
		// Internal playout (§9.1). PlayoutSecret is a FUNC so a regenerated token takes
		// effect without a restart (§11 rotation).
		PlayoutSessions: playoutSessions,
		PlayoutResolver: playoutResolverSvc,
		// The XMLTV guide reads the same resolver, so listings cannot drift from playout.
		PlayoutGuide:  playoutGuideSvc,
		PlayoutSecret: playoutSecret,
		// Bound to the ffmpeg path once, like probeAudio above: the answer depends on the
		// BUILD, so it cannot change without the binary changing. Memoised inside, so the
		// `-filters` exec happens on the first offline card and never again.
		PlayoutFont: playout.CardFontFor(set.str("playout.ffmpeg_path")),
		PlayoutEncoder: func(
			ctx context.Context, args []string, onProgress func(playout.Progress),
		) (*playout.Process, error) {
			// `onProgress` was nil here since the supervisor was written, so ffmpeg's parsed
			// progress samples were discarded every time. Passing it through is what makes the
			// dashboard's encoder speed measured rather than invented (V16).
			return playout.Start(ctx, set.str("playout.ffmpeg_path"), args, log, onProgress)
		},
	}), nil
}
