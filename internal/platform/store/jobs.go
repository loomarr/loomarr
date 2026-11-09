package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/loomarr/loomarr/internal/platform/store/channelstore"
	"github.com/loomarr/loomarr/internal/platform/store/storedb"
	"github.com/loomarr/loomarr/internal/proposals"
)

// Job is a persisted suggester generation task (§8). The worker pool claims
// queued jobs via ClaimDueJobs and runs them; the IntentJSON blob carries the
// proposals.Intent (like titles.title_json). IntentHash is the cache key.
type Job struct {
	ID         string
	Kind       string // "suggest" (human/user flow) or "recurate" (scheduled channel grant)
	Status     string // queued | running | done | failed
	IntentJSON string // "" is stored as the column's default, the empty intent {}
	IntentHash string
	CreatedBy  string
	// ChannelID names the channel a refine or re-curation job re-curates; its approval changes
	// that channel instead of creating one. Empty for a job that originates a channel, which the
	// channel's `intent_ref` binds.
	ChannelID        string
	LastError        string
	FailureCode      string
	FailureTraceJSON string
	WorkflowVersion  int
	ReachedLive      bool
	Deadline         time.Time
	Attempts         int
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

const ProposalWorkflowVersion = 1

// ProposalJobAttempt is one exact execution lease. Attempt is a monotonically
// increasing compare-and-swap token within its Job.
type ProposalJobAttempt struct {
	JobID           string
	Attempt         int
	WorkflowVersion int
	Status          string
	StartedAt       time.Time
	CompletedAt     time.Time
	FailureCode     string
}

// ProposalJob is the consistent read projection for one generation execution.
// Proposal is nil until the current execution is done; it may then be submitted,
// approved, or denied independently from the job lifecycle.
type ProposalJob struct {
	Job      Job
	Attempts []ProposalJobAttempt
	Proposal *Proposal
	Channel  *Channel
}

// Proposal is a persisted suggester output (§8). Status drives the approval queue
// (submitted → approved/denied); CreatedBy powers My proposals (§12); ApprovedBy
// records whoever decided it, approver or denier, in the `decided_by` column (§8 audit). Body is the proposals.Proposal, persisted as one JSON column.
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
	Note string
	// Body is the proposal itself. The store encodes it on write and decodes it once on read,
	// so nothing above the store handles the stored JSON.
	Body proposals.Proposal
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
		`INSERT INTO proposal_jobs (id, kind, status, intent_json, intent_hash, created_by, last_error, failure_code,
			workflow_version, reached_live, deadline, attempts, created_at, updated_at, failure_trace_json, channel_id)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`),
		j.ID, j.Kind, j.Status, jobIntent(j.IntentJSON), j.IntentHash, j.CreatedBy, j.LastError, j.FailureCode,
		workflowVersionForCreate(j.WorkflowVersion), j.ReachedLive, epoch(j.Deadline), j.Attempts,
		epoch(j.CreatedAt), epoch(j.UpdatedAt), storedb.OptionalJSON(j.FailureTraceJSON), j.ChannelID)
	if err != nil {
		return fmt.Errorf("create job %s: %w", j.ID, err)
	}
	return nil
}

const jobSelect = `SELECT id, kind, status, intent_json, intent_hash, created_by, last_error, failure_code,
	workflow_version, reached_live, deadline, attempts, created_at, updated_at, failure_trace_json, channel_id FROM proposal_jobs`

func (s *sqlStore) GetJob(ctx context.Context, id string) (Job, error) {
	return scanJob(s.db.QueryRowContext(ctx, s.ph(jobSelect+` WHERE id = ?`), id))
}

func (s *sqlStore) ListProposalJobIDs(ctx context.Context, limit int) ([]string, error) {
	return s.listProposalJobIDs(ctx, "", false, limit)
}

func (s *sqlStore) ListProposalJobIDsByCreator(ctx context.Context, createdBy string, limit int) ([]string, error) {
	return s.listProposalJobIDs(ctx, createdBy, true, limit)
}

