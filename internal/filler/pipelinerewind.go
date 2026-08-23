package filler

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"time"
)

// Rewind (§10 V51b) — the sanctioned way to make a clip re-run part of the pipeline, INCLUDING the
// invalidation that makes re-running mean anything.
//
// ⚠ **One implementation, deliberately, because the invalidation rules are the hard part.** Each
// rung's `Applies` short-circuits on its OWN output — `transcribe` skips a clip that already has a
// transcript, `vision` skips one already stamped — so "run it again" means nothing unless the
// derived data is cleared with it. Several call sites deciding that separately is several chances
// to rewind a stage that then immediately skips itself, which looks exactly like the button doing
// nothing.
//
// ⚠ **NOTHING CALLS THIS YET, and this comment used to claim otherwise.** It described
// `/reprocess`, `/retag` and `/resplit` as "operator-facing routes ... thin wrappers over this".
// None of the three exists — not in the router, not in `api/openapi.yaml`, not anywhere in the
// tree — and `Rewind` has no callers and no tests. The claim cost real time: with 17 compilations
// stuck at `split`/`review` and `ListPipelineWork` claiming `running` only, this paragraph named
// the exact escape hatch that would free them, and it was fiction. **Wiring the routes is real
// work, not a formality** (each needs its `from` stage, the `force` rule for transcode, and an
// authorization decision), so the honest state is recorded here rather than implied by a comment.
//
// ⚠ `Requeue` below is NOT that work. It is the one narrow transition that IS wired — un-park a
// reel after an operator re-detect — and it deliberately invalidates nothing, because the caller
// has just replaced the proposal itself.

// RewindStore is the slice of the store `Rewind` invalidates through.
//
// ⚠ These are the EXISTING single writers of each derived column, reused rather than replaced.
// Rewind adds no new writer to them — it calls the ones that already own them, with the "not yet"
// value each one already defines. Terminal retry's catalog restore is a separate atomic store
// transition because it must change the tombstone, hold, and pipeline row together.
type RewindStore interface {
	// SetClipLanguage with "" — the only value meaning "not yet heard".
	SetClipLanguage(ctx context.Context, path, language string, at time.Time) error
	// SetClipTranscript with "" — `TranscriptNone` is the "heard, wordless" sentinel, so it is a
	// FINAL answer and must not be used to mean "not yet".
	SetClipTranscript(ctx context.Context, path, transcript string, at time.Time) error
	// ClearClipVisionTags removes the `vision_tagged` stamp and the text it read.
	//
	// ⚠ A separate narrow method rather than widening `ApplyClipVision`, whose doc pins it as
	// the ONLY writer of visible_text/vision_tagged and which writes what it is GIVEN. Passing it
	// empty strings would work by accident today and break the first time it learns to gap-fill.
	ClearClipVisionTags(ctx context.Context, path string, at time.Time) error
	ListSplitProposals(ctx context.Context) ([]SplitProposal, error)
	DeleteSplitProposal(ctx context.Context, id string) error
}

// ErrTranscodeNeedsForce is returned when a rewind would re-encode a clip without `force`.
var ErrTranscodeNeedsForce = errors.New("re-encoding loses a generation of quality and the original is gone; pass force to do it anyway")

// ErrPipelineNotRetryable distinguishes an execution retry from an arbitrary rewind or content
// override. API callers map it to a conflict after reloading the authoritative row.
var ErrPipelineNotRetryable = errors.New("pipeline row does not have a retryable execution failure")

// WithRewind attaches the invalidation seam. Without it, `Rewind` still resets the ladder but
// cannot clear derived data — so it refuses rather than producing a rewind that silently
// self-skips.
func (p *Pipeline) WithRewind(store RewindStore, clipDir string) *Pipeline {
	p.rewind = store
	p.clipDir = clipDir
	return p
}

