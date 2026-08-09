package filler

import (
	"context"
	"fmt"
	"io/fs"
	"path/filepath"
	"time"
)

// The TRANSCRIBE stage (§10 V51b): what does the clip SAY?
//
// V44's `TranscribeJob` logic, unchanged — the same `MediaTools.Transcribe` seam, the same
// thin-source selectivity, the same `TranscriptNone` sentinel. The sweep around it is gone.
//
// ⚠ **It runs before `tag`, and as a rung that ordering is now guaranteed rather than hoped for.**
// The transcript is one of the text signals the tagger grounds against, so a tagger that runs
// first scores a clip against a transcript that arrives ten minutes later — which is precisely
// what two independent cron schedules produced. `StageOrder` states the dependency once.

// TranscriptNone is the sentinel a wordless clip records (§10 V44) — the transcript column's
// analogue of the language gate's LangNone.
//
// ⚠ **A NON-EMPTY marker is load-bearing.** The transcript column has the same two meanings
// `language` does — "not yet transcribed" and "transcribed, wordless" — and the row default for
// BOTH is `""`. Recording a literal `""` for a silent clip could not distinguish them, so every
// pass would re-run Whisper on every wordless clip forever, ~341s each under QEMU.
//
// It is deliberately a bracketed marker, not real speech, so a search over transcripts never
// surfaces it as content and the tagger — which reads `Transcript` as a text signal — sees a token
// it will not ground a brand or era on. A wordless clip must still be tagged from its OTHER
// signals (its filename, its sidecar, or the vision rung), never from this placeholder.
//
// ⚠ **A documented tension with 00038's column note**, surfaced rather than papered over: that
// migration says the row stores `""` for a wordless clip and "does not need to tell [the two
// meanings] apart". Both cannot hold. This resolves it toward the never-re-run guarantee the same
// migration states as its whole point.
const TranscriptNone = "[no speech]"

// ThinSourceRunes is the description length below which a clip's source text is "thin" and Whisper
// earns its cost (§10 V44).
//
// ~40 characters: an archive.org item that declares only "commercial" or a bare title says almost
// nothing the tagger can ground a brand or era on, while a two-sentence real description does. The
// threshold is where a source description stops being a signal and starts being a placeholder — an
// empty sidecar (the common drop-folder case) is trivially below it.
const ThinSourceRunes = 40

// TranscribeClipStore is the slice of the store the transcribe stage writes through.
type TranscribeClipStore interface {
	// SetClipTranscript records what the clip says — for EVERY outcome, including the
	// TranscriptNone sentinel for a wordless one.
	SetClipTranscript(ctx context.Context, path, transcript string, at time.Time) error
}

// TranscribeStage turns speech into the tagger's richest input.
type TranscribeStage struct {
	tools   MediaTools
	store   TranscribeClipStore
	clipDir string
	// drop is FILLER_DIR as an fs.FS, for reading the sidecar that decides whether a clip's source
	// text is thin. nil ⇒ every clip reads as thin, which only ever transcribes MORE — the safe
	// direction, since a clip we cannot read a description for is exactly the one Whisper helps.
	drop    fs.FS
	enabled func() bool
	now     func() time.Time
}

// NewTranscribeStage builds the stage.
func NewTranscribeStage(tools MediaTools, store TranscribeClipStore, clipDir string, drop fs.FS, enabled func() bool, now func() time.Time) *TranscribeStage {
	if now == nil {
		now = time.Now
	}
	return &TranscribeStage{tools: tools, store: store, clipDir: clipDir, drop: drop, enabled: enabled, now: now}
}

func (s *TranscribeStage) ID() StageID     { return StageTranscribe }
func (s *TranscribeStage) Cost() StageCost { return CostWhisper }

// Applies when transcription is on, the clip has not been heard, and it stands to gain.
func (s *TranscribeStage) Applies(_ context.Context, c StoreClip) (bool, string) {
	if s.tools == nil {
		return false, "no transcription backend is configured"
	}
	if s.enabled == nil || !s.enabled() {
		return false, "transcription is off"
	}
	// ⚠ `""` is the ONLY value meaning "not yet". A recorded `TranscriptNone` is a FINAL answer —
	// the clip was heard and is wordless — and re-hearing it would spend ~341s under QEMU to learn
	// the same nothing.
	if c.Transcript != "" {
		return false, "already transcribed"
	}
	if !s.needsTranscription(c) {
		return false, "its source description already says enough"
	}
	return true, ""
}

// Run transcribes the clip and records the result.
func (s *TranscribeStage) Run(ctx context.Context, c StoreClip) (StageResult, error) {
	start, end := LanguageSpan(c.DurationMs)
	file := filepath.Join(s.clipDir, filepath.FromSlash(c.Path))

	segs, err := s.tools.Transcribe(ctx, file, start, end)
	if err != nil {
		// Not recorded: a backend failure says nothing about the clip, so the runner retries and
		// then skips. ⚠ A missing transcript must never strand a clip — `transcribe` is not in
		// `fatalStages` for exactly this reason.
		return StageResult{}, fmt.Errorf("transcribe %s: %w", c.Path, err)
	}
	reportProgress(ctx, StageTranscribe, 100)

	text := TranscriptText(segs)
	wordless := text == ""
	if wordless {
		// ⚠ The NON-EMPTY sentinel, never a literal "". See TranscriptNone: `""`-vs-`""` is not a
		// distinction, so recording an empty string for a silent clip would make every pass hear
		// it again forever.
		text = TranscriptNone
	}
	if s.store != nil && c.Path != "" {
		if err := s.store.SetClipTranscript(ctx, c.Path, text, s.now().UTC()); err != nil {
			return StageResult{}, err
		}
	}
	updated := c
	updated.Transcript = text
	if wordless {
		return StageResult{Clip: updated, Verdict: VerdictContinue, Note: "no speech in it"}, nil
	}
	return StageResult{Clip: updated, Verdict: VerdictContinue}, nil
}

// needsTranscription decides whether a clip earns Whisper (§10 V44 "selective by design"): its
// source text is THIN, OR it is still untagged after the text-only pass.
//
// ⚠ The two conditions are an OR, and both matter. A clip with a THIN description is a candidate
// even if it happens to be tagged — its transcript is still the only place a brand can be grounded.
// A clip with a RICH description that is nonetheless still UNTAGGED is also a candidate — the
// description did not carry what the tagger needed, so the spoken text is the next signal to try.
// Only a clip that is BOTH richly-described AND already tagged has nothing to gain.
func (s *TranscribeStage) needsTranscription(c StoreClip) bool {
	if s.sourceTextThin(c) {
		return true
	}
	return !c.Tagged()
}

// sourceTextThin reports whether a clip's source description is empty or near-empty (§10 V44).
//
// ⚠ Reads the sidecar via `SidecarText`, the SAME signal the tagger reads, so "thin" here means
// exactly "thin to the tagger".
func (s *TranscribeStage) sourceTextThin(c StoreClip) bool {
	if s.drop == nil || c.Path == "" {
		return true
	}
	return len([]rune(SidecarText(s.drop, c.Path))) < ThinSourceRunes
}
