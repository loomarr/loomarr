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
	"time"

	"github.com/mantonx/loomarr/internal/api"
	"github.com/mantonx/loomarr/internal/auth"
	"github.com/mantonx/loomarr/internal/binder"
	"github.com/mantonx/loomarr/internal/catalog"
	"github.com/mantonx/loomarr/internal/channels"
	"github.com/mantonx/loomarr/internal/clipfetch"
	"github.com/mantonx/loomarr/internal/events"
	"github.com/mantonx/loomarr/internal/filler"
	"github.com/mantonx/loomarr/internal/library"
	"github.com/mantonx/loomarr/internal/llm"
	"github.com/mantonx/loomarr/internal/metrics"
	"github.com/mantonx/loomarr/internal/programmer"
	"github.com/mantonx/loomarr/internal/reconcile"
	"github.com/mantonx/loomarr/internal/recurate"
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
		jobReg.Add(scheduler.Job{
			Name: "reconcile", Title: "Reconcile downloads",
			DefaultCron: "0 */5 * * * *", ScheduleKey: "job.reconcile.schedule",
			Run: func(ctx context.Context) error { _, err := rec.Tick(ctx); return err },
		})
		// Session cleanup is its own scheduler job now (was piggybacking the reconcile
		// ticker via the janitor).
		sessionSweeper := auth.NewSessionSweeper(st)
		jobReg.Add(scheduler.Job{
			Name: "session-sweep", Title: "Clear expired sessions",
			DefaultCron: "0 0 * * * *", ScheduleKey: "job.session_sweep.schedule",
			Run: func(ctx context.Context) error { _, err := sessionSweeper.Sweep(ctx, time.Now()); return err },
		})

		// Poll-based availability (§4, §18.1): the PRIMARY path a requested title reaches
		// `available`. The scan lists what the media server recently added and confirms any
		// in-flight title now present — the same LibraryConfirmed transition the reconciler's
		// deadline backstop applies, but continuous. `lib` (a *library.Client) satisfies
		// LibraryScanner. Incremental runs often (recent window); Full is the daily safety net.
		libScan := reconcile.NewLibraryScan(st, lib, emitter, set.dur("job.library_scan.lookback"), time.Now, log)
		jobReg.Add(scheduler.Job{
			Name: "library-scan", Title: "Scan library for new titles",
			DefaultCron: "0 */5 * * * *", ScheduleKey: "job.library_scan.schedule",
			Run: func(ctx context.Context) error { _, err := libScan.Incremental(ctx); return err },
		})
		jobReg.Add(scheduler.Job{
			Name: "library-full-scan", Title: "Full library sweep",
			DefaultCron: "0 0 3 * * *", ScheduleKey: "job.library_full_scan.schedule",
			Run: func(ctx context.Context) error { _, err := libScan.Full(ctx); return err },
		})

		// Queue poller (§18.1) — one poll job, whichever requester is active. The direct arr
		// reads Sonarr/Radarr queues for real byte-progress; Seerr reads /media for a COARSE
		// status (Downloading / Partly available, no percentage). Both promote grabbed titles
		// requested→downloading and persist what they know; availability stays the library
		// scan's job. The two branches are mutually exclusive (a provider is arr XOR seerr), so
		// their distinct job names never collide in the registry.
		if arr := set.arrRequester(); arr != nil {
			queuePoll := reconcile.NewQueuePoll(st, arr, emitter, set.dur("downloading.ttl"), time.Now, log)
			jobReg.Add(scheduler.Job{
				Name: "arr-queue-poll", Title: "Poll Sonarr/Radarr downloads",
				DefaultCron: "0 * * * * *", ScheduleKey: "job.arr_queue_poll.schedule",
				Run: func(ctx context.Context) error { _, err := queuePoll.Poll(ctx); return err },
			})
		} else if seerr := set.seerrRequester(); seerr != nil {
			queuePoll := reconcile.NewQueuePoll(st, seerr, emitter, set.dur("downloading.ttl"), time.Now, log)
			jobReg.Add(scheduler.Job{
				Name: "seerr-queue-poll", Title: "Poll Seerr acquisition status",
				DefaultCron: "0 * * * * *", ScheduleKey: "job.seerr_queue_poll.schedule",
				Run: func(ctx context.Context) error { _, err := queuePoll.Poll(ctx); return err },
			})
		}
	}

	// Scheduler + Tunarr (§9, Phase 10): the channel reconcile engine + periodic
	// sweep, plus the Live TV wiring connector (guide-refresh poker). Wired when a
	// store, Tunarr, and the media-server library are configured.
	var channelSvc api.ChannelService
	var liveTVSvc api.LiveTVService
	var tunarrConnectSvc api.TunarrConnector
	if st != nil {
		lib := library.NewDynamic(flavorOrDefault(set), set.libraryConn(), instanceDeviceID(rootCtx, st))
		prog := programmer.NewDynamic(set.tunarrConn(), set.str("tunarr.transcode_config_id")).WithFillerPolicy(set.intv("filler.weight"), set.intv("filler.cooldown_seconds"))
		connector := setup.NewLiveTVConnector(lib, setup.TunarrURLsFrom(set.str("tunarr.url")))
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

		chEvery := set.dur("channel.reconcile_every")
		// The channel sweep is a scheduler job now (§18.1) — same desired-vs-actual Sweep
		// (with its own ClaimDueChannels lease), driven by the shared heartbeat. The Runner's
		// lease/batch are still constructed; only its standalone loop is gone.
		chSweep := channels.NewRunner(engine, st, chEvery, 2*chEvery, 50, time.Now, log)
		jobReg.Add(scheduler.Job{
			Name: "channel-sweep", Title: "Reconcile channels with Tunarr",
			DefaultCron: "0 */10 * * * *", ScheduleKey: "job.channel_sweep.schedule",
			Run: func(ctx context.Context) error { chSweep.Sweep(ctx); return nil },
		})
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
		recurateRunner := recurate.NewRunner(st, svc, log)
		jobReg.Add(scheduler.Job{
			Name: "channel-recurate", Title: "Re-curate self-updating channels",
			DefaultCron: "0 0 4 * * 0", ScheduleKey: "job.recurate.schedule",
			Run: func(ctx context.Context) error { _, err := recurateRunner.Run(ctx); return err },
		})

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
		syncer := filler.NewSyncer(fillerSourceAdapter{fillerProg}, fillerStoreAdapter{st}, set.str("filler.dir"), time.Now, log)

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
			tagger = filler.NewTagger(fillerTagStoreAdapter{st}, provider, drop, time.Now, log)
		}
		// Ingest tooling ships only in the loomarr:filler image (§16). Absent paths are
		// the NORMAL state on loomarr:latest, so a nil fetcher is expected, not an error
		// — the `ingest` feature gate reports it and the UI explains the image variant.
		var fetcher *clipfetch.Ingestor
		if yt, ff := set.str("ingest.ytdlp_path"), set.str("ingest.ffmpeg_path"); yt != "" && ff != "" {
			fetcher = clipfetch.New(
				clipfetch.NewYtDlpDownloader(yt, ff),
				clipfetch.NewArchiveDownloader(false),
				set.str("filler.dir"), log)
			log.Info("filler ingest available", "ytdlp", yt, "ffmpeg", ff)
		}
		fillerSvc = fillerServiceAdapter{
			syncer: syncer, tagger: tagger, fetcher: fetcher,
			bus: eventBus, newID: newID, timeout: set.dur("ingest.timeout"),
		}

		// Pod assembler → scheduler (§10). If the channel engine is up, teach it to
		// build a matched filler-list per channel (attached to Tunarr on reconcile).
		// The SAME adapter instance backs the §12 preview endpoint, so preview and
		// reconcile share one assembler and one policy — they cannot drift.
		podAdapter := filler.NewPodAdapter(clipCatalogAdapter{st}, filler.Policy{PodMax: set.intv("filler.pod_max")}, log)
		podPreview = podPreviewAdapter{store: st, pods: podAdapter}
		if engine, ok := channelSvc.(*channels.Engine); ok {
			engine.WithPods(podAdapter)
			log.Info("filler pod assembler wired into the scheduler")
		}

		// Filler catalog sync is a scheduler job now (§18.1) — same Syncer.Sync, on the
		// shared heartbeat. Interval key: filler.sync_every.
		fillerSyncer := syncer
		jobReg.Add(scheduler.Job{
			Name: "filler-sync", Title: "Sync filler catalog",
			DefaultCron: "0 */15 * * * *", ScheduleKey: "job.filler_sync.schedule",
			Run: func(ctx context.Context) error { _, err := fillerSyncer.Sync(ctx); return err },
		})
		log.Info("filler catalog sync registered", "dir", set.str("filler.dir"), "every", set.dur("filler.sync_every"), "ai_tagging", set.boolv("filler.ai_tagging"))
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
		// Local account management (§11) needs no media server — a local user is
		// verified entirely in-app, exactly like the bootstrap admin. So it is wired
		// unconditionally alongside the store, not inside the `lib != nil` branch.
		passwordSvc = auth.NewPasswordService(st, newID, time.Now)
		sessMgr = mgr
		authorizer = api.NewSessionAuthorizer(mgr, apiToken)
	}

	// The settings API surface (config-design §8): wired when the store (hence the
	// settings service) is up. Connection Test probes reuse the live adapters.
	// The guide reader powers /v1/channels/now-next. It reads the LIVE Tunarr connection
	// through the settings snapshot, so saving a new Tunarr URL takes effect without a
	// restart, like every other connection (config-design §3 hot-apply). A 2h window is
	// comfortably longer than any single program, so "next" is always present.
	var guideSvc api.GuideReader
	if set.str("tunarr.url") != "" {
		guideSvc = guideAdapter{
			tunarr: programmer.NewDynamic(set.tunarrConn(), set.str("tunarr.transcode_config_id")),
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
		go sched.Run(rootCtx)
		jobsSvc = jobsAdapter{s: sched}
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
		Channels:      channelSvc,
		LiveTV:        liveTVSvc,
		TunarrConnect: tunarrConnectSvc,
		Suggest:       suggestSvc,
		Search:        searchSvc,
		Icons:         iconSvc,
		Events:        eventBus,
		Filler:        fillerSvc,
		Pods:          podPreview,
		SystemLLM:     systemLLM,
		Jobs:          jobsSvc,
		Settings:      settingsSvc,
		Guide:         guideSvc,
		Provision:     provisionSvc,
		Binder:        chBinder,
		LiveConfig:    liveConfig,
		LiveConfigInt: set.intv,
	}), nil
}
