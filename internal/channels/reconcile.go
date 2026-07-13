package channels

import (
	"context"
	"errors"
	"fmt"

	"github.com/mantonx/loomarr/internal/programmer"
	"github.com/mantonx/loomarr/internal/provision"
	"github.com/mantonx/loomarr/internal/schedule"
	"github.com/mantonx/loomarr/internal/store"
)

// Reconcile brings one channel's actual Tunarr state in line with its desired
// lineup (§9). It is idempotent and minimal-diff: a second reconcile with no
// input change makes no Tunarr calls beyond the reads needed to confirm no diff.
// Safe to call from the API (POST /v1/channels/{id}/reconcile), the sweep, or a
// backfill event. Serialized per channel by the mutex (§18).
//
// Steps:
//  1. Load the channel; skip if detached.
//  2. Recompute desired from the approved lineup + current availability (pure).
//  3. Revalidate program slots against the library — a vanished program demotes
//     to a placeholder and flags the channel drifted (§9 slot revalidation).
//  4. Ensure the Tunarr channel exists (create → capture server-assigned id, or
//     update if metadata changed). Track whether this was channel-affecting.
//  5. Diff desired lineup vs Tunarr's actual; push only if they differ.
//  6. Persist the (possibly updated) channel: new TunarrID, desired snapshot,
//     status, next reconcile deadline.
//  7. If channel-affecting (created/renamed/lineup changed), poke the guide (§9,
//     best-effort).
func (e *Engine) Reconcile(ctx context.Context, channelID string) error {
	lock := e.lockFor(channelID)
	lock.Lock()
	defer lock.Unlock()

	ch, err := e.store.GetChannel(ctx, channelID)
	if err != nil {
		return fmt.Errorf("load channel %s: %w", channelID, err)
	}
	if ch.Status == schedule.StatusDetached {
		return nil // no longer managed (§9 ownership)
	}

	// 2: recompute desired from the approved lineup + current availability
	// (pure). ComputeDesired already resolves each entry against the library, so
	// a title that has vanished simply comes back as a placeholder this pass.
	desired := schedule.ComputeDesired(ch.Channel, ch.Lineup, e.avail, e.policy)

	// 3: drift detection (§9 slot revalidation) is a comparison against what we
	// *previously* scheduled: a slot that was a real program in the persisted
	// desired and is no longer a program now means a scheduled item vanished
	// (deleted / re-id'd). An old `available` is never trusted forever — surface
	// it as StatusDrifted so the Channels view flags it.
	drifted := programWentStale(ch.Desired, desired.Slots)

	channelAffecting := false

	// 4: ensure the Tunarr channel exists / is up to date.
	spec := programmer.ChannelSpec{
		TunarrID: ch.TunarrID,
		Number:   ch.Number,
		Name:     ch.Name,
		Group:    ch.Group,
		Logo:     ch.Logo,
	}
	wasNew := ch.TunarrID == ""
	tunarrID, err := e.ensureChannel(ctx, spec)
	if err != nil {
		return err
	}
	if wasNew || tunarrID != ch.TunarrID {
		ch.TunarrID = tunarrID
		channelAffecting = true // created (or recreated after out-of-band delete)
	}

	// 5: diff desired lineup vs actual; push only on a difference.
	actual, err := e.prog.GetLineup(ctx, ch.TunarrID)
	if err != nil {
		return fmt.Errorf("read lineup %s: %w", ch.TunarrID, err)
	}
	if lineupDiffers(desired.Slots, actual) {
		if err := e.prog.SetLineup(ctx, ch.TunarrID, desired.Slots); err != nil {
			return fmt.Errorf("push lineup %s: %w", ch.TunarrID, err)
		}
		channelAffecting = true
	}

	// 6: persist. Status reflects drift; a channel with any real program is live.
	ch.Desired = desired.Slots
	ch.Status = e.statusFor(desired, drifted)
	ch.ReconcileDeadline = e.now().Add(e.reconcileTTL)
	ch.UpdatedAt = e.now().Unix()
	if err := e.store.UpsertChannel(ctx, ch); err != nil {
		return fmt.Errorf("persist channel %s: %w", channelID, err)
	}

	// 7: guide freshness (best-effort; never fails the reconcile).
	if channelAffecting {
		e.pokeGuide(ctx, channelID)
	}
	return nil
}

