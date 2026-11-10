package store

import (
	"context"
	"fmt"
	"time"

	"github.com/loomarr/loomarr/internal/library/provision"
	"github.com/loomarr/loomarr/internal/platform/store/storedb"
	"github.com/loomarr/loomarr/internal/proposals"
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
//
// ⚠ **A job whose proposal is still `submitted` is not finished, whatever its status.** Approving
// that proposal needs the job: a refine or re-curation job is how it finds its channel, and
// without it approval would create a second channel. The job becomes purgeable once the proposal
// is decided.
//
// A channel does not depend on the job it was made from: it carries its own approved intent and
// requester (`channels.intent_json`, `channels.requested_by`), so a purge never takes away what a
// refine or a re-curation needs.
func (s *sqlStore) PurgeFinishedJobs(ctx context.Context, before time.Time) (int, error) {
	res, err := s.db.ExecContext(ctx, s.ph(
		`DELETE FROM proposal_jobs WHERE status IN ('done','failed') AND updated_at < ?
		   AND NOT EXISTS (SELECT 1 FROM proposals p WHERE p.job_id = proposal_jobs.id AND p.status = 'submitted')`),
		epoch(before))
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

// PurgeUnusedTitles removes the settled titles (available or given up on) last changed before
// `before` that no channel's lineup and no open proposal names, and returns the count (#2528).
//
// Every read of a title starts from a lineup or a pick, so a title neither names is read only if
// it is picked again, and then a missing row means "request it": approval writes it afresh and a
// title once given up on is tried again, which after the window is the right answer. A title
// still wanted, requested or downloading is in flight and stays at any age. A pending row no
// lineup names any more goes too, whatever its age.
func (s *sqlStore) PurgeUnusedTitles(ctx context.Context, before time.Time) (int, error) {
	named := map[provision.Key]bool{}
	open, err := s.ListProposalsByStatus(ctx, openProposalStatus)
	if err != nil {
		return 0, fmt.Errorf("purge unused titles: %w", err)
	}
	for _, p := range open {
		for _, list := range [][]proposals.ProposalItem{p.Body.Lineup, p.Body.Acquisitions, p.Body.Alternates} {
			for _, pick := range list {
				if key, err := pick.Key(); err == nil {
					named[key] = true
				}
			}
		}
	}
	candidates, err := s.distinctStrings(ctx, s.db, `SELECT key FROM titles WHERE state IN (?, ?) AND updated_at < ?
		AND NOT EXISTS (SELECT 1 FROM channel_lineup l WHERE l.title_key = titles.key)`,
		string(provision.Available), string(provision.Unavailable), epoch(before))
	if err != nil {
		return 0, fmt.Errorf("purge unused titles: %w", err)
	}
	total := 0
	for _, key := range candidates {
		if named[provision.Key(key)] {
			continue
		}
		// The state and age are checked again: the row may have moved on since it was listed.
		res, err := s.db.ExecContext(ctx, s.ph(`DELETE FROM titles WHERE key = ? AND state IN (?, ?) AND updated_at < ?`),
			key, string(provision.Available), string(provision.Unavailable), epoch(before))
		if err != nil {
			return total, fmt.Errorf("purge unused title %s: %w", key, err)
		}
		n, _ := res.RowsAffected()
		total += int(n)
	}
	// A pending row exists only because a lineup named the title (storedb.PendingTitle); once none
	// does, it records nothing.
	res, err := s.db.ExecContext(ctx, s.ph(`DELETE FROM titles WHERE state = ?
		AND NOT EXISTS (SELECT 1 FROM channel_lineup l WHERE l.title_key = titles.key)`), storedb.PendingTitle)
	if err != nil {
		return total, fmt.Errorf("purge unnamed pending titles: %w", err)
	}
	n, _ := res.RowsAffected()
	return total + int(n), nil
}

// PurgeDiscardedProposals removes `denied` and `superseded` proposals older than `before` and
// returns the count. A superseded proposal was replaced by a newer one from the same job before
// anyone decided it, so nothing reads it again.
//
// ⚠ **`approved` and `submitted` are kept indefinitely**, for two different reasons:
//   - `approved` is the audit trail behind `decided_by` — the record that someone authorized
//     spending real resources (§7). An audit trail with a retention window is not one.
//   - `submitted` is a member still waiting for an answer. Ageing it out would discard their
//     request rather than decline it, so it would vanish from "My requests" with nothing
//     saying why.
func (s *sqlStore) PurgeDiscardedProposals(ctx context.Context, before time.Time) (int, error) {
	res, err := s.db.ExecContext(ctx, s.ph(
		`DELETE FROM proposals WHERE status IN ('denied', 'superseded') AND updated_at < ?`), epoch(before))
	if err != nil {
		return 0, fmt.Errorf("purge discarded proposals: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return 0, nil
	}
	return int(n), nil
}
