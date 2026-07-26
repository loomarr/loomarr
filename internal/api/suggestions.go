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
		OperationID: "bulk-approve-proposals", Method: http.MethodPost, Path: "/v1/suggestions/approve",
		Summary:     "Approve several proposals (admin)",
		Description: "Admin only. Approves each id through the SAME single-approve gate (§8) — no batch path. Returns a per-id result so one already-handled proposal does not hide the rest.",
		Tags:        []string{"suggestions"},
	}, s.bulkApproveProposals)

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
	ID         string `json:"id"`
	JobID      string `json:"jobId"`
	Status     string `json:"status" enum:"submitted,approved,denied"`
	CreatedBy  string `json:"createdBy,omitempty"`
	ApprovedBy string `json:"approvedBy,omitempty"`
	DenyReason string `json:"denyReason,omitempty"`
	// ModSummary and Note are the approval provenance (§7, D-K). Both have been PERSISTED
	// since V25 and neither left the server: the note an approver wrote to explain an edited
	// request reached the database and nothing could display it — the same "stored but never
	// rendered" shape the audit found `denyReason` in before V23 wired it up.
	//
	// ModSummary is generated server-side ("dropped 2, added 1"); Note is the approver's own
	// words. The distinction is deliberate and worth preserving in any UI: a summary the
	// approver types is a claim, one the code writes is a record.
	ModSummary string `json:"modSummary,omitempty" doc:"What the approver changed before approving, generated server-side"`
	Note       string `json:"note,omitempty" doc:"The approver's message to whoever requested this"`
	// ApprovedAt is when the gate let this through — the audit rows' ordering key (§7, V27).
	// Omitted while unapproved. RFC3339 so a client can render it in the viewer's timezone;
	// the store keeps epoch seconds.
	ApprovedAt string           `json:"approvedAt,omitempty" doc:"When this was approved (RFC3339)"`
	Proposal   suggest.Proposal `json:"proposal"`
}

// rfc3339OrEmpty renders a timestamp for the wire, mapping the ZERO time to "" so `omitempty`
// drops the field entirely. A proposal that was never approved must carry no approval time —
// emitting "0001-01-01T00:00:00Z" would put a date on a decision that never happened, and every
// client would then need to know which sentinel means "never".
func rfc3339OrEmpty(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339)
}

func proposalToDTO(p store.Proposal) ProposalDTO {
	var payload suggest.Proposal
	// A malformed stored proposal shouldn't 500 a list; leave the payload zero-valued.
	if len(p.ProposalJSON) > 0 {
		_ = json.Unmarshal([]byte(p.ProposalJSON), &payload)
	}
	return ProposalDTO{
		ID: p.ID, JobID: p.JobID, Status: p.Status, CreatedBy: p.CreatedBy,
		ApprovedBy: p.ApprovedBy, DenyReason: p.DenyReason,
		ModSummary: p.ModSummary, Note: p.Note, ApprovedAt: rfc3339OrEmpty(p.ApprovedAt),
		Proposal: payload,
	}
}

