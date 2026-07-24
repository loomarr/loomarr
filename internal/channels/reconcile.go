package channels

import (
	"context"
	"errors"
	"fmt"

	"github.com/mantonx/loomarr/internal/filler"
	"github.com/mantonx/loomarr/internal/metrics"
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
//  7. Nudge the media server (best-effort, §9): a NEW channel triggers a tuner
//     re-scan (discover it in the channel list); a lineup-only change triggers a
//     guide (EPG) refresh. These are distinct operations — a guide refresh alone
//     won't surface a new channel.
func (e *Engine) Reconcile(ctx context.Context, channelID string) (err error) {
	lock := e.lockFor(channelID)
	lock.Lock()
	defer lock.Unlock()

	// §17 reconcile-loop latency + channel-reconcile counter. e.now() is the
	// injected clock, so the duration is deterministic (0) under a fixed-time test.
	start := e.now()
	defer func() { metrics.ReconcileObserved(e.now().Sub(start), err == nil) }()

	ch, err := e.store.GetChannel(ctx, channelID)
	if err != nil {
		return fmt.Errorf("load channel %s: %w", channelID, err)
	}
	if ch.Status == schedule.StatusDetached || ch.Status == schedule.StatusPaused {
		return nil // detached = no longer managed (§9 ownership); paused = deliberately off the sweep
	}

	// Heal any entry that reached us UNRATED but whose title is now in the library
	// (§389 amendment): a fail-closed audience gate would otherwise drop it, taking
	// the channel to dead air (§9). Self-healing and bounded — only empty ratings are
	// looked up, and the healed value is stamped back onto ch.Lineup and persisted by
	// the UpsertChannel below, so a future reconcile skips the lookup entirely.
	e.healRatings(ctx, ch.Lineup)

	// 2: recompute desired from the approved lineup + current availability under the
	// channel's ChannelPolicy (programming-design §3–§7). ComputeDesiredAt resolves
	// each entry against the library (a vanished title comes back a placeholder),
	// applies the audience/scope/seasonal filters, and orders with separation. Apply
	// the global commercial-break density (§10) so breaks are interleaved, and pass
	// the wall-clock so seasonality (§6) evaluates against the container TZ.
	chDomain := ch.Channel
	chDomain.BreaksPerHour = e.breaksPerHour
	chDomain.DefaultWindow = e.defaultWindow // §6.5 rolling-window horizon from settings
	desired := schedule.ComputeDesiredAt(chDomain, ch.Lineup, e.avail, e.policy, ch.Policy, e.now())

	// Record the relaxation-ladder steps this pass applied (§7) back onto the
	// channel's policy so the UI surfaces them; recomputed from scratch each
	// reconcile, so a recovered pool automatically un-relaxes to an empty list.
	ch.Policy.Applied = desired.Applied

	// NOTE (§10 redesign): filler is no longer inlined into the desired lineup. The
	// interleaved SlotFiller break gaps (schedule.interleaveBreaks) flow through to
	// SetLineup as FLEX; the channel's matched clips live in a Tunarr filler-list
	// (attached below, after the channel exists) that Tunarr plays into that flex.
	// So `desired` is NOT mutated here — see attachFillerList.

	// 3: drift detection (§9 slot revalidation) is a comparison against what we
	// *previously* scheduled: a slot that was a real program in the persisted
	// desired and is no longer a program now means a scheduled item vanished
	// (deleted / re-id'd). An old `available` is never trusted forever — surface
	// it as StatusDrifted so the Channels view flags it.
	staleCount := staleProgramCount(ch.Desired, desired.EligibleKeys)
	drifted := staleCount > 0
	metrics.SlotSubstitutions(staleCount) // §17: no-op when 0

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
	channelListChanged := false // a NEW channel needs a tuner re-scan, not just a guide refresh (§9)
	if wasNew || tunarrID != ch.TunarrID {
		ch.TunarrID = tunarrID
		channelAffecting = true   // created (or recreated after out-of-band delete)
		channelListChanged = true // the media server must re-scan the tuner to discover it

		// CHECKPOINT the new id NOW, before the lineup push (which can fail — e.g. a
		// large library resolve timing out). Persisting the create separately makes it
		// durable on its own: if a later step errors, the next reconcile finds this id,
		// GetChannel confirms the channel exists, and it goes straight to the lineup —
		// no recreate, no number collision. Without this, a failed push discarded the
		// new id and every retry re-created channel N, orphaning a shell and 500ing on
		// the collision. Best-effort: a checkpoint failure is non-fatal (the end-of-
		// reconcile persist is the backstop), so it never blocks the push it precedes.
		if err := e.store.UpsertChannel(ctx, ch); err != nil && e.log != nil {
			e.log.Warn("checkpoint of new Tunarr id failed (reconcile continues)",
				"channel", channelID, "tunarrId", tunarrID, "err", err)
		}
	}

	// 4b: build + attach the channel's Tunarr filler-list from the matched clip
	// pool (§10), now that the channel exists (TunarrID is set). Tunarr plays these
	// clips into the flex gaps SetLineup leaves between programs. Best-effort: a
	// filler failure never fails the reconcile (§9 resilience — the channel still
	// plays, just without commercials this pass; the next sweep retries).
	if e.pods != nil {
		e.attachFillerList(ctx, ch)
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

	// Tell the UI the channel changed so it updates live (no manual refresh — the
	// "self-maintaining" model, §9). Best-effort: nil notifier / a dropped frame is a
	// latency concern, never a correctness one (GET /v1/channels is the truth on load).
	if e.notify != nil {
		e.notify.ChannelChanged(ch.ID, string(ch.Status))
	}

	// 7: media-server freshness (best-effort; never fails the reconcile). A NEW
	// channel needs a tuner re-scan so the media server discovers it in its channel
	// list (a guide refresh alone won't surface it, §9); a lineup-only change needs
	// just a guide (EPG) refresh.
	if channelListChanged {
		e.rescanTuner(ctx, channelID)
	} else if channelAffecting {
		e.pokeGuide(ctx, channelID)
	}
	return nil
}

// Purge fully removes a channel: it deletes the Tunarr channel (if one was ever
// pushed) and then hard-deletes the store row — the `?purge=true` path of DELETE
// /v1/channels/{id} (§7), as opposed to the default detach (which keeps both). It
// takes the per-channel lock so a concurrent reconcile can't race the deletion, and
// the Tunarr delete is idempotent (a 404 is already-gone). A channel that was never
// reconciled has no TunarrID, so only the store row is removed.
func (e *Engine) Purge(ctx context.Context, channelID string) error {
	lock := e.lockFor(channelID)
	lock.Lock()
	defer lock.Unlock()

	ch, err := e.store.GetChannel(ctx, channelID)
	if err != nil {
		return fmt.Errorf("load channel %s: %w", channelID, err)
	}
	if ch.TunarrID != "" && e.prog != nil {
		if derr := e.prog.DeleteChannel(ctx, ch.TunarrID); derr != nil {
			return fmt.Errorf("delete tunarr channel %s: %w", ch.TunarrID, derr)
		}
	}
	return e.store.DeleteChannel(ctx, channelID)
}

// attachFillerList builds the channel's matched clip pool via the assembler and
// hands its Tunarr program uuids to the Programmer, which builds + attaches the
// channel's Tunarr filler-list (§10). Tunarr plays those clips into the flex gaps
// the scheduler leaves between programs. Best-effort: a filler error is logged,
// never returned — the channel still plays (§9 resilience), and EnsureFillerList
// is internally idempotent so a stable pool makes no Tunarr write.
func (e *Engine) attachFillerList(ctx context.Context, ch store.Channel) {
	ids, ok := e.pods.BuildFillerList(ctx, ch.ID, PodSeed(ch.ID), SelectionForChannel(ch))
	if !ok {
		ids = nil // empty catalog / only-fallback → detach (channel falls back to flex)
	}
	if err := e.prog.EnsureFillerList(ctx, ch.TunarrID, ids); err != nil && e.log != nil {
		e.log.Warn("attach filler list (channel plays without commercials this pass)",
			"channel", ch.ID, "err", err)
	}
}

// healRatings fills the OfficialRating of any entry that reached the scheduler
// unrated, looking it up against the library by the entry's Key (§389 amendment).
// Mutates the slice in place — the caller persists ch.Lineup — so the heal is a
// one-time repair: a future reconcile finds the entry rated and skips it.
//
// Bounded and best-effort by design: only empty ratings are looked up (a stamped
// channel makes ZERO calls), a not-yet-present title (ok=false) is left unrated to
// try again next pass, and a lookup error never fails the reconcile — the channel
// still plays (§9 resilience).
func (e *Engine) healRatings(ctx context.Context, lineup []schedule.LineupEntry) {
	if e.ratings == nil {
		return
	}
	for i := range lineup {
		if lineup[i].OfficialRating != "" {
			continue
		}
		raw, ok, err := e.ratings.Rating(ctx, lineup[i].Key)
		if err != nil {
			if e.log != nil {
				e.log.Warn("heal rating (entry stays unrated this pass)", "key", lineup[i].Key, "err", err)
			}
			continue
		}
		if ok {
			lineup[i].OfficialRating = schedule.NormalizeRating(raw)
		}
	}
}

// SelectionForChannel translates a channel's persisted filler policy (§10
// FillerSelection) into the filler-package Selection the assembler consumes — the
// boundary translation that keeps the filler domain free of the schedule type. The era
// defaults from the channel's PROGRAM scope era when the filler selection sets no era of
// its own (a "90s action" channel gets 90s ads for free); a Range is a from–to window,
// so we take its lower bound as the representative era for the ladder. A nil filler
// policy yields the zero Selection = the whole catalog (the additive default).
//
// Exported so the §12 pod PREVIEW derives the selection identically. If preview computed
// its own, the two would drift and the UI would confidently show pods the reconciler
// never builds — the whole failure mode preview exists to prevent.
func SelectionForChannel(ch store.Channel) filler.Selection {
	sel := filler.Selection{}
	f := ch.Policy.Filler
	if f != nil {
		sel.Audience = filler.Audience(f.Audience)
		sel.Categories = f.Categories
		sel.Kinds = f.Kinds
		sel.Pinned = f.Pinned
		sel.Excluded = f.Excluded
		if f.Era != nil {
			sel.Era = f.Era.From
		}
	}
	// Default the era from the channel's program scope when the filler selection didn't
	// set one (the "seed filler era from scope.era" default, applied live rather than
	// only stamped at create — so an existing channel benefits too).
	if sel.Era == 0 && ch.Policy.Scope.Era != nil {
		sel.Era = ch.Policy.Scope.Era.From
	}
	return sel
}

// PodSeed derives a deterministic pod seed from the channel id (§10 seeded-
// deterministic — same channel rebuilds the same clip pool, so the filler-list
// attach is idempotent across reconciles). Exported for the same reason as PodEra:
// preview must seed from the identical value or it previews a different pod.
func PodSeed(channelID string) int64 {
	var h int64 = 1469598103934665603 // FNV-1a offset basis
	for _, b := range []byte(channelID) {
		h ^= int64(b)
		h *= 1099511628211
	}
	return h
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

// rescanTuner asks the media server to re-read the tuner channel list so a newly
// created channel is discovered (§9), logging (never returning) failures.
func (e *Engine) rescanTuner(ctx context.Context, channelID string) {
	if e.guide == nil {
		return
	}
	if err := e.guide.RescanTuner(ctx); err != nil {
		e.log.Warn("tuner re-scan failed (new channel may not appear until periodic scan, reconcile ok)",
			"channel", channelID, "err", err)
	}
}

// staleProgramCount reports how many Keys that were real programs in the
// previously-persisted desired the library can no longer supply (§9 slot
// revalidation). A non-zero count means scheduled items genuinely vanished from
// the library since the last reconcile — the signal for StatusDrifted and the §17
// slot-substitution count. A first-ever reconcile (empty prev) can't drift.
//
// The comparison is against the freshly-computed ELIGIBLE set — every key the
// library can currently supply for this channel — NOT the aired `Slots` (§6.5).
// This is the curation-rule/rolling-window drift fix: with rules or a rolling
// window, a program legitimately leaves `Slots` because the active rule narrowed
// scope, a seasonal bench removed it, or the window rotated to a different slice —
// none of which is drift. A key is stale ONLY when it is absent from `eligible`,
// i.e. the library actually lost the title. Comparing against `Slots` (as an
// earlier version did) would false-flag every daypart/window rotation as
// StatusDrifted. A nil `eligible` (policy-free path never populates it) falls back
// to treating any prev program as still-eligible (no drift) — the whole-run,
// no-window case where prev ⊆ next holds anyway.
func staleProgramCount(prev []schedule.Slot, eligible map[provision.Key]bool) int {
	n := 0
	for _, s := range prev {
		if s.IsProgram() && s.Key != "" && eligible != nil && !eligible[s.Key] {
			n++
		}
	}
	return n
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
// diff is expressed in domain terms. Since the §10 redesign moved filler into a
// Tunarr filler-list, filler slots are ALWAYS flex in the pushed lineup — only a
// program is content.
func pushShape(s schedule.Slot) (string, string) {
	if s.Kind == schedule.SlotProgram {
		return "content", s.LibraryItemID
	}
	return "flex", ""
}

// ErrChannelGone is returned by backfill helpers when the channel was deleted
// between the event and the reconcile (a benign race; the caller drops the event).
var ErrChannelGone = errors.New("channels: channel gone")

// isNotFound reports whether err is the store's not-found sentinel.
func isNotFound(err error) bool { return errors.Is(err, store.ErrNotFound) }
