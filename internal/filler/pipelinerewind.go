package filler

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"time"
)

// Rewind (§10 V51b) — the ONE sanctioned way to make a clip run part of the pipeline again.
//
// ⚠ **One implementation, deliberately, because the invalidation rules are the hard part.** The
// operator-facing routes (`/reprocess`, `/retag`, `/resplit`, and the existing `/split` and `/tag`)
// are thin wrappers over this. Each rung's `Applies` short-circuits on its OWN output — `transcribe`
// skips a clip that already has a transcript, `vision` skips one already stamped — so "run it
// again" means nothing unless the derived data is cleared with it. Three call sites deciding that
// separately is three chances to rewind a stage that then immediately skips itself, which looks
// exactly like the button doing nothing.

// RewindStore is the slice of the store `Rewind` invalidates through.
//
// ⚠ These are the EXISTING single writers of each column, reused rather than replaced. Rewind adds
// no new writer to any clip column — it calls the ones that already own them, with the "not yet"
// value each one already defines.
type RewindStore interface {
	// SetClipLanguage with "" — the only value meaning "not yet heard".
	SetClipLanguage(ctx context.Context, path, language string, at time.Time) error
	// SetClipTranscript with "" — `TranscriptNone` is the "heard, wordless" sentinel, so it is a
	// FINAL answer and must not be used to mean "not yet".
	SetClipTranscript(ctx context.Context, path, transcript string, at time.Time) error
	// ClearClipVisionTags removes the `vision_tagged` stamp and the text it read.
	//
	// ⚠ A separate narrow method rather than widening `SetClipVisionTags`, whose doc pins it as
	// the ONLY writer of visible_text/vision_tagged and which writes what it is GIVEN. Passing it
	// empty strings would work by accident today and break the first time it learns to gap-fill.
	ClearClipVisionTags(ctx context.Context, path string, at time.Time) error
	ListSplitProposals(ctx context.Context) ([]SplitProposal, error)
	DeleteSplitProposal(ctx context.Context, id string) error
}

// ErrTranscodeNeedsForce is returned when a rewind would re-encode a clip without `force`.
var ErrTranscodeNeedsForce = errors.New("re-encoding loses a generation of quality and the original is gone; pass force to do it anyway")

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
// ⚠ A rewind on a composite does NOT touch its confirmed segments. They are independent clips with
// their own ladders; a re-split proposes a new cut list beside them and the operator resolves the
// overlap. Deleting them would destroy tagged, possibly hand-corrected adverts to re-derive them
// from a detector that has no idea a human was involved.
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
	row.Disposition = DispositionRunning
	// ⚠ The reject is cleared as well. A rewound clip is no longer refused — leaving the reason
	// behind would show a clip that is visibly running AND visibly rejected, and the Incoming tab
	// filters on exactly this field.
	row.RejectReason, row.RejectDetail = "", ""
	row.NextRun = time.Time{}
	row.UpdatedAt = now
	if err := p.store.UpsertClipPipeline(ctx, row); err != nil {
		return err
	}
	p.publish(row, clip)
	return nil
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
