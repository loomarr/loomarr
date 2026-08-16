package filler

import (
	"context"
	"fmt"
	"path/filepath"
	"time"
)

// The LANGUAGE stage (§10 V51b): what language is the speech in, and is it the one this install
// asked for?
//
// The logic is V40's `LanguageJob` unchanged — same detector seam, same `LanguageSpan` window,
// same `LanguageRejects` verdict. What changes is everything around it: the job's own work list,
// its own batch constant and its own tombstone call are gone, because the runner owns all three.
//
// ⚠ **The window V40 documented is now closed, and that is the point of moving it.** The job ran
// on a timer *after* a clip was already catalogued, so a foreign clip could air once before the
// gate reached it — an accepted cost of not blocking the scan. As a rung it runs before the clip
// is ever filed, so there is no window at all.

// LanguageClipStore is the slice of the store the language stage writes through.
type LanguageClipStore interface {
	// SetClipLanguage records what was heard — for EVERY answer including `none`, because an
	// unrecorded answer is indistinguishable from "not yet checked".
	SetClipLanguage(ctx context.Context, path, language string, at time.Time) error
}

// LanguageStage hears a clip and refuses the ones speaking the wrong language.
type LanguageStage struct {
	detector LanguageDetector
	store    LanguageClipStore
	clipDir  string
	// want is the language filler is expected to be in, read live so a settings change applies on
	// the next pass. Empty ⇒ the gate is off and the stage does not apply.
	want func() string
	now  func() time.Time
}

// NewLanguageStage builds the stage. A nil detector or an empty `want` makes it permanently
// inapplicable, so an install that has not opted in pays nothing and its ladder says why.
func NewLanguageStage(detector LanguageDetector, store LanguageClipStore, clipDir string, want func() string, now func() time.Time) *LanguageStage {
	if now == nil {
		now = time.Now
	}
	return &LanguageStage{detector: detector, store: store, clipDir: clipDir, want: want, now: now}
}

func (s *LanguageStage) ID() StageID { return StageLanguage }

// Cost: whisper. The local backend samples ~10s of audio, which is ~3s natively and ~341s under
// QEMU — the same spend as a transcription, so it draws on the same budget.
func (s *LanguageStage) Cost() StageCost { return CostWhisper }

// Applies when the gate is on and this clip has not been heard yet.
func (s *LanguageStage) Applies(_ context.Context, c StoreClip) (bool, string) {
	if s.detector == nil {
		return false, "no language backend is configured"
	}
	if s.want == nil || s.want() == "" {
		return false, "the language gate is off"
	}
	// ⚠ `""` is the ONLY value meaning "not yet". `none` is a final answer — the clip was heard
	// and has no speech — and re-checking it would spend the whole whisper cost to learn the same
	// thing. `SetClipTranscript` draws the identical distinction with `TranscriptNone`.
	if c.Language != "" {
		return false, "already heard: " + c.Language
	}
	if why := s.detector.UnavailableReason(); why != "" {
		return false, why
	}
	return true, ""
}

// Run hears the clip, records the answer, and rejects a foreign one.
func (s *LanguageStage) Run(ctx context.Context, c StoreClip) (StageResult, error) {
	want := ""
	if s.want != nil {
		want = s.want()
	}
	start, end := LanguageSpan(c.DurationMs)
	file := filepath.Join(s.clipDir, filepath.FromSlash(c.Path))

	detected, err := s.detector.DetectLanguage(ctx, file, start, end)
	if err != nil {
		// A backend failure says nothing about the clip, so nothing is recorded and the runner
		// retries with backoff. `language` is not fatal, so exhausting the retries skips the rung
		// and the clip carries on — a gate we could not run must never strand a commercial.
		return StageResult{}, fmt.Errorf("detect language for %s: %w", c.Path, err)
	}
	reportProgress(ctx, StageLanguage, 100)

	if detected == LangUndetermined {
		// ⚠ **The backend ran and could not tell — treated as a failure so the retry ladder
		// applies, which is a deliberate change from V40's forever-retry.** The old job left
		// `language` empty and re-asked on every tick, so an install with `want` set and no
		// whisper model re-ran the same unanswerable question hourly, at full cost, forever.
		// Bounded retries end that.
		//
		// The cost of bounding it, stated rather than hidden: an operator who wires a model up
		// *after* a clip has exhausted its three attempts will not have it re-heard automatically,
		// because the rung is resolved. `Rewind(hash, StageLanguage)` is the sanctioned way back,
		// and it is one click from the clip.
		return StageResult{}, fmt.Errorf("the language backend ran but could not tell")
	}

	// Record BEFORE judging. If the reject below fails to persist, the answer is still stored and
	// the next pass will not spend whisper on it again.
	if s.store != nil && c.Path != "" {
		if err := s.store.SetClipLanguage(ctx, c.Path, detected, s.now().UTC()); err != nil {
			return StageResult{}, err
		}
	}
	updated := c
	updated.Language = detected

	if detected == LangNone {
		return StageResult{Clip: updated, Verdict: VerdictContinue, Note: "no speech to judge"}, nil
	}
	if LanguageRejects(detected, want) {
		return StageResult{
			Clip: updated, Verdict: VerdictReject, Reason: ReasonLanguage,
			Detail: fmt.Sprintf("heard %s; this install wants %s",
				NormalizeLanguage(detected), NormalizeLanguage(want)),
		}, nil
	}
	return StageResult{Clip: updated, Verdict: VerdictContinue}, nil
}