// ensureChannel creates or updates the Tunarr channel. On create, Tunarr assigns
// the id (Phase-0 finding 1) — EnsureChannel returns it; we must persist it.
// Handles out-of-band deletion: if we hold a TunarrID but the channel is gone,
// recreate it.
func (e *Engine) ensureChannel(ctx context.Context, spec programmer.ChannelSpec) (string, error) {
	if spec.TunarrID != "" {
		_, ok, err := e.prog.GetChannel(ctx, spec.TunarrID)
		if err != nil {
			return "", fmt.Errorf("check channel %s: %w", spec.TunarrID, err)
		}
		if !ok {
			// The channel was deleted in Tunarr out of band; recreate it.
			spec.TunarrID = ""
		}
	}
	id, err := e.prog.EnsureChannel(ctx, spec)
	if err != nil {
		return "", fmt.Errorf("ensure channel: %w", err)
	}
	return id, nil
}

// statusFor derives the channel's Loomarr-side status from its desired lineup.
func (e *Engine) statusFor(d schedule.DesiredLineup, drifted bool) schedule.ChannelStatus {
	if drifted {
		return schedule.StatusDrifted
	}
	return schedule.StatusLive
}

// pokeGuide triggers a guide refresh, logging (never returning) failures (§9).
func (e *Engine) pokeGuide(ctx context.Context, channelID string) {
	if e.guide == nil {
		return
	}
	if err := e.guide.PokeGuideRefresh(ctx); err != nil {
		e.log.Warn("guide refresh poke failed (freshness degraded, reconcile ok)",
			"channel", channelID, "err", err)
	}
}

// programWentStale reports whether any Key that was a real program in the
// previously-persisted desired is no longer a program in the freshly-computed
// desired (§9 slot revalidation). That means a scheduled item vanished from the
// library since the last reconcile — the signal for StatusDrifted. A first-ever
// reconcile (empty prev) can't drift.
func programWentStale(prev, next []schedule.Slot) bool {
	nowProgram := map[provision.Key]bool{}
	for _, s := range next {
		if s.IsProgram() && s.Key != "" {
			nowProgram[s.Key] = true
		}
	}
	for _, s := range prev {
		if s.IsProgram() && s.Key != "" && !nowProgram[s.Key] {
			return true
		}
	}
	return false
}

// lineupDiffers reports whether the desired slots differ from Tunarr's actual
// programming in a way that requires a push. Comparison is on the pushable shape
// (kind + library item + duration) since Tunarr can't round-trip our Key
// (lineup.go itemToSlot). A length change or any positional difference triggers a
// push — this is the minimal-diff decision (§9): equal ⇒ no Tunarr write.
func lineupDiffers(desired, actual []schedule.Slot) bool {
	if len(desired) != len(actual) {
		return true
	}
	for i := range desired {
		if !pushEqual(desired[i], actual[i]) {
			return true
		}
	}
	return false
}

// pushEqual compares two slots by what actually reaches Tunarr. A desired
// program/filler-with-item is a "content" item keyed by LibraryItemID; everything
// else is flex. So two slots are push-equal iff they render to the same Tunarr
// item type + id.
func pushEqual(want, got schedule.Slot) bool {
	wType, wID := pushShape(want)
	gType, gID := pushShape(got)
	return wType == gType && wID == gID
}

// pushShape returns the (tunarr-type, item-id) a slot renders to, matching
// programmer.slotToItem's logic. Kept here (not exported from programmer) so the
// diff is expressed in domain terms.
func pushShape(s schedule.Slot) (string, string) {
	switch s.Kind {
	case schedule.SlotProgram:
		return "content", s.LibraryItemID
	case schedule.SlotFiller:
		if s.LibraryItemID != "" {
			return "content", s.LibraryItemID
		}
		return "flex", ""
	default:
		return "flex", ""
	}
}

// ErrChannelGone is returned by backfill helpers when the channel was deleted
// between the event and the reconcile (a benign race; the caller drops the event).
var ErrChannelGone = errors.New("channels: channel gone")

// isNotFound reports whether err is the store's not-found sentinel.
func isNotFound(err error) bool { return errors.Is(err, store.ErrNotFound) }
