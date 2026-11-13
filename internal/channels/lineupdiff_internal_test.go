package channels

import (
	"testing"

	"github.com/loomarr/loomarr/internal/channels/schedule"
)

// The minimal diff (§9) compares what a push would send: the programs by item and each break by
// the length Tunarr is given. A break that changes length is pushed; one shorter than Tunarr's
// floor reads back at the floor and is not. A program's length is its item's runtime, which
// Tunarr may report as its own, so it never forces a push by itself.
func TestLineupDiffersOnWhatAPushSends(t *testing.T) {
	program := func(id string, ms int64) schedule.Slot {
		return schedule.Slot{Kind: schedule.SlotProgram, LibraryItemID: id, DurationMs: ms}
	}
	flex := func(ms int64) schedule.Slot { return schedule.Slot{Kind: schedule.SlotFlex, DurationMs: ms} }
	filler := func(ms int64) schedule.Slot { return schedule.Slot{Kind: schedule.SlotFiller, DurationMs: ms} }

	for _, tc := range []struct {
		name            string
		desired, actual []schedule.Slot
		differs         bool
	}{
		{"same lineup", []schedule.Slot{program("1000", 3600000), flex(120000)},
			[]schedule.Slot{program("1000", 3600000), flex(120000)}, false},
		{"a break changes length", []schedule.Slot{program("1000", 3600000), flex(300000)},
			[]schedule.Slot{program("1000", 3600000), flex(120000)}, true},
		{"filler is a break", []schedule.Slot{filler(120000)}, []schedule.Slot{flex(120000)}, false},
		{"a short break reads back at the floor", []schedule.Slot{flex(0)}, []schedule.Slot{flex(30000)}, false},
		{"another program", []schedule.Slot{program("1000", 3600000)}, []schedule.Slot{program("1001", 3600000)}, true},
		{"Tunarr's own runtime for a program", []schedule.Slot{program("1000", 3600000)},
			[]schedule.Slot{program("1000", 3598000)}, false},
		{"one more slot", []schedule.Slot{program("1000", 3600000), flex(60000)},
			[]schedule.Slot{program("1000", 3600000)}, true},
	} {
		if got := lineupDiffers(tc.desired, tc.actual); got != tc.differs {
			t.Errorf("%s: lineupDiffers = %v; want %v", tc.name, got, tc.differs)
		}
	}
}
