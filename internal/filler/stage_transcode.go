package filler

import (
	"context"
	"fmt"
	"github.com/mantonx/loomarr/internal/mediatools"
	"os"
	"path/filepath"
	"time"
)

// The TRANSCODE stage (§10 V51b): every clip is re-encoded once, to one mezzanine profile.
//
// See transcode.go for what the profile is and — more importantly — what it is NOT. This file is
// the rung: when it applies, what it writes, and how the clip's row follows the file.

// TranscodeClipStore is the slice of the store the transcode stage writes through.
type TranscodeClipStore interface {
	// UpsertClip records the new location and the re-measured facts. ⚠ `path` IS in UpsertClip's
	// DO UPDATE list, and this is the case that blesses it: the extension changes when a clip
	// arrives as something other than mp4.
	UpsertClip(ctx context.Context, c StoreClip) error
}

// TranscodeStage re-encodes a clip to the mezzanine profile.
type TranscodeStage struct {
	store   TranscodeClipStore
	probe   Prober
	clipDir string
	profile MezzanineProfile
	// ffmpegPath is the operator's configured binary; empty falls back to PATH.
	ffmpegPath func() string
	// targetLUFS folds loudness normalisation into the same pass. A closure returning 0 leaves the
	// audio alone, which is what `filler.autofile.normalize_loudness` off means.
	targetLUFS func() float64
	now        func() time.Time
}

// NewTranscodeStage builds the stage.
func NewTranscodeStage(store TranscodeClipStore, probe Prober, clipDir string, profile MezzanineProfile, ffmpegPath func() string, targetLUFS func() float64, now func() time.Time) *TranscodeStage {
	if probe == nil {
		probe = FFprobe
	}
	if now == nil {
		now = time.Now
	}
	if profile.VideoCodec == "" {
		profile = mediatools.DefaultMezzanine()
	}
	return &TranscodeStage{
		store: store, probe: probe, clipDir: clipDir, profile: profile,
		ffmpegPath: ffmpegPath, targetLUFS: targetLUFS, now: now,
	}
}

func (s *TranscodeStage) ID() StageID     { return StageTranscode }
func (s *TranscodeStage) Cost() StageCost { return CostTranscode }

// Applies unless this clip already carries this profile's mark.
//
// ⚠ **The sidecar marker is a SECOND line of defence, not the primary one.** The pipeline ladder
// is what normally stops a re-encode: the rung is `done` and is never revisited. The marker exists
// for the case the sibling table was built to survive — a `clips` rebuild that also loses the
// pipeline row — because a second generation of loss on a file whose original is gone is not
// recoverable, and re-reading a small JSON file is a cheap way never to risk it.
//
// It records the profile ID rather than a bare flag, so a future profile change re-encodes from
// the operator's own file rather than being silently skipped as "already done".
func (s *TranscodeStage) Applies(_ context.Context, c StoreClip) (bool, string) {
	if c.Path == "" {
		return false, "the clip has no file"
	}
	full := filepath.Join(s.clipDir, filepath.FromSlash(c.Path))
	if tags, ok := ReadSidecarTags(full); ok && tags.Mezzanine == s.profile.ID() {
		return false, "already encoded to the ingest profile"
	}
	return true, ""
}

