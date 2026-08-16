package backendtransition

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"testing"
	"time"

	"github.com/mantonx/loomarr/internal/store"
	"github.com/mantonx/loomarr/internal/testkit"
)

func TestControllerAppliesInDurableOrderAndPublishesRuntimeAfterSave(t *testing.T) {
	base := initializedStore(t, BackendTunarr)
	probe := testkit.NewBackendTransitionPhaseProbe(BackendTunarr)
	st := transitionFaultStore(base, probe, nil)
	controller := NewController(st, probe, probe, probe)
	var fleetSnapshot Snapshot
	probe.ObserveFleet(func() { fleetSnapshot = controller.Runtime().Snapshot() })

	var snapshots []Snapshot
	st.AfterSave = func() { snapshots = append(snapshots, controller.Runtime().Snapshot()) }
	if err := controller.Apply(context.Background(), BackendInternal); err != nil {
		t.Fatal(err)
	}

	wantOrder := []string{
		"save:tunarr/internal",
		"fleet:internal",
		"publisher.prepare:internal",
		"publisher.refresh:internal",
		"cutover:tunarr->internal",
		"save:internal/",
		"publisher.retire:internal",
	}
	if got := probe.Phases(); !slices.Equal(got, wantOrder) {
		t.Fatalf("phase order = %v, want %v", got, wantOrder)
	}
	if len(snapshots) != 2 {
		t.Fatalf("runtime observations at durable writes = %v, want two", snapshots)
	}
	assertSnapshot(t, snapshots[0], BackendTunarr, "", false)
	assertSnapshot(t, snapshots[1], BackendTunarr, BackendInternal, true)
	assertSnapshot(t, fleetSnapshot, BackendTunarr, BackendInternal, true)
	assertSnapshot(t, controller.Runtime().Snapshot(), BackendInternal, "", true)
	assertDurableState(t, base, BackendInternal, "")
}

func TestControllerInitializePublishesDurableStateWithoutSideEffects(t *testing.T) {
	st := testkit.SQLiteStore(t)
	probe := testkit.NewBackendTransitionPhaseProbe(BackendTunarr)
	controller := NewController(st, probe, probe, probe)

	if err := controller.Initialize(context.Background(), func(context.Context) (string, error) {
		return BackendInternal, nil
	}); err != nil {
		t.Fatal(err)
	}
	assertDurableState(t, st, BackendInternal, "")
	assertSnapshot(t, controller.Runtime().Snapshot(), BackendInternal, "", true)
	if got := probe.Phases(); len(got) != 0 {
		t.Fatalf("Initialize invoked transition adapters: %v", got)
	}

	if err := controller.Initialize(context.Background(), func(context.Context) (string, error) {
		return "external", nil
	}); !errors.Is(err, ErrInvalidState) {
		t.Fatalf("invalid desired error = %v, want ErrInvalidState from Load", err)
	}
	assertSnapshot(t, controller.Runtime().Snapshot(), BackendInternal, "", true)
	if got := probe.Phases(); len(got) != 0 {
		t.Fatalf("invalid Initialize invoked transition adapters: %v", got)
	}
}

