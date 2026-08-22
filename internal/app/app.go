// Package app is the composition root: it wires every subsystem from an open
// store into the API handler that cmd/loomarr serves and the integration tests
// drive. Keeping it importable (not package main) is what lets tests exercise
// the REAL wiring end to end (§21), and keeps cmd/loomarr a thin entrypoint.
package app

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"github.com/mantonx/loomarr/internal/api"
	"github.com/mantonx/loomarr/internal/config"
	"github.com/mantonx/loomarr/internal/llm"
	"github.com/mantonx/loomarr/internal/playout"
	"github.com/mantonx/loomarr/internal/programmer"
	"github.com/mantonx/loomarr/internal/settings"
	"github.com/mantonx/loomarr/internal/store"
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

	channelsBuilt, err := buildChannels(
		rootCtx, st, set, ov, owner, capturePlayoutResolver, libraryClient, secrets,
		readGeneratedSecret, eventBus, emitter, activityRec, jobReg, episodeRefresh, fillerLayout, log,
	)
	if err != nil {
		return nil, err
	}
	channelSvc, liveTVSvc := channelsBuilt.channelService, channelsBuilt.liveTV
	tunerRescanner, tunarrConnectSvc := channelsBuilt.tunerRescanner, channelsBuilt.tunarrConnector
	backendController := channelsBuilt.backendController
	refreshBackendSettings, desiredBackend := channelsBuilt.refreshBackendSettings, channelsBuilt.desiredBackend
	appliedBackendContext := channelsBuilt.appliedBackend
	checkpointSnapshot := channelsBuilt.checkpoint
	playoutObserver, preparedObserver := channelsBuilt.playoutObserver, channelsBuilt.preparedObserver
	playoutSvc, playoutResolverSvc := channelsBuilt.playout, channelsBuilt.playoutResolverService
	encodePool, playoutGuideSvc := channelsBuilt.encodePool, channelsBuilt.playoutGuide
	playoutRes, chanNumbers := channelsBuilt.playoutResolver, channelsBuilt.channelNumbers
	setResidentVRAM := channelsBuilt.setResidentVRAM

	approval := buildApproval(st, channelSvc, playoutRes, activityRec, chanNumbers, log)
	chBinder, proposalApprover := approval.binder, approval.approver

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

	backupsSvc := buildBackups(st, set, jobReg, log)

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

	authServices := buildAuth(st, set, secrets, readGeneratedSecret, libraryClient, log)
	backup, authorizer := authServices.backup, authServices.authorizer
	loginSvc, ssoSvc := authServices.login, authServices.sso
	sessMgr, userSync := authServices.sessions, authServices.userSync
	provisionSvc, passwordSvc := authServices.provision, authServices.password
	deviceMgr, deviceLimiter := authServices.deviceManager, authServices.deviceLimiter
	playoutSecret, playoutSecretCurrent := authServices.playoutSecret, authServices.playoutSecretCurrent

	guideSvc := buildGuide(st, set, playoutRes, appliedBackendContext)
	settingsSvc := buildSettings(
		st, set, desiredSet, secrets, libraryClient, tmdbClient,
		refreshSecretRedactor, readGeneratedSecret, log,
	)

	// liveConfig lets the always-constructed feature routes gate on the CURRENT
	// config (config-design §3 hot-apply): a saved connection enables the route with
	// no restart (§8.1). Only when the settings service exists (a store is open).
	var liveConfig func(key string) string
	var libraryConfigured func() bool
	if st != nil {
		liveConfig = set.str
		libraryConfigured = set.libraryConfigured
	}

	jobsSvc := buildScheduler(rootCtx, st, set, jobReg, emitter, owner, log)

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
		setResidentVRAM(residentLLMVRAMFn)
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
