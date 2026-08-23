package channels

import (
	"slices"

	"github.com/loomarr/loomarr/internal/schedule"
)

// capCommercialBreaks makes an internal-playout break no longer than the real media
// assembled for it. Keyed filler slots are unavailable-program placeholders, not
// commercial breaks, and retain the programme runtime they stand in for.
func capCommercialBreaks(slots []schedule.Slot, playableMs int64) []schedule.Slot {
	if playableMs <= 0 {
		return slots
	}
	var out []schedule.Slot
	for i, slot := range slots {
		if slot.Kind != schedule.SlotFiller || slot.Key != "" || slot.DurationMs <= playableMs {
			continue
		}
		if out == nil {
			out = slices.Clone(slots)
		}
		out[i].DurationMs = playableMs
	}
	if out == nil {
		return slots
	}
	return out
}
