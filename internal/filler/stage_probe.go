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
	// tools runs the boundary scan that decides whether a long file is a compilation. nil ⇒
	// composite detection is off and a long recording stays airable until someone splits it by
	// hand — the pre-V51b behaviour.
	tools MediaTools
	// compositeOver is `filler.autosplit.max_duration`: longer than one advert, so worth asking
	// whether it is many.
	compositeOver func() time.Duration
	now           func() time.Time
}

// NewProbeStage builds the stage. A nil prober defaults to the real ffprobe.
func NewProbeStage(probe Prober, store ProbeClipStore, clipDir string, minDurationMs func() int64, tools MediaTools, compositeOver func() time.Duration, now func() time.Time) *ProbeStage {
	if probe == nil {
		probe = FFprobe
	}
	if now == nil {
		now = time.Now
	}
	return &ProbeStage{
		probe: probe, store: store, clipDir: clipDir, minDurationMs: minDurationMs,
		tools: tools, compositeOver: compositeOver, now: now,
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

	composite, err := s.looksComposite(ctx, file, updated)
	if err != nil {
		return StageResult{}, err
	}
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

// compositeBoundaries is how many black/silence gaps make a long file a compilation rather than
// one long advert.
//
// Two: a single gap is what a fade-out at the end of one clip looks like, while two interior
// boundaries mean at least three spans — a shape a single advert does not have.
const compositeBoundaries = 2

// looksComposite decides whether a clip is a CONTAINER of adverts (§10 V45).
//
// ⚠ **The expensive half is gated on duration, and without that gate this rung becomes the most
// expensive one in the pipeline.** `BlackSilence` is a full decode — minutes on a 16-minute file —
// so it runs only for clips longer than `filler.autosplit.max_duration`. Under that a clip is
// advert-shaped by definition and there is nothing to ask.
//
// ⚠ **Moving this mark to probe is a real behaviour change, and a deliberate one.** Until V51b
// `is_composite` was set only by split `Confirm`, so a 16-minute recording was AIRABLE until
// somebody split it — the exact bug §10 V45 describes. Setting it at measurement time makes the
// default-off `IncludeComposites` filter exclude it immediately, so the worst case is a long
// recording sitting out of rotation until its cuts are reviewed, rather than sixteen minutes of
// unrelated adverts airing as one break.
func (s *ProbeStage) looksComposite(ctx context.Context, file string, c StoreClip) (bool, error) {
	if c.IsComposite {
		return true, nil // already marked; nothing to re-measure
	}
	if s.tools == nil || s.compositeOver == nil {
		return false, nil
	}
	over := s.compositeOver()
	if over <= 0 || time.Duration(c.DurationMs)*time.Millisecond <= over {
		return false, nil
	}
	// ⚠ A boundary-scan failure is NOT an error the rung fails on. The clip has been measured and
	// is perfectly usable; all we lose is the composite mark, and the split rung's own detection
	// would fail the same way. Failing here would retry an expensive decode three times and then
	// REJECT a clip for being long, which is not a fault.
	blacks, silences, err := s.tools.BlackSilence(ctx, file)
	if err != nil {
		return false, nil
	}
	return len(blacks)+len(silences) >= compositeBoundaries, nil
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
	case pr.Height <= 0:
		return VerdictReject, ReasonNoVideo, "the file carries no video stream"
	default:
		return VerdictContinue, "", ""
	}
}
