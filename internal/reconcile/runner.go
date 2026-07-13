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
	log     *slog.Logger
}

// NewRunner wires the loop. janitor may be nil (no retention targets yet).
func NewRunner(rec *Reconciler, janitor *Janitor, every time.Duration, log *slog.Logger) *Runner {
	if every <= 0 {
		every = 5 * time.Minute
	}
	return &Runner{rec: rec, janitor: janitor, every: every, log: log}
}

// Run blocks, ticking every `every` until ctx is done. Each tick runs one
// reconcile pass then a janitor sweep. Errors are logged, never fatal — a
// down dependency degrades the loop, never wedges the process (§6).
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
		}
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
