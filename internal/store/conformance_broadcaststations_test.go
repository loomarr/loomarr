package store

import (
	"context"
	"reflect"
	"slices"
	"testing"

	"github.com/loomarr/loomarr/internal/broadcaststation"
)

// The broadcast station directory (#2060): a fresh store holds exactly the shipped seed.

func testBroadcastStationsSeededOnOpen(t *testing.T, newStore NewStoreFunc) {
	t.Helper()
	s := newStore(t)
	got, err := s.ListBroadcastStations(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	revisions := broadcaststation.SeedRevisions()
	want, err := revisions[len(revisions)-1].Load()
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("a fresh store's stations are not the shipped seed: %d stored, %d shipped", len(got), len(want))
	}
	// Cleveland's channel 3 was KYW from 1956 to 1965, before Philadelphia's: the history an archive
	// title from 1962 resolves against.
	i := slices.IndexFunc(got, func(s broadcaststation.Station) bool { return s.ID == "fcc:73195" })
	if i < 0 {
		t.Fatal("WKYC (fcc:73195) is missing from the stored directory")
	}
	if !slices.Contains(got[i].CallSigns, broadcaststation.CallSign{Sign: "KYW", From: "1956-01-01", To: "1965-01-01"}) {
		t.Fatalf("WKYC's stored call signs %+v lack KYW 1956-1965", got[i].CallSigns)
	}
}
