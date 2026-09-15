package filler

import (
	"context"
	"time"
)

// The SCORE stage (§10 V51b): the last rung, where a clip's grounding is turned into a number and
// the number decides what happens to it.
//
// ⚠ It scores what the rungs ABOVE produced; it never asks a model anything. The confidence is the
// same grounding-capped ceiling `TagSuggestion.Score` computes — what could be VERIFIED in the
// clip's own text sets the maximum, and nothing here may raise it.

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
// ⚠ It reuses `TagSuggestion.Score` rather than re-deriving the ceilings, so the pipeline and the
// tagger cannot disagree about what a clip is worth. In particular `SuggestedEra > 0` — an era the
// model proposed but could not ground — caps the score below a fully-grounded result.
func ScoreClip(c StoreClip) int {
	sug := TagSuggestion{
		Era:          c.Era,
		Audience:     c.Audience,
		Category:     c.Category,
		Brand:        c.Brand,
		SuggestedEra: c.SuggestedEra,
	}
	// ⚠ **The persisted confidence is passed as the model layer, and that is what preserves the
	// LOWERING half of `Score`.** The model's own self-report exists only inside the tag rung, which
	// writes the layered result to the row; by the time this runs, that number is all that is left
	// of it. Passing 0 here instead would let a clip the model was unsure about — but whose tags all
	// happen to verify — score the full grounded 100 and become Ready unattended, silently undoing the
	// asymmetry `Score` documents.
	//
	// Raising is still impossible: `Score` only ever takes the model layer when it is BELOW the
	// grounded ceiling, so a stale high value on the row cannot lift a clip whose grounding got
	// worse. A stale LOW one survives, which errs toward asking a human — the safe direction.
	return sug.Score(c.Confidence)
}
