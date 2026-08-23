package api

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/loomarr/loomarr/internal/filler"
	"github.com/loomarr/loomarr/internal/store"
)

// Filler pulls — the approval gate for filler acquisition (§10 V35).
//
// ⚠ **The safety property this file exists for: a pending pull downloads NOTHING.** Proposing
// writes a row. Approving is the one place work is enqueued, and it enqueues through the
// EXISTING ingest path — a pull that downloaded through its own route would be a second
// implementation of ingest, which is the shape §10 rejects by name and the shape that let
// `filler_sources` ship with no reader.
//
// ⚠ **What the gate binds is composed plans, not an admin's own hands.** An admin searching one
// source and clicking "Queue download" on one result stays direct (`POST /v1/filler/ingest`),
// mirroring §7 where an admin may `POST /v1/titles` because the admin *is* the gate.

// PullPlanRowDTO is one source a pull would draw from.
type PullPlanRowDTO struct {
	SourceID string `json:"sourceId" doc:"The registered collection this row fetches from"`
	Tag      string `json:"tag" doc:"Short label for the row's chip (an era, an audience)"`
	Name     string `json:"name"`
	Why      string `json:"why" doc:"Why THIS source is in the plan"`
	// ⚠ An estimate, never a promise: what a source yields depends on what is still there,
	// what deduplicates against the catalog, and what the splitter makes of a compilation.
	EstimateClips int  `json:"estimateClips" doc:"Expected clips from this row. An ESTIMATE — render it as one."`
	Dropped       bool `json:"dropped" doc:"The operator excluded this row before approving. Retained rather than removed, so the record shows what was proposed as well as what was agreed to."`
}

// PullDTO is a proposed acquisition awaiting a human.
type PullDTO struct {
	ID         string           `json:"id"`
	Title      string           `json:"title"`
	Reason     string           `json:"reason" doc:"The gap this pull closes. Shown above the plan — 'approve this' without a reason is a button, not a decision."`
	ProposedBy string           `json:"proposedBy"`
	Status     string           `json:"status" enum:"pending,approved,dismissed"`
	Note       string           `json:"note,omitempty" doc:"The operator's narrowing instruction, captured at approval"`
	Plan       []PullPlanRowDTO `json:"plan"`
	// EstimateClips totals the rows the operator has NOT dropped.
	EstimateClips int    `json:"estimateClips"`
	CreatedAt     string `json:"createdAt" doc:"RFC3339"`
	DecidedAt     string `json:"decidedAt,omitempty" doc:"RFC3339; absent while pending"`
	DecidedBy     string `json:"decidedBy,omitempty"`
}

func pullToDTO(p filler.Pull) PullDTO {
	// Non-nil even when empty: a JSON `null` here would make every consumer guard before
	// iterating, and a pull with no rows is a real (refused) state, not a missing one.
	rows := make([]PullPlanRowDTO, 0, len(p.Plan))
	for _, r := range p.Plan {
		rows = append(rows, PullPlanRowDTO{
			SourceID: r.SourceID, Tag: r.Tag, Name: r.Name, Why: r.Why,
			EstimateClips: r.EstimateClips, Dropped: r.Dropped,
		})
	}
	dto := PullDTO{
		ID: p.ID, Title: p.Title, Reason: p.Reason, ProposedBy: p.ProposedBy,
		Status: string(p.Status), Note: p.Note, Plan: rows,
		EstimateClips: p.EstimatedClips(),
		CreatedAt:     p.CreatedAt.UTC().Format(time.RFC3339),
		DecidedBy:     p.DecidedBy,
	}
	if !p.DecidedAt.IsZero() {
		dto.DecidedAt = p.DecidedAt.UTC().Format(time.RFC3339)
	}
	return dto
}

func (s *Server) registerFillerPulls(api huma.API) {
	huma.Register(api, withRole(huma.Operation{
		OperationID: "propose-filler-pull", Method: http.MethodPost, Path: "/v1/filler/pulls",
		Summary: "Propose a pull",
		Description: "Admin only (§10 V35). Composes a plan across the ENABLED filler sources and writes it " +
			"to the approval queue. ⚠ **Downloads nothing** — that is the whole point: the machine proposes, " +
			"a human commits. Refused with 409 when every source the plan would need is switched off, naming " +
			"the switch to flip rather than writing a pull that could never run.",
		Tags: []string{"filler"},
	}, RoleAdmin), s.proposeFillerPull)

	huma.Register(api, withRole(huma.Operation{
		OperationID: "list-filler-pulls", Method: http.MethodGet, Path: "/v1/filler/pulls",
		Summary: "List pulls awaiting a decision",
		Description: "Admin only (§10 V35). `status` filters (pending | approved | dismissed); omit it for all. " +
			"Decided pulls are KEPT — the queue's History answers what was agreed to and who said so.",
		Tags: []string{"filler"},
	}, RoleAdmin), s.listFillerPulls)

	huma.Register(api, withRole(huma.Operation{
		OperationID: "approve-filler-pull", Method: http.MethodPost, Path: "/v1/filler/pulls/{id}/approve",
		Summary: "Approve a pull — the commit point",
		Description: "Admin only (§10 V35). THE gate: this is the only path on which a pull downloads anything, " +
			"and it enqueues through the EXISTING ingest job rather than a route of its own. The body carries the " +
			"operator's edits — rows dropped, and a note narrowing what to fetch. Dropped rows are recorded, not " +
			"erased. Approving an already-decided pull is a 409, so a double-click cannot enqueue twice.",
		Tags: []string{"filler"},
	}, RoleAdmin), s.approveFillerPull)

	huma.Register(api, withRole(huma.Operation{
		OperationID: "dismiss-filler-pull", Method: http.MethodPost, Path: "/v1/filler/pulls/{id}/dismiss",
		Summary:     "Decline a pull",
		Description: "Admin only (§10 V35). Records the decision and downloads nothing. The row is kept.",
		Tags:        []string{"filler"},
	}, RoleAdmin), s.dismissFillerPull)
}

