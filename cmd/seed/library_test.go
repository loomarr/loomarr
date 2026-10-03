package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/proposalworkflow"
	"github.com/loomarr/loomarr/internal/provision"
	"github.com/loomarr/loomarr/internal/store"
	"github.com/loomarr/loomarr/internal/suggest"
)

// #1534: seedLibraryChannels (SEED_LIBRARY_PICKS) hit the same bug as the
// default seed — it wrote a done store.Job with no versioned Attempt, so
// listing it through the real workflow (what GET /v1/proposal-jobs does)
// failed with ErrInvalidState instead of 200.
func TestSeedLibraryChannelsProposalJobsAreListableThroughTheProposalWorkflow(t *testing.T) {
	ctx := context.Background()
	st, err := store.Open(ctx, "sqlite://"+t.TempDir()+"/seed.db", true)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })

	now := time.Now()
	admin := store.User{ID: "admin-test", Name: "admin", Role: store.RoleAdmin, CreatedAt: now, UpdatedAt: now}
	if err := st.UpsertUser(ctx, admin); err != nil {
		t.Fatal(err)
	}

	picks := []suggest.ProposalItem{{
		MediaType: provision.Movie, TMDBID: 90000101, Name: "Library Pick One", Year: 1991,
		InLibrary: true, LibraryItemID: "lib-item-1",
	}}
	raw, err := json.Marshal(picks)
	if err != nil {
		t.Fatal(err)
	}
	picksPath := filepath.Join(t.TempDir(), "picks.json")
	if err := os.WriteFile(picksPath, raw, 0o600); err != nil {
		t.Fatal(err)
	}

	if err := seedLibraryChannels(ctx, st, picksPath); err != nil {
		t.Fatal(err)
	}

	workflow := proposalworkflow.New(st, func() string { return "unused" }, time.Now)
	journeys, err := workflow.List(ctx, proposalworkflow.Viewer{UserID: admin.ID, Admin: true}, proposalworkflow.ListOptions{})
	if err != nil {
		t.Fatalf("List seeded library Proposal Jobs through the workflow: %v", err)
	}
	if len(journeys) != 1 {
		t.Fatalf("Journeys = %d, want 1", len(journeys))
	}
	if journeys[0].Milestone != proposalworkflow.MilestoneBuilding {
		t.Fatalf("Milestone = %q, want %q", journeys[0].Milestone, proposalworkflow.MilestoneBuilding)
	}
}
