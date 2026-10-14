package app

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"github.com/loomarr/loomarr/internal/api"
	"github.com/loomarr/loomarr/internal/channels"
	"github.com/loomarr/loomarr/internal/events"
	"github.com/loomarr/loomarr/internal/filler"
	"github.com/loomarr/loomarr/internal/provision"
	"github.com/loomarr/loomarr/internal/store"
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
	engine        atomic.Pointer[channels.Engine]
	notifications atomic.Pointer[productNotificationCoordinator]
	bus           *events.Bus

	// clipSeen throttles intra-stage filler progress, keyed by clip hash. See FillerClipStage.
	clipMu   sync.Mutex
	clipSeen map[string]clipFrameMark
}

// clipFrameMark is the last `filler_clip` frame published for one clip.
type clipFrameMark struct {
	at  time.Time
	pct int
}

// setEngine wires the scheduler backfill sink once the engine exists.
func (e *eventEmitter) setEngine(eng *channels.Engine) { e.engine.Store(eng) }
func (e *eventEmitter) setNotifications(coordinator *productNotificationCoordinator) {
	e.notifications.Store(coordinator)
}

func (e *eventEmitter) ProposalSubmitted(ctx context.Context, proposal store.Proposal) {
	if coordinator := e.notifications.Load(); coordinator != nil {
		coordinator.ProposalSubmitted(ctx, proposal)
	}
}

func (e *eventEmitter) ProposalApproved(ctx context.Context, proposal store.Proposal, channelID string) {
	if coordinator := e.notifications.Load(); coordinator != nil {
		coordinator.ProposalApproved(ctx, proposal, channelID)
	}
}

func (e *eventEmitter) ProposalDeclined(ctx context.Context, proposal store.Proposal) {
	if coordinator := e.notifications.Load(); coordinator != nil {
		coordinator.ProposalDeclined(ctx, proposal)
	}
}

// Emit fans a terminal provisioning event to the scheduler backfill and the SSE
// bus. It never blocks on either (OnAvailability logs its own per-channel errors;
// Publish drops for slow subscribers).
func (e *eventEmitter) Emit(ctx context.Context, ev provision.DomainEvent) {
	if eng := e.engine.Load(); eng != nil {
		eng.OnAvailability(ctx, ev)
	}
	e.bus.Publish(events.Event{
		Type:    "title",
		Payload: api.TitleEvent{Key: string(ev.Key), State: string(ev.State), Name: ev.Title.Name},
	})
	if coordinator := e.notifications.Load(); coordinator != nil {
		coordinator.Provisioned(ctx, ev)
	}
}

// SuggestionPhase publishes one generation-progress frame (§8, type=suggestion)
// so the workspace's GenerationProgress advances searching→reasoning→scoring→
// done/failed live. Satisfies suggest.ProgressEmitter. Best-effort like the
// title sink: a dropped frame is a latency bug, never a correctness bug — GET
// /v1/proposals/{id} is the source of truth on reconnect.
// round carries the tool-loop iteration so the UI can show a long run progressing. It is a
// real int now: it was stringified only because the payload used to be a flat
// map[string]string, and the frontend mirror carried a warning that declaring it a number
// there "would typecheck and then compare wrong at runtime". A typed frame removes the
// hazard instead of documenting it. 0 (outside the loop) is still sent rather than omitted,
// so the frame shape never varies.
func (e *eventEmitter) SuggestionPhase(jobID, phase string, round int) {
	e.bus.Publish(events.Event{
		Type:    "suggestion",
		Payload: api.SuggestionEvent{JobID: jobID, Phase: phase, Round: round},
	})
}

// ChannelChanged publishes a `channel` frame after a reconcile so the Channels/detail
// pages update live — the "no manual rebuild" model (§9). Satisfies
// channels.ChannelNotifier. Best-effort: GET /v1/channels is the truth on load.
func (e *eventEmitter) ChannelChanged(channelID, previousStatus, status string) {
	e.bus.Publish(events.Event{
		Type:    "channel",
		Payload: api.ChannelEvent{ChannelID: channelID, Status: status},
	})
	if coordinator := e.notifications.Load(); coordinator != nil {
		coordinator.ChannelChanged(channelID, previousStatus, status)
	}
}

// JobChanged publishes a `job` frame whenever a scheduled job's state changes (running →
// ok/error, Run-now) so the Settings → Tasks page updates live (§18.1). Satisfies
// scheduler.Notifier. The page refetches GET /v1/jobs on this frame — the BE stays the single
// source of timing truth. Best-effort like the other frames.
func (e *eventEmitter) JobChanged(name string) {
	e.bus.Publish(events.Event{Type: "job", Payload: api.JobEvent{Name: name}})
}

