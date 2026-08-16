package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"sort"

	"github.com/mantonx/loomarr/internal/provision"
)

// ProposalApproval is the complete durable consequence of one approval decision.
// Titles are ordered because the first candidate for a key wins: an in-library
// available record precedes any acquisition copy of the same title.
type ProposalApproval struct {
	Proposal Proposal
	Titles   []provision.Record
	Channel  Channel
}

type encodedApprovalTitle struct {
	record provision.Record
	json   string
}

// CommitProposalApproval is the approval gate's persistence boundary. The proposal
// compare-and-swap and every insert-only title write commit together or not at all.
func (s *sqlStore) CommitProposalApproval(ctx context.Context, commit ProposalApproval) (int, error) {
	return s.commitProposalApproval(ctx, commit, nil)
}

func (s *sqlStore) CommitProposalApprovalGuarded(
	ctx context.Context,
	commit ProposalApproval,
	guard ProposalApprovalGuard,
) (int, error) {
	if guard == nil {
		return 0, fmt.Errorf("approve proposal %s: guard is nil", commit.Proposal.ID)
	}
	return s.commitProposalApproval(ctx, commit, guard)
}

func (s *sqlStore) commitProposalApproval(
	ctx context.Context,
	commit ProposalApproval,
	guard ProposalApprovalGuard,
) (int, error) {
	if commit.Proposal.Status != "approved" {
		return 0, fmt.Errorf("approve proposal %s: terminal status is %q", commit.Proposal.ID, commit.Proposal.Status)
	}
	if err := validateApprovalChannel(commit.Proposal, commit.Channel); err != nil {
		return 0, fmt.Errorf("approve proposal %s: %w", commit.Proposal.ID, err)
	}
	// Deduplicate before sorting so the caller's first candidate for a key still wins
	// (available lineup records intentionally precede acquisition copies). Sorting the
	// unique keys gives every concurrent Postgres transaction the same unique-index lock
	// order; opposite proposal order must not turn overlapping approvals into a deadlock.
	encoded := make([]encodedApprovalTitle, 0, len(commit.Titles))
	seen := make(map[provision.Key]bool, len(commit.Titles))
	for _, rec := range commit.Titles {
		if seen[rec.Key] {
			continue
		}
		if err := validateApprovalTitle(rec); err != nil {
			return 0, fmt.Errorf("approve proposal %s: %w", commit.Proposal.ID, err)
		}
		seen[rec.Key] = true
		blob, err := json.Marshal(rec.Title)
		if err != nil {
			return 0, fmt.Errorf("approve proposal %s: marshal title %s: %w", commit.Proposal.ID, rec.Key, err)
		}
		encoded = append(encoded, encodedApprovalTitle{record: rec, json: string(blob)})
	}
	sort.Slice(encoded, func(i, j int) bool { return encoded[i].record.Key < encoded[j].record.Key })

	orderKey := approvalOrderKey(commit.Proposal)
	if s.dialect == DialectSQLite {
		release, err := s.acquireSQLiteApprovalOrder(ctx, orderKey)
		if err != nil {
			return 0, err
		}
		defer release()
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("approve proposal %s: begin: %w", commit.Proposal.ID, err)
	}
	defer func() { _ = tx.Rollback() }()

	switch s.dialect {
	case DialectSQLite:
		// The process semaphore above is the complete lock domain: SQLite is
		// explicitly single-instance.
	case DialectPostgres:
		const lockSQL = `SELECT pg_advisory_xact_lock(hashtextextended(current_database() || ':' || $1, 0))`
		if _, err := tx.ExecContext(ctx, lockSQL, "loomarr:proposal-approval:"+orderKey); err != nil {
			return 0, fmt.Errorf("approve proposal %s: acquire requester ordering: %w", commit.Proposal.ID, err)
		}
	default:
		return 0, fmt.Errorf("approve proposal %s: unsupported store dialect %q", commit.Proposal.ID, s.dialect)
	}

	if guard != nil {
		if err := guard(ctx, proposalApprovalTxReader{s: s, tx: tx}); err != nil {
			return 0, fmt.Errorf("approve proposal %s: guard: %w", commit.Proposal.ID, err)
		}
	}

	p := commit.Proposal
	result, err := tx.ExecContext(ctx, s.ph(
		`UPDATE proposals SET job_id=?, status=?, created_by=?, approved_by=?, deny_reason=?,
		   mod_summary=?, note=?, proposal_json=?, approved_at=?, updated_at=?
		 WHERE id=? AND status='submitted'`),
		p.JobID, p.Status, p.CreatedBy, p.ApprovedBy, p.DenyReason, p.ModSummary, p.Note,
		p.ProposalJSON, epoch(p.ApprovedAt), epoch(p.UpdatedAt), p.ID)
	if err != nil {
		return 0, fmt.Errorf("approve proposal %s: %w", p.ID, err)
	}
	if err := proposalDecisionResult(ctx, tx, s.ph(`SELECT status FROM proposals WHERE id = ?`), p.ID, result); err != nil {
		return 0, err
	}

	// The supersession guard must share the approval transaction. A check in the
	// coordinator alone has a gap: a newer proposal can commit after that check but
	// before this approval plans, letting the older planner read the newer channel
	// revision and successfully CAS an older lineup over it. Channel revision protects
	// stale snapshots; this protects stale proposal decisions.
	//
	// Run it after the submitted->approved CAS so SQLite takes its write lock before
	// reading the comparison set; a deferred read transaction upgraded after another
	// commit can otherwise fail with SQLITE_BUSY_SNAPSHOT instead of a domain result.
	// The transaction rolls this status change back when a superseder exists.
	// Compare against the candidate row's persisted job/time, not caller fields. Epoch
	// seconds cannot order a tie, so equal timestamps fail closed instead of guessing.
	var supersedingID string
	err = tx.QueryRowContext(ctx, s.ph(
		`SELECT newer.id
		   FROM proposals newer
		   JOIN proposals candidate ON candidate.id = ?
		  WHERE newer.job_id = candidate.job_id
		    AND newer.status = 'approved'
		    AND newer.id <> candidate.id
		    AND newer.created_at >= candidate.created_at
		  LIMIT 1`), p.ID).Scan(&supersedingID)
	if err == nil {
		return 0, fmt.Errorf("%w: proposal %s is preceded by approved proposal %s",
			ErrProposalSuperseded, p.ID, supersedingID)
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return 0, fmt.Errorf("approve proposal %s: check supersession: %w", p.ID, err)
	}

	enqueued := 0
	for _, item := range encoded {
		r := item.record
		result, err := tx.ExecContext(ctx, s.ph(
			`INSERT INTO titles
			   (key, title_json, state, library_id, requested_at, deadline, attempts, last_error, updated_at)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
			 ON CONFLICT(key) DO NOTHING`),
			string(r.Key), item.json, string(r.State), r.LibraryID,
			epoch(r.RequestedAt), epoch(r.Deadline), r.Attempts, r.LastError, epoch(r.UpdatedAt))
		if err != nil {
			return 0, fmt.Errorf("approve proposal %s: insert title %s: %w", p.ID, r.Key, err)
		}
		inserted, err := result.RowsAffected()
		if err != nil {
			return 0, fmt.Errorf("approve proposal %s: count title %s: %w", p.ID, r.Key, err)
		}
		if inserted == 1 && r.State == provision.Wanted {
			enqueued++
		}
	}
	if _, err := s.saveChannel(ctx, tx, commit.Channel); err != nil {
		if isConstraintViolation(err) {
			return 0, fmt.Errorf("%w: %v", ErrChannelConflict, err)
		}
		return 0, fmt.Errorf("approve proposal %s: %w", p.ID, err)
	}
	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("approve proposal %s: commit: %w", p.ID, err)
	}
	return enqueued, nil
}

