package playout

import (
	"time"

	"github.com/loomarr/loomarr/internal/channels/schedule"
)

// FirstUnfinished is the index of the first slot of the accepted cycle, walked from epoch, that has
// not ended by `at`: everything before it has aired and will not air again before the cycle is
// replaced. A cycle that has already aired whole loops, so every slot airs again and the answer is
// 0; so is an `at` before the anchor.
func FirstUnfinished(slots []schedule.Slot, epoch, at time.Time) int {
	elapsed := at.Sub(epoch)
	if elapsed <= 0 || elapsed >= cycleDuration(slots) {
		return 0
	}
	for i, s := range slots {
		if elapsed < slotDuration(s) {
			return i
		}
		elapsed -= slotDuration(s)
	}
	return 0
}

// CycleLength is how long one pass of the cycle airs.
func CycleLength(slots []schedule.Slot) time.Duration { return cycleDuration(slots) }

// KeepOnAir is the part of the accepted cycle, walked from epoch, that a re-deal at `at` must not
// change: the programme on air (from its first part) and every programme starting before `until`,
// each with its continuations and the breaks after it. start is the anchor of the cycle that
// replaces this one.
//
// With fromAnchor, on the cycle's first pass, everything that aired is kept too, ahead of the
// programme on air, and start is epoch: the anchor does not move, and the guide still shows the
// hours before the edit as they aired. The caller asks for that only when the cycle it builds will
// not loop before it is replaced, since a loop would air the kept part again. Otherwise, and once
// the cycle has looped (what aired is then not one run of slots), the kept part starts at the
// programme on air and so does the anchor. A cycle with nothing that airs keeps nothing.
func KeepOnAir(slots []schedule.Slot, epoch, at, until time.Time, fromAnchor bool) (kept []schedule.Slot, start time.Time) {
	total := cycleDuration(slots)
	if total <= 0 || at.Before(epoch) {
		return nil, time.Time{}
	}
	idx, cursor := 0, epoch
	if !fromAnchor || at.Sub(epoch) >= total {
		into := at.Sub(epoch) % total
		cursor = at.Add(-into)
		for idx < len(slots) && into >= slotDuration(slots[idx]) {
			into -= slotDuration(slots[idx])
			cursor = cursor.Add(slotDuration(slots[idx]))
			idx++
		}
		idx, cursor = rewindToProgrammeStart(slots, idx%len(slots), cursor)
	}
	start = cursor
	for n := 0; n < len(slots); n++ {
		s := slots[(idx+n)%len(slots)]
		// A new programme starting at or after `until` ends what is kept; its continuations and
		// the breaks between programmes belong to the one before it.
		if n > 0 && !cursor.Before(until) && !continuesProgramme(s) && !isBreak(s) {
			break
		}
		kept = append(kept, s)
		cursor = cursor.Add(slotDuration(s))
	}
	return kept, start
}

// WithoutProgrammes is slots with every programme whose library item is in `aired` taken out,
// together with its continuations and the breaks after it, so a re-deal that follows what has
// aired does not air it again. Breaks before the first programme stay.
func WithoutProgrammes(slots []schedule.Slot, aired map[string]bool) []schedule.Slot {
	var out []schedule.Slot
	dropping := false
	for _, s := range slots {
		if !continuesProgramme(s) && !isBreak(s) {
			dropping = s.Kind == schedule.SlotProgram && aired[s.LibraryItemID]
		}
		if !dropping {
			out = append(out, s)
		}
	}
	return out
}
