package api

import (
	"context"
	"errors"

	"github.com/mantonx/loomarr/internal/schedule"
	"github.com/mantonx/loomarr/internal/store"
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
// approval planning uses, so explicit create and approval cannot disagree
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

// numberConflict returns a 409 when a channel number is already taken, and nil when it is free.
// Same rationale as the two helpers above: ONE answer, shared with the approve path.
//
// ⚠ Callers must not ask about a number the channel ALREADY holds — there is no "except me"
// escape, because Tunarr's channel list carries no identity to exclude by (see
// binder.NumberInUse). `updateChannel` only calls this when the number actually changes.
//
// ⚠ **The number must be free in TUNARR too** (design §2: "a channel number must be free in
// TUNARR too, and a collision moves LOOMARR'S channel"). `binder.nextFreeChannelNumber` has
// unioned both sources since #258, but the handlers an operator actually TYPES a number into
// checked `GetChannelByNumber` alone. The two disagreed in a way a user could see: a clash with
// a Loomarr channel was refused up front with this exact message, while a clash with a channel
// that exists only in Tunarr was accepted with a 201 and then renumbered underneath them by the
// reconcile (§9 V54). The renumber is the safety net working — but it should not be reachable
// by typing a number the server could have refused.
//
// ⚠ Falls back to the STORE-ONLY check when no binder is wired, rather than skipping the check.
// A store-only install still gets the Loomarr half of the answer; returning "free" there would
// trade a narrow inconsistency for a duplicate-number regression.
func (s *Server) numberConflict(ctx context.Context, number int) error {
	if s.binder == nil {
		return s.numberConflictFromStore(ctx, number)
	}
	inUse, err := s.binder.NumberInUse(ctx, number)
	if err != nil {
		return err
	}
	if inUse {
		return errConflict("Channel number in use",
			"Another channel already uses that number. Pick a different one.")
	}
	return nil
}

// numberConflictFromStore is the Loomarr-only half, for an install with no binder.
func (s *Server) numberConflictFromStore(ctx context.Context, number int) error {
	_, err := s.store.GetChannelByNumber(ctx, number)
	switch {
	case errors.Is(err, store.ErrNotFound):
		return nil
	case err != nil:
		return err
	default:
		return errConflict("Channel number in use",
			"Another channel already uses that number. Pick a different one.")
	}
}
