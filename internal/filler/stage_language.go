package filler

import (
	"context"
	"fmt"
	"path/filepath"
	"time"

	"github.com/loomarr/loomarr/internal/filler/language"
	"github.com/loomarr/loomarr/internal/filler/pipelinestate"
	"github.com/loomarr/loomarr/internal/platform/store/clipstore"
)

// The LANGUAGE stage (§10 V51b): what language is the speech in, and does some channel air it
// (#2030 decision 13)?
//
// The decision is the language package's: its detector seam, its `language.Span` window and its
// `language.Rejects` verdict. The runner owns the work list, the batching and the tombstones.
//
// ⚠ It is a rung, so it runs before the clip becomes Ready: a foreign clip never airs before the
// gate has heard it.

// LanguageClipStore is the slice of the store the language stage writes through.
type LanguageClipStore interface {
	// SetClipLanguage records what was heard — for EVERY answer including `none`, because an
	// unrecorded answer is indistinguishable from "not yet checked".
	SetClipLanguage(ctx context.Context, path, language string, at time.Time) error
}

// LanguageStage hears a clip and refuses the ones speaking the wrong language.
type LanguageStage struct {
	detector language.Detector
	store    LanguageClipStore
	clipDir  string
	// want is the set of languages filler may be in (language.IntakeSet), read live so a new channel
	// or a settings change applies on the next pass. Empty ⇒ the gate is off and the stage does not
	// apply. It reads the channels, so it can fail.
	want func(context.Context) (string, error)
	now  func() time.Time
}

// NewLanguageStage builds the stage. A nil detector or an empty `want` makes it permanently
// inapplicable, so an install that has not opted in pays nothing and its ladder says why.
func NewLanguageStage(detector language.Detector, store LanguageClipStore, clipDir string, want func(context.Context) (string, error), now func() time.Time) *LanguageStage {
	if now == nil {
		now = time.Now
	}
	return &LanguageStage{detector: detector, store: store, clipDir: clipDir, want: want, now: now}
}

func (s *LanguageStage) ID() pipelinestate.StageID { return pipelinestate.StageLanguage }

// Cost: whisper. The local backend samples at most 30s of audio, so it remains background work and
// draws on the same bounded budget as transcription.
func (s *LanguageStage) Cost() StageCost { return CostWhisper }

// Applies when the gate is on and this clip has not been heard yet.
func (s *LanguageStage) Applies(ctx context.Context, c clipstore.Clip) (bool, string) {
	if s.detector == nil {
		return false, "no language backend is configured"
	}
	if s.want == nil {
		return false, "the language gate is off"
	}
	// ⚠ A failed read is not "off": an inapplicable rung is skipped for good, so the clip would
	// never be heard. It applies, and Run meets the same failure and retries with backoff.
	if want, err := s.want(ctx); err == nil && want == "" {
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
func (s *LanguageStage) Run(ctx context.Context, c clipstore.Clip) (StageResult, error) {
	want := ""
	if s.want != nil {
		var err error
		if want, err = s.want(ctx); err != nil {
			return StageResult{}, fmt.Errorf("read the channels' commercial languages: %w", err)
		}
	}
	start, end := language.Span(c.DurationMs)
	file := filepath.Join(s.clipDir, filepath.FromSlash(c.Path))

	detected, err := s.detector.DetectLanguage(ctx, file, start, end)
	if err != nil {
		// A backend failure says nothing about the clip, so nothing is recorded and the runner
		// retries with backoff. `language` is not fatal, so exhausting the retries skips the rung
		// and the clip carries on — a gate we could not run must never strand a commercial.
		return StageResult{}, fmt.Errorf("detect language for %s: %w", c.Path, err)
	}
	pipelinestate.ReportProgress(ctx, pipelinestate.StageLanguage, 100)

	if detected == language.Undetermined {
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

	if detected == language.None {
		return StageResult{Clip: updated, Verdict: pipelinestate.VerdictContinue, Note: "no speech to judge"}, nil
	}
	if language.Rejects(detected, want) {
		return StageResult{
			Clip: updated, Verdict: pipelinestate.VerdictReject, Reason: pipelinestate.ReasonLanguage,
			Detail: fmt.Sprintf("heard %s; the channels air %s",
				language.Normalize(detected), language.Names(want)),
		}, nil
	}
	return StageResult{Clip: updated, Verdict: pipelinestate.VerdictContinue}, nil
}
