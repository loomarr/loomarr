package main

import (
	"context"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/proposalworkflow"
	"github.com/loomarr/loomarr/internal/provision"
	"github.com/loomarr/loomarr/internal/store"
)

// #1534: a seeded backend's Proposal Job must be listable through the real
// workflow — the same path GET /v1/proposal-jobs uses — not just readable as a
// raw store.Job. Seed used to write store.Job{Kind:"suggest",Status:"done"}
// without ever creating its versioned proposal_job_attempts row, so the
// workflow's own invariant check (a done Job must have a succeeded Attempt)
// rejected it with ErrInvalidState and the endpoint 500'd.
func TestSeedProposalJobsAreListableThroughTheProposalWorkflow(t *testing.T) {
	ctx := context.Background()
	st, err := store.Open(ctx, "sqlite://"+t.TempDir()+"/seed.db", true)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })

	if err := seedTitlesAndChannel(ctx, st, "admin-test"); err != nil {
		t.Fatal(err)
	}

	workflow := proposalworkflow.New(st, func() string { return "unused" }, time.Now)
	journeys, err := workflow.List(ctx, proposalworkflow.Viewer{UserID: "admin-test", Admin: true}, proposalworkflow.ListOptions{})
	if err != nil {
		t.Fatalf("List seeded Proposal Jobs through the workflow: %v", err)
	}
	if len(journeys) != 1 {
		t.Fatalf("Journeys = %d, want 1", len(journeys))
	}
	if journeys[0].Milestone != proposalworkflow.MilestoneBuilding {
		t.Fatalf("Milestone = %q, want %q", journeys[0].Milestone, proposalworkflow.MilestoneBuilding)
	}
}

func TestSeedClipsCreatesDistinctTaxonomyReadyCatalog(t *testing.T) {
	ctx := context.Background()
	st, err := store.Open(ctx, "sqlite://"+t.TempDir()+"/seed.db", true)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })

	if err := seedClips(ctx, st); err != nil {
		t.Fatal(err)
	}
	clips, err := st.ListClips(ctx, store.ClipFilter{})
	if err != nil {
		t.Fatal(err)
	}
	if len(clips) != 4 {
		t.Fatalf("seeded filler clips = %d, want 4 distinct rows", len(clips))
	}
	seen := map[string]bool{}
	for _, clip := range clips {
		if clip.Hash == "" || clip.Path == "" {
			t.Errorf("seed clip %q has empty identity: hash=%q path=%q", clip.Name, clip.Hash, clip.Path)
		}
		if seen[clip.Hash] {
			t.Errorf("seed clip hash %q is duplicated", clip.Hash)
		}
		seen[clip.Hash] = true
		if len(clip.AssertedTags) == 0 {
			t.Errorf("seed clip %q has no directly asserted taxonomy tags", clip.Name)
		}
	}
}

func TestSeedApprovalPreservesPendingAcquisitionsInChannel(t *testing.T) {
	ctx := context.Background()
	st, err := store.Open(ctx, "sqlite://"+t.TempDir()+"/seed.db", true)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })

	if err := seedTitlesAndChannel(ctx, st, "admin-test"); err != nil {
		t.Fatal(err)
	}
	channels, err := st.ListChannels(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(channels) != 1 {
		t.Fatalf("channels = %d, want 1", len(channels))
	}
	got := make(map[provision.Key]bool, len(channels[0].Lineup))
	for _, entry := range channels[0].Lineup {
		got[entry.Key] = true
	}
	for _, id := range []string{"demo-film-08", "demo-film-26", "demo-film-29"} {
		key := provision.Key(demoFilm(id).Key())
		if !got[key] {
			t.Errorf("approved acquisition %s was stripped from the seeded channel", key)
		}
	}
}
