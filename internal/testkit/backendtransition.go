package testkit

import (
	"context"
	"sync"
)

// Snapshotter is a shared, typed snapshot-reader double. The generic result keeps
// testkit independent of the package defining a snapshot while still satisfying
// that package's narrow Snapshot method structurally.
type Snapshotter[T any] struct {
	Result T
	Err    error
}

func (s Snapshotter[T]) Snapshot(context.Context) (T, error) { return s.Result, s.Err }

// BackendTransition is the shared in-memory double for the settings consequence and repair seam.
// It runs the supplied mutation before recording the resolved desired target, mirroring the
// production interface without reproducing any prepare/publish phases in API tests.
type BackendTransition struct {
	mu                sync.Mutex
	Err               error
	BeforeMutationErr error
	Desired           func() string
	targets           []string
	reconnects        int
	TunersReset       int
	ReconnectErr      error
}

// Reconnect satisfies the API's serialized Live TV repair seam.
func (t *BackendTransition) Reconnect(context.Context) (int, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.reconnects++
	return t.TunersReset, t.ReconnectErr
}

// Reconnects reports how many force-repair operations were requested.
func (t *BackendTransition) Reconnects() int {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.reconnects
}

func (t *BackendTransition) ApplyMutation(ctx context.Context, mutation func(context.Context) bool) error {
	if t.BeforeMutationErr != nil {
		return t.BeforeMutationErr
	}
	if mutation == nil || !mutation(ctx) {
		return nil
	}
	desired := ""
	if t.Desired != nil {
		desired = t.Desired()
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	t.targets = append(t.targets, desired)
	return t.Err
}

// Targets returns the desired backends passed to Apply in order.
func (t *BackendTransition) Targets() []string {
	t.mu.Lock()
	defer t.mu.Unlock()
	return append([]string(nil), t.targets...)
}

// BackendTransitionProbe is the shared scripted double for the backend transition
// Fleet, Publisher, and Cutover seams. It supports both cross-controller concurrency
// probes and deterministic phase/fault recording without importing the owning package.
type BackendTransitionProbe struct {
	once                sync.Once
	entered             chan struct{}
	release             chan struct{}
	blockFirstPublisher bool

	mu              sync.Mutex
	activePublisher int
	maxPublisher    int
	targets         []string
	phases          []string

	fleetCalls       int
	fleetFailOnce    error
	fleetEntered     chan struct{}
	fleetRelease     chan struct{}
	fleetSecondEntry chan struct{}
	fleetObserve     func()

	registered      map[string]bool
	needsRepair     bool
	prepareChanges  []bool
	prepareCalls    int
	refreshCalls    int
	retireCalls     int
	reconnectCalls  int
	prepareFailOnce error
	refreshFailOnce error
	retireFailOnce  error

	cutoverCalls    int
	cutoverFailOnce error
}

// NewBackendTransitionProbe constructs a probe whose first Prepare is blocked.
func NewBackendTransitionProbe() *BackendTransitionProbe {
	return &BackendTransitionProbe{
		entered: make(chan struct{}), release: make(chan struct{}),
		blockFirstPublisher: true, registered: make(map[string]bool),
	}
}

// NewBackendTransitionPhaseProbe constructs a nonblocking phase/fault recorder.
// initialTargets are publisher registrations that already exist.
func NewBackendTransitionPhaseProbe(initialTargets ...string) *BackendTransitionProbe {
	registered := make(map[string]bool, len(initialTargets))
	for _, target := range initialTargets {
		registered[target] = true
	}
	return &BackendTransitionProbe{registered: registered}
}

// RecordPhase appends a package-specific phase to the shared ordered trace.
func (p *BackendTransitionProbe) RecordPhase(phase string) {
	if p == nil {
		return
	}
	p.mu.Lock()
	p.phases = append(p.phases, phase)
	p.mu.Unlock()
}

// Phases returns the ordered phase trace.
func (p *BackendTransitionProbe) Phases() []string {
	if p == nil {
		return nil
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]string(nil), p.phases...)
}

// ObserveFleet runs fn during each fleet barrier after its phase is recorded.
func (p *BackendTransitionProbe) ObserveFleet(fn func()) {
	p.mu.Lock()
	p.fleetObserve = fn
	p.mu.Unlock()
}

// BlockFleet scripts first-entry blocking and optional second-entry notification.
func (p *BackendTransitionProbe) BlockFleet(entered, release, secondEntry chan struct{}) {
	p.mu.Lock()
	p.fleetEntered, p.fleetRelease, p.fleetSecondEntry = entered, release, secondEntry
	p.mu.Unlock()
}

func (p *BackendTransitionProbe) FailFleetOnce(err error) {
	p.mu.Lock()
	p.fleetFailOnce = err
	p.mu.Unlock()
}

func (p *BackendTransitionProbe) FailPrepareOnce(err error) {
	p.mu.Lock()
	p.prepareFailOnce = err
	p.mu.Unlock()
}

func (p *BackendTransitionProbe) FailRefreshOnce(err error) {
	p.mu.Lock()
	p.refreshFailOnce = err
	p.mu.Unlock()
}

