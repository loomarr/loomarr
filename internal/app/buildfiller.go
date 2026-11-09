package app

import (
	"context"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"github.com/loomarr/loomarr/internal/app/playoutresolve"
	"github.com/loomarr/loomarr/internal/filler/capability"
	"github.com/loomarr/loomarr/internal/filler/cliplibrary"
	"github.com/loomarr/loomarr/internal/filler/language"
	"github.com/loomarr/loomarr/internal/filler/pod"
	"github.com/loomarr/loomarr/internal/filler/records"
	"github.com/loomarr/loomarr/internal/filler/storeadapter"
	"github.com/loomarr/loomarr/internal/filler/structure/cut"
	"github.com/loomarr/loomarr/internal/media/tools"
	"github.com/loomarr/loomarr/internal/platform/settings"
	"github.com/loomarr/loomarr/internal/platform/store/channelstore"

	"github.com/loomarr/loomarr/internal/channels/schedule"
	"github.com/loomarr/loomarr/internal/filler"
	"github.com/loomarr/loomarr/internal/filler/defectfloor"
	"github.com/loomarr/loomarr/internal/filler/fetch"
	"github.com/loomarr/loomarr/internal/filler/screening"
	archiveprovider "github.com/loomarr/loomarr/internal/filler/source/archive"
	youtubeprovider "github.com/loomarr/loomarr/internal/filler/source/youtube"
	"github.com/loomarr/loomarr/internal/filler/station"
	"github.com/loomarr/loomarr/internal/library"
	"github.com/loomarr/loomarr/internal/media/encode"
	"github.com/loomarr/loomarr/internal/media/images"
	"github.com/loomarr/loomarr/internal/platform/diagnostics"
	"github.com/loomarr/loomarr/internal/platform/llm"
	"github.com/loomarr/loomarr/internal/platform/metrics"
	"github.com/loomarr/loomarr/internal/platform/storagegovernor"
	"github.com/loomarr/loomarr/internal/platform/store"
	"github.com/loomarr/loomarr/internal/playout"
	"github.com/loomarr/loomarr/internal/playout/tunarr"
)

// This file holds the composition root's per-subsystem builders (§14.1).
//
// ⚠ **The shape matters, and it is NOT the one §14.1 rejected.** That rejection was for methods
// on a shared builder struct, which would convert ~70 composition locals into fields on a
// mutable carrier — widening their scope and trading compile-time use-before-assignment errors
// for runtime nils. A `buildX` FUNCTION takes only what it needs and RETURNS values, so nothing
// widens and the compiler still catches use-before-assignment. §14.1 records the distinction.
//
// Coupling, not size, predicts extraction cost — extract along the seam, not the line count.

// offerCuratedSources offers this build's curated sources the install has not been offered, switched
// off (#2035). Enabling one is the operator's consent to fetch from it. A failure costs only the
// offer, never the boot: the next start tries again.
func offerCuratedSources(ctx context.Context, st records.Store, log *slog.Logger) {
	if st == nil {
		return
	}
	list, err := filler.CuratedSources()
	if err != nil {
		log.Error("filler: curated sources unreadable", "err", err)
		return
	}
	added, err := st.OfferCuratedSources(ctx, list, time.Now())
	if err != nil {
		log.Warn("filler: curated sources not offered", "version", list.Version, "err", err)
		return
	}
	if added > 0 {
		log.Info("filler: curated sources offered, switched off", "version", list.Version, "added", added)
	}
}

