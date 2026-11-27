// Package operationstore keeps interactive operations: the reconnect snapshot of work a request
// launches (a filler split, an LLM model pull), so a page can pick its operation up again.
//
// It is a cluster of the store, built on the storedb kernel and never importing the store: the
// store embeds it, and internal/architecture_test.go enforces the direction.
package operationstore

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/loomarr/loomarr/internal/platform/store/storedb"
)

// Store is the interactive-operation cluster on the storedb kernel.
type Store struct{ h storedb.Handle }

// New builds the interactive-operation cluster on h.
func New(h storedb.Handle) *Store { return &Store{h: h} }

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
	Error       string
	StartedAt   time.Time
	CompletedAt time.Time
	UpdatedAt   time.Time
}

func (s *Store) UpsertInteractiveOperation(ctx context.Context, operation InteractiveOperation) error {
	_, err := s.h.ExecContext(ctx, s.h.Rebind(`INSERT INTO interactive_operations
		(id, kind, subject, status, percent, completed, total, error,
		 started_at, completed_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
		 kind=excluded.kind, subject=excluded.subject, status=excluded.status,
		 percent=excluded.percent, completed=excluded.completed, total=excluded.total,
		 error=excluded.error, started_at=excluded.started_at,
		 completed_at=excluded.completed_at, updated_at=excluded.updated_at`),
		operation.ID, string(operation.Kind), operation.Subject, string(operation.Status),
		operation.Percent, operation.Completed, operation.Total, operation.Error,
		storedb.Epoch(operation.StartedAt), storedb.Epoch(operation.CompletedAt), storedb.Epoch(operation.UpdatedAt))
	if err != nil {
		return fmt.Errorf("upsert interactive operation %s: %w", operation.ID, err)
	}
	return nil
}

func (s *Store) GetInteractiveOperation(ctx context.Context, id string) (InteractiveOperation, error) {
	var operation InteractiveOperation
	var kind, status string
	var startedAt, completedAt, updatedAt int64
	err := s.h.QueryRowContext(ctx, s.h.Rebind(`SELECT id, kind, subject, status, percent, completed,
		total, error, started_at, completed_at, updated_at
		FROM interactive_operations WHERE id = ?`), id).Scan(
		&operation.ID, &kind, &operation.Subject, &status, &operation.Percent,
		&operation.Completed, &operation.Total, &operation.Error,
		&startedAt, &completedAt, &updatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return InteractiveOperation{}, storedb.ErrNotFound
	}
	if err != nil {
		return InteractiveOperation{}, fmt.Errorf("get interactive operation %s: %w", id, err)
	}
	operation.Kind = InteractiveOperationKind(kind)
	operation.Status = InteractiveOperationStatus(status)
	operation.StartedAt = storedb.FromEpoch(startedAt)
	operation.CompletedAt = storedb.FromEpoch(completedAt)
	operation.UpdatedAt = storedb.FromEpoch(updatedAt)
	return operation, nil
}

// PurgeFinishedInteractiveOperations deletes operations that succeeded or failed before `before`.
// A row exists so a page can pick its operation up again after a reconnect; once finished and
// old, nothing will ask for it. Queued and running ones are never purged.
func (s *Store) PurgeFinishedInteractiveOperations(ctx context.Context, before time.Time) (int, error) {
	res, err := s.h.ExecContext(ctx, s.h.Rebind(`DELETE FROM interactive_operations
		WHERE status IN (?, ?) AND completed_at < ?`),
		string(InteractiveOperationSuccess), string(InteractiveOperationError), storedb.Epoch(before))
	if err != nil {
		return 0, fmt.Errorf("purge finished interactive operations: %w", err)
	}
	n, _ := res.RowsAffected()
	return int(n), nil
}

func (s *Store) RecoverInterruptedInteractiveOperations(ctx context.Context, at time.Time) (int, error) {
	res, err := s.h.ExecContext(ctx, s.h.Rebind(`UPDATE interactive_operations
		SET status = ?, error = ?, completed_at = ?, updated_at = ?
		WHERE status IN (?, ?)`),
		string(InteractiveOperationError), "application restarted before the operation completed",
		storedb.Epoch(at), storedb.Epoch(at), string(InteractiveOperationQueued), string(InteractiveOperationRunning))
	if err != nil {
		return 0, fmt.Errorf("recover interactive operations: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("count interrupted interactive operations: %w", err)
	}
	return int(n), nil
}
