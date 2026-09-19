package filler

import (
	"context"
	"time"
)

// The SCORE stage (§10 V51b): the last rung, where a clip's grounding is turned into a number and
// the number decides what happens to it.
//
// ⚠ It scores what the rungs ABOVE produced; it never asks a model anything. The confidence is the
// grounding-capped confidence reflects what could be verified in the clip's own signals, and
// nothing here may raise it.

// ScoreClipStore is the slice of the store the score stage writes through.
type ScoreClipStore interface {
	// SetClipConfidence is diagnostic only. It cannot change catalog eligibility.
	SetClipConfidence(ctx context.Context, path string, confidence int, at time.Time) error
}

// ScoreStage persists descriptive classification evidence. Missing classification is not a
// household approval task and therefore never changes the runtime disposition.
type ScoreStage struct {
	store ScoreClipStore
	now   func() time.Time
}

// NewScoreStage builds the stage.
func NewScoreStage(store ScoreClipStore, _ func() bool, now func() time.Time) *ScoreStage {
	if now == nil {
		now = time.Now
	}
	return &ScoreStage{store: store, now: now}
}

func (s *ScoreStage) ID() StageID     { return StageScore }
func (s *ScoreStage) Cost() StageCost { return CostCheap }

// Applies: always. Scoring is arithmetic over data already on the row — no exec, no model call —
// so there is never a reason to skip it.
func (s *ScoreStage) Applies(context.Context, StoreClip) (bool, string) { return true, "" }

// Run scores the clip and returns the verdict.
func (s *ScoreStage) Run(ctx context.Context, c StoreClip) (StageResult, error) {
	score := ScoreClip(c)
	if s.store != nil && c.Path != "" {
		if err := s.store.SetClipConfidence(ctx, c.Path, score, s.now().UTC()); err != nil {
			return StageResult{}, err
		}
	}
	reportProgress(ctx, StageScore, 100)

	return StageResult{Verdict: VerdictContinue}, nil
}

// ScoreClip computes a clip's grounding-capped confidence from what is on its row.
//
// SuggestedEra > 0 means a historical model proposed an era it could not ground, so that retained
// diagnostic remains capped below a fully grounded result. Confidence never controls readiness.
func ScoreClip(c StoreClip) int {
	ceiling := 100
	switch {
	case c.SuggestedEra > 0:
		ceiling = 40
	case c.Audience == "" || c.Category == "":
		ceiling = 50
	case c.Era == 0:
		ceiling = 60
	}
	if c.Confidence > 0 && c.Confidence < ceiling {
		return c.Confidence
	}
	return ceiling
}
