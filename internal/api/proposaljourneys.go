package api

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/danielgtaylor/huma/v2"

	"github.com/mantonx/loomarr/internal/proposalworkflow"
	"github.com/mantonx/loomarr/internal/store"
	"github.com/mantonx/loomarr/internal/suggest"
)

// ProposalWorkflow is the sole authoritative read seam for the first-channel
// journey. HTTP handlers never join raw Job, Proposal, and Channel records.
type ProposalWorkflow interface {
	Inspect(context.Context, proposalworkflow.Viewer, string) (proposalworkflow.Journey, error)
}

type ProposalJourneyDTO struct {
	Version   int                         `json:"version" doc:"Current First-channel Journey schema version"`
	JobID     string                      `json:"jobId"`
	Milestone string                      `json:"milestone" enum:"generating,awaiting_approval,denied,building,live,failed"`
	Intent    suggest.Intent              `json:"intent"`
	Attempts  []ProposalJobAttemptDTO     `json:"attempts"`
	Failure   *ProposalJourneyFailureDTO  `json:"failure,omitempty"`
	Proposal  *ProposalJourneyProposalDTO `json:"proposal,omitempty"`
	Channel   *ProposalJourneyChannelDTO  `json:"channel,omitempty"`
	Actions   []string                    `json:"actions" doc:"Server-authorized next actions"`
	CreatedAt time.Time                   `json:"createdAt"`
	UpdatedAt time.Time                   `json:"updatedAt"`
}

type ProposalJobAttemptDTO struct {
	Version     int                        `json:"version"`
	Number      int                        `json:"number"`
	Status      string                     `json:"status" enum:"running,succeeded,failed,interrupted"`
	StartedAt   time.Time                  `json:"startedAt"`
	CompletedAt time.Time                  `json:"completedAt,omitempty"`
	Failure     *ProposalJourneyFailureDTO `json:"failure,omitempty"`
}

type ProposalJourneyFailureDTO struct {
	Code    string `json:"code" enum:"no_grounded_titles,generation_failed"`
	Message string `json:"message"`
}

type ProposalJourneyProposalDTO struct {
	ID       string           `json:"id"`
	Status   string           `json:"status" enum:"submitted,approved,denied"`
	Proposal suggest.Proposal `json:"proposal"`
}

type ProposalJourneyChannelDTO struct {
	ID     string `json:"id"`
	Status string `json:"status" enum:"building,live,empty,drifted,detached,paused"`
}

func (s *Server) registerProposalJourneys(api huma.API) {
	huma.Register(api, withRole(huma.Operation{
		OperationID: "get-proposal-job", Method: http.MethodGet, Path: "/v1/proposal-jobs/{jobId}",
		Summary:     "Get the authoritative First-channel Journey",
		Description: "Restores one versioned Proposal Job, bounded Attempt history, safe failure, Proposal, intent-bound Channel, milestone, and server-authorized actions. Members may read only their own Job; admins may read any.",
		Tags:        []string{"proposal-jobs"},
	}, RoleMember), s.getProposalJourney)
}

type proposalJourneyInput struct {
	JobID string `path:"jobId"`
}

type proposalJourneyOutput struct {
	Body ProposalJourneyDTO
}

func (s *Server) getProposalJourney(ctx context.Context, in *proposalJourneyInput) (*proposalJourneyOutput, error) {
	if s.proposalWorkflow == nil {
		return nil, huma.Error501NotImplemented("Proposal workflow is not configured")
	}
	viewer := proposalworkflow.Viewer{Admin: roleFrom(ctx) == RoleAdmin}
	if user, ok := userFrom(ctx); ok {
		viewer.UserID = user.ID
	}
	journey, err := s.proposalWorkflow.Inspect(ctx, viewer, in.JobID)
	switch {
	case errors.Is(err, store.ErrNotFound):
		return nil, errNotFound("Channel request not found", "That channel request doesn't exist or has expired.")
	case errors.Is(err, proposalworkflow.ErrForbidden):
		return nil, apiErr(http.StatusForbidden, "Channel request unavailable",
			"You can only view channel requests submitted by your account.")
	case errors.Is(err, proposalworkflow.ErrInvalidState):
		return nil, apiErrWithCause(http.StatusInternalServerError, "Couldn't restore the channel request",
			"Loomarr found inconsistent saved workflow state and stopped rather than guessing. Check the server logs.", err)
	case err != nil:
		return nil, apiErrWithCause(http.StatusInternalServerError, "Couldn't read the channel request",
			"Loomarr couldn't restore this channel request. Try again in a moment.", err)
	}
	return &proposalJourneyOutput{Body: proposalJourneyDTO(journey)}, nil
}

func proposalJourneyDTO(journey proposalworkflow.Journey) ProposalJourneyDTO {
	dto := ProposalJourneyDTO{
		Version: journey.Version, JobID: journey.JobID, Milestone: string(journey.Milestone),
		Intent: journey.Intent, Attempts: make([]ProposalJobAttemptDTO, 0, len(journey.Attempts)),
		Actions: make([]string, len(journey.Actions)), CreatedAt: journey.CreatedAt, UpdatedAt: journey.UpdatedAt,
	}
	for i, action := range journey.Actions {
		dto.Actions[i] = string(action)
	}
	for _, attempt := range journey.Attempts {
		dto.Attempts = append(dto.Attempts, ProposalJobAttemptDTO{
			Version: attempt.Version, Number: attempt.Number, Status: string(attempt.Status),
			StartedAt: attempt.StartedAt, CompletedAt: attempt.CompletedAt,
			Failure: proposalJourneyFailureDTO(attempt.Failure),
		})
	}
	dto.Failure = proposalJourneyFailureDTO(journey.Failure)
	if journey.Proposal != nil {
		dto.Proposal = &ProposalJourneyProposalDTO{
			ID: journey.Proposal.ID, Status: string(journey.Proposal.Status), Proposal: journey.Proposal.Proposal,
		}
	}
	if journey.Channel != nil {
		dto.Channel = &ProposalJourneyChannelDTO{ID: journey.Channel.ID, Status: string(journey.Channel.Status)}
	}
	return dto
}

func proposalJourneyFailureDTO(failure *proposalworkflow.Failure) *ProposalJourneyFailureDTO {
	if failure == nil {
		return nil
	}
	return &ProposalJourneyFailureDTO{Code: string(failure.Code), Message: failure.Message}
}
