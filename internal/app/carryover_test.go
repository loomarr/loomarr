package app

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/playout"
	"github.com/loomarr/loomarr/internal/provision"
	"github.com/loomarr/loomarr/internal/schedule"
)

// Rolling-window carry-over (#1675).
//
// Production shape, shrunk: every window arranges its own slice of 22-minute episodes and the
// slice is a little longer than the window, so one episode always crosses the boundary. Before
// the fix the next slice was walked from the channel's fixed anchor, so it was entered mid-phase:
// the crossing episode was cut and the next one joined part-way through. Live on the demo library
// a 22-minute episode aired 16 minutes, then the next slice joined another episode 8 minutes in.

// windowDecks arranges three 22-minute episodes per rolling window, named for the window.
type windowDecks struct {
	window time.Duration
	asked  []time.Time
}

func (d *windowDecks) CyclePreview(_ context.Context, _ string, at time.Time) (
	time.Time, []schedule.Slot, schedule.ActiveRuleAttribution, time.Duration, error,
) {
	d.asked = append(d.asked, at)
	opened := schedule.WindowStart(at, d.window)
	var slots []schedule.Slot
	for ep := 1; ep <= 3; ep++ {
		id := fmt.Sprintf("%s-e%d", opened.Format("15h"), ep)
		slots = append(slots, schedule.Slot{
			Kind: schedule.SlotProgram, Key: provision.Key("series:tvdb:1"), Title: id, LibraryItemID: id,
			DurationMs: (22 * time.Minute).Milliseconds(),
		})
	}
	return at, slots, schedule.ActiveRuleAttribution{}, d.window, nil
}

// carryOverResolver has window 00:00–01:00 committed: its slice, walked from the anchor 00:00.
func carryOverResolver(now time.Time) (*playoutResolver, time.Time) {
	w0 := time.Date(2026, time.August, 15, 0, 0, 0, 0, time.UTC)
	decks := &windowDecks{window: time.Hour}
	_, committed, _, _, _ := decks.CyclePreview(context.Background(), "ch1", w0)
	accepted := &stubChannels{}
	accepted.ch.ID, accepted.ch.Desired, accepted.ch.PlayoutAnchor = "ch1", committed, w0
	accepted.ch.Status = schedule.StatusLive
	return &playoutResolver{engine: decks, channels: accepted, now: func() time.Time { return now }}, w0
}

// A programme spanning the boundary airs whole, and the next window starts at its end — in the
// guide, with no clipped block and no overlap.
func TestGuide_ProgrammeCrossingTheWindowBoundaryAirsWhole(t *testing.T) {
	t.Parallel()
	r, w0 := carryOverResolver(time.Date(2026, time.August, 15, 0, 30, 0, 0, time.UTC))

	bs, err := r.segmentedBroadcasts(context.Background(), "ch1", w0, w0.Add(3*time.Hour), playout.BroadcastsBetween)
	if err != nil {
		t.Fatal(err)
	}
	for i, b := range bs {
		if b.Duration() != 22*time.Minute {
			t.Errorf("%s aired %v of 22m (%s → %s): a programme was cut", b.Title, b.Duration(), b.Start.Format("15:04"), b.Stop.Format("15:04"))
		}
		if i > 0 && !b.Start.Equal(bs[i-1].Stop) {
			t.Errorf("%s starts %s but %s stops %s: the timeline is not continuous",
				b.Title, b.Start.Format("15:04"), bs[i-1].Title, bs[i-1].Stop.Format("15:04"))
		}
	}
	// 00:44–01:06 is the last episode of the 00h slice; the 01h slice starts where it ends.
	if len(bs) < 4 || bs[2].Title != "00h-e3" || bs[3].Title != "01h-e1" || !bs[3].Start.Equal(w0.Add(66*time.Minute)) {
		t.Fatalf("handoff wrong: %v", guideLine(bs))
	}
}

// The encoder walks the same timeline: the crossing episode keeps airing past the boundary, and
// the next window's first episode starts at its end — before any reconcile turns the window.
func TestAiringAt_CarriesTheCrossingProgrammeOverTheBoundary(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		at         time.Duration // after 00:00
		want       string
		wantOffset time.Duration
	}{
		{at: 61 * time.Minute, want: "00h-e3", wantOffset: 17 * time.Minute},
		{at: 66*time.Minute + time.Second, want: "01h-e1", wantOffset: time.Second},
	} {
		r, w0 := carryOverResolver(w0At(tc.at))
		slots, epoch, err := r.acceptedCycle(context.Background(), "ch1")
		if err != nil {
			t.Fatal(err)
		}
		got := playout.AiringAt(slots, epoch, w0.Add(tc.at))
		if got.LibraryItemID != tc.want || got.Offset != tc.wantOffset {
			t.Errorf("at +%v: airing %s at offset %v, want %s at %v", tc.at, got.LibraryItemID, got.Offset, tc.want, tc.wantOffset)
		}
	}
}

func w0At(d time.Duration) time.Time {
	return time.Date(2026, time.August, 15, 0, 0, 0, 0, time.UTC).Add(d)
}

func guideLine(bs []playout.Broadcast) []string {
	var out []string
	for _, b := range bs {
		out = append(out, fmt.Sprintf("%s %s–%s", b.Title, b.Start.Format("15:04"), b.Stop.Format("15:04")))
	}
	return out
}
