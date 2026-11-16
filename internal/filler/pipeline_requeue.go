package filler

import (
	"context"
	"time"

	"github.com/loomarr/loomarr/internal/filler/pipelinestate"
)

// requeueResumableSplitReviews returns review rows to the split stage when it can now progress them
// without a person — incomplete detection, a missing structure decision or shadow observation, a
// duplicate or below-floor fragment, an unchecked segment. It is data-selected rather than
// version-selected: once curated/classified, those markers disappear or become checked, so a restart
// does not loop a genuinely ambiguous proposal back through the machine.
func (p *Pipeline) requeueResumableSplitReviews(ctx context.Context) (int, error) {
	return p.requeueSplitReviews(ctx, func(s *SplitStage) (map[string]struct{}, error) {
		return s.resumableReviewHashes(ctx)
	})
}

// dedupRecheckEvery is how often reviews held for an unchecked duplicate comparison are offered to
// the split stage again.
const dedupRecheckEvery = 30 * time.Minute

func (p *Pipeline) requeueSplitReviews(ctx context.Context, pick func(*SplitStage) (map[string]struct{}, error)) (int, error) {
	stage, ok := p.stages[pipelinestate.StageSplit].(*SplitStage)
	if !ok || stage == nil {
		return 0, nil
	}
	hashes, err := pick(stage)
	if err != nil || len(hashes) == 0 {
		return 0, err
	}
	rows, err := p.store.ListClipPipelines(ctx, pipelinestate.PipelineFilter{})
	if err != nil {
		return 0, err
	}
	n := 0
	for _, row := range rows {
		if _, ok := hashes[row.ClipHash]; !ok || row.Disposition != pipelinestate.DispositionReview || row.Stage != pipelinestate.StageSplit {
			continue
		}
		kept := row.Stages[:0:0]
		for _, rec := range row.Stages {
			if rec.Stage != pipelinestate.StageSplit {
				kept = append(kept, rec)
			}
		}
		row.Stages = kept
		row.Status, row.Attempts, row.Progress = pipelinestate.StatusQueued, 0, 0
		row.Disposition = pipelinestate.DispositionRunning
		row.RejectReason, row.RejectDetail = "", ""
		row.NextRun = time.Time{}
		row.UpdatedAt = p.now().UTC()
		if err := p.store.UpsertClipPipeline(ctx, row); err != nil {
			return n, err
		}
		n++
	}
	return n, nil
}
