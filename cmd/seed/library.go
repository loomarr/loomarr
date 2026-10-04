package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"log/slog"
	"os"
	"time"

	"github.com/loomarr/loomarr/internal/binder"
	"github.com/loomarr/loomarr/internal/store"
	"github.com/loomarr/loomarr/internal/suggest"
)

// seedLibraryChannels makes one channel per real library title, for live checks on an isolated
// backend when no LLM is available to write proposals. picksPath holds a JSON array of in-library
// candidates exactly as GET /v1/search?scope=library returns them (mediaType, tmdbId/tvdbId, name,
// year, inLibrary, libraryItemId), so every channel plays real media from the configured server.
//
// It uses the same gate as the default seed: each title becomes a stored proposal that
// suggest.Approver approves, which makes the title `available` and binds its channel. No
// `available` row or lineup is written directly. The running backend's reconcile then computes each
// channel's schedule from the media server, like any approved channel.
func seedLibraryChannels(ctx context.Context, st store.Store, picksPath string) error {
	raw, err := os.ReadFile(picksPath)
	if err != nil {
		return fmt.Errorf("read picks: %w", err)
	}
	var picks []suggest.ProposalItem
	if err := json.Unmarshal(raw, &picks); err != nil {
		return fmt.Errorf("parse picks: %w", err)
	}
	adminID, err := existingAdmin(ctx, st)
	if err != nil {
		return err
	}
	approver := suggest.NewApprover(st, binder.New(st, nil, nil, slog.New(slog.DiscardHandler)), time.Now)
	// One minter per kind for the run, prefixed by the run's start so a later run never reuses an id.
	run := time.Now().Unix()
	jobIDs, propIDs := newID(fmt.Sprintf("job_lib%d", run)), newID(fmt.Sprintf("prop_lib%d", run))
	for i, pick := range picks {
		if !pick.InLibrary || pick.LibraryItemID == "" {
			return fmt.Errorf("pick %q is not an in-library search result", pick.Name)
		}
		now := time.Now()
		intent := suggest.Intent{Description: pick.Name}
		intentJSON, err := json.Marshal(intent)
		if err != nil {
			return err
		}
		jobID := jobIDs()
		propJSON, err := json.Marshal(suggest.Proposal{Intent: intent, Lineup: []suggest.ProposalItem{pick}})
		if err != nil {
			return err
		}
		propID := propIDs()
		// CreateSuggestionResult (built for #1720's no-generation proposals) writes the done
		// Job, its succeeded Attempt 1, and the submitted Proposal in one transaction — the
		// same shape the real suggester worker leaves behind. A raw CreateJob+CreateProposal
		// pair left no proposal_job_attempts row, so the workflow's own invariant check
		// rejected the seeded Job with ErrInvalidState and GET /v1/proposal-jobs 500'd (#1534).
		if err := st.CreateSuggestionResult(ctx, store.Job{
			ID: jobID, Kind: "suggest", Status: "done", IntentJSON: string(intentJSON),
			IntentHash: fmt.Sprintf("seed-library-%d-%s", i, pick.LibraryItemID), CreatedBy: adminID,
			CreatedAt: now, UpdatedAt: now,
		}, store.Proposal{
			ID: propID, JobID: jobID, Status: "submitted", CreatedBy: adminID,
			ProposalJSON: string(propJSON), CreatedAt: now, UpdatedAt: now,
		}); err != nil {
			return fmt.Errorf("create suggestion result: %w", err)
		}
		stored, err := st.GetProposal(ctx, propID)
		if err != nil {
			return fmt.Errorf("get proposal: %w", err)
		}
		approved, err := approver.Approve(ctx, stored, nil, adminID)
		if err != nil {
			return fmt.Errorf("approve %q (the gate): %w", pick.Name, err)
		}
		log.Printf("seed: library channel %s ← %s %q (library item %s)", approved.ChannelID, pick.MediaType, pick.Name, pick.LibraryItemID)
	}
	return nil
}

// existingAdmin is the first admin of an already-bootstrapped store, who owns the proposals.
func existingAdmin(ctx context.Context, st store.Store) (string, error) {
	users, err := st.ListUsers(ctx)
	if err != nil {
		return "", fmt.Errorf("list users: %w", err)
	}
	for _, u := range users {
		if u.Role == store.RoleAdmin {
			return u.ID, nil
		}
	}
	return "", errors.New("no admin: sign in to the backend once (dev login) or run the default seed first")
}
