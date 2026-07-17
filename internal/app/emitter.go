package app

import (
	"context"
	"sync/atomic"

	"github.com/mantonx/loomarr/internal/channels"
	"github.com/mantonx/loomarr/internal/events"
	"github.com/mantonx/loomarr/internal/provision"
)

// eventEmitter is the composition-root fan-out for provisioning domain events
// (§4 inv. 2): the reconciler and the webhook ingest handler both Emit through
// it, and it routes each event to two sinks —
//
//   - the channel scheduler's backfill (OnAvailability): recompute + push the
//     lineups of every channel referencing the changed key, so an acquisition
//     that just landed `available` appears without waiting for the next sweep
//     (#10). The engine is set after it's constructed (setEngine); until then,
//     and if the scheduler is unconfigured, this sink is skipped.
//   - the SSE event bus (Publish): a UI-facing `title` state-change frame (#11).
//
// Both sinks are latency optimizations, never load-bearing: the channel sweep
// reconciles every channel regardless, and GET is the source of truth for SSE
// (§8/§9). engine is read on the hot Emit path from reconciler/HTTP goroutines
// while setEngine writes it once during setup, so it's an atomic.Pointer (race-
// free lock-free read; a pre-wire event simply misses the backfill sink).
type eventEmitter struct {
	engine atomic.Pointer[channels.Engine]
	bus    *events.Bus
}

// setEngine wires the scheduler backfill sink once the engine exists.
func (e *eventEmitter) setEngine(eng *channels.Engine) { e.engine.Store(eng) }

// Emit fans a terminal provisioning event to the scheduler backfill and the SSE
// bus. It never blocks on either (OnAvailability logs its own per-channel errors;
// Publish drops for slow subscribers).
func (e *eventEmitter) Emit(ctx context.Context, ev provision.DomainEvent) {
	if eng := e.engine.Load(); eng != nil {
		eng.OnAvailability(ctx, ev)
	}
	e.bus.Publish(events.Event{
		Type: "title",
		Payload: map[string]string{
			"key":   string(ev.Key),
			"state": string(ev.State),
			"name":  ev.Title.Name,
		},
	})
}