// ActivityRecorded publishes an `activity` frame when a Dashboard feed row is written (§12,
// V32). Satisfies activity.Notifier.
//
// ⚠ **No payload.** The frame says "something happened"; the page refetches GET /v1/activity,
// which is the truth on reconnect (§8). Carrying the row itself would invite a client to
// render a list assembled from frames — and this bus drops frames for a slow subscriber by
// design, so that list would silently be missing entries.
func (e *eventEmitter) ActivityRecorded() {
	// An explicit empty STRUCT rather than a nil payload. `json.Marshal(nil)` yields the
	// literal `null`, which the browser's JSON.parse accepts today — but the client wraps
	// every frame's parse in a try/catch that SWALLOWS failures (a latency bus is never
	// load-bearing), so a future change making that unparseable would disable this frame
	// silently, with no error anywhere. It must also be a NAMED type: huma reads the event
	// name off the payload's Go type, and a bare map has no name to read.
	e.bus.Publish(events.Event{Type: "activity", Payload: api.ActivityEvent{}})
}

// HealthChanged invalidates Current Health after a material check-state transition. It carries no
// mutable report payload; GET /v1/diagnostics/health remains the source of truth.
func (e *eventEmitter) HealthChanged() {
	e.bus.Publish(events.Event{Type: "health", Payload: api.HealthEvent{}})
}

// clipFrameMinGap / clipFrameMinPoints are the throttle for intra-stage filler progress.
//
// ⚠ **Throttled HERE rather than at the source, which is the call `PlayoutEvent` already
// documents for ffmpeg progress samples.** ffmpeg emits about once a second for the duration of a
// transcode; republishing every sample would push a frame a second at every open browser for a
// number that moves by fractions. The source stays honest (it reports what it measures) and the
// fan-out decides what is worth sending.
const (
	clipFrameMinGap    = time.Second
	clipFrameMinPoints = 5
)

// FillerClipStage publishes a `filler_clip` frame for one clip's pipeline state (§10 V51b).
// Satisfies filler.Notifier.
//
// ⚠ **A status TRANSITION always publishes; only intra-stage percentages are throttled.** What a
// watching operator must not miss is a rung starting, finishing, or refusing a clip — those are
// the events the row is made of. The percentage between them is decoration, and dropping some of
// it costs a slightly jumpy bar. Getting this backwards would silently hide the rejection frame
// behind a rate limit.
func (e *eventEmitter) FillerClipStage(p filler.ClipPipeline, c filler.StoreClip) {
	if p.Status == filler.StatusRunning && p.Disposition == filler.DispositionRunning {
		if !e.shouldPublishClipFrame(p.ClipHash, p.Progress) {
			return
		}
	} else {
		// A terminal or transitional frame bypasses the throttle AND clears the clip's mark, so
		// the next stage's first progress sample is never suppressed for being close to the last
		// stage's final one.
		e.forgetClipFrame(p.ClipHash)
	}
	// ⚠ The payload is composed INLINE rather than in a local, deliberately. The parity guard
	// (`TestEveryPublishedEventIsInTheEventTypeMap`) reads these publish sites as SOURCE — there
	// is no runtime seam that can see a frame which has not fired yet — and its regex matches
	// `Type: "…", Payload: api.X{`. Hoisting the struct into a variable hides the frame from the
	// guard, which then reports it as declared-but-never-published.
	e.bus.Publish(events.Event{Type: "filler_clip", Payload: api.FillerClipEvent{
		Hash: p.ClipHash, Stage: string(p.Stage), Status: string(p.Status),
		Progress: p.Progress, Disposition: string(p.Disposition),
		Reason: string(p.RejectReason), Detail: p.RejectDetail, Name: c.Name,
	}})
}

func (e *eventEmitter) shouldPublishClipFrame(hash string, pct int) bool {
	now := time.Now()
	e.clipMu.Lock()
	defer e.clipMu.Unlock()
	if e.clipSeen == nil {
		e.clipSeen = map[string]clipFrameMark{}
	}
	last, seen := e.clipSeen[hash]
	if seen && now.Sub(last.at) < clipFrameMinGap && pct-last.pct < clipFrameMinPoints {
		return false
	}
	e.clipSeen[hash] = clipFrameMark{at: now, pct: pct}
	return true
}

// forgetClipFrame drops a clip's throttle mark.
//
// ⚠ It is also what stops the map growing without bound. Every clip reaches a terminal or
// transitional frame — that is what the pipeline does — so an entry's lifetime is one stage, not
// the process's. A map keyed by clip hash and never pruned would be a slow leak in a long-running
// appliance that ingests thousands of clips.
func (e *eventEmitter) forgetClipFrame(hash string) {
	e.clipMu.Lock()
	delete(e.clipSeen, hash)
	e.clipMu.Unlock()
}
