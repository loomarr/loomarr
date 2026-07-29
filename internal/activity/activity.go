// Package activity records what Loomarr did, for the Dashboard's Recent activity feed
// (§5, §12, V32).
//
// ⚠ **Why this is not a tap on the event bus.** Loomarr already fans state changes out over
// SSE, and persisting there would be one line in one place. But `internal/events` is
// explicitly in-memory, single-process, and drops events for a slow subscriber — its own
// doc comment says "a dropped event is a latency bug, not a correctness bug", which is true
// for SSE precisely because the GET endpoints are the truth. A feed built on it would lose
// rows when the install is busiest, which is when an operator most wants to read it.
//
// It is also domain-NEUTRAL: the bus carries `{type:"title"}`, not "Darkwing Duck landed —
// CH 42 slot 05 backfilled in place". Only the subsystem making the change knows what
// happened, so each writes its own line through this recorder.
package activity

import (
	"context"
	"log/slog"

	"github.com/mantonx/loomarr/internal/store"
)

// Sink is the storage this package needs — accept-interfaces, so callers can pass a double
// without the store.
type Sink interface {
	RecordActivity(ctx context.Context, a store.Activity) error
}

// Notifier receives a signal after a row is written, for the SSE bus (optional).
//
// The frame carries NO payload beyond "something was written": the Dashboard re-reads
// GET /v1/activity, which is the truth on reconnect (§8). Pushing the row itself would
// invite a client to render from a frame it might have missed one of.
type Notifier interface{ ActivityRecorded() }

// Recorder writes feed rows. Nil-safe throughout: a subsystem constructed without one (unit
// tests, a store-less boot) records nothing rather than guarding at every call site.
type Recorder struct {
	sink   Sink
	log    *slog.Logger
	notify Notifier
}

// New builds a Recorder. A nil sink yields a recorder whose writes are no-ops.
func New(sink Sink, log *slog.Logger) *Recorder {
	return &Recorder{sink: sink, log: log}
}

// WithNotifier wires the SSE bus so the Dashboard learns of a new row without polling
// (§7 `activity` frame). Chainable, matching the scheduler's WithNotifier.
//
// ⚠ The feed takes a frame precisely BECAUSE it is event-shaped — the server knows at write
// time and GET /v1/activity is authoritative on reconnect. The Services panel next to it
// polls instead, because a probe result is not an event anyone observes: the server only
// learns it by asking, so a "push" would just be a server-side timer probing forever.
func (r *Recorder) WithNotifier(n Notifier) *Recorder { r.notify = n; return r }

// Record appends one row.
//
// ⚠ **Best-effort by contract, like RecordAiring.** Callers write from inside a real
// operation — a title landing, a channel reconciling — so a failure here is logged and
// swallowed. Recording that something happened must never be able to stop it happening,
// which is why this returns nothing at all: there is no error a caller should act on, and
// returning one would invite a caller to treat telemetry as load-bearing.
func (r *Recorder) Record(ctx context.Context, a store.Activity) {
	if r == nil || r.sink == nil {
		return
	}
	if err := r.sink.RecordActivity(ctx, a); err != nil {
		if r.log != nil {
			r.log.Warn("activity not recorded", "kind", a.Kind, "err", err)
		}
		return // nothing was written, so there is nothing to announce
	}
	if r.notify != nil {
		r.notify.ActivityRecorded()
	}
}

// Info/Warn/Error are the three levels the UI renders a dot for. Helpers rather than raw
// Record calls so a write point states its severity in one word and cannot invent a fourth
// level the frontend has no colour for.
func (r *Recorder) Info(ctx context.Context, kind, subjectID, text string) {
	r.Record(ctx, store.Activity{Kind: kind, Level: store.ActivityInfo, Text: text, SubjectID: subjectID})
}

func (r *Recorder) Warn(ctx context.Context, kind, subjectID, text string) {
	r.Record(ctx, store.Activity{Kind: kind, Level: store.ActivityWarn, Text: text, SubjectID: subjectID})
}

func (r *Recorder) Error(ctx context.Context, kind, subjectID, text string) {
	r.Record(ctx, store.Activity{Kind: kind, Level: store.ActivityError, Text: text, SubjectID: subjectID})
}
