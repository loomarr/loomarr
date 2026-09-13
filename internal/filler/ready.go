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
	if clip.Hash == "" || clip.Path == "" || row.ClipHash != clip.Hash || row.Disposition != DispositionRunning {
		return row, fmt.Errorf("%w: clip and pipeline identity must be current", ErrReadyStale)
	}
	if clip.IsComposite {
		return row, fmt.Errorf("%w: composite containers are not playable", ErrReadyStale)
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
	row.Disposition = DispositionFiled
	row.Status = StatusDone
	row.Progress = 100
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
	if err := t.repository.CommitFillerReady(ctx, ReadyCommit{Event: event, Pipeline: row}); err != nil {
		return row, err
	}
	return row, nil
}