func (s *sqlStore) listProposalJobIDs(ctx context.Context, createdBy string, scoped bool, limit int) ([]string, error) {
	if limit <= 0 {
		return []string{}, nil
	}
	query := `SELECT id FROM proposal_jobs WHERE kind = 'suggest'`
	args := []any{}
	if scoped {
		query += ` AND created_by = ?`
		args = append(args, createdBy)
	}
	query += ` ORDER BY updated_at DESC, id DESC LIMIT ?`
	args = append(args, limit)
	rows, err := s.db.QueryContext(ctx, s.ph(query), args...)
	if err != nil {
		return nil, fmt.Errorf("list Proposal Job ids: %w", err)
	}
	defer func() { _ = rows.Close() }()
	ids := make([]string, 0)
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("list Proposal Job ids: %w", err)
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list Proposal Job ids: %w", err)
	}
	return ids, nil
}

func (s *sqlStore) GetProposalJob(ctx context.Context, id string) (ProposalJob, error) {
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return ProposalJob{}, fmt.Errorf("get proposal job %s: begin snapshot: %w", id, err)
	}
	defer func() { _ = tx.Rollback() }()

	row := tx.QueryRowContext(ctx, s.ph(
		`SELECT j.id, j.kind, j.status, j.intent_json, j.intent_hash, j.created_by, j.last_error, j.failure_code,
		        j.workflow_version, j.reached_live, j.deadline, j.attempts, j.created_at, j.updated_at, j.failure_trace_json,
		        j.channel_id,
		        p.id, p.job_id, p.status, p.created_by, p.decided_by, p.deny_reason,
		        p.mod_summary, p.note, p.proposal_json, p.approved_at, p.created_at, p.updated_at
		   FROM proposal_jobs j
		   LEFT JOIN proposals p
		     ON (j.status = 'done' OR p.status = 'submitted')
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
	err = row.Scan(
		&out.Job.ID, &out.Job.Kind, &out.Job.Status, &out.Job.IntentJSON, &out.Job.IntentHash,
		&out.Job.CreatedBy, &out.Job.LastError, &out.Job.FailureCode, &out.Job.WorkflowVersion, &out.Job.ReachedLive,
		&deadline, &out.Job.Attempts, &jobCreatedAt, &jobUpdatedAt, (*storedb.OptionalJSON)(&out.Job.FailureTraceJSON),
		&out.Job.ChannelID,
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
		body, err := decodeProposalBody(pID.String, pJSON.String)
		if err != nil {
			return ProposalJob{}, fmt.Errorf("get proposal job %s: %w", id, err)
		}
		out.Proposal = &Proposal{
			ID: pID.String, JobID: pJobID.String, Status: pStatus.String, CreatedBy: pCreatedBy.String,
			ApprovedBy: pApprovedBy.String, DenyReason: pDenyReason.String, ModSummary: pModSummary.String,
			Note: pNote.String, Body: body,
			ApprovedAt: fromEpoch(pApprovedAt.Int64), CreatedAt: fromEpoch(pCreatedAt.Int64),
			UpdatedAt: fromEpoch(pUpdatedAt.Int64),
		}
	}
	out.Attempts, err = listProposalJobAttempts(ctx, tx, s.ph, id)
	if err != nil {
		return ProposalJob{}, fmt.Errorf("get proposal job %s: %w", id, err)
	}
	channel, channelErr := channelstore.ChannelForJobIn(ctx, tx, s.ph, id)
	if channelErr == nil {
		out.Channel = &channel
	} else if !errors.Is(channelErr, ErrNotFound) && !errors.Is(channelErr, ErrJobChannelGone) {
		return ProposalJob{}, fmt.Errorf("get proposal job %s Channel: %w", id, channelErr)
	}
	if err := tx.Commit(); err != nil {
		return ProposalJob{}, fmt.Errorf("get proposal job %s: commit snapshot: %w", id, err)
	}
	return out, nil
}

func (s *sqlStore) UpdateJob(ctx context.Context, j Job) error {
	_, err := s.db.ExecContext(ctx, s.ph(
		`UPDATE proposal_jobs SET kind=?, status=?, intent_json=?, intent_hash=?, created_by=?,
		   last_error=?, failure_code=?, workflow_version=?, reached_live=?, deadline=?, attempts=?, updated_at=?, failure_trace_json=? WHERE id=?`),
		j.Kind, j.Status, jobIntent(j.IntentJSON), j.IntentHash, j.CreatedBy, j.LastError, j.FailureCode,
		j.WorkflowVersion, j.ReachedLive, epoch(j.Deadline), j.Attempts, epoch(j.UpdatedAt), storedb.OptionalJSON(j.FailureTraceJSON), j.ID)
	if err != nil {
		return fmt.Errorf("update job %s: %w", j.ID, err)
	}
	return nil
}

// ClaimDueJobs atomically starts and leases due queued jobs (§8/§18).
// Placeholders: 1=leaseUntil, 2=now, 3=limit.
func (s *sqlStore) ClaimDueJobs(ctx context.Context, now time.Time, lease time.Duration, limit int) ([]Job, error) {
	if limit <= 0 || lease <= 0 {
		return nil, nil
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("claim due jobs: begin: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	query := storedb.LockRowsSkipLocked(s.dialect, jobSelect+` WHERE status IN ('queued', 'running') AND deadline <= ? AND deadline > 0
		ORDER BY deadline LIMIT ?`)
	rows, err := tx.QueryContext(ctx, s.ph(query), epoch(now), limit)
	if err != nil {
		return nil, fmt.Errorf("claim due jobs: select: %w", err)
	}
	jobs, err := scanJobs(rows)
	closeErr := rows.Close()
	if err != nil {
		return nil, fmt.Errorf("claim due jobs: scan: %w", err)
	}
	if closeErr != nil {
		return nil, fmt.Errorf("claim due jobs: close rows: %w", closeErr)
	}

	for i := range jobs {
		job := &jobs[i]
		if job.WorkflowVersion == ProposalWorkflowVersion && job.Status == "running" {
			result, err := tx.ExecContext(ctx, s.ph(
				`UPDATE proposal_job_attempts
				    SET status='interrupted', completed_at=?
				  WHERE job_id=? AND attempt=? AND workflow_version=? AND status='running'`),
				epoch(now), job.ID, job.Attempts, ProposalWorkflowVersion)
			if err != nil {
				return nil, fmt.Errorf("claim due jobs: interrupt %s Attempt %d: %w", job.ID, job.Attempts, err)
			}
			affected, err := result.RowsAffected()
			if err != nil || affected != 1 {
				return nil, fmt.Errorf("claim due jobs: %s current Attempt %d is missing or terminal", job.ID, job.Attempts)
			}
		}

		nextAttempt := job.Attempts + 1
		if job.WorkflowVersion == 0 {
			nextAttempt = 1
		}
		result, err := tx.ExecContext(ctx, s.ph(
			`UPDATE proposal_jobs
			    SET status='running', workflow_version=?, attempts=?, deadline=?, updated_at=?
			  WHERE id=? AND status IN ('queued', 'running') AND deadline <= ? AND deadline > 0`),
			ProposalWorkflowVersion, nextAttempt, epoch(now.Add(lease)), epoch(now), job.ID, epoch(now))
		if err != nil {
			return nil, fmt.Errorf("claim due jobs: start %s: %w", job.ID, err)
		}
		affected, err := result.RowsAffected()
		if err != nil || affected != 1 {
			return nil, fmt.Errorf("claim due jobs: lost locked Job %s", job.ID)
		}
		if _, err := tx.ExecContext(ctx, s.ph(
			`INSERT INTO proposal_job_attempts
			    (job_id, attempt, workflow_version, status, started_at, completed_at, failure_code)
			 VALUES (?, ?, ?, 'running', ?, 0, '')`),
			job.ID, nextAttempt, ProposalWorkflowVersion, epoch(now)); err != nil {
			return nil, fmt.Errorf("claim due jobs: create %s Attempt %d: %w", job.ID, nextAttempt, err)
		}
		job.Status = "running"
		job.WorkflowVersion = ProposalWorkflowVersion
		job.Attempts = nextAttempt
		job.Deadline = now.Add(lease)
		job.UpdatedAt = now
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("claim due jobs: commit: %w", err)
	}
	return jobs, nil
}

const proposalJobAttemptSelect = `SELECT job_id, attempt, workflow_version, status,
	started_at, completed_at, failure_code FROM proposal_job_attempts`

func (s *sqlStore) ListProposalJobAttempts(ctx context.Context, jobID string) ([]ProposalJobAttempt, error) {
	return listProposalJobAttempts(ctx, s.db, s.ph, jobID)
}

type attemptQueryer interface {
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
}

func listProposalJobAttempts(
	ctx context.Context,
	queryer attemptQueryer,
	placeholder func(string) string,
	jobID string,
) ([]ProposalJobAttempt, error) {
	rows, err := queryer.QueryContext(ctx, placeholder(
		proposalJobAttemptSelect+` WHERE job_id = ? ORDER BY attempt`), jobID)
	if err != nil {
		return nil, fmt.Errorf("list Proposal Job Attempts for %s: %w", jobID, err)
	}
	defer func() { _ = rows.Close() }()
	var attempts []ProposalJobAttempt
	for rows.Next() {
		var attempt ProposalJobAttempt
		var startedAt, completedAt int64
		if err := rows.Scan(&attempt.JobID, &attempt.Attempt, &attempt.WorkflowVersion,
			&attempt.Status, &startedAt, &completedAt, &attempt.FailureCode); err != nil {
			return nil, err
		}
		attempt.StartedAt = fromEpoch(startedAt)
		attempt.CompletedAt = fromEpoch(completedAt)
		attempts = append(attempts, attempt)
	}
	return attempts, rows.Err()
}

// FindJobByIntentHash returns the most recent successful job with a matching
// intent hash (§8 cache). `since` bounds the TTL: only jobs created at/after
// `since` count. Queued, running, and failed attempts cannot shadow reusable
// content from a completed job. Returns ErrNotFound when no success qualifies.
func (s *sqlStore) FindJobByIntentHash(ctx context.Context, hash string, since time.Time) (Job, error) {
	row := s.db.QueryRowContext(ctx, s.ph(jobSelect+
		` WHERE intent_hash = ? AND status = 'done' AND created_at >= ? ORDER BY created_at DESC LIMIT 1`),
		hash, epoch(since))
	return scanJob(row)
}

func scanJob(sc scannable) (Job, error) {
	var (
		j                              Job
		deadline, createdAt, updatedAt int64
	)
	err := sc.Scan(&j.ID, &j.Kind, &j.Status, &j.IntentJSON, &j.IntentHash, &j.CreatedBy,
		&j.LastError, &j.FailureCode, &j.WorkflowVersion, &j.ReachedLive, &deadline, &j.Attempts, &createdAt, &updatedAt, (*storedb.OptionalJSON)(&j.FailureTraceJSON), &j.ChannelID)
	if errors.Is(err, sql.ErrNoRows) {
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

func workflowVersionForCreate(version int) int {
	if version == 0 {
		return ProposalWorkflowVersion
	}
	return version
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
	body, err := encodeProposalBody(p)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, s.ph(
		`INSERT INTO proposals (id, job_id, status, created_by, decided_by, deny_reason, mod_summary, note, proposal_json, approved_at, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`),
		p.ID, p.JobID, p.Status, p.CreatedBy, p.ApprovedBy, p.DenyReason, p.ModSummary, p.Note,
		body, epoch(p.ApprovedAt), epoch(p.CreatedAt), epoch(p.UpdatedAt))
	if err != nil {
		return fmt.Errorf("create proposal %s: %w", p.ID, err)
	}
	return nil
}

// jobIntent is the intent_json a job stores: its intent, or the column's default when it has none.
// The column holds JSON on both engines (#2793), and "" is not JSON.
func jobIntent(intent string) string {
	if intent == "" {
		return "{}"
	}
	return intent
}

// encodeProposalBody is the one place a proposal becomes its stored JSON.
func encodeProposalBody(p Proposal) (string, error) {
	raw, err := json.Marshal(p.Body)
	if err != nil {
		return "", fmt.Errorf("encode proposal %s: %w", p.ID, err)
	}
	return string(raw), nil
}

// decodeProposalBody is the one place stored proposal JSON becomes a proposals.Proposal. The column
// is always JSON (#2793), but JSON that isn't a proposal is an error: a caller must never act on a
// proposal it could not read.
func decodeProposalBody(id, raw string) (proposals.Proposal, error) {
	var body proposals.Proposal
	if err := json.Unmarshal([]byte(raw), &body); err != nil {
		return proposals.Proposal{}, fmt.Errorf("decode proposal %s: %w", id, err)
	}
	return body, nil
}

const proposalSelect = `SELECT id, job_id, status, created_by, decided_by, deny_reason,
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
// approval's "which approved proposal does this job's channel bind to?" query.
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
		bodyJSON                         string
		approvedAt, createdAt, updatedAt int64
	)
	err := sc.Scan(&p.ID, &p.JobID, &p.Status, &p.CreatedBy, &p.ApprovedBy, &p.DenyReason,
		&p.ModSummary, &p.Note, &bodyJSON, &approvedAt, &createdAt, &updatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return Proposal{}, ErrNotFound
	}
	if err != nil {
		return Proposal{}, err
	}
	if p.Body, err = decodeProposalBody(p.ID, bodyJSON); err != nil {
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
