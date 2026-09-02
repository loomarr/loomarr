package suggest_test

import (
	"context"
	"errors"
	"log/slog"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/quality"
	"github.com/loomarr/loomarr/internal/store"
	"github.com/loomarr/loomarr/internal/suggest"
	"github.com/loomarr/loomarr/internal/testkit"
)

func TestApproverReplansAfterChannelWriteRace(t *testing.T) {
	for _, conflict := range []error{store.ErrChannelConflict, store.ErrChannelStale} {
		t.Run(conflict.Error(), func(t *testing.T) {
			st := &testkit.ApprovalStore{CommitErrors: []error{conflict, nil}}
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
			if len(channels.Committed) != 1 || channels.Committed[0] != "ch-approved" {
				t.Fatalf("post-commit = %+v, want only the replanned channel id", channels.Committed)
			}
			if result.ChannelID != "ch-approved" {
				t.Errorf("channel id = %q", result.ChannelID)
			}
		})
	}
}

func TestApproverRechecksSupersessionAfterStaleChannelRetry(t *testing.T) {
	p := store.Proposal{
		ID: "older", JobID: "job1", Status: "submitted", ProposalJSON: `{}`,
		CreatedAt: time.Unix(100, 0),
	}
	newer := store.Proposal{ID: "newer", JobID: p.JobID, Status: "approved", CreatedAt: time.Unix(200, 0)}
	st := &testkit.ApprovalStore{
		CommitErrors: []error{store.ErrChannelStale},
		LatestFunc: func(call int) (store.Proposal, error) {
			if call == 1 {
				return store.Proposal{}, store.ErrNotFound
			}
			return newer, nil
		},
	}
	channels := &testkit.ApprovalChannels{}
	approver := suggest.NewApprover(st, channels, time.Now)

	_, err := approver.Approve(context.Background(), p, nil, "admin")
	if !errors.Is(err, suggest.ErrSuperseded) {
		t.Fatalf("error = %v, want ErrSuperseded", err)
	}
	if len(st.Commits) != 1 || len(channels.Plans) != 1 {
		t.Fatalf("commits/plans = %d/%d, want 1/1", len(st.Commits), len(channels.Plans))
	}
	if len(channels.Committed) != 0 {
		t.Fatalf("post-commit ran for superseded proposal: %+v", channels.Committed)
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
	qualitySink := &testkit.QualityRecorder{Err: errors.New("ledger unavailable")}
	decisionQuality := quality.NewProposalDecisionRecorder(qualitySink, slog.New(slog.DiscardHandler))
	approver := suggest.NewApprover(st, channels, func() time.Time { return decisionAt }).
		WithDecisionQuality(decisionQuality)
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
	if len(channels.Committed) != 1 || channels.Committed[0] != "ch-approved" {
		t.Fatalf("post-commit calls = %+v", channels.Committed)
	}
	observations := qualitySink.Observations()
	if len(observations) != 1 || observations[0].Stage != quality.StageApproval ||
		observations[0].Outcome != quality.OutcomeApproved || !observations[0].At.Equal(decisionAt) {
		t.Fatalf("approval observations = %+v", observations)
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
	qualitySink := &testkit.QualityRecorder{}
	decisionQuality := quality.NewProposalDecisionRecorder(qualitySink, slog.New(slog.DiscardHandler))
	approver := suggest.NewApprover(st, channels, time.Now).WithDecisionQuality(decisionQuality)
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
	if observations := qualitySink.Observations(); len(observations) != 0 {
		t.Fatalf("failed transaction recorded approval: %+v", observations)
	}
}