// buildSyncer constructs the catalog syncer and its scan sources (§10 V38c).
//
// ⚠ Policy remains live, while the storage layout is immutable for this application generation.
// The source switch and minimum duration therefore remain closures, but scan and intake share the
// one captured root/watch pair so a settings write cannot move files between generations.
//
// ⚠ The library scanner stays wired when no media server is configured, and that is a SUPPORTED
// install rather than a degraded one: its adapter maps the explicit unconfigured result to an
// empty optional source, so folder rows still drain and a saved connection enables the next scan.
func buildSyncer(st records.Store, set resolved, layout cliplibrary.Layout, log *slog.Logger,
	fillerProg tunarr.Adapter, lib *library.Client, governor *storagegovernor.Governor) *filler.Syncer {
	src := cliplibrary.DirSource{
		Layout: layout,
		Probe:  cliplibrary.FFprobeNextTo(settings.PlayoutFFmpegPath.Get(set)),
		// ⚠ **Artwork must be given `playout.ffmpeg_path`**: its nil default shells out to whatever
		// `ffmpeg` PATH resolves to, so an operator who points that setting at a custom build
		// would get frames from a DIFFERENT binary than playout uses, silently.
		Artwork: cliplibrary.FFmpegArtwork(settings.PlayoutFFmpegPath.Get(set)),
		Storage: governor,
		// The quality gate's floor (§10 V40).
		MinDuration: func() time.Duration { return settings.FillerMinDuration.Get(set) },
		// ⚠ **Log must be assigned**: the "some thumbnails could not be generated" warning exists
		// because extraction is best-effort and failures are skipped (the shape that once produced
		// a silently-empty catalog, see FFprobeNextTo). A generator that counts failures into a
		// logger nobody wired is the same silence with extra steps.
		Log: log.Warn,
	}
	// Keep the adapter present across the process lifetime, but resolve availability per
	// call. An install may start internal-only and add Tunarr later; boot-gating this field
	// left every clip without a Tunarr program uuid until restart. The adapter itself no-ops
	// while the URL is empty, so local-only scans stay quiet and fully supported.
	src.Tunarr = fillerSourceAdapter{
		prog:       fillerProg,
		configured: func() bool { return settings.TunarrURL.Get(set) != "" },
	}

	syncer := filler.NewSyncer(src, storeadapter.NewSync(st), layout, time.Now, log).
		WithEnabled(func() bool { return settings.FillerSourceFolderEnabled.Get(set) }).
		WithAcquisitionManifests(st).
		WithStorageGovernor(governor).
		WithInferiorCopies(st)

	libScanner := cliplibrary.NewLibraryScanner(func(msg string, args ...any) { log.Warn(msg, args...) })
	cutover := &fillerLibraryCutover{st: st, roots: st, lib: lib, log: log, now: time.Now, warned: map[string]bool{}}
	return syncer.WithScanSources(fillerScanSourceAdapter{st: st, cutover: cutover}, libScanner)
}

// buildFetcher constructs the clip downloader (§10, §16). Nil when ffmpeg is absent, which is a
// normal state on `loomarr:latest` rather than an error — the `ingest` feature gate reports it.
//
// ⚠ **TWO downloaders, resolved INDEPENDENTLY** (§10 V38b, wiring fixed V38c.8). archive.org is
// fetched over plain HTTP and needs only ffmpeg; yt-dlp is for YouTube and shells out to ffmpeg
// itself. Requiring BOTH was the V38b defect surviving in the wiring after the feature GATE was
// split, and the result was worse than the original bug: `features.ingest` reported true
// (correctly — ffmpeg is present), the Sources rows offered "Fetch now", and every archive fetch
// then failed at the point of use with "ingest tooling not present in this image". Two claims
// that cannot both be true, which is the exact shape that started V38b.
//
// ⚠ An UNSET path falls back to a PATH lookup, matching `settings.toolRunnable` — §15
// describes these as defaulting to the vendored binaries, and only the Docker image set them, so
// a source build had ingest off with the tools installed.
func buildFetcher(set resolved, layout cliplibrary.Layout, log *slog.Logger, artifacts fetch.ArtifactWriter, governor *storagegovernor.Governor) *fetch.Ingestor {
	ytPath := resolveTool(settings.IngestYtdlpPath.Get(set), "yt-dlp")
	ffPath := resolveTool(settings.IngestFFmpegPath.Get(set), "ffmpeg")
	if ffPath == "" {
		return nil
	}

	// ⚠ A missing YouTube downloader is FINE — the Ingestor finds none registered for that
	// provider and counts that source as failed rather than dying. So a box with ffmpeg and no yt-dlp fetches
	// archive collections (the seeded ones) and reports honestly on playlists, instead of
	// refusing everything.
	downloaders := fetch.Downloaders{archiveprovider.ID: fetch.NewArchiveDownloader()}
	if ytPath != "" {
		downloaders[youtubeprovider.ID] = fetch.NewYtDlpDownloader(ytPath, ffPath)
	}
	log.Info("filler ingest available", "ytdlp", orNone(ytPath), "ffmpeg", ffPath)
	return fetch.New(downloaders, layout.WatchDir(), artifacts, log).
		WithStorageGovernor(governor)
}

