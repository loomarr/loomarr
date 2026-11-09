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
	"github.com/loomarr/loomarr/internal/proposals/suggest"
)

const (
	jobKindSuggest  = "suggest"
	jobKindRecurate = "recurate"
)

// Record creates a fresh caller-owned lifecycle for a Proposal that needed no generation: a
// library channel idea whose titles the library already grounded (#1720). It lands exactly where a
// generated Proposal does (done, one succeeded Attempt, submitted for approval), so the queue,
// the requester's journey and the approval gate treat it the same. Every item must carry a
// grounded identity, as a generated Proposal's must.
func (w *Workflow) Record(
	ctx context.Context,
	intent proposals.Intent,
	prop proposals.Proposal,
	createdBy string,
) (suggest.WorkflowProposal, error) {
	if len(prop.Lineup) == 0 {
		return suggest.WorkflowProposal{}, fmt.Errorf("%w: a recorded Proposal needs a lineup", ErrInvalidState)
	}
	if err := validateProposalIdentities(prop); err != nil {
		return suggest.WorkflowProposal{}, err
	}
	return w.repository.RecordProposal(ctx, intent, prop, createdBy)
}

// Submit creates a fresh caller-owned lifecycle. cacheSince only saves the
// grounded result activity; identity, ownership, decisions, and history remain fresh.
func (w *Workflow) Submit(
	ctx context.Context,
	intent proposals.Intent,
	createdBy string,
	cacheSince time.Time,
) (suggest.WorkflowSubmission, error) {
	return w.repository.SubmitIntent(ctx, intent, createdBy, cacheSince)
}

// SubmitForChannel creates a fresh lifecycle that re-curates an existing Channel: a refine
// (kind suggest, owned by the person who asked) or a scheduled re-curation (kind recurate,
// ownerless). The Job names the Channel so approving its Proposal changes that Channel; it never
// touches the Job the Channel was made from, and it never reads the generation cache because a
// refine always regenerates.
func (w *Workflow) SubmitForChannel(
	ctx context.Context,
	channelID string,
	intent proposals.Intent,
	kind, createdBy string,
) (string, error) {
	if channelID == "" {
		return "", fmt.Errorf("%w: a channel Proposal Job needs a channel", ErrInvalidState)
	}
	if kind != jobKindSuggest && kind != jobKindRecurate {
		return "", fmt.Errorf("%w: unsupported Proposal Job kind %q", ErrInvalidState, kind)
	}
	return w.repository.SubmitChannelIntent(ctx, channelID, intent, kind, createdBy)
}

// Revise replaces the Intent behind the exact submitted Proposal an authorized
// operator is reviewing. The repository performs the proposal/job transition as
// one compare-and-swap; the prior Proposal stays visible until a replacement wins.
func (w *Workflow) Revise(ctx context.Context, viewer Viewer, jobID string, intent proposals.Intent) error {
	journey, err := w.Inspect(ctx, viewer, jobID)
	if err != nil {
		return err
	}
	if journey.Proposal == nil || journey.Proposal.Status != ProposalSubmitted {
		return ErrNotRevisable
	}
	if err := w.repository.ReviseIntent(ctx, jobID, journey.Proposal.ID, len(journey.Attempts), intent); err != nil {
		return fmt.Errorf("revise Proposal Job %q: %w", jobID, err)
	}
	return nil
}

func (r *storeRepository) SubmitIntent(
	ctx context.Context,
	intent proposals.Intent,
	createdBy string,
	cacheSince time.Time,
) (suggest.WorkflowSubmission, error) {
	blob, err := json.Marshal(intent)
	if err != nil {
		return suggest.WorkflowSubmission{}, fmt.Errorf("marshal Intent: %w", err)
	}
	now := r.now()
	job := proposalstore.Job{
		ID: r.newID(), Kind: jobKindSuggest, Status: "queued", IntentJSON: string(blob),
		IntentHash: suggest.IntentHash(intent), CreatedBy: createdBy,
		WorkflowVersion: WorkflowVersion1, Deadline: now, CreatedAt: now, UpdatedAt: now,
	}
	if cached, findErr := r.store.FindJobByIntentHash(ctx, job.IntentHash, cacheSince); findErr == nil && cached.Status == "done" {
		job.Status = "done"
		cloned, cloneErr := r.store.CloneSuggestionSuccess(ctx, cached.ID, job, r.newID())
		if cloneErr == nil {
			return suggest.WorkflowSubmission{JobID: job.ID, CachedProposal: &suggest.WorkflowProposal{
				ID: cloned.ID, JobID: job.ID, CreatedBy: createdBy, Proposal: cloned.Body, CreatedAt: cloned.CreatedAt,
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

func (r *storeRepository) RecordProposal(
	ctx context.Context,
	intent proposals.Intent,
	prop proposals.Proposal,
	createdBy string,
) (suggest.WorkflowProposal, error) {
	prop.Intent = intent
	intentBlob, err := json.Marshal(intent)
	if err != nil {
		return suggest.WorkflowProposal{}, fmt.Errorf("marshal Intent: %w", err)
	}
	now := r.now()
	job := proposalstore.Job{
		ID: r.newID(), Kind: jobKindSuggest, Status: "done", IntentJSON: string(intentBlob),
		IntentHash: suggest.IntentHash(intent), CreatedBy: createdBy,
		WorkflowVersion: WorkflowVersion1, Deadline: now, CreatedAt: now, UpdatedAt: now,
	}
	row := proposalstore.Proposal{
		ID: r.newID(), JobID: job.ID, Status: "submitted", CreatedBy: createdBy,
		Body: prop, CreatedAt: now, UpdatedAt: now,
	}
	if err := r.store.CreateSuggestionResult(ctx, job, row); err != nil {
		return suggest.WorkflowProposal{}, err
	}
	return suggest.WorkflowProposal{
		ID: row.ID, JobID: job.ID, CreatedBy: createdBy, Proposal: prop, CreatedAt: now,
	}, nil
}

func (r *storeRepository) SubmitChannelIntent(
	ctx context.Context,
	channelID string,
	intent proposals.Intent,
	kind, createdBy string,
) (string, error) {
	blob, err := json.Marshal(intent)
	if err != nil {
		return "", fmt.Errorf("marshal Intent: %w", err)
	}
	now := r.now()
	job := proposalstore.Job{
		ID: r.newID(), Kind: kind, Status: "queued", IntentJSON: string(blob),
		IntentHash: suggest.IntentHash(intent), CreatedBy: createdBy, ChannelID: channelID,
		WorkflowVersion: WorkflowVersion1, Deadline: now, CreatedAt: now, UpdatedAt: now,
	}
	if err := r.store.CreateJob(ctx, job); err != nil {
		return "", err
	}
	return job.ID, nil
}

func (r *storeRepository) ReviseIntent(
	ctx context.Context,
	jobID, proposalID string,
	expectedAttempt int,
	intent proposals.Intent,
) error {
	blob, err := json.Marshal(intent)
	if err != nil {
		return fmt.Errorf("marshal Intent: %w", err)
	}
	now := r.now()
	if err := r.store.ReviseSubmittedProposal(
		ctx, jobID, proposalID, expectedAttempt, string(blob), suggest.IntentHash(intent), now, now,
	); err != nil {
		return err
	}
	return nil
}
