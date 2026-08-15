package suggest_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/mantonx/loomarr/internal/store"
	"github.com/mantonx/loomarr/internal/suggest"
	"github.com/mantonx/loomarr/internal/testkit"
)

func TestApproverReplansAfterChannelUniquenessRace(t *testing.T) {
	st := &testkit.ApprovalStore{CommitErrors: []error{store.ErrChannelConflict, nil}}
	channels := &testkit.ApprovalChannels{PlanFunc: func(p store.Proposal, attempt int) store.Channel {
		ch := store.Channel{}
		ch.ID, ch.IntentRef, ch.Number = "ch-approved", p.JobID, attempt
		return ch
	}}
	approver := suggest.NewApprover(st, channels, time.Now)
	p := store.Proposal{ID: "p1", JobID: "job1", Status: "submitted", ProposalJSON: `{}`}

	result, err := approver.Approve(context.Background(), p, nil, "admin")
	if err != nil {
		t.Fatal(err)
	}
	if len(channels.Plans) != 2 || len(st.Commits) != 2 {
		t.Fatalf("plans/commits = %d/%d, want 2/2", len(channels.Plans), len(st.Commits))
	}
	if len(channels.Committed) != 1 || channels.Committed[0].Number != 2 {
		t.Fatalf("post-commit = %+v, want only the replanned channel", channels.Committed)
	}
	if result.ChannelID != "ch-approved" {
		t.Errorf("channel id = %q", result.ChannelID)
	}
}

func TestApproverOwnsPlanCommitPostCommitOrdering(t *testing.T) {
	st := &testkit.ApprovalStore{Enqueued: 1}
	channels := &testkit.ApprovalChannels{PlanFunc: func(p store.Proposal, _ int) store.Channel {
		ch := store.Channel{}
		ch.ID, ch.IntentRef = "ch-approved", p.JobID
		return ch
	}}
	decisionAt := time.Unix(1_800_000_000, 0).UTC()
	approver := suggest.NewApprover(st, channels, func() time.Time { return decisionAt })
	p := store.Proposal{
		ID: "p1", JobID: "job1", Status: "submitted",
		ProposalJSON: `{"acquisitions":[{"mediaType":"movie","tmdbId":100,"name":"Speed"}]}`,
	}

	result, err := approver.Approve(context.Background(), p, nil, "admin")
	if err != nil {
		t.Fatal(err)
	}
	if result.Enqueued != 1 || result.ChannelID != "ch-approved" {
		t.Fatalf("result = %+v", result)
	}
	if len(channels.Plans) != 1 || channels.Plans[0].Status != "approved" || channels.Plans[0].ApprovedBy != "admin" {
		t.Fatalf("planner did not receive the exact effective proposal: %+v", channels.Plans)
	}
	if !channels.Plans[0].ApprovedAt.Equal(decisionAt) {
		t.Errorf("planned approvedAt = %v, want %v", channels.Plans[0].ApprovedAt, decisionAt)
	}
	if len(st.Commits) != 1 || st.Commits[0].Channel.ID != "ch-approved" {
		t.Fatalf("atomic commit = %+v", st.Commits)
	}
	if len(channels.Committed) != 1 || channels.Committed[0].ID != "ch-approved" {
		t.Fatalf("post-commit calls = %+v", channels.Committed)
	}
}

func TestApproverPlanFailureDoesNotReachStore(t *testing.T) {
	st := &testkit.ApprovalStore{}
	channels := &testkit.ApprovalChannels{PlanError: errors.New("cannot plan")}
	approver := suggest.NewApprover(st, channels, time.Now)
	p := store.Proposal{ID: "p1", JobID: "job1", Status: "submitted", ProposalJSON: `{}`}

	if _, err := approver.Approve(context.Background(), p, nil, "admin"); err == nil {
		t.Fatal("approval succeeded despite channel-plan failure")
	}
	if len(st.Commits) != 0 || len(channels.Committed) != 0 {
		t.Fatalf("plan failure reached commit/post-commit: commits=%d after=%d", len(st.Commits), len(channels.Committed))
	}
}

func TestApproverStoreFailureDoesNotRunPostCommit(t *testing.T) {
	st := &testkit.ApprovalStore{CommitError: errors.New("local transaction failed")}
	channels := &testkit.ApprovalChannels{}
	approver := suggest.NewApprover(st, channels, time.Now)
	p := store.Proposal{ID: "p1", JobID: "job1", Status: "submitted", ProposalJSON: `{}`}

	if _, err := approver.Approve(context.Background(), p, nil, "admin"); err == nil {
		t.Fatal("approval succeeded despite local transaction failure")
	}
	if len(st.Commits) != 1 {
		t.Fatalf("commit attempts = %d, want 1", len(st.Commits))
	}
	if len(channels.Committed) != 0 {
		t.Fatalf("post-commit ran after failed transaction: %+v", channels.Committed)
	}
}
