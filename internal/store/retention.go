package store

import (
	"context"
	"fmt"
	"time"
)

// Retention purges for the accumulating suggester tables (§5, §18.1).
//
// ⚠ **`JOBS_RETENTION` and `PROPOSALS_RETENTION` were declared long before anything read
// them**, and §5 described the purge as shipped for several phases while it did not exist —
// the same declared-but-unconsumed shape V12 found in `backup.retain`. These are the readers.
//
// Both are conservative by design: they remove only rows whose work is finished AND whose
// absence loses nothing anyone can still act on. Everything else accumulates, which is the
// correct trade — a landfill is a nuisance, a missing audit row is a wrong answer.

// PurgeFinishedJobs removes `done`/`failed` jobs older than `before` and returns the count.
//
// ⚠ **`queued` and `running` are never purged, however old.** Age is not evidence that work
// finished: a job can sit queued behind a busy worker pool, and `running` means a worker
// holds its lease right now. Deleting either would strand the worker or silently drop
// requested work — and the reconciler's deadline discipline (§4) already ages out genuinely
// stuck jobs by moving them to a terminal state, which is what makes them eligible here.
func (s *sqlStore) PurgeFinishedJobs(ctx context.Context, before time.Time) (int, error) {
	res, err := s.db.ExecContext(ctx, s.ph(
		`DELETE FROM jobs WHERE status IN ('done','failed') AND updated_at < ?`), epoch(before))
	if err != nil {
		return 0, fmt.Errorf("purge finished jobs: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		// The delete succeeded; only the count is unavailable. Returning an error here would
		// make the job report a failure for work that was actually done.
		return 0, nil
	}
	return int(n), nil
}

// PurgeDeniedProposals removes `denied` proposals older than `before` and returns the count.
//
// ⚠ **`approved` and `submitted` are kept indefinitely**, for two different reasons:
//   - `approved` is the audit trail behind `approved_by` — the record that someone authorized
//     spending real resources (§7). An audit trail with a retention window is not one.
//   - `submitted` is a member still waiting for an answer. Ageing it out would discard their
//     request rather than decline it, so it would vanish from "My requests" with nothing
//     saying why.
func (s *sqlStore) PurgeDeniedProposals(ctx context.Context, before time.Time) (int, error) {
	res, err := s.db.ExecContext(ctx, s.ph(
		`DELETE FROM proposals WHERE status = 'denied' AND updated_at < ?`), epoch(before))
	if err != nil {
		return 0, fmt.Errorf("purge denied proposals: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return 0, nil
	}
	return int(n), nil
}
