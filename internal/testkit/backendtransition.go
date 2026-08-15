package testkit

import (
	"context"
	"sync"
)

// BackendTransition is the shared in-memory double for the settings consequence seam.
type BackendTransition struct {
	mu      sync.Mutex
	Err     error
	targets []string
}

func (t *BackendTransition) Apply(_ context.Context, desired string) error {
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

// BackendTransitionProbe is the shared concurrency double for the backend
// transition Fleet and Publisher seams. Its first publisher Prepare blocks until
// ReleasePublisher is called, and it records whether any publisher phases overlap.
// Keeping this in testkit avoids package-local copies of an external-service double.
type BackendTransitionProbe struct {
	once    sync.Once
	entered chan struct{}
	release chan struct{}

	mu              sync.Mutex
	activePublisher int
	maxPublisher    int
	targets         []string
}

// NewBackendTransitionProbe constructs a probe whose first Prepare is blocked.
func NewBackendTransitionProbe() *BackendTransitionProbe {
	return &BackendTransitionProbe{entered: make(chan struct{}), release: make(chan struct{})}
}

// PrepareInheritedBackend satisfies backendtransition.Fleet. Channel convergence
// itself is deliberately a no-op; this double observes publisher serialization.
func (*BackendTransitionProbe) PrepareInheritedBackend(context.Context, string) error { return nil }

// Prepare satisfies backendtransition.Publisher and blocks the first publisher phase.
func (p *BackendTransitionProbe) Prepare(ctx context.Context, target string) (bool, error) {
	err := p.publisherPhase(ctx, target, true)
	return true, err
}

// Refresh satisfies backendtransition.Publisher.
func (p *BackendTransitionProbe) Refresh(ctx context.Context, target string) error {
	return p.publisherPhase(ctx, target, false)
}

// RetireStale satisfies backendtransition.Publisher.
func (p *BackendTransitionProbe) RetireStale(ctx context.Context, target string) error {
	return p.publisherPhase(ctx, target, false)
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

	if mayBlock {
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
