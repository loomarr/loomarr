package store

import (
	"context"
	"fmt"

	"github.com/loomarr/loomarr/internal/filler"
)

// CommitPullApproval owns the whole durable boundary. The first write takes the pending
// decision; an insert failure rolls it back, so no downloader may start on a partial commit.
func (s *sqlStore) CommitPullApproval(ctx context.Context, p filler.Pull, run filler.AcquisitionRun) error {
	if p.ID == "" || p.Status != filler.PullApproved || p.DecidedAt.IsZero() ||
		run.ID == "" || run.PullID != p.ID || run.Trigger != filler.AcquisitionPull ||
		run.Status != filler.AcquisitionQueued || len(p.Committed()) == 0 || run.Requested != len(p.Committed()) {
		return fmt.Errorf("invalid filler pull approval")
	}
	plan, err := encodePullPlan(p)
	if err != nil {
		return fmt.Errorf("encode pull decision: %w", err)
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin pull decision: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	result, err := tx.ExecContext(ctx, s.ph(`UPDATE filler_pulls
		SET status = ?, note = ?, plan_json = ?, decided_at = ?, decided_by = ?
		WHERE id = ? AND status = ? AND NOT EXISTS
		(SELECT 1 FROM filler_acquisition_runs WHERE pull_id = ?)`),
		string(p.Status), p.Note, string(plan), epoch(p.DecidedAt), p.DecidedBy,
		p.ID, string(filler.PullPending), p.ID)
	if err != nil {
		return fmt.Errorf("claim pull approval: %w", err)
	}
	n, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("count pull approval: %w", err)
	}
	if n != 1 {
		return ErrPullNotPending
	}
	if _, err := tx.ExecContext(ctx, s.ph(acquisitionRunInsert), acquisitionRunArgs(run)...); err != nil {
		return fmt.Errorf("queue approved pull: %w", err)
	}
	if _, err := tx.ExecContext(ctx, s.ph(`INSERT INTO filler_pull_commits (pull_id, acquisition_id) VALUES (?, ?)`), p.ID, run.ID); err != nil {
		return fmt.Errorf("bind approved pull: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit pull approval: %w", err)
	}
	return nil
}

// DismissPull preserves the winning audit when approval races with dismissal.
func (s *sqlStore) DismissPull(ctx context.Context, p filler.Pull) error {
	if p.Status != filler.PullDismissed || p.DecidedAt.IsZero() {
		return fmt.Errorf("invalid filler pull dismissal")
	}
	result, err := s.db.ExecContext(ctx, s.ph(`UPDATE filler_pulls
		SET status = ?, decided_at = ?, decided_by = ? WHERE id = ? AND status = ?`),
		string(p.Status), epoch(p.DecidedAt), p.DecidedBy, p.ID, string(filler.PullPending))
	if err != nil {
		return fmt.Errorf("dismiss filler pull: %w", err)
	}
	n, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("count filler dismissal: %w", err)
	}
	if n != 1 {
		return ErrPullNotPending
	}
	return nil
}
