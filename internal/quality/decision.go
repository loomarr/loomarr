package quality

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"log/slog"
	"time"
)

// ObservationRecorder is the durable receipt seam used by quality classifiers.
type ObservationRecorder interface {
	RecordQualityObservation(context.Context, Observation) error
}

// ProposalDecisionRecorder translates committed Proposal decisions into the
// closed quality vocabulary. Its methods deliberately return no error: the
// decision already committed, so observability cannot revise its outcome.
type ProposalDecisionRecorder struct {
	sink ObservationRecorder
	log  *slog.Logger
}

func NewProposalDecisionRecorder(sink ObservationRecorder, log *slog.Logger) *ProposalDecisionRecorder {
	if log == nil {
		log = slog.Default()
	}
	return &ProposalDecisionRecorder{sink: sink, log: log}
}

func (r *ProposalDecisionRecorder) ProposalApproved(ctx context.Context, proposalID string, at time.Time) {
	r.record(ctx, proposalID, at, OutcomeApproved)
}

func (r *ProposalDecisionRecorder) ProposalDeclined(ctx context.Context, proposalID string, at time.Time) {
	r.record(ctx, proposalID, at, OutcomeDeclined)
}

func (r *ProposalDecisionRecorder) record(ctx context.Context, proposalID string, at time.Time, outcome Outcome) {
	if r == nil || r.sink == nil {
		return
	}
	if proposalID == "" || at.IsZero() {
		r.log.Warn("proposal decision quality observation is missing authoritative fields", "outcome", outcome)
		return
	}
	observation := Observation{
		IdempotencyKey: proposalDecisionKey(proposalID),
		At:             at,
		Stage:          StageApproval,
		Outcome:        outcome,
	}
	if err := r.sink.RecordQualityObservation(ctx, observation); err != nil {
		r.log.Warn("record proposal decision quality observation", "outcome", outcome, "err", err)
	}
}

func proposalDecisionKey(proposalID string) string {
	hash := sha256.New()
	_, _ = hash.Write([]byte(proposalID))
	_, _ = hash.Write([]byte{0})
	_, _ = hash.Write([]byte(StageApproval))
	return hex.EncodeToString(hash.Sum(nil))
}
