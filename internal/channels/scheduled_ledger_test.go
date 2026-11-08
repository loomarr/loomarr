package channels_test

import (
	"context"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/channels/schedule"
	"github.com/loomarr/loomarr/internal/playout"
)

// Reconcile records what the accepted cycle put on air, watched or not (#1677), so a window that
// opens on a channel nobody watched still knows what the window before it aired. Before this, the
// only history was the watched airings, and the curation sim had films re-airing within hours.
func TestReconcile_RecordsTheScheduledAiringsNobodyWatched(t *testing.T) {
	st := newStore(t)
	entries, avail := films(40, 100*time.Minute) // a 67 h pool on a 24 h window
	opened := time.Unix(1_800_000_000, 0).UTC().Truncate(24 * time.Hour)
	now := opened.Add(time.Hour)
	e := recencyEngine(st, avail, &now)
	seedShuffleChannel(t, st, "sl", entries)

	first := reconcileDesired(t, e, st, "sl")
	// Hourly sweeps through the window and past the turn, with no viewer at all.
	var turned bool
	var last = first
	for now = now.Add(time.Hour); now.Before(opened.Add(30 * time.Hour)); now = now.Add(time.Hour) {
		last = reconcileDesired(t, e, st, "sl")
		if !last.PlayoutAnchor.Equal(first.PlayoutAnchor) {
			turned = true
			break
		}
	}
	if !turned {
		t.Fatal("fixture: the window never turned")
	}
	newOpened := last.PlayoutAnchor

	// Every programme the first window aired before it handed over is in the scheduled ledger as of
	// the new window's opening, and nothing is in the watched history.
	aired := playout.BroadcastsBetween(first.Desired, first.PlayoutAnchor, first.PlayoutAnchor, newOpened)
	ledger, err := st.LastScheduledByChannel(context.Background(), "sl", newOpened)
	if err != nil {
		t.Fatal(err)
	}
	n := 0
	for _, b := range aired {
		if b.Kind != schedule.SlotProgram || b.LibraryItemID == "" {
			continue
		}
		n++
		if got, ok := ledger[b.LibraryItemID]; !ok || !got.Equal(b.Start) {
			t.Errorf("%s aired at %v but the ledger as of the new window has %v (present %v)", b.LibraryItemID, b.Start, got, ok)
		}
	}
	if n == 0 {
		t.Fatal("fixture: the first window aired nothing")
	}
	if watched, _ := st.LastAiredByChannel(context.Background(), "sl", newOpened.Add(time.Hour)); len(watched) != 0 {
		t.Errorf("watched history = %v, want it untouched by the schedule", watched)
	}

	// The new window does not open on anything the window before it aired.
	for _, b := range playout.BroadcastsBetween(last.Desired, last.PlayoutAnchor, newOpened, newOpened.Add(24*time.Hour)) {
		if at, ok := ledger[b.LibraryItemID]; ok && b.Kind == schedule.SlotProgram && b.Start.Sub(at) < 24*time.Hour {
			t.Errorf("%s re-airs at %v, %v after its scheduled airing", b.LibraryItemID, b.Start, b.Start.Sub(at))
		}
	}
}
