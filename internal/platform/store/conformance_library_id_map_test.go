package store

import (
	"context"
	"testing"
	"time"
)

// A pass replaces the whole id map: ids it no longer lists are gone, and a gap keeps its reason.
func testLibraryIDMap(t *testing.T, newStore NewStoreFunc) {
	ctx := context.Background()
	st := newStore(t)
	if got, err := st.LibraryIDMap(ctx); err != nil || got == nil || len(got) != 0 {
		t.Fatalf("empty map = %#v, %v; want a non-nil empty slice", got, err)
	}
	at := time.UnixMilli(1_800_000_000_000)
	first := []LibraryIDMapping{
		{MediaID: "90000002", Kind: "episode", ItemID: "item-e"},
		{MediaID: "90000001", Kind: "movie", ItemID: "item-m"},
		{MediaID: "90000003", Kind: "series", Reason: "its episodes name two shows"},
	}
	if err := st.ReplaceLibraryIDMap(ctx, first, at); err != nil {
		t.Fatal(err)
	}
	got, err := st.LibraryIDMap(ctx)
	if err != nil || len(got) != 3 || got[0] != first[1] || got[1] != first[0] || got[2] != first[2] {
		t.Fatalf("map = %+v, %v; want the three rows ordered by media id", got, err)
	}
	second := []LibraryIDMapping{{MediaID: "90000001", Kind: "movie", ItemID: "item-m2"}}
	if err := st.ReplaceLibraryIDMap(ctx, second, at.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	if got, err := st.LibraryIDMap(ctx); err != nil || len(got) != 1 || got[0] != second[0] {
		t.Fatalf("after a second pass, map = %+v, %v; want only its row", got, err)
	}
}
