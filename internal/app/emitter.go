package app

import (
	"context"
	"strconv"
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

// SuggestionPhase publishes one generation-progress frame (§8, type=suggestion)
// so the workspace's GenerationProgress advances searching→reasoning→scoring→
// done/failed live. Satisfies suggest.ProgressEmitter. Best-effort like the
// title sink: a dropped frame is a latency bug, never a correctness bug — GET
// /v1/suggestions/{id} is the source of truth on reconnect.
// round carries the tool-loop iteration so the UI can show a long run progressing;
// it is stringified because the SSE payload is a flat map[string]string, and 0
// (outside the loop) is sent as "0" rather than omitted so the frame shape never
// varies — a receiver reads one field, not a field that sometimes exists.
func (e *eventEmitter) SuggestionPhase(jobID, phase string, round int) {
	e.bus.Publish(events.Event{
		Type:    "suggestion",
		Payload: map[string]string{"jobId": jobID, "phase": phase, "round": strconv.Itoa(round)},
	})
}

// ChannelChanged publishes a `channel` frame after a reconcile so the Channels/detail
// pages update live — the "no manual rebuild" model (§9). Satisfies
// channels.ChannelNotifier. Best-effort: GET /v1/channels is the truth on load.
func (e *eventEmitter) ChannelChanged(channelID, status string) {
	e.bus.Publish(events.Event{
		Type:    "channel",
		Payload: map[string]string{"channelId": channelID, "status": status},
	})
}

// JobChanged publishes a `job` frame whenever a scheduled job's state changes (running →
// ok/error, Run-now) so the Settings → Tasks page updates live (§18.1). Satisfies
// scheduler.Notifier. The page refetches GET /v1/jobs on this frame — the BE stays the single
// source of timing truth. Best-effort like the other frames.
func (e *eventEmitter) JobChanged(name string) {
	e.bus.Publish(events.Event{
		Type:    "job",
		Payload: map[string]string{"name": name},
	})
}

// ActivityRecorded publishes an `activity` frame when a Dashboard feed row is written (§12,
// V32). Satisfies activity.Notifier.
//
// ⚠ **No payload.** The frame says "something happened"; the page refetches GET /v1/activity,
// which is the truth on reconnect (§8). Carrying the row itself would invite a client to
// render a list assembled from frames — and this bus drops frames for a slow subscriber by
// design, so that list would silently be missing entries.
func (e *eventEmitter) ActivityRecorded() {
	e.bus.Publish(events.Event{Type: "activity"})
}
