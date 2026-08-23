package filler

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/loomarr/loomarr/internal/mediatools"
)

// The TRANSCODE stage (§10 V51b): every clip is re-encoded once, to one mezzanine profile.
//
// See transcode.go for what the profile is and — more importantly — what it is NOT. This file is
// the rung: when it applies, what it writes, and how the clip's row follows the file.

// TranscodeClipStore is the slice of the store the transcode stage writes through.
type TranscodeClipStore interface {
	// ReplaceClipIdentity atomically re-keys the clip and everything that refers to it. A
	// transcode changes bytes, so keeping the intake hash would make the next scan discover a
	// second, metadata-empty clip under the transformed bytes' real identity.
	ReplaceClipIdentity(ctx context.Context, oldHash string, c StoreClip) error
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
	// transcode is a seam around the ffmpeg driver. Production always uses mediatools.Transcode;
	// tests replace it with a byte writer so the lifecycle can be exercised without a host binary.
	transcode func(context.Context, mediatools.TranscodeRequest, func(int)) (MediaQuality, error)
	// inspect backfills quality facts for a mezzanine made before those facts rode the encode.
	inspect func(context.Context, string, string, int64, bool) (MediaQuality, error)
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
		transcode: mediatools.Transcode,
		inspect:   mediatools.InspectQuality,
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
	if tags, ok := ReadSidecarTags(full); ok && tags.Mezzanine == s.profile.ID() && tags.MediaQuality != nil {
		// A clean report is finished work. An anomalous report must remain applicable so a
		// failed hold/tombstone write can cheaply re-emit the same safety verdict next pass.
		if verdict, _, _ := mediaQualityVerdict(*tags.MediaQuality); verdict == VerdictContinue {
			return false, "already encoded to the ingest profile"
		}
	}
	return true, ""
}

const transcodeStagingDir = ".transcode"