type proposeFillerPullInput struct {
	Body struct {
		Title  string `json:"title,omitempty" doc:"Optional operator-supplied summary; Loomarr composes one when omitted"`
		Reason string `json:"reason,omitempty" doc:"Optional; the gap this pull closes"`
	}
}

type pullOutput struct {
	Body PullDTO
}

// proposeFillerPull composes a plan and writes it to the queue. It downloads nothing.
func (s *Server) proposeFillerPull(ctx context.Context, in *proposeFillerPullInput) (*pullOutput, error) {
	if s.store == nil {
		return nil, huma.Error501NotImplemented("no store configured")
	}

	srcs, err := s.store.ListFillerSources(ctx)
	if err != nil {
		return nil, huma.Error500InternalServerError("list filler sources", err)
	}
	// ⚠ Only ENABLED sources may enter a plan. This is the precondition the mock draws as its
	// own empty state, and it belongs here rather than in the UI: a pull composed from a
	// switched-off source is one that can never run, and discovering that at approval time —
	// after a human agreed to it — is the worst moment to find out.
	// ⚠ `Fetchable()` as well as `Enabled` (V37). The flat table now also holds the config-backed
	// singletons, which are SCANNED rather than downloaded from — including one in a plan would
	// enqueue a fetch of an empty URL at approval time.
	var enabled []store.FillerSource
	for _, src := range srcs {
		if src.Enabled && src.Fetchable() {
			enabled = append(enabled, src)
		}
	}
	if len(enabled) == 0 {
		return nil, errConflict("Every source is switched off",
			"Loomarr can't plan a pull because none of your filler sources are turned on. Switch one on under Filler → Sources, then try again.")
	}

	now := time.Now().UTC()
	plan := make([]filler.PullPlanRow, 0, len(enabled))
	for _, src := range enabled {
		plan = append(plan, filler.PullPlanRow{
			SourceID: src.ID,
			Tag:      src.Kind,
			Name:     orPlaceholder(src.Label, src.URI),
			Why:      "A source you added and left switched on.",
			// ⚠ Deliberately 0, and the DTO says an estimate is an estimate. A real per-source
			// forecast is an upstream search per row at propose time; inventing a number here
			// would put a figure in front of an operator that nothing measured. See §6.5.
			EstimateClips: 0,
		})
	}

	p := filler.Pull{
		ID:         "pull_" + fmt.Sprintf("%d", now.UnixNano()),
		Title:      orPlaceholder(strings.TrimSpace(in.Body.Title), "Pull filler from your sources"),
		Reason:     orPlaceholder(strings.TrimSpace(in.Body.Reason), "Your catalog can be topped up from the sources you have switched on."),
		ProposedBy: auditActor(ctx),
		Status:     filler.PullPending,
		Plan:       plan,
		CreatedAt:  now,
	}
	if err := s.store.UpsertPull(ctx, p); err != nil {
		return nil, huma.Error500InternalServerError("save pull", err)
	}
	return &pullOutput{Body: pullToDTO(p)}, nil
}

type listFillerPullsInput struct {
	Status string `query:"status" enum:"pending,approved,dismissed," doc:"Omit for all"`
}

type listFillerPullsOutput struct {
	Body struct {
		Pulls []PullDTO `json:"pulls"`
		Total int       `json:"total"`
	}
}

func (s *Server) listFillerPulls(ctx context.Context, in *listFillerPullsInput) (*listFillerPullsOutput, error) {
	if s.store == nil {
		return nil, huma.Error501NotImplemented("no store configured")
	}
	pulls, err := s.store.ListPulls(ctx, filler.PullStatus(in.Status))
	if err != nil {
		return nil, huma.Error500InternalServerError("list pulls", err)
	}
	out := &listFillerPullsOutput{}
	out.Body.Pulls = make([]PullDTO, 0, len(pulls))
	for _, p := range pulls {
		out.Body.Pulls = append(out.Body.Pulls, pullToDTO(p))
	}
	out.Body.Total = len(out.Body.Pulls)
	return out, nil
}

