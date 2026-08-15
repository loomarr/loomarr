package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync/atomic"
	"time"

	"github.com/mantonx/loomarr/internal/backendtransition"
	"github.com/mantonx/loomarr/internal/schedule"
	"github.com/mantonx/loomarr/internal/store"
)

func durableInternalTransportPlayable(
	ctx context.Context,
	st interface {
		GetChannel(context.Context, string) (store.Channel, error)
	},
	checkpoint lifecycleCheckpoint,
	channelID string,
) (bool, error) {
	channel, err := st.GetChannel(ctx, channelID)
	if errors.Is(err, store.ErrNotFound) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("read channel %s: %w", channelID, err)
	}
	snapshot, err := checkpoint.Snapshot(ctx)
	if err != nil {
		return false, fmt.Errorf("read backend checkpoint: %w", err)
	}
	return schedule.InternalTransportPlayable(
		channel.Status, channel.Policy, snapshot.PublishedInternal,
	), nil
}

type playoutAdmissionGate struct{ open atomic.Bool }

func (g *playoutAdmissionGate) Available() bool { return g != nil && g.open.Load() }

type lifecycleOrigin interface {
	StopChannel(string)
	StopAll()
}

type lifecycleCheckpoint interface {
	Snapshot(context.Context) (backendtransition.Snapshot, error)
}

// postgresPlayoutLifecycle keeps process-local delivery aligned with durable channel and backend
// publication state. Notifications are the commit latency path; fail-closed StopAll plus full
// reconciliation on every subscription is the notification-loss recovery path.
type postgresPlayoutLifecycle struct {
	store      store.Store
	checkpoint lifecycleCheckpoint
	origin     lifecycleOrigin
	gate       *playoutAdmissionGate
	log        *slog.Logger
	listen     func(
		context.Context,
		store.Store,
		func(context.Context) error,
		func(context.Context, store.Invalidation) error,
	) error
}

func (l *postgresPlayoutLifecycle) Start(ctx context.Context) error {
	if l == nil || l.store == nil || l.checkpoint == nil || l.origin == nil || l.gate == nil {
		return fmt.Errorf("postgres playout lifecycle is incomplete")
	}
	first := make(chan error, 1)
	go l.run(ctx, first)
	select {
	case err := <-first:
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (l *postgresPlayoutLifecycle) run(ctx context.Context, first chan<- error) {
	initial := true
	backoff := 100 * time.Millisecond
	for {
		l.closeAdmission()
		becameReady := false
		listen := l.listen
		if listen == nil {
			listen = store.ListenInvalidations
		}
		err := listen(ctx, l.store, func(readyCtx context.Context) error {
			if err := l.reconcileAll(readyCtx); err != nil {
				return err
			}
			l.gate.open.Store(true)
			becameReady = true
			if initial {
				first <- nil
				initial = false
			}
			return nil
		}, l.apply)
		l.closeAdmission()
		if ctx.Err() != nil {
			if initial {
				first <- ctx.Err()
			}
			return
		}
		if initial && !becameReady {
			first <- err
			return
		}
		if l.log != nil {
			l.log.Warn("playout lifecycle: durable listener lost; local playout closed",
				"err", err, "retry_in", backoff)
		}
		timer := time.NewTimer(backoff)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
		if backoff < 5*time.Second {
			backoff *= 2
			if backoff > 5*time.Second {
				backoff = 5 * time.Second
			}
		}
	}
}

func (l *postgresPlayoutLifecycle) closeAdmission() {
	l.gate.open.Store(false)
	l.origin.StopAll()
}

func (l *postgresPlayoutLifecycle) reconcileAll(ctx context.Context) error {
	snapshot, err := l.checkpoint.Snapshot(ctx)
	if err != nil {
		return fmt.Errorf("read backend checkpoint: %w", err)
	}
	channels, err := l.store.ListChannels(ctx)
	if err != nil {
		return fmt.Errorf("list channels: %w", err)
	}
	for _, channel := range channels {
		if !schedule.InternalTransportPlayable(
			channel.Status, channel.Policy, snapshot.PublishedInternal,
		) {
			l.origin.StopChannel(channel.ID)
		}
	}
	return nil
}

func (l *postgresPlayoutLifecycle) apply(ctx context.Context, event store.Invalidation) error {
	switch event.Kind {
	case store.InvalidationChannel:
		if event.ChannelID == "" {
			return fmt.Errorf("channel invalidation has no channel id")
		}
		// Payload state preserves an off-air transition even if a later resume is already
		// durable by the time this notification is handled.
		if !event.Status.Reconcilable() || event.Status == schedule.StatusEmpty ||
			event.Backend == schedule.PlayoutBackendTunarr {
			l.origin.StopChannel(event.ChannelID)
			return nil
		}
		if event.Backend == schedule.PlayoutBackendInternal {
			return nil
		}
		snapshot, err := l.checkpoint.Snapshot(ctx)
		if err != nil {
			return fmt.Errorf("read backend checkpoint for channel %s: %w", event.ChannelID, err)
		}
		if !snapshot.PublishedInternal {
			l.origin.StopChannel(event.ChannelID)
		}
		return nil

	case store.InvalidationBackend:
		snapshot, err := backendtransition.ParseSnapshot(event.Value)
		if err != nil {
			return fmt.Errorf("decode committed backend checkpoint: %w", err)
		}
		if snapshot.PublishedInternal {
			return nil
		}
		channels, err := l.store.ListChannels(ctx)
		if err != nil {
			return fmt.Errorf("list channels for backend cutover: %w", err)
		}
		for _, channel := range channels {
			if channel.Status.Reconcilable() && channel.Status != schedule.StatusEmpty &&
				!schedule.HasExplicitPlayoutBackend(channel.Policy) {
				l.origin.StopChannel(channel.ID)
			}
		}
		return nil

	default:
		return fmt.Errorf("unknown durable invalidation kind %q", event.Kind)
	}
}
