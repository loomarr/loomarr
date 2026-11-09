package filler

import (
	"context"
	"fmt"
	"io/fs"
	"path/filepath"
	"time"

	"github.com/loomarr/loomarr/internal/filler/asset"
	"github.com/loomarr/loomarr/internal/filler/clips"
	"github.com/loomarr/loomarr/internal/filler/language"
	"github.com/loomarr/loomarr/internal/filler/pipelinestate"
	"github.com/loomarr/loomarr/internal/media/tools"
	"github.com/loomarr/loomarr/internal/platform/store/clipstore"
)

// The TRANSCRIBE stage (§10 V51b): what does the clip SAY?
//
// It goes through the `MediaTools.Transcribe` seam, is selective about thin sources, and records
// the `TranscriptNone` sentinel for a wordless clip.
//
// ⚠ **It runs before `tag`, and as a rung that ordering is now guaranteed rather than hoped for.**
// The transcript is one of the text signals the tagger grounds against, so a tagger that runs
// first scores a clip against a transcript that arrives ten minutes later — which is precisely
// what two independent cron schedules produced. `StageOrder` states the dependency once.

// TranscriptionPassVersion changes when the media-to-transcript semantics change. Progressive
// enrichment binds completion to this identity so a changed implementation may revisit eligible
// clips without making the readiness conveyor authoritative for catch-up.
const TranscriptionPassVersion = "filler-transcription-v1"

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
	tools   tools.MediaTools
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
func NewTranscribeStage(tools tools.MediaTools, store TranscribeClipStore, clipDir string, drop fs.FS, enabled func() bool, now func() time.Time) *TranscribeStage {
	if now == nil {
		now = time.Now
	}
	return &TranscribeStage{tools: tools, store: store, clipDir: clipDir, drop: drop, enabled: enabled, now: now}
}

func (s *TranscribeStage) ID() pipelinestate.StageID { return pipelinestate.StageTranscribe }
func (s *TranscribeStage) Cost() StageCost           { return CostWhisper }

// Applies when transcription is on, the clip has not been heard, and it stands to gain.
func (s *TranscribeStage) Applies(_ context.Context, c clipstore.Clip) (bool, string) {
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

// Observe transcribes without persisting. Progressive catch-up uses this half so the transcript,
// accepted facts, and completion identity can commit atomically in its own store transaction.
func (s *TranscribeStage) Observe(ctx context.Context, c clipstore.Clip) (StageResult, string, error) {
	start, end := language.Span(c.DurationMs)
	file := filepath.Join(s.clipDir, filepath.FromSlash(c.Path))

	segs, err := s.tools.Transcribe(ctx, file, start, end)
	if err != nil {
		// Not recorded: a backend failure says nothing about the clip, so the runner retries and
		// then skips. ⚠ A missing transcript must never strand a clip — `transcribe` is not in
		// `fatalStages` for exactly this reason.
		return StageResult{}, "", fmt.Errorf("transcribe %s: %w", c.Path, err)
	}
	pipelinestate.ReportProgress(ctx, pipelinestate.StageTranscribe, 100)

	text := TranscriptText(segs)
	wordless := text == ""
	if wordless {
		// ⚠ The NON-EMPTY sentinel, never a literal "". See TranscriptNone: `""`-vs-`""` is not a
		// distinction, so recording an empty string for a silent clip would make every pass hear
		// it again forever.
		text = clips.TranscriptNone
	}
	updated := c
	updated.Transcript = text
	if wordless {
		return StageResult{Clip: updated, Verdict: pipelinestate.VerdictContinue, Note: "no speech in it"}, text, nil
	}
	return StageResult{Clip: updated, Verdict: pipelinestate.VerdictContinue}, text, nil
}

// Run is the readiness-stage adapter around Observe.
func (s *TranscribeStage) Run(ctx context.Context, c clipstore.Clip) (StageResult, error) {
	result, transcript, err := s.Observe(ctx, c)
	if err != nil {
		return StageResult{}, err
	}
	if s.store != nil && c.Path != "" {
		if err := s.store.SetClipTranscript(ctx, c.Path, transcript, s.now().UTC()); err != nil {
			return StageResult{}, err
		}
	}
	return result, nil
}

// needsTranscription decides whether a clip earns Whisper (§10 V44 "selective by design"): its
// source text is THIN, OR it is still untagged after the text-only pass.
//
// ⚠ The two conditions are an OR, and both matter. A clip with a THIN description is a candidate
// even if it happens to be tagged — its transcript is still the only place a brand can be grounded.
// A clip with a RICH description that is nonetheless still UNTAGGED is also a candidate — the
// description did not carry what the tagger needed, so the spoken text is the next signal to try.
// Only a clip that is BOTH richly-described AND already tagged has nothing to gain.
func (s *TranscribeStage) needsTranscription(c clipstore.Clip) bool {
	if s.sourceTextThin(c) {
		return true
	}
	return !c.Tagged()
}

// sourceTextThin reports whether a clip's source description is empty or near-empty (§10 V44).
//
// ⚠ Reads the sidecar via `SidecarText`, the SAME signal the tagger reads, so "thin" here means
// exactly "thin to the tagger".
func (s *TranscribeStage) sourceTextThin(c clipstore.Clip) bool {
	if s.drop == nil || c.Path == "" {
		return true
	}
	return len([]rune(asset.SidecarText(s.drop, c.Path))) < ThinSourceRunes
}
