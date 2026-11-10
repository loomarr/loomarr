package workflow

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/channels/schedule"
	"github.com/loomarr/loomarr/internal/library/provision"
	"github.com/loomarr/loomarr/internal/platform/store"
	"github.com/loomarr/loomarr/internal/platform/store/channelstore"
	"github.com/loomarr/loomarr/internal/platform/store/proposalstore"
	"github.com/loomarr/loomarr/internal/proposals"
	"github.com/loomarr/loomarr/internal/testkit"
)

// testIntentHash keys the generation cache by the Intent's exact JSON: equal Intents collide and
// different ones do not, which is all these tests need from the suggester's normalizing hash.
func testIntentHash(intent proposals.Intent) string {
	blob, _ := json.Marshal(intent)
	sum := sha256.Sum256(blob)
	return hex.EncodeToString(sum[:])
}

// openStore opens a migrated store with the people these tests name saved as members.
func openStore(t *testing.T) store.Store {
	t.Helper()
	st, err := store.Open(context.Background(), testkit.MigratedSQLiteDSN(t), true)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	testkit.SaveMembers(t, st, "alice", "bob", "admin", "member-1")
	return st
}

func TestStoreWorkflowSubmissionCacheKeepsCallerLifecycleFresh(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	st := openStore(t)
	now := time.Date(2026, time.August, 22, 15, 0, 0, 0, time.UTC)
	var next atomic.Int64
	workflow := New(st, func() string { return "workflow-" + strconv.FormatInt(next.Add(1), 10) }, func() time.Time { return now }, testIntentHash)
	intent := proposals.Intent{Description: "Saturday morning cartoons"}
	first, err := workflow.Submit(ctx, intent, "alice", now.Add(-time.Hour))
	if err != nil || first.JobID == "" || first.CachedProposal != nil {
		t.Fatalf("first Submit = %+v, %v", first, err)
	}
	work, err := workflow.Claim(ctx, now, time.Minute, 1)
	if err != nil || len(work) != 1 {
		t.Fatalf("Claim = %+v, %v", work, err)
	}
	grounded := proposals.Proposal{Intent: intent, Lineup: []proposals.ProposalItem{{
		MediaType: provision.Movie, TMDBID: 90000603, Name: "Marigold Protocol", InLibrary: true,
	}}, Trace: selectedDecisionTrace("movie:tmdb:90000603", "Marigold Protocol", "library")}
	if _, err := workflow.Complete(ctx, work[0], grounded); err != nil {
		t.Fatal(err)
	}

	second, err := workflow.Submit(ctx, intent, "bob", now.Add(-time.Hour))
	if err != nil || second.JobID == first.JobID || second.CachedProposal == nil {
		t.Fatalf("cached Submit = %+v, %v", second, err)
	}
	journey, err := workflow.Inspect(ctx, Viewer{UserID: "bob"}, second.JobID)
	if err != nil || journey.Milestone != MilestoneAwaitingApproval || journey.Proposal == nil {
		t.Fatalf("bob cached Journey = %+v, %v", journey, err)
	}
	if _, err := workflow.Inspect(ctx, Viewer{UserID: "alice"}, second.JobID); !errors.Is(err, ErrForbidden) {
		t.Fatalf("cross-owner cached Journey = %v, want ErrForbidden", err)
	}

	refined := intent
	refined.RefineText = "more action"
	if err := workflow.Revise(ctx, Viewer{UserID: "alice"}, second.JobID, refined); !errors.Is(err, ErrForbidden) {
		t.Fatalf("cross-owner revision = %v, want ErrForbidden", err)
	}
	if err := workflow.Revise(ctx, Viewer{UserID: "bob"}, second.JobID, refined); err != nil {
		t.Fatal(err)
	}
	requeued, err := workflow.Inspect(ctx, Viewer{UserID: "bob"}, second.JobID)
	if err != nil || requeued.Milestone != MilestoneGenerating || requeued.Intent.RefineText != "more action" ||
		requeued.Proposal == nil || requeued.Proposal.ID != second.CachedProposal.ID {
		t.Fatalf("requeued Journey = %+v, %v", requeued, err)
	}
	if err := workflow.Revise(ctx, Viewer{UserID: "bob"}, second.JobID, refined); !errors.Is(err, ErrNotRevisable) {
		t.Fatalf("concurrent revision = %v, want ErrNotRevisable", err)
	}
	revision, err := workflow.Claim(ctx, now, time.Minute, 1)
	if err != nil || len(revision) != 1 || revision[0].JobID != second.JobID {
		t.Fatalf("claim revision = %+v, %v", revision, err)
	}
	if err := workflow.Fail(ctx, revision[0], string(FailureGenerationFailed), "provider unavailable"); err != nil {
		t.Fatal(err)
	}
	restored, err := workflow.Inspect(ctx, Viewer{UserID: "bob", Admin: true}, second.JobID)
	if err != nil || restored.Milestone != MilestoneAwaitingApproval || restored.Proposal == nil ||
		restored.Proposal.ID != second.CachedProposal.ID || restored.Failure == nil ||
		len(restored.Actions) != 3 || restored.Actions[0] != ActionReview {
		t.Fatalf("failed revision Journey = %+v, %v", restored, err)
	}
}

