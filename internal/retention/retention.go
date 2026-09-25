// Package retention owns the scheduled purges that keep the accumulating tables bounded
// (§5, §18.1): finished jobs, denied proposals, and old activity/notification rows.
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
	"errors"
	"log/slog"
	"time"

	"github.com/loomarr/loomarr/internal/diagnostics"
	"github.com/loomarr/loomarr/internal/invitation"
	"github.com/loomarr/loomarr/internal/notifications"
	"github.com/loomarr/loomarr/internal/recovery"
	"github.com/loomarr/loomarr/internal/scheduler"
)

// Windows are the configured retention periods, read per run.
type Windows struct {
	Proposals           func() time.Duration
	Jobs                func() time.Duration
	Activity            func() time.Duration
	Diagnostics         func() time.Duration
	DiagnosticsMaxBytes func() int64
}

// Store is the destructive persistence role behind retention policy. The
// composite store remains at the composition root; this module can delete only
// the record classes its interface names.
type Store interface {
	PurgeDeniedProposals(ctx context.Context, before time.Time) (int, error)
	PurgeFinishedJobs(ctx context.Context, before time.Time) (int, error)
	PurgeActivity(ctx context.Context, before time.Time) (int, error)
	PurgeTerminalInvitations(ctx context.Context, before time.Time) (int, error)
	PurgeTerminalNotifications(ctx context.Context, before time.Time) (int, error)
	PurgeTerminalPasswordRecoveries(ctx context.Context, before time.Time) (int, error)
	PurgeDiagnostics(ctx context.Context, before time.Time, maxBytes int64) (diagnostics.PurgeResult, error)
	MaintainQualityLedger(ctx context.Context, now time.Time) error
	PurgeExpiredSessions(ctx context.Context, now time.Time) (int, error)
}

// Service runs the purges.
type Service struct {
	store       Store
	diagnostics interface {
		Purge(context.Context, time.Time, int64) (diagnostics.PurgeResult, error)
	}
	win Windows
	now func() time.Time
	log *slog.Logger
}

// WithDiagnostics assigns the filesystem-aware purge coordinator. The store-only fallback keeps
// narrow tests and store-less adapters useful, but production always supplies ProcessManager.
func (s *Service) WithDiagnostics(purger interface {
	Purge(context.Context, time.Time, int64) (diagnostics.PurgeResult, error)
}) *Service {
	s.diagnostics = purger
	return s
}

