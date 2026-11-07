package schedule

// pendingSlot decides what a lineup entry becomes when its title is NOT yet
// available (§9 pending-slot policy). This is a behavioral fork the design doc
// calls out explicitly, so it lives in its own small function:
//
//   - PodFill (§9 DEFAULT): the gap is filled with matched filler so the channel
//     reads as continuous broadcast — "never dead air". The slot becomes
//     SlotFiller here as a *placeholder*; §10's pod assembly later swaps in the
//     era/audience-matched clips. Crucially the entry's Key is preserved on the
//     filler slot so backfill can find "the slot that was holding this title's
//     place" and replace it in-place when `available` arrives (§9 stable
//     placement — no global reshuffle of a live channel).
//
//   - ComingSoon: the gap stays an explicit SlotPending slot (a "coming soon"
//     interstitial card is the config alternative in §9). The viewer sees a
//     placeholder card rather than filler; the slot still carries the Key for
//     in-place backfill.
func pendingSlot(e LineupEntry, policy PendingPolicy) Slot {
	// Key + Title are preserved in every branch: backfill correlates the
	// incoming `available` event to *this* slot by Key and replaces it in place
	// (§9 stable placement). No LibraryItemID — the title isn't available yet.
	s := Slot{Key: e.Key, Title: e.Title, DurationMs: e.DurationMs}
	switch policy {
	case ComingSoon:
		s.Kind = SlotPending // explicit gap; UI shows a "coming soon" card
	case PodFill:
		fallthrough
	default: // pod-fill is the §9 default, so an unset policy pod-fills
		s.Kind = SlotFiller // §10 pod assembly swaps matched clips in later
	}
	return s
}