// Rewind resets a clip's ladder from `from` onward and invalidates what those rungs own.
//
// ⚠ **`tag` clears NOTHING, and "reset means clear" is the intuitive-and-wrong reading.** The
// tagger's rule is add-knowledge-never-replace (`unionLeaves`, and the scalars gap-fill), so a
// re-tag that first blanked the clip would silently destroy the operator's hand-edits and their
// confirmed eras — the very things a person went to the trouble of getting right. A re-tag simply
// runs the classifier again and merges.
//
// ⚠ A rewind on a composite does NOT touch its confirmed segments. They remain the active
// generation while the replacement proposal is incomplete. Final confirmation atomically
// tombstones superseded children, preserving their files and metadata for restore; exact reused
// hashes and channel-pinned clips survive. Touching them here would replace a complete reel with
// an unfinished one and destroy the recovery boundary.
func (p *Pipeline) Rewind(ctx context.Context, hash string, from StageID, force bool) error {
	idx := StageIndex(from)
	if idx < 0 {
		return fmt.Errorf("%w: %q", ErrUnknownStage, from)
	}
	if from == StageTranscode && !force {
		return ErrTranscodeNeedsForce
	}
	if p.rewind == nil {
		return errors.New("this install cannot re-run pipeline stages: no invalidation seam is wired")
	}

	clip, found, err := p.clips.GetClip(ctx, hash)
	if err != nil {
		return err
	}
	if !found {
		return fmt.Errorf("clip %s is not in the catalog", hash)
	}

	if err := p.invalidate(ctx, clip, idx); err != nil {
		return err
	}

	row, found, err := p.store.GetClipPipeline(ctx, hash)
	if err != nil {
		return err
	}
	now := p.now().UTC()
	if !found {
		row = ClipPipeline{ClipHash: hash, EnrolledAt: now}
	}
	row = resetPipelineRow(row, from, now)
	if err := p.store.UpsertClipPipeline(ctx, row); err != nil {
		return err
	}
	p.publish(row, clip)
	return nil
}

// RetryFailure retries exactly the failed rung the lifecycle projection permits. It is narrower
// than Rewind: callers cannot choose a rung, cannot override a measured content decision, and do
// not need to authorize transcode force because the server has proved that the transcode itself
// failed. Completed upstream work is retained.
func (p *Pipeline) RetryFailure(ctx context.Context, hash string) error {
	row, found, err := p.store.GetClipPipeline(ctx, hash)
	if err != nil {
		return err
	}
	if !found {
		return fmt.Errorf("%w: clip %s has no pipeline row", ErrPipelineNotRetryable, hash)
	}
	now := p.now().UTC()
	lifecycle := row.Lifecycle(now)
	if lifecycle.Recovery != RecoveryRetry || lifecycle.RetryStage == "" {
		return fmt.Errorf("%w: clip %s", ErrPipelineNotRetryable, hash)
	}
	if p.rewind == nil {
		return errors.New("this install cannot retry pipeline stages: no invalidation seam is wired")
	}
	clip, found, err := p.clips.GetClip(ctx, hash)
	if err != nil {
		return err
	}
	if !found {
		return fmt.Errorf("clip %s is not in the catalog", hash)
	}
	if err := p.invalidate(ctx, clip, StageIndex(lifecycle.RetryStage)); err != nil {
		return err
	}
	failed := row
	restore := failed.Disposition == DispositionRejected
	row = resetPipelineRow(row, lifecycle.RetryStage, now)
	if err := p.store.RetryClipPipeline(ctx, failed, row, restore); err != nil {
		return err
	}
	p.publish(row, clip)
	return nil
}

func resetPipelineRow(row ClipPipeline, from StageID, now time.Time) ClipPipeline {
	idx := StageIndex(from)
	// Keep the rungs BELOW the rewind point; drop the rest. The ladder is a picture of where the
	// clip is, so leaving a `done` vision rung above a queued transcribe one would show history
	// that is about to be overwritten.
	kept := row.Stages[:0:0]
	for _, rec := range row.Stages {
		if i := StageIndex(rec.Stage); i >= 0 && i < idx {
			kept = append(kept, rec)
		}
	}
	row.Stages = kept
	row.Stage, row.Status, row.Attempts, row.Progress = from, StatusQueued, 0, 0
	row.ForceRun = true
	row.Disposition = DispositionRunning
	// ⚠ The reject is cleared as well. A rewound clip is no longer refused — leaving the reason
	// behind would show a clip that is visibly running AND visibly rejected, and the Incoming tab
	// filters on exactly this field.
	row.RejectReason, row.RejectDetail = "", ""
	row.NextRun = time.Time{}
	row.UpdatedAt = now
	return row
}

