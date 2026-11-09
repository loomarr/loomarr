package approval_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/channels/approval"
	"github.com/loomarr/loomarr/internal/channels/schedule"
	"github.com/loomarr/loomarr/internal/platform/store"
	"github.com/loomarr/loomarr/internal/platform/store/identitystore"
	"github.com/loomarr/loomarr/internal/platform/store/proposalstore"
	"github.com/loomarr/loomarr/internal/proposals"
	"github.com/loomarr/loomarr/internal/proposals/accept"
	"github.com/loomarr/loomarr/internal/testkit"
)

// proposalFor builds an approved candidate for jobID carrying intent, made by createdBy.
func proposalFor(t *testing.T, id, jobID, createdBy string, intent proposals.Intent) proposalstore.Proposal {
	t.Helper()
	return proposalstore.Proposal{
		ID: id, JobID: jobID, Status: "approved", CreatedBy: createdBy, ApprovedBy: "admin",
		Body: proposals.Proposal{
			Intent: intent,
			Lineup: []proposals.ProposalItem{inLib(1, "Exact")},
		}, CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}
}

// A channel made by approval owns its request: the approved intent, minus the inputs that belong
// to the one run that produced it, and the person who asked.
func TestPlan_NewChannelOwnsItsIntentAndRequester(t *testing.T) {
	t.Parallel()
	st := newStore(t)
	putAvailable(t, st, 1, "Exact")
	p := proposalFor(t, "p-new", "job-new", "u-member", proposals.Intent{
		Description: "90s action", Era: "1990s", MustExclude: []string{"Slow Burn"},
		ChannelName: "Fixed Name", RefineText: "one-run note",
		CurrentLineup: []proposals.LineupContext{{Key: "movie:tmdb:90000001"}},
	})
	if err := st.CreateJob(context.Background(), proposalstore.Job{ID: "job-new", Kind: "suggest", Status: "done"}); err != nil {
		t.Fatal(err)
	}

	ch, err := approval.New(st, nil, nil, testkit.Logger()).PlanApprovedChannel(context.Background(), p)
	if err != nil {
		t.Fatal(err)
	}
	if ch.RequestedBy != "u-member" {
		t.Errorf("RequestedBy = %q, want the requester u-member", ch.RequestedBy)
	}
	var owned proposals.Intent
	if err := json.Unmarshal([]byte(ch.IntentJSON), &owned); err != nil {
		t.Fatalf("channel intent %q: %v", ch.IntentJSON, err)
	}
	if owned.Description != "90s action" || owned.Era != "1990s" || len(owned.MustExclude) != 1 {
		t.Errorf("channel lost the request: %+v", owned)
	}
	if owned.RefineText != "" || len(owned.CurrentLineup) != 0 || owned.ChannelName != "" {
		t.Errorf("channel kept one-run inputs: %+v", owned)
	}
}