// Run re-encodes the clip, moves its sidecar if the extension changed, and updates its row.
func (s *TranscodeStage) Run(ctx context.Context, c StoreClip) (StageResult, error) {
	oldRel := c.Path
	newRel := mediatools.MezzanineOutputPath(oldRel)
	oldFull := filepath.Join(s.clipDir, filepath.FromSlash(oldRel))
	newFull := filepath.Join(s.clipDir, filepath.FromSlash(newRel))

	// Measure the input so the output can be checked against it. The probe rung has already run,
	// but `HadAudio` is not on the clip row and re-probing one file is cheap next to an encode.
	in, err := s.probe(ctx, oldFull)
	if err != nil {
		return StageResult{}, fmt.Errorf("re-probe %s before transcode: %w", oldRel, err)
	}

	lufs := 0.0
	if s.targetLUFS != nil {
		lufs = s.targetLUFS()
	}
	ffmpeg := ""
	if s.ffmpegPath != nil {
		ffmpeg = s.ffmpegPath()
	}

	// ⚠ Same extension is a temp-then-rename over the SAME path, which `Transcode` already does —
	// it writes `<out>.mezz.tmp<ext>` and renames. ffmpeg is never pointed at its own input.
	req := mediatools.TranscodeRequest{
		In: oldFull, Out: newFull,
		DurationMs: in.DurationMs, HadAudio: !in.Silent,
		TargetLUFS: lufs, Profile: s.profile,
		FFmpegPath: ffmpeg, Probe: s.probe,
	}
	if err := mediatools.Transcode(ctx, req, func(pct int) { reportProgress(ctx, StageTranscode, pct) }); err != nil {
		return StageResult{}, err
	}

	// ⚠ Move the sidecar BEFORE deleting the old media file and before the row moves. It carries
	// `originalName`, the only surviving copy of the clip's arrival filename and therefore the
	// only thing keeping filename-grounded eras alive.
	if err := mediatools.MoveSidecar(oldFull, newFull); err != nil {
		return StageResult{}, fmt.Errorf("transcode %s: the sidecar could not be moved: %w", oldRel, err)
	}

	// Re-measure the installed file: the encode may legitimately shift the duration by a frame,
	// and the row must describe what is on disk.
	out, err := s.probe(ctx, newFull)
	if err != nil {
		return StageResult{}, fmt.Errorf("re-probe %s after transcode: %w", newRel, err)
	}

	updated := c
	updated.Path = newRel
	updated.DurationMs = out.DurationMs
	if q := QualityFromHeight(out.Height); q != "" {
		updated.Quality = q
	}
	updated.UpdatedAt = s.now().UTC()
	if s.store != nil {
		if err := s.store.UpsertClip(ctx, updated); err != nil {
			return StageResult{}, err
		}
	}

	// The old file goes only AFTER the row points at the new one. The other order leaves a window
	// where the catalog names a file that no longer exists — and if the process dies in it, the
	// clip is permanently broken rather than merely duplicated.
	if newFull != oldFull {
		if err := os.Remove(oldFull); err != nil && !os.IsNotExist(err) {
			// Not fatal: the clip is installed, correct and catalogued. A stray original wastes
			// disk, which the next sync reports, and failing the rung here would re-encode a file
			// that is already done.
			return StageResult{Clip: updated, Verdict: VerdictContinue,
				Note: "re-encoded, but the original file could not be deleted"}, nil
		}
	}

	// ⚠ The marker is written AFTER the rename, never before. Written first, a failed encode would
	// leave a file marked as encoded that never was — and nothing would ever revisit it. This is
	// the ordering V42 chose for `normalizedLufs`, for the same reason.
	tags, _ := ReadSidecarTags(newFull)
	tags.Mezzanine = s.profile.ID()
	if lufs != 0 {
		// The loudness filter ran in this pass, so the loudness marker is true as well — recording
		// it here is what stops a later pass re-normalising a file that is already at target and
		// walking its loudness down run after run.
		tags.NormalizedLUFS = lufs
	}
	if err := WriteSidecarTags(newFull, tags, false); err != nil {
		// The file IS encoded; only the marker failed. Report it rather than claiming a success we
		// cannot prove — the runner retries, `Applies` sees no marker, and the clip is re-encoded
		// once more. That is a real cost, which is why it is an error and not a silent shrug.
		return StageResult{Clip: updated}, fmt.Errorf("transcode %s: encoded, but the marker could not be written: %w", newRel, err)
	}

	return StageResult{Clip: updated, Verdict: VerdictContinue}, nil
}