// buildSplitter constructs the compilation splitter (§10, V34). Nil without a drop-folder — clip
// paths are relative to it, so there is nothing to cut.
//
// ⚠ ffmpeg/ffprobe come from `playout.ffmpeg_path`, a core runtime dep on the single image, NOT
// from the ingest pair. Splitting works on files already on disk and must not die just because
// yt-dlp is absent. whisper is optional: without it, over-long segments come back Unsplittable
// rather than guessed (§15).
//
// The LLM provider wires whenever one is configured because splitting is operator-invoked.
func buildSplitter(st records.Store, set resolved, layout cliplibrary.Layout, log *slog.Logger, wake *fillerChannelWake, recorder *metrics.Recorder, governor *storagegovernor.Governor, imageService *images.Service) *filler.Splitter {
	dir := layout.ClipDir()
	if dir == "" {
		return nil
	}

	splitProvider := activeFillerProvider(set, recorder)

	tools := buildFillerMediaTools(set, recorder)

	// The same live minimum is enforced during detection and at the scan boundary (§10 V34).
	splitter := filler.NewSplitter(storeadapter.NewSplit(st, wake), tools, splitProvider, dir,
		func() time.Duration { return settings.FillerMinDuration.Get(set) }, newID, time.Now, log).
		WithStorageGovernor(governor)
	if imageService != nil {
		splitter.WithSplitArtwork(fillerSplitArtworkAdapter{images: imageService})
	}
	return splitter
}

// activeFillerProvider resolves the same branded provider selection as the AI surface. OpenRouter
// credentials live under llm.api_key.openrouter rather than the legacy base key; every filler text
// path must therefore build from Selection instead of reconstructing the wire from registry rows.
func activeFillerProvider(set resolved, recorder *metrics.Recorder) llm.Provider {
	sel := resolveSelection(set)
	if sel.URL == "" {
		return nil
	}
	return buildProviderFor(sel, recorder)
}

// buildFillerMediaTools selects local whisper or hosted timed transcription behind the same
// MediaTools interface. Every selector is a closure: changing provider, model, URL or key applies
// to the next span without restarting, matching the rest of the filler settings contract.
func buildFillerMediaTools(set resolved, recorder *metrics.Recorder) *tools.FFmpegTools {
	ffmpegPath := settings.PlayoutFFmpegPath.Get(set)
	mediaTools := tools.NewFFmpegTools(ffmpegPath, cliplibrary.FFprobePathNextTo(ffmpegPath),
		settings.IngestWhisperPath.Get(set), settings.IngestWhisperModel.Get(set), "")
	hosted := &tools.HostedTranscriber{
		FFmpegPath: ffmpegPath,
		Client: func() tools.AudioTranscriptionClient {
			return hostedSpeechClient(set, recorder)
		},
		Model: func() string { return settings.ASRModel.Get(set) },
	}
	return mediaTools.WithTranscriber(func() tools.SpanTranscriber {
		if settings.ASRProvider.Get(set) != "hosted" {
			return nil
		}
		return hosted
	})
}

// intakeLanguages is the language gate's wanted set, read live (#2030 decision 13): the union of
// the channels' commercial languages (language.IntakeSet). A detached channel no longer airs,
// so it no longer counts; a paused one resumes with its language, so it does.
func intakeLanguages(st interface {
	ListChannels(context.Context) ([]channelstore.Channel, error)
}, set resolved) func(context.Context) (string, error) {
	return func(ctx context.Context) (string, error) {
		channels, err := st.ListChannels(ctx)
		if err != nil {
			return "", err
		}
		own := make([]string, 0, len(channels))
		for _, ch := range channels {
			if ch.Status == schedule.StatusDetached {
				continue
			}
			lang := ""
			if ch.Policy.Filler != nil {
				lang = ch.Policy.Filler.Language
			}
			own = append(own, lang)
		}
		return language.IntakeSet(settings.FillerLanguage.Get(set), own), nil
	}
}

