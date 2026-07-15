package reconcile

import (
	"context"
	"log/slog"
	"time"
)

// Runner drives the reconcile + janitor loop on a ticker (§7). It runs until the
// context is cancelled (graceful shutdown), draining in-flight work.
type Runner struct {
	rec     *Reconciler
	janitor *Janitor
	every   time.Duration
	// intervalFn, when set, is re-read after each tick so a changed RECONCILE_EVERY
	// setting hot-applies next cycle (config-design §3 "intervals re-read per tick").
	// nil ⇒ the fixed `every`. The result is floored to avoid a hot-spin.
	intervalFn func() time.Duration
	log        *slog.Logger
}

// NewRunner wires the loop. janitor may be nil (no retention targets yet).
func NewRunner(rec *Reconciler, janitor *Janitor, every time.Duration, log *slog.Logger) *Runner {
	if every <= 0 {
		every = 5 * time.Minute
	}
	return &Runner{rec: rec, janitor: janitor, every: every, log: log}
}

// WithInterval sets a provider read after every tick so the interval hot-applies
// (config-design §3). Returns the runner for chaining. A provider returning ≤0
// keeps the last good interval.
func (r *Runner) WithInterval(fn func() time.Duration) *Runner {
	r.intervalFn = fn
	return r
}

// Run blocks, ticking every `every` until ctx is done. Each tick runs one
// reconcile pass then a janitor sweep. Errors are logged, never fatal — a
// down dependency degrades the loop, never wedges the process (§6). If an
// interval provider is set, the ticker is re-tuned per cycle (hot-apply).
func (r *Runner) Run(ctx context.Context) {
	t := time.NewTicker(r.every)
	defer t.Stop()
	r.log.Info("reconciler started", "every", r.every)

	// Run once immediately so startup recovery doesn't wait a full interval.
	r.once(ctx)
	for {
		select {
		case <-ctx.Done():
			r.log.Info("reconciler stopped")
			return
		case <-t.C:
			r.once(ctx)
			r.retune(t)
		}
	}
}

// retune adjusts the ticker to the current interval setting (config-design §3
// hot-apply). No-op when no provider is set or the value is unchanged/invalid.
func (r *Runner) retune(t *time.Ticker) {
	if r.intervalFn == nil {
		return
	}
	next := r.intervalFn()
	if next > 0 && next != r.every {
		r.every = next
		t.Reset(next)
		r.log.Info("reconciler interval changed", "every", next)
	}
}

func (r *Runner) once(ctx context.Context) {
	n, err := r.rec.Tick(ctx)
	if err != nil {
		r.log.Error("reconcile tick", "err", err)
	} else if n > 0 {
		r.log.Debug("reconcile tick", "reconciled", n)
	}
	if r.janitor != nil {
		r.janitor.Sweep(ctx)
	}
}
