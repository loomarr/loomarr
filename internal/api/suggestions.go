package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/danielgtaylor/huma/v2"

	"github.com/mantonx/loomarr/internal/provision"
	"github.com/mantonx/loomarr/internal/store"
	"github.com/mantonx/loomarr/internal/suggest"
)

// userIDFromHuma returns the authenticated user's id, or "" for a token
// (break-glass) caller with no user record. Used to stamp created_by/approved_by.
func userIDFromHuma(ctx context.Context) string {
	if u, ok := userFrom(ctx); ok {
		return u.ID
	}
	return ""
}

// registerSuggestions mounts /v1/suggestions* (§7/§8). Submit is open to any
// authenticated user (members request; §8 human-in-the-loop); list/get are
// visible to all; approve/deny require admin (the approval gate — §11). Approve
// is the ONLY path from a proposal to an acquisition, and it is admin-gated, so
// nothing unapproved ever reaches /v1/titles (§19).
func (s *Server) registerSuggestions(api huma.API) {
	huma.Register(api, huma.Operation{
		OperationID: "submit-suggestion", Method: http.MethodPost, Path: "/v1/suggestions",
		Summary: "Start a suggestion job from an intent", Description: "Any authenticated user (§8).",
		Tags: []string{"suggestions"},
	}, s.submitSuggestion)

	huma.Register(api, huma.Operation{
		OperationID: "list-proposals", Method: http.MethodGet, Path: "/v1/suggestions",
		Summary: "List proposals by status", Description: "status=submitted is the approval queue.",
		Tags: []string{"suggestions"},
	}, s.listProposals)

	huma.Register(api, huma.Operation{
		OperationID: "get-proposal", Method: http.MethodGet, Path: "/v1/suggestions/{id}",
		Summary: "Get a proposal + its job status", Tags: []string{"suggestions"},
	}, s.getProposal)

	huma.Register(api, huma.Operation{
		OperationID: "approve-proposal", Method: http.MethodPost, Path: "/v1/suggestions/{id}/approve",
		Summary: "Approve a proposal (admin)", Description: "Admin only. Enqueues acquisitions through the approval gate (§8).",
		Tags: []string{"suggestions"},
	}, s.approveProposal)

	huma.Register(api, huma.Operation{
		OperationID: "deny-proposal", Method: http.MethodPost, Path: "/v1/suggestions/{id}/deny",
		Summary: "Deny a proposal (admin)", Description: "Admin only.", Tags: []string{"suggestions"},
	}, s.denyProposal)
}

// --- submit ---

type submitInput struct {
	Body struct {
		Description string   `json:"description" example:"90s action movies"`
		Era         string   `json:"era,omitempty"`
		Tone        string   `json:"tone,omitempty"`
		MustInclude []string `json:"mustInclude,omitempty"`
		MustExclude []string `json:"mustExclude,omitempty"`
		MaxAcquire  int      `json:"maxAcquisitions,omitempty"`
	}
}
type submitOutput struct {
	Body struct {
		JobID string `json:"jobId"`
	}
}

func (s *Server) submitSuggestion(ctx context.Context, in *submitInput) (*submitOutput, error) {
	if s.suggest == nil || s.featureOff(ctx, "suggestions") {
		return nil, huma.Error501NotImplemented("suggester not configured")
	}
	if in.Body.Description == "" {
		return nil, huma.Error400BadRequest("description is required")
	}
	createdBy := userIDFromHuma(ctx) // the member who requested it (§8 My proposals)
	jobID, err := s.suggest.Submit(ctx, in.Body.Description, in.Body.Era, in.Body.Tone,
		in.Body.MustInclude, in.Body.MustExclude, in.Body.MaxAcquire, createdBy)
	if err != nil {
		return nil, err
	}
	out := &submitOutput{}
	out.Body.JobID = jobID
	return out, nil
}

// --- list / get ---

