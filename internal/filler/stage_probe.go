package filler

import (
	"context"
	"fmt"
	"path/filepath"
	"time"
)

// The PROBE stage (§10 V51b): measure the file, and refuse the ones that are not clips.
//
// This is the quality gate V40 put at the scan boundary, made a first-class rung so its verdict is
// recorded with a reason rather than expressed as a file quietly never becoming a row.
//
// ⚠ **Both gates stay, deliberately.** `ScanDir` still refuses a 33ms truncated download at the
// boundary — that must never become a catalog row at all — and this stage is the second line, for
// clips catalogued before a floor was configured and for anything that arrives another way. They
// are not redundant: one prevents a row, the other explains one.

// ProbeClipStore is the slice of the store the probe stage writes through.
type ProbeClipStore interface {
	UpsertClip(ctx context.Context, c StoreClip) error
	// SetClipComposite marks a clip as a CONTAINER of adverts rather than one (§10 V45).
	SetClipComposite(ctx context.Context, hash string, composite bool, at time.Time) error
}

// ProbeStage measures a clip and applies the hard rejects.
type ProbeStage struct {
	probe   Prober
	store   ProbeClipStore
	clipDir string
	// minDurationMs is the floor below which a file is not a usable clip. A closure so a settings
	// change hot-applies, matching AutoFilePolicy's contract.
	minDurationMs func() int64
	// compositeOver is `filler.autosplit.max_duration`: longer than one advert, so worth asking
	// whether it is many. Probe owns only this cheap duration decision; the split stage owns the
	// full-file boundary scan.
	compositeOver func() time.Duration
	now           func() time.Time
}

// NewProbeStage builds the stage. A nil prober defaults to the real ffprobe.
func NewProbeStage(probe Prober, store ProbeClipStore, clipDir string, minDurationMs func() int64, compositeOver func() time.Duration, now func() time.Time) *ProbeStage {
	if probe == nil {
		probe = FFprobe
	}
	if now == nil {
		now = time.Now
	}
	return &ProbeStage{
		probe: probe, store: store, clipDir: clipDir, minDurationMs: minDurationMs,
		compositeOver: compositeOver, now: now,
	}
}

func (s *ProbeStage) ID() StageID     { return StageProbe }
func (s *ProbeStage) Cost() StageCost { return CostCheap }

// Applies: always. Every clip is measured, including one that already has a duration — the file
// may have been replaced under a stable path, and re-probing is one cheap exec.
func (s *ProbeStage) Applies(context.Context, StoreClip) (bool, string) { return true, "" }

// Run measures the clip and either updates it or refuses it.
func (s *ProbeStage) Run(ctx context.Context, c StoreClip) (StageResult, error) {
	if c.Path == "" {
		return StageResult{}, fmt.Errorf("clip %s has no location to probe", c.Hash)
	}
	file := filepath.Join(s.clipDir, c.Path)

	// ⚠ A probe FAILURE is an error, not a reject — it returns to the runner, which retries with
	// backoff and only rejects once the attempts are exhausted. ffprobe failing once on a file
	// still being copied is the common case, and tombstoning on the first attempt would refuse
	// clips that are merely mid-download.
	pr, err := s.probe(ctx, file)
	if err != nil {
		return StageResult{}, fmt.Errorf("probe %s: %w", c.Path, err)
	}
	reportProgress(ctx, StageProbe, 100)

	if v, reason, detail := s.verdict(pr); v == VerdictReject {
		return StageResult{Verdict: VerdictReject, Reason: reason, Detail: detail}, nil
	}
	// A catalog rebuild loses pipeline state but not sidecar evidence. Re-apply a prior content
	// quality decision here so a near-total-black clip cannot become airable merely because the
	// database was recreated; first-time clips have no report yet and continue to transcode.
	if tags, ok := ReadSidecarTags(file); ok && tags.MediaQuality != nil {
		if v, reason, detail := EvaluateMediaQuality(*tags.MediaQuality); v != VerdictContinue {
			out := StageResult{Verdict: v, Reason: reason, Detail: detail}
			if v == VerdictReview {
				out.Note, out.Detail = detail, ""
			}
			return out, nil
		}
	}

	// Persist what the probe learned. `UpsertClip` owns duration and quality (they are scan-derived
	// facts, in its DO UPDATE list by design), so this needs no new writer.
	updated := c
	updated.DurationMs = pr.DurationMs
	if q := QualityFromHeight(pr.Height); q != "" {
		updated.Quality = q
	}
	updated.UpdatedAt = s.now().UTC()
	if s.store != nil && (c.DurationMs != updated.DurationMs || c.Quality != updated.Quality) {
		if err := s.store.UpsertClip(ctx, updated); err != nil {
			return StageResult{}, err
		}
	}

	composite := s.looksComposite(updated)
	if composite && !updated.IsComposite {
		if s.store != nil {
			if err := s.store.SetClipComposite(ctx, updated.Hash, true, s.now().UTC()); err != nil {
				return StageResult{}, err
			}
		}
		updated.IsComposite = true
		return StageResult{Clip: updated, Verdict: VerdictContinue,
			Note: "this is a recording of several adverts, not one"}, nil
	}
	return StageResult{Clip: updated, Verdict: VerdictContinue}, nil
}

