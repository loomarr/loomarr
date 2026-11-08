package store

import (
	"context"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/inventory"
)

// The Library's rows follow inventory writes: a snapshot writes its item's row, an origin marked
// unseen projects its item again, and an item the projector does not list keeps the row it had.
func testLibraryProjection(t *testing.T, newStore NewStoreFunc) {
	ctx := context.Background()
	st := newStore(t)
	at := time.UnixMilli(1_790_000_000_000).UTC()
	// The stub lists an item under its first origin's name, and notes whether that origin is
	// present; an item named "Unlisted" is not listed.
	st.ProjectLibraryWith(func(item inventory.Item) (LibraryItem, bool) {
		o := item.Origins[0]
		if o.Observation.Facts.Name == "Unlisted" {
			return LibraryItem{}, false
		}
		name := o.Observation.Facts.Name
		if !o.MissingAt.IsZero() {
			name += " (gone)"
		}
		return LibraryItem{ID: string(item.ID), Kind: "movie", Name: name,
			Origin: LibraryOrigin{Authority: string(o.Key.Authority), Key: o.Key.ExternalItemID}}, true
	})
	name := func(id inventory.ItemID) string {
		t.Helper()
		it, err := st.LibraryItem(ctx, string(id))
		if err != nil {
			t.Fatalf("library item %s: %v", id, err)
		}
		return it.Name
	}

	snap := inventorySnapshot("library-a", "item-1", "media-1", "rev-1", "Pilot", at)
	id, err := st.ApplyInventorySnapshot(ctx, snap)
	if err != nil {
		t.Fatal(err)
	}
	// A title is read with a present file.
	if err := st.SaveLibraryFiles(ctx, []LibraryFile{{Path: "/media/p/p.mkv", Root: "/media", Size: 1, ModifiedNs: 1,
		Identity: "digest", Kind: "movie", Header: LibraryHeaderRead, SeenAt: at, ItemID: string(id)}}); err != nil {
		t.Fatal(err)
	}
	if got := name(id); got != "Pilot" {
		t.Fatalf("after a snapshot, name = %q, want Pilot", got)
	}

	// A write the projector declines leaves the row as it was.
	unlisted := inventorySnapshot("library-a", "item-1", "media-1", "rev-1", "Unlisted", at.Add(time.Minute))
	if _, err := st.ApplyInventorySnapshot(ctx, unlisted); err != nil {
		t.Fatal(err)
	}
	if got := name(id); got != "Pilot" {
		t.Fatalf("after a declined projection, name = %q, want Pilot kept", got)
	}

	renamed := inventorySnapshot("library-a", "item-1", "media-1", "rev-1", "Pilot Redux", at.Add(2*time.Minute))
	if _, err := st.ApplyInventorySnapshot(ctx, renamed); err != nil {
		t.Fatal(err)
	}
	if err := st.MarkInventoryUnseen(ctx, "library-a", at.Add(time.Hour), nil); err != nil {
		t.Fatal(err)
	}
	if got := name(id); got != "Pilot Redux (gone)" {
		t.Fatalf("after its origin went missing, name = %q, want it projected again", got)
	}
}