type listProposalsInput struct {
	Status string `query:"status" enum:"submitted,approved,denied" doc:"Filter by status; submitted = the approval queue"`
	// Mine scopes the list to the CALLER's own proposals — the "My requests" surface (§12).
	//
	// A bool resolved from the session, deliberately NOT a `user=<id>` parameter. The design
	// sketch read `ListProposals(status[, user])`, but a client-supplied id is a client-supplied
	// identity: any member could read another's requests by editing a URL. The only trustworthy
	// answer to "whose?" is the session's.
	Mine bool `query:"mine" doc:"Scope to the caller's own proposals (the My requests surface). Resolved from the session, not from a user id."`
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

	var props []store.Proposal
	var err error
	if in.Mine {
		// Scope from the SESSION, never from a parameter. The important property is that this
		// cannot widen: `ListProposalsByCreator` is `WHERE created_by = ?`, so the worst case is
		// too FEW rows, never another member's.
		//
		// A break-glass API_TOKEN caller has no user record, so its id is "" — and submit
		// stamps `created_by` with the same "" for a token-submitted proposal. So `mine=true`
		// on a token returns exactly the proposals that token submitted (usually none), which
		// is the honest reading of "mine" for a caller that is not a person. What it can never
		// do is fall through to everyone's, which is the failure mode worth designing against.
		props, err = s.store.ListProposalsByCreator(ctx, userIDFromHuma(ctx))
		if err != nil {
			return nil, err
		}
		// ListProposalsByCreator spans every status (it answers "what have I asked for?"), so
		// the status filter is applied here rather than pushed into a second store method.
		// `status=""` already defaulted to "submitted" above, so an unfiltered "all of mine"
		// is deliberately NOT expressible — the surface asks for one tab at a time.
		filtered := props[:0]
		for _, p := range props {
			if p.Status == status {
				filtered = append(filtered, p)
			}
		}
		props = filtered
	} else {
		props, err = s.store.ListProposalsByStatus(ctx, status)
		if err != nil {
			return nil, err
		}
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

// --- bulk approve (V27) ---

type bulkApproveInput struct {
	Body struct {
		// IDs are approved one at a time, each through the SAME `approveProposal` handler. No
		// batch store write and no second gate: the phase gate for this is literally "bulk
		// approve goes through the same chokepoint", and a batch path would be the second
		// acquisition path §7 forbids.
		//
		// No edit field, deliberately. Edit-before-approve (V25b) is a per-proposal judgement —
		// "drop these two titles" means nothing applied across a batch — so bulk is the
		// unmodified path only. An admin who wants to change a proposal opens it.
		IDs []string `json:"ids" minItems:"1" doc:"Proposal ids to approve, each through the single approval gate"`
	}
}

type bulkApproveResult struct {
	ID        string `json:"id"`
	OK        bool   `json:"ok"`
	Enqueued  int    `json:"enqueued,omitempty" doc:"Acquisitions enqueued as wanted titles"`
	ChannelID string `json:"channelId,omitempty"`
	Error     string `json:"error,omitempty" doc:"Why this one did not approve (already handled, not found, …)"`
}

type bulkApproveOutput struct {
	Body struct {
		// PER-ID results rather than a single status. One id that was already approved must not
		// abort the rest — but the caller still has to learn which ones did not go through, or
		// "approve 5" silently becoming "approved 4" is invisible.
		Results  []bulkApproveResult `json:"results"`
		Approved int                 `json:"approved" doc:"How many were approved"`
	}
}

// bulkApproveProposals approves several proposals in one request (V27). ADMIN ONLY, like the
// single approve it delegates to.
//
// ⚠ It calls `s.approveProposal` per id — the same handler, not a copy of its body. Everything
// that makes a single approval correct (the submitted-status check, the one `suggest.Approve`
// call, the channel bind, the audit stamps) therefore applies unchanged, and a future change to
// approval semantics cannot land in one path and miss the other.
//
// Sequential, not concurrent: approvals write titles and channels, and household-scale batches
// are small. Concurrency here would buy milliseconds and risk interleaving writes.
func (s *Server) bulkApproveProposals(ctx context.Context, in *bulkApproveInput) (*bulkApproveOutput, error) {
	if err := requireAdmin(ctx); err != nil {
		return nil, err
	}
	out := &bulkApproveOutput{}
	out.Body.Results = make([]bulkApproveResult, 0, len(in.Body.IDs))
	for _, id := range in.Body.IDs {
		res, err := s.approveProposal(ctx, &approveInput{ID: id})
		if err != nil {
			// A per-id failure is DATA, not a request failure: the ones that worked are already
			// durable, so 500ing the whole call would hide successful approvals behind an error.
			out.Body.Results = append(out.Body.Results, bulkApproveResult{
				ID: id, OK: false, Error: humaErrMessage(err),
			})
			continue
		}
		out.Body.Results = append(out.Body.Results, bulkApproveResult{
			ID: id, OK: true, Enqueued: res.Body.Enqueued, ChannelID: res.Body.ChannelID,
		})
		out.Body.Approved++
	}
	return out, nil
}

// humaErrMessage renders a handler error for a per-id result line, preferring the human-facing
// title a huma StatusError carries ("Already handled") over the raw error string.
func humaErrMessage(err error) string {
	var se huma.StatusError
	if errors.As(err, &se) {
		return se.Error()
	}
	return err.Error()
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
