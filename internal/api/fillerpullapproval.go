package api

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/loomarr/loomarr/internal/filler"
	"github.com/loomarr/loomarr/internal/fillerstore"
	"github.com/loomarr/loomarr/internal/store"
)

type approveFillerPullInput struct {
	ID   string `path:"id"`
	Body struct {
		// DropSourceIDs are the rows the operator excluded. Recorded on the pull rather than
		// removed from it, so the audit shows what was proposed as well as what was agreed to.
		DropSourceIDs    []string `json:"dropSourceIds,omitempty"`
		DropCandidateIDs []string `json:"dropCandidateIds,omitempty"`
		Note             string   `json:"note,omitempty" doc:"Optional annotation for the approval record; does not change what is downloaded"`
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
	// Fast feedback for a visible decision; the transaction below owns concurrent decisions.
	if p.Status != filler.PullPending {
		return nil, errConflict("That pull has already been decided",
			"Someone already "+string(p.Status)+" this pull. Propose a new one if you still want the clips.")
	}

	dropped := make(map[string]bool, len(in.Body.DropSourceIDs)+len(in.Body.DropCandidateIDs))
	for _, id := range in.Body.DropSourceIDs {
		dropped[id] = true
	}
	for _, id := range in.Body.DropCandidateIDs {
		dropped[id] = true
	}
	for i := range p.Plan {
		if dropped[p.Plan[i].SourceID] || dropped[p.Plan[i].CandidateID()] {
			p.Plan[i].Dropped = true
		}
	}

	committed := p.Committed()
	if len(committed) == 0 {
		return nil, errConflict("Nothing left to pull",
			"Every source in this plan was dropped, so there's nothing to fetch. Dismiss it instead.")
	}

	// Re-check registered policy at the commit point; proposal-time source state is evidence, not
	// authority after the pull has waited in the queue.
	srcs, err := s.store.ListFillerSources(ctx)
	if err != nil {
		return nil, huma.Error500InternalServerError("list filler sources", err)
	}
	live := make(map[string]fillerstore.FillerSource, len(srcs))
	home := s.fillerHomeGeography()
	for _, src := range srcs {
		if src.EffectiveEnabled() && src.Fetchable() && src.GeographicallyEligible(home) {
			live[src.ID] = src
		}
	}

	remoteStates, err := s.store.ListAcquisitionRemoteStates(ctx)
	if err != nil {
		return nil, huma.Error500InternalServerError("revalidate acquired filler candidates", err)
	}
	otherPulls, err := s.store.ListPulls(ctx, "")
	if err != nil {
		return nil, huma.Error500InternalServerError("revalidate queued filler candidates", err)
	}
	for _, other := range otherPulls {
		if other.ID == p.ID || other.Status == filler.PullDismissed {
			continue
		}
		for _, row := range other.Committed() {
			if row.Identity().Validate() == nil {
				remoteStates[row.Identity().Key()] = filler.RemoteQueued
			}
		}
	}

	targets := make([]filler.AcquisitionTarget, 0, len(committed))
	for _, row := range committed {
		src, ok := live[row.SourceID]
		if !ok {
			return nil, errConflict("A source in this pull is no longer available",
				"“"+row.Name+"” has been switched off or removed since this pull was proposed. Dismiss it and propose a new one.")
		}
		if row.Identity().Validate() == nil && remoteStates[row.Identity().Key()] != "" {
			return nil, errConflict("A candidate in this pull is already acquired or queued",
				"Another decision reached “"+row.Name+"” while this pull was waiting. Dismiss it and propose a fresh plan.")
		}
		url := row.URL
		if url == "" {
			url = src.URI // compatibility for source-level V35-V65 pull records
		} else if row.Provider != src.Kind || row.RemoteID == "" {
			return nil, errConflict("A candidate in this pull no longer matches its source",
				"The candidate identity no longer belongs to the registered provider. Dismiss it and propose a new pull.")
		}
		targets = append(targets, filler.AcquisitionTarget{SourceID: src.ID, RemoteID: row.RemoteID, Kind: src.Kind, URL: url, Gap: row.Gap})
	}

	p.Status = filler.PullApproved
	p.Note = strings.TrimSpace(in.Body.Note)
	p.DecidedAt = time.Now().UTC()
	p.DecidedBy = auditActor(ctx)
	if _, err := s.filler.IngestPull(ctx, p.ID, targets, func(commitCtx context.Context, run filler.AcquisitionRun) error {
		return s.store.CommitPullApproval(commitCtx, p, run)
	}); err != nil {
		if errors.Is(err, fillerstore.ErrPullNotPending) {
			return nil, errConflict("That pull has already been decided or started", "Another decision or acquisition already owns this pull. Review its history before proposing a new one.")
		}
		if errors.Is(err, ErrIngestUnavailable) {
			return nil, errConflict("Downloading isn't available on this install",
				"This build can't run the download tooling, so an approved pull would have nothing to fetch with.")
		}
		return nil, apiErrWithCause(http.StatusBadGateway, "Couldn't start the pull",
			"Loomarr couldn't start downloading. Check the Filler sources and try again.", err)
	}

	return &pullOutput{Body: pullToDTO(p)}, nil
}

// --- bulk approve ---

type bulkApproveFillerPullInput struct {
	Body struct {
		// IDs are approved one at a time, each through the SAME approveFillerPull handler (the
		// commit point). No batch store write and no second gate — mirrors bulk-approve-proposals.
		//
		// maxItems:100 (Refs #1659): the native Requests screen approves a whole group of filler
		// downloads at once, and groups can exceed the 24-25 used elsewhere in this API for
		// interactive lookups — each item only queues work through the existing single-approve
		// path below, so this bounds request size without forcing the client to chunk. Deliberately
		// NOT applied to bulk-approve-proposals (internal/api/proposals.go): that is an existing,
		// uncapped contract, and capping it is outside this change.
		IDs []string `json:"ids" minItems:"1" maxItems:"100" doc:"Pull ids to approve, each through the single approval gate"`
	}
}

type bulkApproveFillerPullResult struct {
	ID    string `json:"id"`
	OK    bool   `json:"ok"`
	Error string `json:"error,omitempty" doc:"Why this one did not approve (already handled, not found, …)"`
}

type bulkApproveFillerPullOutput struct {
	Body struct {
		// PER-ID results rather than a single status: one id that was already decided must not
		// hide the rest, but the caller still has to learn which ones did not go through.
		Results  []bulkApproveFillerPullResult `json:"results"`
		Approved int                           `json:"approved" doc:"How many pulls were approved"`
	}
}

// bulkApproveFillerPulls approves several pulls in one request (Refs #1659). ADMIN ONLY, like the
// single approve it delegates to.
//
// ⚠ It calls `s.approveFillerPull` per id — the same handler, not a copy of its body. Everything
// that makes a single approval correct (the pending-status check, the source/candidate
// revalidation, the ingest commit, the audit stamps) therefore applies unchanged, and a future
// change to approval semantics cannot land in one path and miss the other. Approving a pull starts
// downloads, so there is no second implementation of that here.
//
// Sequential, not concurrent, like bulk-approve-proposals: approvals write pulls and start
// downloads, and household-scale batches are small. Concurrency here would buy milliseconds and
// risk interleaving writes.
func (s *Server) bulkApproveFillerPulls(ctx context.Context, in *bulkApproveFillerPullInput) (*bulkApproveFillerPullOutput, error) {
	out := &bulkApproveFillerPullOutput{}
	out.Body.Results = make([]bulkApproveFillerPullResult, 0, len(in.Body.IDs))
	for _, id := range in.Body.IDs {
		_, err := s.approveFillerPull(ctx, &approveFillerPullInput{ID: id})
		if err != nil {
			// A per-id failure is DATA, not a request failure: the ones that worked are already
			// durable, so 500ing the whole call would hide successful approvals behind an error.
			out.Body.Results = append(out.Body.Results, bulkApproveFillerPullResult{
				ID: id, OK: false, Error: humaErrMessage(err),
			})
			continue
		}
		out.Body.Results = append(out.Body.Results, bulkApproveFillerPullResult{ID: id, OK: true})
		out.Body.Approved++
	}
	return out, nil
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
	if err := s.store.DismissPull(ctx, p); err != nil {
		if errors.Is(err, fillerstore.ErrPullNotPending) {
			return nil, errConflict("That pull has already been decided", "Another decision won. Reload the pull to see its history.")
		}
		return nil, huma.Error500InternalServerError("save pull decision", err)
	}
	return &pullOutput{Body: pullToDTO(p)}, nil
}
