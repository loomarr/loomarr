// Package retention owns the scheduled purges that keep the accumulating tables bounded
// (§5, §18.1): finished jobs, denied proposals, and old activity rows.
//
// ⚠ **Why its own package rather than a method on the store.** The purge is a POLICY — what
// may be deleted, in what order, and after how long — while the store owns the SQL that
// executes it. Keeping them apart is also what makes this compile at all: `scheduler`
// imports `store`, so `store` can never import `scheduler`, and a job declared next to the
// SQL would be an import cycle.
//
// The retention windows are read live (a `func() time.Duration`), so an operator changing
// `proposals.retention` in Settings takes effect on the next run with no restart
// (config-design §3).
package retention

import (
	"context"
	"log/slog"
	"time"

	"github.com/mantonx/loomarr/internal/scheduler"
	"github.com/mantonx/loomarr/internal/store"
)

// Windows are the configured retention periods, read per run.
type Windows struct {
	Proposals func() time.Duration
	Jobs      func() time.Duration
	Activity  func() time.Duration
}

// Service runs the purges.
type Service struct {
	store store.Store
	win   Windows
	now   func() time.Time
	log   *slog.Logger
}

// New builds the retention service. A nil `now` means time.Now.
func New(st store.Store, win Windows, now func() time.Time, log *slog.Logger) *Service {
	if now == nil {
		now = time.Now
	}
	return &Service{store: st, win: win, now: now, log: log}
}

// PurgeRecords removes denied proposals and finished jobs past their windows.
//
// ⚠ **PROPOSALS FIRST, THEN JOBS.** `proposals.job_id` has no foreign key, so the ordering
// constraint is ours to keep: removing a job first would leave a proposal pointing at
// nothing. No read path joins the two, so an orphan is cosmetic — but a purge that
// manufactures one on every run makes the data harder to reason about for no gain.
//
// ⚠ What it must NOT remove is the substance: `approved` proposals are the audit trail
// behind `approved_by`, `submitted` ones are members still waiting for an answer, and
// `queued`/`running` jobs are not finished (age is not evidence work completed). Those
// exemptions live in the store's WHERE clauses and are pinned by the conformance suite.
func (s *Service) PurgeRecords(ctx context.Context) error {
	now := s.now()
	proposals, err := s.store.PurgeDeniedProposals(ctx, now.Add(-s.win.Proposals()))
	if err != nil {
		return err
	}
	jobs, err := s.store.PurgeFinishedJobs(ctx, now.Add(-s.win.Jobs()))
	if err != nil {
		return err
	}
	if (proposals > 0 || jobs > 0) && s.log != nil {
		s.log.Info("retention purge", "denied_proposals", proposals, "finished_jobs", jobs)
	}
	return nil
}

// PurgeActivity removes activity rows past their window (§32).
func (s *Service) PurgeActivity(ctx context.Context) error {
	n, err := s.store.PurgeActivity(ctx, s.now().Add(-s.win.Activity()))
	if err != nil {
		return err
	}
	if n > 0 && s.log != nil {
		s.log.Info("activity purged", "rows", n)
	}
	return nil
}

// Jobs returns both purges as scheduler jobs (§18.1).
//
// ⚠ Deliberately staggered rather than sharing a cron: two full-table deletes firing in the
// same second is avoidable contention for no benefit, and 04:15/04:30 keeps them out of the
// 03:00 backup's way.
func (s *Service) Jobs() []scheduler.Job {
	return []scheduler.Job{
		{
			Name: "retention-purge", Title: "Clean up old jobs and proposals",
			Description: "Deletes finished job records and denied requests once they are older than your retention settings. Approved requests and anything still running are kept.",
			DefaultCron: "0 30 4 * * *", ScheduleKey: "job.retention_purge.schedule",
			Run: s.PurgeRecords,
		},
		{
			Name: "activity-purge", Title: "Clean up old activity",
			Description: "Trims the dashboard activity feed to the age you configured, so it stays quick to read.",
			DefaultCron: "0 15 4 * * *", ScheduleKey: "job.activity_purge.schedule",
			Run: s.PurgeActivity,
		},
	}
}
