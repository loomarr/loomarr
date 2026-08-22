package proposalworkflow

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/mantonx/loomarr/internal/store"
	"github.com/mantonx/loomarr/internal/suggest"
)

const (
	jobKindSuggest  = "suggest"
	jobKindRecurate = "recurate"
)

type submissionRepository interface {
	SubmitIntent(context.Context, suggest.Intent, string, time.Time) (suggest.WorkflowSubmission, error)
	RequeueIntent(context.Context, string, suggest.Intent, string) error
}

// Submit creates a fresh caller-owned lifecycle. cacheSince only saves the
// grounded result activity; identity, ownership, decisions, and history remain fresh.
func (w *Workflow) Submit(
	ctx context.Context,
	intent suggest.Intent,
	createdBy string,
	cacheSince time.Time,
) (suggest.WorkflowSubmission, error) {
	repository, ok := w.repository.(submissionRepository)
	if !ok {
		return suggest.WorkflowSubmission{}, fmt.Errorf("%w: submission repository unavailable", ErrInvalidState)
	}
	return repository.SubmitIntent(ctx, intent, createdBy, cacheSince)
}

// Requeue keeps a Channel's stable intent reference while replacing only its
// terminal execution. The repository compare-and-swap rejects concurrent reruns.
func (w *Workflow) Requeue(ctx context.Context, jobID string, intent suggest.Intent, kind string) error {
	if kind != jobKindSuggest && kind != jobKindRecurate {
		return fmt.Errorf("%w: unsupported Proposal Job kind %q", ErrInvalidState, kind)
	}
	repository, ok := w.repository.(submissionRepository)
	if !ok {
		return fmt.Errorf("%w: submission repository unavailable", ErrInvalidState)
	}
	return repository.RequeueIntent(ctx, jobID, intent, kind)
}

func (r *storeRepository) SubmitIntent(
	ctx context.Context,
	intent suggest.Intent,
	createdBy string,
	cacheSince time.Time,
) (suggest.WorkflowSubmission, error) {
	blob, err := json.Marshal(intent)
	if err != nil {
		return suggest.WorkflowSubmission{}, fmt.Errorf("marshal Intent: %w", err)
	}
	now := r.now()
	job := store.Job{
		ID: r.newID(), Kind: jobKindSuggest, Status: "queued", IntentJSON: string(blob),
		IntentHash: suggest.IntentHash(intent), CreatedBy: createdBy,
		WorkflowVersion: WorkflowVersion1, Deadline: now, CreatedAt: now, UpdatedAt: now,
	}
	if cached, findErr := r.store.FindJobByIntentHash(ctx, job.IntentHash, cacheSince); findErr == nil && cached.Status == "done" {
		job.Status = "done"
		proposal, cloneErr := r.store.CloneSuggestionSuccess(ctx, cached.ID, job, r.newID())
		if cloneErr == nil {
			var payload suggest.Proposal
			if err := json.Unmarshal([]byte(proposal.ProposalJSON), &payload); err != nil {
				return suggest.WorkflowSubmission{}, fmt.Errorf("decode cached Proposal: %w", err)
			}
			return suggest.WorkflowSubmission{JobID: job.ID, CachedProposal: &suggest.WorkflowProposal{
				ID: proposal.ID, JobID: job.ID, CreatedBy: createdBy, Proposal: payload, CreatedAt: proposal.CreatedAt,
			}}, nil
		}
		if !errors.Is(cloneErr, store.ErrNotFound) {
			return suggest.WorkflowSubmission{}, cloneErr
		}
		job.Status = "queued"
	}
	if err := r.store.CreateJob(ctx, job); err != nil {
		return suggest.WorkflowSubmission{}, err
	}
	return suggest.WorkflowSubmission{JobID: job.ID}, nil
}

func (r *storeRepository) RequeueIntent(ctx context.Context, jobID string, intent suggest.Intent, kind string) error {
	job, err := r.store.GetJob(ctx, jobID)
	if err != nil {
		return fmt.Errorf("load Proposal Job %q: %w", jobID, err)
	}
	blob, err := json.Marshal(intent)
	if err != nil {
		return fmt.Errorf("marshal Intent: %w", err)
	}
	now := r.now()
	if err := r.store.RequeueSuggestionJob(
		ctx, job.ID, job.Attempts, kind, string(blob), suggest.IntentHash(intent), now, now,
	); err != nil {
		return fmt.Errorf("requeue Proposal Job %q: %w", jobID, err)
	}
	return nil
}
