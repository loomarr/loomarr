package store

import (
	"context"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/channels/schedule"
	"github.com/loomarr/loomarr/internal/platform/store/clipstore"
)

// A channel's planned breaks (#3838 step 2) round-trip with their pod as stored, a second plan at
// the same start replaces the first, a read is bounded to its window and its channel, keeping a set
// of starts deletes the rest, and the breaks go with their channel.
func testFillerBreaks(t *testing.T, newStore NewStoreFunc) {
	ctx := context.Background()
	st := newStore(t)
	saved := map[string]int64{}
	for i, id := range []string{"ch-a", "ch-b"} {
		ch, err := st.SaveChannel(ctx, Channel{ID: id, Name: id, Number: 80 + i, Strategy: schedule.Shuffle,
			Status: schedule.StatusLive})
		if err != nil {
			t.Fatal(err)
		}
		saved[id] = ch.Revision
	}
	at := time.Unix(1_800_000_000, 0).UTC()
	pod := `{"entries":[{"hash":"h1","path":"a.mp4","duration_ms":30000}]}`
	brk := func(ch string, start time.Duration, length time.Duration) clipstore.FillerBreak {
		return clipstore.FillerBreak{ChannelID: ch, Start: at.Add(start), Ceiling: 2 * time.Minute, Length: length,
			PodJSON: pod, PlannedAt: at}
	}
	if err := st.SaveFillerBreaks(ctx, []clipstore.FillerBreak{
		brk("ch-a", 0, 30*time.Second), brk("ch-a", 30*time.Minute, 0), brk("ch-a", time.Hour, 30*time.Second),
		brk("ch-b", 0, 30*time.Second),
	}); err != nil {
		t.Fatal(err)
	}
	// A plan at a start already stored replaces it.
	again := brk("ch-a", 0, 95*time.Second)
	again.Ceiling = 100 * time.Second
	if err := st.SaveFillerBreaks(ctx, []clipstore.FillerBreak{again}); err != nil {
		t.Fatal(err)
	}

	got, err := st.ListFillerBreaks(ctx, "ch-a", at, at.Add(30*time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || !got[0].Start.Equal(at) || !got[1].Start.Equal(at.Add(30*time.Minute)) {
		t.Fatalf("breaks in the first half hour = %+v, want the two at 0 and 30 minutes, earliest first", got)
	}
	if got[0].Ceiling != 100*time.Second || got[0].Length != 95*time.Second || got[1].Length != 0 ||
		!got[0].PlannedAt.Equal(at) || got[0].ChannelID != "ch-a" {
		t.Errorf("first break %+v, want the replacing plan (100s ceiling, 95s long) and the empty one kept at 0", got[0])
	}
	if got[0].PodJSON == "" {
		t.Error("the pod came back empty")
	}

	// Keeping within a range clears only that range: [0, 1h) with nothing kept leaves the hour mark.
	if err := st.KeepFillerBreaks(ctx, "ch-a", at.Add(-time.Hour), at.Add(time.Hour), nil); err != nil {
		t.Fatal(err)
	}
	if left, _ := st.ListFillerBreaks(ctx, "ch-a", at, at.Add(2*time.Hour)); len(left) != 1 || !left[0].Start.Equal(at.Add(time.Hour)) {
		t.Errorf("after clearing the first hour: %+v, want only the break at the hour mark", left)
	}
	// An open range keeps one start, deletes the channel's others, and leaves the other channel alone.
	if err := st.SaveFillerBreaks(ctx, []clipstore.FillerBreak{brk("ch-a", 0, 30*time.Second)}); err != nil {
		t.Fatal(err)
	}
	if err := st.KeepFillerBreaks(ctx, "ch-a", time.Time{}, time.Time{}, []time.Time{at.Add(time.Hour)}); err != nil {
		t.Fatal(err)
	}
	if left, _ := st.ListFillerBreaks(ctx, "ch-a", at, at.Add(2*time.Hour)); len(left) != 1 || !left[0].Start.Equal(at.Add(time.Hour)) {
		t.Errorf("after keeping the hour mark: %+v", left)
	}
	if other, _ := st.ListFillerBreaks(ctx, "ch-b", at, at); len(other) != 1 {
		t.Errorf("keeping ch-a's breaks touched ch-b's: %+v", other)
	}
	// Keeping nothing in an open range clears the channel.
	if err := st.KeepFillerBreaks(ctx, "ch-a", time.Time{}, time.Time{}, nil); err != nil {
		t.Fatal(err)
	}
	if left, _ := st.ListFillerBreaks(ctx, "ch-a", at, at.Add(2*time.Hour)); len(left) != 0 {
		t.Errorf("keeping nothing left %+v", left)
	}

	// The breaks go with their channel.
	if err := st.DeleteChannel(ctx, "ch-b", saved["ch-b"]); err != nil {
		t.Fatal(err)
	}
	if left, _ := st.ListFillerBreaks(ctx, "ch-b", at, at); len(left) != 0 {
		t.Errorf("a deleted channel's breaks are still stored: %+v", left)
	}
}