func TestControllerReconnectRepairsAppliedBackendWithoutPublishingPrepared(t *testing.T) {
	st := initializedStore(t, BackendTunarr)
	state, err := Load(context.Background(), st, BackendInternal)
	if err != nil {
		t.Fatal(err)
	}
	state, err = state.MarkPrepared(BackendInternal)
	if err != nil {
		t.Fatal(err)
	}
	if err := Save(context.Background(), st, state); err != nil {
		t.Fatal(err)
	}
	probe := testkit.NewBackendTransitionPhaseProbe(BackendTunarr)
	controller := NewController(st, probe, probe, nil)

	reset, err := controller.ReconnectCurrent(context.Background(), func(context.Context) (string, error) {
		return BackendInternal, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if reset != 1 {
		t.Fatalf("tuners reset = %d, want 1", reset)
	}
	if got := probe.Phases(); !slices.Equal(got, []string{"publisher.reconnect:tunarr"}) {
		t.Fatalf("reconnect phases = %v, want applied Tunarr only", got)
	}
	assertDurableState(t, st, BackendTunarr, BackendInternal)
}

func TestControllerReconnectSerializesWithAnotherControllerCutover(t *testing.T) {
	st := initializedStore(t, BackendTunarr)
	probe := testkit.NewBackendTransitionProbe()
	cutover := NewController(st, probe, probe, nil)
	repair := NewController(st, probe, probe, nil)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	cutoverDone := make(chan error, 1)
	go func() { cutoverDone <- cutover.Apply(ctx, BackendInternal) }()
	select {
	case <-probe.PublisherEntered():
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}

	repairDone := make(chan error, 1)
	go func() {
		_, err := repair.ReconnectCurrent(ctx, func(context.Context) (string, error) {
			return BackendInternal, nil
		})
		repairDone <- err
	}()
	select {
	case err := <-repairDone:
		t.Fatalf("reconnect completed inside another controller's cutover: %v", err)
	case <-time.After(100 * time.Millisecond):
	}

	probe.ReleasePublisher()
	if err := <-cutoverDone; err != nil {
		t.Fatal(err)
	}
	if err := <-repairDone; err != nil {
		t.Fatal(err)
	}
	if got := probe.MaxPublisherConcurrency(); got != 1 {
		t.Fatalf("publisher concurrency = %d, want 1", got)
	}
	targets := probe.PublisherTargets()
	if len(targets) == 0 || targets[len(targets)-1] != BackendInternal {
		t.Fatalf("publisher targets = %v, want reconnect of newly applied internal", targets)
	}
	assertDurableState(t, st, BackendInternal, "")
}

func TestControllerRestartRetriesEveryPhase(t *testing.T) {
	crash := errors.New("simulated process interruption")
	for _, phase := range []string{
		"save-prepared", "fleet", "publisher-prepare", "refresh", "cutover", "save-published", "retire",
	} {
		t.Run(phase, func(t *testing.T) {
			base := initializedStore(t, BackendTunarr)
			failWrites := map[int]error{}
			probe := testkit.NewBackendTransitionPhaseProbe(BackendTunarr)
			st := transitionFaultStore(base, nil, failWrites)
			switch phase {
			case "fleet":
				probe.FailFleetOnce(crash)
			case "save-prepared":
				failWrites[1] = crash
			case "publisher-prepare":
				probe.FailPrepareOnce(crash)
			case "refresh":
				probe.FailRefreshOnce(crash)
			case "cutover":
				probe.FailCutoverOnce(crash)
			case "save-published":
				failWrites[2] = crash
			case "retire":
				probe.FailRetireOnce(crash)
			}

			first := NewController(st, probe, probe, probe)
			if err := first.Apply(context.Background(), BackendInternal); !errors.Is(err, crash) {
				t.Fatalf("first Apply error = %v, want simulated interruption", err)
			}

			wantApplied, wantPrepared := BackendTunarr, ""
			switch phase {
			case "fleet", "publisher-prepare", "refresh", "cutover", "save-published":
				wantPrepared = BackendInternal
			case "retire":
				wantApplied = BackendInternal
			}
			assertDurableState(t, base, wantApplied, wantPrepared)
			assertSnapshot(t, first.Runtime().Snapshot(), wantApplied, wantPrepared,
				wantApplied == BackendInternal || wantPrepared == BackendInternal)

			// A new process has no in-memory phase history. It must resume from the durable row
			// and converge without operator repair.
			restarted := NewController(st, probe, probe, probe)
			if err := restarted.Apply(context.Background(), BackendInternal); err != nil {
				t.Fatalf("Apply after restart: %v", err)
			}
			assertDurableState(t, base, BackendInternal, "")
			assertSnapshot(t, restarted.Runtime().Snapshot(), BackendInternal, "", true)

			wantFleetCalls := 1
			if phase == "fleet" || phase == "publisher-prepare" || phase == "refresh" ||
				phase == "cutover" || phase == "save-published" || phase == "retire" {
				// A durable in-progress target and an applied target awaiting cleanup both
				// replay the fleet barrier before publisher work after restart.
				wantFleetCalls = 2
			}
			if got := probe.FleetCalls(); got != wantFleetCalls {
				t.Fatalf("fleet calls = %d, want %d", got, wantFleetCalls)
			}
		})
	}
}

func TestControllerDesiredReversalReplacesPreparedTargetBeforeRepair(t *testing.T) {
	base := initializedStore(t, BackendTunarr)
	ctx := context.Background()
	state, err := Load(ctx, base, BackendTunarr)
	if err != nil {
		t.Fatal(err)
	}
	state, err = state.MarkPrepared(BackendInternal)
	if err != nil {
		t.Fatal(err)
	}
	if err := Save(ctx, base, state); err != nil {
		t.Fatal(err)
	}

	probe := testkit.NewBackendTransitionPhaseProbe(BackendTunarr, BackendInternal)
	st := transitionFaultStore(base, probe, nil)
	controller := NewController(st, probe, probe, probe)

	if err := controller.Apply(ctx, BackendTunarr); err != nil {
		t.Fatal(err)
	}
	want := []string{
		"save:tunarr/tunarr",
		"fleet:tunarr",
		"publisher.prepare:tunarr",
		"publisher.refresh:tunarr",
		"save:tunarr/",
		"publisher.retire:tunarr",
	}
	if got := probe.Phases(); !slices.Equal(got, want) {
		t.Fatalf("reversal phases = %v, want %v", got, want)
	}
	if probe.FleetCalls() != 1 || probe.CutoverCalls() != 0 {
		t.Fatalf("reversal fleet/cutover calls: fleet=%d cutover=%d, want 1/0",
			probe.FleetCalls(), probe.CutoverCalls())
	}
	assertDurableState(t, base, BackendTunarr, "")
	assertSnapshot(t, controller.Runtime().Snapshot(), BackendTunarr, "", false)
	if probe.IsRegistered(BackendInternal) {
		t.Fatal("cancelled internal registration survived stale retirement")
	}
}

func TestControllerFailedTargetReplacementLeavesPriorTargetDurableAndRetryable(t *testing.T) {
	base := initializedStore(t, BackendTunarr)
	ctx := context.Background()
	state, _ := Load(ctx, base, BackendTunarr)
	state, _ = state.MarkPrepared(BackendInternal)
	if err := Save(ctx, base, state); err != nil {
		t.Fatal(err)
	}

	cancelErr := errors.New("checkpoint unavailable")
	st := transitionFaultStore(base, nil, map[int]error{1: cancelErr})
	probe := testkit.NewBackendTransitionPhaseProbe(BackendTunarr)
	first := NewController(st, probe, probe, nil)
	if err := first.Apply(ctx, BackendTunarr); !errors.Is(err, cancelErr) {
		t.Fatalf("Apply error = %v, want target replacement failure", err)
	}
	assertDurableState(t, base, BackendTunarr, BackendInternal)
	assertSnapshot(t, first.Runtime().Snapshot(), BackendTunarr, BackendInternal, true)

	restarted := NewController(st, probe, probe, nil)
	if err := restarted.Apply(ctx, BackendTunarr); err != nil {
		t.Fatalf("retry target replacement: %v", err)
	}
	assertDurableState(t, base, BackendTunarr, "")
}

func TestControllerSteadyStateRepairsURLsAndRetriesRefresh(t *testing.T) {
	base := initializedStore(t, BackendTunarr)
	probe := testkit.NewBackendTransitionPhaseProbe(BackendTunarr)
	probe.RequirePublisherRepair() // same backend, but its resolved registration URLs changed
	refreshErr := errors.New("media server refresh failed")
	probe.FailRefreshOnce(refreshErr)
	controller := NewController(base, probe, probe, nil)

	if err := controller.Apply(context.Background(), BackendTunarr); !errors.Is(err, refreshErr) {
		t.Fatalf("first repair error = %v, want refresh failure", err)
	}
	assertDurableState(t, base, BackendTunarr, "")
	if got, want := probe.Phases(), []string{
		"fleet:tunarr",
		"publisher.prepare:tunarr",
		"publisher.refresh:tunarr",
	}; !slices.Equal(got, want) {
		t.Fatalf("first repair phases = %v, want %v", got, want)
	}

	// Fleet and Refresh must both replay. The publisher target now exists, but a prior process
	// may have crashed after only part of the inherited channel fleet adopted the changed URL.
	if err := controller.Apply(context.Background(), BackendTunarr); err != nil {
		t.Fatalf("retry repair: %v", err)
	}
	if got := probe.PrepareChanges(); !slices.Equal(got, []bool{true, false}) {
		t.Fatalf("Prepare changed results = %v, want [true false]", got)
	}
	if got := probe.RefreshCalls(); got != 2 {
		t.Fatalf("Refresh calls = %d, want retry despite unchanged registration", got)
	}
	if got := probe.RetireCalls(); got != 1 {
		t.Fatalf("Retire calls = %d, want only the successful repair", got)
	}
	if probe.FleetCalls() != 2 {
		t.Fatalf("steady-state repair fleet calls = %d, want one per attempt", probe.FleetCalls())
	}
	if got, want := probe.Phases(), []string{
		"fleet:tunarr",
		"publisher.prepare:tunarr",
		"publisher.refresh:tunarr",
		"fleet:tunarr",
		"publisher.prepare:tunarr",
		"publisher.refresh:tunarr",
		"publisher.retire:tunarr",
	}; !slices.Equal(got, want) {
		t.Fatalf("repair retry phases = %v, want %v", got, want)
	}
}

func TestControllerSteadyStateURLRepairStopsBeforePublisherWhenFleetFails(t *testing.T) {
	base := initializedStore(t, BackendTunarr)
	probe := testkit.NewBackendTransitionPhaseProbe(BackendTunarr)
	fleetErr := errors.New("channel fleet unavailable")
	probe.FailFleetOnce(fleetErr)
	controller := NewController(base, probe, probe, nil)

	if err := controller.Apply(context.Background(), BackendTunarr); !errors.Is(err, fleetErr) {
		t.Fatalf("repair error = %v, want fleet failure", err)
	}
	if got, want := probe.Phases(), []string{"fleet:tunarr"}; !slices.Equal(got, want) {
		t.Fatalf("failed repair phases = %v, want %v", got, want)
	}
	assertDurableState(t, base, BackendTunarr, "")

	if err := controller.Apply(context.Background(), BackendTunarr); err != nil {
		t.Fatalf("retry repair: %v", err)
	}
	if got, want := probe.Phases(), []string{
		"fleet:tunarr",
		"fleet:tunarr",
		"publisher.prepare:tunarr",
		"publisher.refresh:tunarr",
		"publisher.retire:tunarr",
	}; !slices.Equal(got, want) {
		t.Fatalf("repair retry phases = %v, want %v", got, want)
	}
}

func TestControllerRetirementFailureLeavesNewBackendApplied(t *testing.T) {
	base := initializedStore(t, BackendTunarr)
	probe := testkit.NewBackendTransitionPhaseProbe(BackendTunarr)
	probe.FailRetireOnce(errors.New("stale tuner busy"))
	controller := NewController(base, probe, probe, nil)

	if err := controller.Apply(context.Background(), BackendInternal); err == nil {
		t.Fatal("Apply succeeded despite retirement failure")
	}
	assertDurableState(t, base, BackendInternal, "")
	assertSnapshot(t, controller.Runtime().Snapshot(), BackendInternal, "", true)

	if err := controller.Apply(context.Background(), BackendInternal); err != nil {
		t.Fatalf("retirement retry: %v", err)
	}
	assertDurableState(t, base, BackendInternal, "")
}

func TestControllerSerializesConcurrentApplyAndRuntimeReads(t *testing.T) {
	base := initializedStore(t, BackendTunarr)
	entered := make(chan struct{})
	release := make(chan struct{})
	secondEntry := make(chan struct{})
	probe := testkit.NewBackendTransitionPhaseProbe(BackendTunarr)
	probe.BlockFleet(entered, release, secondEntry)
	controller := NewController(base, probe, probe, probe)

	stopReads := make(chan struct{})
	readsDone := make(chan struct{})
	go func() {
		defer close(readsDone)
		for {
			select {
			case <-stopReads:
				return
			default:
				_ = controller.Runtime().Snapshot()
			}
		}
	}()

	const callers = 8
	errs := make(chan error, callers)
	for range callers {
		go func() { errs <- controller.Apply(context.Background(), BackendInternal) }()
	}
	<-entered
	select {
	case <-secondEntry:
		t.Fatal("a second Apply entered fleet preparation while the first was blocked")
	case <-time.After(100 * time.Millisecond):
	}
	close(release)
	for range callers {
		if err := <-errs; err != nil {
			t.Errorf("concurrent Apply: %v", err)
		}
	}
	close(stopReads)
	<-readsDone

	if got := probe.FleetCalls(); got != callers {
		t.Fatalf("fleet calls = %d, want one barrier per serialized transition/repair", got)
	}
	assertDurableState(t, base, BackendInternal, "")
	assertSnapshot(t, controller.Runtime().Snapshot(), BackendInternal, "", true)
}

func TestControllerApplyCurrentResolvesDesiredAfterWaitingForPriorTransition(t *testing.T) {
	base := initializedStore(t, BackendTunarr)
	entered := make(chan struct{})
	release := make(chan struct{})
	probe := testkit.NewBackendTransitionPhaseProbe(BackendTunarr)
	probe.BlockFleet(entered, release, nil)
	controller := NewController(
		base,
		probe,
		probe,
		probe,
	)
	firstDone := make(chan error, 1)
	go func() { firstDone <- controller.Apply(context.Background(), BackendInternal) }()
	<-entered

	desired := BackendInternal
	resolved := make(chan struct{})
	secondDone := make(chan error, 1)
	go func() {
		secondDone <- controller.ApplyCurrent(context.Background(), func(context.Context) (string, error) {
			close(resolved)
			return desired, nil
		})
	}()
	select {
	case <-resolved:
		t.Fatal("queued ApplyCurrent resolved desired before entering the controller lock")
	case <-time.After(100 * time.Millisecond):
	}
	desired = BackendTunarr
	close(release)
	if err := <-firstDone; err != nil {
		t.Fatal(err)
	}
	if err := <-secondDone; err != nil {
		t.Fatal(err)
	}
	assertDurableState(t, base, BackendTunarr, "")
}

func TestControllerMutateAndApplyCurrentOwnsMutationThroughPublication(t *testing.T) {
	base := initializedStore(t, BackendTunarr)
	probe := testkit.NewBackendTransitionPhaseProbe(BackendTunarr)
	controller := NewController(
		transitionFaultStore(base, probe, nil),
		probe,
		probe,
		probe,
	)
	desired := BackendTunarr
	refreshed := false
	err := controller.MutateAndApplyCurrent(context.Background(), func(context.Context) error {
		probe.RecordPhase("refresh")
		refreshed = true
		return nil
	}, func(context.Context) bool {
		if !refreshed {
			t.Fatal("mutation ran before settings refresh")
		}
		probe.RecordPhase("mutation")
		desired = BackendInternal
		return true
	}, func(context.Context) (string, error) {
		probe.RecordPhase("resolve")
		return desired, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		"refresh",
		"mutation",
		"resolve",
		"save:tunarr/internal",
		"fleet:internal",
		"publisher.prepare:internal",
		"publisher.refresh:internal",
		"cutover:tunarr->internal",
		"save:internal/",
		"publisher.retire:internal",
	}
	if got := probe.Phases(); !slices.Equal(got, want) {
		t.Fatalf("mutation transition order = %v, want %v", got, want)
	}
	assertDurableState(t, base, BackendInternal, "")
}

func TestControllerMutateAndApplyCurrentSkipsRepairAfterIneffectiveMutation(t *testing.T) {
	base := initializedStore(t, BackendTunarr)
	probe := testkit.NewBackendTransitionPhaseProbe(BackendTunarr)
	controller := NewController(base, probe, probe, nil)
	resolved := false
	refreshed := false
	err := controller.MutateAndApplyCurrent(context.Background(), func(context.Context) error {
		refreshed = true
		return nil
	}, func(context.Context) bool {
		if !refreshed {
			t.Fatal("ineffective mutation evaluated stale settings")
		}
		return false
	}, func(context.Context) (string, error) {
		resolved = true
		return BackendTunarr, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if resolved || len(probe.Phases()) != 0 {
		t.Fatalf("ineffective mutation resolved or repaired: resolved=%v phases=%v", resolved, probe.Phases())
	}
}

func TestControllerMutateAndApplyCurrentStopsBeforeMutationWhenRefreshFails(t *testing.T) {
	refreshErr := errors.New("refresh durable settings")
	controller := NewController(initializedStore(t, BackendTunarr), nil, nil, nil)
	mutated := false
	err := controller.MutateAndApplyCurrent(context.Background(), func(context.Context) error {
		return refreshErr
	}, func(context.Context) bool {
		mutated = true
		return true
	}, func(context.Context) (string, error) {
		return BackendInternal, nil
	})
	if !errors.Is(err, refreshErr) {
		t.Fatalf("refresh failure = %v, want %v", err, refreshErr)
	}
	if mutated {
		t.Fatal("mutation ran after settings refresh failed")
	}
}

func initializedStore(t testing.TB, applied string) store.Store {
	t.Helper()
	st := testkit.SQLiteStore(t)
	if _, err := Load(context.Background(), st, applied); err != nil {
		t.Fatalf("initialize state: %v", err)
	}
	return st
}

func assertDurableState(t testing.TB, st stateStore, applied, prepared string) {
	t.Helper()
	state, err := Load(context.Background(), st, applied)
	if err != nil {
		t.Fatalf("load durable state: %v", err)
	}
	if state.Applied() != applied || state.Prepared() != prepared {
		t.Fatalf("durable state = %q/%q, want %q/%q",
			state.Applied(), state.Prepared(), applied, prepared)
	}
}

func assertSnapshot(t testing.TB, got Snapshot, applied, prepared string, internal bool) {
	t.Helper()
	want := Snapshot{Applied: applied, Prepared: prepared, PublishedInternal: internal}
	if got != want {
		t.Fatalf("runtime snapshot = %+v, want %+v", got, want)
	}
}

func transitionFaultStore(
	base store.Store,
	probe *testkit.BackendTransitionProbe,
	failWrites map[int]error,
) *testkit.FaultSettingStore {
	return &testkit.FaultSettingStore{
		Store:      base,
		FailWrites: failWrites,
		Inspect: func(_ string, value string) error {
			state, err := decode(value)
			if err != nil {
				return err
			}
			probe.RecordPhase(fmt.Sprintf("save:%s/%s", state.Applied(), state.Prepared()))
			return nil
		},
	}
}
