package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/danielgtaylor/huma/v2"

	"github.com/mantonx/loomarr/internal/store"
	"github.com/mantonx/loomarr/internal/suggest"
)

// ProposalJobDTO is the authoritative recovery view for one Intent execution (§7/§8).
// The job lifecycle remains distinct from the optional Proposal decision lifecycle.
type ProposalJobDTO struct {
	JobID     string           `json:"jobId"`
	Status    string           `json:"status" enum:"queued,running,done,failed"`
	Intent    suggest.Intent   `json:"intent"`
	Attempts  int              `json:"attempts"`
	CreatedAt string           `json:"createdAt" doc:"RFC3339"`
	UpdatedAt string           `json:"updatedAt" doc:"RFC3339"`
	Failure   *ProposalFailure `json:"failure,omitempty"`
	Proposal  *ProposalDTO     `json:"proposal,omitempty"`
}

// ProposalFailure is bounded, requester-safe failure copy. The store's private diagnostic
// never crosses this HTTP boundary.
type ProposalFailure struct {
	Code    string `json:"code" enum:"no_grounded_titles,timed_out,provider_unavailable,generation_failed"`
	Message string `json:"message"`
}

func (s *Server) registerProposalJobs(api huma.API) {
	huma.Register(api, withRole(huma.Operation{
		OperationID: "list-proposal-jobs", Method: http.MethodGet, Path: "/v1/proposal-jobs",
		Summary:     "List proposal-job history",
		Description: "Returns bounded authoritative execution history. Members are always scoped to their own jobs; mine=true scopes session admins and returns empty for API_TOKEN (§7/§8).",
		Tags:        []string{"proposal-jobs"},
	}, RoleMember), s.listProposalJobs)

	huma.Register(api, withRole(huma.Operation{
		OperationID: "get-proposal-job", Method: http.MethodGet, Path: "/v1/proposal-jobs/{jobId}",
		Summary:     "Get authoritative proposal-job state",
		Description: "Returns the complete Intent, execution state, safe failure, and newest Proposal. Members may read only their own jobs; admins may read any (§7/§8).",
		Tags:        []string{"proposal-jobs"},
	}, RoleMember), s.getProposalJob)
}

type proposalJobIDInput struct {
	JobID string `path:"jobId"`
}

type proposalJobOutput struct{ Body ProposalJobDTO }

type listProposalJobsInput struct {
	Mine   bool   `query:"mine" doc:"Scope to the authenticated person's own jobs. API_TOKEN has no person and returns an empty list."`
	Status string `query:"status" enum:"queued,running,done,failed" doc:"Optional execution lifecycle filter"`
	Limit  int    `query:"limit" minimum:"1" maximum:"100" default:"50" doc:"Maximum jobs to return (default 50, max 100)"`
}

type listProposalJobsOutput struct {
	Body struct {
		ProposalJobs []ProposalJobDTO `json:"proposalJobs"`
	}
}

func (s *Server) listProposalJobs(ctx context.Context, in *listProposalJobsInput) (*listProposalJobsOutput, error) {
	filter := store.ProposalJobFilter{Status: in.Status, Limit: in.Limit}
	user, hasUser := userFrom(ctx)

	// A member is caller-scoped regardless of the query flag: omitting `mine` must never widen a
	// caller-owned resource. Admins omit `mine` deliberately for household diagnosis.
	if roleFrom(ctx) != RoleAdmin {
		if !hasUser {
			return emptyProposalJobs(), nil
		}
		filter.CreatedBy = user.ID
	} else if in.Mine {
		// API_TOKEN is an admin credential, but not a person. CreatedBy="" means "no filter" at
		// the store seam, so short-circuit rather than turning `mine` into an all-users read.
		if !hasUser {
			return emptyProposalJobs(), nil
		}
		filter.CreatedBy = user.ID
	}

	jobs, err := s.store.ListProposalJobs(ctx, filter)
	if errors.Is(err, store.ErrInvalidProposalJobFilter) {
		return nil, errUnprocessable("Invalid proposal-job filter",
			"Use a valid execution status and a limit between 1 and 100.")
	}
	if err != nil {
		return nil, apiErrWithCause(http.StatusInternalServerError, "Couldn't read channel requests",
			"Loomarr couldn't read channel-request history. Try again in a moment.", err)
	}

	out := emptyProposalJobs()
	for _, job := range jobs {
		dto, err := proposalJobToDTO(job)
		if err != nil {
			return nil, apiErrWithCause(http.StatusInternalServerError, "Couldn't read channel requests",
				"Loomarr couldn't read channel-request history. Try again in a moment.", err)
		}
		out.Body.ProposalJobs = append(out.Body.ProposalJobs, dto)
	}
	return out, nil
}