// Run re-encodes the clip, hashes the transformed bytes, files them at that identity's canonical
// path, and atomically re-keys the catalog metadata. The original remains intact until the new
// file, sidecar and database references are all durable.
func (s *TranscodeStage) Run(ctx context.Context, c StoreClip) (StageResult, error) {
	if s.store == nil {
		return StageResult{}, fmt.Errorf("transcode %s: no clip store is configured", c.Path)
	}
	oldRel := c.Path
	oldFull := filepath.Join(s.clipDir, filepath.FromSlash(oldRel))

	// The inspection report is also the retry record for the airability gate. Re-emit its
	// decision without decoding or encoding again if the previous pass could not hold the clip.
	if tags, ok := ReadSidecarTags(oldFull); ok && tags.Mezzanine == s.profile.ID() && tags.MediaQuality != nil {
		return mediaQualityResult(c, *tags.MediaQuality), nil
	}

	// Older mezzanines already carry the profile marker but predate content inspection. Re-encoding
	// would add a needless generation of loss, so spend this rung's bounded transcode budget on one
	// detector-only decode, persist the evidence beside the bytes, and decide from that.
	if tags, ok := ReadSidecarTags(oldFull); ok && tags.Mezzanine == s.profile.ID() && tags.MediaQuality == nil {
		in, err := s.probe(ctx, oldFull)
		if err != nil {
			return StageResult{}, fmt.Errorf("re-probe %s before quality backfill: %w", oldRel, err)
		}
		ffmpeg := ""
		if s.ffmpegPath != nil {
			ffmpeg = s.ffmpegPath()
		}
		quality, err := s.inspect(ctx, ffmpeg, oldFull, in.DurationMs, !in.Silent)
		if err != nil {
			return StageResult{}, err
		}
		tags.MediaQuality = &quality
		if err := WriteSidecarTags(oldFull, tags, false); err != nil {
			return StageResult{}, fmt.Errorf("persist quality inspection %s: %w", oldRel, err)
		}
		return mediaQualityResult(c, quality), nil
	}

	stageDir := filepath.Join(s.clipDir, transcodeStagingDir)
	if err := os.MkdirAll(stageDir, 0o755); err != nil {
		return StageResult{}, fmt.Errorf("create transcode staging folder: %w", err)
	}
	stageFull := filepath.Join(stageDir, c.Hash+".mp4")
	_ = os.Remove(stageFull)
	_ = os.Remove(sidecarPathFor(stageFull))
	defer func() {
		_ = os.Remove(stageFull)
		_ = os.Remove(sidecarPathFor(stageFull))
	}()

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

	req := mediatools.TranscodeRequest{
		In: oldFull, Out: stageFull,
		DurationMs: in.DurationMs, HadAudio: !in.Silent,
		TargetLUFS: lufs, Profile: s.profile,
		FFmpegPath: ffmpeg, Probe: s.probe,
	}
	quality, err := s.transcode(ctx, req, func(pct int) { reportProgress(ctx, StageTranscode, pct) })
	if err != nil {
		return StageResult{}, err
	}

	// Re-measure the staged file: the encode may legitimately shift the duration by a frame,
	// and the row must describe what is on disk.
	out, err := s.probe(ctx, stageFull)
	if err != nil {
		return StageResult{}, fmt.Errorf("re-probe %s after transcode: %w", oldRel, err)
	}
	newHash, err := ClipID(stageFull)
	if err != nil {
		return StageResult{}, fmt.Errorf("hash transformed clip %s: %w", oldRel, err)
	}
	newRel := filepath.ToSlash(ClipRelPath(newHash, ".mp4"))
	newFull := filepath.Join(s.clipDir, filepath.FromSlash(newRel))
	alreadyPublished := false
	if newFull != oldFull {
		if _, err := os.Stat(newFull); err == nil {
			// A previous attempt may have published the verified pair and then lost the database
			// commit. The marker distinguishes that recoverable saga state from an unrelated sparse-
			// hash collision; retry the durable re-key without overwriting either file.
			tags, ok := ReadSidecarTags(newFull)
			if !ok || tags.Mezzanine != s.profile.ID() {
				return StageResult{}, fmt.Errorf("transcode %s: transformed identity %s already exists", oldRel, newHash)
			}
			alreadyPublished = true
		} else if !os.IsNotExist(err) {
			return StageResult{}, fmt.Errorf("transcode %s: inspect transformed identity: %w", oldRel, err)
		}
	}

	// Build the replacement sidecar while the media is still hidden from the catalog scan. Split
	// children historically had no sidecar; seeding OriginalName from the durable display name is
	// what prevents their next scan from presenting the old hash as the title.
	if err := copySidecarForTransform(oldFull, stageFull); err != nil {
		return StageResult{}, fmt.Errorf("transcode %s: copy sidecar: %w", oldRel, err)
	}
	tags, _ := ReadSidecarTags(stageFull)
	if tags.OriginalName == "" {
		tags.OriginalName = c.Name
	}
	tags.Kind = string(c.Kind)
	tags.Era = c.Era
	tags.Audience = string(c.Audience)
	tags.Category = c.Category
	tags.Brand = c.Brand
	tags.Transcript = c.Transcript
	tags.Confidence = c.Confidence
	tags.SuggestedEra = c.SuggestedEra
	tags.Mezzanine = s.profile.ID()
	quality.DurationMs = out.DurationMs
	tags.MediaQuality = &quality
	if lufs != 0 {
		tags.NormalizedLUFS = lufs
	}
	if err := WriteSidecarTags(stageFull, tags, false); err != nil {
		return StageResult{}, fmt.Errorf("transcode %s: write replacement sidecar: %w", oldRel, err)
	}

	if err := os.MkdirAll(filepath.Dir(newFull), 0o755); err != nil {
		return StageResult{}, fmt.Errorf("transcode %s: create content shard: %w", oldRel, err)
	}
	// Sidecar first: the scan ignores it without media. Hard links publish without replacement,
	// so a race or sparse-hash collision can never overwrite an existing content-addressed clip.
	if !alreadyPublished {
		if err := publishTranscodePair(stageFull, newFull); err != nil {
			return StageResult{}, fmt.Errorf("transcode %s: publish transformed media: %w", oldRel, err)
		}
	}

	updated := c
	updated.Hash = newHash
	updated.Path = newRel
	updated.TunarrProgramID = "" // the registered program named the old path; the next scan refreshes it
	updated.DurationMs = out.DurationMs
	if q := QualityFromHeight(out.Height); q != "" {
		updated.Quality = q
	}
	updated.UpdatedAt = s.now().UTC()
	if err := s.store.ReplaceClipIdentity(ctx, c.Hash, updated); err != nil {
		return StageResult{}, err
	}

	// The old file goes only AFTER every database reference points at the new identity. If cleanup
	// fails, the correct clip remains playable and the stale bytes merely waste disk.
	if newFull != oldFull {
		if err := os.Remove(oldFull); err != nil && !os.IsNotExist(err) {
			return StageResult{Clip: updated, Verdict: VerdictContinue,
				Note: "re-encoded, but the original file could not be deleted"}, nil
		}
		_ = os.Remove(sidecarPathFor(oldFull))
	}

	return mediaQualityResult(updated, quality), nil
}

func mediaQualityResult(c StoreClip, quality MediaQuality) StageResult {
	verdict, reason, detail := mediaQualityVerdict(quality)
	result := StageResult{Clip: c, Verdict: verdict}
	switch verdict {
	case VerdictReject:
		result.Reason, result.Detail = reason, detail
	case VerdictReview:
		result.Note = detail
	}
	return result
}

func copySidecarForTransform(oldMedia, stagedMedia string) error {
	from := sidecarPathFor(oldMedia)
	if _, err := os.Stat(from); errors.Is(err, os.ErrNotExist) {
		return nil
	} else if err != nil {
		return err
	}
	return copyFile(from, sidecarPathFor(stagedMedia))
}

func publishTranscodePair(stagedMedia, finalMedia string) error {
	if stagedMedia == finalMedia {
		return nil
	}
	stagedSidecar, finalSidecar := sidecarPathFor(stagedMedia), sidecarPathFor(finalMedia)
	if err := os.Link(stagedSidecar, finalSidecar); err != nil {
		return err
	}
	if err := os.Link(stagedMedia, finalMedia); err != nil {
		_ = os.Remove(finalSidecar)
		return err
	}
	_ = os.Remove(stagedSidecar)
	_ = os.Remove(stagedMedia)
	return nil
}
