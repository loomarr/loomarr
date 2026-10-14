package filler

import (
	"context"
	"errors"
	"fmt"
	"time"
)

// EnrollmentKind names the durable user choice that put a clip into Filler.
type EnrollmentKind string

const (
	EnrollmentAcquisition EnrollmentKind = "acquisition"
	EnrollmentSource      EnrollmentKind = "source"
)

// Enrollment is household-use authority, not a content or licensing judgment.
type Enrollment struct {
	Kind      EnrollmentKind
	Reference string
}

// EnrollmentFor derives authority from facts already persisted with the clip and conveyor. A
// remote fetch or explicit queue has an acquisition id; watched folders and libraries retain their
// source id on the clip. Nothing guesses from a label or classification score.
func EnrollmentFor(clip Clip, row ClipPipeline) (Enrollment, bool) {
	if row.AcquisitionID != "" {
		return Enrollment{Kind: EnrollmentAcquisition, Reference: row.AcquisitionID}, true
	}
	if clip.Source != "" {
		return Enrollment{Kind: EnrollmentSource, Reference: clip.Source}, true
	}
	return Enrollment{}, false
}

// ReadyEvent is the immutable effective outcome written with catalog publication.
type ReadyEvent struct {
	ID            string
	ClipHash      string
	AcquisitionID string
	Enrollment    Enrollment
	Placement     Placement
	CreatedAt     time.Time
}

// ReadyCommit is the complete state one atomic store operation must publish.
type ReadyCommit struct {
	Event    ReadyEvent
	Pipeline ClipPipeline
}

// Validate proves the caller is committing the one finished, playable outcome shape. The store
// calls it again at the transaction boundary so no alternate ReadyRepository caller can bypass the
// terminal module's ladder checks.
func (c ReadyCommit) Validate() error {
	e, p := c.Event, c.Pipeline
	if e.ID != "ready:"+e.ClipHash || e.ClipHash == "" || e.ClipHash != p.ClipHash ||
		e.AcquisitionID != p.AcquisitionID || e.Enrollment.Reference == "" ||
		(e.Placement != PlacementBreakBody && e.Placement != PlacementBookend) ||
		p.Disposition != DispositionReady || e.CreatedAt.IsZero() {
		return fmt.Errorf("%w: incomplete ready commit", ErrReadyStale)
	}
	switch e.Enrollment.Kind {
	case EnrollmentAcquisition:
		if e.AcquisitionID == "" || e.Enrollment.Reference != e.AcquisitionID {
			return fmt.Errorf("%w: acquisition enrollment does not match the conveyor", ErrReadyStale)
		}
	case EnrollmentSource:
		if e.AcquisitionID != "" {
			return fmt.Errorf("%w: source enrollment cannot replace acquisition authority", ErrReadyStale)
		}
	default:
		return fmt.Errorf("%w: enrollment kind is invalid", ErrReadyStale)
	}
	return validateCompletedReadyLadder(p)
}

// ValidateAgainst proves that the proposed terminal value is exactly the next state of the row
// locked by the repository. The runner persists the final rung as running before doing its work;
// the proposed value may add only that rung's successful record and the terminal Ready fields.
// This prevents a caller with an old or fabricated completed ladder from publishing over a newer
// conveyor row whose four header strings happen to match.
func (c ReadyCommit) ValidateAgainst(current ClipPipeline) error {
	if err := c.Validate(); err != nil {
		return err
	}
	p := c.Pipeline
	if current.ClipHash != p.ClipHash || current.AcquisitionID != p.AcquisitionID ||
		current.Stage != p.Stage || current.Stage != StageScore || current.Status != StatusRunning ||
		current.Disposition != DispositionRunning || current.Attempts != p.Attempts ||
		p.ForceRun || current.RejectReason != p.RejectReason ||
		current.RejectDetail != p.RejectDetail || !current.EnrolledAt.Equal(p.EnrolledAt) ||
		current.PreparationAttempt != p.PreparationAttempt ||
		!samePersistedSecond(current.PreparationStartedAt, p.PreparationStartedAt) ||
		current.PreparationStartReason != p.PreparationStartReason ||
		!samePersistedSecond(current.StageQueuedAt, p.StageQueuedAt) ||
		!samePersistedSecond(current.StageStartedAt, p.StageStartedAt) ||
		p.UpdatedAt.Before(current.UpdatedAt) {
		return fmt.Errorf("%w: conveyor row is no longer the completed run", ErrReadyStale)
	}
	if len(p.Stages) != len(current.Stages)+1 {
		return fmt.Errorf("%w: terminal rung is not the only ladder change", ErrReadyStale)
	}
	for i := range current.Stages {
		if current.Stages[i] != p.Stages[i] {
			return fmt.Errorf("%w: conveyor ladder changed before publication", ErrReadyStale)
		}
	}
	last := p.Stages[len(p.Stages)-1]
	if last.Stage != StageScore || last.Status != StatusDone {
		return fmt.Errorf("%w: final score record is not complete", ErrReadyStale)
	}
	return nil
}