// A refine or re-curation job names its channel, so approving it patches that channel in place:
// the channel keeps its origin binding and its requester, whoever ran the refine.
func TestPlan_JobNamingAChannelPatchesItWithoutMovingTheRequest(t *testing.T) {
	t.Parallel()
	st := newStore(t)
	ctx := context.Background()
	putAvailable(t, st, 1, "Exact")
	seedChannel(t, st, "c1", "job-origin", []schedule.LineupEntry{entry(2, "Old")}, schedule.ChannelPolicy{})
	saved, err := st.GetChannel(ctx, "c1")
	if err != nil {
		t.Fatal(err)
	}
	if err := st.UpsertUser(ctx, identitystore.User{ID: "u-member", Name: "Member", Role: identitystore.RoleMember}); err != nil {
		t.Fatal(err)
	}
	saved.RequestedBy = "u-member"
	saved.IntentJSON = `{"description":"90s action"}`
	if _, err := st.SaveChannel(ctx, saved); err != nil {
		t.Fatal(err)
	}
	if err := st.CreateJob(ctx, proposalstore.Job{
		ID: "job-refine", Kind: "suggest", Status: "done", CreatedBy: "u-admin", ChannelID: "c1",
	}); err != nil {
		t.Fatal(err)
	}

	p := proposalFor(t, "p-refine", "job-refine", "u-admin", proposals.Intent{
		Description: "90s action", RefineText: "more heists",
	})
	ch, err := approval.New(st, nil, nil, testkit.Logger()).PlanApprovedChannel(ctx, p)
	if err != nil {
		t.Fatal(err)
	}
	if ch.ID != "c1" || ch.IntentRef != "job-origin" {
		t.Errorf("planned channel = id %q intentRef %q, want c1 still bound to job-origin", ch.ID, ch.IntentRef)
	}
	if ch.RequestedBy != "u-member" {
		t.Errorf("RequestedBy = %q, want the original requester u-member (not the admin who refined)", ch.RequestedBy)
	}
	if ch.IntentJSON != `{"description":"90s action"}` {
		t.Errorf("channel intent = %q, want the base intent without the refine note", ch.IntentJSON)
	}
	if got := lineupKeys(ch); !got[movieKey(1)] {
		t.Errorf("refined lineup was not applied: %+v", ch.Lineup)
	}
}

// A job whose channel was removed is an error: approving it must not create a second channel.
// The job goes with its channel (proposal_jobs.channel_id is ON DELETE CASCADE, #2793), so its
// suggestion is left with no job, which approval refuses.
func TestPlan_JobNamingARemovedChannelFails(t *testing.T) {
	t.Parallel()
	st := newStore(t)
	ctx := context.Background()
	putAvailable(t, st, 1, "Exact")
	seedChannel(t, st, "c-removed", "job-origin", []schedule.LineupEntry{entry(2, "Old")}, schedule.ChannelPolicy{})
	if err := st.CreateJob(ctx, proposalstore.Job{
		ID: "job-orphan", Kind: "recurate", Status: "done", ChannelID: "c-removed",
	}); err != nil {
		t.Fatal(err)
	}
	p := proposalFor(t, "p-orphan", "job-orphan", "", proposals.Intent{Description: "90s action"})
	removed, err := st.GetChannel(ctx, "c-removed")
	if err != nil {
		t.Fatal(err)
	}
	if err := st.DeleteChannel(ctx, removed.ID, removed.Revision); err != nil {
		t.Fatal(err)
	}

	_, err = approval.New(st, nil, nil, testkit.Logger()).PlanApprovedChannel(ctx, p)
	if !errors.Is(err, accept.ErrProposalJobGone) {
		t.Fatalf("plan for a removed channel = %v, want ErrProposalJobGone", err)
	}
	if chans, err := st.ListChannels(ctx); err != nil || len(chans) != 0 {
		t.Errorf("channels after refusing = %d, %v; want none", len(chans), err)
	}
}

// A first approval always has its job. A proposal whose job row is gone (purged, or never
// existed) must not become a channel: it may be a refine or re-curation that lost the link to its
// channel, and creating one would duplicate that channel.
func TestPlan_RefusesToCreateAChannelForAProposalWithNoJob(t *testing.T) {
	t.Parallel()
	st := newStore(t)
	putAvailable(t, st, 1, "Exact")
	p := proposalFor(t, "p-nojob", "job-missing", "u-member", proposals.Intent{Description: "90s action"})

	ch, err := approval.New(st, nil, nil, testkit.Logger()).PlanApprovedChannel(context.Background(), p)
	if err == nil {
		t.Fatalf("planned a channel %q for a proposal whose job does not exist", ch.ID)
	}
	if !errors.Is(err, accept.ErrProposalJobGone) || errors.Is(err, store.ErrNotFound) {
		t.Errorf("error %v must be ErrProposalJobGone, not 'no channel yet' (which replans)", err)
	}
}
