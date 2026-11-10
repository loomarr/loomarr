package proposals

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/danielgtaylor/huma/v2"

	"github.com/loomarr/loomarr/internal/api/httpcore"
	"github.com/loomarr/loomarr/internal/channels/schedule"
	"github.com/loomarr/loomarr/internal/platform/holidayvocab"
	"github.com/loomarr/loomarr/internal/platform/rating"
	"github.com/loomarr/loomarr/internal/platform/store"
	"github.com/loomarr/loomarr/internal/platform/store/activitystore"
	"github.com/loomarr/loomarr/internal/platform/store/proposalstore"
	"github.com/loomarr/loomarr/internal/proposals"
	"github.com/loomarr/loomarr/internal/proposals/accept"
	"github.com/loomarr/loomarr/internal/proposals/outlook"
	"github.com/loomarr/loomarr/internal/proposals/suggest"
	"github.com/loomarr/loomarr/internal/provision"
)

// registerProposals mounts /v1/proposals* (§7/§8). Submit is open to any
// authenticated user (members request; §8 human-in-the-loop); list/get are
// visible to all; approve/deny require admin (the approval gate — §11). Approve
// is the ONLY path from a proposal to an acquisition, and it is admin-gated, so
// nothing unapproved ever reaches /v1/titles (§19).
func (s *Handlers) registerProposals(api huma.API) {
	huma.Register(api, httpcore.WithRole(huma.Operation{
		OperationID: "submit-proposal", Method: http.MethodPost, Path: "/v1/proposals",
		Summary: "Start a proposal job from an intent", Description: "Any authenticated user (§8).",
		Tags: []string{"proposals"},
	}, httpcore.RoleMember), s.submitProposal)

	huma.Register(api, httpcore.WithRole(huma.Operation{
		OperationID: "list-proposals", Method: http.MethodGet, Path: "/v1/proposals",
		Summary: "List proposals by status", Description: "status=submitted is the approval queue.",
		Tags: []string{"proposals"},
	}, httpcore.RoleMember), s.listProposals)

	huma.Register(api, httpcore.WithRole(huma.Operation{
		OperationID: "get-proposal", Method: http.MethodGet, Path: "/v1/proposals/{id}",
		Summary: "Get a proposal + its job status", Tags: []string{"proposals"},
	}, httpcore.RoleMember), s.getProposal)

	huma.Register(api, httpcore.WithRole(huma.Operation{
		OperationID: "get-proposal-outlook", Method: http.MethodPost, Path: "/v1/proposals/{id}/outlook",
		Summary:     "Observe launch readiness and fresh programming for the exact pending proposal and edits",
		Description: "Read-only; never approves, acquires or prepares media.", Tags: []string{"proposals"},
	}, httpcore.RoleMember), s.getProposalOutlook)

	huma.Register(api, httpcore.WithRole(huma.Operation{
		OperationID: "approve-proposal", Method: http.MethodPost, Path: "/v1/proposals/{id}/approve",
		Summary: "Approve a proposal (admin)", Description: "Admin only. Enqueues acquisitions through the approval gate (§8).",
		Tags: []string{"proposals"},
	}, httpcore.RoleAdmin), s.approveProposal)

	huma.Register(api, httpcore.WithRole(huma.Operation{
		OperationID: "bulk-approve-proposals", Method: http.MethodPost, Path: "/v1/proposals/approve",
		Summary:     "Approve several proposals (admin)",
		Description: "Admin only. Approves each id through the SAME single-approve gate (§8) — no batch path. Returns a per-id result so one already-handled proposal does not hide the rest.",
		Tags:        []string{"proposals"},
	}, httpcore.RoleAdmin), s.bulkApproveProposals)

	huma.Register(api, httpcore.WithRole(huma.Operation{
		OperationID: "deny-proposal", Method: http.MethodPost, Path: "/v1/proposals/{id}/deny",
		Summary: "Deny a proposal (admin)", Description: "Admin only.", Tags: []string{"proposals"},
	}, httpcore.RoleAdmin), s.denyProposal)
}

// --- submit ---

