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
	if _, err := workflow.Complete(ctx, first[0], proposal); !errors.Is(err, ErrStaleAttempt) {
		t.Fatalf("late Attempt completion = %v, want ErrStaleAttempt", err)
	}
	if _, err := st.GetProposal(ctx, "proposal-1"); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("late Attempt published Proposal: %v", err)
	}
	if _, err := workflow.Complete(ctx, second[0], proposal); err != nil {
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

func TestStoreWorkflowPersistsPrivateDiagnosticButProjectsSafeFailure(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	st, err := store.Open(ctx, "sqlite://"+filepath.Join(t.TempDir(), "failure.db"), true)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	now := time.Date(2026, time.August, 22, 17, 0, 0, 0, time.UTC)
	workflow := New(st, func() string { return "unused" }, func() time.Time { return now })
	if err := st.CreateJob(ctx, store.Job{
		ID: "job-failed", Kind: "suggest", Status: "queued", CreatedBy: "member-1",
		IntentJSON:      `{"description":"Obscure regional monster movies"}`,
		WorkflowVersion: store.ProposalWorkflowVersion,
		Deadline:        now, CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}
	works, err := workflow.Claim(ctx, now, time.Minute, 1)
	if err != nil || len(works) != 1 {
		t.Fatalf("Claim = %+v, %v", works, err)
	}
	diagnostic := "provider secret-token returned catalog internals"
	if err := workflow.Fail(ctx, works[0], string(FailureNoGroundedTitles), diagnostic); err != nil {
		t.Fatal(err)
	}

	journey, err := workflow.Inspect(ctx, Viewer{UserID: "member-1"}, "job-failed")
	if err != nil {
		t.Fatal(err)
	}
	if journey.Failure == nil || journey.Failure.Code != FailureNoGroundedTitles ||
		journey.Failure.Message == diagnostic {
		t.Fatalf("safe Journey failure = %+v", journey.Failure)
	}
	persisted, err := st.GetJob(ctx, "job-failed")
	if err != nil || persisted.LastError != diagnostic || persisted.FailureCode != string(FailureNoGroundedTitles) {
		t.Fatalf("private persisted failure = (%+v, %v)", persisted, err)
	}
}

func TestStoreWorkflowListsCallerJourneysIncludingJobsWithoutProposal(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	st, err := store.Open(ctx, "sqlite://"+filepath.Join(t.TempDir(), "list.db"), true)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	now := time.Date(2026, time.August, 22, 18, 0, 0, 0, time.UTC)
	workflow := New(st, func() string { return "unused" }, func() time.Time { return now })
	for _, job := range []store.Job{
		{ID: "alice-running", Kind: "suggest", Status: "queued", CreatedBy: "alice", IntentJSON: `{"description":"Anime after school"}`, WorkflowVersion: store.ProposalWorkflowVersion, Deadline: now, CreatedAt: now, UpdatedAt: now},
		{ID: "bob-running", Kind: "suggest", Status: "queued", CreatedBy: "bob", IntentJSON: `{"description":"Noir"}`, WorkflowVersion: store.ProposalWorkflowVersion, Deadline: now, CreatedAt: now, UpdatedAt: now},
	} {
		if err := st.CreateJob(ctx, job); err != nil {
			t.Fatal(err)
		}
	}
	journeys, err := workflow.List(ctx, Viewer{UserID: "alice"}, ListOptions{Mine: true})
	if err != nil || len(journeys) != 1 || journeys[0].JobID != "alice-running" || journeys[0].Proposal != nil {
		t.Fatalf("alice Journeys = %+v, %v", journeys, err)
	}
	all, err := workflow.List(ctx, Viewer{UserID: "admin", Admin: true}, ListOptions{})
	if err != nil || len(all) != 2 {
		t.Fatalf("admin Journeys = %+v, %v", all, err)
	}
}
