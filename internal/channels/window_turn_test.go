package channels_test

import (
	"slices"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/playout"
)

// Reconcile turns the rolling window with carry-over (#1675): the programme crossing the boundary
// finishes on the old arrangement, and the new window's slice is anchored at its end. Before the
// fix a reconcile just past the boundary swapped in the next slice under the fixed anchor, so the
// crossing programme was cut and the next slice was joined mid-programme.
//
// Twenty 100-minute films (a 33 h pool) on a 24 h window: the slice rotates every window and a
// film almost always straddles the boundary.
func TestReconcile_WindowTurnCarriesTheCrossingProgrammeOver(t *testing.T) {
	st := newStore(t)
	entries, avail := films(20, 100*time.Minute)
	opened := time.Unix(1_800_000_000, 0).UTC().Truncate(24 * time.Hour)
	now := opened.Add(3 * time.Hour)
	e := recencyEngine(st, avail, &now)
	seedShuffleChannel(t, st, "wt", entries)

	first := reconcileDesired(t, e, st, "wt")
	boundary := opened.Add(24 * time.Hour)
	crossing := playout.BroadcastsBetween(first.Desired, first.PlayoutAnchor, boundary, boundary.Add(time.Second))[0]
	if !crossing.Start.Before(boundary) || !crossing.Stop.After(boundary.Add(2*time.Minute)) {
		t.Fatalf("fixture: want a film straddling the boundary, got %s %v–%v", crossing.LibraryItemID, crossing.Start, crossing.Stop)
	}

	// Just past the boundary the crossing film is still on air: the window has not turned.
	now = boundary.Add(time.Minute)
	during := reconcileDesired(t, e, st, "wt")
	if !slices.Equal(programIDs(first.Desired), programIDs(during.Desired)) || !during.PlayoutAnchor.Equal(first.PlayoutAnchor) {
		t.Fatalf("reconcile replaced the arrangement while %s was still airing across the boundary", crossing.LibraryItemID)
	}
	if got := playout.AiringAt(during.Desired, during.PlayoutAnchor, now); got.LibraryItemID != crossing.LibraryItemID {
		t.Fatalf("on air at the boundary: %s, want the crossing film %s", got.LibraryItemID, crossing.LibraryItemID)
	}

	// Once it ends, the next reconcile commits the new slice anchored exactly at its end.
	now = crossing.Stop.Add(2 * time.Minute)
	turned := reconcileDesired(t, e, st, "wt")
	if !turned.PlayoutAnchor.Equal(crossing.Stop) {
		t.Fatalf("new window anchored at %v, want the crossing film's end %v", turned.PlayoutAnchor, crossing.Stop)
	}
	if slices.Equal(programIDs(first.Desired), programIDs(turned.Desired)) {
		t.Fatal("the window never turned: the next slice was not committed")
	}
	if got := playout.AiringAt(turned.Desired, turned.PlayoutAnchor, crossing.Stop); got.Offset != 0 || got.LibraryItemID != programIDs(turned.Desired)[0] {
		t.Fatalf("at the handoff %s is %v in, want the new slice's first programme from its start", got.LibraryItemID, got.Offset)
	}
}
