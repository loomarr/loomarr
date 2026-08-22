package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/danielgtaylor/huma/v2"

	"github.com/mantonx/loomarr/internal/store"
	"github.com/mantonx/loomarr/internal/suggest"
)

// ProposalJobDTO is the authoritative, requester-safe view of one suggestion run.
// LastError deliberately stays private: provider diagnostics are useful in logs, but are
// neither stable UI copy nor safe to return to every requester.
type ProposalJobDTO struct {
	JobID   string           `json:"jobId"`
	Status  string           `json:"status" enum:"queued,running,done,failed"`
	Intent  suggest.Intent   `json:"intent"`
	Failure *ProposalFailure `json:"failure,omitempty"`
}

type ProposalFailure struct {
	Code    string `json:"code" enum:"no_grounded_titles,generation_failed"`
	Message string `json:"message"`
}

func (s *Server) registerProposalJobs(api huma.API) {
	huma.Register(api, withRole(huma.Operation{
		OperationID: "get-proposal-job", Method: http.MethodGet, Path: "/v1/proposal-jobs/{jobId}",
		Summary:     "Get authoritative proposal-job state",
		Description: "Returns the preserved Intent, execution lifecycle, and bounded failure. Members may read only jobs they submitted; admins may read any (§7/§8).",
		Tags:        []string{"proposal-jobs"},
	}, RoleMember), s.getProposalJob)
}

type proposalJobInput struct {
	JobID string `path:"jobId"`
}

type proposalJobOutput struct {
	Body ProposalJobDTO
}

func (s *Server) getProposalJob(ctx context.Context, in *proposalJobInput) (*proposalJobOutput, error) {
	job, err := s.store.GetJob(ctx, in.JobID)
	if errors.Is(err, store.ErrNotFound) {
		return nil, errNotFound("Channel request not found", "That channel request doesn't exist or has expired.")
	}
	if err != nil {
		return nil, apiErrWithCause(http.StatusInternalServerError, "Couldn't read the channel request",
			"Loomarr couldn't restore this channel request. Try again in a moment.", err)
	}
	if roleFrom(ctx) != RoleAdmin {
		user, ok := userFrom(ctx)
		if !ok || user.ID != job.CreatedBy {
			return nil, apiErr(http.StatusForbidden, "Channel request unavailable",
				"You can only view channel requests submitted by your account.")
		}
	}

	var intent suggest.Intent
	if err := json.Unmarshal([]byte(job.IntentJSON), &intent); err != nil {
		return nil, apiErrWithCause(http.StatusInternalServerError, "Couldn't read the channel request",
			"Loomarr couldn't restore this channel request. Try again in a moment.", err)
	}
	out := &proposalJobOutput{Body: ProposalJobDTO{JobID: job.ID, Status: job.Status, Intent: intent}}
	if job.Status == "failed" {
		out.Body.Failure = proposalFailureFor(job.FailureCode)
	}
	return out, nil
}

func proposalFailureFor(code string) *ProposalFailure {
	if code == suggest.FailureCodeNoGroundedTitles {
		return &ProposalFailure{
			Code:    code,
			Message: "No grounded titles matched this request. Try the same request again, or edit its description and constraints.",
		}
	}
	return &ProposalFailure{
		Code:    suggest.FailureCodeGenerationFailed,
		Message: "Loomarr couldn't generate this channel. Try again, or check the server logs for details.",
	}
}
