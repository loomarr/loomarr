package store

import (
	"reflect"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/broadcaststation"
)

func TestBroadcastStationSeedRevisionReplacesTheDirectory(t *testing.T) {
	s := newSQLiteStore(t).(*sqlStore)
	ctx := t.Context()
	at := time.Unix(1_700_000_000, 0).UTC()
	corrected := []broadcaststation.Station{{
		ID: "fcc:59441", Country: "US", City: "Cleveland", Region: "OH", Latitude: 41.47926, Longitude: -81.68022,
		CallSigns: []broadcaststation.CallSign{{Sign: "WEWS", From: "1947-12-17"}},
	}}
	revisions := append(broadcaststation.SeedRevisions(),
		broadcaststation.SeedRevision{Version: 99, Load: func() ([]broadcaststation.Station, error) { return corrected, nil }})

	if err := s.convergeBroadcastStationSeed(ctx, revisions, at); err != nil {
		t.Fatal(err)
	}
	got, err := s.ListBroadcastStations(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, corrected) {
		t.Fatalf("after a newer revision the directory = %d stations, want exactly the revision's one", len(got))
	}

	// Applied revisions never re-run, and an older one never replaces a newer directory.
	loads := 0
	again := []broadcaststation.SeedRevision{{Version: 1, Load: func() ([]broadcaststation.Station, error) {
		loads++
		return nil, nil
	}}}
	if err := s.convergeBroadcastStationSeed(ctx, again, at); err != nil {
		t.Fatal(err)
	}
	if err := s.convergeBroadcastStationSeed(ctx, revisions, at); err != nil {
		t.Fatal(err)
	}
	if loads != 0 {
		t.Fatal("an older revision was loaded after a newer one applied")
	}
	if got, _ = s.ListBroadcastStations(ctx); !reflect.DeepEqual(got, corrected) {
		t.Fatal("re-converging changed an applied directory")
	}
}
