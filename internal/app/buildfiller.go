package app

import (
	"io/fs"
	"log/slog"
	"os"
	"time"

	"github.com/mantonx/loomarr/internal/clipfetch"
	"github.com/mantonx/loomarr/internal/filler"
	"github.com/mantonx/loomarr/internal/llm"
	"github.com/mantonx/loomarr/internal/store"
)

// This file is the first of BuildHandler's per-subsystem builders (§14.1).
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
