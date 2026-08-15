package backendtransition

import (
	"context"
	"errors"
	"testing"

	"github.com/mantonx/loomarr/internal/store"
	"github.com/mantonx/loomarr/internal/testkit"
)

func TestDurableViewObservesExternalCheckpointWrites(t *testing.T) {
	st := testkit.SQLiteStore(t)
	ctx := context.Background()
	state, err := Load(ctx, st, BackendTunarr)
	if err != nil {
		t.Fatal(err)
	}
	view := NewDurableView(st)
	if got, err := view.Snapshot(ctx); err != nil || got.ReconcileBackend() != BackendTunarr {
		t.Fatalf("initial snapshot = (%+v, %v)", got, err)
	}

	state, err = state.MarkPrepared(BackendInternal)
	if err != nil {
		t.Fatal(err)
	}
	if err := Save(ctx, st, state); err != nil {
		t.Fatal(err)
	}
	got, err := view.Snapshot(ctx)
	if err != nil {
		t.Fatal(err)
	}
	assertSnapshot(t, got, BackendTunarr, BackendInternal, true)
	if got.ReconcileBackend() != BackendInternal {
		t.Fatalf("reconcile backend = %q, want internal", got.ReconcileBackend())
	}
}

func TestDurableViewFailsClosedWithoutInitializing(t *testing.T) {
	st := testkit.SQLiteStore(t)
	view := NewDurableView(st)
	if _, err := view.Snapshot(context.Background()); !errors.Is(err, ErrInvalidState) {
		t.Fatalf("missing snapshot error = %v, want ErrInvalidState", err)
	}
	if _, err := st.GetSetting(context.Background(), stateKey); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("missing view initialized checkpoint: %v", err)
	}

	if err := st.SetSetting(context.Background(), stateKey, `{`); err != nil {
		t.Fatal(err)
	}
	if _, err := view.Snapshot(context.Background()); !errors.Is(err, ErrInvalidState) {
		t.Fatalf("corrupt snapshot error = %v, want ErrInvalidState", err)
	}
}
