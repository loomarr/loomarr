package channels_test

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"

	"github.com/loomarr/loomarr/internal/channels"
	"github.com/loomarr/loomarr/internal/channels/schedule"
	"github.com/loomarr/loomarr/internal/library/provision"
	"github.com/loomarr/loomarr/internal/platform/store"
	"github.com/loomarr/loomarr/internal/playout/tunarr/tunarrtest"
)

// A reconcile that cannot read what the library holds does not know what the channel can air.
// It fails and leaves the channel as it was; it never treats the failed read as "not in the
// library", which would commit an empty lineup and push it.

// failingTitles is a title store that starts refusing reads when told to.
type failingTitles struct {
	channels.AvailabilityStore
	fail atomic.Bool
}

func (f *failingTitles) GetTitle(ctx context.Context, key provision.Key) (provision.Record, error) {
	if f.fail.Load() {
		return provision.Record{}, errors.New("database is locked")
	}
	return f.AvailabilityStore.GetTitle(ctx, key)
}

func seedAvailableTitle(t *testing.T, st store.Store, key provision.Key, title provision.Title, libraryID string) {
	t.Helper()
	rec := provision.Record{Key: key, Title: title, State: provision.Available, LibraryID: libraryID}
	if err := st.UpsertTitle(context.Background(), rec); err != nil {
		t.Fatalf("seed title %s: %v", key, err)
	}
}

func programmeCount(slots []schedule.Slot) int {
	n := 0
	for _, slot := range slots {
		if slot.Kind == schedule.SlotProgram {
			n++
		}
	}
	return n
}

func TestReconcileFailsAndKeepsTheLineupWhenATitleReadFails(t *testing.T) {
	ctx := context.Background()
	st := newStore(t)
	seedAvailableTitle(t, st, "movie:tmdb:90000001", provision.Title{MediaType: provision.Movie, TMDBID: 90000001}, "lib-1")
	seedAvailableTitle(t, st, "movie:tmdb:90000002", provision.Title{MediaType: provision.Movie, TMDBID: 90000002}, "lib-2")
	seedChannel(t, st, "c1", 5, entry("movie:tmdb:90000001", "Lantern Road"), entry("movie:tmdb:90000002", "Harbour Nine"))

	titles := &failingTitles{AvailabilityStore: st}
	tun := tunarrtest.NewTunarr()
	if err := newEngine(st, tun, channels.NewStoreAvailability(ctx, titles, nil, nil), nil).Reconcile(ctx, "c1"); err != nil {
		t.Fatalf("first reconcile: %v", err)
	}
	before, err := st.GetChannel(ctx, "c1")
	if err != nil {
		t.Fatal(err)
	}
	if programmeCount(before.Desired) != 2 {
		t.Fatalf("the channel starts with %d programmes, want 2", programmeCount(before.Desired))
	}
	pushesBefore := tun.Pushes

	// A fresh availability, so the short memo from the first reconcile cannot hide the failure.
	titles.fail.Store(true)
	err = newEngine(st, tun, channels.NewStoreAvailability(ctx, titles, nil, nil), nil).Reconcile(ctx, "c1")

	if err == nil {
		t.Fatal("reconcile reported success although it could not read the channel's titles")
	}
	after, getErr := st.GetChannel(ctx, "c1")
	if getErr != nil {
		t.Fatal(getErr)
	}
	if programmeCount(after.Desired) != 2 || after.Status != before.Status {
		t.Errorf("the channel changed: %d programmes and status %q, want 2 and %q",
			programmeCount(after.Desired), after.Status, before.Status)
	}
	if tun.Pushes != pushesBefore {
		t.Errorf("the lineup was pushed %d more time(s) after the failed read", tun.Pushes-pushesBefore)
	}
}

func TestReconcileFailsAndKeepsTheLineupWhenASeriesEpisodeReadFails(t *testing.T) {
	ctx := context.Background()
	st := newStore(t)
	seedAvailableTitle(t, st, "series:tvdb:90000701", provision.Title{MediaType: provision.Series, TVDBID: 90000701}, "lib-show")
	seedChannel(t, st, "c1", 5, entry("series:tvdb:90000701", "Quiet Harbour"))

	var failEpisodes atomic.Bool
	episodes := func(context.Context, string) ([]schedule.ResolvedProgram, error) {
		if failEpisodes.Load() {
			return nil, errors.New("database is locked")
		}
		return []schedule.ResolvedProgram{
			{LibraryItemID: "ep-1", Title: "One", DurationMs: 1_320_000, Season: 1, Episode: 1},
			{LibraryItemID: "ep-2", Title: "Two", DurationMs: 1_320_000, Season: 1, Episode: 2},
		}, nil
	}
	tun := tunarrtest.NewTunarr()
	if err := newEngine(st, tun, channels.NewStoreAvailability(ctx, st, nil, episodes), nil).Reconcile(ctx, "c1"); err != nil {
		t.Fatalf("first reconcile: %v", err)
	}
	before, err := st.GetChannel(ctx, "c1")
	if err != nil {
		t.Fatal(err)
	}
	if programmeCount(before.Desired) == 0 {
		t.Fatal("the channel starts with no programmes; the series did not resolve")
	}
	pushesBefore := tun.Pushes

	failEpisodes.Store(true)
	err = newEngine(st, tun, channels.NewStoreAvailability(ctx, st, nil, episodes), nil).Reconcile(ctx, "c1")

	if err == nil {
		t.Fatal("reconcile reported success although it could not read the series' episodes")
	}
	after, getErr := st.GetChannel(ctx, "c1")
	if getErr != nil {
		t.Fatal(getErr)
	}
	if programmeCount(after.Desired) != programmeCount(before.Desired) {
		t.Errorf("the channel went from %d programmes to %d", programmeCount(before.Desired), programmeCount(after.Desired))
	}
	if tun.Pushes != pushesBefore {
		t.Errorf("the lineup was pushed %d more time(s) after the failed read", tun.Pushes-pushesBefore)
	}
}

// A title the library has no record of is an answer, not a failure: the reconcile goes on and
// the channel simply does not air it.
func TestReconcileTreatsATitleWithNoRecordAsNotInTheLibrary(t *testing.T) {
	ctx := context.Background()
	st := newStore(t)
	seedAvailableTitle(t, st, "movie:tmdb:90000001", provision.Title{MediaType: provision.Movie, TMDBID: 90000001}, "lib-1")
	seedChannel(t, st, "c1", 5, entry("movie:tmdb:90000001", "Lantern Road"), entry("movie:tmdb:90000009", "Never Fetched"))

	if err := newEngine(st, tunarrtest.NewTunarr(), channels.NewStoreAvailability(ctx, st, nil, nil), nil).Reconcile(ctx, "c1"); err != nil {
		t.Fatalf("reconcile with one unknown title: %v", err)
	}
	ch, err := st.GetChannel(ctx, "c1")
	if err != nil {
		t.Fatal(err)
	}
	if programmeCount(ch.Desired) != 1 {
		t.Errorf("the channel airs %d programmes, want the one title the library holds", programmeCount(ch.Desired))
	}
}
