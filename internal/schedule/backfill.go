package schedule

import "github.com/mantonx/loomarr/internal/provision"

// This file holds the PURE backfill/substitution operations the reconcile engine
// applies to a DesiredLineup when provisioning events arrive (§9 backfill loop).
// They operate on an already-computed lineup so the engine can also reach them
// from the periodic sweep (event-loss recovery, §9) — the sweep just recomputes
// the whole DesiredLineup, but these helpers express the two point mutations
// that a live event triggers, which the reconcile diff turns into minimal Tunarr
// calls.

// PlaceAvailable replaces the (pending/filler) placeholder slot holding `key`
// with a real program slot, IN PLACE (§9 stable placement — no reshuffle of a
// live channel). Returns the updated lineup and whether a slot was changed.
//
// It matches on Key, which every placeholder preserves (pending.go). If several
// slots share the key (a series across multiple slots is possible), all matching
// non-program placeholders are placed. Slots already SlotProgram are left alone
// (idempotent: a duplicate `available` event is a no-op).
func (d DesiredLineup) PlaceAvailable(key provision.Key, libraryItemID, title string, durationMs int64) (DesiredLineup, bool) {
	changed := false
	out := make([]Slot, len(d.Slots))
	copy(out, d.Slots)
	for i, s := range out {
		if s.Key != key || s.Kind == SlotProgram {
			continue
		}
		out[i] = Slot{
			Kind:          SlotProgram,
			Key:           key,
			LibraryItemID: libraryItemID,
			Title:         orDefault(title, s.Title),
			DurationMs:    orDefaultInt(durationMs, s.DurationMs),
		}
		changed = true
	}
	return DesiredLineup{ChannelID: d.ChannelID, Slots: out}, changed
}

// Substitute permanently replaces the slot(s) holding `key` when the title will
// never arrive (§9: on `unavailable`, substitute — next alternate, else the
// fallback pool). The caller supplies the replacement slot already resolved from
// alternates[]/fallback (the pool selection is the reconcile engine's job; the
// pure domain only performs the swap). Returns the updated lineup and whether
// anything changed.
func (d DesiredLineup) Substitute(key provision.Key, replacement Slot) (DesiredLineup, bool) {
	changed := false
	out := make([]Slot, len(d.Slots))
	copy(out, d.Slots)
	for i, s := range out {
		if s.Key != key {
			continue
		}
		out[i] = replacement
		changed = true
	}
	return DesiredLineup{ChannelID: d.ChannelID, Slots: out}, changed
}

// RevalidateAgainstLibrary is the sweep's slot-revalidation pass (§9 / §4
// invariant 1): every program slot is re-checked against current availability;
// a program whose item has vanished (deleted, replaced, re-id'd) is turned back
// into a placeholder under `policy` so the next reconcile substitutes it. Returns
// the revalidated lineup and whether any program went stale — the caller flags
// the channel StatusDrifted when true (§9). An old `available` is never trusted
// forever.
func (d DesiredLineup) RevalidateAgainstLibrary(avail Availability, policy PendingPolicy) (DesiredLineup, bool) {
	drifted := false
	out := make([]Slot, len(d.Slots))
	copy(out, d.Slots)
	for i, s := range out {
		if !s.IsProgram() {
			continue
		}
		if _, ok := avail.Resolve(s.Key); ok {
			continue // still present
		}
		// The program vanished from the library — demote to a placeholder.
		out[i] = pendingSlot(LineupEntry{Key: s.Key, Title: s.Title, DurationMs: s.DurationMs}, policy)
		drifted = true
	}
	return DesiredLineup{ChannelID: d.ChannelID, Slots: out}, drifted
}

func orDefault(v, fallback string) string {
	if v == "" {
		return fallback
	}
	return v
}

func orDefaultInt(v, fallback int64) int64 {
	if v == 0 {
		return fallback
	}
	return v
}
