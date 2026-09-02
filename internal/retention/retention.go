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

// PurgeNotifications applies the fixed §11 product policy. It is deliberately not a setting:
// terminal notification evidence is retained for 30 days while queued/sending work is exempt.
func (s *Service) PurgeNotifications(ctx context.Context) error {
	n, err := s.store.PurgeTerminalNotifications(ctx, s.now().Add(-notifications.Retention))
	if err != nil {
		return err
	}
	if n > 0 && s.log != nil {
		s.log.Info("terminal notifications purged", "intents", n)
	}
	return nil
}

// PurgeInvitations applies the fixed §11 audit window to terminal and long-expired admission
// decisions. Active Invitations remain available for redemption.
func (s *Service) PurgeInvitations(ctx context.Context) error {
	n, err := s.store.PurgeTerminalInvitations(ctx, s.now().Add(-invitation.Retention))
	if err != nil {
		return err
	}
	if n > 0 && s.log != nil {
		s.log.Info("terminal invitations purged", "invitations", n)
	}
	return nil
}

// PurgePasswordRecoveries applies the same fixed 30-day account-security audit window.
func (s *Service) PurgePasswordRecoveries(ctx context.Context) error {
	n, err := s.store.PurgeTerminalPasswordRecoveries(ctx, s.now().Add(-recovery.Retention))
	if err != nil {
		return err
	}
	if n > 0 && s.log != nil {
		s.log.Info("terminal password recoveries purged", "recoveries", n)
	}
	return nil
}

// PurgeDiagnostics enforces both the age window and logical retained-byte budget (§5, §17).
// Active Process runs are protected by the store contract regardless of age or pressure.
func (s *Service) PurgeDiagnostics(ctx context.Context) error {
	before, maxBytes := s.now().Add(-s.win.Diagnostics()), s.win.DiagnosticsMaxBytes()
	var result diagnostics.PurgeResult
	var err error
	if s.diagnostics != nil {
		result, err = s.diagnostics.Purge(ctx, before, maxBytes)
	} else {
		result, err = s.store.PurgeDiagnostics(ctx, before, maxBytes)
	}
	if err != nil {
		return err
	}
	if (result.Events > 0 || result.ProcessRuns > 0) && s.log != nil {
		s.log.Info("diagnostics purged", "events", result.Events, "process_runs", result.ProcessRuns,
			"retained_bytes", result.RetainedBytes)
	}
	return nil
}

// Housekeeping bounds every append-only operational table in one pass. The individual
// purges remain methods because they are independently testable policies, but separate
// schedules offered no useful operator decision and made routine cleanup look like three
// unrelated features.
func (s *Service) Housekeeping(ctx context.Context) error {
	var errs []error
	if err := s.PurgeRecords(ctx); err != nil {
		errs = append(errs, err)
	}
	if err := s.PurgeActivity(ctx); err != nil {
		errs = append(errs, err)
	}
	if err := s.PurgeInvitations(ctx); err != nil {
		errs = append(errs, err)
	}
	if err := s.PurgePasswordRecoveries(ctx); err != nil {
		errs = append(errs, err)
	}
	if err := s.PurgeNotifications(ctx); err != nil {
		errs = append(errs, err)
	}
	if err := s.PurgeDiagnostics(ctx); err != nil {
		errs = append(errs, err)
	}
	if err := s.store.MaintainQualityLedger(ctx, s.now()); err != nil {
		errs = append(errs, err)
	}
	if n, err := s.store.PurgeExpiredSessions(ctx, s.now()); err != nil {
		errs = append(errs, err)
	} else if n > 0 && s.log != nil {
		s.log.Info("expired sessions purged", "rows", n)
	}
	return errors.Join(errs...)
}

// Job returns the single daily system-housekeeping task (§18.1).
func (s *Service) Job() scheduler.Job {
	return scheduler.Job{
		Name: "housekeeping", Group: scheduler.GroupSystem, Title: "Clean up old data",
		Description: "Rolls up discovery-quality observations and removes expired sessions, old activity, invitations, password recoveries, diagnostics and notifications, denied requests, and completed jobs after their retention periods.",
		DefaultCron: "0 30 4 * * *", ScheduleKey: "job.housekeeping.schedule",
		Run: s.Housekeeping,
	}
}