// samePersistedSecond compares timestamps at the precision owned by filler_clip_pipeline. The
// pipeline retains sub-second stage timing in its JSON ladder, while the current-stage columns use
// the store's Unix-second convention. A terminal commit still carries the in-memory nanoseconds
// from the just-finished rung, so comparing those columns at nanosecond precision would reject the
// exact row that the repository just read back.
func samePersistedSecond(a, b time.Time) bool {
	if a.IsZero() || b.IsZero() {
		return a.IsZero() && b.IsZero()
	}
	return a.Unix() == b.Unix()
}

// ReadyRepository is the one publication seam. Ordinary clip and pipeline writers cannot clear a
// hold while also claiming terminal completion.
type ReadyRepository interface {
	CommitFillerReady(context.Context, ReadyCommit) error
}

var (
	ErrReadyUnavailable = errors.New("filler terminal readiness is not configured")
	ErrReadyStale       = errors.New("filler terminal readiness state changed")
)

// TerminalReady validates domain state and delegates the indivisible publication write.
type TerminalReady struct {
	repository ReadyRepository
	now        func() time.Time
}

func NewTerminalReady(repository ReadyRepository, now func() time.Time) *TerminalReady {
	if now == nil {
		now = time.Now
	}
	return &TerminalReady{repository: repository, now: now}
}

// Commit makes one completed non-composite playable. It never changes Kind.
func (t *TerminalReady) Commit(ctx context.Context, clip StoreClip, row ClipPipeline) (ClipPipeline, error) {
	if t == nil || t.repository == nil {
		return row, ErrReadyUnavailable
	}
	if clip.Hash == "" || clip.Path == "" || row.ClipHash != clip.Hash || !clip.Held ||
		!clip.RemovedAt.IsZero() || row.Disposition != DispositionRunning {
		return row, fmt.Errorf("%w: clip and pipeline identity must be current", ErrReadyStale)
	}
	if clip.IsComposite {
		return row, fmt.Errorf("%w: composite containers are not playable", ErrReadyStale)
	}
	if err := validateCompletedReadyLadder(row); err != nil {
		return row, err
	}
	enrollment, ok := EnrollmentFor(clip.Clip, row)
	if !ok {
		return row, fmt.Errorf("%w: enrollment authority is missing", ErrReadyStale)
	}
	placement := PlacementForRole(clip.Kind, true, false)
	if placement == PlacementNotPlayable {
		return row, fmt.Errorf("%w: no playable placement", ErrReadyStale)
	}
	at := t.now().UTC()
	row.Disposition = DispositionReady
	row.Status = StatusDone
	row.Progress = 100
	row.PreparationProgress = 100
	row.NextRun = time.Time{}
	row.UpdatedAt = at
	event := ReadyEvent{
		ID:            "ready:" + clip.Hash,
		ClipHash:      clip.Hash,
		AcquisitionID: row.AcquisitionID,
		Enrollment:    enrollment,
		Placement:     placement,
		CreatedAt:     at,
	}
	commit := ReadyCommit{Event: event, Pipeline: row}
	if err := commit.Validate(); err != nil {
		return row, err
	}
	if err := t.repository.CommitFillerReady(ctx, commit); err != nil {
		return row, err
	}
	return row, nil
}

// validateCompletedReadyLadder keeps publication tied to the real conveyor rather than merely to
// a caller holding a ClipPipeline value. Every configured rung must have reached a terminal
// per-stage result, the final score rung must have completed (it is never optional), and the row
// itself must describe that same final state. Skipped optional/non-applicable work is evidence too;
// a missing, queued, running, or failed record is not.
func validateCompletedReadyLadder(row ClipPipeline) error {
	if len(StageOrder) == 0 || row.Stage != StageOrder[len(StageOrder)-1] ||
		row.Status != StatusDone || len(row.Stages) != len(StageOrder) {
		return fmt.Errorf("%w: pipeline has not completed its final stage", ErrReadyStale)
	}
	for i, stage := range StageOrder {
		record := row.Stages[i]
		if record.Stage != stage || (record.Status != StatusDone && record.Status != StatusSkipped) {
			return fmt.Errorf("%w: pipeline stage %s is not complete", ErrReadyStale, stage)
		}
	}
	if row.Stages[StageIndex(StageProbe)].Status != StatusDone ||
		row.Stages[StageIndex(StageScore)].Status != StatusDone {
		return fmt.Errorf("%w: required probe and score checks did not run", ErrReadyStale)
	}
	return nil
}
