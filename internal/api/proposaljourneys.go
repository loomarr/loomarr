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
	Revise(context.Context, proposalworkflow.Viewer, string, suggest.Intent) error
}

// ProposalProgress reads the live snapshot of a generation that is running on this process.
// ok is false for a job that is queued, finished or not running here.
type ProposalProgress interface {
	Progress(jobID string) (snapshot suggest.ProgressSnapshot, ok bool)
}

// ProposalJourneyProgressDTO is what a generating request has honestly done so far. Every
// field is a real event: a phase change, a catalog_search's own arguments, or a pick that
// resolved against the surfaced catalog (never one final validation would drop). It is in
// memory on the running process — a run does not survive a restart — and is absent once the
// Proposal or the failure exists. There is no percentage and no ETA by design: Target is the
// most picks the model is asked for, so "4 of about 8" never overstates.
type ProposalJourneyProgressDTO struct {
	Stage     string                        `json:"stage" enum:"reading,searching,choosing,building" doc:"User-facing step: reading the request, searching the library, choosing titles, building the lineup"`
	Terms     []string                      `json:"terms" doc:"What the catalog searches were actually asked for (bounded)"`
	Picks     []ProposalJourneyProgressPick `json:"picks" doc:"Titles chosen so far, in the order the model chose them, each resolved against the surfaced catalog"`
	Target    int                           `json:"target" doc:"The most titles the model is asked to pick"`
	StartedAt time.Time                     `json:"startedAt"`
}

type ProposalJourneyProgressPick struct {
	Key           string `json:"key"`
	MediaType     string `json:"mediaType" enum:"movie,series"`
	Name          string `json:"name"`
	Year          int    `json:"year,omitempty"`
	TMDBID        int    `json:"tmdbId,omitempty"`
	InLibrary     bool   `json:"inLibrary"`
	LibraryItemID string `json:"libraryItemId,omitempty"`
}

type ProposalJourneyDTO struct {
	Version   int                         `json:"version" doc:"Current First-channel Journey schema version"`
	JobID     string                      `json:"jobId"`
	Milestone string                      `json:"milestone" enum:"generating,awaiting_approval,denied,building,live,failed"`
	Intent    suggest.Intent              `json:"intent"`
	Attempts  []ProposalJobAttemptDTO     `json:"attempts"`
	Failure   *ProposalJourneyFailureDTO  `json:"failure,omitempty"`
	Progress  *ProposalJourneyProgressDTO `json:"progress,omitempty" doc:"Live generation progress; present only while generating and running on this server"`
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
	Code           string                `json:"code" enum:"no_grounded_titles,selection_empty,budget_exhausted,generation_failed"`
	Reason         string                `json:"reason" enum:"retrieval_unavailable,reference_unreadable,no_catalog_match,named_set_unproven,constraints_conflict,date_semantics_unclear,invalid_tool_calls,provider_timeout,provider_unavailable,provider_response_invalid,discovery_budget_exhausted,generation_failed"`
	RecoveryAction string                `json:"recoveryAction" enum:"edit_reference,broaden_request,provide_examples,resolve_constraints,clarify_dates,simplify_request,retry_later"`
	Message        string                `json:"message"`
	Guidance       string                `json:"guidance"`
	Trace          suggest.DecisionTrace `json:"trace,omitempty"`
}

type ProposalJourneyProposalDTO struct {
	ID             string           `json:"id"`
	Status         string           `json:"status" enum:"submitted,approved,denied,superseded"`
	ApprovedBy     string           `json:"approvedBy,omitempty"`
	ApprovedByName string           `json:"approvedByName,omitempty" doc:"Display name of the approver, or Automatic for an auto-approval; empty if unknown"`
	ApprovedAt     string           `json:"approvedAt,omitempty" doc:"When this was approved (RFC3339)"`
	DenyReason     string           `json:"denyReason,omitempty"`
	ModSummary     string           `json:"modSummary,omitempty"`
	Note           string           `json:"note,omitempty"`
	Proposal       suggest.Proposal `json:"proposal"`
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
	huma.Register(api, withRole(huma.Operation{
		OperationID: "revise-proposal-job", Method: http.MethodPost, Path: "/v1/proposal-jobs/{jobId}/revise",
		Summary:     "Revise the pending proposal for a First-channel Journey",
		Description: "Admin only. Keeps the current submitted proposal visible while its replacement runs; success supersedes it atomically and failure leaves it approvable.",
		Tags:        []string{"proposal-jobs"},
	}, RoleAdmin), s.reviseProposalJourney)
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
	names, err := s.journeyPersonNames(ctx)
	if err != nil {
		return nil, apiErrWithCause(http.StatusInternalServerError, "Couldn't read channel requests",
			"Loomarr couldn't restore your channel requests. Try again in a moment.", err)
	}
	out := &proposalJourneyListOutput{}
	out.Body.Journeys = make([]ProposalJourneyDTO, 0, len(journeys))
	for _, journey := range journeys {
		dto := proposalJourneyDTO(journey, names)
		dto.Progress = s.journeyProgress(journey) // the In progress card's stage line
		out.Body.Journeys = append(out.Body.Journeys, dto)
	}
	return out, nil
}

