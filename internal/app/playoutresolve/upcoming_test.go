package playoutresolve

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/channels/schedule"
	"github.com/loomarr/loomarr/internal/playout/timeline"
)

// The stored upcoming airings (#4476) are the guide's programmes. Read before a window turn, they
// match the guide, the next window at the break lengths reconcile planned for it, and only that
// window and later ones are forecasts. After the turn, with no reconcile in between, the guide's
// window on air is still what was stored for it. The day window is in a DST zone, on the 25-hour
// day the clocks go back.
func TestUpcomingMatchesGuideAcrossTurn(t *testing.T) {
	t.Parallel()
	zone, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Skip("no tzdata:", err)
	}
	ctx := context.Background()
	w0 := time.Date(2026, time.November, 1, 0, 0, 0, 0, zone)
	decks := &windowDecks{
		window: 24 * time.Hour, zone: zone, episodes: 24, episode: time.Hour, label: "Jan02",
		breakLen: 5 * time.Minute,
	}
	decks.planNextWindow(w0, 2*time.Minute)
	_, committed, _, _, _ := decks.CyclePreview(ctx, "ch1", w0)
	turn := timeline.CarryOverEnd(committed, w0, schedule.NextWindowStart(w0, decks.window, zone))

	now := turn.Add(-3 * time.Hour)
	r := deckResolver(decks, w0, now)
	r.now = func() time.Time { return now }

	stored, err := r.UpcomingProgrammes(ctx, "ch1", now, now.Add(48*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	guide, err := r.BroadcastsBetween(ctx, "ch1", now, now.Add(48*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if got, want := upcomingLine(stored), guideProgrammes(guide); fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("before the turn, stored\n%v\nwant the guide's\n%v", got, want)
	}
	beforeTurn, afterTurn := 0, 0
	for _, p := range stored {
		if wantForecast := !p.Start.Before(turn); p.Forecast != wantForecast {
			t.Errorf("%s at %s forecast = %v, want %v (turn %s)", p.LibraryItemID, p.Start.Format(time.DateTime),
				p.Forecast, wantForecast, turn.Format(time.DateTime))
		}
		if p.Start.Before(turn) {
			beforeTurn++
		} else {
			afterTurn++
		}
	}
	if beforeTurn < 2 || afterTurn < 2 {
		t.Fatalf("stored %d programmes before the turn and %d after; want both sides covered", beforeTurn, afterTurn)
	}

	// After the turn, with no reconcile, the window on air is what was stored for it.
	now = turn.Add(2 * time.Hour)
	onAir, err := r.UpcomingProgrammes(ctx, "ch1", now, now.Add(48*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	var live []UpcomingProgramme
	for _, p := range onAir {
		if !p.Forecast {
			live = append(live, p)
		}
	}
	if len(live) < 4 {
		t.Fatalf("after the turn %d programmes are on air, want the new window: %v", len(live), upcomingLine(onAir))
	}
	end := live[len(live)-1].Stop
	var storedLive []UpcomingProgramme
	for _, p := range stored {
		if p.Stop.After(now) && !p.Start.Before(turn) && !p.Stop.After(end) {
			storedLive = append(storedLive, p)
		}
	}
	if got, want := upcomingLine(live), upcomingLine(storedLive); fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("after the turn, the window on air\n%v\nwant what was stored for it\n%v", got, want)
	}
	guideAfter, err := r.BroadcastsBetween(ctx, "ch1", now, end)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := upcomingLine(live), guideProgrammes(guideAfter); fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("after the turn, the window on air\n%v\nwant the guide's\n%v", got, want)
	}
}

func upcomingLine(ps []UpcomingProgramme) []string {
	out := make([]string, 0, len(ps))
	for _, p := range ps {
		out = append(out, fmt.Sprintf("%s %s %s–%s", p.LibraryItemID, p.Key, p.Start.Format(time.DateTime), p.Stop.Format(time.DateTime)))
	}
	return out
}

// guideProgrammes is the guide's programmes with a library item, in the same shape as upcomingLine.
func guideProgrammes(bs []timeline.Broadcast) []string {
	out := []string{}
	for _, b := range bs {
		if b.Kind != schedule.SlotProgram || b.LibraryItemID == "" || b.Nominal {
			continue
		}
		out = append(out, fmt.Sprintf("%s %s %s–%s", b.LibraryItemID, b.Key, b.Start.Format(time.DateTime), b.Stop.Format(time.DateTime)))
	}
	return out
}
