// Package housekeeping owns the scheduled purges that keep the accumulating tables bounded
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
package housekeeping

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/loomarr/loomarr/internal/people/access"
	"github.com/loomarr/loomarr/internal/people/notifications"
	"github.com/loomarr/loomarr/internal/platform/diagnostics"
	"github.com/loomarr/loomarr/internal/platform/jobs"
	"github.com/loomarr/loomarr/internal/platform/settings"
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
	PurgeDiscardedProposals(ctx context.Context, before time.Time) (int, error)
	PurgeExpiredDevicePairings(ctx context.Context, now time.Time) (int, error)
	PurgeFinishedInteractiveOperations(ctx context.Context, before time.Time) (int, error)
	PurgeSupersededDiscoveryFeedback(ctx context.Context) (int, error)
	PurgeOrphanNotificationReferenceRecipients(ctx context.Context, before time.Time) (int, error)
	PurgeSpentFillerRecords(ctx context.Context, now time.Time) (int, error)
	PurgeUnusedTitles(ctx context.Context, before time.Time) (int, error)
	PurgeSpentCatalog(ctx context.Context, now time.Time) (int, error)
	PurgeFinishedJobs(ctx context.Context, before time.Time) (int, error)
	PurgeActivity(ctx context.Context, before time.Time) (int, error)
	PurgeTerminalInvitations(ctx context.Context, before time.Time) (int, error)
	PurgeTerminalNotifications(ctx context.Context, before time.Time) (int, error)
	PurgeTerminalPasswordRecoveries(ctx context.Context, before time.Time) (int, error)
	PurgeDiagnostics(ctx context.Context, before time.Time, maxBytes int64) (diagnostics.PurgeResult, error)
	PurgeClientCrashReports(ctx context.Context, before time.Time, keep int) (int, error)
	MaintainQualityLedger(ctx context.Context, now time.Time) error
	PurgeExpiredSessions(ctx context.Context, now time.Time) (int, error)
	RecordTableRows(ctx context.Context, at, before time.Time) (int, error)
}

// tableRowHistory is how long the nightly row counts are kept: over a year, so a table's growth
// can be compared with the same season last year.
const tableRowHistory = 400 * 24 * time.Hour

// Service runs the purges.
type Service struct {
	store       Store
	diagnostics interface {
		Purge(context.Context, time.Time, int64) (diagnostics.PurgeResult, error)
	}
	tidy func(context.Context) (freedBytes int64, err error) // nil on Postgres, which tidies itself
	win  Windows
	now  func() time.Time
	log  *slog.Logger
}

// WithTidier ends each pass by handing the space its purges freed back to the disk (SQLite only,
// #2446). Without it a SQLite file keeps its largest-ever size however much is deleted.
func (s *Service) WithTidier(tidy func(context.Context) (freedBytes int64, err error)) *Service {
	s.tidy = tidy
	return s
}

// WithDiagnostics assigns the filesystem-aware purge coordinator. The store-only fallback keeps
// narrow tests and store-less adapters useful, but production always supplies ProcessManager.
func (s *Service) WithDiagnostics(purger interface {
	Purge(context.Context, time.Time, int64) (diagnostics.PurgeResult, error)
}) *Service {
	s.diagnostics = purger
	return s
}

