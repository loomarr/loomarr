package testkit

import (
	"context"
	"fmt"
	"path/filepath"
	"sync"
	"testing"

	"github.com/mantonx/loomarr/internal/store"
)

// InvalidationListenStep scripts one subscription attempt for InvalidationListener.
// Started is closed when subscribed; BeforeReady and AfterReady gate the corresponding
// phases. WaitForCancellation keeps a successful subscription alive until its context ends.
type InvalidationListenStep struct {
	Started             chan struct{}
	BeforeReady         <-chan struct{}
	AfterReady          <-chan struct{}
	Err                 error
	WaitForCancellation bool
}

// InvalidationListener is the shared scripted double for store invalidation subscriptions.
// It records calls by consuming one step per subscription attempt.
type InvalidationListener struct {
	mu    sync.Mutex
	steps []InvalidationListenStep
	calls int
}

// NewInvalidationListener constructs a listener with the supplied subscription attempts.
func NewInvalidationListener(steps ...InvalidationListenStep) *InvalidationListener {
	return &InvalidationListener{steps: append([]InvalidationListenStep(nil), steps...)}
}

// Calls reports the number of subscription attempts received.
func (l *InvalidationListener) Calls() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.calls
}

// Listen matches store.ListenInvalidations and executes the next scripted attempt.
func (l *InvalidationListener) Listen(
	ctx context.Context,
	_ store.Store,
	ready func(context.Context) error,
	_ func(context.Context, store.Invalidation) error,
) error {
	l.mu.Lock()
	if l.calls >= len(l.steps) {
		l.mu.Unlock()
		return fmt.Errorf("test invalidation listener: unexpected subscription attempt")
	}
	step := l.steps[l.calls]
	l.calls++
	l.mu.Unlock()

	if step.Started != nil {
		close(step.Started)
	}
	if err := waitInvalidationStep(ctx, step.BeforeReady); err != nil {
		return err
	}
	if ready != nil {
		if err := ready(ctx); err != nil {
			return err
		}
	}
	if err := waitInvalidationStep(ctx, step.AfterReady); err != nil {
		return err
	}
	if step.WaitForCancellation {
		<-ctx.Done()
		return ctx.Err()
	}
	return step.Err
}

func waitInvalidationStep(ctx context.Context, gate <-chan struct{}) error {
	if gate == nil {
		return nil
	}
	select {
	case <-gate:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// SQLiteStore opens a fresh migrated production store for a test. Domain tests use
// this shared adapter when behavior depends on real settings/channel persistence.
func SQLiteStore(t testing.TB) store.Store {
	t.Helper()
	st, err := store.Open(context.Background(), "sqlite://"+filepath.Join(t.TempDir(), "loomarr.db"), true)
	if err != nil {
		t.Fatalf("open SQLite test store: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	return st
}
