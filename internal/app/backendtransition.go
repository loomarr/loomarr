package app

import (
	"context"
	"fmt"
	"sync"

	"github.com/mantonx/loomarr/internal/backendtransition"
	"github.com/mantonx/loomarr/internal/schedule"
	"github.com/mantonx/loomarr/internal/setup"
	"github.com/mantonx/loomarr/internal/store"
)

// backendPublisher adapts the transition controller's backend names to explicit URL
// snapshots. No phase re-reads live desired settings, so a concurrent save cannot make one
// transition prepare one target and refresh or retire against another.
type backendPublisher struct {
	connector      *setup.LiveTVConnector
	urls           func(target string) setup.TunarrURLs
	mu             sync.Mutex
	preparedTarget string
	preparedURLs   setup.TunarrURLs
}

// currentBackendTransition keeps desired resolution inside Controller's serialization
// boundary. The API argument describes the save that triggered the work, but a maintenance
// retry may already be queued; resolving here prevents that older waiter from publishing after
// a newer saved value.
type currentBackendTransition struct {
	controller *backendtransition.Controller
	desired    func(context.Context) (string, error)
}

// transitionReconcileBackend keeps ordinary channel maintenance on the same durable target as
// an in-progress fleet barrier. Request routing continues to use Applied; reconcile must use
// Prepared when present or it can overwrite channels the barrier has already moved.
func transitionReconcileBackend(runtime *backendtransition.Runtime, fallback func() string) string {
	if runtime != nil {
		snapshot := runtime.Snapshot()
		if snapshot.Prepared != "" {
			return snapshot.Prepared
		}
		if snapshot.Applied != "" {
			return snapshot.Applied
		}
	}
	if fallback == nil {
		return ""
	}
	return fallback()
}

func (t currentBackendTransition) Apply(ctx context.Context, _ string) error {
	if t.controller == nil {
		return fmt.Errorf("backend transition controller is unavailable")
	}
	return t.controller.ApplyCurrent(ctx, t.desired)
}

func (p *backendPublisher) Prepare(ctx context.Context, target string) (bool, error) {
	if p.connector == nil || p.urls == nil {
		return false, fmt.Errorf("live TV publisher is unavailable")
	}
	p.mu.Lock()
	p.preparedTarget = target
	p.preparedURLs = p.urls(target)
	urls := p.preparedURLs
	p.mu.Unlock()
	result, err := p.connector.Prepare(ctx, urls)
	return result.TunerAdded || result.ListingAdded, err
}

func (p *backendPublisher) Refresh(ctx context.Context, target string) error {
	urls, err := p.prepared(target)
	if err != nil {
		return err
	}
	return p.connector.RefreshTarget(ctx, urls)
}

func (p *backendPublisher) RetireStale(ctx context.Context, target string) error {
	urls, err := p.prepared(target)
	if err != nil {
		return err
	}
	_, err = p.connector.RetireStale(ctx, urls)
	return err
}

func (p *backendPublisher) prepared(target string) (setup.TunarrURLs, error) {
	if p == nil || p.connector == nil {
		return setup.TunarrURLs{}, fmt.Errorf("live TV publisher is unavailable")
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.preparedTarget != target {
		return setup.TunarrURLs{}, fmt.Errorf("live TV publisher target %q was not prepared", target)
	}
	return p.preparedURLs, nil
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
