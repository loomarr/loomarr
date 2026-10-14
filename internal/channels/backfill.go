package channels

import (
	"context"

	"github.com/loomarr/loomarr/internal/provision"
)

// This file wires the backfill loop (§9): provisioning availability events
// trigger a reconcile of the channels that reference the changed title. Crucially
// this is a *latency optimization* — the periodic sweep (runner.go) reconciles
// every channel regardless, so a dropped event is recovered on the next sweep
// (§9 "events are never load-bearing"). That property is what the §19 event-loss
// test asserts: drop the event and the sweep still backfills.

// OnAvailability handles a provisioning domain event for the scheduler (§4 inv.
// 2 emits these on terminal transitions). For `available`/`unavailable` on a key,
// it reconciles every channel whose approved lineup references that key, so the
// pending placeholder is replaced in place (available) or substituted
// (unavailable) — both are just "recompute desired + push", which Reconcile
// already does idempotently. Errors are per-channel and logged, never returned as
// a batch failure (one bad channel shouldn't block the others).
func (e *Engine) OnAvailability(ctx context.Context, ev provision.DomainEvent) {
	// Only availability-affecting transitions matter to the scheduler. Other
	// events (e.g. requested→downloading) don't change what's playable.
	if !affectsAvailability(ev) {
		return
	}
	ids, err := e.channelsReferencing(ctx, ev.Key)
	if err != nil {
		e.log.Error("backfill: find channels referencing key", "key", ev.Key, "err", err)
		return
	}
	for _, id := range ids {
		if err := e.Reconcile(ctx, id); err != nil {
			if isNotFound(err) {
				continue // channel deleted between lookup and reconcile — benign race
			}
			e.log.Error("backfill reconcile", "channel", id, "key", ev.Key, "err", err)
		}
	}
}

// affectsAvailability reports whether an event changes a title's playability, so
// backfill only wakes on transitions the scheduler cares about (available or
// unavailable). This keeps the event path cheap; anything missed is caught by the
// sweep anyway.
func affectsAvailability(ev provision.DomainEvent) bool {
	switch ev.State {
	case provision.Available, provision.Unavailable:
		return true
	default:
		return false
	}
}

// channelsReferencing returns the ids of channels whose approved lineup includes
// key. v1 scans all channels (household scale — a handful); when catalogs grow, a
// key→channel index is the optimization (§20), not now.
func (e *Engine) channelsReferencing(ctx context.Context, key provision.Key) ([]string, error) {
	all, err := e.store.ListChannels(ctx)
	if err != nil {
		return nil, err
	}
	var ids []string
	for _, ch := range all {
		for _, entry := range ch.Lineup {
			if entry.Key == key {
				ids = append(ids, ch.ID)
				break
			}
		}
	}
	return ids, nil
}
