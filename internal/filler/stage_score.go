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
	// SetClipConfidence is the score's single writer (§10 V51a).
	SetClipConfidence(ctx context.Context, path string, confidence int, at time.Time) error
	// SetClipsHeld files a clip into the catalog. The stage only ever FILES (held=false,
	// autoFiled=true) — sending a clip back for review is a human's decision, made from Incoming.
	SetClipsHeld(ctx context.Context, paths []string, held, autoFiled bool, at time.Time) (int, error)
}

// ScoreStage decides a clip's fate.
type ScoreStage struct {
	store    ScoreClipStore
	autoFile *AutoFilePolicy
	// rejectUnidentified promotes "nothing grounded anywhere" from review to a hard reject.
	rejectUnidentified func() bool
	now                func() time.Time
}

// NewScoreStage builds the stage.
func NewScoreStage(store ScoreClipStore, autoFile *AutoFilePolicy, rejectUnidentified func() bool, now func() time.Time) *ScoreStage {
	if now == nil {
		now = time.Now
	}
	return &ScoreStage{store: store, autoFile: autoFile, rejectUnidentified: rejectUnidentified, now: now}
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

	if s.unidentified(c) {
		if s.rejectUnidentified != nil && s.rejectUnidentified() {
			return StageResult{
				Verdict: VerdictReject, Reason: ReasonUnidentified,
				Detail: "every signal was read and none of them said what this is",
			}, nil
		}
		return StageResult{Verdict: VerdictReview, Note: "nothing could be grounded — this needs a person"}, nil
	}

	if c.Held && s.autoFile.Allows(score) {
		if s.store != nil {
			if _, err := s.store.SetClipsHeld(ctx, []string{c.Path}, false, true, s.now().UTC()); err != nil {
				return StageResult{}, err
			}
		}
		return StageResult{Verdict: VerdictContinue, Note: "filed without asking"}, nil
	}
	if c.Held {
		return StageResult{Verdict: VerdictReview, Note: "tagged, but not confidently enough to file unattended"}, nil
	}
	return StageResult{Verdict: VerdictContinue}, nil
}

// unidentified reports that every tier ran and NOTHING was grounded.
//
// ⚠ **The `AITagged` guard is the load-bearing half, and leaving it out would be the worst bug in
// this phase.** `filler.reject.unidentified` defaults ON, so without it a clip the tagger has
// simply never reached — an install with no LLM, a catalog imported before tagging existed, the
// very first pipeline pass — would be tombstoned for "we could not identify it" when nothing ever
// tried. **You cannot conclude that a signal is absent from a tier that never ran.**
//
// So the reject needs BOTH: no grounding of any kind, AND evidence that something looked. Anything
// else falls through to review, which is the direction that costs an operator a decision rather
// than a clip.
func (s *ScoreStage) unidentified(c StoreClip) bool {
	if !c.AITagged {
		return false // nothing looked, so nothing can be concluded
	}
	return !hasAnyGrounding(c)
}

// hasAnyGrounding reports whether ANY tier found something — tags, era, audience, an advertiser,
// speech, or on-screen text.
func hasAnyGrounding(c StoreClip) bool {
	return c.Era > 0 || c.Audience != "" || c.Category != "" || len(c.Tags) > 0 ||
		c.Brand != "" || c.VisibleText != "" ||
		(c.Transcript != "" && c.Transcript != TranscriptNone)
}

// ScoreClip computes a clip's grounding-capped confidence from what is on its row.
//
// ⚠ It reuses `TagSuggestion.Score` rather than re-deriving the ceilings, so the pipeline and the
// tagger cannot disagree about what a clip is worth. In particular `SuggestedEra > 0` — an era the
// model proposed but could not ground — caps the score strictly below every settable threshold, so
// a fabricated era can never be auto-filed however this install is configured.
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
	// happen to verify — score the full grounded 100 and be filed unattended, silently undoing the
	// asymmetry `Score` documents.
	//
	// Raising is still impossible: `Score` only ever takes the model layer when it is BELOW the
	// grounded ceiling, so a stale high value on the row cannot lift a clip whose grounding got
	// worse. A stale LOW one survives, which errs toward asking a human — the safe direction.
	return sug.Score(c.Confidence)
}