type approveFillerPullInput struct {
	ID   string `path:"id"`
	Body struct {
		// DropSourceIDs are the rows the operator excluded. Recorded on the pull rather than
		// removed from it, so the audit shows what was proposed as well as what was agreed to.
		DropSourceIDs []string `json:"dropSourceIds,omitempty"`
		Note          string   `json:"note,omitempty" doc:"Narrowing instruction, e.g. 'no local dealers, no PSAs'"`
	}
}

// approveFillerPull is THE commit point — the only path on which a pull downloads anything.
func (s *Server) approveFillerPull(ctx context.Context, in *approveFillerPullInput) (*pullOutput, error) {
	if s.store == nil {
		return nil, huma.Error501NotImplemented("no store configured")
	}
	if s.filler == nil {
		return nil, errNotImplemented("Filler isn't set up",
			"Set up commercials and filler before approving a pull.")
	}

	p, err := s.store.GetPull(ctx, in.ID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return nil, errNotFound("Pull not found", "That pull doesn't exist — it may have been removed.")
		}
		return nil, huma.Error500InternalServerError("read pull", err)
	}
	// ⚠ A decided pull cannot be approved again. Without this a double-click, a retried
	// request, or a second admin on the same queue enqueues the same downloads twice.
	if p.Status != filler.PullPending {
		return nil, errConflict("That pull has already been decided",
			"Someone already "+string(p.Status)+" this pull. Propose a new one if you still want the clips.")
	}

	dropped := make(map[string]bool, len(in.Body.DropSourceIDs))
	for _, id := range in.Body.DropSourceIDs {
		dropped[id] = true
	}
	for i := range p.Plan {
		if dropped[p.Plan[i].SourceID] {
			p.Plan[i].Dropped = true
		}
	}

	committed := p.Committed()
	if len(committed) == 0 {
		// Refused rather than recorded as an approval that fetched nothing — "approved" with
		// an empty commit set is indistinguishable in the history from an approval that failed.
		return nil, errConflict("Nothing left to pull",
			"Every source in this plan was dropped, so there's nothing to fetch. Dismiss it instead.")
	}

	// ⚠ Re-checked at the COMMIT point, not trusted from propose time. A source can be switched
	// off or removed while a pull sits in the queue, and approving into a disabled source would
	// either fail obscurely or quietly fetch from something the operator turned off.
	srcs, err := s.store.ListFillerSources(ctx)
	if err != nil {
		return nil, huma.Error500InternalServerError("list filler sources", err)
	}
	// Same pair of conditions as propose — deliberately the same predicate, via the same method,
	// so the commit-time re-check cannot drift from what was plannable at propose time.
	live := make(map[string]store.FillerSource, len(srcs))
	for _, src := range srcs {
		if src.Enabled && src.Fetchable() {
			live[src.ID] = src
		}
	}
	urls := make([]string, 0, len(committed))
	for _, row := range committed {
		src, ok := live[row.SourceID]
		if !ok {
			return nil, errConflict("A source in this pull is no longer available",
				"“"+row.Name+"” has been switched off or removed since this pull was proposed. Dismiss it and propose a new one.")
		}
		urls = append(urls, src.URI)
	}

	// THE gate's other half: the work goes through the ordinary ingest job. A pull never
	// downloads by a route of its own.
	if _, err := s.filler.Ingest(ctx, urls); err != nil {
		if errors.Is(err, ErrIngestUnavailable) {
			return nil, errConflict("Downloading isn't available on this install",
				"This build can't run the download tooling, so an approved pull would have nothing to fetch with.")
		}
		return nil, apiErrWithCause(http.StatusBadGateway, "Couldn't start the pull",
			"Loomarr couldn't start downloading. Check the Filler sources and try again.", err)
	}

	p.Status = filler.PullApproved
	p.Note = strings.TrimSpace(in.Body.Note)
	p.DecidedAt = time.Now().UTC()
	p.DecidedBy = auditActor(ctx)
	if err := s.store.UpsertPull(ctx, p); err != nil {
		return nil, huma.Error500InternalServerError("save pull decision", err)
	}
	return &pullOutput{Body: pullToDTO(p)}, nil
}

type dismissFillerPullInput struct {
	ID string `path:"id"`
}

func (s *Server) dismissFillerPull(ctx context.Context, in *dismissFillerPullInput) (*pullOutput, error) {
	if s.store == nil {
		return nil, huma.Error501NotImplemented("no store configured")
	}
	p, err := s.store.GetPull(ctx, in.ID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return nil, errNotFound("Pull not found", "That pull doesn't exist — it may have been removed.")
		}
		return nil, huma.Error500InternalServerError("read pull", err)
	}
	if p.Status != filler.PullPending {
		return nil, errConflict("That pull has already been decided",
			"Someone already "+string(p.Status)+" this pull.")
	}
	p.Status = filler.PullDismissed
	p.DecidedAt = time.Now().UTC()
	p.DecidedBy = auditActor(ctx)
	if err := s.store.UpsertPull(ctx, p); err != nil {
		return nil, huma.Error500InternalServerError("save pull decision", err)
	}
	return &pullOutput{Body: pullToDTO(p)}, nil
}