// looksComposite decides whether a clip is a CONTAINER of adverts (§10 V45).
//
// ⚠ Duration is the ONLY question probe owns. The previous version also ran BlackSilence here,
// fully decoding a long recording before the split stage fully decoded it again. Besides doubling
// work, an hour-scale capture could exhaust the pass while merely deciding whether the stage that
// owns boundary detection should run. Over the configured maximum is enough to prove the file is
// not a normally airable advert; split determines whether it contains many adverts or one long
// infomercial.
//
// ⚠ **Moving this mark to probe is a real behaviour change, and a deliberate one.** Until V51b
// `is_composite` was set only by split `Confirm`, so a 16-minute recording was AIRABLE until
// somebody split it — the exact bug §10 V45 describes. Setting it at measurement time makes the
// default-off `IncludeComposites` filter exclude it immediately, so the worst case is a long
// recording sitting out of rotation until its cuts are reviewed, rather than sixteen minutes of
// unrelated adverts airing as one break.
func (s *ProbeStage) looksComposite(c StoreClip) bool {
	if c.IsComposite {
		return true // already marked; nothing to re-measure
	}
	if s.compositeOver == nil {
		return false
	}
	over := s.compositeOver()
	return over > 0 && time.Duration(c.DurationMs)*time.Millisecond > over
}

// verdict applies the hard gates. Each carries the MEASURED fact, not just a code — "8.2s; floor
// is 10s" is arguable, "too short" is an assertion.
//
// ⚠ All three are hard rejects with no override, because none of them is a judgement call: a
// video-only file plays as dead air mid-break (which reads as the stream dropping), and a file
// with no video stream is not something a channel can show. Offering a human a button here would
// be offering a control that cannot work.
func (s *ProbeStage) verdict(pr Probed) (Verdict, RejectReason, string) {
	floor := int64(0)
	if s.minDurationMs != nil {
		floor = s.minDurationMs()
	}
	switch {
	case pr.DurationMs <= 0:
		return VerdictReject, ReasonUnprobeable, "ffprobe reported no usable duration"
	case floor > 0 && pr.DurationMs < floor:
		return VerdictReject, ReasonTooShort,
			fmt.Sprintf("%.1fs; the floor is %.1fs", float64(pr.DurationMs)/1000, float64(floor)/1000)
	case pr.Silent:
		return VerdictReject, ReasonNoAudio, "the file carries no audio stream at all"
	case pr.NoVideo:
		return VerdictReject, ReasonNoVideo, "the file carries no video stream"
	default:
		return VerdictContinue, "", ""
	}
}
