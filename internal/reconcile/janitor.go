package reconcile

import (
	"context"
	"log/slog"
	"time"
)

// Retention holds the §5 retention windows the janitor enforces. Zero means
// "keep" (a sweep target with no window is skipped).
type Retention struct {
	Sessions  time.Duration // SESSION_TTL — expired session rows (Phase 9)
	Jobs      time.Duration // JOBS_RETENTION — completed/failed jobs (Phase 11)
	Proposals time.Duration // PROPOSALS_RETENTION — denied/superseded (Phase 11)
}

// Sweeper purges one retention target. Each swept table registers a Sweeper as
// its owning phase lands (sessions → Phase 9, jobs/proposals → Phase 11), so the
// janitor mechanism exists now and later phases add behavior without reshaping
// the ticker (§5, §7).
type Sweeper interface {
	// Sweep removes rows older than the cutoff; returns how many it purged.
	Sweep(ctx context.Context, now time.Time) (int, error)
	// Name identifies the target in logs/metrics.
	Name() string
}

// Janitor runs the registered sweepers on the reconcile ticker (§5 piggyback).
type Janitor struct {
	sweepers []Sweeper
	log      *slog.Logger
	now      func() time.Time
}

// NewJanitor builds a janitor over zero or more sweepers.
func NewJanitor(log *slog.Logger, now func() time.Time, sweepers ...Sweeper) *Janitor {
	if now == nil {
		now = time.Now
	}
	return &Janitor{sweepers: sweepers, log: log, now: now}
}

// Sweep runs every registered sweeper once. A failing sweeper is logged and
// skipped — retention is best-effort and must never wedge the reconcile loop.
func (j *Janitor) Sweep(ctx context.Context) {
	now := j.now()
	for _, s := range j.sweepers {
		n, err := s.Sweep(ctx, now)
		if err != nil {
			j.log.Warn("janitor: sweep failed", "target", s.Name(), "err", err)
			continue
		}
		if n > 0 {
			j.log.Info("janitor: purged", "target", s.Name(), "count", n)
		}
	}
}
