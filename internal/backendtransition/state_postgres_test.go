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

func TestPostgresDurableViewObservesAnotherReplicaCheckpoint(t *testing.T) {
	stores := testkit.PostgresStores(t, 2)
	ctx := context.Background()
	state, err := Load(ctx, stores[0], BackendTunarr)
	if err != nil {
		t.Fatal(err)
	}
	view := NewDurableView(stores[1])
	state, err = state.MarkPrepared(BackendInternal)
	if err != nil {
		t.Fatal(err)
	}
	if err := Save(ctx, stores[0], state); err != nil {
		t.Fatal(err)
	}
	snapshot, err := view.Snapshot(ctx)
	if err != nil {
		t.Fatal(err)
	}
	assertSnapshot(t, snapshot, BackendTunarr, BackendInternal, true)
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

		// Simulate a PATCH handled by another replica. The durable write itself must wait
		// behind the older transition, not merely its follow-on Apply; otherwise this newer
		// desired value could commit while the older owner later publishes its stale target.
		mutationEntered := make(chan struct{})
		var mutationErr error
		var mutationResults []settings.PatchResult
		queuedDone := make(chan error, 1)
		go func() {
			queuedDone <- queued.MutateAndApplyCurrent(ctx, queuedSettings.Refresh, func(mutationCtx context.Context) bool {
				close(mutationEntered)
				mutationResults, mutationErr = queuedSettings.Patch(mutationCtx,
					postgresSettingsWriter{Store: stores[1]},
					map[string]string{"playout.backend": BackendTunarr}, "test")
				return mutationErr != nil || len(mutationResults) == 1 &&
					mutationResults[0].Status == settings.PatchSaved
			}, desiredSnapshot(queuedSettings))
		}()
		select {
		case err := <-queuedDone:
			t.Fatalf("queued opposing transition completed before publisher release: %v", err)
		case <-mutationEntered:
			t.Fatal("newer desired setting committed before older transition released its lock")
		case <-time.After(150 * time.Millisecond):
		}

		probe.ReleasePublisher()
		if err := <-firstDone; err != nil {
			t.Fatalf("first transition: %v", err)
		}
		if err := <-queuedDone; err != nil {
			t.Fatalf("queued transition: %v", err)
		}
		if mutationErr != nil {
			t.Fatalf("save newer desired backend: %v", mutationErr)
		}
		if len(mutationResults) != 1 || mutationResults[0].Status != settings.PatchSaved {
			t.Fatalf("save newer desired backend = %+v, want saved", mutationResults)
		}
		select {
		case <-mutationEntered:
		default:
			t.Fatal("queued desired mutation never entered after lock release")
		}
		if got := probe.MaxPublisherConcurrency(); got != 1 {
			t.Fatalf("max publisher concurrency = %d, want 1", got)
		}
		assertPostgresTransition(t, stores[0], BackendTunarr)
		if got := queuedSettings.String("playout.backend"); got != BackendTunarr {
			t.Fatalf("queued replica refreshed desired = %q, want %q", got, BackendTunarr)
		}
	})

	t.Run("queued patch observes environment ownership handoff", func(t *testing.T) {
		t.Setenv("PLAYOUT_BACKEND", BackendInternal)
		resetPostgresTransition(t, stores[0], BackendInternal, BackendInternal)
		if err := stores[0].SetSettingEnvOverride(ctx, "playout.backend", false, "", "test"); err != nil {
			t.Fatalf("reset environment ownership: %v", err)
		}

		firstSettings := postgresSettingsService(t, stores[0])
		queuedSettings := postgresSettingsService(t, stores[1])
		if got := queuedSettings.Provenance("playout.backend"); got != settings.ProvenanceEnv {
			t.Fatalf("queued replica initial provenance = %q, want env", got)
		}

		probe := testkit.NewBackendTransitionProbe()
		first := NewController(stores[0], probe, probe, nil)
		queued := NewController(stores[1], probe, probe, nil)
		firstWriter := postgresSettingsWriter{Store: stores[0]}
		queuedWriter := postgresSettingsWriter{Store: stores[1]}

		var handoffStatus settings.EnvOverrideStatus
		var handoffErr error
		firstDone := make(chan error, 1)
		go func() {
			firstDone <- first.MutateAndApplyCurrent(ctx, firstSettings.Refresh,
				func(mutationCtx context.Context) bool {
					handoffStatus, handoffErr = firstSettings.SetEnvOverride(
						mutationCtx, firstWriter, "playout.backend", true, "test",
					)
					return handoffStatus == settings.EnvOverrideApplied || handoffErr != nil
				}, desiredSnapshot(firstSettings))
		}()
		select {
		case <-probe.PublisherEntered():
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		}

		mutationEntered := make(chan struct{})
		var patchResults []settings.PatchResult
		var patchErr error
		queuedDone := make(chan error, 1)
		go func() {
			queuedDone <- queued.MutateAndApplyCurrent(ctx, queuedSettings.Refresh,
				func(mutationCtx context.Context) bool {
					close(mutationEntered)
					patchResults, patchErr = queuedSettings.Patch(mutationCtx, queuedWriter,
						map[string]string{"playout.backend": BackendTunarr}, "test")
					return patchErr != nil || len(patchResults) == 1 && patchResults[0].Status == settings.PatchSaved
				}, desiredSnapshot(queuedSettings))
		}()
		select {
		case <-mutationEntered:
			t.Fatal("queued patch mutated before environment ownership handoff released its lock")
		case err := <-queuedDone:
			t.Fatalf("queued patch completed before environment ownership handoff: %v", err)
		case <-time.After(150 * time.Millisecond):
		}

		probe.ReleasePublisher()
		if err := <-firstDone; err != nil {
			t.Fatalf("environment ownership handoff: %v", err)
		}
		if handoffErr != nil || handoffStatus != settings.EnvOverrideApplied {
			t.Fatalf("environment ownership handoff = %q, %v", handoffStatus, handoffErr)
		}
		if err := <-queuedDone; err != nil {
			t.Fatalf("queued patch transition: %v", err)
		}
		if patchErr != nil {
			t.Fatalf("queued patch: %v", patchErr)
		}
		if len(patchResults) != 1 || patchResults[0].Status != settings.PatchSaved {
			t.Fatalf("queued patch results = %+v, want saved rather than pinned", patchResults)
		}
		if got := queuedSettings.Provenance("playout.backend"); got != settings.ProvenanceDB {
			t.Fatalf("queued replica refreshed provenance = %q, want db", got)
		}
		assertPostgresTransition(t, stores[0], BackendTunarr)
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

func desiredSnapshot(svc *settings.Service) func(context.Context) (string, error) {
	return func(context.Context) (string, error) {
		return svc.String("playout.backend"), nil
	}
}

type postgresSettingsWriter struct{ store.Store }

func (w postgresSettingsWriter) Apply(ctx context.Context, batch settings.PersistenceBatch) error {
	storeBatch := store.SettingBatch{
		Upserts:   make([]store.SettingMutation, 0, len(batch.Upserts)),
		Deletes:   append([]string(nil), batch.Deletes...),
		UpdatedBy: batch.UpdatedBy,
		UpdatedAt: batch.UpdatedAt,
	}
	for _, row := range batch.Upserts {
		storeBatch.Upserts = append(storeBatch.Upserts, store.SettingMutation{
			Key: row.Key, Value: row.Value,
		})
	}
	return w.ApplySettingBatch(ctx, storeBatch)
}

func (w postgresSettingsWriter) SetEnvOverride(
	ctx context.Context, key string, on bool, seed, by string,
) error {
	return w.SetSettingEnvOverride(ctx, key, on, seed, by)
}
