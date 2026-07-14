package store

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// Job is a persisted suggester generation task (§8). The worker pool claims
// queued jobs via ClaimDueJobs and runs them; IntentJSON/ProposalJSON blobs
// carry the suggest.Intent / suggest.Proposal so the store stays domain-neutral
// (like titles.title_json). IntentHash is the cache key.
type Job struct {
	ID         string
	Kind       string // "suggest"
	Status     string // queued | running | done | failed
	IntentJSON string
	IntentHash string
	CreatedBy  string
	LastError  string
	Deadline   time.Time
	Attempts   int
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

// Proposal is a persisted suggester output (§8). Status drives the approval queue
// (submitted → approved/denied); CreatedBy powers My proposals (§12); ApprovedBy
// records the admin (§8 audit). ProposalJSON carries the suggest.Proposal.
type Proposal struct {
	ID           string
	JobID        string
	Status       string // submitted | approved | denied
	CreatedBy    string
	ApprovedBy   string
	DenyReason   string
	ProposalJSON string
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

// --- jobs ---

func (s *sqlStore) CreateJob(ctx context.Context, j Job) error {
	_, err := s.db.ExecContext(ctx, s.ph(
		`INSERT INTO jobs (id, kind, status, intent_json, intent_hash, created_by, last_error, deadline, attempts, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`),
		j.ID, j.Kind, j.Status, j.IntentJSON, j.IntentHash, j.CreatedBy, j.LastError,
		epoch(j.Deadline), j.Attempts, epoch(j.CreatedAt), epoch(j.UpdatedAt))
	if err != nil {
		return fmt.Errorf("create job %s: %w", j.ID, err)
	}
	return nil
}

const jobSelect = `SELECT id, kind, status, intent_json, intent_hash, created_by, last_error,
	deadline, attempts, created_at, updated_at FROM jobs`

func (s *sqlStore) GetJob(ctx context.Context, id string) (Job, error) {
	return scanJob(s.db.QueryRowContext(ctx, s.ph(jobSelect+` WHERE id = ?`), id))
}

func (s *sqlStore) UpdateJob(ctx context.Context, j Job) error {
	_, err := s.db.ExecContext(ctx, s.ph(
		`UPDATE jobs SET kind=?, status=?, intent_json=?, intent_hash=?, created_by=?,
		   last_error=?, deadline=?, attempts=?, updated_at=? WHERE id=?`),
		j.Kind, j.Status, j.IntentJSON, j.IntentHash, j.CreatedBy, j.LastError,
		epoch(j.Deadline), j.Attempts, epoch(j.UpdatedAt), j.ID)
	if err != nil {
		return fmt.Errorf("update job %s: %w", j.ID, err)
	}
	return nil
}

// ClaimDueJobs leases due queued jobs (§8/§18). Placeholders: 1=leaseUntil, 2=now, 3=limit.
func (s *sqlStore) ClaimDueJobs(ctx context.Context, now time.Time, lease time.Duration, limit int) ([]Job, error) {
	rows, err := s.db.QueryContext(ctx, s.jobClaimSQL, epoch(now.Add(lease)), epoch(now), limit)
	if err != nil {
		return nil, fmt.Errorf("claim due jobs: %w", err)
	}
	defer func() { _ = rows.Close() }()
	return scanJobs(rows)
}

// FindJobByIntentHash returns a recent job with a matching intent hash (§8 cache).
// `since` bounds the TTL: only jobs created at/after `since` count. Returns the
// most recent match, or ErrNotFound.
func (s *sqlStore) FindJobByIntentHash(ctx context.Context, hash string, since time.Time) (Job, error) {
	row := s.db.QueryRowContext(ctx, s.ph(jobSelect+
		` WHERE intent_hash = ? AND created_at >= ? ORDER BY created_at DESC LIMIT 1`),
		hash, epoch(since))
	return scanJob(row)
}

func scanJob(sc scannable) (Job, error) {
	var (
		j                              Job
		deadline, createdAt, updatedAt int64
	)
	err := sc.Scan(&j.ID, &j.Kind, &j.Status, &j.IntentJSON, &j.IntentHash, &j.CreatedBy,
		&j.LastError, &deadline, &j.Attempts, &createdAt, &updatedAt)
	if err == sql.ErrNoRows {
		return Job{}, ErrNotFound
	}
	if err != nil {
		return Job{}, err
	}
	j.Deadline = fromEpoch(deadline)
	j.CreatedAt = fromEpoch(createdAt)
	j.UpdatedAt = fromEpoch(updatedAt)
	return j, nil
}

func scanJobs(rows *sql.Rows) ([]Job, error) {
	var out []Job
	for rows.Next() {
		j, err := scanJob(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, j)
	}
	return out, rows.Err()
}

// --- proposals ---

func (s *sqlStore) CreateProposal(ctx context.Context, p Proposal) error {
	_, err := s.db.ExecContext(ctx, s.ph(
		`INSERT INTO proposals (id, job_id, status, created_by, approved_by, deny_reason, proposal_json, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`),
		p.ID, p.JobID, p.Status, p.CreatedBy, p.ApprovedBy, p.DenyReason, p.ProposalJSON,
		epoch(p.CreatedAt), epoch(p.UpdatedAt))
	if err != nil {
		return fmt.Errorf("create proposal %s: %w", p.ID, err)
	}
	return nil
}

const proposalSelect = `SELECT id, job_id, status, created_by, approved_by, deny_reason,
	proposal_json, created_at, updated_at FROM proposals`

func (s *sqlStore) GetProposal(ctx context.Context, id string) (Proposal, error) {
	return scanProposal(s.db.QueryRowContext(ctx, s.ph(proposalSelect+` WHERE id = ?`), id))
}

func (s *sqlStore) UpdateProposal(ctx context.Context, p Proposal) error {
	_, err := s.db.ExecContext(ctx, s.ph(
		`UPDATE proposals SET job_id=?, status=?, created_by=?, approved_by=?, deny_reason=?,
		   proposal_json=?, updated_at=? WHERE id=?`),
		p.JobID, p.Status, p.CreatedBy, p.ApprovedBy, p.DenyReason, p.ProposalJSON,
		epoch(p.UpdatedAt), p.ID)
	if err != nil {
		return fmt.Errorf("update proposal %s: %w", p.ID, err)
	}
	return nil
}

func (s *sqlStore) ListProposalsByStatus(ctx context.Context, status string) ([]Proposal, error) {
	rows, err := s.db.QueryContext(ctx, s.ph(proposalSelect+` WHERE status = ? ORDER BY created_at DESC`), status)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	return scanProposals(rows)
}

func (s *sqlStore) ListProposalsByCreator(ctx context.Context, userID string) ([]Proposal, error) {
	rows, err := s.db.QueryContext(ctx, s.ph(proposalSelect+` WHERE created_by = ? ORDER BY created_at DESC`), userID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	return scanProposals(rows)
}

func scanProposal(sc scannable) (Proposal, error) {
	var (
		p                    Proposal
		createdAt, updatedAt int64
	)
	err := sc.Scan(&p.ID, &p.JobID, &p.Status, &p.CreatedBy, &p.ApprovedBy, &p.DenyReason,
		&p.ProposalJSON, &createdAt, &updatedAt)
	if err == sql.ErrNoRows {
		return Proposal{}, ErrNotFound
	}
	if err != nil {
		return Proposal{}, err
	}
	p.CreatedAt = fromEpoch(createdAt)
	p.UpdatedAt = fromEpoch(updatedAt)
	return p, nil
}

func scanProposals(rows *sql.Rows) ([]Proposal, error) {
	var out []Proposal
	for rows.Next() {
		p, err := scanProposal(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}
