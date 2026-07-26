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

// submitInput takes the FULL typed suggest.Intent, for the same reason ProposalDTO
// carries suggest.Proposal (§12, 1:1): the previous hand-mirrored body had already
// drifted from the domain — it omitted RuntimeTgt, so `runtimeTargetMin` was
// unreachable even though the suggester honors it (prompt + scoring) and §13 lists a
// runtime target among the intent constraints a user may set. Typing from the domain
// removes the whole drift class rather than patching one missing field.
type submitInput struct {
	Body suggest.Intent
}
type submitOutput struct {
	Body struct {
		JobID string `json:"jobId"`
	}
}

func (s *Server) submitSuggestion(ctx context.Context, in *submitInput) (*submitOutput, error) {
	if s.suggest == nil || s.featureOff(ctx, "suggestions") {
		return nil, errNotImplemented("AI isn't set up", "Connect an AI provider in Settings → AI to build channels from a sentence.")
	}
	if in.Body.Description == "" {
		return nil, errBadRequest("Description required", "Describe the channel you want in a sentence.")
	}
	createdBy := userIDFromHuma(ctx) // the member who requested it (§8 My proposals)
	jobID, err := s.suggest.Submit(ctx, in.Body, createdBy)
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
		return nil, errNotFound("Suggestion not found", "That suggestion doesn't exist — it may have expired or been removed.")
	}
	if err != nil {
		return nil, err
	}
	return &proposalOutput{Body: proposalToDTO(p)}, nil
}

// --- approve / deny (the approval gate — §8/§11) ---

// The approve path reads the same typed ProposalPayload the list/get endpoints return
// (defined above) — one mirror of the stored suggest.Proposal, no longer a second.

// ApprovalEditDTO is the approver's modification, sent with POST …/approve (§7, D-K).
//
// Every field is OPTIONAL: an empty body approves the proposal exactly as generated, which is
// what the endpoint did before edit-before-approve existed. Existing callers keep working.
type ApprovalEditDTO struct {
	// Drop lists provisioning keys ("movie:tmdb:603") the approver removed. Keys rather than
	// indexes: an index means "the third one in the list I was looking at", which is wrong the
	// moment anything reorders between render and submit.
	Drop []string `json:"drop,omitempty" doc:"Provisioning keys to remove from the proposal before approving"`
	// Add lists titles the approver added via search. They become acquisitions and go through
	// the same idempotent enqueue as anything the model proposed.
	//
	// The DOMAIN type, not a hand-written mirror — the same call this file already makes for
	// ProposalDTO.Proposal, and for the same reason recorded there: the previous mirror had
	// already drifted from what it mirrored.
	Add []suggest.ProposalItem `json:"add,omitempty" doc:"Titles to add as acquisitions"`
	// Note is the message to whoever requested this — why it came back altered.
	Note string `json:"note,omitempty" doc:"A message to the requester, stored with the approval"`
}

type approveInput struct {
	ID string `path:"id"`
	// A POINTER, so the body is genuinely optional. With a value type huma requires one, and
	// `POST …/approve` with no body starts 400ing — which broke every existing caller
	// (the approve button, the e2e smoke, six handler tests) the moment this field was added.
	// Edit-before-approve is additive: approving unmodified must keep working exactly as it did.
	Body *ApprovalEditDTO
}

// approvalEditFromDTO maps the wire shape to the domain edit, returning nil when nothing was
// modified.
//
// NIL RATHER THAN AN EMPTY STRUCT is deliberate: an unmodified approval must be
// indistinguishable from the pre-edit behaviour all the way down — same code path, untouched
// ProposalJSON bytes, empty ModSummary. An empty-but-non-nil edit would still mark the proposal
// as "approved with modifications: none", which is a different and false claim.
func approvalEditFromDTO(b *ApprovalEditDTO) *suggest.ApprovalEdit {
	if b == nil || (len(b.Drop) == 0 && len(b.Add) == 0 && b.Note == "") {
		return nil
	}
	drop := make([]provision.Key, 0, len(b.Drop))
	for _, k := range b.Drop {
		drop = append(drop, provision.Key(k))
	}
	return &suggest.ApprovalEdit{DropKeys: drop, Add: b.Add, Note: b.Note}
}

type approveOutput struct {
	Body struct {
		Status    string `json:"status"`
		Enqueued  int    `json:"enqueued" doc:"Acquisitions enqueued as wanted titles"`
		ChannelID string `json:"channelId,omitempty" doc:"The channel this approval created or patched (§7). Empty only if channel creation failed — the approval itself still stands."`
	}
}

// approveProposal is the approval gate (§8/§11/§19): ADMIN ONLY. It flips the
// proposal to approved and enqueues each acquisition as a `wanted` title — the
// only path from a proposal to /v1/titles. Members cannot reach this (403), so
// nothing unapproved ever acquires.
func (s *Server) approveProposal(ctx context.Context, in *approveInput) (*approveOutput, error) {
	if err := requireAdmin(ctx); err != nil {
		return nil, err
	}
	p, err := s.store.GetProposal(ctx, in.ID)
	if errors.Is(err, store.ErrNotFound) {
		return nil, errNotFound("Suggestion not found", "That suggestion doesn't exist — it may have expired or been removed.")
	}
	if err != nil {
		return nil, err
	}
	if p.Status != "submitted" {
		return nil, errConflict("Already handled", "This suggestion has already been approved or dismissed.")
	}

	// The gate has ONE implementation, shared with the auto-approve path (§8, §11), so
	// the two can never disagree about what approving means.
	//
	// ⚠ The edit is PASSED to Approve rather than applied here first. Approving decides what
	// gets acquired; applying the edit in the handler would move that decision outside the gate
	// and leave auto-approve running different logic (§7, D-K).
	enqueued, err := suggest.Approve(ctx, s.store, p, approvalEditFromDTO(in.Body), userIDFromHuma(ctx), time.Now)
	if errors.Is(err, suggest.ErrNotSubmitted) {
		return nil, errConflict("Already handled", "This suggestion has already been approved or dismissed.")
	}
	if err != nil {
		return nil, err
	}
	out := &approveOutput{}
	out.Body.Status = "approved"
	out.Body.Enqueued = enqueued

	// …and materialize the channel. §7: approve → "enqueue acquisitions + create/patch
	// channel". Only the first half was ever implemented, which made Loomarr's whole
	// purpose unreachable from the UI: describe → review → approve, and then no channel
	// ever appeared. There is no create-a-channel screen because this is meant to BE the
	// path (§13), so nothing else could close the gap.
	//
	// Deliberately AFTER the gate and non-fatal: the approval and its acquisitions are
	// durable regardless. If channel creation fails, the operator has an approved
	// proposal and a logged error, not a half-applied approval that must be retried
	// from scratch. A nil binder (unwired in a narrow unit test) is treated the same
	// way — approval still stands, there's just no channel to report.
	if s.binder == nil {
		return out, nil
	}
	chID, err := s.binder.BindApprovedChannel(ctx, p)
	if err != nil {
		s.log.Error("approved, but the channel could not be created", "proposal", p.ID, "err", err)
		return out, nil
	}
	out.Body.ChannelID = chID
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
		return nil, errNotFound("Suggestion not found", "That suggestion doesn't exist — it may have expired or been removed.")
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
