package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// Job is a persisted suggester generation task (§8). The worker pool claims
// queued jobs via ClaimDueJobs and runs them; IntentJSON/ProposalJSON blobs
// carry the suggest.Intent / suggest.Proposal so the store stays domain-neutral
// (like titles.title_json). IntentHash is the cache key.
type Job struct {
	ID         string
	Kind       string // "suggest" (human/user flow) or "recurate" (scheduled channel grant)
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

// ProposalJob is the consistent read projection for one generation execution.
// Proposal is nil until the current execution is done; it may then be submitted,
// approved, or denied independently from the job lifecycle.
type ProposalJob struct {
	Job      Job
	Proposal *Proposal
}

// Proposal is a persisted suggester output (§8). Status drives the approval queue
// (submitted → approved/denied); CreatedBy powers My proposals (§12); ApprovedBy
// records the admin (§8 audit). ProposalJSON carries the suggest.Proposal.
type Proposal struct {
	ID         string
	JobID      string
	Status     string // submitted | approved | denied
	CreatedBy  string
	ApprovedBy string
	DenyReason string
	// ModSummary is what the approver CHANGED before approving ("dropped 2, added 1"),
	// generated server-side rather than typed. A summary the approver writes is a claim;
	// one the code writes is a record (§7, D-K edit-before-approve).
	ModSummary string
	// Note is the approver's message to whoever requested it ("swapped Con Air for
	// Face/Off — we already have that one"). It is why a request coming back altered is
	// explicable rather than mysterious.
	Note         string
	ProposalJSON string
	// ApprovedAt is WHEN the gate let this through — the audit rows' ordering key (§7, V27).
	// Zero = never approved. Deliberately not `UpdatedAt`: three callers write that (approve,
	// deny, recurate), so a re-curation would silently move an approval's timestamp.
	ApprovedAt time.Time
	CreatedAt  time.Time
	UpdatedAt  time.Time
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

func (s *sqlStore) GetProposalJob(ctx context.Context, id string) (ProposalJob, error) {
	row := s.db.QueryRowContext(ctx, s.ph(
		`SELECT j.id, j.kind, j.status, j.intent_json, j.intent_hash, j.created_by, j.last_error,
		        j.deadline, j.attempts, j.created_at, j.updated_at,
		        p.id, p.job_id, p.status, p.created_by, p.approved_by, p.deny_reason,
		        p.mod_summary, p.note, p.proposal_json, p.approved_at, p.created_at, p.updated_at
		   FROM jobs j
		   LEFT JOIN proposals p
		     ON j.status = 'done'
		    AND p.created_by = j.created_by
		    AND p.id = (
		        SELECT p2.id FROM proposals p2
		         WHERE p2.job_id = j.id AND p2.created_by = j.created_by
		         ORDER BY p2.created_at DESC, p2.id DESC
		         LIMIT 1
		    )
		  WHERE j.id = ?`), id)

	var (
		out                                   ProposalJob
		deadline, jobCreatedAt, jobUpdatedAt  int64
		pID, pJobID, pStatus, pCreatedBy      sql.NullString
		pApprovedBy, pDenyReason, pModSummary sql.NullString
		pNote, pJSON                          sql.NullString
		pApprovedAt, pCreatedAt, pUpdatedAt   sql.NullInt64
	)
	err := row.Scan(
		&out.Job.ID, &out.Job.Kind, &out.Job.Status, &out.Job.IntentJSON, &out.Job.IntentHash,
		&out.Job.CreatedBy, &out.Job.LastError, &deadline, &out.Job.Attempts, &jobCreatedAt, &jobUpdatedAt,
		&pID, &pJobID, &pStatus, &pCreatedBy, &pApprovedBy, &pDenyReason,
		&pModSummary, &pNote, &pJSON, &pApprovedAt, &pCreatedAt, &pUpdatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return ProposalJob{}, ErrNotFound
	}
	if err != nil {
		return ProposalJob{}, fmt.Errorf("get proposal job %s: %w", id, err)
	}
	out.Job.Deadline = fromEpoch(deadline)
	out.Job.CreatedAt = fromEpoch(jobCreatedAt)
	out.Job.UpdatedAt = fromEpoch(jobUpdatedAt)
	if pID.Valid {
		out.Proposal = &Proposal{
			ID: pID.String, JobID: pJobID.String, Status: pStatus.String, CreatedBy: pCreatedBy.String,
			ApprovedBy: pApprovedBy.String, DenyReason: pDenyReason.String, ModSummary: pModSummary.String,
			Note: pNote.String, ProposalJSON: pJSON.String,
			ApprovedAt: fromEpoch(pApprovedAt.Int64), CreatedAt: fromEpoch(pCreatedAt.Int64),
			UpdatedAt: fromEpoch(pUpdatedAt.Int64),
		}
	}
	return out, nil
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

// ClaimDueJobs atomically starts and leases due queued jobs (§8/§18).
// Placeholders: 1=leaseUntil, 2=now, 3=limit.
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
		`INSERT INTO proposals (id, job_id, status, created_by, approved_by, deny_reason, mod_summary, note, proposal_json, approved_at, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`),
		p.ID, p.JobID, p.Status, p.CreatedBy, p.ApprovedBy, p.DenyReason, p.ModSummary, p.Note,
		p.ProposalJSON, epoch(p.ApprovedAt), epoch(p.CreatedAt), epoch(p.UpdatedAt))
	if err != nil {
		return fmt.Errorf("create proposal %s: %w", p.ID, err)
	}
	return nil
}

