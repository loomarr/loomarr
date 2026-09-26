package playout

import (
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/schedule"
)

// A 45-minute film split at a fade 20 minutes in, between two ordinary programmes:
//
//	A 0–10 · break 10–11 · film part 1 11–31 · mid-roll 31–32 · film part 2 32–57 · break 57–58 · B 58–68
func midRollCycle() []schedule.Slot {
	film := schedule.Slot{Kind: schedule.SlotProgram, Key: "movie:tmdb:9", LibraryItemID: "film", Title: "Film"}
	part1, part2 := film, film
	part1.Segment, part1.DurationMs = 1, (20 * time.Minute).Milliseconds()
	part2.Segment, part2.SourceOffsetMs, part2.DurationMs = 2, (20 * time.Minute).Milliseconds(), (25 * time.Minute).Milliseconds()
	brk := schedule.Slot{Kind: schedule.SlotFiller, DurationMs: time.Minute.Milliseconds()}
	mid := brk
	mid.MidRoll = true
	return []schedule.Slot{
		{Kind: schedule.SlotProgram, Key: "movie:tmdb:1", LibraryItemID: "a", Title: "A", DurationMs: (10 * time.Minute).Milliseconds()},
		brk, part1, mid, part2, brk,
		{Kind: schedule.SlotProgram, Key: "movie:tmdb:2", LibraryItemID: "b", Title: "B", DurationMs: (10 * time.Minute).Milliseconds()},
	}
}

var midRollEpoch = time.Date(2026, 9, 26, 20, 0, 0, 0, time.UTC)

func at(min float64) time.Time {
	return midRollEpoch.Add(time.Duration(min * float64(time.Minute)))
}

// Resuming after a mid-roll break seeks the source to the cut: Offset is the source position the
// encoder's input starts at, while StartedAt/Remaining bound this part on the wall clock.
func TestAiringAtResumesASegmentAtItsCut(t *testing.T) {
	got := AiringAt(midRollCycle(), midRollEpoch, at(33))
	if got.LibraryItemID != "film" || got.Offset != 21*time.Minute || got.Remaining != 24*time.Minute || !got.StartedAt.Equal(at(32)) {
		t.Fatalf("airing = %+v, want film at source 21m with 24m left, part started 32m", got)
	}
	if first := AiringAt(midRollCycle(), midRollEpoch, at(11)); first.Offset != 0 || first.Remaining != 20*time.Minute {
		t.Fatalf("part 1 airing = %+v", first)
	}
	if brk := AiringAt(midRollCycle(), midRollEpoch, at(31.5)); brk.Kind != schedule.SlotFiller {
		t.Fatalf("mid-roll airing = %+v, want the break", brk)
	}
}

// The guide lists a split programme once, from its first part's start to its last part's stop —
// the mid-roll break is inside the entry, never an entry of its own (§10) — wherever the request
// window begins or ends.
func TestBroadcastsBetweenShowsASplitProgrammeAsOneEntry(t *testing.T) {
	for _, tc := range []struct {
		name     string
		from, to float64
	}{
		{"window inside part 2", 33, 34},
		{"window inside part 1", 12, 13},
		{"window inside the mid-roll break", 31.5, 31.6},
	} {
		t.Run(tc.name, func(t *testing.T) {
			bs := BroadcastsBetween(midRollCycle(), midRollEpoch, at(tc.from), at(tc.to))
			if len(bs) != 1 || bs[0].LibraryItemID != "film" || !bs[0].Start.Equal(at(11)) || !bs[0].Stop.Equal(at(57)) {
				t.Fatalf("broadcasts = %+v, want one film entry 11m–57m", bs)
			}
		})
	}
	whole := BroadcastsBetween(midRollCycle(), midRollEpoch, at(0), at(68))
	var titles []string
	for _, b := range whole {
		titles = append(titles, string(b.Kind)+":"+b.Title)
	}
	if len(whole) != 5 || whole[2].Title != "Film" || !whole[3].Start.Equal(at(57)) {
		t.Fatalf("cycle = %v", titles)
	}
}

// A pending acquisition stays anchored to the programme it precedes when an earlier programme is
// split: the pending walk must step over a split programme as one entry.
func TestBroadcastsWithPendingStepsOverSplitProgrammes(t *testing.T) {
	slots := midRollCycle()
	pending := schedule.Slot{Kind: schedule.SlotPending, Key: "movie:tmdb:5", Title: "Soon"}
	slots = append(slots[:6], append([]schedule.Slot{pending}, slots[6:]...)...)
	bs := BroadcastsWithPending(slots, midRollEpoch, at(0), at(68))
	for i, b := range bs {
		if b.Kind == schedule.SlotPending {
			if i+1 >= len(bs) || bs[i+1].Title != "B" || !b.Stop.Equal(bs[i+1].Start) {
				t.Fatalf("pending placed before %+v", bs[min(i+1, len(bs)-1)])
			}
			return
		}
	}
	t.Fatalf("no pending entry in %+v", bs)
}

// Per-item callers (preparation) see every part and mid-roll break at the start AiringAt gives it.
func TestSegmentsBetweenKeepsEachPart(t *testing.T) {
	segs := SegmentsBetween(midRollCycle(), midRollEpoch, at(33), at(34))
	if len(segs) != 1 || !segs[0].Start.Equal(at(32)) || !segs[0].Stop.Equal(at(57)) {
		t.Fatalf("segments = %+v, want film part 2 at 32m–57m", segs)
	}
	if all := SegmentsBetween(midRollCycle(), midRollEpoch, at(0), at(68)); len(all) != 7 {
		t.Fatalf("got %d segments, want 7", len(all))
	}
}

// The pins a re-schedule must honour: every programme on air or starting inside [from, to), with
// the cuts the accepted cycle gave it (nil = whole). Later programmes are left free to re-split.
func TestCommittedSplitsPinsWhatIsOnAirOrAboutToBe(t *testing.T) {
	// 33 m is inside the film's part 2; the horizon to 60 m also reaches B (starts 58 m).
	pins := CommittedSplits(midRollCycle(), midRollEpoch, at(33), at(60))
	cuts, filmPinned := pins["film"]
	if !filmPinned || len(cuts) != 1 || cuts[0] != (20*time.Minute).Milliseconds() {
		t.Fatalf("film pin = %v (pinned %v), want its accepted cut at 20 m", cuts, filmPinned)
	}
	if bCuts, ok := pins["b"]; !ok || bCuts != nil {
		t.Fatalf("B pin = %v (pinned %v), want pinned whole", bCuts, ok)
	}
	if _, ok := pins["a"]; ok {
		t.Fatal("A airs after the horizon and must stay free to be split")
	}
}

// A repeat of the same programme elsewhere in the cycle does not double its pinned cuts.
func TestCommittedSplitsReadsOneAiring(t *testing.T) {
	slots := append(midRollCycle(), midRollCycle()...)
	pins := CommittedSplits(slots, midRollEpoch, at(33), at(34))
	if cuts := pins["film"]; len(cuts) != 1 || cuts[0] != (20*time.Minute).Milliseconds() {
		t.Fatalf("film pin = %v, want one cut at 20 m", cuts)
	}
}
