package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

type InteractiveOperationKind string

const (
	InteractiveOperationFillerSplit InteractiveOperationKind = "filler_split"
	InteractiveOperationLLMPull     InteractiveOperationKind = "llm_pull"
)

type InteractiveOperationStatus string

const (
	InteractiveOperationQueued  InteractiveOperationStatus = "queued"
	InteractiveOperationRunning InteractiveOperationStatus = "running"
	InteractiveOperationSuccess InteractiveOperationStatus = "success"
	InteractiveOperationError   InteractiveOperationStatus = "error"
)

// InteractiveOperation is the durable reconnect snapshot for request-launched work owned by one
// application generation. It is deliberately not a queue: there are no claims, retries, or due time.
type InteractiveOperation struct {
	ID          string
	Kind        InteractiveOperationKind
	Subject     string
	Status      InteractiveOperationStatus
	Percent     int
	Completed   int64
	Total       int64
	ResultID    string
	Error       string
	StartedAt   time.Time
	CompletedAt time.Time
	UpdatedAt   time.Time
}

func (s *sqlStore) UpsertInteractiveOperation(ctx context.Context, operation InteractiveOperation) error {
	_, err := s.db.ExecContext(ctx, s.ph(`INSERT INTO interactive_operations
		(id, kind, subject, status, percent, completed, total, result_id, error,
		 started_at, completed_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
		 kind=excluded.kind, subject=excluded.subject, status=excluded.status,
		 percent=excluded.percent, completed=excluded.completed, total=excluded.total,
		 result_id=excluded.result_id, error=excluded.error, started_at=excluded.started_at,
		 completed_at=excluded.completed_at, updated_at=excluded.updated_at`),
		operation.ID, string(operation.Kind), operation.Subject, string(operation.Status),
		operation.Percent, operation.Completed, operation.Total, operation.ResultID, operation.Error,
		epoch(operation.StartedAt), epoch(operation.CompletedAt), epoch(operation.UpdatedAt))
	if err != nil {
		return fmt.Errorf("upsert interactive operation %s: %w", operation.ID, err)
	}
	return nil
}

func (s *sqlStore) GetInteractiveOperation(ctx context.Context, id string) (InteractiveOperation, error) {
	var operation InteractiveOperation
	var kind, status string
	var startedAt, completedAt, updatedAt int64
	err := s.db.QueryRowContext(ctx, s.ph(`SELECT id, kind, subject, status, percent, completed,
		total, result_id, error, started_at, completed_at, updated_at
		FROM interactive_operations WHERE id = ?`), id).Scan(
		&operation.ID, &kind, &operation.Subject, &status, &operation.Percent,
		&operation.Completed, &operation.Total, &operation.ResultID, &operation.Error,
		&startedAt, &completedAt, &updatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return InteractiveOperation{}, ErrNotFound
	}
	if err != nil {
		return InteractiveOperation{}, fmt.Errorf("get interactive operation %s: %w", id, err)
	}
	operation.Kind = InteractiveOperationKind(kind)
	operation.Status = InteractiveOperationStatus(status)
	operation.StartedAt = fromEpoch(startedAt)
	operation.CompletedAt = fromEpoch(completedAt)
	operation.UpdatedAt = fromEpoch(updatedAt)
	return operation, nil
}

func (s *sqlStore) RecoverInterruptedInteractiveOperations(ctx context.Context, at time.Time) (int, error) {
	res, err := s.db.ExecContext(ctx, s.ph(`UPDATE interactive_operations
		SET status = ?, error = ?, completed_at = ?, updated_at = ?
		WHERE status IN (?, ?)`),
		string(InteractiveOperationError), "application restarted before the operation completed",
		epoch(at), epoch(at), string(InteractiveOperationQueued), string(InteractiveOperationRunning))
	if err != nil {
		return 0, fmt.Errorf("recover interactive operations: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("count interrupted interactive operations: %w", err)
	}
	return int(n), nil
}
