package filler

import (
	"context"

	"github.com/loomarr/loomarr/internal/filler/pipelinestate"
)

// The FINISH stage: the last rung of the pipeline.
//
// ⚠ It does no work, and that is its job. Every other rung can be skipped (vision when no model is
// configured, split when a clip is single), so the ready commit needs one rung that always runs to
// anchor on: the runner persists this rung as running, and TerminalReady publishes only a ladder
// whose last record is this rung, done. It replaced the score rung, whose confidence number never
// controlled readiness and was retired with the column (#2323).
type FinishStage struct{}

// NewFinishStage builds the stage.
func NewFinishStage() *FinishStage { return &FinishStage{} }

func (s *FinishStage) ID() pipelinestate.StageID { return pipelinestate.StageFinish }
func (s *FinishStage) Cost() StageCost           { return CostCheap }

// Applies: always. The ready commit depends on this rung having run.
func (s *FinishStage) Applies(context.Context, StoreClip) (bool, string) { return true, "" }

// Run completes the rung and hands the clip to terminal readiness.
func (s *FinishStage) Run(ctx context.Context, _ StoreClip) (StageResult, error) {
	reportProgress(ctx, pipelinestate.StageFinish, 100)
	return StageResult{Verdict: pipelinestate.VerdictContinue}, nil
}