// ProposalDTO is the API view of a persisted proposal (§8). Proposal is the FULL typed
// suggest.Proposal (lineup, acquisitions, alternates, scores, rationale, policy) — typed
// straight from the domain struct so orval generates the FE's proposal types 1:1 (§12),
// with zero hand-written mirror on either side and no fidelity lost through the wire.
type ProposalDTO struct {
	ID         string           `json:"id"`
	JobID      string           `json:"jobId"`
	Status     string           `json:"status" enum:"submitted,approved,denied"`
	CreatedBy  string           `json:"createdBy,omitempty"`
	ApprovedBy string           `json:"approvedBy,omitempty"`
	DenyReason string           `json:"denyReason,omitempty"`
	Proposal   suggest.Proposal `json:"proposal"`
}

func proposalToDTO(p store.Proposal) ProposalDTO {
	var payload suggest.Proposal
	// A malformed stored proposal shouldn't 500 a list; leave the payload zero-valued.
	if len(p.ProposalJSON) > 0 {
		_ = json.Unmarshal([]byte(p.ProposalJSON), &payload)
	}
	return ProposalDTO{
		ID: p.ID, JobID: p.JobID, Status: p.Status, CreatedBy: p.CreatedBy,
		ApprovedBy: p.ApprovedBy, DenyReason: p.DenyReason, Proposal: payload,
	}
}

type listProposalsInput struct {
	Status string `query:"status" enum:"submitted,approved,denied" doc:"Filter by status; submitted = the approval queue"`
}
type listProposalsOutput struct {
	Body struct {
		Proposals []ProposalDTO `json:"proposals"`
	}
}

func (s *Server) listProposals(ctx context.Context, in *listProposalsInput) (*listProposalsOutput, error) {
	status := in.Status
	if status == "" {
		status = "submitted"
	}
	props, err := s.store.ListProposalsByStatus(ctx, status)
	if err != nil {
		return nil, err
	}
	out := &listProposalsOutput{}
	out.Body.Proposals = make([]ProposalDTO, 0, len(props))
	for _, p := range props {
		out.Body.Proposals = append(out.Body.Proposals, proposalToDTO(p))
	}
	return out, nil
}

type proposalIDInput struct {
	ID string `path:"id"`
}
type proposalOutput struct{ Body ProposalDTO }

func (s *Server) getProposal(ctx context.Context, in *proposalIDInput) (*proposalOutput, error) {
	p, err := s.store.GetProposal(ctx, in.ID)
	if errors.Is(err, store.ErrNotFound) {
		return nil, huma.Error404NotFound("no such proposal")
	}
	if err != nil {
		return nil, err
	}
	return &proposalOutput{Body: proposalToDTO(p)}, nil
}

// --- approve / deny (the approval gate — §8/§11) ---

// The approve path reads the same typed ProposalPayload the list/get endpoints return
// (defined above) — one mirror of the stored suggest.Proposal, no longer a second.

type approveOutput struct {
	Body struct {
		Status   string `json:"status"`
		Enqueued int    `json:"enqueued" doc:"Acquisitions enqueued as wanted titles"`
	}
}