// New builds the housekeeping service. A nil `now` means time.Now.
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
// behind `decided_by`, `submitted` ones are members still waiting for an answer, and
// `queued`/`running` jobs are not finished (age is not evidence work completed). Those
// exemptions live in the store's WHERE clauses and are pinned by the conformance suite.
//
// The purges below return the rows they deleted so Housekeeping can report one per-run total.
func (s *Service) PurgeRecords(ctx context.Context) (int, error) {
	now := s.now()
	proposals, err := s.store.PurgeDiscardedProposals(ctx, now.Add(-s.win.Proposals()))
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
	n, err := s.store.PurgeTerminalInvitations(ctx, s.now().Add(-access.Retention))
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
	n, err := s.store.PurgeTerminalPasswordRecoveries(ctx, s.now().Add(-access.RecoveryRetention))
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

// FinishedOperationRetention is how long a finished model download or split detection is kept
// for a page to pick up again. A fixed product policy, like notifications: nothing reads it after.
const FinishedOperationRetention = 7 * 24 * time.Hour

// RecipientGrace is how long a requester row may point at a title that isn't written yet:
// approval records who asked before provisioning writes the title.
const RecipientGrace = 30 * 24 * time.Hour

// UnusedTitleRetention is how long a settled title no channel or open request names is kept. It
// outlasts a channel being rebuilt or a request being refined, either of which may name it again.
const UnusedTitleRetention = 180 * 24 * time.Hour

// PurgeSpent removes rows nothing will read again (#2528): pairing codes past their expiry,
// finished interactive operations past FinishedOperationRetention, discovery choices a newer one
// replaced, requesters of a title or channel that is gone, spent filler records, titles nothing
// has named for UnusedTitleRetention, and the catalog's gone titles, files and caches. Each
// returns its own count; the total is what the pass reports.
func (s *Service) PurgeSpent(ctx context.Context) (int, error) {
	now := s.now()
	var errs []error
	total := 0
	for _, purge := range []struct {
		what string
		run  func() (int, error)
	}{
		{"expired device pairings", func() (int, error) { return s.store.PurgeExpiredDevicePairings(ctx, now) }},
		{"finished interactive operations", func() (int, error) {
			return s.store.PurgeFinishedInteractiveOperations(ctx, now.Add(-FinishedOperationRetention))
		}},
		{"superseded discovery feedback", func() (int, error) { return s.store.PurgeSupersededDiscoveryFeedback(ctx) }},
		{"orphan notification recipients", func() (int, error) {
			return s.store.PurgeOrphanNotificationReferenceRecipients(ctx, now.Add(-RecipientGrace))
		}},
		{"filler records", func() (int, error) { return s.store.PurgeSpentFillerRecords(ctx, now) }},
		{"unused titles", func() (int, error) { return s.store.PurgeUnusedTitles(ctx, now.Add(-UnusedTitleRetention)) }},
		// After the titles: a title purged tonight releases its episode list and paths.
		{"spent catalog", func() (int, error) { return s.store.PurgeSpentCatalog(ctx, now) }},
	} {
		n, err := purge.run()
		if err != nil {
			errs = append(errs, err)
			continue
		}
		total += n
		if n > 0 && s.log != nil {
			s.log.Info("spent rows purged", "what", purge.what, "rows", n)
		}
	}
	return total, errors.Join(errs...)
}

// PurgeClientCrashReports applies the fixed crash-report policy (#1816): 30 days or the newest
// 1000, whichever is fewer. Like notifications it is deliberately not a setting; the byte budget
// above does not cover these rows, the row cap bounds them instead.
func (s *Service) PurgeClientCrashReports(ctx context.Context) (int, error) {
	n, err := s.store.PurgeClientCrashReports(ctx, s.now().Add(-diagnostics.ClientCrashRetention),
		diagnostics.ClientCrashRetainedMax)
	if n > 0 && s.log != nil {
		s.log.Info("client crash reports purged", "reports", n)
	}
	return n, err
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
	count(s.PurgeClientCrashReports(ctx))
	count(s.PurgeSpent(ctx))
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
	// After the purges, so each table's count is what retention keeps (#2793).
	if _, err := s.store.RecordTableRows(ctx, s.now(), s.now().Add(-tableRowHistory)); err != nil {
		errs = append(errs, fmt.Errorf("record table rows: %w", err))
	}
	// Last, so it returns what every purge above freed.
	var freed int64
	if s.tidy != nil {
		n, err := s.tidy(ctx)
		if err != nil {
			errs = append(errs, fmt.Errorf("tidy database: %w", err))
		}
		freed = n
	}
	if s.log != nil {
		s.log.Info("housekeeping finished", "rows_deleted", rows,
			"diagnostic_events", diag.Events, "diagnostic_process_runs", diag.ProcessRuns,
			"freed_bytes", freed,
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
func (s *Service) Job() jobs.Job {
	return jobs.Job{
		Name: "housekeeping", Group: jobs.GroupSystem, Title: "Clean up old data",
		Description: "Rolls up discovery-quality observations and removes expired sessions and pairing codes, old activity, invitations, password recoveries, diagnostics and notifications, denied and replaced requests, completed jobs and finished downloads after their retention periods, then gives the freed space back to the disk.",
		DefaultCron: "0 30 4 * * *", ScheduleKey: settings.JobHousekeepingSchedule.Key(),
		Timeout: housekeepingTimeout,
		Run:     s.Housekeeping,
	}
}
