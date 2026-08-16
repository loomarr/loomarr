package reconcile

import (
	"context"
	"errors"
	"log/slog"
	"testing"
	"time"

	"github.com/mantonx/loomarr/internal/provision"
	"github.com/mantonx/loomarr/internal/schedule"
	"github.com/mantonx/loomarr/internal/store"
	"github.com/mantonx/loomarr/internal/testkit"
	"github.com/mantonx/loomarr/internal/testkit/libraryfixture"
)

func TestEpisodeRefreshSnapshotsResolverOnceAcrossRun(t *testing.T) {
	ctx := context.Background()
	st := testkit.SQLiteStore(t)
	now := time.Date(2026, 8, 16, 12, 0, 0, 0, time.UTC)

	shows := []struct {
		key       provision.Key
		libraryID string
	}{
		{key: provision.Key("series:tvdb:101"), libraryID: "show-101"},
		{key: provision.Key("series:tvdb:202"), libraryID: "show-202"},
	}
	lineup := make([]schedule.LineupEntry, 0, len(shows))
	for _, show := range shows {
		if err := st.UpsertTitle(ctx, provision.Record{
			Key: show.key, Title: provision.Title{MediaType: provision.Series},
			State: provision.Available, LibraryID: show.libraryID,
		}); err != nil {
			t.Fatal(err)
		}
		if err := st.UpsertSeriesEpisodes(ctx, store.SeriesEpisodes{
			LibraryID: show.libraryID,
			FetchedAt: now.Add(-48 * time.Hour),
		}); err != nil {
			t.Fatal(err)
		}
		lineup = append(lineup, schedule.LineupEntry{Key: show.key})
	}
	if _, err := st.SaveChannel(ctx, store.Channel{
		Channel: schedule.Channel{
			ID: "channel-1", Name: "Channel 1", Number: 1,
			Strategy: schedule.Sequential, Status: schedule.StatusBuilding,
		},
		Lineup: lineup,
	}); err != nil {
		t.Fatal(err)
	}

	rotatedErr := errors.New("rotated resolver must not serve an in-flight run")
	snapshots := 0
	primaryResults := make(map[string][]schedule.ResolvedProgram, len(shows))
	for _, show := range shows {
		primaryResults[show.libraryID] = []schedule.ResolvedProgram{{LibraryItemID: show.libraryID + "-episode"}}
	}
	primary := libraryfixture.NewEpisodes(primaryResults)
	rotated := libraryfixture.NewEpisodes(nil)
	rotated.Err = rotatedErr
	current := EpisodeResolver(primary.Resolve)
	refresh := NewDynamicEpisodeRefresh(st, func() EpisodeResolver {
		snapshots++
		snapshot := current
		current = rotated.Resolve // settings rotate after this operation binds its resolver
		return snapshot
	}, func() time.Duration { return 24 * time.Hour }, func() time.Time { return now }, slog.New(slog.DiscardHandler))

	refreshed, err := refresh.Run(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if refreshed != len(shows) {
		t.Fatalf("refreshed = %d, want %d", refreshed, len(shows))
	}
	if snapshots != 1 {
		t.Fatalf("resolver snapshots = %d, want one for the run", snapshots)
	}
	if calls := primary.Calls(); len(calls) != len(shows) {
		t.Fatalf("primary resolver calls = %v, want both shows", calls)
	}
	if calls := rotated.Calls(); len(calls) != 0 {
		t.Fatalf("rotated resolver calls = %v, want none", calls)
	}

	// A later operation does see the rotation. Advance the clock far enough that the two
	// rows written above are stale again; the rotated resolver fails both without changing
	// the successful, operation-wide snapshot used by the first run.
	now = now.Add(48 * time.Hour)
	refreshed, err = refresh.Run(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if refreshed != 0 {
		t.Fatalf("refreshed after rotation = %d, want zero", refreshed)
	}
	if snapshots != 2 {
		t.Fatalf("resolver snapshots after second run = %d, want two", snapshots)
	}
	if calls := rotated.Calls(); len(calls) != len(shows) {
		t.Fatalf("rotated resolver calls after second run = %v, want %d", calls, len(shows))
	}
}
