package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// CommitSuggestionSuccess is the generation-success persistence boundary. A
// proposal cannot become visible unless its job becomes done in the same commit.
func (s *sqlStore) CommitSuggestionSuccess(
	ctx context.Context,
	jobID string,
	expectedAttempt int,
	p Proposal,
	updatedAt time.Time,
) error {
	if p.JobID != jobID {
		return fmt.Errorf("complete suggestion job %s: proposal belongs to job %s", jobID, p.JobID)
	}
	if p.Status != "submitted" {
		return fmt.Errorf("complete suggestion job %s: proposal status is %q", jobID, p.Status)
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("complete suggestion job %s: begin: %w", jobID, err)
	}
	defer func() { _ = tx.Rollback() }()

	result, err := tx.ExecContext(ctx, s.ph(
		`UPDATE jobs SET status='done', last_error='', failure_code='', failure_trace_json='', updated_at=?
		  WHERE id=? AND status='running' AND attempts=? AND created_by=?`),
		epoch(updatedAt), jobID, expectedAttempt, p.CreatedBy)
	if err != nil {
		return fmt.Errorf("complete suggestion job %s: transition: %w", jobID, err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("complete suggestion job %s: transition count: %w", jobID, err)
	}
	if affected != 1 {
		var status, createdBy string
		err := tx.QueryRowContext(ctx, s.ph(`SELECT status, created_by FROM jobs WHERE id = ?`), jobID).
			Scan(&status, &createdBy)
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil {
			return fmt.Errorf("complete suggestion job %s: read status: %w", jobID, err)
		}
		if createdBy != p.CreatedBy {
			return fmt.Errorf("%w: job %s belongs to %q, proposal belongs to %q",
				ErrJobOwnershipMismatch, jobID, createdBy, p.CreatedBy)
		}
		return fmt.Errorf("%w: job %s has status %s", ErrJobNotRunning, jobID, status)
	}
	result, err = tx.ExecContext(ctx, s.ph(
		`UPDATE proposal_job_attempts
		    SET status='succeeded', completed_at=?, failure_code=''
		  WHERE job_id=? AND attempt=? AND workflow_version=? AND status='running'`),
		epoch(updatedAt), jobID, expectedAttempt, ProposalWorkflowVersion)
	if err != nil {
		return fmt.Errorf("complete suggestion job %s: close Attempt %d: %w", jobID, expectedAttempt, err)
	}
	affected, err = result.RowsAffected()
	if err != nil || affected != 1 {
		return fmt.Errorf("%w: job %s Attempt %d is missing or terminal", ErrJobNotRunning, jobID, expectedAttempt)
	}

	// Proposal timestamps are stored at second precision. Make successive
	// executions on a stable channel job strictly monotonic so "newest" remains
	// causal even when a refine finishes in the same wall-clock second.
	var latestCreatedAt sql.NullInt64
	if err := tx.QueryRowContext(ctx, s.ph(
		`SELECT MAX(created_at) FROM proposals WHERE job_id = ?`), jobID).Scan(&latestCreatedAt); err != nil {
		return fmt.Errorf("complete suggestion job %s: read latest proposal: %w", jobID, err)
	}
	if latestCreatedAt.Valid && epoch(p.CreatedAt) <= latestCreatedAt.Int64 {
		p.CreatedAt = fromEpoch(latestCreatedAt.Int64 + 1)
		if p.UpdatedAt.Before(p.CreatedAt) {
			p.UpdatedAt = p.CreatedAt
		}
		if _, err := tx.ExecContext(ctx, s.ph(
			`UPDATE jobs SET updated_at=? WHERE id=?`), epoch(p.CreatedAt), jobID); err != nil {
			return fmt.Errorf("complete suggestion job %s: order completion: %w", jobID, err)
		}
	}

	if err := insertProposalTx(ctx, tx, s, p); err != nil {
		return fmt.Errorf("complete suggestion job %s: %w", jobID, err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("complete suggestion job %s: commit: %w", jobID, err)
	}
	return nil
}

// RequeueSuggestionJob replaces only a terminal execution the caller actually
// observed. It never rewrites attempts, requester identity, or creation time.
func (s *sqlStore) RequeueSuggestionJob(
	ctx context.Context,
	jobID string,
	expectedAttempt int,
	kind, intentJSON, intentHash string,
	deadline, updatedAt time.Time,
) error {
	result, err := s.db.ExecContext(ctx, s.ph(
		`UPDATE jobs
			SET kind=?, status='queued', intent_json=?, intent_hash=?, last_error='', failure_code='', failure_trace_json='', deadline=?, updated_at=?
		  WHERE id=? AND status IN ('done', 'failed') AND attempts=?`),
		kind, intentJSON, intentHash, epoch(deadline), epoch(updatedAt), jobID, expectedAttempt)
	if err != nil {
		return fmt.Errorf("requeue suggestion job %s: transition: %w", jobID, err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("requeue suggestion job %s: transition count: %w", jobID, err)
	}
	if affected == 1 {
		return nil
	}
	var status string
	err = s.db.QueryRowContext(ctx, s.ph(`SELECT status FROM jobs WHERE id = ?`), jobID).Scan(&status)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return fmt.Errorf("requeue suggestion job %s: read status: %w", jobID, err)
	}
	return fmt.Errorf("%w: job %s has status %s or a newer attempt", ErrJobNotTerminal, jobID, status)
}

// CommitSuggestionFailure is the generation-failure persistence boundary. It
// updates only lifecycle fields and loses safely if another execution has
// already replaced this attempt.
func (s *sqlStore) CommitSuggestionFailure(
	ctx context.Context,
	jobID string,
	expectedAttempt int,
	cause, failureCode, failureTraceJSON string,
	updatedAt time.Time,
) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("fail suggestion job %s: begin: %w", jobID, err)
	}
	defer func() { _ = tx.Rollback() }()

	result, err := tx.ExecContext(ctx, s.ph(
		`UPDATE jobs
		    SET status='failed', last_error=?, failure_code=?, failure_trace_json=?, updated_at=?
		  WHERE id=? AND status='running' AND attempts=?`),
		cause, failureCode, failureTraceJSON, epoch(updatedAt), jobID, expectedAttempt)
	if err != nil {
		return fmt.Errorf("fail suggestion job %s: transition: %w", jobID, err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("fail suggestion job %s: transition count: %w", jobID, err)
	}
	if affected != 1 {
		var status string
		err = tx.QueryRowContext(ctx, s.ph(`SELECT status FROM jobs WHERE id = ?`), jobID).Scan(&status)
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil {
			return fmt.Errorf("fail suggestion job %s: read status: %w", jobID, err)
		}
		return fmt.Errorf("%w: job %s has status %s or a newer attempt", ErrJobNotRunning, jobID, status)
	}
	result, err = tx.ExecContext(ctx, s.ph(
		`UPDATE proposal_job_attempts
		    SET status='failed', completed_at=?, failure_code=?
		  WHERE job_id=? AND attempt=? AND workflow_version=? AND status='running'`),
		epoch(updatedAt), failureCode, jobID, expectedAttempt, ProposalWorkflowVersion)
	if err != nil {
		return fmt.Errorf("fail suggestion job %s: close Attempt %d: %w", jobID, expectedAttempt, err)
	}
	affected, err = result.RowsAffected()
	if err != nil || affected != 1 {
		return fmt.Errorf("%w: job %s Attempt %d is missing or terminal", ErrJobNotRunning, jobID, expectedAttempt)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("fail suggestion job %s: commit: %w", jobID, err)
	}
	return nil
}

// CloneSuggestionSuccess copies only cached proposal content. The new job and
// proposal are a fresh request lifecycle owned by the current caller.
func (s *sqlStore) CloneSuggestionSuccess(
	ctx context.Context,
	sourceJobID string,
	job Job,
	proposalID string,
) (Proposal, error) {
	if job.Status != "done" {
		return Proposal{}, fmt.Errorf("clone suggestion job %s: new status is %q", job.ID, job.Status)
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Proposal{}, fmt.Errorf("clone suggestion job %s: begin: %w", job.ID, err)
	}
	defer func() { _ = tx.Rollback() }()

	// Acquire SQLite's WAL write snapshot before reading the cache source. If the
	// source changes concurrently, the later SELECT still sees one coherent
	// snapshot instead of failing a deferred read-to-write upgrade.
	if _, err := tx.ExecContext(ctx, s.ph(
		`INSERT INTO jobs (id, kind, status, intent_json, intent_hash, created_by, last_error, failure_code,
		                    workflow_version, reached_live, deadline, attempts, created_at, updated_at, failure_trace_json)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`),
		job.ID, job.Kind, job.Status, job.IntentJSON, job.IntentHash, job.CreatedBy, "", "",
		workflowVersionForCreate(job.WorkflowVersion), job.ReachedLive, epoch(job.Deadline), job.Attempts,
		epoch(job.CreatedAt), epoch(job.UpdatedAt), ""); err != nil {
		return Proposal{}, fmt.Errorf("clone suggestion job %s: create job: %w", job.ID, err)
	}

	var proposalJSON string
	if err := tx.QueryRowContext(ctx, s.ph(
		`SELECT p.proposal_json
		   FROM jobs j
		   JOIN proposals p ON p.id = (
		       SELECT p2.id FROM proposals p2
		        WHERE p2.job_id = j.id AND p2.created_by = j.created_by
		        ORDER BY p2.created_at DESC, p2.id DESC
		        LIMIT 1
		   )
		  WHERE j.id = ? AND j.status = 'done' AND j.kind = 'suggest'
		    AND j.intent_hash = ?`), sourceJobID, job.IntentHash).
		Scan(&proposalJSON); errors.Is(err, sql.ErrNoRows) {
		return Proposal{}, ErrNotFound
	} else if err != nil {
		return Proposal{}, fmt.Errorf("clone suggestion job %s: read source result: %w", job.ID, err)
	}

	p := Proposal{
		ID: proposalID, JobID: job.ID, Status: "submitted", CreatedBy: job.CreatedBy,
		ProposalJSON: proposalJSON, CreatedAt: job.CreatedAt, UpdatedAt: job.UpdatedAt,
	}
	if err := insertProposalTx(ctx, tx, s, p); err != nil {
		return Proposal{}, fmt.Errorf("clone suggestion job %s: %w", job.ID, err)
	}
	if _, err := tx.ExecContext(ctx, s.ph(
		`INSERT INTO proposal_job_attempts
		    (job_id, attempt, workflow_version, status, started_at, completed_at, failure_code)
		 VALUES (?, 1, ?, 'succeeded', ?, ?, '')`),
		job.ID, ProposalWorkflowVersion, epoch(job.CreatedAt), epoch(job.UpdatedAt)); err != nil {
		return Proposal{}, fmt.Errorf("clone suggestion job %s: create cached Attempt: %w", job.ID, err)
	}
	if _, err := tx.ExecContext(ctx, s.ph(
		`UPDATE jobs SET workflow_version=?, attempts=1 WHERE id=?`),
		ProposalWorkflowVersion, job.ID); err != nil {
		return Proposal{}, fmt.Errorf("clone suggestion job %s: version cached result: %w", job.ID, err)
	}
	if err := tx.Commit(); err != nil {
		return Proposal{}, fmt.Errorf("clone suggestion job %s: commit: %w", job.ID, err)
	}
	return p, nil
}

func insertProposalTx(ctx context.Context, tx *sql.Tx, s *sqlStore, p Proposal) error {
	_, err := tx.ExecContext(ctx, s.ph(
		`INSERT INTO proposals (id, job_id, status, created_by, approved_by, deny_reason, mod_summary, note, proposal_json, approved_at, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`),
		p.ID, p.JobID, p.Status, p.CreatedBy, p.ApprovedBy, p.DenyReason, p.ModSummary, p.Note,
		p.ProposalJSON, epoch(p.ApprovedAt), epoch(p.CreatedAt), epoch(p.UpdatedAt))
	if err != nil {
		return fmt.Errorf("create proposal %s: %w", p.ID, err)
	}
	return nil
}
