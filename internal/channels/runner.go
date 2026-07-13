package channels

import (
	"context"
	"log/slog"
	"time"

	"github.com/mantonx/loomarr/internal/store"
)

// Runner drives the periodic channel-reconcile sweep (§9 CHANNEL_RECONCILE_EVERY,
// §18). Each tick claims due channels (leased so replicas don't collide) and
// reconciles each — re-deriving desired from the store, revalidating slots, and
// pushing the minimal diff. This is what makes backfill crash-safe and
// multi-replica correct: events only reduce latency; the sweep is the guarantee.
type Runner struct {
	engine *Engine
	store  claimer
	every  time.Duration
	batch  int
	lease  time.Duration
	log    *slog.Logger
	now    func() time.Time
}

// claimer is the slice of the store the sweep needs (ClaimDueChannels). Narrowed
// to an interface so the runner is testable with a fake.
type claimer interface {
	ClaimDueChannels(ctx context.Context, now time.Time, lease time.Duration, limit int) ([]store.Channel, error)
}

// NewRunner wires the sweep loop.
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
	return &Runner{engine: e, store: st, every: every, batch: batch, lease: lease, log: log, now: now}
}

// Run blocks, ticking every `every` until ctx is done. Errors are logged, never
// fatal — a down Tunarr degrades freshness, never wedges the process (§6).
func (r *Runner) Run(ctx context.Context) {
	t := time.NewTicker(r.every)
	defer t.Stop()
	r.log.Info("channel sweep started", "every", r.every)
	r.Sweep(ctx) // run once immediately so startup recovery doesn't wait an interval
	for {
		select {
		case <-ctx.Done():
			r.log.Info("channel sweep stopped")
			return
		case <-t.C:
			r.Sweep(ctx)
		}
	}
}

// Sweep runs one pass: claim due channels and reconcile each. Returns the number
// of channels reconciled. Exposed for tests and for a manual kick.
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
