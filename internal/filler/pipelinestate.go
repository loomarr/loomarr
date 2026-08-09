package filler

import (
	"context"
	"time"
)

// The per-clip ingest pipeline's STATE (§10 V51b) — what stage a clip is at, how that stage is
// going, and what the clip's outcome was.
//
// ⚠ This lives in a table beside `clips`, never on it. `clips` is a synced cache of the
// drop-folder that has been dropped and recreated twice; this records that ~341s of Whisper and a
// paid vision call have ALREADY been spent, which is exactly the kind of fact a cache rebuild must
// not silently discard. Migration 00044 carries the full argument.

// StageID names one rung of the pipeline.
type StageID string

const (
	StageProbe      StageID = "probe"
	StageTranscode  StageID = "transcode"
	StageSplit      StageID = "split"
	StageLanguage   StageID = "language"
	StageTranscribe StageID = "transcribe"
	StageTag        StageID = "tag"
	StageVision     StageID = "vision"
	StageScore      StageID = "score"
)

// StageOrder is the pipeline, in order. It is the ONE definition of the sequence: the runner
// advances through it, the ladder is rendered from it, and `Rewind` resets a suffix of it.
//
// ⚠ Order is not arbitrary. `probe` measures the file everything else reasons about; `transcode`
// rewrites bytes so it must precede anything that hashes or reads frames; `split` spawns new
// clips, so it runs before the per-clip metadata rungs that those children will each need;
// `transcribe` must precede `tag` because the transcript is one of the text signals `tag` grounds
// against (running them the other way round is what the cron schedule did, and why a clip could
// be scored low against a transcript that arrived ten minutes later); `score` is last because it
// reads what every rung above produced.
var StageOrder = []StageID{
	StageProbe, StageTranscode, StageSplit, StageLanguage,
	StageTranscribe, StageTag, StageVision, StageScore,
}

// StageIndex returns a stage's position in StageOrder, or -1 when it is not a stage.
//
// ⚠ Returns -1 rather than 0 for an unknown id. 0 is a real, meaningful position (probe), so a
// zero-value fallback would silently rewind an unknown stage to the very beginning of the
// pipeline — re-running a transcode and every expensive rung after it.
func StageIndex(id StageID) int {
	for i, s := range StageOrder {
		if s == id {
			return i
		}
	}
	return -1
}

// StageStatus is how the CURRENT stage is going.
type StageStatus string

const (
	StatusQueued  StageStatus = "queued"
	StatusRunning StageStatus = "running"
	StatusDone    StageStatus = "done"
	StatusFailed  StageStatus = "failed"
	// StatusSkipped means the stage does not APPLY to this clip in this install — vision with
	// `filler.vision.enabled` off, transcription for a clip whose description already says enough.
	//
	// ⚠ Distinct from `done`, and the difference is re-evaluation: a skipped stage is asked again
	// on every pass, so turning a setting on picks up clips that already went past it. A `done`
	// stage is never revisited. Collapsing the two would mean an operator enabling vision sees it
	// apply only to clips that arrive afterwards.
	StatusSkipped StageStatus = "skipped"
)

// Disposition is the CLIP-level outcome — what the operator sees.
type Disposition string

const (
	DispositionRunning  Disposition = "running"
	DispositionReview   Disposition = "review"
	DispositionFiled    Disposition = "filed"
	DispositionRejected Disposition = "rejected"
)

// Terminal reports whether the pipeline is finished with this clip. A terminal clip is not picked
// up by the work list again unless `Rewind` puts it back.
//
// ⚠ `review` is TERMINAL for the pipeline even though it is not terminal for the operator. The
// pipeline has done everything it can and is waiting on a person; leaving it in the work list
// would mean re-running the whole ladder against a clip whose only missing input is a human
// decision.
func (d Disposition) Terminal() bool {
	return d == DispositionFiled || d == DispositionRejected || d == DispositionReview
}