func approvalOrderKey(p Proposal) string {
	if p.CreatedBy != "" {
		return "requester:" + p.CreatedBy
	}
	// A proposal without a requester cannot use the per-user auto-approve grant,
	// but its manual decision still receives stable ordering for retries.
	return "proposal:" + p.ID
}

func (s *sqlStore) acquireSQLiteApprovalOrder(ctx context.Context, key string) (func(), error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	candidate := make(chan struct{}, 1)
	candidate <- struct{}{}
	actual, _ := s.approvalLocks.LoadOrStore(key, candidate)
	lock := actual.(chan struct{})
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-lock:
	}
	return func() { lock <- struct{}{} }, nil
}

// proposalApprovalTxReader is the guard's transaction-bound adapter. It exposes
// exactly the three reads quota accounting needs; no caller can escape to the pool
// while assuming the transaction-scoped Postgres ordering still fences its view.
type proposalApprovalTxReader struct {
	s  *sqlStore
	tx *sql.Tx
}

func (r proposalApprovalTxReader) GetUser(ctx context.Context, id string) (User, error) {
	return scanUser(r.tx.QueryRowContext(ctx, r.s.ph(
		`SELECT id, name, role, disabled, quota, auto_approve, password_hash, created_at, updated_at
		 FROM users WHERE id = ?`), id))
}

