package playout

import (
	"fmt"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/channels/schedule"
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

// A re-deal keeps the programme on air from its first part, and every programme starting before
// the run-ahead, with the break after each. On the cycle's first pass it also keeps what aired
// before them, from the anchor, which does not move; once the cycle has looped it keeps from the
// programme on air, which becomes the anchor. Either way the walk of what is kept airs the same
// thing as the accepted cycle at every instant up to `until`.
func TestKeepOnAirKeepsWhatTheStreamMayHaveEncoded(t *testing.T) {
	accepted := breakCycle(113_366)
	looped := breakEpoch.Add(cycleDuration(accepted) + 5*time.Hour)
	for _, tc := range []struct {
		name                  string
		at                    time.Time
		fromAnchor, fromStart bool
	}{
		{"first pass, from the anchor", breakNow, true, true},
		{"first pass, from the programme on air", breakNow, false, false},
		{"looped", looped, true, false},
	} {
		until := tc.at.Add(OnAirLookahead)
		kept, start := KeepOnAir(accepted, breakEpoch, tc.at, until, tc.fromAnchor)
		onAir := AiringAt(accepted, breakEpoch, tc.at)
		want := tc.at.Add(-onAir.Offset)
		if tc.fromStart {
			want = breakEpoch
		}
		if !start.Equal(want) {
			t.Fatalf("%s: kept from %v, want %v", tc.name, start, want)
		}
		if len(kept) == 0 || kept[len(kept)-1].Kind != schedule.SlotFiller {
			t.Fatalf("%s: kept %+v, want whole programmes each with its break", tc.name, kept)
		}
		for at := tc.at; at.Before(until); at = at.Add(time.Second) {
			if got, want := AiringAt(kept, start, at).LibraryItemID, AiringAt(accepted, breakEpoch, at).LibraryItemID; got != want {
				t.Fatalf("%s: at %v the kept cycle airs %q, the accepted one %q", tc.name, at.Sub(tc.at), got, want)
			}
		}
	}
	if kept, _ := KeepOnAir(nil, breakEpoch, breakNow, breakNow.Add(OnAirLookahead), true); kept != nil {
		t.Fatalf("an empty cycle kept %+v", kept)
	}
}

// A split programme is kept whole from its first part, and dropped whole with its mid-roll.
func TestSplitProgrammesAreKeptAndDroppedWhole(t *testing.T) {
	mins := func(m int) int64 { return (time.Duration(m) * time.Minute).Milliseconds() }
	film := []schedule.Slot{
		{Kind: schedule.SlotProgram, LibraryItemID: "film", DurationMs: mins(50), Segment: 1},
		{Kind: schedule.SlotFiller, DurationMs: mins(2), MidRoll: true},
		{Kind: schedule.SlotProgram, LibraryItemID: "film", DurationMs: mins(40), Segment: 2, SourceOffsetMs: mins(50)},
		{Kind: schedule.SlotFiller, DurationMs: mins(2)},
	}
	cycle := append(append([]schedule.Slot{}, film...),
		schedule.Slot{Kind: schedule.SlotProgram, LibraryItemID: "ep", DurationMs: mins(22)},
		schedule.Slot{Kind: schedule.SlotFiller, DurationMs: mins(2)})

	// Inside the second part: the film is kept from its first part.
	kept, start := KeepOnAir(cycle, breakEpoch, breakEpoch.Add(60*time.Minute), breakEpoch.Add(61*time.Minute), false)
	if !start.Equal(breakEpoch) || len(kept) != len(film) {
		t.Fatalf("kept %d slots from %v, want the film's %d from its start", len(kept), start.Sub(breakEpoch), len(film))
	}
	if got := WithoutProgrammes(cycle, map[string]bool{"film": true}); len(got) != 2 || got[0].LibraryItemID != "ep" {
		t.Fatalf("dropping the film left %+v, want the episode and its break", got)
	}
}

// Slots that have finished airing are behind FirstUnfinished; a cycle that has looped airs every
// slot again.
func TestFirstUnfinished(t *testing.T) {
	cycle := breakCycle(113_366)
	if got := FirstUnfinished(cycle, breakEpoch, breakEpoch.Add(22*time.Minute)); got != 1 {
		t.Errorf("at the end of the first episode: %d, want its break (1)", got)
	}
	if got := FirstUnfinished(cycle, breakEpoch, breakEpoch.Add(-time.Hour)); got != 0 {
		t.Errorf("before the anchor: %d, want 0", got)
	}
	if got := FirstUnfinished(cycle, breakEpoch, breakEpoch.Add(48*time.Hour)); got != 0 {
		t.Errorf("after the cycle looped: %d, want 0", got)
	}
}

// The stop rule: a channel's live stream restarts only when what it airs over [from, to) changes.
func TestOnAirChanged(t *testing.T) {
	accepted := breakCycle(113_366)
	to := breakNow.Add(12 * time.Second)

	if OnAirChanged(accepted, breakEpoch, accepted, breakEpoch, breakNow, to) {
		t.Error("an identical schedule changed what is on air")
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
