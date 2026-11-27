package filler

import (
	"context"
	"path/filepath"

	"github.com/loomarr/loomarr/internal/filler/asset"
	"github.com/loomarr/loomarr/internal/filler/pipelinestate"
)

// retireRemovedRows retires running rows whose clip was removed from the catalog. advance does the
// same for a due row, but a row whose terminal commit was refused rests a month before it is due
// again, so without this sweep it would sit parked until then and be refused once more.
func (p *Pipeline) retireRemovedRows(ctx context.Context) (int, error) {
	rows, err := p.store.ListClipPipelines(ctx, pipelinestate.PipelineFilter{
		Dispositions: []pipelinestate.Disposition{pipelinestate.DispositionRunning},
	})
	if err != nil {
		return 0, err
	}
	n := 0
	for _, row := range rows {
		clip, ok, err := p.clips.GetClip(ctx, row.ClipHash)
		if err != nil {
			return n, err
		}
		if !ok || clip.RemovedAt.IsZero() {
			continue
		}
		if _, err := p.retireRemoved(ctx, row, clip); err != nil {
			return n, err
		}
		n++
	}
	return n, nil
}

// rejudgeDoubtfulReviews applies "doubt means drop" (#4356) to clips judged before it. A fetched,
// single clip held in review because a black, silent or frozen span might be intentional is now
// rejected as `undecided` the moment a check reaches that verdict; one held under the old rule
// would otherwise wait for a person forever (2026-11-18: 13 such clips, held since late October).
//
// The verdict is re-read from the quality report the transcode rung stored in the clip's sidecar,
// through the same EvaluateMediaQuality the rung uses, so nothing is re-encoded and the rule cannot
// drift from the live one. What #4382 kept in review stays there: a clip an operator added, a reel
// waiting to be split, and any review the sidecar's report does not explain.
func (p *Pipeline) rejudgeDoubtfulReviews(ctx context.Context) (int, error) {
	if p.clipDir == "" {
		return 0, nil
	}
	rows, err := p.store.ListClipPipelines(ctx, pipelinestate.PipelineFilter{
		Dispositions: []pipelinestate.Disposition{pipelinestate.DispositionReview},
	})
	if err != nil {
		return 0, err
	}
	n := 0
	for _, row := range rows {
		if row.AcquisitionID == "" || row.Stage == pipelinestate.StageSplit {
			continue
		}
		clip, ok, err := p.clips.GetClip(ctx, row.ClipHash)
		if err != nil {
			return n, err
		}
		if !ok || clip.IsComposite || clip.Path == "" {
			continue
		}
		tags, ok := asset.ReadSidecarTags(filepath.Join(p.clipDir, filepath.FromSlash(clip.Path)))
		if !ok || tags.MediaQuality == nil {
			continue
		}
		verdict, _, detail := EvaluateMediaQuality(*tags.MediaQuality)
		if verdict != pipelinestate.VerdictReview {
			continue
		}
		row.Disposition = pipelinestate.DispositionRejected
		row.RejectReason, row.RejectDetail = pipelinestate.ReasonUndecided, detail
		row.ForceRun = false
		if err := p.persist(ctx, row, clip); err != nil {
			return n, err
		}
		n++
	}
	return n, nil
}