// Requeue returns a reel parked at the split rung to the belt (§10 V54a). It reports whether it
// moved anything, so a caller can tell "put back" from "was never stuck".
//
// ⚠ **`Rewind`-lite, and deliberately not `Rewind`.** Rewind exists to clear derived data so a rung
// cannot skip itself. This runs immediately AFTER `Propose` has written a fresh cut list, so there
// is nothing to invalidate — and Rewind's own `RewindStore` would `DeleteSplitProposal`, destroying
// the very proposal the re-detect just produced. Same disposition flip, none of the invalidation.
//
// ⚠ **Scope is migration 00050's `WHERE` clause**, for the migration's reason: `split`/`review` is
// the unreachable state — `ListPipelineWork` claims `running` only, so nothing else visits it. A
// `rejected` row keeps its own restore path (`Soft()`), because a re-detect must not quietly
// overturn a refusal the operator can see and argue with; a `running` row has nothing to un-park.
//
// ⚠ The caller must not call this BEFORE detection finishes — see `Propose`'s call site. An
// un-parked row is claimable, and a rung claiming it mid-detection grounds the outgoing segment
// list or re-`Propose`s the same reel concurrently.
func (p *Pipeline) Requeue(ctx context.Context, hash string) (bool, error) {
	row, found, err := p.store.GetClipPipeline(ctx, hash)
	if err != nil {
		return false, err
	}
	if !found || row.Stage != StageSplit || row.Disposition != DispositionReview {
		return false, nil
	}
	// The four columns migration 00050 set. Attempts are given back for its reason too: the
	// retries these rows spent were spent losing to a gate that could not be won, and carrying
	// them would exhaust the budget on the way to succeeding.
	now := p.now().UTC()
	row.Disposition, row.Status, row.Attempts = DispositionRunning, StatusQueued, 0
	row.NextRun = time.Time{} // zero is "due now" — ListPipelineWork's `next_run <= ?`
	row.UpdatedAt = now
	if err := p.store.UpsertClipPipeline(ctx, row); err != nil {
		return false, err
	}
	// Best-effort: the clip is only needed to decorate the event. A missing row must not fail an
	// un-park that already succeeded.
	clip, _, _ := p.clips.GetClip(ctx, hash)
	p.publish(row, clip)
	if p.log != nil {
		// ⚠ Logged HERE, not at the call site: the operator-facing adapter has no logger by
		// design, and this is the line that distinguishes "the re-detect worked and the reel is
		// moving" from "the reel is stuck" — the one fact that was unobservable before V54a.
		p.log.Info("split: re-detected reel returned to the belt", "clip", hash)
	}
	return true, nil
}

// invalidate clears the derived data owned by every rung at or after `idx`.
func (p *Pipeline) invalidate(ctx context.Context, clip StoreClip, idx int) error {
	now := p.now().UTC()
	at := func(id StageID) bool { return StageIndex(id) >= idx }

	if at(StageTranscode) {
		// The loudness marker, so the re-encode normalises again rather than reading its own
		// previous work as "already at target". The `mezzanine` marker goes with it — it is the
		// gate that would otherwise make the rung skip itself immediately.
		full := filepath.Join(p.clipDir, filepath.FromSlash(clip.Path))
		if tags, ok := ReadSidecarTags(full); ok {
			tags.NormalizedLUFS, tags.Mezzanine = 0, ""
			if err := WriteSidecarTags(full, tags, false); err != nil {
				return fmt.Errorf("rewind %s: clearing the encode marker: %w", clip.Path, err)
			}
		}
	}
	if at(StageSplit) {
		// Else `Propose` replaces a proposal the operator may be mid-edit on.
		props, err := p.rewind.ListSplitProposals(ctx)
		if err != nil {
			return err
		}
		for _, pr := range props {
			if pr.ClipHash == clip.Hash {
				if err := p.rewind.DeleteSplitProposal(ctx, pr.ID); err != nil {
					return err
				}
			}
		}
	}
	if at(StageLanguage) {
		if err := p.rewind.SetClipLanguage(ctx, clip.Path, "", now); err != nil {
			return err
		}
	}
	if at(StageTranscribe) {
		if err := p.rewind.SetClipTranscript(ctx, clip.Path, "", now); err != nil {
			return err
		}
	}
	if at(StageVision) {
		if err := p.rewind.ClearClipVisionTags(ctx, clip.Path, now); err != nil {
			return err
		}
	}
	// StageTag and StageProbe clear nothing: probe has no derived state of its own beyond the
	// measurements it overwrites, and tag is add-only (see the doc comment above).
	return nil
}