// submitInput takes the FULL typed proposals.Intent, for the same reason ProposalDTO
// carries proposals.Proposal (§12, 1:1): a hand-mirrored body drifts from the domain —
// the first one omitted RuntimeTgt, so `runtimeTargetMin` was
// unreachable even though the suggester honors it (prompt + scoring) and §13 lists a
// runtime target among the intent constraints a user may set. Typing from the domain
// removes the whole drift class rather than patching one missing field.
type submitInput struct {
	Body proposals.Intent
}
type submitOutput struct {
	Body struct {
		JobID string `json:"jobId"`
	}
}

// checkDescription refuses a channel description shorter than proposals.MinDescriptionLen once
// trimmed. The spec's minLength counts raw characters, so three spaces pass it; this is the
// check that makes the rule the same one the web form applies.
func checkDescription(description string) error {
	if utf8.RuneCountInString(strings.TrimSpace(description)) < proposals.MinDescriptionLen {
		return httpcore.BadRequest("Description too short",
			fmt.Sprintf("Describe the channel you want in at least %d characters (a sentence is plenty).", proposals.MinDescriptionLen))
	}
	return nil
}

func (s *Handlers) submitProposal(ctx context.Context, in *submitInput) (*submitOutput, error) {
	if s.suggest == nil {
		return nil, httpcore.FeatureNotConfigured("AI isn't set up", "Connect an AI provider and select a tool-capable lineup model in Settings → AI to build channels from a sentence.")
	}
	// A demo submitter builds its proposal from the library with no AI, so the AI and TMDB settings
	// it would otherwise need are not required (#2729).
	if canned, ok := s.suggest.(interface{ Canned() bool }); !ok || !canned.Canned() {
		if err := httpcore.SuggestionConfigurationError(ctx, s.settings); err != nil {
			return nil, err
		}
	}
	if err := checkDescription(in.Body.Description); err != nil {
		return nil, err
	}
	createdBy := httpcore.UserIDFrom(ctx) // the member who requested it (§8 My proposals)
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
// proposals.Proposal (lineup, acquisitions, alternates, scores, rationale, policy) — typed
// straight from the domain struct so orval generates the FE's proposal types 1:1 (§12),
// with zero hand-written mirror on either side and no fidelity lost through the wire.
type ProposalDTO struct {
	ID         string `json:"id"`
	JobID      string `json:"jobId"`
	Status     string `json:"status" enum:"submitted,approved,denied,superseded"`
	CreatedBy  string `json:"createdBy,omitempty"`
	ApprovedBy string `json:"approvedBy,omitempty"`
	// CreatedByName / ApprovedByName are the people behind those ids, resolved server-side so a
	// client never shows a 32-hex id. Empty when the user no longer exists (or the caller was
	// the break-glass token); "auto" resolves to "Automatic".
	CreatedByName  string `json:"createdByName,omitempty" doc:"Display name of the requester; empty if unknown"`
	ApprovedByName string `json:"approvedByName,omitempty" doc:"Display name of the approver, or Automatic for an auto-approval; empty if unknown"`
	DenyReason     string `json:"denyReason,omitempty"`
	// ModSummary and Note are the approval provenance (§7, D-K). Both are PERSISTED (V25) and
	// both must be served: a note an approver wrote to explain an edited request that reached
	// the database but not the wire would be the "stored but never rendered" shape the audit
	// found `denyReason` in before V23 wired it up.
	//
	// ModSummary is generated server-side ("dropped 2, added 1"); Note is the approver's own
	// words. The distinction is deliberate and worth preserving in any UI: a summary the
	// approver types is a claim, one the code writes is a record.
	ModSummary string `json:"modSummary,omitempty" doc:"What the approver changed before approving, generated server-side"`
	Note       string `json:"note,omitempty" doc:"The approver's message to whoever requested this"`
	// ApprovedAt is when the gate let this through — the audit rows' ordering key (§7, V27).
	// Omitted while unapproved. RFC3339 so a client can render it in the viewer's timezone;
	// the store keeps epoch seconds.
	ApprovedAt string             `json:"approvedAt,omitempty" doc:"When this was approved (RFC3339)"`
	Proposal   proposals.Proposal `json:"proposal"`
	// EpisodeSelectionPreview is a read-only projection from Proposal.Intent. It lets the
	// approval UI explain a search-added series before approval; the client cannot submit it,
	// and the approval gate independently re-derives every series item from the same Intent.
	EpisodeSelectionPreview schedule.EpisodeSelection `json:"episodeSelectionPreview,omitempty" doc:"Server-derived episode policy applied to any series added before approval"`
	// RaiseAudienceTo names the card's "Raise to <rating>" (#3012): the lowest audience limit that
	// lets every rated left-out title in, sent back as the approval edit's `audience`. Derived
	// here so no client keeps its own copy of the rating ladder.
	RaiseAudienceTo httpcore.RatingWire `json:"raiseAudienceTo,omitempty" doc:"The lowest audience limit that admits every rated left-out title; absent when none was left out for its rating"`
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

func proposalToDTO(p proposalstore.Proposal, holidays holidayvocab.Vocabulary) ProposalDTO {
	payload := p.Body
	raise, _ := accept.AudienceToAdmitRefused(payload)
	return ProposalDTO{
		ID: p.ID, JobID: p.JobID, Status: p.Status, CreatedBy: p.CreatedBy,
		ApprovedBy: p.ApprovedBy, DenyReason: p.DenyReason,
		ModSummary: p.ModSummary, Note: p.Note, ApprovedAt: rfc3339OrEmpty(p.ApprovedAt),
		Proposal: payload, EpisodeSelectionPreview: suggest.EpisodeSelectionForIntent(payload.Intent, holidays),
		RaiseAudienceTo: httpcore.RatingWire(raise),
	}
}

// personNames resolves user ids to display names for a proposal payload, and labels the
// auto-approval grant "Automatic".
type personNames struct{ httpcore.PersonNames }

func (s *Handlers) personNames(ctx context.Context) (personNames, error) {
	names, err := httpcore.LoadPersonNames(ctx, s.store)
	if err != nil {
		return personNames{}, err
	}
	return personNames{names}, nil
}

// name maps a stored created_by/decided_by to something a person can read. An unknown id
// yields "" rather than the id itself: the raw id is exactly what this exists to keep off screen.
func (n personNames) name(id string) string {
	if id == proposalstore.AutoApprovedBy {
		return "Automatic"
	}
	return n.Name(id)
}

func (n personNames) apply(dto ProposalDTO) ProposalDTO {
	dto.CreatedByName = n.name(dto.CreatedBy)
	dto.ApprovedByName = n.name(dto.ApprovedBy)
	return dto
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

func (s *Handlers) listProposals(ctx context.Context, in *listProposalsInput) (*listProposalsOutput, error) {
	status := in.Status
	if status == "" {
		status = "submitted"
	}

	var props []proposalstore.Proposal
	var err error
	if in.Mine {
		// Scope from the SESSION, never from a parameter. The important property is that this
		// cannot widen: `ListProposalsByCreator` matches `created_by` to one id, so the worst case
		// is too FEW rows, never another member's.
		//
		// A break-glass API_TOKEN caller has no user record, so its id is "" — and submit
		// stamps `created_by` NULL for a token-submitted proposal, which "" matches. So `mine=true`
		// on a token returns exactly the proposals that token submitted (usually none), which
		// is the honest reading of "mine" for a caller that is not a person. What it can never
		// do is fall through to everyone's, which is the failure mode worth designing against.
		props, err = s.store.ListProposalsByCreator(ctx, httpcore.UserIDFrom(ctx))
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
		// Everyone's proposals, readable by any authenticated user: read visibility is global in
		// this household-scale app (design §342), and the maintainer re-confirmed it on 2026-09-25
		// (#1429). Do not add an admin gate here without changing §342 first; members' UI hides
		// admin-only ACTIONS, which the server still enforces on the mutating routes.
		props, err = s.store.ListProposalsByStatus(ctx, status)
		if err != nil {
			return nil, err
		}
	}

	names, err := s.personNames(ctx)
	if err != nil {
		return nil, err
	}
	out := &listProposalsOutput{}
	out.Body.Proposals = make([]ProposalDTO, 0, len(props))
	holidays := s.holidays()
	for _, p := range props {
		out.Body.Proposals = append(out.Body.Proposals, names.apply(proposalToDTO(p, holidays)))
	}
	return out, nil
}

type proposalIDInput struct {
	ID string `path:"id"`
}
type proposalOutput struct{ Body ProposalDTO }

func (s *Handlers) getProposal(ctx context.Context, in *proposalIDInput) (*proposalOutput, error) {
	p, err := s.store.GetProposal(ctx, in.ID)
	if errors.Is(err, store.ErrNotFound) {
		return nil, httpcore.NotFound("Suggestion not found", "That suggestion doesn't exist — it may have expired or been removed.")
	}
	if err != nil {
		return nil, err
	}
	names, err := s.personNames(ctx)
	if err != nil {
		return nil, err
	}
	return &proposalOutput{Body: names.apply(proposalToDTO(p, s.holidays()))}, nil
}

// --- approve / deny (the approval gate — §8/§11) ---

// The approve path reads the same typed ProposalPayload the list/get endpoints return
// (defined above) — one mirror of the stored proposals.Proposal, not a second.

// ApprovalEditDTO is the approver's modification, sent with POST …/approve (§7, D-K).
//
// Every field is OPTIONAL: an empty body approves the proposal exactly as generated.
type ApprovalEditDTO struct {
	// Drop lists provisioning keys ("movie:tmdb:603") the approver removed. Keys rather than
	// indexes: an index means "the third one in the list I was looking at", which is wrong the
	// moment anything reorders between render and submit.
	Drop []string `json:"drop,omitempty" doc:"Provisioning keys to remove from the proposal before approving"`
	// Add lists titles the approver added via search or a grounded collection. The approval
	// gate rechecks Library presence: owned titles join the Lineup and only missing titles
	// become acquisitions.
	//
	// The DOMAIN type, not a hand-written mirror — the same call this file makes for
	// ProposalDTO.Proposal, and for the same reason recorded there: a mirror drifts from what it
	// mirrors.
	Add []proposals.ProposalItem `json:"add,omitempty" doc:"Titles to add; current Library presence determines Lineup or acquisition placement"`
	// Note is the message to whoever requested this — why it came back altered.
	Note string `json:"note,omitempty" doc:"A message to the requester, stored with the approval"`
	// Audience is the card's "Raise to <rating>" (#3012): a new audience limit, applied as the
	// channel is created. It may only loosen the proposal's own limit; the left-out titles it
	// admits come back in.
	Audience httpcore.RatingWire `json:"audience,omitempty" doc:"A higher audience limit for the channel; left-out titles it admits are included"`
}

type approveInput struct {
	ID string `path:"id"`
	// A POINTER, so the body is genuinely optional. With a value type huma requires one, and
	// `POST …/approve` with no body would 400 for every caller that approves unmodified (the
	// approve button, the e2e smoke, the handler tests). Edit-before-approve is additive.
	Body *ApprovalEditDTO
}

// approvalEditFromDTO maps the wire shape to the domain edit, returning nil when nothing was
// modified.
//
// NIL RATHER THAN AN EMPTY STRUCT is deliberate: an unmodified approval must carry no human
// edit and an empty ModSummary. The server approval gate may still canonicalise a missing or
// crafted series selector from the trusted original Intent; an empty-but-non-nil edit would
// falsely describe that server grounding as "approved with modifications: none".
func approvalEditFromDTO(b *ApprovalEditDTO) *accept.ApprovalEdit {
	if b == nil || (len(b.Drop) == 0 && len(b.Add) == 0 && b.Note == "" && b.Audience == "") {
		return nil
	}
	drop := make([]provision.Key, 0, len(b.Drop))
	for _, k := range b.Drop {
		drop = append(drop, provision.Key(k))
	}
	return &accept.ApprovalEdit{DropKeys: drop, Add: b.Add, Note: b.Note, Audience: rating.Rating(b.Audience)}
}

type approveOutput struct {
	Body struct {
		Status    string `json:"status"`
		Enqueued  int    `json:"enqueued" doc:"Acquisitions enqueued as wanted titles"`
		ChannelID string `json:"channelId" doc:"The channel atomically created or patched by this approval (§7)"`
	}
}

// errProposalTooOld is the answer for a proposal whose job was purged before the proposal was
// decided: its channel cannot be created, but nothing is wrong with the server.
func errProposalTooOld() huma.StatusError {
	return httpcore.Unprocessable("This request is too old to approve",
		"Loomarr no longer has the details this request needs to create a channel. Deny it and ask again.")
}

// approveProposal is the approval gate (§8/§11/§19): ADMIN ONLY. It flips the
// proposal to approved and enqueues each acquisition as a `wanted` title — the
// only path from a proposal to /v1/titles. Members cannot reach this (403), so
// nothing unapproved ever acquires.
func (s *Handlers) approveProposal(ctx context.Context, in *approveInput) (*approveOutput, error) {
	p, err := s.store.GetProposal(ctx, in.ID)
	if errors.Is(err, store.ErrNotFound) {
		return nil, httpcore.NotFound("Suggestion not found", "That suggestion doesn't exist — it may have expired or been removed.")
	}
	if err != nil {
		return nil, err
	}
	if p.Status != "submitted" {
		return nil, httpcore.Conflict("Already handled", "This suggestion has already been approved or dismissed.")
	}

	// The gate has ONE implementation, shared with the auto-approve path (§8, §11), so
	// the two can never disagree about what approving means.
	//
	// ⚠ The edit is PASSED to Approve rather than applied here first. Approving decides what
	// gets acquired; applying the edit in the handler would move that decision outside the gate
	// and leave auto-approve running different logic (§7, D-K).
	if s.approver == nil {
		return nil, errors.New("approve: approval coordinator is not configured")
	}
	result, err := s.approver.Approve(ctx, p, approvalEditFromDTO(in.Body), httpcore.UserIDFrom(ctx))
	if errors.Is(err, accept.ErrNotSubmitted) {
		return nil, httpcore.Conflict("Already handled", "This suggestion has already been approved or dismissed.")
	}
	if errors.Is(err, accept.ErrSuperseded) {
		return nil, httpcore.Conflict("Newer suggestion already approved", "This older version cannot replace a channel after a newer version was approved. Dismiss it from the queue instead.")
	}
	if errors.Is(err, accept.ErrProposalJobGone) {
		return nil, errProposalTooOld()
	}
	if errors.Is(err, accept.ErrRevisionActive) {
		return nil, httpcore.Conflict("Suggestions are updating", "Wait for the updated suggestions before creating this channel.")
	}
	if errors.Is(err, accept.ErrEmptyApproval) {
		return nil, httpcore.Unprocessable("Choose at least one title", "A channel needs at least one included title before it can be created.")
	}
	if errors.Is(err, accept.ErrAudienceNotRaised) {
		return nil, httpcore.Unprocessable("That audience limit isn't higher", "Choose a rating above this channel's current audience limit.")
	}
	if err != nil {
		return nil, err
	}
	out := &approveOutput{}
	out.Body.Status = "approved"
	out.Body.Enqueued = result.Enqueued
	out.Body.ChannelID = result.ChannelID

	// The Dashboard feed (§12, V32). Written AFTER the gate, so a line only appears for an
	// approval that actually happened — recording intent would put entries in the feed for
	// requests that were refused.
	//
	// Deliberately NOT inside accept.Approver: that is the one chokepoint every approve path
	// shares (§7), and threading a recorder through it to satisfy a dashboard panel would
	// widen the gate's signature for telemetry.
	//
	// The line says WHAT HAPPENED, not what was requested: a feed label is not worth reading
	// the proposal's channel name into the approval path.
	s.activity.Info(ctx, activitystore.ActivityKindProposal, p.ID, approvedLine(result.Enqueued))
	return out, nil
}

// --- bulk approve (V27) ---

type bulkApproveInput struct {
	Body struct {
		// IDs are approved one at a time, each through the SAME `approveProposal` handler. No
		// batch store write and no second gate: bulk approve goes through the same
		// chokepoint, and a batch path would be the second acquisition path §7 forbids.
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
	Status    int    `json:"status,omitempty" doc:"The HTTP status approving this id alone would have answered; absent on success"`
	Retryable bool   `json:"retryable,omitempty" doc:"True when the failure was a server or upstream fault, so retrying this id may succeed. Absent for a permanent refusal (not found, already handled, invalid, not set up)"`
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
// that makes a single approval correct (the submitted-status check, the one `Approver.Approve`
// call, the channel bind, the audit stamps) therefore applies unchanged, and a future change to
// approval semantics cannot land in one path and miss the other.
//
// Sequential, not concurrent: approvals write titles and channels, and household-scale batches
// are small. Concurrency here would buy milliseconds and risk interleaving writes.
func (s *Handlers) bulkApproveProposals(ctx context.Context, in *bulkApproveInput) (*bulkApproveOutput, error) {
	out := &bulkApproveOutput{}
	out.Body.Results = make([]bulkApproveResult, 0, len(in.Body.IDs))
	for _, id := range in.Body.IDs {
		res, err := s.approveProposal(ctx, &approveInput{ID: id})
		if err != nil {
			// A per-id failure is DATA, not a request failure: the ones that worked are already
			// durable, so 500ing the whole call would hide successful approvals behind an error.
			status, retryable := httpcore.BulkFailure(err)
			out.Body.Results = append(out.Body.Results, bulkApproveResult{
				ID: id, OK: false, Error: httpcore.ErrMessage(err), Status: status, Retryable: retryable,
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

func (s *Handlers) denyProposal(ctx context.Context, in *denyInput) (*denyOutput, error) {
	p, err := s.store.GetProposal(ctx, in.ID)
	if errors.Is(err, store.ErrNotFound) {
		return nil, httpcore.NotFound("Suggestion not found", "That suggestion doesn't exist — it may have expired or been removed.")
	}
	if err != nil {
		return nil, err
	}
	p.Status = "denied"
	p.ApprovedBy = httpcore.UserIDFrom(ctx)
	p.DenyReason = in.Body.Reason
	p.UpdatedAt = time.Now()
	if err := s.store.CommitProposalDenial(ctx, p); err != nil {
		switch {
		case errors.Is(err, proposalstore.ErrProposalNotSubmitted):
			return nil, httpcore.Conflict("Already handled", "This suggestion has already been approved or dismissed.")
		case errors.Is(err, proposalstore.ErrProposalRevisionActive):
			return nil, httpcore.Conflict("Suggestions are updating", "Wait for the updated suggestions before dismissing this channel.")
		default:
			return nil, err
		}
	}
	if s.decisionQuality != nil {
		s.decisionQuality.ProposalDenied(ctx, p.ID, p.UpdatedAt)
	}
	if s.proposalNotifications != nil {
		s.proposalNotifications.ProposalDenied(ctx, p)
	}
	out := &denyOutput{}
	out.Body.Status = "denied"
	return out, nil
}

// approvedLine is the Dashboard feed's wording for an approval (§12, V32).
//
// Zero enqueued is the COMMON case on a well-stocked library — every picked title is already
// there — so it gets its own phrasing rather than "0 titles queued", which reads like a
// failure of the thing that just succeeded.
func approvedLine(enqueued int) string {
	switch enqueued {
	case 0:
		return "Request approved — everything is already in the library"
	case 1:
		return "Request approved — 1 title queued for download"
	default:
		return fmt.Sprintf("Request approved — %d titles queued for download", enqueued)
	}
}

func (s *Handlers) getProposalOutlook(ctx context.Context, in *approveInput) (*struct{ Body outlook.Assessment }, error) {
	p, err := s.store.GetProposal(ctx, in.ID)
	if errors.Is(err, store.ErrNotFound) {
		return nil, httpcore.NotFound("Suggestion not found", "That suggestion no longer exists.")
	}
	if err != nil {
		return nil, err
	}
	if p.Status != "submitted" {
		return nil, httpcore.Conflict("Already handled", "Only pending suggestions have a pre-approval outlook.")
	}
	if s.proposalOutlook == nil {
		return nil, httpcore.FeatureNotConfigured("Outlook unavailable", "Connect the media library to estimate this lineup.")
	}
	assessment, err := s.proposalOutlook.Assess(ctx, p, approvalEditFromDTO(in.Body))
	if errors.Is(err, accept.ErrProposalJobGone) {
		return nil, errProposalTooOld()
	}
	if errors.Is(err, accept.ErrAudienceNotRaised) {
		return nil, httpcore.Unprocessable("That audience limit isn't higher", "Choose a rating above this channel's current audience limit.")
	}
	if err != nil {
		return nil, err
	}
	return &struct{ Body outlook.Assessment }{Body: assessment}, nil
}
