package filler

import (
	"context"
	"fmt"

	"github.com/loomarr/loomarr/internal/filler/pipelinestate"
	"github.com/loomarr/loomarr/internal/platform/store/clipstore"
)

// save applies an airability gate, writes the row, tombstones a refused clip, and publishes the
// result. A review is not terminal until its clip is held: otherwise a transient store failure
// could leave content the operator was explicitly asked to inspect eligible for a pod forever.
func (p *Pipeline) save(ctx context.Context, row pipelinestate.ClipPipeline, clip clipstore.Clip) error {
	row.UpdatedAt = p.now().UTC()
	applyPreparationProgress(&row)
	row.UnscheduleIfTerminal()
	if row.Disposition == pipelinestate.DispositionReview && clip.Path != "" {
		if _, err := p.clips.HoldClips(ctx, []string{clip.Path}, p.now().UTC()); err != nil {
			return fmt.Errorf("hold clip for review: %w", err)
		}
	}
	if err := p.store.UpsertClipPipeline(ctx, row); err != nil {
		return err
	}
	if row.Disposition == pipelinestate.DispositionRejected {
		p.tombstone(ctx, clip)
	}
	p.publish(row, clip)
	return nil
}

// tombstone takes a refused clip out of rotation.
//
// ⚠ **Two places, one truth, and the split is deliberate.** `clips.removed_at` is WHETHER a clip
// may air — pod assembly's `WHERE removed_at = 0` is the only reader that matters and it is
// unchanged by this phase — while the pipeline row is WHY. Folding the reason into `clips` would
// put it back in the synced cache that gets recreated, and folding airability into the pipeline
// row would mean every pod query joined a table it has no business knowing about.
//
// ⚠ Written HERE rather than in each rejecting stage, so a new rule cannot ship a reject that
// records a reason and leaves the clip playable. That failure would be invisible: the operator
// sees "rejected" in Incoming while the clip keeps airing.
//
// Best-effort by design: the pipeline row is already committed, so a failure here leaves a clip
// marked rejected and still airable until the next pass re-saves it — noisy but recoverable, and
// strictly better than failing the run and losing the reason too.
func (p *Pipeline) tombstone(ctx context.Context, clip clipstore.Clip) {
	if clip.Path == "" {
		return
	}
	if _, err := p.clips.SetClipsRemoved(ctx, []string{clip.Path}, p.now().UTC()); err != nil && p.log != nil {
		p.log.Warn("filler pipeline: could not tombstone a refused clip", "clip", clip.Path, "err", err)
	}
}

func (p *Pipeline) publish(row pipelinestate.ClipPipeline, clip clipstore.Clip) {
	if p.notify != nil {
		p.notify(row, clip)
	}
}