func (p *BackendTransitionProbe) FailRetireOnce(err error) {
	p.mu.Lock()
	p.retireFailOnce = err
	p.mu.Unlock()
}

func (p *BackendTransitionProbe) FailCutoverOnce(err error) {
	p.mu.Lock()
	p.cutoverFailOnce = err
	p.mu.Unlock()
}

// RequirePublisherRepair makes the next Prepare report a change for an existing target.
func (p *BackendTransitionProbe) RequirePublisherRepair() {
	p.mu.Lock()
	p.needsRepair = true
	p.mu.Unlock()
}

// PrepareInheritedBackend satisfies backendtransition.Fleet.
func (p *BackendTransitionProbe) PrepareInheritedBackend(_ context.Context, target string) error {
	p.mu.Lock()
	p.fleetCalls++
	call := p.fleetCalls
	err := p.fleetFailOnce
	p.fleetFailOnce = nil
	entered, release, second := p.fleetEntered, p.fleetRelease, p.fleetSecondEntry
	observe := p.fleetObserve
	p.mu.Unlock()

	p.RecordPhase("fleet:" + target)
	if call == 1 && entered != nil {
		close(entered)
	}
	if call == 2 && second != nil {
		close(second)
	}
	if observe != nil {
		observe()
	}
	if call == 1 && release != nil {
		<-release
	}
	return err
}

// Prepare satisfies backendtransition.Publisher and blocks the first publisher phase.
func (p *BackendTransitionProbe) Prepare(ctx context.Context, target string) (bool, error) {
	p.RecordPhase("publisher.prepare:" + target)
	if err := p.publisherPhase(ctx, target, true); err != nil {
		return false, err
	}
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
	p.prepareChanges = append(p.prepareChanges, changed)
	return changed, nil
}

// Refresh satisfies backendtransition.Publisher.
func (p *BackendTransitionProbe) Refresh(ctx context.Context, target string) error {
	p.RecordPhase("publisher.refresh:" + target)
	if err := p.publisherPhase(ctx, target, false); err != nil {
		return err
	}
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

// RetireStale satisfies backendtransition.Publisher.
func (p *BackendTransitionProbe) RetireStale(ctx context.Context, target string) error {
	p.RecordPhase("publisher.retire:" + target)
	if err := p.publisherPhase(ctx, target, false); err != nil {
		return err
	}
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

// Reconnect satisfies backendtransition.Publisher and records one publisher phase.
func (p *BackendTransitionProbe) Reconnect(ctx context.Context, target string) (int, error) {
	p.RecordPhase("publisher.reconnect:" + target)
	if err := p.publisherPhase(ctx, target, false); err != nil {
		return 0, err
	}
	p.mu.Lock()
	p.reconnectCalls++
	p.mu.Unlock()
	return 1, nil
}

func (p *BackendTransitionProbe) publisherPhase(ctx context.Context, target string, mayBlock bool) error {
	p.mu.Lock()
	p.activePublisher++
	if p.activePublisher > p.maxPublisher {
		p.maxPublisher = p.activePublisher
	}
	p.targets = append(p.targets, target)
	p.mu.Unlock()
	defer func() {
		p.mu.Lock()
		p.activePublisher--
		p.mu.Unlock()
	}()

	if mayBlock && p.blockFirstPublisher {
		blocked := false
		p.once.Do(func() {
			blocked = true
			close(p.entered)
		})
		if blocked {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-p.release:
			}
		}
	}
	return nil
}

// BeforePublish satisfies backendtransition.Cutover.
func (p *BackendTransitionProbe) BeforePublish(_ context.Context, from, to string) error {
	p.RecordPhase("cutover:" + from + "->" + to)
	p.mu.Lock()
	defer p.mu.Unlock()
	p.cutoverCalls++
	if p.cutoverFailOnce != nil {
		err := p.cutoverFailOnce
		p.cutoverFailOnce = nil
		return err
	}
	return nil
}

// PublisherEntered closes when the first publisher Prepare begins.
func (p *BackendTransitionProbe) PublisherEntered() <-chan struct{} { return p.entered }

// ReleasePublisher lets the first publisher Prepare complete.
func (p *BackendTransitionProbe) ReleasePublisher() { close(p.release) }

// MaxPublisherConcurrency reports the largest number of simultaneous publisher phases.
func (p *BackendTransitionProbe) MaxPublisherConcurrency() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.maxPublisher
}

// PublisherTargets returns the target observed for each publisher phase, in call order.
func (p *BackendTransitionProbe) PublisherTargets() []string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]string(nil), p.targets...)
}

func (p *BackendTransitionProbe) FleetCalls() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.fleetCalls
}

func (p *BackendTransitionProbe) CutoverCalls() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.cutoverCalls
}

func (p *BackendTransitionProbe) PrepareChanges() []bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]bool(nil), p.prepareChanges...)
}

func (p *BackendTransitionProbe) RefreshCalls() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.refreshCalls
}

func (p *BackendTransitionProbe) RetireCalls() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.retireCalls
}

func (p *BackendTransitionProbe) IsRegistered(target string) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.registered[target]
}