// approveProposal is the approval gate (§8/§11/§19): ADMIN ONLY. It flips the
// proposal to approved and enqueues each acquisition as a `wanted` title — the
// only path from a proposal to /v1/titles. Members cannot reach this (403), so
// nothing unapproved ever acquires.
func (s *Server) approveProposal(ctx context.Context, in *proposalIDInput) (*approveOutput, error) {
	if err := requireAdmin(ctx); err != nil {
		return nil, err
	}
	p, err := s.store.GetProposal(ctx, in.ID)
	if errors.Is(err, store.ErrNotFound) {
		return nil, huma.Error404NotFound("no such proposal")
	}
	if err != nil {
		return nil, err
	}
	if p.Status != "submitted" {
		return nil, huma.Error409Conflict("proposal is not in the submitted state")
	}

	var body suggest.Proposal
	if err := json.Unmarshal([]byte(p.ProposalJSON), &body); err != nil {
		return nil, huma.Error500InternalServerError("stored proposal is malformed", err)
	}

	// In-library picks become `available` title Records so the scheduler can place
	// them (§8: "the approved lineup feeds the scheduler"). Without this, an
	// in-library pick is unresolvable and never becomes a program.
	for _, l := range body.Lineup {
		if !l.InLibrary || l.LibraryItemID == "" {
			continue // a not-in-library lineup item is covered by acquisitions
		}
		title := provision.Title{
			MediaType: provision.MediaType(l.MediaType),
			TMDBID:    l.TMDBID, TVDBID: l.TVDBID, Name: l.Name, Year: l.Year, Seasons: l.Seasons,
		}
		key, kerr := title.Key()
		if kerr != nil {
			continue // grounded proposals are always keyable; skip defensively
		}
		// Idempotent: don't clobber an existing record (it may already be tracked
		// through the provisioner). Only record freshly-observed in-library titles.
		if _, gerr := s.store.GetTitle(ctx, key); gerr == nil {
			continue
		} else if !errors.Is(gerr, store.ErrNotFound) {
			return nil, gerr
		}
		rec := provision.Record{
			Key: key, Title: title,
			State: provision.Available, LibraryID: l.LibraryItemID,
		}
		if err := s.store.UpsertTitle(ctx, rec); err != nil {
			return nil, err
		}
	}

	enqueued := 0
	for _, a := range body.Acquisitions {
		title := provision.Title{
			MediaType: provision.MediaType(a.MediaType),
			TMDBID:    a.TMDBID, TVDBID: a.TVDBID, Name: a.Name, Year: a.Year, Seasons: a.Seasons,
		}
		key, kerr := title.Key()
		if kerr != nil {
			// A grounded proposal never has an unkeyable acquisition; skip defensively.
			continue
		}
		// Idempotent enqueue (§4 inv. 3) — only create if absent.
		if _, gerr := s.store.GetTitle(ctx, key); gerr == nil {
			continue
		} else if !errors.Is(gerr, store.ErrNotFound) {
			return nil, gerr
		}
		// Deadline = now so the acquisition is DUE IMMEDIATELY: the provisioning
		// reconciler's ClaimDueTitles only claims rows with `deadline <= now AND
		// deadline > 0`, so a zero-deadline wanted title is never claimed and never
		// submitted to Seerr. (Caught by the live smoke: approved acquisitions sat in
		// `wanted` forever because the deadline was unset.)
		rec := provision.Record{Key: key, Title: title, State: provision.Wanted, Deadline: time.Now()}
		if err := s.store.UpsertTitle(ctx, rec); err != nil {
			return nil, err
		}
		enqueued++
	}

	p.Status = "approved"
	p.ApprovedBy = userIDFromHuma(ctx)
	if err := s.store.UpdateProposal(ctx, p); err != nil {
		return nil, err
	}
	out := &approveOutput{}
	out.Body.Status = "approved"
	out.Body.Enqueued = enqueued
	return out, nil
}

type denyInput struct {
	ID   string `path:"id"`
	Body struct {
		Reason string `json:"reason,omitempty"`
	}
}
type denyOutput struct {
	Body struct {
		Status string `json:"status"`
	}
}

func (s *Server) denyProposal(ctx context.Context, in *denyInput) (*denyOutput, error) {
	if err := requireAdmin(ctx); err != nil {
		return nil, err
	}
	p, err := s.store.GetProposal(ctx, in.ID)
	if errors.Is(err, store.ErrNotFound) {
		return nil, huma.Error404NotFound("no such proposal")
	}
	if err != nil {
		return nil, err
	}
	p.Status = "denied"
	p.ApprovedBy = userIDFromHuma(ctx)
	p.DenyReason = in.Body.Reason
	if err := s.store.UpdateProposal(ctx, p); err != nil {
		return nil, err
	}
	out := &denyOutput{}
	out.Body.Status = "denied"
	return out, nil
}
