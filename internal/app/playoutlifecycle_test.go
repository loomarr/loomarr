package app

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/backendtransition"
	"github.com/loomarr/loomarr/internal/schedule"
	"github.com/loomarr/loomarr/internal/store"
	"github.com/loomarr/loomarr/internal/testkit"
)

func TestPostgresPlayoutLifecycleReconcilesDurableEligibility(t *testing.T) {
	t.Parallel()
	st := testkit.MigratedSQLiteStore(t)
	seedLifecycleChannel(t, st, "inherited", 1, schedule.StatusLive, nil)
	seedLifecycleChannel(t, st, "pinned", 2, schedule.StatusLive,
		&schedule.PlayoutPolicy{Backend: schedule.PlayoutBackendInternal})
	seedLifecycleChannel(t, st, "paused", 3, schedule.StatusPaused, nil)
	origin := &testkit.Playout{}
	lifecycle := &postgresPlayoutLifecycle{
		store: st,
		checkpoint: testkit.Snapshotter[backendtransition.Snapshot]{Result: backendtransition.Snapshot{
			Applied: schedule.PlayoutBackendTunarr, PublishedInternal: false,
		}},
		origin: origin,
		gate:   &playoutAdmissionGate{},
	}

	if err := lifecycle.reconcileAll(context.Background()); err != nil {
		t.Fatal(err)
	}
	if !origin.Stopped("inherited") || !origin.Stopped("paused") {
		t.Fatalf("durably off-air channels not stopped: %v", origin.StoppedChannels())
	}
	if origin.Stopped("pinned") {
		t.Fatalf("explicit internal pin stopped during global Tunarr state: %v", origin.StoppedChannels())
	}
}

func TestPostgresPlayoutLifecyclePreservesCommittedPausePayload(t *testing.T) {
	t.Parallel()
	wantReadErr := errors.New("checkpoint should not be read for a committed pause")
	origin := &testkit.Playout{}
	lifecycle := &postgresPlayoutLifecycle{
		checkpoint: testkit.Snapshotter[backendtransition.Snapshot]{Err: wantReadErr}, origin: origin,
	}
	if err := lifecycle.apply(context.Background(), store.Invalidation{
		Kind: store.InvalidationChannel, ChannelID: "paused", Status: schedule.StatusPaused,
	}); err != nil {
		t.Fatalf("apply committed pause: %v", err)
	}
	if !origin.Stopped("paused") {
		t.Fatal("committed pause did not stop the channel")
	}
}

func TestPostgresPlayoutLifecycleRestartsOnlyChangedInternalSchedule(t *testing.T) {
	t.Parallel()
	origin := &testkit.Playout{}
	lifecycle := &postgresPlayoutLifecycle{
		checkpoint: testkit.Snapshotter[backendtransition.Snapshot]{Result: backendtransition.Snapshot{
			Applied: schedule.PlayoutBackendInternal, PublishedInternal: true,
		}},
		origin: origin,
	}
	lifecycle.rememberSchedule("simpsons", "old-cycle")

	for _, version := range []string{"old-cycle", "new-cycle", "new-cycle"} {
		if err := lifecycle.apply(context.Background(), store.Invalidation{
			Kind: store.InvalidationChannel, ChannelID: "simpsons", Status: schedule.StatusLive,
			ScheduleVersion: version,
		}); err != nil {
			t.Fatal(err)
		}
	}
	if got := origin.StoppedChannels(); len(got) != 1 || got[0] != "simpsons" {
		t.Fatalf("schedule cutover stops = %v, want exactly one for the changed version", got)
	}
}

func TestPostgresPlayoutLifecycleBackendCutoverKeepsExplicitInternalPins(t *testing.T) {
	t.Parallel()
	st := testkit.MigratedSQLiteStore(t)
	seedLifecycleChannel(t, st, "inherited", 1, schedule.StatusLive, nil)
	seedLifecycleChannel(t, st, "pinned", 2, schedule.StatusLive,
		&schedule.PlayoutPolicy{Backend: schedule.PlayoutBackendInternal})
	origin := &testkit.Playout{}
	lifecycle := &postgresPlayoutLifecycle{store: st, origin: origin}
	if err := lifecycle.apply(context.Background(), store.Invalidation{
		Kind: store.InvalidationSetting, Key: "system.unrelated", Value: `{`,
	}); err != nil {
		t.Fatalf("ignore unrelated setting invalidation: %v", err)
	}
	if err := lifecycle.apply(context.Background(), store.Invalidation{
		Kind:  store.InvalidationSetting,
		Key:   backendtransition.CheckpointSettingKey,
		Value: `{"version":1,"applied":"tunarr","prepared":""}`,
	}); err != nil {
		t.Fatal(err)
	}
	if !origin.Stopped("inherited") || origin.Stopped("pinned") {
		t.Fatalf("cutover stops = %v, want inherited only", origin.StoppedChannels())
	}
}

func TestPostgresPlayoutLifecycleListenerLossClosesUntilDurableCatchUp(t *testing.T) {
	t.Parallel()
	st := testkit.MigratedSQLiteStore(t)
	origin := &testkit.Playout{}
	gate := &playoutAdmissionGate{}
	failFirst := make(chan struct{})
	secondSubscribed := make(chan struct{})
	allowSecondCatchUp := make(chan struct{})
	listener := testkit.NewInvalidationListener(
		testkit.InvalidationListenStep{AfterReady: failFirst, Err: errors.New("listener lost")},
		testkit.InvalidationListenStep{
			Started: secondSubscribed, BeforeReady: allowSecondCatchUp, WaitForCancellation: true,
		},
	)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	lifecycle := &postgresPlayoutLifecycle{
		store: st, checkpoint: testkit.Snapshotter[backendtransition.Snapshot]{Result: backendtransition.Snapshot{
			Applied: schedule.PlayoutBackendInternal, PublishedInternal: true,
		}}, origin: origin, gate: gate,
		listen: listener.Listen,
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
	if got := listener.Calls(); got != 2 {
		t.Fatalf("listener subscription attempts = %d, want 2", got)
	}
	if gate.Available() {
		t.Fatal("admission reopened before reconnect catch-up")
	}
	stopAllCalls := origin.StopAllCalls()
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
