package quality_test

import (
	"context"
	"errors"
	"log/slog"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/quality"
	"github.com/loomarr/loomarr/internal/testkit"
)

func TestProposalDecisionRecorderEmitsClosedOpaqueObservations(t *testing.T) {
	sink := &testkit.QualityRecorder{}
	recorder := quality.NewProposalDecisionRecorder(sink, slog.New(slog.DiscardHandler))
	approvedAt := time.Unix(1_800_000_000, 0).UTC()
	declinedAt := approvedAt.Add(time.Minute)

	recorder.ProposalApproved(context.Background(), "proposal-private-a", approvedAt)
	recorder.ProposalDeclined(context.Background(), "proposal-private-b", declinedAt)

	got := sink.Observations()
	if len(got) != 2 {
		t.Fatalf("observations = %+v, want two", got)
	}
	want := []struct {
		at      time.Time
		outcome quality.Outcome
	}{{approvedAt, quality.OutcomeApproved}, {declinedAt, quality.OutcomeDeclined}}
	for i, observation := range got {
		if observation.Stage != quality.StageApproval || observation.Outcome != want[i].outcome ||
			!observation.At.Equal(want[i].at) {
			t.Errorf("observation[%d] = %+v", i, observation)
		}
		if len(observation.IdempotencyKey) != 64 ||
			observation.IdempotencyKey == "proposal-private-a" ||
			observation.IdempotencyKey == "proposal-private-b" {
			t.Errorf("observation[%d] key is not opaque: %q", i, observation.IdempotencyKey)
		}
		if observation.Duration != 0 || observation.ToolCalls != 0 ||
			observation.CandidateCount != 0 || observation.CostNanos != 0 || observation.RunSnapshotID != "" {
			t.Errorf("observation[%d] leaked unrelated facts: %+v", i, observation)
		}
	}
}

func TestProposalDecisionRecorderIsIdempotentAndBestEffort(t *testing.T) {
	sink := &testkit.QualityRecorder{Err: errors.New("ledger unavailable")}
	recorder := quality.NewProposalDecisionRecorder(sink, slog.New(slog.DiscardHandler))
	at := time.Unix(1_800_000_000, 0).UTC()

	recorder.ProposalApproved(context.Background(), "proposal-private", at)
	recorder.ProposalApproved(context.Background(), "proposal-private", at)

	got := sink.Observations()
	if len(got) != 2 || got[0].IdempotencyKey != got[1].IdempotencyKey {
		t.Fatalf("replayed observations = %+v", got)
	}
}
