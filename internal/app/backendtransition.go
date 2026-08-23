package app

import (
	"context"
	"fmt"
	"sync"

	"github.com/loomarr/loomarr/internal/backendtransition"
	"github.com/loomarr/loomarr/internal/schedule"
	"github.com/loomarr/loomarr/internal/setup"
	"github.com/loomarr/loomarr/internal/store"
)

// backendTransitionDependencies are the composition-owned adapters needed to construct the
// durable backend workflow. Keeping initialization here prevents the composition root from knowing the
// controller's boot ordering in addition to all of its other subsystem wiring.
type backendTransitionDependencies struct {
	store     store.Store
	fleet     backendtransition.Fleet
	publisher backendtransition.Publisher
	cutover   backendtransition.Cutover
	desired   func(context.Context) (string, error)
}

func buildBackendTransition(
	ctx context.Context,
	deps backendTransitionDependencies,
) (*backendtransition.Controller, error) {
	controller := backendtransition.NewController(
		deps.store, deps.fleet, deps.publisher, deps.cutover,
	)
	if err := controller.Initialize(ctx, deps.desired); err != nil {
		return nil, fmt.Errorf("initialize playout backend transition: %w", err)
	}
	return controller, nil
}

// backendPublisher adapts the transition controller's backend names to one operation snapshot:
// target URLs plus the bound media-library connection. No phase re-reads live settings, so a
// concurrent save cannot make one transition prepare one target/server and refresh or retire
// against another.
type backendPublisher struct {
	connector       *setup.LiveTVConnector
	urls            func(context.Context, string) (setup.LiveTVURLs, error)
	mu              sync.Mutex
	preparedTarget  string
	preparedURLs    setup.LiveTVURLs
	preparedLibrary *setup.LiveTVConnector
}

// currentBackendTransition refreshes the settings snapshot, then keeps the mutation and desired
// resolution inside the Controller's distributed serialization boundary. That prevents an older
// replica from deciding a write against stale value or provenance state, or publishing after a
// newer desired value was durably saved by another.
type currentBackendTransition struct {
	controller *backendtransition.Controller
	refresh    func(context.Context) error
	desired    func(context.Context) (string, error)
}

func (t currentBackendTransition) ApplyMutation(
	ctx context.Context,
	mutation func(context.Context) bool,
) error {
	if t.controller == nil {
		return fmt.Errorf("backend transition controller is unavailable")
	}
	return t.controller.MutateAndApplyCurrent(ctx, t.refresh, mutation, t.desired)
}

func (t currentBackendTransition) Reconnect(ctx context.Context) (int, error) {
	if t.controller == nil {
		return 0, fmt.Errorf("backend transition controller is unavailable")
	}
	if t.desired == nil {
		return 0, fmt.Errorf("desired backend resolver is unavailable")
	}
	return t.controller.ReconnectCurrent(ctx, func(lockCtx context.Context) (string, error) {
		if t.refresh == nil {
			return "", fmt.Errorf("settings refresh is unavailable")
		}
		if err := t.refresh(lockCtx); err != nil {
			return "", err
		}
		return t.desired(lockCtx)
	})
}

func (p *backendPublisher) Prepare(ctx context.Context, target string) (bool, error) {
	if p.connector == nil || p.urls == nil {
		return false, fmt.Errorf("live TV publisher is unavailable")
	}
	urls, err := p.urls(ctx, target)
	if err != nil {
		return false, err
	}
	operation := p.connector.Snapshot()
	result, err := operation.Prepare(ctx, urls)
	if err == nil {
		p.mu.Lock()
		p.preparedTarget = target
		p.preparedURLs = urls
		p.preparedLibrary = operation
		p.mu.Unlock()
	}
	return result.TunerAdded || result.ListingAdded, err
}

func (p *backendPublisher) Refresh(ctx context.Context, target string) error {
	operation, urls, err := p.prepared(target)
	if err != nil {
		return err
	}
	return operation.RefreshTarget(ctx, urls)
}

func (p *backendPublisher) RetireStale(ctx context.Context, target string) error {
	operation, urls, err := p.prepared(target)
	if err != nil {
		return err
	}
	_, err = operation.RetireStale(ctx, urls)
	return err
}

func (p *backendPublisher) Reconnect(ctx context.Context, target string) (int, error) {
	if p == nil || p.connector == nil || p.urls == nil {
		return 0, fmt.Errorf("live TV publisher is unavailable")
	}
	urls, err := p.urls(ctx, target)
	if err != nil {
		return 0, err
	}
	result, err := p.connector.ReconnectTarget(ctx, urls)
	return result.TunerRemoved, err
}

func (p *backendPublisher) prepared(target string) (*setup.LiveTVConnector, setup.LiveTVURLs, error) {
	if p == nil || p.connector == nil {
		return nil, setup.LiveTVURLs{}, fmt.Errorf("live TV publisher is unavailable")
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.preparedTarget != target || p.preparedLibrary == nil {
		return nil, setup.LiveTVURLs{}, fmt.Errorf("live TV publisher target %q was not prepared", target)
	}
	return p.preparedLibrary, p.preparedURLs, nil
}

type channelStopper interface {
	StopChannel(channelID string)
}

// inheritedInternalCutover retires only sessions that actually belonged to the old global
// internal backend. Explicit internal pins keep playing after a global switch to Tunarr.
type inheritedInternalCutover struct {
	channels interface {
		ListChannels(context.Context) ([]store.Channel, error)
	}
	playout channelStopper
}

func (c inheritedInternalCutover) BeforePublish(ctx context.Context, from, to string) error {
	if from != backendtransition.BackendInternal || to == backendtransition.BackendInternal {
		return nil
	}
	if c.channels == nil || c.playout == nil {
		return fmt.Errorf("internal playout cutover is unavailable")
	}
	channels, err := c.channels.ListChannels(ctx)
	if err != nil {
		return fmt.Errorf("list channels for internal playout cutover: %w", err)
	}
	for _, channel := range channels {
		if schedule.PlaysInternally(channel.Policy, from) &&
			!schedule.PlaysInternally(channel.Policy, to) {
			c.playout.StopChannel(channel.ID)
		}
	}
	return nil
}