// StageRecord is one finished rung on a clip's ladder — what the Incoming tab renders as history.
type StageRecord struct {
	Stage  StageID     `json:"stage"`
	Status StageStatus `json:"status"`
	// Note is the operator-facing sentence for a skip or a failure ("the description already says
	// enough"). Empty for an ordinary success — a row that needs no explanation gets none.
	Note     string    `json:"note,omitempty"`
	Attempts int       `json:"attempts,omitempty"`
	At       time.Time `json:"at"`
}

// ClipPipeline is one clip's pipeline row.
type ClipPipeline struct {
	ClipHash    string
	Stage       StageID
	Status      StageStatus
	Progress    int
	Disposition Disposition
	// RejectReason is a stable code; RejectDetail is the measured fact behind it.
	RejectReason RejectReason
	RejectDetail string
	Attempts     int
	// NextRun is when this row is next eligible. Zero means "now".
	NextRun    time.Time
	Stages     []StageRecord
	EnrolledAt time.Time
	UpdatedAt  time.Time
}

// Record appends (or replaces) this stage's entry on the ladder.
//
// ⚠ Replaces in place when the stage is already present, so a retry does not append a second row
// for the same rung. The ladder is a picture of the pipeline, not a log of attempts — the attempt
// COUNT carries that, and a ladder that grew a row per retry would push the useful rungs off the
// operator's screen for exactly the clips that are struggling.
func (p *ClipPipeline) Record(stage StageID, status StageStatus, note string, attempts int, at time.Time) {
	for i := range p.Stages {
		if p.Stages[i].Stage == stage {
			p.Stages[i] = StageRecord{Stage: stage, Status: status, Note: note, Attempts: attempts, At: at}
			return
		}
	}
	p.Stages = append(p.Stages, StageRecord{Stage: stage, Status: status, Note: note, Attempts: attempts, At: at})
}

// PipelineFilter narrows a pipeline listing.
type PipelineFilter struct {
	// ConveyorOnly returns everything on the Incoming belt: clips the machine is still working on
	// (`running`) AND clips it has finished with and handed to a person (`review`).
	//
	// ⚠ **It is deliberately BOTH, and the predecessor's name is why this changed.**
	// `NonTerminalOnly` (retired-ok) meant `running` alone, which forced Incoming to answer "who
	// needs a human?" from a second, older query over tag-shape — and the two disagreed: every clip
	// is enrolled at `probe/queued` AND held-and-untagged, so the same clip appeared in both lists,
	// one saying it needed a decision while the other said nothing needed the operator at all.
	// `review` is the pipeline's own word for the handoff, so one query now answers both halves.
	ConveyorOnly bool
	// RejectedOnly returns the audit half: what was refused, and why.
	RejectedOnly bool
	// Limit caps the rows returned; 0 means unbounded.
	Limit int
}

// PipelineStore is the slice of the store the pipeline needs (the sync.go/splitjob.go pattern —
// declared here so `filler` does not import `store`; `app` bridges them).
//
// ⚠ `UpsertClipPipeline` is the ONLY writer of this table, which is what keeps the state machine
// in one place. Every other filler column keeps its existing owner (SetClipLanguage,
// SetClipTranscript, SetClipVisionTags, SetClipsHeld, …) — this phase adds no second writer to any
// of them, which is why the 1600-line store conformance suite needs no rework to accept it.
type PipelineStore interface {
	// ListPipelineWork returns non-terminal rows due at or before `now`, oldest first.
	ListPipelineWork(ctx context.Context, now time.Time, limit int) ([]ClipPipeline, error)
	// ListClipsWithoutPipeline returns catalogued clips that have no pipeline row yet — the
	// lazy enrolment the pipeline self-heals with, instead of a data migration.
	ListClipsWithoutPipeline(ctx context.Context, limit int) ([]StoreClip, error)
	GetClipPipeline(ctx context.Context, hash string) (ClipPipeline, bool, error)
	ListClipPipelines(ctx context.Context, f PipelineFilter) ([]ClipPipeline, error)
	UpsertClipPipeline(ctx context.Context, p ClipPipeline) error
}
