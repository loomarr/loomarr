package playout

import (
	"fmt"
	"slices"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/schedule"
)

// breakCycle is a day of 22-minute episodes, each followed by a break of breakMs: the shape of
// the household channel on 2026-10-24, whose breaks were all one pod's playable length.
func breakCycle(breakMs int64) []schedule.Slot {
	var slots []schedule.Slot
	for i := 0; i < 64; i++ {
		slots = append(slots,
			schedule.Slot{Kind: schedule.SlotProgram, LibraryItemID: fmt.Sprintf("ep%02d", i), Title: fmt.Sprintf("Episode %d", i), DurationMs: (22 * time.Minute).Milliseconds()},
			schedule.Slot{Kind: schedule.SlotFiller, DurationMs: breakMs},
		)
	}
	return slots
}

var (
	breakEpoch = time.Date(2026, 10, 24, 0, 10, 55, 0, time.UTC)
	// 21 h after the anchor, about 51 breaks in: where the viewer was at 21:20.
	breakNow = breakEpoch.Add(21*time.Hour + 10*time.Minute)
)

// A reconcile that re-sizes the channel's breaks must not move what is on air: every break that
// has aired, or airs before the freeze horizon, keeps its accepted length. On 2026-10-24 each
// re-size moved the programme on air by 40 s to 4.5 min, so the viewer saw scenes again, or the
// same break twice.
func TestKeepAcceptedBreaksHoldsTheAiredTimeline(t *testing.T) {
	accepted := breakCycle(113_366)
	next := breakCycle(113_866) // half a second longer per break
	if same := AiringAt(next, breakEpoch, breakNow); same == AiringAt(accepted, breakEpoch, breakNow) {
		t.Fatal("precondition: re-sized breaks should move the airing on now")
	}

	until := breakNow.Add(30 * time.Minute)
	kept := KeepAcceptedBreaks(accepted, next, breakEpoch, until)

	for _, at := range []time.Time{breakEpoch, breakNow, until.Add(-time.Second)} {
		if got, want := AiringAt(kept, breakEpoch, at), AiringAt(accepted, breakEpoch, at); got != want {
			t.Fatalf("at %v: kept airs %+v, accepted aired %+v", at.Sub(breakEpoch), got, want)
		}
	}
	// After the horizon the new length is free to take effect: nothing there has aired.
	last := kept[len(kept)-1]
	if last.DurationMs != 113_866 {
		t.Fatalf("last break = %d ms, want the re-sized 113866 beyond the horizon", last.DurationMs)
	}
}

// An edit that changes no timing (a file re-identified, a title healed) keeps every aired break
// held: nothing it changes moves the walk. One that changes timing (a programme removed or
// re-timed) holds the breaks before it, so what airs up to the edit stays where it was, and
// commits the rest as computed.
func TestKeepAcceptedBreaksHoldsUpToTheFirstTimingChange(t *testing.T) {
	accepted := breakCycle(113_366)
	held := func(slots []schedule.Slot) int {
		n := 0
		for _, s := range slots {
			if isBreak(s) && s.DurationMs == 113_366 {
				n++
			}
		}
		return n
	}
	aired := held(KeepAcceptedBreaks(accepted, breakCycle(113_866), breakEpoch, breakNow))
	if aired < 50 {
		t.Fatalf("precondition: %d breaks aired by 21:10, want about 53", aired)
	}
	for _, tc := range []struct {
		name string
		edit func([]schedule.Slot) []schedule.Slot
		held int // breaks held, counting from the cycle start
	}{
		{"file re-identified", func(s []schedule.Slot) []schedule.Slot { s[4].LibraryItemID = "other"; return s }, aired},
		{"title healed", func(s []schedule.Slot) []schedule.Slot { s[4].Title = "Pilot"; return s }, aired},
		// An episode and its break swapped for the next ones moves no timing, only which episode airs
		// where (OnAirChanged sees that); a longer programme inserted moves everything after it.
		{"episode and break removed", func(s []schedule.Slot) []schedule.Slot { return append(s[:4:4], s[6:]...) }, aired},
		{"film inserted", func(s []schedule.Slot) []schedule.Slot {
			film := schedule.Slot{Kind: schedule.SlotProgram, LibraryItemID: "film", DurationMs: (95 * time.Minute).Milliseconds()}
			return slices.Insert(s, 4, film)
		}, 2},
		{"programme re-timed", func(s []schedule.Slot) []schedule.Slot { s[2].DurationMs += 1000; return s }, 1},
	} {
		if got := held(KeepAcceptedBreaks(accepted, tc.edit(breakCycle(113_866)), breakEpoch, breakNow)); got != tc.held {
			t.Errorf("%s: %d breaks held, want %d", tc.name, got, tc.held)
		}
	}
}

// Nothing has aired before a channel's anchor, and an empty accepted cycle has nothing to hold.
func TestKeepAcceptedBreaksBeforeAnythingAired(t *testing.T) {
	next := breakCycle(113_866)
	if kept := KeepAcceptedBreaks(breakCycle(113_366), next, breakEpoch, breakEpoch); kept[1].DurationMs != 113_866 {
		t.Fatal("held a break on a timeline that has not started")
	}
	if kept := KeepAcceptedBreaks(nil, next, breakEpoch, breakNow); kept[1].DurationMs != 113_866 {
		t.Fatal("held a break with no accepted cycle")
	}
}

// The stop rule: a channel's live stream restarts only when what it airs over [from, to) changes.
func TestOnAirChanged(t *testing.T) {
	accepted := breakCycle(113_366)
	to := breakNow.Add(12 * time.Second)

	if OnAirChanged(accepted, breakEpoch, accepted, breakEpoch, breakNow, to) {
		t.Error("an identical schedule changed what is on air")
	}
	kept := KeepAcceptedBreaks(accepted, breakCycle(113_866), breakEpoch, breakNow.Add(30*time.Minute))
	if OnAirChanged(accepted, breakEpoch, kept, breakEpoch, breakNow, to) {
		t.Error("a re-size held through the horizon changed what is on air")
	}
	if !OnAirChanged(accepted, breakEpoch, breakCycle(113_866), breakEpoch, breakNow, to) {
		t.Error("a re-size that moves the programme on air was not seen")
	}
	// A change beyond the window reaches the packager at its next lookup, without a restart.
	later := breakCycle(113_366)
	later[len(later)-2].LibraryItemID = "replaced tonight"
	if OnAirChanged(accepted, breakEpoch, later, breakEpoch, breakNow, to) {
		t.Error("a change hours ahead restarted the stream")
	}
	if !OnAirChanged(accepted, breakEpoch, accepted, breakEpoch.Add(time.Minute), breakNow, to) {
		t.Error("a moved anchor was not seen")
	}
}
