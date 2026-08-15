//go:build integration

package backendtransition

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/mantonx/loomarr/internal/settings"
	"github.com/mantonx/loomarr/internal/store"
	"github.com/mantonx/loomarr/internal/testkit"
)

func TestPostgresStateRoundTrip(t *testing.T) {
	st := testkit.PostgresStore(t)
	ctx := context.Background()
	state, err := Load(ctx, st, BackendInternal)
	if err != nil {
		t.Fatal(err)
	}
	state, err = state.MarkPrepared(BackendTunarr)
	if err != nil {
		t.Fatal(err)
	}
	if err := Save(ctx, st, state); err != nil {
		t.Fatal(err)
	}
	reloaded, err := Load(ctx, st, BackendInternal)
	if err != nil {
		t.Fatal(err)
	}
	assertState(t, reloaded, BackendInternal, BackendTunarr, true)
}

func TestPostgresControllersSerializePublisherAndRefreshDesiredInsideLock(t *testing.T) {
	t.Setenv("PLAYOUT_BACKEND", "")
	stores := testkit.PostgresStores(t, 2)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	t.Run("opposing target queued behind transition observes newer durable desired", func(t *testing.T) {
		resetPostgresTransition(t, stores[0], BackendTunarr, BackendInternal)
		firstSettings := postgresSettingsService(t, stores[0])
		queuedSettings := postgresSettingsService(t, stores[1])
		probe := testkit.NewBackendTransitionProbe()
		first := NewController(stores[0], probe, probe, nil)
		queued := NewController(stores[1], probe, probe, nil)

		firstDone := make(chan error, 1)
		go func() { firstDone <- first.ApplyCurrent(ctx, refreshedDesired(firstSettings)) }()
		select {
		case <-probe.PublisherEntered():
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		}

		// Simulate a PATCH handled by the other replica while the first transition
		// owns the advisory lock. Its local snapshot is intentionally stale until
		// the queued controller enters the lock and refreshes it.
		if err := stores[1].SetSetting(ctx, "playout.backend", BackendTunarr); err != nil {
			t.Fatal(err)
		}
		if got := queuedSettings.String("playout.backend"); got != BackendInternal {
			t.Fatalf("queued replica snapshot = %q, want deliberately stale %q", got, BackendInternal)
		}

		queuedDone := make(chan error, 1)
		go func() { queuedDone <- queued.ApplyCurrent(ctx, refreshedDesired(queuedSettings)) }()
		select {
		case err := <-queuedDone:
			t.Fatalf("queued opposing transition completed before publisher release: %v", err)
		case <-time.After(150 * time.Millisecond):
		}

		probe.ReleasePublisher()
		if err := <-firstDone; err != nil {
			t.Fatalf("first transition: %v", err)
		}
		if err := <-queuedDone; err != nil {
			t.Fatalf("queued transition: %v", err)
		}
		if got := probe.MaxPublisherConcurrency(); got != 1 {
			t.Fatalf("max publisher concurrency = %d, want 1", got)
		}
		assertPostgresTransition(t, stores[0], BackendTunarr)
		if got := queuedSettings.String("playout.backend"); got != BackendTunarr {
			t.Fatalf("queued replica refreshed desired = %q, want %q", got, BackendTunarr)
		}
	})

	t.Run("same target controllers serialize repair phases", func(t *testing.T) {
		resetPostgresTransition(t, stores[0], BackendTunarr, BackendInternal)
		firstSettings := postgresSettingsService(t, stores[0])
		queuedSettings := postgresSettingsService(t, stores[1])
		probe := testkit.NewBackendTransitionProbe()
		first := NewController(stores[0], probe, probe, nil)
		queued := NewController(stores[1], probe, probe, nil)

		firstDone := make(chan error, 1)
		queuedDone := make(chan error, 1)
		go func() { firstDone <- first.ApplyCurrent(ctx, refreshedDesired(firstSettings)) }()
		select {
		case <-probe.PublisherEntered():
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		}
		go func() { queuedDone <- queued.ApplyCurrent(ctx, refreshedDesired(queuedSettings)) }()
		select {
		case err := <-queuedDone:
			t.Fatalf("queued same-target transition completed before publisher release: %v", err)
		case <-time.After(150 * time.Millisecond):
		}

		probe.ReleasePublisher()
		if err := <-firstDone; err != nil {
			t.Fatalf("first transition: %v", err)
		}
		if err := <-queuedDone; err != nil {
			t.Fatalf("queued repair: %v", err)
		}
		if got := probe.MaxPublisherConcurrency(); got != 1 {
			t.Fatalf("max publisher concurrency = %d, want 1", got)
		}
		assertPostgresTransition(t, stores[0], BackendInternal)
	})

	t.Run("waiting advisory-lock acquisition honors context cancellation", func(t *testing.T) {
		entered := make(chan struct{})
		release := make(chan struct{})
		holderDone := make(chan error, 1)
		go func() {
			holderDone <- stores[0].WithSettingLock(context.Background(), "cancel-probe",
				func(context.Context) error {
					close(entered)
					<-release
					return nil
				})
		}()
		<-entered

		waitCtx, waitCancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
		defer waitCancel()
		err := stores[1].WithSettingLock(waitCtx, "cancel-probe", func(context.Context) error {
			t.Fatal("cancelled waiter entered protected callback")
			return nil
		})
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("cancelled lock acquisition = %v, want context deadline exceeded", err)
		}

		close(release)
		if err := <-holderDone; err != nil {
			t.Fatalf("release held advisory lock: %v", err)
		}
		if err := stores[1].WithSettingLock(context.Background(), "cancel-probe",
			func(context.Context) error { return nil }); err != nil {
			t.Fatalf("acquire after cancelled waiter: %v", err)
		}
	})
}

func resetPostgresTransition(t testing.TB, st store.Store, applied, desired string) {
	t.Helper()
	ctx := context.Background()
	if err := Save(ctx, st, State{applied: applied}); err != nil {
		t.Fatalf("reset transition state: %v", err)
	}
	if err := st.SetSetting(ctx, "playout.backend", desired); err != nil {
		t.Fatalf("set desired backend: %v", err)
	}
}

func assertPostgresTransition(t testing.TB, st store.Store, applied string) {
	t.Helper()
	state, err := Load(context.Background(), st, applied)
	if err != nil {
		t.Fatal(err)
	}
	if state.Applied() != applied || state.Prepared() != "" {
		t.Fatalf("transition state = %q/%q, want %q/empty", state.Applied(), state.Prepared(), applied)
	}
}

func postgresSettingsService(t testing.TB, st store.Store) *settings.Service {
	t.Helper()
	loader := settings.StoreLoader{List: func(ctx context.Context) ([]settings.SettingRow, error) {
		rows, err := st.ListSettings(ctx)
		if err != nil {
			return nil, err
		}
		out := make([]settings.SettingRow, len(rows))
		for i, row := range rows {
			out[i] = settings.SettingRow{
				Key: row.Key, Value: row.Value, UpdatedBy: row.UpdatedBy, EnvOverride: row.EnvOverride,
			}
		}
		return out, nil
	}}
	svc, err := settings.New(context.Background(), settings.NewRegistry(), loader,
		slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatalf("new settings service: %v", err)
	}
	return svc
}

func refreshedDesired(svc *settings.Service) func(context.Context) (string, error) {
	return func(ctx context.Context) (string, error) {
		if err := svc.Refresh(ctx); err != nil {
			return "", err
		}
		return svc.String("playout.backend"), nil
	}
}