const proposalSelect = `SELECT id, job_id, status, created_by, approved_by, deny_reason,
	mod_summary, note, proposal_json, approved_at, created_at, updated_at FROM proposals`

func (s *sqlStore) GetProposal(ctx context.Context, id string) (Proposal, error) {
	return scanProposal(s.db.QueryRowContext(ctx, s.ph(proposalSelect+` WHERE id = ?`), id))
}

func (s *sqlStore) ListProposalsByStatus(ctx context.Context, status string) ([]Proposal, error) {
	rows, err := s.db.QueryContext(ctx, s.ph(proposalSelect+` WHERE status = ? ORDER BY created_at DESC`), status)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	return scanProposals(rows)
}

// NewestProposalByStatusForJob returns the most recent proposal for one job in one status —
// the binder's "which approved proposal does this channel bind to?" query.
//
// ⚠ NEWEST wins, and that is load-bearing rather than a tiebreak. A job legitimately accrues
// SEVERAL approved proposals over its life: a refine re-runs the channel's own job, and the
// channel must bind to the latest approved lineup, not the original (§7; asserted by
// TestRefine_NewestApprovedWins). The `ORDER BY created_at DESC` here is the same one
// `ListProposalsByStatus` applies — the caller used to read every approved proposal in the
// install and take the first match, relying on that ordering from a different method.
//
// ⚠ Filtered in SQL and indexed on job_id (00037) because retention deliberately never purges
// APPROVED proposals — they are the audit trail — so the table this scanned grows monotonically
// for the life of the install while denied ones are swept. Measured: 0.38ms at 100 rows, 3.45ms
// at 1k, 19.4ms at 5k, linear, on every bind including every scheduled auto-curate cycle.
func (s *sqlStore) NewestProposalByStatusForJob(ctx context.Context, jobID, status string) (Proposal, error) {
	row := s.db.QueryRowContext(ctx, s.ph(
		proposalSelect+` WHERE job_id = ? AND status = ? ORDER BY created_at DESC, id DESC LIMIT 1`), jobID, status)
	return scanProposal(row)
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
		p                                Proposal
		approvedAt, createdAt, updatedAt int64
	)
	err := sc.Scan(&p.ID, &p.JobID, &p.Status, &p.CreatedBy, &p.ApprovedBy, &p.DenyReason,
		&p.ModSummary, &p.Note, &p.ProposalJSON, &approvedAt, &createdAt, &updatedAt)
	if err == sql.ErrNoRows {
		return Proposal{}, ErrNotFound
	}
	if err != nil {
		return Proposal{}, err
	}
	p.ApprovedAt = fromEpoch(approvedAt)
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
