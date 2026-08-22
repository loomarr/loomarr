package proposalworkflow

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/mantonx/loomarr/internal/provision"
	"github.com/mantonx/loomarr/internal/store"
	"github.com/mantonx/loomarr/internal/suggest"
)

func TestStoreWorkflowRecoversCrashAndRejectsLateAttemptResult(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	st, err := store.Open(ctx, "sqlite://"+filepath.Join(t.TempDir(), "workflow.db"), true)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })

	started := time.Date(2026, time.August, 22, 16, 0, 0, 0, time.UTC)
	clock := started
	workflow := New(st, func() string { return "proposal-1" }, func() time.Time { return clock })
	intent := suggest.Intent{Description: "Saturday morning cartoons"}
	if err := st.CreateJob(ctx, store.Job{
		ID: "job-1", Kind: "suggest", Status: "queued", CreatedBy: "member-1",
		IntentJSON:      `{"description":"Saturday morning cartoons"}`,
		WorkflowVersion: store.ProposalWorkflowVersion,
		Deadline:        started, CreatedAt: started, UpdatedAt: started,
	}); err != nil {
		t.Fatal(err)
	}

	queued, err := workflow.Inspect(ctx, Viewer{UserID: "member-1"}, "job-1")
	if err != nil || queued.Milestone != MilestoneGenerating || len(queued.Attempts) != 0 {
		t.Fatalf("queued Journey = (%+v, %v)", queued, err)
	}

	first, err := workflow.Claim(ctx, started, time.Minute, 1)
	if err != nil || len(first) != 1 || first[0].Attempt != 1 || first[0].Intent.Description != intent.Description {
		t.Fatalf("first claim = (%+v, %v)", first, err)
	}

	// Simulate the process disappearing after claim. Once its lease expires, a
	// different worker recovers the same Job with a fresh compare-and-swap token.
	clock = started.Add(2 * time.Minute)
	second, err := workflow.Claim(ctx, clock, time.Minute, 1)
	if err != nil || len(second) != 1 || second[0].Attempt != 2 {
		t.Fatalf("recovery claim = (%+v, %v)", second, err)
	}

	proposal := suggest.Proposal{Intent: intent, Lineup: []suggest.ProposalItem{{
		MediaType: provision.Movie, TMDBID: 603, Name: "The Matrix", InLibrary: true,
	}}}
	if err := workflow.Complete(ctx, first[0], proposal); !errors.Is(err, ErrStaleAttempt) {
		t.Fatalf("late Attempt completion = %v, want ErrStaleAttempt", err)
	}
	if _, err := st.GetProposal(ctx, "proposal-1"); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("late Attempt published Proposal: %v", err)
	}
	if err := workflow.Complete(ctx, second[0], proposal); err != nil {
		t.Fatalf("current Attempt completion: %v", err)
	}

	journey, err := workflow.Inspect(ctx, Viewer{UserID: "member-1"}, "job-1")
	if err != nil {
		t.Fatal(err)
	}
	if journey.Milestone != MilestoneAwaitingApproval || journey.Proposal == nil || journey.Proposal.ID != "proposal-1" {
		t.Fatalf("completed Journey = %+v", journey)
	}
	if len(journey.Attempts) != 2 || journey.Attempts[0].Status != AttemptInterrupted ||
		journey.Attempts[1].Status != AttemptSucceeded {
		t.Fatalf("completed Attempt history = %+v", journey.Attempts)
	}
}