// buildPipeline constructs the ingest pipeline: one driver over ten rungs (§10 V51b/V61).
//
// The external inputs are the six parameters below: `langDetect`, `fillerTools`, `fillerDrop`,
// `visionProvider` and `clipDir` are locals of this unit, not couplings to the rest of the
// composition root. A coupling count is a property of the BOUNDARY, so measure from the first
// line the unit owns.
//
// ⚠ Every rung is registered UNCONDITIONALLY, even when the thing it needs is absent — a stage
// missing from the slice reports "not available on this install" on every clip's ladder, while
// one that is present says why it skipped in the operator's own terms. That is what makes the
// ladder explain an install rather than merely show gaps in it. Do not make registration
// conditional to "clean up" the nil cases.
func buildPipeline(st records.Store, set resolved, layout cliplibrary.Layout, log *slog.Logger, emitter *eventEmitter,
	splitter *filler.Splitter, wake *fillerChannelWake,
	processDiagnostics *diagnostics.ProcessManager, storageGovernor *storagegovernor.Governor,
	recorder *metrics.Recorder, headroom filler.PlaybackHeadroom,
	playoutRes *playoutresolve.Resolver, checkReplacement filler.ReplacementCheck,
	interactiveLLM *llm.InteractiveGate) (*filler.Pipeline, *filler.TranscribeStage, *filler.VisionStage) {
	// Filler encodes on the channels' own hardware (#2121), asked per encode: detection settles
	// after boot.
	encodeHost := func() encode.HostProfile {
		return playoutRes.FileEncodeHost(encode.Encoder(settings.PlayoutEncoder.Get(set)))
	}
	// The language gate (§10 V40). Registered unconditionally: no intake language (no household
	// language and no channel naming one) makes Run a no-op, so an install that has not opted in
	// pays nothing and the Tasks row still exists to be seen and paused.
	//
	// ⚠ The DETECTOR is chosen at construction, not per run, because the two backends need
	// entirely different collaborators — one a local binary and a model file, the other an
	// HTTP client with a key. The wanted languages and the reject rule still hot-apply; changing
	// the PROVIDER needs a restart, which is the same bargain `llm.provider` makes.
	wantLanguages := intakeLanguages(st, set)
	var langDetect language.Detector
	if settings.ASRProvider.Get(set) == "hosted" {
		// Language and transcripts share one speech-recognition service. A dedicated ASR endpoint
		// should not need a second chat model that happens to accept audio just to answer the same
		// question. The closures keep URL, model and credential changes live between clips.
		langDetect = language.NewHosted(
			func() tools.AudioTranscriptionClient { return hostedSpeechClient(set, recorder) },
			func() string { return settings.ASRModel.Get(set) },
			settings.PlayoutFFmpegPath.Get(set), "")
	} else {
		// ⚠ `filler.language_model`, NOT `ingest.whisper_model`. The latter is
		// `ggml-small.en.bin` — an ENGLISH-ONLY build that does not identify languages at all,
		// so pointing this at it would answer "en" for every clip on earth and the gate would
		// silently never reject anything.
		langDetect = language.NewWhisper(
			settings.IngestWhisperPath.Get(set), settings.FillerLanguageModel.Get(set),
			settings.PlayoutFFmpegPath.Get(set), "")
	}
	// The ffmpeg tooling the metadata rungs share (a core runtime dep — NOT the ingest pair, so
	// they run on files already on disk regardless of whether yt-dlp is present).
	fillerTools := buildFillerMediaTools(set, recorder)

	// The generation's clip-root FS reads the info-JSON sidecars ingest writes beside each clip.
	// nil ⇒ every clip reads as thin-sourced and filename-only tagged.
	fillerDrop := layout.FS()

	// Vision: keyframes → a multimodal model, resolved LIVE (§10 V54a).
	//
	// The provider is wired even while the capability is off; the enabled closure still prevents
	// calls. That lets a later opt-in wake Ready clips without a restart, while the endpoint itself
	// remains resolved per call by `capability.HotVision`, so changing the provider, URL, key or model
	// applies to the next clip. Gating construction on a configured URL would make an install with
	// vision enabled and no reachable endpoint indistinguishable from one with vision switched off.
	h := capability.NewHotVision(set, mainSelection(set), log, recorder)
	// Background vision (the vision rung and split-time grounding) yields each model call to an
	// interactive suggestion in flight, so Add a channel gets the AI server's slots first (#1508).
	visionProvider := filler.YieldToInteractive(h, interactiveLLM, filler.DefaultInteractiveYield(), log)
	if settings.FillerVisionEnabled.Get(set) {
		// Resolve ONCE at boot purely to report it. A failure here is not fatal — the operator may
		// fix the setting without restarting, which is the entire point of resolving per call —
		// but it must not be silent, because this is the diagnosis for "grounding never runs".
		if err := h.Check(); err != nil {
			log.Warn("filler vision enabled but not currently reachable; grounding will no-op until this is fixed",
				"err", err)
		}
	}
	// ── The ingest pipeline (§10 V51b) ────────────────────────────────────────────────────
	//
	// One driver over ten rungs.
	//
	// Every rung is registered unconditionally (see buildPipeline): `Applies` answers WHY a rung
	// skipped ("vision tagging is off", "no language backend is configured"), which is the same
	// visible-but-idle contract the Tasks page rows use.
	clipDir := layout.ClipDir()
	screeningRuntime, screeningErr := buildQualificationSegmentScreeningRuntime(st, layout)
	if screeningErr != nil {
		log.Error("rendered-child screening qualification runtime was not activated", "err", screeningErr)
	}
	transcribeStage := filler.NewTranscribeStage(fillerTools, storeadapter.NewTranscribe(st), clipDir, fillerDrop,
		func() bool { return settings.FillerTranscribeEnabled.Get(set) }, time.Now)
	visionStage := filler.NewVisionStage(fillerTools, visionProvider, storeadapter.NewVision(st), clipDir,
		func() bool { return settings.FillerVisionEnabled.Get(set) }, time.Now)
	transcodeStage := filler.NewTranscodeStage(
		storeadapter.NewPipelineClips(st), cliplibrary.FFprobeNextTo(settings.PlayoutFFmpegPath.Get(set)),
		clipDir, tools.DefaultMezzanine(),
		func() string { return settings.PlayoutFFmpegPath.Get(set) },
		// The playback target, for the too_quiet floor only: files are never levelled on
		// disk, playout corrects them at air time (#2032).
		func() float64 {
			// ⚠ `filler.target_lufs` is a STRING setting (an empty value means "no
			// correction at all", which no numeric kind can express), so it is parsed
			// here. An unparseable value yields 0, meaning no target and no floor.
			lufs, err := strconv.ParseFloat(settings.FillerTargetLufs.Get(set), 64)
			if err != nil {
				return 0
			}
			return lufs
		}, time.Now).WithMediaDerivatives().WithConditioning(fillerTools.MeasureConditioning).
		WithDiagnostics(processDiagnostics).WithStorageGovernor(storageGovernor).
		WithEncodeHost(encodeHost).
		WithAiringReplacement(storeadapter.NewPipelineClips(st), checkReplacement)
	pipelineStages := []filler.Stage{
		filler.NewProbeStage(
			cliplibrary.FFprobeNextTo(settings.PlayoutFFmpegPath.Get(set)), storeadapter.NewPipelineClips(st), clipDir,
			func() int64 { return settings.FillerMinDuration.Get(set).Milliseconds() },
			func() time.Duration { return settings.FillerAutosplitMaxDuration.Get(set) }, time.Now),
		transcodeStage,
		// Rendered compilation children must not reach enrichment or the compatibility score gate
		// until the five certified authorities are wired. The qualification runtime records the
		// exact rights and playback answers plus explicit holds for the three uncertified safety
		// axes. Terminal release remains nil, so no qualification aggregate can authorize airplay.
		screening.NewSegmentScreeningStage(screeningRuntime, nil, clipDir),
		defectfloor.NewDefectsStage(storeadapter.NewDefects(st)),
		filler.NewLanguageStage(langDetect, storeadapter.NewLanguage(st), clipDir, wantLanguages, time.Now),
		transcribeStage,
		visionStage,
		filler.NewFinishStage(),
	}
	if splitter != nil {
		// A compilation can contain adverts in several languages, so the whole-clip language rung
		// cannot make this decision. Attach the same detector and intake languages to the
		// splitter: both scheduled and explicitly requested proposals finish bounded per-segment
		// checks before becoming reviewable, and confirmed children reuse the result.
		splitter.WithSegmentLanguage(filler.SegmentLanguagePolicy{
			Detector: langDetect,
			Want:     wantLanguages,
			Budget:   func() int { return settings.FillerPipelineMaxWhisper.Get(set) },
		})
		autoSplitPolicy := &filler.AutoSplitPolicy{
			Enabled:       func() bool { return settings.FillerAutosplitEnabled.Get(set) },
			MinConfidence: func() int { return settings.FillerAutosplitMinConfidence.Get(set) },
			MaxDuration:   func() time.Duration { return settings.FillerAutosplitMaxDuration.Get(set) },
		}
		minClipDuration := func() time.Duration { return settings.FillerMinDuration.Get(set) }
		// The two long-reel files apply live (#1659): each path edit builds the whole runtime
		// afresh, and the split stage reads it once per split.
		structureRuntime := &liveStructureRuntime{
			paths: func() (string, string) {
				return settings.FillerStructureWindowAuthorityPath.Get(set), settings.FillerStructureWindowDeploymentPath.Get(set)
			},
			build: func(authorityPath, deploymentPath string) filler.StructureRuntime {
				materialization := &filler.StructureMaterializationPolicy{}
				rt := filler.StructureRuntime{Materialization: materialization}
				policyVersion := "production-shadow-no-certified-slices-v1"
				windowAuthority, authorityErr := loadWindowStructureAuthority(authorityPath)
				if authorityErr != nil {
					log.Error("long-reel materialization authority was not loaded; certified splitting remains disabled", "err", authorityErr)
				}
				windowDeployment, deploymentErr := loadWindowStructureDeployment(deploymentPath, windowAuthority)
				if deploymentErr != nil {
					log.Error("long-reel deployment was not loaded; structure inference and certified splitting remain disabled", "err", deploymentErr)
				}
				cutFamily := func() encode.Family { return cut.CutFamily(encodeHost()) }
				windowRuntime, runtimeErr := buildCertifiedWindowStructureRuntime(st, set, layout, windowAuthority, windowDeployment, cutFamily)
				if runtimeErr != nil {
					log.Error("long-reel runtime was not activated; structure inference and certified splitting remain disabled", "err", runtimeErr)
				} else if windowRuntime != nil {
					// ⚠ Only on success: a failed build returns a typed nil that compares non-nil.
					rt.Decisioner = windowRuntime
					materialization.WindowAuthority = windowAuthority
					policyVersion = "production-window-authority-" + windowAuthority.SHA256[:12] + "-deployment-" + windowDeployment.SHA256[:12]
					log.Info("certified long-reel runtime activated", "authority_sha256", windowAuthority.SHA256,
						"deployment_sha256", windowDeployment.SHA256, "minimum_source_ms", windowAuthority.MinimumSourceDurationMS,
						"maximum_source_ms", windowAuthority.MaximumSourceDurationMS)
				}
				// The observer records compatibility beside the complete-plan answer for measurement
				// only. Missing or malformed authority leaves application proposals held; it never
				// restores compatibility as application authority.
				structureShadow, shadowErr := filler.NewStructureSplitShadow(st, autoSplitPolicy, materialization, minClipDuration, policyVersion)
				if shadowErr != nil {
					log.Error("could not construct filler structure split shadow", "err", shadowErr)
				}
				// Attach even when construction returned a nil typed pointer. Its observer methods
				// fail closed, preserving the rule that an internal wiring fault cannot silently
				// restore unattended compatibility publication.
				rt.Shadow = structureShadow
				return rt
			},
		}
		// Build once at boot so a bad file is reported at startup.
		structureRuntime.Current()
		// ⚠ Appended rather than placed in order — `NewPipeline` indexes the slice by stage id
		// and `StageOrder` is the ONE definition of the sequence, so the order here is
		// irrelevant. Stating that is worth a line, because a slice that looks like a pipeline
		// invites someone to "fix" its order.
		splitStage := filler.NewSplitStage(splitter, storeadapter.NewSplit(st, wake)).
			WithLogger(log).
			WithAutoConfirm(*autoSplitPolicy, minClipDuration).
			// The split-time grounder (§10 V54). Without it the auto-confirm gate has no data
			// and `filler.autosplit.enabled` — default ON — can never fire.
			//
			// ⚠ Gated on the SAME `filler.vision.enabled` the vision rung uses, via a zero
			// budget rather than a nil grounder: an operator who turned vision off did not
			// ask for it back on a different rung, and one switch governing both is what
			// keeps "is Loomarr sending my frames to a model" answerable in one place.
			WithSegmentVision(&filler.SegmentVision{
				Tools:    fillerTools,
				Provider: visionProvider,
				Taxa:     storeadapter.NewVision(st),
				ClipDir:  clipDir,
				Budget: func() int {
					if !settings.FillerVisionEnabled.Get(set) {
						return 0
					}
					return settings.FillerPipelineMaxSplitVision.Get(set)
				},
			})
		// The gate is attached even without a certified runtime. An empty policy then produces an
		// attributable hold instead of allowing the diagnostic compatibility outcome to create
		// children.
		splitStage.WithStructureRuntime(structureRuntime.Current)
		pipelineStages = append(pipelineStages, splitStage)
	}
	fillerPipeline := filler.NewPipeline(st, storeadapter.NewPipelineClips(st), pipelineStages,
		filler.Budget{
			MaxClips: func() int { return settings.FillerPipelineMaxClips.Get(set) },
			// ⚠ A closure RETURNING ZERO means "never transcode on this box", which is a
			// different state from a nil closure meaning "use the default". That three-state
			// encoding is the only way an operator can turn the most expensive rung off.
			MaxTranscodes: func() int { return settings.FillerTranscodeMaxPerRun.Get(set) },
			MaxWhisper:    func() int { return settings.FillerPipelineMaxWhisper.Get(set) },
			MaxVision:     func() int { return settings.FillerPipelineMaxVision.Get(set) },
			MaxSplits:     func() int { return settings.FillerPipelineMaxSplits.Get(set) },
		},
		emitter.FillerClipStage, time.Now, log).
		WithRewind(storeadapter.NewRewind(st), clipDir).
		// Idle passes move the airing library onto the current encode profile (#2121).
		WithLibraryReencode(transcodeStage, storeadapter.NewAiringClips(st))
	if headroom != nil {
		// Media rungs yield to live playback instead of competing with it for CPU (#1512 G5).
		fillerPipeline.WithPlaybackHeadroom(headroom, filler.DefaultPlaybackYield())
	}
	log.Info("filler ingest pipeline registered",
		"stages", len(pipelineStages), "language", settings.FillerLanguage.Get(set),
		"transcribe", settings.FillerTranscribeEnabled.Get(set),
		"vision", settings.FillerVisionEnabled.Get(set),
		"vision_provider", activeFillerVisionCapability(set).Available,
		"autosplit", settings.FillerAutosplitEnabled.Get(set))
	return fillerPipeline, transcribeStage, visionStage
}

