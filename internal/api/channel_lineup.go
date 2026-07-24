package api

import (
	"context"

	"github.com/mantonx/loomarr/internal/schedule"
)

// lineupFromIntent resolves the approved proposal identified by intentRef (the
// suggestion job id) and maps its lineup into the scheduler's []LineupEntry —
// the "create a channel from an approved proposal (intent + lineup)" flow the
// API contract promises (§7 POST /v1/channels, §9). Without this, a channel
// created from an approved intent builds EMPTY (the intentRef would be inert).
//
// Returns (nil, nil) when intentRef is "" — a hand-made channel legitimately has
// no source proposal and gets its lineup via PUT /v1/channels/{id} instead.
//
// Delegates to the shared *binder.Binder (§7) — the same lineup resolution the
// approve path uses, so createChannel and BindApprovedChannel can never disagree
// about what an approved proposal's lineup is. Guarded for a nil binder the same
// way an empty intentRef is: unit tests that only exercise hand-made channels
// (no intentRef) needn't wire one.
func (s *Server) lineupFromIntent(ctx context.Context, intentRef string) ([]schedule.LineupEntry, error) {
	if intentRef == "" || s.binder == nil {
		return nil, nil
	}
	return s.binder.LineupFromIntent(ctx, intentRef)
}

// policyFromIntent resolves the approved proposal's grounded ChannelPolicy
// (programming-design §8) so it lands on the channel row at create time. Returns a
// zero policy (⇒ built-in defaults) for a hand-made channel (empty intentRef) or a
// proposal that carried no policy. Mirrors lineupFromIntent — same approved-proposal
// gate, so an unapproved intent never brings a policy onto a live channel.
func (s *Server) policyFromIntent(ctx context.Context, intentRef string) (schedule.ChannelPolicy, error) {
	if intentRef == "" || s.binder == nil {
		return schedule.ChannelPolicy{}, nil
	}
	return s.binder.PolicyFromIntent(ctx, intentRef)
}