// A library idea's Proposal needs no generation (#1720). Recorded, it must read exactly like a
// generated one: the requester's Journey is awaiting approval, the claimable queue stays empty
// (nothing re-runs it through the model), and an ungrounded item is refused before any write.
func TestStoreWorkflowRecordsAGroundedProposalAwaitingApproval(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	st := openStore(t)
	now := time.Date(2026, time.September, 28, 18, 0, 0, 0, time.UTC)
	var next atomic.Int64
	workflow := New(st, func() string { return "record-" + strconv.FormatInt(next.Add(1), 10) }, func() time.Time { return now }, testIntentHash)

	intent := proposals.Intent{Description: "Every comedy title in your library that no channel plays yet, on one channel."}
	prop := proposals.Proposal{ChannelName: "Comedy Movies", FromIdea: "genre:comedy", Lineup: []proposals.ProposalItem{
		{MediaType: provision.Movie, TMDBID: 90000001, Name: "Comedy One", Year: 1994, InLibrary: true},
		{MediaType: provision.Series, TVDBID: 90000002, Name: "Sitcom Two", Year: 1996, InLibrary: true},
	}}
	recorded, err := workflow.Record(ctx, intent, prop, "member-1")
	if err != nil {
		t.Fatal(err)
	}
	if recorded.JobID == "" || recorded.ID == "" || recorded.Proposal.FromIdea != "genre:comedy" ||
		recorded.Proposal.Intent.Description != intent.Description {
		t.Fatalf("Record = %+v", recorded)
	}
	journey, err := workflow.Inspect(ctx, Viewer{UserID: "member-1"}, recorded.JobID)
	if err != nil || journey.Milestone != MilestoneAwaitingApproval || journey.Proposal == nil ||
		journey.Proposal.ID != recorded.ID || journey.Proposal.Status != ProposalSubmitted {
		t.Fatalf("recorded Journey = %+v, %v", journey, err)
	}
	if work, err := workflow.Claim(ctx, now.Add(time.Hour), time.Minute, 5); err != nil || len(work) != 0 {
		t.Fatalf("a recorded Proposal became claimable model work: %+v, %v", work, err)
	}

	ungrounded := proposals.Proposal{Lineup: []proposals.ProposalItem{{MediaType: provision.Movie, Name: "No Identity"}}}
	if _, err := workflow.Record(ctx, intent, ungrounded, "member-1"); !errors.Is(err, ErrInvalidState) {
		t.Fatalf("ungrounded Record = %v, want ErrInvalidState", err)
	}
	if _, err := workflow.Record(ctx, intent, proposals.Proposal{}, "member-1"); !errors.Is(err, ErrInvalidState) {
		t.Fatalf("empty Record = %v, want ErrInvalidState", err)
	}
	queue, err := st.ListProposalsByStatus(ctx, "submitted")
	if err != nil || len(queue) != 1 {
		t.Fatalf("refused records reached the queue: %+v, %v", queue, err)
	}
}

