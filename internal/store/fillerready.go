package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/loomarr/loomarr/internal/filler"
)

// GetFillerReadyEvent returns the effective publication event for one clip.
func (s *sqlStore) GetFillerReadyEvent(ctx context.Context, clipHash string) (filler.ReadyEvent, bool, error) {
	var event filler.ReadyEvent
	var enrollmentKind, placement string
	var createdAt int64
	err := s.db.QueryRowContext(ctx, s.ph(`SELECT id, clip_hash, acquisition_id, enrollment_kind,
		enrollment_ref, placement, created_at FROM filler_ready_events WHERE clip_hash = ?`), clipHash).
		Scan(&event.ID, &event.ClipHash, &event.AcquisitionID, &enrollmentKind,
			&event.Enrollment.Reference, &placement, &createdAt)
	if errors.Is(err, sql.ErrNoRows) {
		return filler.ReadyEvent{}, false, nil
	}
	if err != nil {
		return filler.ReadyEvent{}, false, fmt.Errorf("get filler ready event %s: %w", clipHash, err)
	}
	event.Enrollment.Kind = filler.EnrollmentKind(enrollmentKind)
	event.Placement = filler.Placement(placement)
	event.CreatedAt = time.Unix(createdAt, 0).UTC()
	return event, true, nil
}

// CommitFillerReady is the sole non-composite catalog-publication transaction. The event,
// placement, hold release, and conveyor settlement either all become visible or none do.
func (s *sqlStore) CommitFillerReady(ctx context.Context, commit filler.ReadyCommit) error {
	e := commit.Event
	p := commit.Pipeline
	if err := commit.Validate(); err != nil {
		return err
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin filler ready: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var existingClip, existingAcquisition, existingKind, existingRef, existingPlacement, existingOutcome string
	err = tx.QueryRowContext(ctx, s.ph(`SELECT clip_hash, acquisition_id, enrollment_kind,
		enrollment_ref, placement, outcome FROM filler_ready_events WHERE id = ?`), e.ID).
		Scan(&existingClip, &existingAcquisition, &existingKind, &existingRef, &existingPlacement, &existingOutcome)
	if err == nil {
		if existingClip == e.ClipHash && existingAcquisition == e.AcquisitionID &&
			existingKind == string(e.Enrollment.Kind) && existingRef == e.Enrollment.Reference &&
			existingPlacement == string(e.Placement) && existingOutcome == "ready" {
			return nil
		}
		return fmt.Errorf("%w: ready event id conflicts", filler.ErrReadyStale)
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("read filler ready event: %w", err)
	}

	clipQuery := `SELECT held, removed_at, is_composite, source FROM clips WHERE hash = ?`
	if s.dialect == DialectPostgres {
		clipQuery += ` FOR UPDATE`
	}
	var held, composite bool
	var removedAt int64
	var source string
	if err := tx.QueryRowContext(ctx, s.ph(clipQuery), e.ClipHash).Scan(&held, &removedAt, &composite, &source); errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	} else if err != nil {
		return fmt.Errorf("lock ready clip %s: %w", e.ClipHash, err)
	}
	if !held || removedAt != 0 || composite {
		return fmt.Errorf("%w: clip is not a held active non-composite", filler.ErrReadyStale)
	}
	if e.Enrollment.Kind == filler.EnrollmentSource && source != e.Enrollment.Reference {
		return fmt.Errorf("%w: source enrollment changed", filler.ErrReadyStale)
	}

	pipelineQuery := clipPipelineSelect + ` WHERE clip_hash = ?`
	if s.dialect == DialectPostgres {
		pipelineQuery += ` FOR UPDATE`
	}
	current, err := scanClipPipeline(tx.QueryRowContext(ctx, s.ph(pipelineQuery), e.ClipHash))
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	} else if err != nil {
		return fmt.Errorf("lock ready pipeline %s: %w", e.ClipHash, err)
	}
	if err := commit.ValidateAgainst(current); err != nil {
		return err
	}

	raw, err := json.Marshal(p.Stages)
	if err != nil {
		return fmt.Errorf("marshal ready pipeline ladder: %w", err)
	}
	if len(p.Stages) == 0 {
		raw = []byte("[]")
	}

	clipResult, err := tx.ExecContext(ctx, s.ph(`UPDATE clips SET placement = ?, held = ?, auto_filed = ?, updated_at = ?
		WHERE hash = ? AND held = ? AND removed_at = 0 AND is_composite = ?`),
		string(e.Placement), false, false, epoch(e.CreatedAt), e.ClipHash, true, false)
	if err != nil {
		return fmt.Errorf("publish ready clip %s: %w", e.ClipHash, err)
	}
	if n, countErr := clipResult.RowsAffected(); countErr != nil || n != 1 {
		if countErr != nil {
			return fmt.Errorf("count ready clip %s: %w", e.ClipHash, countErr)
		}
		return fmt.Errorf("%w: clip changed before publication", filler.ErrReadyStale)
	}

	pipelineResult, err := tx.ExecContext(ctx, s.ph(`UPDATE filler_clip_pipeline SET
		stage = ?, status = ?, progress = ?, disposition = ?, reject_reason = ?, reject_detail = ?,
		attempts = ?, force_run = ?, next_run = ?, stages_json = ?, updated_at = ?
		WHERE clip_hash = ? AND acquisition_id = ? AND stage = ? AND status = ? AND disposition = ?`),
		string(p.Stage), string(p.Status), p.Progress, string(p.Disposition), string(p.RejectReason),
		p.RejectDetail, p.Attempts, p.ForceRun, epoch(p.NextRun), string(raw), epoch(p.UpdatedAt),
		p.ClipHash, current.AcquisitionID, string(current.Stage), string(current.Status), string(current.Disposition))
	if err != nil {
		return fmt.Errorf("settle ready pipeline %s: %w", e.ClipHash, err)
	}
	if n, countErr := pipelineResult.RowsAffected(); countErr != nil || n != 1 {
		if countErr != nil {
			return fmt.Errorf("count ready pipeline %s: %w", e.ClipHash, countErr)
		}
		return fmt.Errorf("%w: conveyor changed before publication", filler.ErrReadyStale)
	}

	if _, err := tx.ExecContext(ctx, s.ph(`INSERT INTO filler_ready_events
		(id, clip_hash, acquisition_id, enrollment_kind, enrollment_ref, placement, outcome, created_at)
		VALUES (?, ?, ?, ?, ?, ?, 'ready', ?)`), e.ID, e.ClipHash, e.AcquisitionID,
		string(e.Enrollment.Kind), e.Enrollment.Reference, string(e.Placement), epoch(e.CreatedAt)); err != nil {
		return fmt.Errorf("record filler ready event %s: %w", e.ID, err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit filler ready %s: %w", e.ClipHash, err)
	}
	return nil
}
