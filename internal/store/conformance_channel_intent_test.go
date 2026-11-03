package store

import (
	"context"
	"errors"
	"testing"
	"time"
)

// A channel owns the request it was made from. Housekeeping purges finished jobs older than
// `jobs.retention`, and approval does not touch a job, so a channel that is live for longer than
// the window used to lose its intent (no refine, no re-curation) and its requester's name. This
// is the proven scenario: a done job 31 days old, a channel made from it, a purge.
func testChannelOutlivesItsPurgedJob(t *testing.T, newStore NewStoreFunc) {
	s := newStore(t)
	ctx := context.Background()
	if err := s.UpsertUser(ctx, User{ID: "u-member", Name: "Member One", Role: RoleMember}); err != nil {
		t.Fatal(err)
	}
	old := time.Now().Add(-31 * 24 * time.Hour)
	if err := s.CreateJob(ctx, Job{
		ID: "job-origin", Kind: "suggest", Status: "done", CreatedBy: "u-member",
		IntentJSON: `{"description":"90s action"}`, UpdatedAt: old,
	}); err != nil {
		t.Fatal(err)
	}
	ch := sampleChannel("ch-owner", 1, time.Time{})
	ch.IntentRef = "job-origin"
	ch.IntentJSON = `{"description":"90s action"}`
	ch.RequestedBy = "u-member"
	saved := mustSaveChannel(t, s, ch)

	purged, err := s.PurgeFinishedJobs(ctx, time.Now().Add(-30*24*time.Hour))
	if err != nil || purged != 1 {
		t.Fatalf("purge = %d, %v; want the 31-day-old done job gone", purged, err)
	}
	if _, err := s.GetJob(ctx, "job-origin"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("origin job survived the purge: %v", err)
	}

	got, err := s.GetChannel(ctx, "ch-owner")
	if err != nil {
		t.Fatal(err)
	}
	if got.IntentJSON != `{"description":"90s action"}` || got.RequestedBy != "u-member" {
		t.Errorf("channel lost its request: intent=%q requestedBy=%q", got.IntentJSON, got.RequestedBy)
	}
	requesters, err := s.ChannelRequesters(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if requesters["ch-owner"] != "Member One" {
		t.Errorf("requester after purge = %q, want Member One", requesters["ch-owner"])
	}

	// A refine job names the channel, so its proposal still resolves to it, and the channel's
	// origin binding is untouched by a later save.
	if err := s.CreateJob(ctx, Job{
		ID: "job-refine", Kind: "suggest", Status: "done", CreatedBy: "u-admin", ChannelID: "ch-owner",
	}); err != nil {
		t.Fatal(err)
	}
	forRefine, err := s.GetChannelForJob(ctx, "job-refine")
	if err != nil || forRefine.ID != saved.ID {
		t.Fatalf("GetChannelForJob(refine) = %+v, %v; want ch-owner", forRefine.ID, err)
	}
	forOrigin, err := s.GetChannelForJob(ctx, "job-origin")
	if err != nil || forOrigin.ID != saved.ID {
		t.Fatalf("GetChannelForJob(purged origin) = %+v, %v; want ch-owner", forOrigin.ID, err)
	}
	if _, err := s.GetChannelForJob(ctx, "job-unbound"); !errors.Is(err, ErrNotFound) {
		t.Errorf("GetChannelForJob(unbound) = %v, want ErrNotFound", err)
	}
}

// A refine or re-curation job approves onto the channel it names without moving the channel's
// origin binding, and never onto another channel or a channel that was removed.
func testApprovalOfJobNamingAChannel(t *testing.T, newStore NewStoreFunc) {
	s := newStore(t)
	ctx := context.Background()
	now := time.Unix(1_800_000_000, 0).UTC()
	ch := sampleChannel("ch-named", 1, time.Time{})
	ch.IntentRef = "job-origin"
	ch.IntentJSON = `{"description":"first"}`
	ch.RequestedBy = "u-member"
	saved := mustSaveChannel(t, s, ch)
	other := mustSaveChannel(t, s, sampleChannel("ch-other", 2, time.Time{}))

	if err := s.CreateJob(ctx, Job{
		ID: "job-refine", Kind: "suggest", Status: "done", CreatedBy: "u-admin", ChannelID: "ch-named",
	}); err != nil {
		t.Fatal(err)
	}
	proposal := Proposal{
		ID: "p-refine", JobID: "job-refine", Status: "submitted", CreatedBy: "u-admin",
		ProposalJSON: `{"lineup":[]}`, CreatedAt: now.Add(-time.Hour), UpdatedAt: now.Add(-time.Hour),
	}
	if err := s.CreateProposal(ctx, proposal); err != nil {
		t.Fatal(err)
	}
	proposal.Status, proposal.ApprovedBy, proposal.ApprovedAt, proposal.UpdatedAt = "approved", "admin", now, now

	if _, err := s.CommitProposalApproval(ctx, ProposalApproval{Proposal: proposal, Channel: other}); err == nil {
		t.Fatal("approving a refine job onto a channel it does not name succeeded")
	}

	next := saved
	next.IntentJSON = `{"description":"second"}`
	if _, err := s.CommitProposalApproval(ctx, ProposalApproval{Proposal: proposal, Channel: next}); err != nil {
		t.Fatalf("approve refine job onto its channel: %v", err)
	}
	got, err := s.GetChannel(ctx, "ch-named")
	if err != nil {
		t.Fatal(err)
	}
	if got.IntentRef != "job-origin" || got.IntentJSON != `{"description":"second"}` || got.RequestedBy != "u-member" {
		t.Errorf("channel after refine approval = ref %q intent %q requestedBy %q", got.IntentRef, got.IntentJSON, got.RequestedBy)
	}

	if err := s.CreateJob(ctx, Job{
		ID: "job-orphan", Kind: "recurate", Status: "done", ChannelID: "ch-removed",
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.GetChannelForJob(ctx, "job-orphan"); !errors.Is(err, ErrJobChannelGone) || errors.Is(err, ErrNotFound) {
		t.Errorf("GetChannelForJob(removed channel) = %v, want ErrJobChannelGone and not ErrNotFound", err)
	}
}

// A channel has an unfinished job while one of its refine or re-curation jobs is queued, running,
// or done with a proposal still waiting for a decision. A failed job, a decided proposal, another
// channel's job, and the job the channel was made from are not unfinished work for it.
func testChannelHasUnfinishedJob(t *testing.T, newStore NewStoreFunc) {
	s := newStore(t)
	ctx := context.Background()
	now := time.Unix(1_800_000_000, 0).UTC()
	idle := func(channelID string) {
		t.Helper()
		got, err := s.ChannelHasUnfinishedJob(ctx, channelID)
		if err != nil || got {
			t.Fatalf("ChannelHasUnfinishedJob(%s) = %v, %v; want false", channelID, got, err)
		}
	}
	busy := func(channelID string) {
		t.Helper()
		got, err := s.ChannelHasUnfinishedJob(ctx, channelID)
		if err != nil || !got {
			t.Fatalf("ChannelHasUnfinishedJob(%s) = %v, %v; want true", channelID, got, err)
		}
	}
	job := func(id, kind, status, channelID string) {
		t.Helper()
		if err := s.CreateJob(ctx, Job{ID: id, Kind: kind, Status: status, ChannelID: channelID,
			CreatedAt: now, UpdatedAt: now}); err != nil {
			t.Fatal(err)
		}
	}
	proposal := func(id, jobID, status string) {
		t.Helper()
		if err := s.CreateProposal(ctx, Proposal{ID: id, JobID: jobID, Status: status,
			ProposalJSON: `{}`, CreatedAt: now, UpdatedAt: now}); err != nil {
			t.Fatal(err)
		}
	}

	idle("ch-a")
	job("origin", "suggest", "queued", "") // the request that made a channel names no channel
	idle("ch-a")
	job("failed", "suggest", "failed", "ch-a")
	idle("ch-a")
	job("other", "suggest", "queued", "ch-b")
	idle("ch-a")
	job("decided", "recurate", "done", "ch-a")
	proposal("p-decided", "decided", "approved")
	idle("ch-a")

	job("waiting", "suggest", "done", "ch-a")
	proposal("p-waiting", "waiting", "submitted")
	busy("ch-a")
	idle("ch-c")
	job("queued", "recurate", "queued", "ch-c")
	busy("ch-c")
	job("running", "suggest", "running", "ch-d")
	busy("ch-d")
}
