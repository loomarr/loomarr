package store

import (
	"context"
	"maps"
	"slices"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/platform/store/librarystore"
)

// A title's page reads back what was written (#4476): a movie's facts and their providers, each
// source with its edition, part and whether it plays (an extra copy does not), its tracks in index
// order with a subtitle file beside it last, a source that plays though its file has gone from the
// list, a movie in parts, and a series' episodes by the items its present files were written as
// (an unnumbered one among them, a missing file's left out), with no episode's sources read.
func testLibraryItemDetail(t *testing.T, newStore NewStoreFunc) {
	ctx := context.Background()
	st := newStore(t)
	at := time.UnixMilli(1_800_000_000_000)
	streams := []librarystore.LibraryStream{
		{Index: 0, Kind: "video", Width: 3840, Height: 2160, HDR: "HDR10"},
		{Index: 1, Kind: "audio", Channels: 2, Language: "en"},
		{Index: 2, Kind: "audio", Channels: 6, Language: "en", Default: true},
		{Index: 3, Kind: "subtitle", Language: "en"},
		{Index: 4, Kind: "subtitle", Language: "es", ExternalRef: "/m/film/film.es.srt"},
	}
	items := []LibraryItem{
		{ID: "film", Kind: "movie", Name: "Film", Overview: "A film.", Rating: "PG", RuntimeMs: 5_400_000,
			Genres: []string{"Drama", "Comedy"},
			From:   librarystore.LibraryFrom{Name: "nfo", Overview: "tmdb", Rating: "radarr", Year: "nfo"},
			Sources: []librarystore.LibrarySource{
				{Path: "/m/film/film.mkv", DurationMs: 5_432_100, Plays: true, Streams: streams},
				{Path: "/m/film/film dvd.mkv", DurationMs: 5_400_000, Streams: streams[:2]},
				{Path: "/m/film/film {edition-Extended}.mkv", DurationMs: 6_000_000, Edition: "Extended", Plays: true},
			}},
		{ID: "parts", Kind: "movie", Name: "Parts", Sources: []librarystore.LibrarySource{
			{Path: "/m/parts/parts cd2.mkv", DurationMs: 3_000_000, Part: 2, Plays: true},
			{Path: "/m/parts/parts cd1.mkv", DurationMs: 3_100_000, Part: 1, Plays: true},
		}},
		{ID: "held", Kind: "movie", Name: "Held", Sources: []librarystore.LibrarySource{
			{Path: "/m/held/a.mkv", Plays: true}, {Path: "/m/held/b.mkv"},
		}},
		{ID: "show", Kind: "series", Name: "Show", Overview: "A show.", Genres: []string{"Animation"}},
		{ID: "ep1", Kind: "episode", Name: "Pilot", Season: 1, Episode: 1, EpisodeEnd: 2,
			From:    librarystore.LibraryFrom{Name: "tmdb"},
			Sources: []librarystore.LibrarySource{{Path: "/t/show/s01e01.mkv", DurationMs: 1_320_000, Plays: true}}},
		{ID: "ep-file", Kind: "episode", Name: "show - bonus", Unnumbered: true,
			Sources: []librarystore.LibrarySource{{Path: "/t/show/bonus.mkv", Plays: true}}},
		{ID: "ep-gone", Kind: "episode", Name: "Gone", Season: 1, Episode: 3,
			Sources: []librarystore.LibrarySource{{Path: "/t/show/s01e03.mkv", Plays: true}}},
	}
	if err := st.SaveLibraryItems(ctx, items, at); err != nil {
		t.Fatal(err)
	}
	file := func(path, item, series string) librarystore.LibraryFile {
		return librarystore.LibraryFile{Path: path, Root: "/m", Size: 9, ModifiedNs: 100, Identity: "digest", Kind: "movie",
			Header: librarystore.LibraryHeaderRead, SeenAt: at, ItemID: item, SeriesID: series}
	}
	gone := file("/t/show/s01e03.mkv", "ep-gone", "show")
	gone.MissingAt = at
	heldA := file("/m/held/a.mkv", "held", "")
	heldA.MissingAt = at
	if err := st.SaveLibraryFiles(ctx, []librarystore.LibraryFile{
		heldA, file("/m/held/b.mkv", "held", ""),
		file("/m/film/film.mkv", "film", ""), file("/m/film/film dvd.mkv", "film", ""),
		file("/m/film/film {edition-Extended}.mkv", "film", ""),
		file("/m/parts/parts cd1.mkv", "parts", ""), file("/m/parts/parts cd2.mkv", "parts", ""),
		file("/t/show/s01e01.mkv", "ep1", "show"), file("/t/show/bonus.mkv", "ep-file", "show"), gone,
	}); err != nil {
		t.Fatal(err)
	}

	film, err := st.LibraryItemDetail(ctx, "film")
	if err != nil {
		t.Fatal(err)
	}
	if film.Overview != "A film." || film.Rating != "PG" || film.RuntimeMs != 5_400_000 ||
		!slices.Equal(film.Genres, []string{"Drama", "Comedy"}) ||
		film.From != (librarystore.LibraryFrom{Name: "nfo", Overview: "tmdb", Rating: "radarr", Year: "nfo"}) {
		t.Errorf("film facts = %+v", film)
	}
	type source struct {
		path     string
		duration int64
		edition  string
		part     int
		plays    bool
		streams  int
	}
	sources := func(srcs []librarystore.LibrarySource) []source {
		var out []source
		for _, s := range srcs {
			out = append(out, source{s.Path, s.DurationMs, s.Edition, s.Part, s.Plays, len(s.Streams)})
		}
		return out
	}
	if got, want := sources(film.Sources), []source{
		{"/m/film/film dvd.mkv", 5_400_000, "", 0, false, 2},
		{"/m/film/film {edition-Extended}.mkv", 6_000_000, "Extended", 0, true, 0},
		{"/m/film/film.mkv", 5_432_100, "", 0, true, 5},
	}; !slices.Equal(got, want) {
		t.Errorf("film sources = %+v, want %+v", got, want)
	}
	if got := film.Sources[2].Streams; !slices.Equal(got, streams) {
		t.Errorf("film streams = %+v, want %+v", got, streams)
	}
	if len(film.Episodes) != 0 {
		t.Errorf("a movie reads episodes %+v", film.Episodes)
	}

	// The sources are the rows as written: a source that plays is read though its file has gone
	// from the list, as it does while a removal waits; the page's extra-copy rule covers that.
	held, err := st.LibraryItemDetail(ctx, "held")
	if err != nil {
		t.Fatal(err)
	}
	if got, want := sources(held.Sources), []source{
		{"/m/held/a.mkv", 0, "", 0, true, 0},
		{"/m/held/b.mkv", 0, "", 0, false, 0},
	}; !slices.Equal(got, want) {
		t.Errorf("held sources = %+v, want %+v", got, want)
	}

	parts, err := st.LibraryItemDetail(ctx, "parts")
	if err != nil {
		t.Fatal(err)
	}
	if got, want := sources(parts.Sources), []source{
		{"/m/parts/parts cd1.mkv", 3_100_000, "", 1, true, 0},
		{"/m/parts/parts cd2.mkv", 3_000_000, "", 2, true, 0},
	}; !slices.Equal(got, want) {
		t.Errorf("parts sources = %+v, want %+v", got, want)
	}

	show, err := st.LibraryItemDetail(ctx, "show")
	if err != nil {
		t.Fatal(err)
	}
	if show.Overview != "A show." || !slices.Equal(show.Genres, []string{"Animation"}) {
		t.Errorf("show facts = %+v", show)
	}
	wantEpisodes := map[string]librarystore.LibraryDetailEpisode{
		"ep1":     {Name: "Pilot", NameFrom: "tmdb", Season: 1, Episode: 1, EpisodeEnd: 2},
		"ep-file": {Name: "show - bonus", Unnumbered: true},
	}
	if len(show.Episodes) != len(wantEpisodes) {
		t.Errorf("episodes = %+v, want %+v", show.Episodes, wantEpisodes)
	}
	for id, want := range wantEpisodes {
		if got := show.Episodes[id]; got != want {
			t.Errorf("episode %s = %+v, want %+v", id, got, want)
		}
	}
	if len(show.Sources) != 0 {
		t.Errorf("a series reads sources %+v; its episodes' are not read", show.Sources)
	}
	onDisk, err := st.LibraryDetailEpisodes(ctx, "show")
	if err != nil || !maps.Equal(onDisk, show.Episodes) {
		t.Errorf("episodes alone = %+v, %v; want the page's %+v", onDisk, err, show.Episodes)
	}

	unwritten, err := st.LibraryItemDetail(ctx, "no-row")
	if err != nil || unwritten.Overview != "" || len(unwritten.Episodes) != 0 || len(unwritten.Sources) != 0 {
		t.Errorf("an id with no row = %+v, %v; want zero", unwritten, err)
	}
}
