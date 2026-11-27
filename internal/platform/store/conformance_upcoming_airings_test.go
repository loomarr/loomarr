package store

import (
	"context"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/channels/schedule"
	"github.com/loomarr/loomarr/internal/library/provision"
)

// testUpcomingAirings: a channel's upcoming airings (#4476) are replaced whole and only at the
// channel's current revision; the read finds a movie by its id and a series through its title key,
// includes the programme on air, and skips finished rows and paused or detached channels; purging
// the channel deletes its rows.
func testUpcomingAirings(t *testing.T, newStore NewStoreFunc) {
	s := newStore(t)
	ctx := context.Background()
	now := time.UnixMilli(1_800_000_000_000).UTC()
	const movieKey, seriesKey = provision.Key("movie:tmdb:90000031"), provision.Key("series:tvdb:90000032")

	if err := s.SaveLibraryItems(ctx, []LibraryItem{
		{ID: "m1", Kind: "movie", Name: "Upcoming Movie"},
		{ID: "s1", Kind: "series", Name: "Upcoming Series"},
		{ID: "ep-7", Kind: "episode", Name: "Upcoming Episode 7"},
		{ID: "ep-8", Kind: "episode", Name: "Upcoming Episode 8"},
	}, now); err != nil {
		t.Fatal(err)
	}
	if err := s.UpsertTitle(ctx, provision.Record{Key: movieKey, State: provision.Available, LibraryID: "m1",
		Title: provision.Title{MediaType: provision.Movie, TMDBID: 90000031, Name: "Upcoming Movie"}}); err != nil {
		t.Fatal(err)
	}
	if err := s.UpsertTitle(ctx, provision.Record{Key: seriesKey, State: provision.Available, LibraryID: "s1",
		Title: provision.Title{MediaType: provision.Series, TVDBID: 90000032, Name: "Upcoming Series"}}); err != nil {
		t.Fatal(err)
	}

	at := func(min int) time.Time { return now.Add(time.Duration(min) * time.Minute) }
	airing := func(item string, key provision.Key, from, to int, forecast bool) UpcomingAiring {
		return UpcomingAiring{Start: at(from), End: at(to), LibraryItemID: item, TitleKey: key, Forecast: forecast}
	}
	a := mustSaveChannel(t, s, sampleChannel("ch-up-a", 31, time.Time{}))
	b := mustSaveChannel(t, s, sampleChannel("ch-up-b", 32, time.Time{}))

	replace := func(ch Channel, airings ...UpcomingAiring) bool {
		t.Helper()
		wrote, err := s.ReplaceUpcomingAirings(ctx, ch.ID, ch.Revision, airings)
		if err != nil {
			t.Fatal(err)
		}
		return wrote
	}
	read := func(item string) []UpcomingAiring {
		t.Helper()
		got, err := s.UpcomingAiringsForItem(ctx, item, now)
		if err != nil {
			t.Fatal(err)
		}
		return got
	}
	same := func(got []UpcomingAiring, want ...UpcomingAiring) bool {
		if len(got) != len(want) {
			return false
		}
		for i := range got {
			g, w := got[i], want[i]
			if g.ChannelID != w.ChannelID || g.ChannelName != w.ChannelName || g.ChannelNumber != w.ChannelNumber || !g.Start.Equal(w.Start) || !g.End.Equal(w.End) ||
				g.LibraryItemID != w.LibraryItemID || g.TitleKey != w.TitleKey || g.Forecast != w.Forecast {
				return false
			}
		}
		return true
	}
	on := func(ch Channel, x UpcomingAiring) UpcomingAiring {
		x.ChannelID, x.ChannelName, x.ChannelNumber = ch.ID, ch.Name, ch.Number
		return x
	}

	// A replace writes the rows; the movie's finished row is not read, its row on air is.
	finished := airing("m1", movieKey, -120, -10, false)
	onAir := airing("m1", movieKey, -10, 80, false)
	ep := airing("ep-7", seriesKey, 80, 110, false)
	later := airing("m1", movieKey, 600, 690, true)
	if !replace(a, finished, onAir, ep, later) {
		t.Fatal("replace at the current revision wrote nothing")
	}
	epB := airing("ep-8", seriesKey, 30, 60, true)
	if !replace(b, epB) {
		t.Fatal("replace on channel b wrote nothing")
	}
	if got := read("m1"); !same(got, on(a, onAir), on(a, later)) {
		t.Fatalf("movie m1 = %+v; want the row on air then the forecast one, both on ch-up-a", got)
	}
	// A series finds its episodes across channels through its title key, earliest first.
	if got := read("s1"); !same(got, on(b, epB), on(a, ep)) {
		t.Fatalf("series s1 = %+v; want ep-8 on ch-up-b then ep-7 on ch-up-a", got)
	}
	// An episode's own id finds it too.
	if got := read("ep-7"); !same(got, on(a, ep)) {
		t.Fatalf("episode ep-7 = %+v; want its one row", got)
	}

	// The replace is whole: rows the new set leaves out are gone.
	if !replace(a, later) {
		t.Fatal("second replace at the current revision wrote nothing")
	}
	if got := read("s1"); !same(got, on(b, epB)) {
		t.Fatalf("after replace, series s1 = %+v; want only ch-up-b's episode", got)
	}

	// A write from a stale revision changes nothing.
	a.Status = schedule.StatusLive
	a2 := mustSaveChannel(t, s, a)
	if wrote, err := s.ReplaceUpcomingAirings(ctx, a.ID, a.Revision, []UpcomingAiring{onAir}); err != nil || wrote {
		t.Fatalf("stale replace = %v, %v; want no write, no error", wrote, err)
	}
	if got := read("m1"); !same(got, on(a, later)) {
		t.Fatalf("after a stale replace, movie m1 = %+v; want the earlier rows kept", got)
	}
	if wrote, err := s.ReplaceUpcomingAirings(ctx, "ch-missing", 1, []UpcomingAiring{onAir}); err != nil || wrote {
		t.Fatalf("replace on an unknown channel = %v, %v; want no write, no error", wrote, err)
	}

	// A paused or detached channel's rows are not read; resuming brings them back.
	if !replace(a2, onAir) {
		t.Fatal("replace at the new revision wrote nothing")
	}
	for _, status := range []schedule.ChannelStatus{schedule.StatusPaused, schedule.StatusDetached} {
		a2.Status = status
		a2 = mustSaveChannel(t, s, a2)
		if got := read("m1"); len(got) != 0 {
			t.Fatalf("%s channel's movie rows = %+v; want none", status, got)
		}
	}
	a2.Status = schedule.StatusLive
	a2 = mustSaveChannel(t, s, a2)
	if got := read("m1"); !same(got, on(a2, onAir)) {
		t.Fatalf("resumed channel's movie m1 = %+v; want the row on air", got)
	}

	// An airing of an item the catalog doesn't hold is left out; the others are written.
	if !replace(a2, airing("90000933", seriesKey, 5, 20, false), onAir) {
		t.Fatal("replace naming an unknown item wrote nothing")
	}
	if got := read("90000933"); len(got) != 0 {
		t.Fatalf("unknown item's rows = %+v; want none", got)
	}
	if got := read("m1"); !same(got, on(a2, onAir)) {
		t.Fatalf("beside an unknown item, movie m1 = %+v; want its row", got)
	}

	// Purging the channel deletes its rows.
	if err := s.DeleteChannel(ctx, a2.ID, a2.Revision); err != nil {
		t.Fatal(err)
	}
	if got := read("m1"); len(got) != 0 {
		t.Fatalf("after purge, movie m1 = %+v; want none", got)
	}
	if !replace(b) {
		t.Fatal("empty replace on channel b wrote nothing")
	}
	if got := read("s1"); len(got) != 0 {
		t.Fatalf("after an empty replace, series s1 = %+v; want none", got)
	}
}
