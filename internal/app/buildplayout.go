package app

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"github.com/loomarr/loomarr/internal/api"
	"github.com/loomarr/loomarr/internal/backendtransition"
	"github.com/loomarr/loomarr/internal/channels"
	"github.com/loomarr/loomarr/internal/diagnostics"
	"github.com/loomarr/loomarr/internal/events"
	"github.com/loomarr/loomarr/internal/filler"
	"github.com/loomarr/loomarr/internal/inventory"
	"github.com/loomarr/loomarr/internal/library"
	"github.com/loomarr/loomarr/internal/llm"
	"github.com/loomarr/loomarr/internal/media"
	"github.com/loomarr/loomarr/internal/mediameasure"
	"github.com/loomarr/loomarr/internal/metrics"
	"github.com/loomarr/loomarr/internal/playout"
	"github.com/loomarr/loomarr/internal/settings"
	"github.com/loomarr/loomarr/internal/setup"
	"github.com/loomarr/loomarr/internal/storagegovernor"
	"github.com/loomarr/loomarr/internal/store"
	"github.com/loomarr/loomarr/internal/tunarr"
)

const playoutGPUIdentityTimeout = 250 * time.Millisecond

type playoutBuild struct {
	observer          api.PlayoutObserver
	capability        func() playout.Capacity
	service           api.Playout
	resolverService   api.PlayoutResolver
	guide             api.PlayoutGuide
	resolver          *playoutResolver
	backendController *backendtransition.Controller
	setResidentVRAM   func(func(context.Context) (float64, string))
	// budget is the one admission ledger; filler's media work waits on it (playbackHeadroomFor).
	budget *playout.ResourceBudget
}

type playoutDeps struct {
	rootCtx               context.Context
	store                 store.Store
	settings              resolved
	owner                 *generationLifecycle
	captureResolver       func(*playoutResolver)
	library               *library.Client
	secrets               *settings.Secrets
	readSecret            func(context.Context, settings.GeneratedSecret) (string, error)
	events                *events.Bus
	layout                filler.Layout
	channels              *channels.Engine
	liveTVConnector       *setup.LiveTVConnector
	backendView           backendtransition.CheckpointView
	resolveDesiredBackend func(context.Context) (string, error)
	// programmer is the SAME Tunarr adapter instance buildChannels built (not an override-aware
	// one: the Live TV URL fallback always reads the live tunarr.url setting, same as before
	// this seam existed — see liveTVURLsFor).
	programmer         tunarr.Adapter
	log                *slog.Logger
	processDiagnostics *diagnostics.ProcessManager
	storageGovernor    *storagegovernor.Governor
	metrics            *metrics.Recorder
	// capacityProbe starts the boot capacity probe (Overrides.CapacityProbe); startup receives its
	// tone-map self-check.
	capacityProbe bool
	startup       *diagnostics.Startup
	// watermarks draws the channel's bug on programme items (#1512 phase 1d).
	watermarks *channelWatermarks
}

