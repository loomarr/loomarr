package app

import (
	"context"
	"io/fs"
	"log/slog"
	"os"
	"strconv"
	"time"

	"github.com/mantonx/loomarr/internal/clipfetch"
	"github.com/mantonx/loomarr/internal/filler"
	"github.com/mantonx/loomarr/internal/library"
	"github.com/mantonx/loomarr/internal/llm"
	"github.com/mantonx/loomarr/internal/programmer"
	"github.com/mantonx/loomarr/internal/store"
)

// This file holds BuildHandler's per-subsystem builders (§14.1).
//
// ⚠ **The shape matters, and it is NOT the one §14.1 rejected.** That rejection was for methods
// on a shared builder struct, which would convert ~70 of BuildHandler's locals into fields on a
// mutable carrier — widening their scope and trading compile-time use-before-assignment errors
// for runtime nils. A `buildX` FUNCTION takes only what it needs and RETURNS values, so nothing
// widens and the compiler still catches use-before-assignment. §14.1 records the distinction.
//
// The measurement that makes this tractable: BuildHandler's 434-line filler section reads only
// EIGHT locals from earlier sections, while its 133-line suggester section reads far more.
// Coupling, not size, predicts extraction cost — extract along the seam, not the line count.

// buildTagger constructs the AI-tagging provider and the manual whole-catalog tagger (§10).
//
// Returns BOTH because they have different lifetimes and different consumers: the provider is
// also handed to the ingest pipeline's tag rung (§10 V51b), which is why it is hoisted out of
// the `if` rather than living inside the tagger. Nil for both is the honest un-opted-in state,
// and every reader treats it that way — the manual sweep becomes a no-op, and the rung reports
// "no language model is configured" on each clip's ladder rather than silently doing nothing.
func buildTagger(st store.Store, set resolved, log *slog.Logger) (llm.Provider, *filler.Tagger) {
	if !set.boolv("filler.ai_tagging") || set.str("llm.url") == "" {
		return nil, nil
	}

	provider := llm.NewProvider(set.str("llm.provider"), set.str("llm.url"), set.str("llm.model"), set.str("llm.api_key"))

	// The drop-folder as an fs.FS so tagging can read the info-JSON sidecars ingest writes
	// beside each clip (§10). An unset FILLER_DIR yields a nil FS and tagging falls back to
	// filenames — the same result as a drop-folder clip that never had a sidecar.
	var drop fs.FS
	if dir := set.str("filler.dir"); dir != "" {
		drop = os.DirFS(dir)
	}

	tagger := filler.NewTagger(fillerTagStoreAdapter{st}, provider, drop, time.Now, log).
		// Auto-filing (§10 V38): a held clip whose grounding-capped score clears the threshold
		// is filed without a human. Closures, not captured values, so a changed threshold
		// applies on the next run rather than the next restart.
		//
		// ⚠ `boolv`, NOT `boolOn`. The two differ only when the settings service cannot answer,
		// and here that difference is the whole safety property: `boolOn` fails OPEN (returns
		// true), which would publish unreviewed clips to live channels exactly when the install
		// is degraded. Holding is the safe failure.
		WithAutoFile(filler.AutoFilePolicy{
			Enabled:       func() bool { return set.boolv("filler.autofile.enabled") },
			MinConfidence: func() int { return set.intv("filler.autofile.min_confidence") },
		})

	return provider, tagger
}

