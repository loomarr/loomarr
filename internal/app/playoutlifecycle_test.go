package app

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/mantonx/loomarr/internal/backendtransition"
	"github.com/mantonx/loomarr/internal/schedule"
	"github.com/mantonx/loomarr/internal/store"
	"github.com/mantonx/loomarr/internal/testkit"
)

type lifecycleCheckpointStub struct {
	snapshot backendtransition.Snapshot
	err      error
}

func (s lifecycleCheckpointStub) Snapshot(context.Context) (backendtransition.Snapshot, error) {
	return s.snapshot, s.err
}

type lifecycleOriginStub struct {
	mu      sync.Mutex
	stopped []string
	all     int
}

func (s *lifecycleOriginStub) StopChannel(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.stopped = append(s.stopped, id)
}

func (s *lifecycleOriginStub) StopAll() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.all++
}

func (s *lifecycleOriginStub) didStop(id string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, stopped := range s.stopped {
		if stopped == id {
			return true
		}
	}
	return false
}

func TestPostgresPlayoutLifecycleReconcilesDurableEligibility(t *testing.T) {
	t.Parallel()
	st := testkit.SQLiteStore(t)
	seedLifecycleChannel(t, st, "inherited", 1, schedule.StatusLive, nil)
	seedLifecycleChannel(t, st, "pinned", 2, schedule.StatusLive,
		&schedule.PlayoutPolicy{Backend: schedule.PlayoutBackendInternal})
	seedLifecycleChannel(t, st, "paused", 3, schedule.StatusPaused, nil)
	origin := &lifecycleOriginStub{}
	lifecycle := &postgresPlayoutLifecycle{
		store: st,
		checkpoint: lifecycleCheckpointStub{snapshot: backendtransition.Snapshot{
			Applied: schedule.PlayoutBackendTunarr, PublishedInternal: false,
		}},
		origin: origin,
		gate:   &playoutAdmissionGate{},
	}

	if err := lifecycle.reconcileAll(context.Background()); err != nil {
		t.Fatal(err)
	}
	if !origin.didStop("inherited") || !origin.didStop("paused") {
		t.Fatalf("durably off-air channels not stopped: %v", origin.stopped)
	}
	if origin.didStop("pinned") {
		t.Fatalf("explicit internal pin stopped during global Tunarr state: %v", origin.stopped)
	}
}

func TestPostgresPlayoutLifecyclePreservesCommittedPausePayload(t *testing.T) {
	t.Parallel()
	wantReadErr := errors.New("checkpoint should not be read for a committed pause")
	origin := &lifecycleOriginStub{}
	lifecycle := &postgresPlayoutLifecycle{
		checkpoint: lifecycleCheckpointStub{err: wantReadErr}, origin: origin,
	}
	if err := lifecycle.apply(context.Background(), store.Invalidation{
		Kind: store.InvalidationChannel, ChannelID: "paused", Status: schedule.StatusPaused,
	}); err != nil {
		t.Fatalf("apply committed pause: %v", err)
	}
	if !origin.didStop("paused") {
		t.Fatal("committed pause did not stop the channel")
	}
}

func TestPostgresPlayoutLifecycleBackendCutoverKeepsExplicitInternalPins(t *testing.T) {
	t.Parallel()
	st := testkit.SQLiteStore(t)
	seedLifecycleChannel(t, st, "inherited", 1, schedule.StatusLive, nil)
	seedLifecycleChannel(t, st, "pinned", 2, schedule.StatusLive,
		&schedule.PlayoutPolicy{Backend: schedule.PlayoutBackendInternal})
	origin := &lifecycleOriginStub{}
	lifecycle := &postgresPlayoutLifecycle{store: st, origin: origin}
	if err := lifecycle.apply(context.Background(), store.Invalidation{
		Kind:  store.InvalidationBackend,
		Value: `{"version":1,"applied":"tunarr","prepared":""}`,
	}); err != nil {
		t.Fatal(err)
	}
	if !origin.didStop("inherited") || origin.didStop("pinned") {
		t.Fatalf("cutover stops = %v, want inherited only", origin.stopped)
	}
}

func TestPostgresPlayoutLifecycleListenerLossClosesUntilDurableCatchUp(t *testing.T) {
	t.Parallel()
	st := testkit.SQLiteStore(t)
	origin := &lifecycleOriginStub{}
	gate := &playoutAdmissionGate{}
	failFirst := make(chan struct{})
	secondSubscribed := make(chan struct{})
	allowSecondCatchUp := make(chan struct{})
	var calls atomic.Int32
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	lifecycle := &postgresPlayoutLifecycle{
		store: st, checkpoint: lifecycleCheckpointStub{snapshot: backendtransition.Snapshot{
			Applied: schedule.PlayoutBackendInternal, PublishedInternal: true,
		}}, origin: origin, gate: gate,
		listen: func(
			ctx context.Context,
			_ store.Store,
			ready func(context.Context) error,
			_ func(context.Context, store.Invalidation) error,
		) error {
			if calls.Add(1) == 1 {
				if err := ready(ctx); err != nil {
					return err
				}
				<-failFirst
				return errors.New("listener lost")
			}
			close(secondSubscribed)
			select {
			case <-allowSecondCatchUp:
				if err := ready(ctx); err != nil {
					return err
				}
			case <-ctx.Done():
				return ctx.Err()
			}
			<-ctx.Done()
			return ctx.Err()
		},
	}
	if err := lifecycle.Start(ctx); err != nil {
		t.Fatal(err)
	}
	if !gate.Available() {
		t.Fatal("admission did not open after initial durable catch-up")
	}
	close(failFirst)
	select {
	case <-secondSubscribed:
	case <-time.After(2 * time.Second):
		t.Fatal("listener was not re-subscribed")
	}
	if gate.Available() {
		t.Fatal("admission reopened before reconnect catch-up")
	}
	origin.mu.Lock()
	stopAllCalls := origin.all
	origin.mu.Unlock()
	if stopAllCalls < 2 {
		t.Fatalf("StopAll calls = %d, want initial close plus listener-loss close", stopAllCalls)
	}
	close(allowSecondCatchUp)
	deadline := time.Now().Add(2 * time.Second)
	for !gate.Available() && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if !gate.Available() {
		t.Fatal("admission did not reopen after reconnect durable catch-up")
	}
}

func seedLifecycleChannel(
	t *testing.T,
	st store.Store,
	id string,
	number int,
	status schedule.ChannelStatus,
	playout *schedule.PlayoutPolicy,
) {
	t.Helper()
	_, err := st.SaveChannel(context.Background(), store.Channel{Channel: schedule.Channel{
		ID: id, Name: id, Number: number, Strategy: schedule.Sequential, Status: status,
	}, Policy: schedule.ChannelPolicy{OperatorPolicy: schedule.OperatorPolicy{Playout: playout}}})
	if err != nil {
		t.Fatal(err)
	}
}