func (r proposalApprovalTxReader) ListProposalsByCreator(ctx context.Context, userID string) ([]Proposal, error) {
	rows, err := r.tx.QueryContext(ctx, r.s.ph(proposalSelect+` WHERE created_by = ? ORDER BY created_at DESC`), userID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	return scanProposals(rows)
}

func (r proposalApprovalTxReader) GetTitle(ctx context.Context, key provision.Key) (provision.Record, error) {
	return scanTitle(r.tx.QueryRowContext(ctx, r.s.ph(
		`SELECT key, title_json, state, library_id, requested_at, deadline, attempts, last_error, updated_at,
		        progress, eta_text, download_status
		 FROM titles WHERE key = ?`), string(key)))
}

// isConstraintViolation recognizes the portable driver contracts without coupling the
// store to either concrete error package. Postgres exposes SQLSTATE 23505; modernc SQLite
// exposes extended result codes whose low byte is SQLITE_CONSTRAINT (19).
func isConstraintViolation(err error) bool {
	var state interface{ SQLState() string }
	if errors.As(err, &state) && state.SQLState() == "23505" {
		return true
	}
	var coded interface{ Code() int }
	return errors.As(err, &coded) && coded.Code()&0xff == 19
}

func validateApprovalChannel(p Proposal, ch Channel) error {
	if err := ch.Validate(); err != nil {
		return fmt.Errorf("invalid channel: %w", err)
	}
	if err := ch.Policy.Validate(); err != nil {
		return fmt.Errorf("invalid channel policy: %w", err)
	}
	if ch.IntentRef == "" {
		return fmt.Errorf("channel %s has no intent ref", ch.ID)
	}
	if ch.IntentRef != p.JobID {
		return fmt.Errorf("channel %s intent ref %q does not match proposal job %q", ch.ID, ch.IntentRef, p.JobID)
	}
	return nil
}

func validateApprovalTitle(rec provision.Record) error {
	wantKey, err := rec.Title.Key()
	if err != nil {
		return fmt.Errorf("invalid title %s: %w", rec.Key, err)
	}
	if rec.Key == "" || rec.Key != wantKey {
		return fmt.Errorf("title key %q does not match %q", rec.Key, wantKey)
	}
	switch rec.State {
	case provision.Available:
		if rec.LibraryID == "" {
			return fmt.Errorf("available title %s has no library id", rec.Key)
		}
	case provision.Wanted:
		if rec.Deadline.IsZero() {
			return fmt.Errorf("wanted title %s has no deadline", rec.Key)
		}
	default:
		return fmt.Errorf("title %s has invalid approval state %q", rec.Key, rec.State)
	}
	return nil
}

// CommitProposalDenial makes denial the other side of the same terminal-decision
// race. It deliberately updates only decision-owned fields.
func (s *sqlStore) CommitProposalDenial(ctx context.Context, p Proposal) error {
	if p.Status != "denied" {
		return fmt.Errorf("deny proposal %s: terminal status is %q", p.ID, p.Status)
	}
	result, err := s.db.ExecContext(ctx, s.ph(
		`UPDATE proposals SET status='denied', approved_by=?, deny_reason=?, updated_at=?
		 WHERE id=? AND status='submitted'`),
		p.ApprovedBy, p.DenyReason, epoch(p.UpdatedAt), p.ID)
	if err != nil {
		return fmt.Errorf("deny proposal %s: %w", p.ID, err)
	}
	return proposalDecisionResult(ctx, s.db, s.ph(`SELECT status FROM proposals WHERE id = ?`), p.ID, result)
}

type proposalStatusReader interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

func proposalDecisionResult(ctx context.Context, q proposalStatusReader, statusQuery, id string, result sql.Result) error {
	n, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("decide proposal %s: affected rows: %w", id, err)
	}
	if n == 1 {
		return nil
	}
	var status string
	err = q.QueryRowContext(ctx, statusQuery, id).Scan(&status)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return fmt.Errorf("decide proposal %s: read status: %w", id, err)
	}
	return ErrProposalNotSubmitted
}