// buildSyncer constructs the catalog syncer and its scan sources (§10 V38c).
//
// ⚠ Every switch here is read LIVE rather than captured, because these settings hot-apply
// (config-design §3): an operator who turns the drop-folder off expects the next scheduled pass
// to stop, not a restart to be required. `Dir`, `MinDuration`, `WithEnabled` and `WithWatchDir`
// are all closures for that reason.
//
// ⚠ The library scanner is nil when no media server is configured, and that is a SUPPORTED
// install rather than a degraded one: folder rows still drain and library rows simply do no
// work. Wiring a non-nil scanner over an absent media server would turn an optional service back
// into a precondition — the dependency §9.1 removed.
func buildSyncer(rootCtx context.Context, st store.Store, set resolved, log *slog.Logger,
	fillerProg *programmer.Tunarr) *filler.Syncer {
	src := filler.DirSource{
		Dir:   func() string { return set.str("filler.dir") },
		Probe: filler.FFprobeNextTo(set.str("playout.ffmpeg_path")),
		// ⚠ **Artwork was relying on its nil default, which ignored `playout.ffmpeg_path`
		// entirely** and shelled out to whatever `ffmpeg` PATH resolved to. An operator who
		// points that setting at a custom build (the whole reason it exists — see the
		// hardware-encode notes) got their frames from a DIFFERENT binary than playout uses,
		// silently, and the setting appeared to do nothing here.
		Artwork: filler.FFmpegArtwork(set.str("playout.ffmpeg_path")),
		// The quality gate's floor (§10 V40).
		MinDuration: func() time.Duration { return set.dur("filler.min_duration") },
		// ⚠ **Log was never assigned either**, so the "some thumbnails could not be generated"
		// warning has never once been emitted. That count exists precisely because extraction is
		// best-effort and failures are skipped — the shape that already produced one
		// silently-empty catalog in this repo's history (see FFprobeNextTo). A generator that
		// counts failures into a logger nobody wired is the same silence with extra steps.
		Log: log.Warn,
	}
	if set.str("tunarr.url") != "" {
		src.Tunarr = fillerSourceAdapter{fillerProg}
	}

	syncer := filler.NewSyncer(src, fillerStoreAdapter{st}, set.str("filler.dir"), time.Now, log).
		WithEnabled(func() bool { return set.boolOn("filler.source.folder.enabled") }).
		// An empty value resolves to `<filler.dir>/_watch`, so the watch folder is configured on
		// every install whether or not the operator has ever set it.
		WithWatchDir(func() string { return set.str("filler.watch_dir") })

	var libScanner *filler.LibraryScanner
	if set.str("library.url") != "" {
		libScanner = filler.NewLibraryScanner(
			fillerLibraryAdapter{library.NewDynamic(
				flavorOrDefault(set), set.libraryConn(), instanceDeviceID(rootCtx, st))},
			func(msg string, args ...any) { log.Warn(msg, args...) },
		)
	}
	return syncer.WithScanSources(fillerScanSourceAdapter{st}, libScanner)
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
// ⚠ An UNSET path falls back to a PATH lookup, matching `settings.toolRunnable` — §15 has always
// described these as defaulting to the vendored binaries, and only the Docker image set them, so
// a source build had ingest off with the tools installed.
func buildFetcher(set resolved, log *slog.Logger) *clipfetch.Ingestor {
	ytPath := resolveTool(set.str("ingest.ytdlp_path"), "yt-dlp")
	ffPath := resolveTool(set.str("ingest.ffmpeg_path"), "ffmpeg")
	if ffPath == "" {
		return nil
	}

	// ⚠ A nil YouTube downloader is FINE — `downloaderFor` returns nil per kind and the Ingestor
	// counts that source as failed rather than dying. So a box with ffmpeg and no yt-dlp fetches
	// archive collections (the seeded ones) and reports honestly on playlists, instead of
	// refusing everything.
	var ytDL clipfetch.Downloader
	if ytPath != "" {
		ytDL = clipfetch.NewYtDlpDownloader(ytPath, ffPath)
	}
	log.Info("filler ingest available", "ytdlp", orNone(ytPath), "ffmpeg", ffPath)
	return clipfetch.New(ytDL, clipfetch.NewArchiveDownloader(false), set.str("filler.dir"), log)
}

// buildSplitter constructs the compilation splitter (§10, V34). Nil without a drop-folder — clip
// paths are relative to it, so there is nothing to cut.
//
// ⚠ ffmpeg/ffprobe come from `playout.ffmpeg_path`, a core runtime dep on the single image, NOT
// from the ingest pair. Splitting works on files already on disk and must not die just because
// yt-dlp is absent. whisper is optional: without it, over-long segments come back Unsplittable
// rather than guessed (§15).
//
// The LLM provider wires whenever one is configured — splitting's rescue and classification are
// operator-invoked, so they are not gated by `filler.ai_tagging`, which gates the batch job.
func buildSplitter(st store.Store, set resolved, log *slog.Logger) *filler.Splitter {
	dir := set.str("filler.dir")
	if dir == "" {
		return nil
	}

	var splitProvider llm.Provider
	if set.str("llm.url") != "" {
		splitProvider = llm.NewProvider(set.str("llm.provider"), set.str("llm.url"), set.str("llm.model"), set.str("llm.api_key"))
	}

	ffmpegPath := set.str("playout.ffmpeg_path")
	tools := filler.NewFFmpegTools(ffmpegPath, filler.FFprobePathNextTo(ffmpegPath),
		set.str("ingest.whisper_path"), set.str("ingest.whisper_model"), "")

	return filler.NewSplitter(fillerSplitStoreAdapter{st}, tools, splitProvider, dir, newID, time.Now, log)
}

// buildPipeline constructs the ingest pipeline: one driver over eight rungs (§10 V51b).
//
// ⚠ **This block was measured as an 11-input seam and skipped on that basis. The measurement
// was wrong, and the way it was wrong is worth keeping.** The window had been drawn at the
// `pipelineStages` slice, so `langDetect`, `fillerTools`, `fillerDrop`, `visionProvider` and
// `clipDir` — all of them DEFINED in the preamble immediately above and referenced nowhere
// else in BuildHandler — counted as inputs crossing the boundary. They are locals of this
// unit, not couplings to the rest of the composition root. Widened to the whole unit, the
// external inputs are the six parameters below, which is `buildSyncer`'s number.
//
// The lesson generalises past this function: a coupling count is a property of the BOUNDARY,
// not of the code, and a boundary drawn mid-unit reports the unit's own locals as coupling.
// Measure from the first line the unit owns, or the number means nothing.
//
// ⚠ Every rung is registered UNCONDITIONALLY, even when the thing it needs is absent — a stage
// missing from the slice reports "not available on this install" on every clip's ladder, while
// one that is present says why it skipped in the operator's own terms. That is what makes the
// ladder explain an install rather than merely show gaps in it. Do not make registration
// conditional to "clean up" the nil cases.
func buildPipeline(st store.Store, set resolved, log *slog.Logger, emitter *eventEmitter,
	splitter *filler.Splitter, taggerProvider llm.Provider) *filler.Pipeline {
	// The language gate (§10 V40). Registered unconditionally: `filler.language` empty makes
	// Run a no-op, so an install that has not opted in pays nothing and the Tasks row still
	// exists to be seen and paused.
	//
	// ⚠ The DETECTOR is chosen at construction, not per run, because the two backends need
	// entirely different collaborators — one a local binary and a model file, the other an
	// HTTP client with a key. `filler.language` and the reject rule still hot-apply; changing
	// the PROVIDER needs a restart, which is the same bargain `llm.provider` makes.
	var langDetect filler.LanguageDetector
	if set.str("filler.language_provider") == "hosted" {
		// ⚠ Its OWN client rather than the tagger's. The tagging provider is built inside
		// `if filler.ai_tagging && llm.url != ""`, so reusing it would silently tie the
		// language gate to a setting that has nothing to do with it — switch AI tagging off
		// and clips would stop being checked, with nothing saying why.
		//
		// ⚠ Nil asker ⇒ the detector reports "cannot tell" and the gate keeps every clip.
		// That is the honest state for an install that selected `hosted` without configuring
		// a key: inert, not broken, and not silently deleting things.
		// ⚠ **CLOSURES, not resolved values.** The first cut called `set.str(...)` here and
		// baked the URL, model and key into a client at boot — so changing `llm.model` in
		// Settings did nothing, the detector kept calling whatever was configured at startup,
		// and every clip failed with a real 404 ("No endpoints found that support input
		// audio") about a request the operator thought they had already fixed. Cost a live
		// debugging session to find, because the error was accurate and the config looked right.
		//
		// Everything else in this feature reads live; the one setting that decides whether the
		// backend can work at all must too.
		langDetect = filler.NewHostedLanguage(
			func() filler.AudioAsker {
				url := set.str("llm.url")
				if url == "" {
					return nil // not configured ⇒ the gate keeps every clip
				}
				return audioAskerAdapter{llm.NewOpenAI(url, set.str("llm.model"), set.str("llm.api_key"))}
			},
			func() string { return set.str("llm.model") },
			set.str("playout.ffmpeg_path"), "")
	} else {
		// ⚠ `filler.language_model`, NOT `ingest.whisper_model`. The latter is
		// `ggml-small.en.bin` — an ENGLISH-ONLY build that does not identify languages at all,
		// so pointing this at it would answer "en" for every clip on earth and the gate would
		// silently never reject anything.
		langDetect = filler.NewWhisperLanguage(
			set.str("ingest.whisper_path"), set.str("filler.language_model"),
			set.str("playout.ffmpeg_path"), "")
	}
	// The ffmpeg tooling the metadata rungs share (a core runtime dep — NOT the ingest pair, so
	// they run on files already on disk regardless of whether yt-dlp is present).
	fillerTools := filler.NewFFmpegTools(
		set.str("playout.ffmpeg_path"), filler.FFprobePathNextTo(set.str("playout.ffmpeg_path")),
		set.str("ingest.whisper_path"), set.str("ingest.whisper_model"), "")

	// The drop-folder FS, for reading the info-JSON sidecars ingest writes beside each clip
	// (nil ⇒ every clip reads as thin-sourced and filename-only tagged, which only ever does
	// MORE work, never wrongly skips).
	var fillerDrop fs.FS
	if dir := set.str("filler.dir"); dir != "" {
		fillerDrop = os.DirFS(dir)
	}

	// Vision: keyframes → a multimodal model. The provider is nil unless a vision model is
	// configured AND the wired LLM actually implements llm.VisionProvider — the same type
	// assertion the model Warmer uses (§8.2). A nil provider makes the job a no-op, so an
	// install without a vision model never touches ffmpeg or spends a token.
	var visionProvider llm.VisionProvider
	if set.boolv("filler.vision.enabled") && set.str("llm.url") != "" {
		// ⚠ `filler.vision.model` when set, ELSE `llm.model` — vision needs an images-capable
		// model, which the tagging model often is not (a text model like qwen3). The dedicated
		// knob lets an operator point vision at a vision model without switching their whole LLM;
		// empty reuses the main model for installs whose model already sees images. Same
		// separation `filler.language_model` makes for the audio gate.
		visionModel := set.str("filler.vision.model")
		if visionModel == "" {
			visionModel = set.str("llm.model")
		}
		p := llm.NewProvider(set.str("llm.provider"), set.str("llm.url"), visionModel, set.str("llm.api_key"))
		if vp, ok := p.(llm.VisionProvider); ok {
			visionProvider = vp
		} else {
			log.Warn("filler vision enabled but the configured LLM provider has no vision path; job will no-op",
				"provider", set.str("llm.provider"))
		}
		log.Info("filler vision provider wired", "model", visionModel)
	}
	// ── The ingest pipeline (§10 V51b) ────────────────────────────────────────────────────
	//
	// One driver over eight rungs, replacing `filler-language`, `filler-split`,
	// `filler-transcribe` and `filler-vision`.
	//
	// ⚠ **Every rung is registered unconditionally, even when the thing it needs is absent.**
	// A stage missing from this slice is reported as "not available on this install" on every
	// clip's ladder, while one that is present says WHY it skipped in the operator's own terms
	// ("vision tagging is off", "no language backend is configured"). Registering all of them
	// and letting `Applies` answer is what makes the ladder explain an install rather than
	// merely show gaps in it — the same visible-but-idle contract the Tasks page rows use.
	clipDir := set.str("filler.dir")
	pipelineStages := []filler.Stage{
		filler.NewProbeStage(
			filler.FFprobeNextTo(set.str("playout.ffmpeg_path")), fillerPipelineClipAdapter{st}, clipDir,
			func() int64 { return set.dur("filler.min_duration").Milliseconds() },
			fillerTools, func() time.Duration { return set.dur("filler.autosplit.max_duration") }, time.Now),
		filler.NewTranscodeStage(
			fillerPipelineClipAdapter{st}, filler.FFprobeNextTo(set.str("playout.ffmpeg_path")),
			clipDir, filler.DefaultMezzanine(),
			func() string { return set.str("playout.ffmpeg_path") },
			// ⚠ The loudness target is applied only when the operator opted in. This is the
			// FIRST production caller of on-file loudness normalisation: V42 built the pass,
			// and `filler.autofile.normalize_loudness` gated a function nothing called, so the
			// setting has been inert since it shipped. Folding it into the encode that is
			// happening anyway is what finally wires it.
			func() float64 {
				if !set.boolv("filler.autofile.normalize_loudness") {
					return 0
				}
				// ⚠ `filler.target_lufs` is a STRING setting (an empty value means "no
				// normalisation at all", which no numeric kind can express), so it is parsed
				// here. An unparseable value yields 0 — the same as off, which is the safe
				// direction: the alternative is applying a garbage target to the operator's
				// audio.
				lufs, err := strconv.ParseFloat(set.str("filler.target_lufs"), 64)
				if err != nil {
					return 0
				}
				return lufs
			}, time.Now),
		filler.NewLanguageStage(langDetect, fillerLanguageStoreAdapter{st}, clipDir,
			func() string { return set.str("filler.language") }, time.Now),
		filler.NewTranscribeStage(fillerTools, fillerTranscribeStoreAdapter{st}, clipDir, fillerDrop,
			func() bool { return set.boolv("filler.transcribe.enabled") }, time.Now),
		filler.NewTagStage(taggerProvider, fillerTagStoreAdapter{st}, fillerDrop, time.Now),
		filler.NewVisionStage(fillerTools, visionProvider, fillerVisionStoreAdapter{st}, clipDir,
			func() bool { return set.boolv("filler.vision.enabled") }, time.Now),
		filler.NewScoreStage(fillerTagStoreAdapter{st}, &filler.AutoFilePolicy{
			// ⚠ `boolv`, the FAIL-CLOSED read, not `boolOn`. The two differ only when the
			// settings service cannot answer, and here that difference is the safety property:
			// failing OPEN would publish unreviewed clips to live channels exactly when the
			// install is degraded.
			Enabled:       func() bool { return set.boolv("filler.autofile.enabled") },
			MinConfidence: func() int { return set.intv("filler.autofile.min_confidence") },
		}, func() bool { return set.boolv("filler.reject.unidentified") }, time.Now),
	}
	if splitter != nil {
		// ⚠ Appended rather than placed in order — `NewPipeline` indexes the slice by stage id
		// and `StageOrder` is the ONE definition of the sequence, so the order here is
		// irrelevant. Stating that is worth a line, because a slice that looks like a pipeline
		// invites someone to "fix" its order.
		pipelineStages = append(pipelineStages,
			filler.NewSplitStage(splitter, fillerSplitStoreAdapter{st}).
				WithAutoConfirm(filler.AutoSplitPolicy{
					Enabled:       func() bool { return set.boolv("filler.autosplit.enabled") },
					MinConfidence: func() int { return set.intv("filler.autosplit.min_confidence") },
					MaxDuration:   func() time.Duration { return set.dur("filler.autosplit.max_duration") },
				}, func() time.Duration { return set.dur("filler.min_duration") }))
	}
	fillerPipeline := filler.NewPipeline(st, fillerPipelineClipAdapter{st}, pipelineStages,
		filler.Budget{
			MaxClips: func() int { return set.intv("filler.pipeline.max_clips") },
			// ⚠ A closure RETURNING ZERO means "never transcode on this box", which is a
			// different state from a nil closure meaning "use the default". That three-state
			// encoding is the only way an operator can turn the most expensive rung off.
			MaxTranscodes: func() int { return set.intv("filler.transcode.max_per_run") },
			MaxWhisper:    func() int { return set.intv("filler.pipeline.max_whisper") },
			MaxVision:     func() int { return set.intv("filler.pipeline.max_vision") },
			MaxSplits:     func() int { return set.intv("filler.pipeline.max_splits") },
		},
		emitter.FillerClipStage, time.Now, log).
		WithRewind(fillerRewindAdapter{st}, clipDir)
	log.Info("filler ingest pipeline registered",
		"stages", len(pipelineStages), "language", set.str("filler.language"),
		"transcribe", set.boolv("filler.transcribe.enabled"),
		"vision", set.boolv("filler.vision.enabled"), "vision_provider", visionProvider != nil,
		"autosplit", set.boolv("filler.autosplit.enabled"))
	return fillerPipeline
}

// buildPodAdapter constructs the pod assembler: the thing that picks which commercials fill a
// break, shared by the §12 preview endpoint, the reconciler and internal playout (§10).
//
// ⚠ Construction ONLY. The three lines that wire this into the channel engine, the preview
// adapter and the playout resolver stay at the call site deliberately — they are cross-subsystem
// back-patches (the resolver is built with the channel engine, the adapter needs the filler
// catalog), and pulling them in here would mean handing a builder the two services it exists to
// stay independent of.
func buildPodAdapter(st store.Store, set resolved, log *slog.Logger) *filler.PodAdapter {
	// ⚠ **A CLOSURE, not a struct literal — the whole filler policy is resolved per pod.**
	// This was a value built here at boot, so every reader took a snapshot: writing
	// `filler.pod_max`, `filler.min_quality`, `filler.min_clip_duration` or
	// `filler.max_clip_duration` changed the stored setting, the API read the new value
	// back, and the assembler went on using the old one until the process restarted.
	//
	// Caught on the live stack, not by a test: `filler.max_clip_duration=45s` left
	// `PoolReport.Eligible` at 20 with a 64s clip in the catalog; it dropped to 19 only
	// after a re-exec. Every unit test builds a `filler.Policy` literal and calls the domain
	// functions directly, so the setting→policy edge is the one segment they cannot reach.
	//
	// config-design §3 defines hot-apply for long-lived clients; `AutoSplitPolicy` already
	// resolves per call and says so. This is that contract, honoured by the pod path too.
	podAdapter := filler.NewPodAdapter(clipCatalogAdapter{st}, func() filler.Policy {
		return filler.Policy{
			PodMax: set.intv("filler.pod_max"),
			// V17c: 0 (the default) leaves selection exactly as it was before the floor
			// existed — see the warning on Policy.MinQualityHeight.
			MinQualityHeight: set.intv("filler.min_quality"),
			// V51f: the pod-eligibility duration bounds finally have keys behind them. Both
			// default to 0s = off, so `durationEligible` keeps returning true on an untouched
			// install — the difference is that it CAN now return false, which is what lets
			// `PoolReport.Eligible` differ from `Commercials` instead of restating it.
			MinClipMs: set.dur("filler.min_clip_duration").Milliseconds(),
			MaxClipMs: set.dur("filler.max_clip_duration").Milliseconds(),
		}
	}, log)
	return podAdapter
}