func emptyProposalJobs() *listProposalJobsOutput {
	out := &listProposalJobsOutput{}
	out.Body.ProposalJobs = []ProposalJobDTO{}
	return out
}

func (s *Server) getProposalJob(ctx context.Context, in *proposalJobIDInput) (*proposalJobOutput, error) {
	job, err := s.store.GetProposalJob(ctx, in.JobID)
	if errors.Is(err, store.ErrNotFound) {
		return nil, errNotFound("Proposal job not found", "That channel request doesn't exist — it may have expired or been removed.")
	}
	if err != nil {
		return nil, apiErrWithCause(http.StatusInternalServerError, "Couldn't read the channel request",
			"Loomarr couldn't restore this channel request. Try again in a moment.", err)
	}
	if roleFrom(ctx) != RoleAdmin {
		user, ok := userFrom(ctx)
		if !ok || user.ID != job.Job.CreatedBy {
			return nil, apiErr(http.StatusForbidden, "Channel request unavailable",
				"You can only view channel requests submitted by your account.")
		}
	}
	dto, err := proposalJobToDTO(job)
	if err != nil {
		return nil, apiErrWithCause(http.StatusInternalServerError, "Couldn't read the channel request",
			"Loomarr couldn't restore this channel request. Try again in a moment.", err)
	}
	return &proposalJobOutput{Body: dto}, nil
}

func proposalJobToDTO(job store.ProposalJob) (ProposalJobDTO, error) {
	var intent suggest.Intent
	if err := json.Unmarshal([]byte(job.Job.IntentJSON), &intent); err != nil {
		return ProposalJobDTO{}, err
	}
	dto := ProposalJobDTO{
		JobID: job.Job.ID, Status: job.Job.Status, Intent: intent, Attempts: job.Job.Attempts,
		CreatedAt: job.Job.CreatedAt.UTC().Format(time.RFC3339),
		UpdatedAt: job.Job.UpdatedAt.UTC().Format(time.RFC3339),
	}
	if job.Proposal != nil {
		proposal := proposalToDTO(*job.Proposal)
		dto.Proposal = &proposal
	}
	if job.Job.Status == "failed" {
		dto.Failure = proposalFailureFor(job.Job.FailureCode)
	}
	return dto, nil
}

// proposalFailureFor maps only the bounded code. LastError is deliberately not an input, so a
// future edit cannot accidentally interpolate raw provider copy or credentials into the response.
func proposalFailureFor(code string) *ProposalFailure {
	switch code {
	case "no_grounded_titles":
		return &ProposalFailure{
			Code: code,
			Message: "No grounded titles matched this request. " +
				"Edit the channel description or constraints and try again.",
		}
	case "timed_out":
		return &ProposalFailure{Code: code, Message: "Channel generation took too long. Try again."}
	case "provider_unavailable":
		return &ProposalFailure{
			Code: code,
			Message: "The AI provider is unavailable right now. " +
				"Check the AI connection or try again later.",
		}
	default:
		// Empty and historical/unrecognised values fail closed to the generic bounded category.
		return &ProposalFailure{
			Code: "generation_failed", Message: "Loomarr couldn't generate this channel. Try again.",
		}
	}
}
