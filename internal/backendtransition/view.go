package backendtransition

import (
	"context"
	"errors"
	"fmt"

	"github.com/mantonx/loomarr/internal/store"
)

// CheckpointView reads the durable backend-publication checkpoint at one operation boundary.
// Postgres replicas must use this view for correctness-sensitive routing and reconciliation:
// Runtime is deliberately only a process-local mirror and cannot observe another replica's save.
type CheckpointView interface {
	Snapshot(ctx context.Context) (Snapshot, error)
}

// DurableView implements CheckpointView over the settings KV. Reads are lock-free: each checkpoint
// is one atomically committed JSON row, while the advisory lock serializes writers and external
// publisher effects only.
type DurableView struct{ store stateLoader }

// NewDurableView returns a strict durable checkpoint reader. Initialize must have created the row
// before request serving; a missing or corrupt row fails closed rather than inferring desired state.
func NewDurableView(st stateLoader) *DurableView { return &DurableView{store: st} }

// Snapshot reads and validates the existing checkpoint without initializing or repairing it.
func (v *DurableView) Snapshot(ctx context.Context) (Snapshot, error) {
	if v == nil || v.store == nil {
		return Snapshot{}, fmt.Errorf("backend transition: checkpoint view is unavailable")
	}
	raw, err := v.store.GetSetting(ctx, stateKey)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return Snapshot{}, fmt.Errorf("backend transition: checkpoint is missing: %w", ErrInvalidState)
		}
		return Snapshot{}, fmt.Errorf("read backend transition checkpoint: %w", err)
	}
	state, err := decode(raw)
	if err != nil {
		return Snapshot{}, err
	}
	return snapshotFor(state), nil
}

func snapshotFor(state State) Snapshot {
	return Snapshot{
		Applied: state.Applied(), Prepared: state.Prepared(),
		PublishedInternal: state.PublishedInternal(),
	}
}

// ParseSnapshot validates one committed checkpoint payload carried by the Postgres lifecycle
// invalidation stream. Keeping decoding here prevents the cross-replica stop path from growing a
// second interpretation of the system-owned state document.
func ParseSnapshot(raw string) (Snapshot, error) {
	state, err := decode(raw)
	if err != nil {
		return Snapshot{}, err
	}
	return snapshotFor(state), nil
}

// ReconcileBackend returns the durable in-progress target when present, otherwise the published
// backend. Ordinary reconciliation uses this ordering so it cannot undo fleet preparation.
func (s Snapshot) ReconcileBackend() string {
	if s.Prepared != "" {
		return s.Prepared
	}
	return s.Applied
}
