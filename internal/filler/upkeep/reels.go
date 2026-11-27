package upkeep

import (
	"context"

	"github.com/loomarr/loomarr/internal/filler/identity"
	"github.com/loomarr/loomarr/internal/filler/pipelinestate"
)

// releaseReels lets go of the file of every split reel Loomarr fetched once every piece cut from it
// is settled (#4421, revising V45). A reel kept whole cannot be evicted, since eviction looks only at
// clips on air: on one install 86 of them held 43% of the filler allowance and stopped acquisition.
//
// The row stays, reaped: provenance and every piece's parent_hash still resolve, and the sync's
// prune spares a reaped row. A re-split fetches the reel again from its recorded acquisition. A reel
// added by hand, one with a split proposal left, and one with a piece still running or waiting on a
// person are kept. The retire commits before any file moves, so a crash between the two leaves bytes
// for the next pass, never a reel the split rung could claim with its file gone.
func (j *Job) releaseReels(ctx context.Context, res *Result) error {
	hashes, err := j.store.ListSettledReels(ctx)
	if err != nil {
		return err
	}
	for _, hash := range hashes {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		retired, err := j.store.RetireSettledReel(ctx, hash, j.now().UTC())
		if err != nil {
			return err
		}
		if retired {
			res.Reels++
		}
	}
	return j.finishReels(ctx, res)
}

// finishReels sets aside the files of every reel already retired, this pass or one a crash cut short.
func (j *Job) finishReels(ctx context.Context, res *Result) error {
	rows, err := j.store.ListClipPipelines(ctx, pipelinestate.PipelineFilter{
		Dispositions: []pipelinestate.Disposition{pipelinestate.DispositionComplete},
	})
	if err != nil {
		return err
	}
	var referenced *identity.AssetRefs
	for _, row := range rows {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if row.FilesDeletedAt.IsZero() || j.disposed[row.ClipHash] {
			continue
		}
		if _, err := j.setAside(ctx, row.ClipHash, &referenced); err != nil {
			return err
		}
		if j.disposed[row.ClipHash] {
			res.Quarantined++
		}
	}
	return nil
}