// hostedSpeechClient resolves one speech-recognition service for both language detection and timed
// transcripts. A blank dedicated URL reuses the active hosted AI service (the OpenRouter path); an
// explicit URL never inherits that service's secret, so a custom host cannot receive it by accident.
func hostedSpeechClient(set resolved, recorder *metrics.Recorder) tools.AudioTranscriptionClient {
	url := strings.TrimSpace(settings.ASRURL.Get(set))
	model := strings.TrimSpace(settings.ASRModel.Get(set))
	provider := "openai"
	key := settings.ASRAPIKey.Get(set)
	if url == "" {
		sel := resolveSelection(set)
		url, provider, key = sel.URL, sel.Provider, sel.APIKey
	}
	if url == "" || model == "" {
		return nil
	}
	return hostedSTTAdapter{llm.NewOpenAIForProvider(provider, url, model, key).WithMetrics(recorder)}
}

// buildPodAdapter constructs the pod assembler: the thing that picks which commercials fill a
// break, shared by the §12 preview endpoint, the reconciler and internal playout (§10).
//
// ⚠ Construction ONLY. The three lines that wire this into the channel engine, the preview
// adapter and the playout resolver stay at the call site deliberately — they are cross-subsystem
// back-patches (the resolver is built with the channel engine, the adapter needs the filler
// catalog), and pulling them in here would mean handing a builder the two services it exists to
// stay independent of.
func buildPodAdapter(st store.Store, set resolved, calendar func() schedule.Calendar, log *slog.Logger) *pod.Adapter {
	// ⚠ **A CLOSURE, not a struct literal — the whole filler policy is resolved per pod.**
	// A value built here at boot would be a snapshot: writing `filler.pod_max`,
	// `filler.min_quality`, `filler.min_clip_duration` or `filler.max_clip_duration` would
	// change the stored setting and the value the API reads back, while the assembler went on
	// using the old one until the process restarted. Unit tests build a `pod.Policy` literal
	// and call the domain directly, so the setting→policy edge is the one segment they cannot
	// reach; only a live check of a setting change catches a regression here.
	//
	// config-design §3 defines hot-apply for long-lived clients; `AutoSplitPolicy` resolves per
	// call too. This is that contract, honoured by the pod path.
	// The radius is read per call like the rest of the policy; the resolver memoises per radius.
	// A closure, not the method value: the store is read only when a place is set.
	list := func(ctx context.Context) ([]station.Station, error) { return st.ListBroadcastStations(ctx) }
	local := &station.LocalResolver{Stations: station.NewLazy(list),
		RadiusKM: func() int { return settings.FillerLocalRadiusKm.Get(set) }, Log: log}
	podAdapter := pod.NewAdapter(storeadapter.NewCatalog(st), clipExposureAdapter{st}, func() pod.Policy {
		return pod.Policy{
			Geography:       set.homeGeography(),
			Language:        settings.FillerLanguage.Get(set),
			LocalStations:   local.Resolve,
			PodMax:          settings.FillerPodMax.Get(set),
			BreakDurationMs: settings.FillerBreakDuration.Get(set).Milliseconds(),
			// V17c: 0 (the default) means no quality floor — see the warning on
			// Policy.MinQualityHeight.
			MinQualityHeight: settings.FillerMinQuality.Get(set),
			// V51f: the pod-eligibility duration bounds. Both default to 0s = off, so
			// `durationEligible` returns true on an untouched install; setting one is what lets
			// `PoolReport.Eligible` differ from `Commercials` instead of restating it.
			MinClipMs: settings.FillerMinClipDuration.Get(set).Milliseconds(),
			MaxClipMs: settings.FillerMaxClipDuration.Get(set).Milliseconds(),
			Cooldown:  time.Duration(settings.FillerCooldownSeconds.Get(set)) * time.Second,
			// The household calendar programming reads (#2030): a holiday's commercials air only
			// in its season.
			Calendar: calendar(),
		}
	}, log)
	return podAdapter
}

// playbackHeadroomFor is filler's PlaybackHeadroom: the ResourceBudget, the one admission ledger
// (#1512 G5), so filler waits exactly while a live transcode holds a lease. nil when there is no
// internal playout, so the pipeline never waits.
func playbackHeadroomFor(budget *playout.ResourceBudget) filler.PlaybackHeadroom {
	if budget == nil {
		return nil
	}
	return budget
}