func buildPlayout(deps playoutDeps) (playoutBuild, error) {
	rootCtx, st, set := deps.rootCtx, deps.store, deps.settings
	owner, capturePlayoutResolver := deps.owner, deps.captureResolver
	libraryClient, readGeneratedSecret := deps.library, deps.readSecret
	lib := libraryClient
	eventBus, fillerLayout := deps.events, deps.layout
	channelEngine, liveTVConnector := deps.channels, deps.liveTVConnector
	backendView, resolveDesiredBackend, log := deps.backendView, deps.resolveDesiredBackend, deps.log
	var playoutObserver api.PlayoutObserver
	var playoutSvc api.Playout
	var playoutResolverSvc api.PlayoutResolver
	var playoutGuideSvc api.PlayoutGuide
	var playoutRes *playoutResolver
	var backendController *backendtransition.Controller
	var residentVRAM func(context.Context) (float64, string)
	// Internal playout (§9.1): Loomarr serves its own channels. Wired here because this is
	// where BOTH halves already exist — the engine that answers "what airs when" and the
	// library client that resolves an item to a streamable URL.
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
	effectivePlayoutCapacity := func(measured int) int {
		residentGiB := 0.0
		if residentVRAM != nil {
			residentGiB, _ = residentVRAM(rootCtx)
		}
		return playout.EffectiveCapacity(measured, set.intv("playout.max_channels"), residentGiB)
	}
	playoutBudget := func() int {
		measured := 0
		if playoutRes != nil {
			measured = int(playoutRes.maxChannels.Load()) // published once by the lazy Detect trial
		}
		return effectivePlayoutCapacity(measured)
	}
	// The ResourceBudget (#1512 G5) is the one admission ledger for the channel packagers and the
	// prepared pool. Its facts are re-read per admission: the cgroup quota or CPU count, the CPU
	// allowance settings, and the measured capacity above (operator cap and VRAM shading included).
	resourceBudget := playout.NewResourceBudget(func() playout.BudgetFacts {
		hardware := playoutRes != nil && playoutRes.detectReady.Load() &&
			!playout.IsSoftwareEncoder(playoutRes.publishedEncoder())
		var costs *playout.MeasuredCosts
		if playoutRes != nil {
			enc := playout.Encoder(set.str("playout.encoder"))
			if enc == "" {
				enc = playoutRes.publishedEncoder()
			}
			costs = playoutRes.CostsFor(enc)
		}
		facts := playoutBudgetFacts(
			playout.ReadHostCPU(os.DirFS("/"), runtime.NumCPU()), hardware, playoutBudget(), costs,
			effectivePlayoutCapacity, playout.TierFor(set.str("playout.quality_tier")),
			set.intv("playout.gpu_cpu_millicores"), set.intv("playout.app_reserve_millicores"),
		)
		facts.ToneCurve = playoutRes.ToneCurve() // HDR capacity follows the operator's curve
		return facts
	}).WithLog(log)
	// The channel packager is built after the resolver it reads the schedule through.
	var packagedHLS *playout.PackagerHLS
	// Host measurements live in playout.state_dir; the encoder evidence moves there from the retired
	// prepared library once (#1512), so an upgrade reuses its verified measurement instead of
	// re-benchmarking. The prepared library's setting is retired (phase 4), so the move reads its
	// default home, <data dir>/prepared, the sibling of the default state directory; a custom
	// state_dir finds nothing there and measures again.
	stateDir := set.str("playout.state_dir")
	legacyPrepared := filepath.Join(filepath.Dir(filepath.Clean(stateDir)), "prepared")
	if moved, merr := playout.MigrateCapabilityEvidence(legacyPrepared, stateDir); merr != nil {
		log.Warn("playout: could not move the encoder measurement to the state directory; it will be re-measured", "err", merr)
	} else if moved {
		log.Info("playout: moved the encoder measurement to the state directory", "dir", stateDir)
	}
	// With the evidence out, the rest of the retired library is media nothing reads (decision 0040).
	// Reclaim it once the generation has built, off the critical path: an upgraded install can hold
	// hundreds of GB there. Only what that library wrote goes (#1563).
	owner.goRunAfterBuild(func(context.Context) {
		got, err := playout.ReclaimRetiredPrepared(legacyPrepared)
		if got.Entries > 0 {
			log.Info("playout: removed the retired prepared-media library",
				"dir", legacyPrepared, "entries", got.Entries, "reclaimed_bytes", got.Bytes, "dir_removed", got.DirRemoved)
		}
		if err != nil {
			log.Warn("playout: could not remove all of the retired prepared-media library; delete what is left by hand",
				"dir", legacyPrepared, "err", err)
		}
	})
	playoutRes = &playoutResolver{
		// The library client with the server-path cache (#1456): airtime input resolution reads
		// the remembered path locally and only asks the media server on a cold or stale entry.
		engine: channelEngine, lib: lib.WithPathCache(libraryPathCache{st}), now: time.Now,
		metrics:       deps.metrics,
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
		codecs:  st,
		cycles:  newCycleCache(time.Now),
		tier:    func() string { return set.str("playout.quality_tier") },
		encoder: func() string { return set.str("playout.encoder") },
		// The operator's HDR curve: the probe measures it and the budget prices HDR by it.
		toneCurve: func() string { return set.str("playout.tone_curve") },
		// fillerDir belongs to the immutable generation layout. Changing storage roots
		// is applied only after the generation drains and rebuilds (§10); `pods` is
		// assigned after the pod adapter is built further down.
		fillerDir: fillerLayout.ClipDir(),
		// The capability probe runs lazily on the first program that needs it, when
		// playout.encoder is unset — so a box with a working GPU uses it instead of
		// silently falling back to software.
		ffmpegPath:     func() string { return set.str("playout.ffmpeg_path") },
		capabilityRoot: func() string { return stateDir },
		// GPU name for the encoder chooser's vendor-native hint (Detect). Read via the LLM
		// package's thin nvidia-smi wrapper — the same GPU signal the rest of the app probes —
		// so playout picks NVENC on an NVIDIA card rather than young cross-vendor Vulkan. Called
		// once, lazily, inside the memoised capability probe; "" (unknown GPU) is a fine default.
		// Bound the external identity command because this cheap check can run on the first tune.
		gpuName: func() string {
			ctx, cancel := context.WithTimeout(rootCtx, playoutGPUIdentityTimeout)
			defer cancel()
			return llm.GPUName(ctx)
		},
		// Preferred audio language (§9.1), read live so a Settings change applies to the
		// next programme rather than the next restart. The prober derives ffprobe from the
		// ffmpeg path — the two ship together, so an operator who moved one moved both.
		audioLanguage:      func() string { return set.str("playout.audio_language") },
		inventory:          inventory.New(st),
		probeSource:        playout.FFprobeSourceNextTo(set.str("playout.ffmpeg_path"), deps.processDiagnostics),
		probeTracks:        playout.FFprobeTracksNextTo(set.str("playout.ffmpeg_path"), deps.processDiagnostics),
		probeFormat:        playout.FFprobeFormatNextTo(set.str("playout.ffmpeg_path"), deps.processDiagnostics),
		probeCopyStart:     playout.FFprobeCopyStartNextTo(set.str("playout.ffmpeg_path"), deps.processDiagnostics),
		processDiagnostics: deps.processDiagnostics,
		// Live read of `library.path_map` (§15, V47), parsed each call so a mapping edit
		// applies without a restart — the same hot-apply posture as audioLanguage.
		pathMap: func() library.PathMap { return library.ParsePathMap(set.str("library.path_map")) },
		log:     log,
	}
	// Loomarr measures each source itself, once per revision, so playout never asks the media
	// server or re-probes a file at airtime (beta.8 G7). One worker, background priority, tied to
	// the app's lifetime; sources are read directly through the path map, never through Emby.
	ffmpegBin := set.str("playout.ffmpeg_path")
	measurer := playoutRes.newMeasurer(
		mediameasure.DefaultTools(ffmpegBin, playout.FFprobeBeside(ffmpegBin)), st)
	playoutRes.measurer = measurer
	playoutRes.analyses = st
	go measurer.Run(rootCtx)
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
		// After Build: it reads the channel engine, whose pods the filler subsystem sets later.
		owner.goRunAfterBuild(func(ctx context.Context) {
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

	resourceBudget.WithMemoryGate(encodeMemoryGate(
		media.HostMemAvailable,
		func() int { return set.intv("playout.memory_reserve_mb") },
		func() int { return set.intv("playout.encode_memory_mb") },
		playoutRes.EncodeHostBytes,
	))

	// The capacity probe (#1512 G5/G11) runs at boot, off the critical path: until it publishes, the
	// budget keeps the whole-stream measurement. Its tone-map self-check reports to Current Health.
	if deps.capacityProbe {
		playoutRes.onTonemap = func(check playout.TonemapCheck) {
			if obs, ok := tonemapObservation(check); ok {
				deps.startup.Observe(diagnostics.StartupCheckPlayoutTonemap, obs)
			}
		}
		deps.startup.Complete(diagnostics.StartupCheckPlayoutTonemap, diagnostics.StartupSkipped,
			"checked by Current Health once the capacity probe finishes", "/settings/system/playback", "")
		// Background work: the probe yields to a viewer the moment a live transcode is admitted.
		owner.goRun(func(ctx context.Context) { playoutRes.probeCapacity(ctx, resourceBudget) })
	} else {
		deps.startup.Complete(diagnostics.StartupCheckPlayoutTonemap, diagnostics.StartupSkipped,
			"capacity probe disabled", "", "")
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
	// Live playout is the channel packager (#1512 phase 2): one encoder per scheduled item,
	// stitched in-process into gapless fMP4 HLS for browsers and one continuous MPEG-TS per
	// media-server tuner. One packager per watched channel, admitted against the transcode
	// budget; viewers of a running channel join it.
	//
	// There is no boot warm-up of the output profile (the encoder evidence and four
	// `ffmpeg -filters` probes, ~0.2 s of the first tune after a restart): idle channels start no
	// FFmpeg, and those probes are FFmpeg runs.
	tonemap, gpuFilters := playout.TonemapperFor(set.str("playout.ffmpeg_path")), playout.GPUFiltersFor(set.str("playout.ffmpeg_path"))
	if pk, perr := playout.NewPackagerHLS(packagerSource{
		res:     playoutRes,
		tonemap: tonemap,
		gpu:     gpuFilters,
		// Filler loudness (#1512 G6), read live so a changed target applies at the next clip.
		targetLUFS: func() string { return set.str("filler.target_lufs") },
		watermark:  deps.watermarks.For,
		log:        log,
	}, set.str("playout.ffmpeg_path"), set.str("playout.hls_dir"), playout.DefaultGrace, log); perr != nil {
		log.Warn("internal playout: channel packager unavailable — live playout disabled", "err", perr)
	} else {
		// One lease per running channel packager in the ResourceBudget ledger (#1520).
		packagedHLS = pk.WithBudget(resourceBudget).WithObserver(deps.metrics)
		// A committed internal Desired-cycle change retires the packager reading the previous
		// cycle; the next tune starts from the new wall-clock position. Peer Postgres replicas
		// receive the same cutover through durable invalidations.
		channelEngine.WithScheduleInvalidator(packagedHLS)
		// A packager starting or stopping is a structural change the dashboard sees at once over
		// the SSE bus (§8); GET /v1/playout/sessions stays the truth. The payload is the count.
		packagedHLS.OnChange(func() {
			eventBus.Publish(events.Event{Type: "playout", Payload: api.PlayoutEvent{Active: packagedHLS.ActiveCount()}})
		})
		playoutObserver = packagedHLS
		owner.addStop(func(context.Context) error {
			packagedHLS.Stop()
			return nil
		})
	}
	origin := playout.NewOrigin(playout.OriginDependencies{
		Packager: packagedHLS,
		Available: func() bool {
			return lifecycleGate == nil || lifecycleGate.Available()
		},
		Eligible: durablePlayoutEligibility,
		Still:    playout.FFmpegStill(set.str("playout.ffmpeg_path"), deps.processDiagnostics),
		// A cold channel's still: one frame of the airing on now, decoded from its source (#1512).
		StillAiring: playoutRes.StillAiring,
		SourceStill: playout.FFmpegSourceStill(set.str("playout.ffmpeg_path"),
			playout.TonemapperFor(set.str("playout.ffmpeg_path")), deps.processDiagnostics),
	})
	owner.addQuiesce(func(context.Context) error {
		origin.Quiesce()
		return nil
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
		return liveTVURLsFor(deps.programmer, target, set.str("server.public_url"), tok, log), nil
	}
	builtBackendController, err := buildBackendTransition(rootCtx, backendTransitionDependencies{
		store: st, fleet: channelEngine,
		publisher: &backendPublisher{connector: liveTVConnector, urls: backendURLs, log: log},
		cutover:   inheritedInternalCutover{channels: st, playout: playoutSvc},
		desired:   resolveDesiredBackend,
	})
	if err != nil {
		return playoutBuild{}, err
	}
	backendController = builtBackendController
	if lifecycleGate != nil {
		lifecycle := &postgresPlayoutLifecycle{
			store: st, checkpoint: backendView, origin: origin, gate: lifecycleGate, log: log,
		}
		if err := lifecycle.StartTracked(rootCtx, owner); err != nil {
			return playoutBuild{}, fmt.Errorf("start postgres playout lifecycle: %w", err)
		}
	}
	// The ladder inputs (tier/encoder) are called UNGUARDED by Profile, so leaving one
	// unset is a panic when a viewer tunes in. `Profile` is
	// invoked by the packager for each channel it starts. Build captures the concrete resolver
	// on the returned generation, which lets package tests assert the real wiring without
	// mutable package state crossing concurrent builds.
	capturePlayoutResolver(playoutRes)
	playoutGuideSvc = playoutRes
	log.Info("internal playout registered",
		"ffmpeg", set.str("playout.ffmpeg_path"), "max_channels_cap", set.intv("playout.max_channels"))

	// server.public_url is the base a media client dials for HLS segments. Internal playout
	// is the default backend, and an unset public URL makes channels appear in the guide but
	// fail at tune time (playout.go returns 503). The first-run wizard requires this (#387),
	// but an env-only, restored-DB, or API-driven install bypasses the wizard entirely and
	// otherwise gets no signal until a viewer hits the dead channel. Warn at boot so the
	// operator sees it in the logs, not in a support ticket. (§9.1; beta-readiness D-4.)
	if internalPlayoutNeedsPublicURL(set.str("server.public_url")) {
		log.Warn("internal playout: server.public_url is unset — in-app playback still works, but a media server or Tunarr cannot fetch channel streams; set SERVER_PUBLIC_URL (or server.public_url) to this instance's reachable base URL")
	}

	return playoutBuild{
		observer: playoutObserver, capability: playoutRes.PublishedCapability,
		service:         playoutSvc,
		resolverService: playoutResolverSvc, guide: playoutGuideSvc,
		resolver: playoutRes, backendController: backendController,
		setResidentVRAM: func(probe func(context.Context) (float64, string)) { residentVRAM = probe },
		budget:          resourceBudget,
	}, nil
}

// defaultEncodeHostBytes is the floor for one hardware encode's host memory. Real preparation
// children held 0.8–1.4 GiB resident (0.55–1.05 GiB excluding shared driver libraries) in the
// incident that motivated the gate, while the capability probe's synthetic trial peaked at ~0.25 GiB
// on the same host: it encodes testsrc and decodes no real file. The probe can therefore only raise
// the estimate, never lower it below this floor.
const defaultEncodeHostBytes = int64(1) << 30

// encodeMemoryGate bounds hardware encodes by host memory (design §9.1). The reserve and per-encode
// settings are MiB and re-read on every lease; a zero reserve disables the gate. A positive
// per-encode setting is used exactly; otherwise the cost is the larger of the probe's measurement
// and defaultEncodeHostBytes.
func encodeMemoryGate(
	available func() (int64, bool), reserveMiB, perEncodeMiB func() int, measured func() int64,
) media.MemoryGate {
	return media.MemoryGate{
		Available: func() (int64, bool) {
			if reserveMiB() <= 0 {
				return 0, false // disabled: report unknown so the gate stays open
			}
			return available()
		},
		Reserve: func() int64 { return int64(reserveMiB()) << 20 },
		PerEncode: func() int64 {
			if v := perEncodeMiB(); v > 0 {
				return int64(v) << 20
			}
			return max(measured(), defaultEncodeHostBytes)
		},
	}
}

// playoutBudgetFacts composes one admission's facts. Once the class probe has measured this host
// (costs), admission is per class and rung, capped by the probed encoder session limit, and the
// operator cap and VRAM shading apply to the measured ceiling. Until then each transcode costs one
// of the measured whole-stream capacity (operator cap and VRAM shading included), and a host that
// measured at most one stream starts sessions on the bottom rung.
func playoutBudgetFacts(
	host playout.HostCPU, hardware bool, measured int, costs *playout.MeasuredCosts, effective func(int) int,
	tier playout.Tier, gpuMillicores, appReserveMillicores int,
) playout.BudgetFacts {
	rungs := playout.LadderHeights(tier)
	facts := playout.BudgetFacts{
		Hardware:         hardware,
		CPUAllowance:     playout.PlayoutCPUAllowance(host, hardware, gpuMillicores, appReserveMillicores),
		CPUSource:        host.Source,
		Rungs:            rungs,
		MeasuredCapacity: measured,
	}
	if costs != nil {
		facts.Costs, facts.SessionLimit = costs.Costs, costs.SessionLimit
		if facts.Costs == nil {
			facts.Costs = map[playout.CostKey]playout.ClassCost{} // completed probe, no successful class
		}
		if ceiling := facts.Ceiling(); ceiling > 0 && effective != nil {
			facts.OperatorCap = effective(ceiling)
		}
		return facts
	}
	if measured <= 1 {
		facts.FirstRung = len(rungs) - 1
	}
	return facts
}
