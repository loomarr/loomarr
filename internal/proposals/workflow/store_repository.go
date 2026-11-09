package workflow

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/loomarr/loomarr/internal/platform/store"
	"github.com/loomarr/loomarr/internal/platform/store/proposalstore"
	"github.com/loomarr/loomarr/internal/proposals"
)

func (r *storeRepository) ListIDs(ctx context.Context, ownerID string, all bool, limit int) ([]string, error) {
	if all {
		return r.store.ListProposalJobIDs(ctx, limit)
	}
	return r.store.ListProposalJobIDsByCreator(ctx, ownerID, limit)
}

type storeRepository struct {
	store      store.ProposalRepository
	newID      func() string
	now        func() time.Time
	intentHash func(proposals.Intent) string
}

// New installs the Store-backed durable workflow. Identifier and clock
// functions are injected so crash/replay tests stay deterministic, and so is the
// Intent hash that keys the generation cache: the suggester owns what makes two
// Intents the same request (suggest.IntentHash), and this package does not import it.
func New(st store.ProposalRepository, newID func() string, now func() time.Time, intentHash func(proposals.Intent) string) *Workflow {
	repository := &storeRepository{store: st, newID: newID, now: now, intentHash: intentHash}
	return newWorkflow(repository)
}

func (r *storeRepository) Load(ctx context.Context, jobID string) (Record, error) {
	snapshot, err := r.store.GetProposalJob(ctx, jobID)
	if err != nil {
		return Record{}, err
	}
	var intent proposals.Intent
	if err := json.Unmarshal([]byte(snapshot.Job.IntentJSON), &intent); err != nil {
		return Record{}, fmt.Errorf("decode Proposal Job %s Intent: %w", jobID, err)
	}
	record := Record{
		Version: snapshot.Job.WorkflowVersion, JobID: snapshot.Job.ID,
		OwnerID: snapshot.Job.CreatedBy, Status: JobStatus(snapshot.Job.Status), Intent: intent,
		ReachedLive: snapshot.Job.ReachedLive, CreatedAt: snapshot.Job.CreatedAt, UpdatedAt: snapshot.Job.UpdatedAt,
	}
	for _, persisted := range snapshot.Attempts {
		attempt := Attempt{
			Version: persisted.WorkflowVersion, Number: persisted.Attempt,
			Status: AttemptStatus(persisted.Status), StartedAt: persisted.StartedAt, CompletedAt: persisted.CompletedAt,
		}
		if persisted.Status == string(AttemptFailed) {
			failure := safeFailure(FailureCode(persisted.FailureCode))
			attempt.Failure = &failure
		}
		record.Attempts = append(record.Attempts, attempt)
	}
	if snapshot.Proposal != nil {
		record.Proposal = &ProposalRef{
			ID: snapshot.Proposal.ID, Status: ProposalStatus(snapshot.Proposal.Status),
			ApprovedBy: snapshot.Proposal.ApprovedBy, ApprovedAt: snapshot.Proposal.ApprovedAt, DenyReason: snapshot.Proposal.DenyReason,
			ModSummary: snapshot.Proposal.ModSummary, Note: snapshot.Proposal.Note, Proposal: snapshot.Proposal.Body,
		}
	}
	if snapshot.Channel != nil {
		record.Channel = &ChannelRef{
			ID: snapshot.Channel.ID, Status: snapshot.Channel.Status,
		}
	}
	if record.Status == JobFailed {
		record.FailureCode = FailureCode(snapshot.Job.FailureCode)
		record.Diagnostic = snapshot.Job.LastError
		if snapshot.Job.FailureTraceJSON != "" {
			if err := json.Unmarshal([]byte(snapshot.Job.FailureTraceJSON), &record.FailureTrace); err != nil {
				return Record{}, fmt.Errorf("decode Proposal Job %s failure trace: %w", jobID, err)
			}
			if err := proposals.ValidateDecisionTrace(record.FailureTrace); err != nil {
				return Record{}, fmt.Errorf("validate Proposal Job %s failure trace: %w", jobID, err)
			}
		}
	}
	return record, nil
}

func (r *storeRepository) ClaimAttempts(
	ctx context.Context,
	now time.Time,
	lease time.Duration,
	limit int,
) ([]Work, error) {
	jobs, err := r.store.ClaimDueJobs(ctx, now, lease, limit)
	if err != nil {
		return nil, err
	}
	works := make([]Work, 0, len(jobs))
	for _, job := range jobs {
		var intent proposals.Intent
		if err := json.Unmarshal([]byte(job.IntentJSON), &intent); err != nil {
			// A stored Intent that cannot decode never will: retrying it only strands the rest of
			// the batch under their leases. Fail this Job terminally and keep going.
			if ferr := r.store.CommitSuggestionFailure(
				ctx, job.ID, job.Attempts, fmt.Sprintf("decode stored Intent: %v", err),
				string(FailureGenerationFailed), "", r.now(),
			); ferr != nil {
				return nil, fmt.Errorf("fail Proposal Job %s with undecodable Intent: %w", job.ID, ferr)
			}
			continue
		}
		works = append(works, Work{
			Version: job.WorkflowVersion, JobID: job.ID, Attempt: job.Attempts,
			Kind: job.Kind, CreatedBy: job.CreatedBy, ChannelID: job.ChannelID, Intent: intent,
		})
	}
	return works, nil
}

func (r *storeRepository) CompleteAttempt(ctx context.Context, completion Completion) (CommittedProposal, error) {
	job, err := r.store.GetJob(ctx, completion.Work.JobID)
	if err != nil {
		return CommittedProposal{}, err
	}
	now := r.now()
	row := proposalstore.Proposal{
		ID: r.newID(), JobID: job.ID, Status: "submitted", CreatedBy: job.CreatedBy,
		Body: completion.Proposal, CreatedAt: now, UpdatedAt: now,
	}
	if err := r.store.CommitSuggestionSuccess(ctx, job.ID, completion.Work.Attempt, row, now); err != nil {
		if errors.Is(err, proposalstore.ErrJobNotRunning) {
			return CommittedProposal{}, ErrStaleAttempt
		}
		return CommittedProposal{}, err
	}
	return CommittedProposal{
		ID: row.ID, JobID: row.JobID, CreatedBy: row.CreatedBy,
		Proposal: completion.Proposal, CreatedAt: row.CreatedAt,
	}, nil
}

func (r *storeRepository) FailAttempt(ctx context.Context, failure AttemptFailure) error {
	if err := r.store.CommitSuggestionFailure(
		ctx, failure.Work.JobID, failure.Work.Attempt, failure.Diagnostic, string(failure.Code), failure.TraceJSON, r.now(),
	); err != nil {
		if errors.Is(err, proposalstore.ErrJobNotRunning) {
			return ErrStaleAttempt
		}
		return err
	}
	return nil
}
