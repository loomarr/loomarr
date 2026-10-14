package channels

import (
	"context"
	"log/slog"
	"time"

	"github.com/loomarr/loomarr/internal/store"
)

// Runner performs the channel-reconcile SWEEP (§9, §18): claim due channels (leased so
// replicas don't collide) and reconcile each — re-deriving desired from the store,
// revalidating slots, and projecting the minimal diff when Tunarr owns playout. This is what
// makes backfill crash-safe and multi-replica correct: events only reduce latency; the sweep
// is the guarantee for internal and Tunarr-backed channels alike.
//
// The loop that drove it is gone — the sweep is now a scheduler job (§18.1). This type keeps
// the lease/batch config and the Sweep pass the scheduler calls.
type Runner struct {
	engine *Engine
	store  claimer
	batch  int
	lease  time.Duration
	log    *slog.Logger
	now    func() time.Time
}

// claimer is the slice of the store the sweep needs (ClaimDueChannels). Narrowed
// to an interface so the sweep is testable with a fake.
type claimer interface {
	ClaimDueChannels(ctx context.Context, now time.Time, lease time.Duration, limit int) ([]store.Channel, error)
}

// NewRunner builds the sweep. `every` sizes the default lease (a claimed channel stays leased
// across a slow reconcile); the scheduler owns cadence, so `every` is not a loop interval here.
func NewRunner(e *Engine, st claimer, every, lease time.Duration, batch int, now func() time.Time, log *slog.Logger) *Runner {
	if every <= 0 {
		every = 10 * time.Minute
	}
	if lease <= 0 {
		lease = 2 * every // a claimed channel stays leased across a slow reconcile
	}
	if batch <= 0 {
		batch = 50
	}
	if now == nil {
		now = time.Now
	}
	return &Runner{engine: e, store: st, batch: batch, lease: lease, log: log, now: now}
}

// Sweep runs one pass: claim due channels and reconcile each. Returns the number of channels
// reconciled. Called by the scheduler's channel-maintenance job (§18.1). Errors are logged, never
// fatal — one channel or selected backend failing never wedges the process (§6).
func (r *Runner) Sweep(ctx context.Context) int {
	claimed, err := r.store.ClaimDueChannels(ctx, r.now(), r.lease, r.batch)
	if err != nil {
		r.log.Error("channel sweep: claim due", "err", err)
		return 0
	}
	n := 0
	for _, ch := range claimed {
		if err := r.engine.Reconcile(ctx, ch.ID); err != nil {
			r.log.Error("channel sweep reconcile", "channel", ch.ID, "err", err)
			continue
		}
		n++
	}
	if n > 0 {
		r.log.Debug("channel sweep", "reconciled", n)
	}
	return n
}