// New builds the retention service. A nil `now` means time.Now.
func New(st Store, win Windows, now func() time.Time, log *slog.Logger) *Service {
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
//
// The purges below return the rows they deleted so Housekeeping can report one per-run total.
func (s *Service) PurgeRecords(ctx context.Context) (int, error) {
	now := s.now()
	proposals, err := s.store.PurgeDeniedProposals(ctx, now.Add(-s.win.Proposals()))
	if err != nil {
		return 0, err
	}
	jobs, err := s.store.PurgeFinishedJobs(ctx, now.Add(-s.win.Jobs()))
	if err != nil {
		return proposals, err
	}
	if (proposals > 0 || jobs > 0) && s.log != nil {
		s.log.Info("retention purge", "denied_proposals", proposals, "finished_jobs", jobs)
	}
	return proposals + jobs, nil
}

// PurgeActivity removes activity rows past their window (§32).
func (s *Service) PurgeActivity(ctx context.Context) (int, error) {
	n, err := s.store.PurgeActivity(ctx, s.now().Add(-s.win.Activity()))
	if err != nil {
		return 0, err
	}
	if n > 0 && s.log != nil {
		s.log.Info("activity purged", "rows", n)
	}
	return n, nil
}

// PurgeNotifications applies the fixed §11 product policy. It is deliberately not a setting:
// terminal notification evidence is retained for 30 days while queued/sending work is exempt.
func (s *Service) PurgeNotifications(ctx context.Context) (int, error) {
	n, err := s.store.PurgeTerminalNotifications(ctx, s.now().Add(-notifications.Retention))
	if err != nil {
		return 0, err
	}
	if n > 0 && s.log != nil {
		s.log.Info("terminal notifications purged", "intents", n)
	}
	return n, nil
}

// PurgeInvitations applies the fixed §11 audit window to terminal and long-expired admission
// decisions. Active Invitations remain available for redemption.
func (s *Service) PurgeInvitations(ctx context.Context) (int, error) {
	n, err := s.store.PurgeTerminalInvitations(ctx, s.now().Add(-invitation.Retention))
	if err != nil {
		return 0, err
	}
	if n > 0 && s.log != nil {
		s.log.Info("terminal invitations purged", "invitations", n)
	}
	return n, nil
}

// PurgePasswordRecoveries applies the same fixed 30-day account-security audit window.
func (s *Service) PurgePasswordRecoveries(ctx context.Context) (int, error) {
	n, err := s.store.PurgeTerminalPasswordRecoveries(ctx, s.now().Add(-recovery.Retention))
	if err != nil {
		return 0, err
	}
	if n > 0 && s.log != nil {
		s.log.Info("terminal password recoveries purged", "recoveries", n)
	}
	return n, nil
}

// PurgeDiagnostics enforces both the age window and logical retained-byte budget (§5, §17).
// Active Process runs are protected by the store contract regardless of age or pressure.
//
// The result is returned EVEN ON ERROR: the purge works in batches, so a run cut short by its
// deadline has still deleted real rows, and the caller's summary must not report zero for them.
func (s *Service) PurgeDiagnostics(ctx context.Context) (diagnostics.PurgeResult, error) {
	before, maxBytes := s.now().Add(-s.win.Diagnostics()), s.win.DiagnosticsMaxBytes()
	var result diagnostics.PurgeResult
	var err error
	if s.diagnostics != nil {
		result, err = s.diagnostics.Purge(ctx, before, maxBytes)
	} else {
		result, err = s.store.PurgeDiagnostics(ctx, before, maxBytes)
	}
	if (result.Events > 0 || result.ProcessRuns > 0) && s.log != nil {
		s.log.Info("diagnostics purged", "events", result.Events, "process_runs", result.ProcessRuns,
			"retained_bytes", result.RetainedBytes)
	}
	return result, err
}

// Housekeeping bounds every append-only operational table in one pass. The individual
// purges remain methods because they are independently testable policies, but separate
// schedules offered no useful operator decision and made routine cleanup look like three
// unrelated features.
//
// It ends with exactly one INFO line — rows deleted and duration — whether or not a stage failed,
// so a night that deleted a million rows and then timed out is visible as such (#1411).
func (s *Service) Housekeeping(ctx context.Context) error {
	started := s.now()
	var errs []error
	rows := 0
	count := func(n int, err error) {
		rows += n
		if err != nil {
			errs = append(errs, err)
		}
	}
	count(s.PurgeRecords(ctx))
	count(s.PurgeActivity(ctx))
	count(s.PurgeInvitations(ctx))
	count(s.PurgePasswordRecoveries(ctx))
	count(s.PurgeNotifications(ctx))
	diag, err := s.PurgeDiagnostics(ctx)
	count(diag.Events+diag.ProcessRuns, err)
	if err := s.store.MaintainQualityLedger(ctx, s.now()); err != nil {
		errs = append(errs, err)
	}
	if n, err := s.store.PurgeExpiredSessions(ctx, s.now()); err != nil {
		errs = append(errs, err)
	} else {
		rows += n
		if n > 0 && s.log != nil {
			s.log.Info("expired sessions purged", "rows", n)
		}
	}
	if s.log != nil {
		s.log.Info("housekeeping finished", "rows_deleted", rows,
			"diagnostic_events", diag.Events, "diagnostic_process_runs", diag.ProcessRuns,
			"duration_ms", s.now().Sub(started).Milliseconds(), "failed_stages", len(errs))
	}
	return errors.Join(errs...)
}

// housekeepingTimeout is the ceiling for one pass. ⚠ It MUST be declared: a job with no Timeout
// runs under River's inherited one-minute JobTimeout, which killed the nightly pass at 08:31 on
// 2026-09-22/23/24 ("delete diagnostic event ...: context deadline exceeded") before it could
// drain a 2.7M-row diagnostic_events table (#1411). The pass now works in short batches that
// yield the database connection, so a generous ceiling cannot starve anything else; it exists
// only so a genuinely wedged pass is still cut off.
const housekeepingTimeout = 25 * time.Minute

// Job returns the single daily system-housekeeping task (§18.1).
func (s *Service) Job() scheduler.Job {
	return scheduler.Job{
		Name: "housekeeping", Group: scheduler.GroupSystem, Title: "Clean up old data",
		Description: "Rolls up discovery-quality observations and removes expired sessions, old activity, invitations, password recoveries, diagnostics and notifications, denied requests, and completed jobs after their retention periods.",
		DefaultCron: "0 30 4 * * *", ScheduleKey: "job.housekeeping.schedule",
		Timeout: housekeepingTimeout,
		Run:     s.Housekeeping,
	}
}