type proposalJourneyInput struct {
	JobID string `path:"jobId"`
}

type proposalJourneyOutput struct {
	Body ProposalJourneyDTO
}

type reviseProposalJourneyInput struct {
	JobID string `path:"jobId"`
	Body  suggest.Intent
}

type reviseProposalJourneyOutput struct {
	Body struct {
		JobID string `json:"jobId"`
	}
}

func (s *Server) reviseProposalJourney(
	ctx context.Context,
	in *reviseProposalJourneyInput,
) (*reviseProposalJourneyOutput, error) {
	if s.proposalWorkflow == nil {
		return nil, huma.Error501NotImplemented("Proposal workflow is not configured")
	}
	if err := s.suggestionConfigurationError(ctx); err != nil {
		return nil, err
	}
	if in.Body.Description == "" {
		return nil, errBadRequest("Description required", "Describe the channel you want in a sentence.")
	}
	viewer := proposalworkflow.Viewer{Admin: roleFrom(ctx) == RoleAdmin}
	if user, ok := userFrom(ctx); ok {
		viewer.UserID = user.ID
	}
	err := s.proposalWorkflow.Revise(ctx, viewer, in.JobID, in.Body)
	switch {
	case errors.Is(err, store.ErrNotFound):
		return nil, errNotFound("Channel request not found", "That channel request doesn't exist or has expired.")
	case errors.Is(err, proposalworkflow.ErrForbidden):
		return nil, apiErr(http.StatusForbidden, "Channel request unavailable",
			"You can only revise channel requests submitted by your account.")
	case errors.Is(err, proposalworkflow.ErrNotRevisable):
		return nil, errConflict("Suggestions changed", "Reload the channel request before editing it again.")
	case errors.Is(err, proposalworkflow.ErrInvalidState):
		return nil, apiErrWithCause(http.StatusInternalServerError, "Couldn't revise the channel request",
			"Loomarr found inconsistent saved workflow state and stopped rather than guessing. Check the server logs.", err)
	case err != nil:
		return nil, apiErrWithCause(http.StatusInternalServerError, "Couldn't revise the channel request",
			"Your current suggestions are unchanged. Try again in a moment.", err)
	}
	out := &reviseProposalJourneyOutput{}
	out.Body.JobID = in.JobID
	return out, nil
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
	names, err := s.journeyPersonNames(ctx)
	if err != nil {
		return nil, apiErrWithCause(http.StatusInternalServerError, "Couldn't read the channel request",
			"Loomarr couldn't restore this channel request. Try again in a moment.", err)
	}
	dto := proposalJourneyDTO(journey, names)
	dto.Progress = s.journeyProgress(journey)
	return &proposalJourneyOutput{Body: dto}, nil
}

// journeyPersonNames resolves approver ids to names for a Journey. A server wired without a store
// (the workflow fakes in tests) has no people to name, so every approver reads as unknown.
func (s *Server) journeyPersonNames(ctx context.Context) (personNames, error) {
	if s.store == nil {
		return personNames{}, nil
	}
	return s.personNames(ctx)
}

func proposalJourneyDTO(journey proposalworkflow.Journey, names personNames) ProposalJourneyDTO {
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
			ApprovedBy: journey.Proposal.ApprovedBy, ApprovedByName: names.name(journey.Proposal.ApprovedBy),
			ApprovedAt: rfc3339OrEmpty(journey.Proposal.ApprovedAt), DenyReason: journey.Proposal.DenyReason,
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
	return &ProposalJourneyFailureDTO{Code: string(failure.Code), Reason: string(failure.Reason), RecoveryAction: string(failure.RecoveryAction), Message: failure.Message, Guidance: failure.Guidance, Trace: proposalworkflow.PublicFailureTrace(failure.Trace)}
}

// journeyProgress attaches the live snapshot to a generating Journey only.
func (s *Server) journeyProgress(journey proposalworkflow.Journey) *ProposalJourneyProgressDTO {
	if s.proposalProgress == nil || journey.Milestone != proposalworkflow.MilestoneGenerating {
		return nil
	}
	snap, ok := s.proposalProgress.Progress(journey.JobID)
	if !ok {
		return nil
	}
	dto := &ProposalJourneyProgressDTO{
		Stage: string(snap.Stage), Terms: append([]string{}, snap.Terms...), Target: snap.Target,
		StartedAt: snap.StartedAt, Picks: make([]ProposalJourneyProgressPick, 0, len(snap.Picks)),
	}
	for _, p := range snap.Picks {
		dto.Picks = append(dto.Picks, ProposalJourneyProgressPick{
			Key: p.Key, MediaType: p.MediaType, Name: p.Name, Year: p.Year, TMDBID: p.TMDBID,
			InLibrary: p.InLibrary, LibraryItemID: p.LibraryItemID,
		})
	}
	return dto
}
