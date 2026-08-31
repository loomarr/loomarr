package proposalworkflow

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/loomarr/loomarr/internal/store"
	"github.com/loomarr/loomarr/internal/suggest"
)

// Store is the private persistence port used by Workflow. The application
// passes its ordinary store.Store; callers never receive this lower-level seam.
type Store interface {
	GetJob(context.Context, string) (store.Job, error)
	GetProposalJob(context.Context, string) (store.ProposalJob, error)
	ListProposalJobIDs(context.Context, int) ([]string, error)
	ListProposalJobIDsByCreator(context.Context, string, int) ([]string, error)
	CreateJob(context.Context, store.Job) error
	FindJobByIntentHash(context.Context, string, time.Time) (store.Job, error)
	CloneSuggestionSuccess(context.Context, string, store.Job, string) (store.Proposal, error)
	RequeueSuggestionJob(context.Context, string, int, string, string, string, time.Time, time.Time) error
	ClaimDueJobs(context.Context, time.Time, time.Duration, int) ([]store.Job, error)
	CommitSuggestionSuccess(context.Context, string, int, store.Proposal, time.Time) error
	CommitSuggestionFailure(context.Context, string, int, string, string, string, time.Time) error
}

func (r *storeRepository) ListIDs(ctx context.Context, ownerID string, all bool, limit int) ([]string, error) {
	if all {
		return r.store.ListProposalJobIDs(ctx, limit)
	}
	return r.store.ListProposalJobIDsByCreator(ctx, ownerID, limit)
}

type storeRepository struct {
	store Store
	newID func() string
	now   func() time.Time
}

// New installs the Store-backed durable workflow. Identifier and clock
// functions are injected so crash/replay tests stay deterministic.
func New(st Store, newID func() string, now func() time.Time) *Workflow {
	repository := &storeRepository{store: st, newID: newID, now: now}
	return newWorkflow(repository)
}

func (r *storeRepository) Load(ctx context.Context, jobID string) (Record, error) {
	snapshot, err := r.store.GetProposalJob(ctx, jobID)
	if err != nil {
		return Record{}, err
	}
	var intent suggest.Intent
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
		var proposal suggest.Proposal
		if err := json.Unmarshal([]byte(snapshot.Proposal.ProposalJSON), &proposal); err != nil {
			return Record{}, fmt.Errorf("decode Proposal Job %s Proposal: %w", jobID, err)
		}
		record.Proposal = &ProposalRef{
			ID: snapshot.Proposal.ID, Status: ProposalStatus(snapshot.Proposal.Status),
			ApprovedBy: snapshot.Proposal.ApprovedBy, DenyReason: snapshot.Proposal.DenyReason,
			ModSummary: snapshot.Proposal.ModSummary, Note: snapshot.Proposal.Note, Proposal: proposal,
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
			if err := suggest.ValidateDecisionTrace(record.FailureTrace); err != nil {
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
		var intent suggest.Intent
		if err := json.Unmarshal([]byte(job.IntentJSON), &intent); err != nil {
			return nil, fmt.Errorf("decode claimed Proposal Job %s Intent: %w", job.ID, err)
		}
		works = append(works, Work{
			Version: job.WorkflowVersion, JobID: job.ID, Attempt: job.Attempts,
			Kind: job.Kind, CreatedBy: job.CreatedBy, Intent: intent,
		})
	}
	return works, nil
}

func (r *storeRepository) CompleteAttempt(ctx context.Context, completion Completion) (suggest.WorkflowProposal, error) {
	job, err := r.store.GetJob(ctx, completion.Work.JobID)
	if err != nil {
		return suggest.WorkflowProposal{}, err
	}
	blob, err := json.Marshal(completion.Proposal)
	if err != nil {
		return suggest.WorkflowProposal{}, fmt.Errorf("encode Proposal: %w", err)
	}
	now := r.now()
	proposal := store.Proposal{
		ID: r.newID(), JobID: job.ID, Status: "submitted", CreatedBy: job.CreatedBy,
		ProposalJSON: string(blob), CreatedAt: now, UpdatedAt: now,
	}
	if err := r.store.CommitSuggestionSuccess(ctx, job.ID, completion.Work.Attempt, proposal, now); err != nil {
		if errors.Is(err, store.ErrJobNotRunning) {
			return suggest.WorkflowProposal{}, ErrStaleAttempt
		}
		return suggest.WorkflowProposal{}, err
	}
	return suggest.WorkflowProposal{
		ID: proposal.ID, JobID: proposal.JobID, CreatedBy: proposal.CreatedBy,
		Proposal: completion.Proposal, CreatedAt: proposal.CreatedAt,
	}, nil
}

func (r *storeRepository) FailAttempt(ctx context.Context, failure AttemptFailure) error {
	if err := r.store.CommitSuggestionFailure(
		ctx, failure.Work.JobID, failure.Work.Attempt, failure.Diagnostic, string(failure.Code), failure.TraceJSON, r.now(),
	); err != nil {
		if errors.Is(err, store.ErrJobNotRunning) {
			return ErrStaleAttempt
		}
		return err
	}
	return nil
}