func TestStoreWorkflowRecoversCrashAndRejectsLateAttemptResult(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	st := openStore(t)

	started := time.Date(2026, time.August, 22, 16, 0, 0, 0, time.UTC)
	clock := started
	workflow := New(st, func() string { return "proposal-1" }, func() time.Time { return clock }, testIntentHash)
	intent := proposals.Intent{Description: "Saturday morning cartoons"}
	if err := st.CreateJob(ctx, proposalstore.Job{
		ID: "job-1", Kind: "suggest", Status: "queued", CreatedBy: "member-1",
		IntentJSON:      `{"description":"Saturday morning cartoons"}`,
		WorkflowVersion: proposalstore.ProposalWorkflowVersion,
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

	prop := proposals.Proposal{Intent: intent, Lineup: []proposals.ProposalItem{{
		MediaType: provision.Movie, TMDBID: 90000603, Name: "Marigold Protocol", InLibrary: true,
	}}, Trace: selectedDecisionTrace("movie:tmdb:90000603", "Marigold Protocol", "library")}
	if _, err := workflow.Complete(ctx, first[0], prop); !errors.Is(err, ErrStaleAttempt) {
		t.Fatalf("late Attempt completion = %v, want ErrStaleAttempt", err)
	}
	if _, err := st.GetProposal(ctx, "proposal-1"); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("late Attempt published Proposal: %v", err)
	}
	if _, err := workflow.Complete(ctx, second[0], prop); err != nil {
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
	st := openStore(t)
	now := time.Date(2026, time.August, 22, 17, 0, 0, 0, time.UTC)
	workflow := New(st, func() string { return "unused" }, func() time.Time { return now }, testIntentHash)
	if err := st.CreateJob(ctx, proposalstore.Job{
		ID: "job-failed", Kind: "suggest", Status: "queued", CreatedBy: "member-1",
		IntentJSON:      `{"description":"Obscure regional monster movies"}`,
		WorkflowVersion: proposalstore.ProposalWorkflowVersion,
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
	persisted.FailureTraceJSON = `{"version":1,"terminal":"private-provider-detail"}`
	if err := st.UpdateJob(ctx, persisted); err != nil {
		t.Fatal(err)
	}
	if _, err := workflow.Inspect(ctx, Viewer{UserID: "member-1"}, "job-failed"); err == nil || !strings.Contains(err.Error(), "validate Proposal Job job-failed failure trace") {
		t.Fatalf("invalid persisted failure trace did not fail closed: %v", err)
	}
}

func TestStoreWorkflowListsCallerJourneysIncludingJobsWithoutProposal(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	st := openStore(t)
	now := time.Date(2026, time.August, 22, 18, 0, 0, 0, time.UTC)
	workflow := New(st, func() string { return "unused" }, func() time.Time { return now }, testIntentHash)
	for _, job := range []proposalstore.Job{
		{ID: "alice-running", Kind: "suggest", Status: "queued", CreatedBy: "alice", IntentJSON: `{"description":"Anime after school"}`, WorkflowVersion: proposalstore.ProposalWorkflowVersion, Deadline: now, CreatedAt: now, UpdatedAt: now},
		{ID: "bob-running", Kind: "suggest", Status: "queued", CreatedBy: "bob", IntentJSON: `{"description":"Noir"}`, WorkflowVersion: proposalstore.ProposalWorkflowVersion, Deadline: now, CreatedAt: now, UpdatedAt: now},
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

// #1413 origin: approval commits the Proposal and its intent-bound Channel in one
// transaction, so the only way an approved Proposal loses its Channel is a later
// hard delete (DELETE /v1/channels/{id}?purge=true). One such orphan must not
// fail the whole list, and the healthy neighbour must still be reported.
// A Job whose stored Intent cannot be decoded is failed on its own; it never aborts the claim, so
// the healthy Jobs claimed in the same batch still run instead of sitting leased until expiry.
func TestStoreWorkflowClaimFailsPoisonJobAndReturnsTheRest(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	st := openStore(t)
	now := time.Date(2026, time.November, 3, 9, 0, 0, 0, time.UTC)
	workflow := New(st, func() string { return "unused" }, func() time.Time { return now }, testIntentHash)
	for _, job := range []proposalstore.Job{
		{ID: "poison", IntentJSON: "[1]"}, // JSON, as the column demands, but not an Intent
		{ID: "good", IntentJSON: `{"description":"Noir"}`},
	} {
		job.Kind, job.Status, job.CreatedBy, job.WorkflowVersion = "suggest", "queued", "alice", proposalstore.ProposalWorkflowVersion
		job.Deadline, job.CreatedAt, job.UpdatedAt = now, now, now
		if err := st.CreateJob(ctx, job); err != nil {
			t.Fatal(err)
		}
	}

	works, err := workflow.Claim(ctx, now, time.Minute, 10)
	if err != nil {
		t.Fatalf("Claim = %v, want the healthy Job despite the poison one", err)
	}
	if len(works) != 1 || works[0].JobID != "good" || works[0].Intent.Description != "Noir" {
		t.Fatalf("Claim returned %+v, want only the good Job", works)
	}
	poison, err := st.GetJob(ctx, "poison")
	if err != nil {
		t.Fatal(err)
	}
	if poison.Status != "failed" || poison.FailureCode != string(FailureGenerationFailed) ||
		!strings.Contains(poison.LastError, "Intent") {
		t.Fatalf("poison Job = status %q code %q last_error %q, want failed with an Intent decode explanation",
			poison.Status, poison.FailureCode, poison.LastError)
	}
	// Failed is terminal: the next claim never hands it out again.
	if again, err := workflow.Claim(ctx, now.Add(time.Hour), time.Minute, 10); err != nil || len(again) != 1 || again[0].JobID != "good" {
		t.Fatalf("second Claim = %+v, %v, want the expired good lease only", again, err)
	}
}

func TestStoreWorkflowListSurvivesApprovedProposalWhoseChannelWasPurged(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	st := openStore(t)
	now := time.Date(2026, time.September, 22, 3, 0, 0, 0, time.UTC)
	workflow := New(st, func() string { return "unused" }, func() time.Time { return now }, testIntentHash)

	approve := func(jobID string, number int) channelstore.Channel {
		t.Helper()
		if err := st.CreateJob(ctx, proposalstore.Job{
			ID: jobID, Kind: "suggest", Status: "queued", CreatedBy: "alice",
			IntentJSON: `{"description":"Noir"}`, WorkflowVersion: proposalstore.ProposalWorkflowVersion,
			Deadline: now, CreatedAt: now, UpdatedAt: now,
		}); err != nil {
			t.Fatal(err)
		}
		if claimed, err := st.ClaimDueJobs(ctx, now, time.Minute, 10); err != nil || len(claimed) == 0 {
			t.Fatalf("claim %s = %+v, %v", jobID, claimed, err)
		}
		prop := proposalstore.Proposal{
			ID: "proposal-" + jobID, JobID: jobID, Status: "submitted", CreatedBy: "alice",
			Body: proposals.Proposal{Lineup: []proposals.ProposalItem{{Name: "Noir"}}}, CreatedAt: now, UpdatedAt: now,
		}
		if err := st.CommitSuggestionSuccess(ctx, jobID, 1, prop, now); err != nil {
			t.Fatal(err)
		}
		prop.Status, prop.ApprovedBy, prop.ApprovedAt = "approved", "admin", now
		ch := channelstore.Channel{}
		ch.ID, ch.IntentRef, ch.FromRequest, ch.Name, ch.Number = "ch-"+jobID, jobID, true, "Channel "+jobID, number
		ch.Strategy, ch.Status, ch.ReconcileDeadline = schedule.Sequential, schedule.StatusBuilding, now
		if _, err := st.CommitProposalApproval(ctx, store.ProposalApproval{Proposal: prop, Channel: ch}); err != nil {
			t.Fatal(err)
		}
		created, err := st.GetChannel(ctx, ch.ID)
		if err != nil {
			t.Fatal(err)
		}
		return created
	}
	approve("job-healthy", 101)
	orphaned := approve("job-orphaned", 102)
	if err := st.DeleteChannel(ctx, orphaned.ID, orphaned.Revision); err != nil {
		t.Fatal(err)
	}

	journeys, err := workflow.List(ctx, Viewer{UserID: "admin", Admin: true}, ListOptions{})
	if err != nil {
		t.Fatalf("List with one orphaned approved Proposal: %v", err)
	}
	byJob := map[string]Journey{}
	for _, journey := range journeys {
		byJob[journey.JobID] = journey
	}
	if len(byJob) != 2 {
		t.Fatalf("Journeys = %+v, want both jobs reported", journeys)
	}
	if got := byJob["job-healthy"]; got.Milestone != MilestoneBuilding || got.Channel == nil {
		t.Fatalf("healthy Journey = %+v, want building with its Channel", got)
	}
	if got := byJob["job-orphaned"]; got.Milestone != MilestoneFailed || got.Failure == nil || got.Channel != nil {
		t.Fatalf("orphaned Journey = %+v, want an explained failed state", got)
	}
}

// A channel job is a fresh, queued lifecycle that names its channel. It never clones a cached
// proposal for an identical intent (a refine always regenerates), and it never touches the job a
// cached copy came from.
func TestStoreWorkflowSubmitForChannelIsFreshAndNeverCached(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	st := openStore(t)
	now := time.Date(2026, time.August, 22, 15, 0, 0, 0, time.UTC)
	var next atomic.Int64
	workflow := New(st, func() string { return "workflow-" + strconv.FormatInt(next.Add(1), 10) }, func() time.Time { return now }, testIntentHash)
	intent := proposals.Intent{Description: "Saturday morning cartoons"}
	first, err := workflow.Submit(ctx, intent, "alice", now.Add(-time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	work, err := workflow.Claim(ctx, now, time.Minute, 1)
	if err != nil || len(work) != 1 {
		t.Fatalf("Claim = %+v, %v", work, err)
	}
	grounded := proposals.Proposal{Intent: intent, Lineup: []proposals.ProposalItem{{
		MediaType: provision.Movie, TMDBID: 90000603, Name: "Marigold Protocol", InLibrary: true,
	}}, Trace: selectedDecisionTrace("movie:tmdb:90000603", "Marigold Protocol", "library")}
	if _, err := workflow.Complete(ctx, work[0], grounded); err != nil {
		t.Fatal(err)
	}

	// The job names its channel, a foreign key (#2793).
	ch := channelstore.Channel{}
	ch.ID, ch.Name, ch.Number, ch.FromRequest = "ch-1", "Cartoons", 1, true
	ch.Strategy, ch.Status = schedule.Sequential, schedule.StatusLive
	if _, err := st.SaveChannel(ctx, ch); err != nil {
		t.Fatal(err)
	}
	jobID, err := workflow.SubmitForChannel(ctx, "ch-1", intent, "suggest", "admin")
	if err != nil || jobID == first.JobID {
		t.Fatalf("SubmitForChannel = %q, %v; want a new job", jobID, err)
	}
	job, err := st.GetJob(ctx, jobID)
	if err != nil {
		t.Fatal(err)
	}
	if job.Status != "queued" || job.ChannelID != "ch-1" || job.CreatedBy != "admin" || job.Kind != "suggest" {
		t.Errorf("channel job = status %q channel %q owner %q kind %q", job.Status, job.ChannelID, job.CreatedBy, job.Kind)
	}
	if source, err := st.GetJob(ctx, first.JobID); err != nil || source.Status != "done" || source.ChannelID != "" {
		t.Errorf("the job a cached copy came from changed: %+v, %v", source, err)
	}

	if _, err := workflow.SubmitForChannel(ctx, "", intent, "suggest", "admin"); !errors.Is(err, ErrInvalidState) {
		t.Errorf("channel job without a channel = %v, want ErrInvalidState", err)
	}
	if _, err := workflow.SubmitForChannel(ctx, "ch-1", intent, "bogus", "admin"); !errors.Is(err, ErrInvalidState) {
		t.Errorf("channel job with an unknown kind = %v, want ErrInvalidState", err)
	}
}
