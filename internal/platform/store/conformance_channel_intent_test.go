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
	if intent := `{"description":"90s action"}`; asWritten(got.IntentJSON, intent) != intent || got.RequestedBy != "u-member" {
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
		CreatedAt: now.Add(-time.Hour), UpdatedAt: now.Add(-time.Hour),
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
	if intent := `{"description":"second"}`; got.IntentRef != "job-origin" || asWritten(got.IntentJSON, intent) != intent || got.RequestedBy != "u-member" {
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
			CreatedAt: now, UpdatedAt: now}); err != nil {
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

// A done job whose proposal is still waiting for a decision is not finished work: purging it
// would leave a refine or re-curation proposal that can no longer find its channel. Approving it
// later would then create a second channel. Once the proposal is decided the job is ordinary.
func testPurgeKeepsJobsAwaitingADecision(t *testing.T, newStore NewStoreFunc) {
	s := newStore(t)
	ctx := context.Background()
	old := time.Now().Add(-100 * 24 * time.Hour)
	for _, j := range []Job{
		{ID: "j-waiting", Kind: "suggest", Status: "done", ChannelID: "ch-1", UpdatedAt: old},
		{ID: "j-approved", Kind: "suggest", Status: "done", ChannelID: "ch-1", UpdatedAt: old},
		{ID: "j-denied", Kind: "suggest", Status: "done", ChannelID: "ch-1", UpdatedAt: old},
		{ID: "j-bare", Kind: "recurate", Status: "done", ChannelID: "ch-1", UpdatedAt: old},
	} {
		if err := s.CreateJob(ctx, j); err != nil {
			t.Fatal(err)
		}
	}
	for _, p := range []Proposal{
		{ID: "p-waiting", JobID: "j-waiting", Status: "submitted", UpdatedAt: old},
		{ID: "p-approved", JobID: "j-approved", Status: "approved", UpdatedAt: old},
		{ID: "p-denied", JobID: "j-denied", Status: "denied", UpdatedAt: old},
	} {
		if err := s.CreateProposal(ctx, p); err != nil {
			t.Fatal(err)
		}
	}

	n, err := s.PurgeFinishedJobs(ctx, time.Now().Add(-30*24*time.Hour))
	if err != nil || n != 3 {
		t.Fatalf("purged %d jobs, %v; want 3 (every job except the one awaiting a decision)", n, err)
	}
	if _, err := s.GetJob(ctx, "j-waiting"); err != nil {
		t.Errorf("a job whose proposal is still submitted was purged: %v", err)
	}
	for _, id := range []string{"j-approved", "j-denied", "j-bare"} {
		if _, err := s.GetJob(ctx, id); !errors.Is(err, ErrNotFound) {
			t.Errorf("job %s survived the purge: %v", id, err)
		}
	}
}

// Supersession is per channel, not per job: with one job per refine, an older proposal for a
// channel must not be approvable after a newer one for the same channel was approved, or its
// replace-lineup bind would drop what the newer approval added.
func testApprovalSupersessionIsPerChannel(t *testing.T, newStore NewStoreFunc) {
	s := newStore(t)
	ctx := context.Background()
	now := time.Unix(1_800_000_000, 0).UTC()
	ch := sampleChannel("ch-super", 1, time.Time{})
	ch.IntentRef = "job-origin"
	saved := mustSaveChannel(t, s, ch)
	other := sampleChannel("ch-elsewhere", 2, time.Time{})
	other.IntentRef = "job-elsewhere"
	otherSaved := mustSaveChannel(t, s, other)

	for _, j := range []Job{
		{ID: "job-refine", Kind: "suggest", Status: "done", CreatedBy: "u-admin", ChannelID: "ch-super"},
		{ID: "job-recurate", Kind: "recurate", Status: "done", ChannelID: "ch-super"},
		{ID: "job-refine-other", Kind: "suggest", Status: "done", CreatedBy: "u-admin", ChannelID: "ch-elsewhere"},
	} {
		if err := s.CreateJob(ctx, j); err != nil {
			t.Fatal(err)
		}
	}
	older := Proposal{ID: "p-refine", JobID: "job-refine", Status: "submitted", CreatedBy: "u-admin",
		CreatedAt: now.Add(-2 * time.Hour), UpdatedAt: now.Add(-2 * time.Hour)}
	newer := Proposal{ID: "p-recurate", JobID: "job-recurate", Status: "submitted",
		CreatedAt: now.Add(-time.Hour), UpdatedAt: now.Add(-time.Hour)}
	unrelated := Proposal{ID: "p-other", JobID: "job-refine-other", Status: "submitted", CreatedBy: "u-admin",
		CreatedAt: now.Add(-3 * time.Hour), UpdatedAt: now.Add(-3 * time.Hour)}
	for _, p := range []Proposal{older, newer, unrelated} {
		if err := s.CreateProposal(ctx, p); err != nil {
			t.Fatal(err)
		}
	}
	approve := func(p Proposal, ch Channel) error {
		p.Status, p.ApprovedBy, p.ApprovedAt, p.UpdatedAt = "approved", "admin", now, now
		_, err := s.CommitProposalApproval(ctx, ProposalApproval{Proposal: p, Channel: ch})
		return err
	}

	if err := approve(newer, saved); err != nil {
		t.Fatalf("approve the newer proposal: %v", err)
	}
	fresh, err := s.GetChannel(ctx, "ch-super")
	if err != nil {
		t.Fatal(err)
	}
	if err := approve(older, fresh); !errors.Is(err, ErrProposalSuperseded) {
		t.Fatalf("approve the older proposal of the same channel = %v, want ErrProposalSuperseded", err)
	}
	// A newer approval for another channel supersedes nothing here.
	if err := approve(unrelated, otherSaved); err != nil {
		t.Fatalf("approve a proposal of another channel: %v", err)
	}
}
