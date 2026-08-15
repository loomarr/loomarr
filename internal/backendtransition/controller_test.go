package backendtransition

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/mantonx/loomarr/internal/store"
	"github.com/mantonx/loomarr/internal/testkit"
)

func TestControllerAppliesInDurableOrderAndPublishesRuntimeAfterSave(t *testing.T) {
	base := initializedStore(t, BackendTunarr)
	record := &phaseRecorder{}
	st := &faultSettingStore{Store: base, record: record}
	fleet := &fleetAdapter{record: record}
	publisher := newPublisherAdapter(record)
	cutover := &cutoverAdapter{record: record}
	controller := NewController(st, fleet, publisher, cutover)
	var fleetSnapshot Snapshot
	fleet.observe = func() { fleetSnapshot = controller.Runtime().Snapshot() }

	var snapshots []Snapshot
	st.afterSave = func() { snapshots = append(snapshots, controller.Runtime().Snapshot()) }
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
	if got := record.snapshot(); !slices.Equal(got, wantOrder) {
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
	record := &phaseRecorder{}
	controller := NewController(st,
		&fleetAdapter{record: record}, newPublisherAdapter(record), &cutoverAdapter{record: record})

	if err := controller.Initialize(context.Background(), func(context.Context) (string, error) {
		return BackendInternal, nil
	}); err != nil {
		t.Fatal(err)
	}
	assertDurableState(t, st, BackendInternal, "")
	assertSnapshot(t, controller.Runtime().Snapshot(), BackendInternal, "", true)
	if got := record.snapshot(); len(got) != 0 {
		t.Fatalf("Initialize invoked transition adapters: %v", got)
	}

	if err := controller.Initialize(context.Background(), func(context.Context) (string, error) {
		return "external", nil
	}); !errors.Is(err, ErrInvalidState) {
		t.Fatalf("invalid desired error = %v, want ErrInvalidState from Load", err)
	}
	assertSnapshot(t, controller.Runtime().Snapshot(), BackendInternal, "", true)
	if got := record.snapshot(); len(got) != 0 {
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
	record := &phaseRecorder{}
	publisher := newPublisherAdapter(record)
	controller := NewController(st, &fleetAdapter{record: record}, publisher, nil)

	reset, err := controller.ReconnectCurrent(context.Background(), func(context.Context) (string, error) {
		return BackendInternal, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if reset != 1 {
		t.Fatalf("tuners reset = %d, want 1", reset)
	}
	if got := record.snapshot(); !slices.Equal(got, []string{"publisher.reconnect:tunarr"}) {
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
			st := &faultSettingStore{Store: base, failWrites: map[int]error{}}
			fleet := &fleetAdapter{}
			publisher := newPublisherAdapter(nil)
			cutover := &cutoverAdapter{}
			switch phase {
			case "fleet":
				fleet.failOnce = crash
			case "save-prepared":
				st.failWrites[1] = crash
			case "publisher-prepare":
				publisher.prepareFailOnce = crash
			case "refresh":
				publisher.refreshFailOnce = crash
			case "cutover":
				cutover.failOnce = crash
			case "save-published":
				st.failWrites[2] = crash
			case "retire":
				publisher.retireFailOnce = crash
			}

			first := NewController(st, fleet, publisher, cutover)
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
			restarted := NewController(st, fleet, publisher, cutover)
			if err := restarted.Apply(context.Background(), BackendInternal); err != nil {
				t.Fatalf("Apply after restart: %v", err)
			}
			assertDurableState(t, base, BackendInternal, "")
			assertSnapshot(t, restarted.Runtime().Snapshot(), BackendInternal, "", true)

			wantFleetCalls := 1
			if phase == "fleet" || phase == "publisher-prepare" || phase == "refresh" ||
				phase == "cutover" || phase == "save-published" {
				wantFleetCalls = 2 // a durable in-progress target always replays the fleet barrier
			}
			if got := fleet.callCount(); got != wantFleetCalls {
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

	record := &phaseRecorder{}
	st := &faultSettingStore{Store: base, record: record}
	fleet := &fleetAdapter{record: record}
	publisher := newPublisherAdapter(record)
	publisher.registered[BackendInternal] = true
	cutover := &cutoverAdapter{record: record}
	controller := NewController(st, fleet, publisher, cutover)

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
	if got := record.snapshot(); !slices.Equal(got, want) {
		t.Fatalf("reversal phases = %v, want %v", got, want)
	}
	if fleet.callCount() != 1 || cutover.callCount() != 0 {
		t.Fatalf("reversal fleet/cutover calls: fleet=%d cutover=%d, want 1/0",
			fleet.callCount(), cutover.callCount())
	}
	assertDurableState(t, base, BackendTunarr, "")
	assertSnapshot(t, controller.Runtime().Snapshot(), BackendTunarr, "", false)
	if publisher.isRegistered(BackendInternal) {
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
	st := &faultSettingStore{Store: base, failWrites: map[int]error{1: cancelErr}}
	publisher := newPublisherAdapter(nil)
	first := NewController(st, &fleetAdapter{}, publisher, nil)
	if err := first.Apply(ctx, BackendTunarr); !errors.Is(err, cancelErr) {
		t.Fatalf("Apply error = %v, want target replacement failure", err)
	}
	assertDurableState(t, base, BackendTunarr, BackendInternal)
	assertSnapshot(t, first.Runtime().Snapshot(), BackendTunarr, BackendInternal, true)

	restarted := NewController(st, &fleetAdapter{}, publisher, nil)
	if err := restarted.Apply(ctx, BackendTunarr); err != nil {
		t.Fatalf("retry target replacement: %v", err)
	}
	assertDurableState(t, base, BackendTunarr, "")
}

func TestControllerSteadyStateRepairsURLsAndRetriesRefresh(t *testing.T) {
	base := initializedStore(t, BackendTunarr)
	publisher := newPublisherAdapter(nil)
	publisher.needsRepair = true // same backend, but its resolved registration URLs changed
	refreshErr := errors.New("media server refresh failed")
	publisher.refreshFailOnce = refreshErr
	fleet := &fleetAdapter{}
	controller := NewController(base, fleet, publisher, nil)

	if err := controller.Apply(context.Background(), BackendTunarr); !errors.Is(err, refreshErr) {
		t.Fatalf("first repair error = %v, want refresh failure", err)
	}
	assertDurableState(t, base, BackendTunarr, "")

	// Prepare now reports unchanged because the repaired URL exists. Refresh must still replay;
	// otherwise the first failure would become permanent.
	if err := controller.Apply(context.Background(), BackendTunarr); err != nil {
		t.Fatalf("retry repair: %v", err)
	}
	if got := publisher.prepareChanges(); !slices.Equal(got, []bool{true, false}) {
		t.Fatalf("Prepare changed results = %v, want [true false]", got)
	}
	if got := publisher.refreshCount(); got != 2 {
		t.Fatalf("Refresh calls = %d, want retry despite unchanged registration", got)
	}
	if got := publisher.retireCount(); got != 1 {
		t.Fatalf("Retire calls = %d, want only the successful repair", got)
	}
	if fleet.callCount() != 0 {
		t.Fatalf("steady-state repair prepared fleet %d times", fleet.callCount())
	}
}

func TestControllerRetirementFailureLeavesNewBackendApplied(t *testing.T) {
	base := initializedStore(t, BackendTunarr)
	publisher := newPublisherAdapter(nil)
	publisher.retireFailOnce = errors.New("stale tuner busy")
	controller := NewController(base, &fleetAdapter{}, publisher, nil)

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
	fleet := &fleetAdapter{entered: entered, release: release, secondEntry: secondEntry}
	publisher := newPublisherAdapter(nil)
	controller := NewController(base, fleet, publisher, &cutoverAdapter{})

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

	if got := fleet.callCount(); got != 1 {
		t.Fatalf("fleet calls = %d, want one transition followed by steady repairs", got)
	}
	assertDurableState(t, base, BackendInternal, "")
	assertSnapshot(t, controller.Runtime().Snapshot(), BackendInternal, "", true)
}

func TestControllerApplyCurrentResolvesDesiredAfterWaitingForPriorTransition(t *testing.T) {
	base := initializedStore(t, BackendTunarr)
	entered := make(chan struct{})
	release := make(chan struct{})
	controller := NewController(
		base,
		&fleetAdapter{entered: entered, release: release},
		newPublisherAdapter(nil),
		&cutoverAdapter{},
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
	record := &phaseRecorder{}
	controller := NewController(
		&faultSettingStore{Store: base, record: record},
		&fleetAdapter{record: record},
		newPublisherAdapter(record),
		&cutoverAdapter{record: record},
	)
	desired := BackendTunarr
	refreshed := false
	err := controller.MutateAndApplyCurrent(context.Background(), func(context.Context) error {
		record.add("refresh")
		refreshed = true
		return nil
	}, func(context.Context) bool {
		if !refreshed {
			t.Fatal("mutation ran before settings refresh")
		}
		record.add("mutation")
		desired = BackendInternal
		return true
	}, func(context.Context) (string, error) {
		record.add("resolve")
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
	if got := record.snapshot(); !slices.Equal(got, want) {
		t.Fatalf("mutation transition order = %v, want %v", got, want)
	}
	assertDurableState(t, base, BackendInternal, "")
}

func TestControllerMutateAndApplyCurrentSkipsRepairAfterIneffectiveMutation(t *testing.T) {
	base := initializedStore(t, BackendTunarr)
	record := &phaseRecorder{}
	controller := NewController(base, &fleetAdapter{record: record}, newPublisherAdapter(record), nil)
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
	if resolved || len(record.snapshot()) != 0 {
		t.Fatalf("ineffective mutation resolved or repaired: resolved=%v phases=%v", resolved, record.snapshot())
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

type phaseRecorder struct {
	mu     sync.Mutex
	phases []string
}

func (r *phaseRecorder) add(phase string) {
	if r == nil {
		return
	}
	r.mu.Lock()
	r.phases = append(r.phases, phase)
	r.mu.Unlock()
}

func (r *phaseRecorder) snapshot() []string {
	if r == nil {
		return nil
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.phases...)
}

type faultSettingStore struct {
	store.Store

	mu         sync.Mutex
	writes     int
	failWrites map[int]error
	record     *phaseRecorder
	afterSave  func()
}

func (s *faultSettingStore) SetSetting(ctx context.Context, key, value string) error {
	s.mu.Lock()
	s.writes++
	write := s.writes
	fail := s.failWrites[write]
	s.mu.Unlock()

	state, err := decode(value)
	if err != nil {
		return err
	}
	s.record.add(fmt.Sprintf("save:%s/%s", state.Applied(), state.Prepared()))
	if fail != nil {
		return fail
	}
	if err := s.Store.SetSetting(ctx, key, value); err != nil {
		return err
	}
	if s.afterSave != nil {
		s.afterSave()
	}
	return nil
}

type fleetAdapter struct {
	mu          sync.Mutex
	calls       int
	failOnce    error
	record      *phaseRecorder
	entered     chan struct{}
	release     chan struct{}
	secondEntry chan struct{}
	observe     func()
}

func (f *fleetAdapter) PrepareInheritedBackend(_ context.Context, target string) error {
	f.mu.Lock()
	f.calls++
	call := f.calls
	err := f.failOnce
	f.failOnce = nil
	if call == 1 && f.entered != nil {
		close(f.entered)
	}
	if call == 2 && f.secondEntry != nil {
		close(f.secondEntry)
	}
	f.mu.Unlock()
	f.record.add("fleet:" + target)
	if f.observe != nil {
		f.observe()
	}
	if call == 1 && f.release != nil {
		<-f.release
	}
	return err
}

func (f *fleetAdapter) callCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls
}

type publisherAdapter struct {
	mu sync.Mutex

	record *phaseRecorder

	registered  map[string]bool
	needsRepair bool
	changes     []bool

	prepareCalls   int
	refreshCalls   int
	retireCalls    int
	reconnectCalls int

	prepareFailOnce error
	refreshFailOnce error
	retireFailOnce  error
}

func newPublisherAdapter(record *phaseRecorder) *publisherAdapter {
	return &publisherAdapter{record: record, registered: map[string]bool{BackendTunarr: true}}
}

func (p *publisherAdapter) Prepare(_ context.Context, target string) (bool, error) {
	p.record.add("publisher.prepare:" + target)
	p.mu.Lock()
	defer p.mu.Unlock()
	p.prepareCalls++
	if p.prepareFailOnce != nil {
		err := p.prepareFailOnce
		p.prepareFailOnce = nil
		return false, err
	}
	changed := !p.registered[target] || p.needsRepair
	p.registered[target] = true
	p.needsRepair = false
	p.changes = append(p.changes, changed)
	return changed, nil
}

func (p *publisherAdapter) Refresh(_ context.Context, target string) error {
	p.record.add("publisher.refresh:" + target)
	p.mu.Lock()
	defer p.mu.Unlock()
	p.refreshCalls++
	if p.refreshFailOnce != nil {
		err := p.refreshFailOnce
		p.refreshFailOnce = nil
		return err
	}
	return nil
}

func (p *publisherAdapter) RetireStale(_ context.Context, target string) error {
	p.record.add("publisher.retire:" + target)
	p.mu.Lock()
	defer p.mu.Unlock()
	p.retireCalls++
	if p.retireFailOnce != nil {
		err := p.retireFailOnce
		p.retireFailOnce = nil
		return err
	}
	for backend := range p.registered {
		if backend != target {
			delete(p.registered, backend)
		}
	}
	return nil
}

func (p *publisherAdapter) Reconnect(_ context.Context, target string) (int, error) {
	p.record.add("publisher.reconnect:" + target)
	p.mu.Lock()
	defer p.mu.Unlock()
	p.reconnectCalls++
	return 1, nil
}

func (p *publisherAdapter) prepareChanges() []bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]bool(nil), p.changes...)
}

func (p *publisherAdapter) refreshCount() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.refreshCalls
}

func (p *publisherAdapter) retireCount() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.retireCalls
}

func (p *publisherAdapter) isRegistered(target string) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.registered[target]
}

type cutoverAdapter struct {
	mu       sync.Mutex
	calls    int
	failOnce error
	record   *phaseRecorder
}

func (c *cutoverAdapter) BeforePublish(_ context.Context, from, to string) error {
	c.record.add("cutover:" + from + "->" + to)
	c.mu.Lock()
	defer c.mu.Unlock()
	c.calls++
	if c.failOnce != nil {
		err := c.failOnce
		c.failOnce = nil
		return err
	}
	return nil
}

func (c *cutoverAdapter) callCount() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.calls
}
