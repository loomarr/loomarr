package api

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/danielgtaylor/huma/v2"

	"github.com/loomarr/loomarr/internal/proposalworkflow"
	"github.com/loomarr/loomarr/internal/store"
	"github.com/loomarr/loomarr/internal/suggest"
)

// ProposalWorkflow is the sole authoritative read seam for the first-channel
// journey. HTTP handlers never join raw Job, Proposal, and Channel records.
type ProposalWorkflow interface {
	Inspect(context.Context, proposalworkflow.Viewer, string) (proposalworkflow.Journey, error)
	List(context.Context, proposalworkflow.Viewer, proposalworkflow.ListOptions) ([]proposalworkflow.Journey, error)
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
	ID         string           `json:"id"`
	Status     string           `json:"status" enum:"submitted,approved,denied"`
	ApprovedBy string           `json:"approvedBy,omitempty"`
	DenyReason string           `json:"denyReason,omitempty"`
	ModSummary string           `json:"modSummary,omitempty"`
	Note       string           `json:"note,omitempty"`
	Proposal   suggest.Proposal `json:"proposal"`
}

type ProposalJourneyChannelDTO struct {
	ID     string `json:"id"`
	Status string `json:"status" enum:"building,live,empty,drifted,detached,paused"`
}

func (s *Server) registerProposalJourneys(api huma.API) {
	huma.Register(api, withRole(huma.Operation{
		OperationID: "list-proposal-jobs", Method: http.MethodGet, Path: "/v1/proposal-jobs",
		Summary:     "List authoritative First-channel Journeys",
		Description: "Lists bounded Proposal Job journeys newest-first, including queued, running, and failed requests with no Proposal. Members are always caller-scoped; admins may inspect all or request mine=true.",
		Tags:        []string{"proposal-jobs"},
	}, RoleMember), s.listProposalJourneys)
	huma.Register(api, withRole(huma.Operation{
		OperationID: "get-proposal-job", Method: http.MethodGet, Path: "/v1/proposal-jobs/{jobId}",
		Summary:     "Get the authoritative First-channel Journey",
		Description: "Restores one versioned Proposal Job, bounded Attempt history, safe failure, Proposal, intent-bound Channel, milestone, and server-authorized actions. Members may read only their own Job; admins may read any.",
		Tags:        []string{"proposal-jobs"},
	}, RoleMember), s.getProposalJourney)
}

type proposalJourneyListInput struct {
	Mine   bool   `query:"mine" doc:"For admins, scope the list to the caller; members are always caller-scoped"`
	Status string `query:"status" enum:"generating,awaiting_approval,denied,building,live,failed" doc:"Optional Journey milestone filter"`
}

type proposalJourneyListOutput struct {
	Body struct {
		Journeys []ProposalJourneyDTO `json:"journeys"`
	}
}

func (s *Server) listProposalJourneys(ctx context.Context, in *proposalJourneyListInput) (*proposalJourneyListOutput, error) {
	if s.proposalWorkflow == nil {
		return nil, huma.Error501NotImplemented("Proposal workflow is not configured")
	}
	viewer := proposalworkflow.Viewer{Admin: roleFrom(ctx) == RoleAdmin}
	if user, ok := userFrom(ctx); ok {
		viewer.UserID = user.ID
	}
	journeys, err := s.proposalWorkflow.List(ctx, viewer, proposalworkflow.ListOptions{
		Mine: in.Mine, Milestone: proposalworkflow.Milestone(in.Status),
	})
	if errors.Is(err, proposalworkflow.ErrInvalidState) {
		return nil, apiErrWithCause(http.StatusInternalServerError, "Couldn't restore channel requests",
			"Loomarr found inconsistent saved workflow state and stopped rather than guessing. Check the server logs.", err)
	}
	if err != nil {
		return nil, apiErrWithCause(http.StatusInternalServerError, "Couldn't read channel requests",
			"Loomarr couldn't restore your channel requests. Try again in a moment.", err)
	}
	out := &proposalJourneyListOutput{}
	out.Body.Journeys = make([]ProposalJourneyDTO, 0, len(journeys))
	for _, journey := range journeys {
		out.Body.Journeys = append(out.Body.Journeys, proposalJourneyDTO(journey))
	}
	return out, nil
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
			ID: journey.Proposal.ID, Status: string(journey.Proposal.Status),
			ApprovedBy: journey.Proposal.ApprovedBy, DenyReason: journey.Proposal.DenyReason,
			ModSummary: journey.Proposal.ModSummary, Note: journey.Proposal.Note,
			Proposal: journey.Proposal.Proposal,
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
