package store

import (
	"context"
	"maps"
	"slices"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/platform/store/clipstore"
	"github.com/loomarr/loomarr/internal/platform/store/librarystore"
)

// A title's page reads back what was written (#4476): a movie's facts, studios, countries and
// credits and their providers, each source with its container, bitrate, edition, part and whether
// it plays (an extra copy does not), the columns of its tracks the page shows in index order with
// a subtitle file beside it last, its chapters, a source that plays though its file has gone from
// the list, a movie in parts, a series' episodes by the items its present files were written as, with their stories
// and lengths and stills (an unnumbered one among them, a missing file's left out) and no episode's tracks
// read, their numbers alone, and an episode's own page with its file's tracks.
func testLibraryItemDetail(t *testing.T, newStore NewStoreFunc) {
	ctx := context.Background()
	st := newStore(t)
	at := time.UnixMilli(1_800_000_000_000)
	streams := []librarystore.LibraryStream{
		{Index: 0, Kind: "video", Codec: "hevc", Profile: "Main 10", Width: 3840, Height: 2160, FrameRate: "24000/1001",
			PixelFormat: "yuv420p10le", ColorTransfer: "smpte2084", Resolution: "4K", HDR: "HDR10", BitDepth: 10,
			BitRate: 20_000_000},
		{Index: 1, Kind: "audio", Codec: "aac", Channels: 2, ChannelLayout: "stereo", SampleRate: 48_000,
			Language: "en", LanguageName: "English", AudioFormat: "AAC", Title: "Commentary", Commentary: true},
		{Index: 2, Kind: "audio", Codec: "truehd", Channels: 6, ChannelLayout: "5.1(side)", Language: "en",
			LanguageName: "English", Default: true, BitDepth: 24, AudioFormat: "Dolby TrueHD"},
		{Index: 3, Kind: "subtitle", Codec: "subrip", Language: "en", LanguageName: "English", Forced: true},
		{Index: 4, Kind: "subtitle", Codec: "subrip", Language: "es", LanguageName: "Spanish", HearingImpaired: true,
			ExternalRef: "/m/film/film.es.srt"},
		{Index: 5, Kind: "audio", Codec: "ac3", Language: "en", Descriptive: true},
	}
	// shown is the tracks as the page reads them: without sample rate, pixel format, colour
	// transfer and resolution label, which it does not show.
	shown := func(sts []librarystore.LibraryStream) []librarystore.LibraryStream {
		out := slices.Clone(sts)
		for i := range out {
			out[i].SampleRate, out[i].PixelFormat, out[i].ColorTransfer, out[i].Resolution = 0, "", "", ""
		}
		return out
	}
	chapters := []librarystore.LibraryChapter{
		{StartMs: 0, EndMs: 600_000, Title: "Opening"}, {StartMs: 600_000, EndMs: -1},
	}
	credits := []librarystore.LibraryCredit{
		{Name: "Ana Actor", Role: "actor", Character: "Lead", ID: librarystore.LibraryID{Namespace: "tmdb", Value: "1"}},
		{Name: "Bo Byname", Role: "actor"},
		{Name: "Ana Actor", Role: "director", ID: librarystore.LibraryID{Namespace: "tmdb", Value: "1"}},
		{Name: "Cy Writer", Role: "writer"},
	}
	items := []LibraryItem{
		{ID: "film", Kind: "movie", Name: "Film", Overview: "A film.", Rating: "PG", RuntimeMs: 5_400_000,
			OriginalTitle: "Le Film", Tagline: "One line.", PremiereDate: "2001-02-03", Score: 7.5,
			Genres: []string{"Drama", "Comedy"}, Studios: []string{"North Studio", "South Studio"},
			Countries: []string{"France"}, Credits: credits,
			From: librarystore.LibraryFrom{Name: "nfo", Overview: "tmdb", Rating: "radarr", Year: "nfo",
				Score: "tmdb", Runtime: "radarr", Premiere: "nfo"},
			Sources: []librarystore.LibrarySource{
				{Path: "/m/film/film.mkv", Container: "matroska,webm", Bitrate: 24_000_000, DurationMs: 5_432_100,
					Plays: true, Streams: streams, Chapters: chapters},
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
		{ID: "ep1", Kind: "episode", Name: "Pilot", Season: 1, Episode: 1, EpisodeEnd: 2, Overview: "It begins.",
			RuntimeMs: 1_380_000, From: librarystore.LibraryFrom{Name: "tmdb"},
			Sources: []librarystore.LibrarySource{{Path: "/t/show/s01e01.mkv", Container: "matroska,webm",
				DurationMs: 1_320_000, Plays: true, Streams: streams[:2], Chapters: chapters[:1]}}},
		{ID: "ep-file", Kind: "episode", Name: "show - bonus", Unnumbered: true, RuntimeMs: 1_500_000,
			Sources: []librarystore.LibrarySource{{Path: "/t/show/bonus.mkv", Plays: true}}},
		{ID: "ep-none", Kind: "episode", Name: "Unknown length", Season: 1, Episode: 4,
			Sources: []librarystore.LibrarySource{{Path: "/t/show/s01e04.mkv", Plays: true}}},
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
		file("/t/show/s01e04.mkv", "ep-none", "show"),
	}); err != nil {
		t.Fatal(err)
	}

	film, err := st.LibraryItemDetail(ctx, "film")
	if err != nil {
		t.Fatal(err)
	}
	if film.Overview != "A film." || film.Rating != "PG" || film.RuntimeMs != 5_400_000 ||
		film.OriginalTitle != "Le Film" || film.Tagline != "One line." || film.PremiereDate != "2001-02-03" ||
		film.Score != 7.5 || !slices.Equal(film.Genres, []string{"Drama", "Comedy"}) ||
		!slices.Equal(film.Studios, []string{"North Studio", "South Studio"}) ||
		!slices.Equal(film.Countries, []string{"France"}) ||
		film.From != (librarystore.LibraryFrom{Name: "nfo", Overview: "tmdb", Rating: "radarr", Year: "nfo",
			Score: "tmdb", Runtime: "radarr", Premiere: "nfo"}) {
		t.Errorf("film facts = %+v", film)
	}
	// Credits read back in billing order with their people's names, one person twice in two roles,
	// and a linked person's catalog id, with no photo.
	ana := librarystore.LibraryID{Namespace: "tmdb", Value: "1"}
	wantCredits := []librarystore.LibraryCredit{
		{Name: "Ana Actor", Role: "actor", Character: "Lead", ID: ana}, {Name: "Bo Byname", Role: "actor"},
		{Name: "Ana Actor", Role: "director", ID: ana}, {Name: "Cy Writer", Role: "writer"},
	}
	if !slices.Equal(film.Credits, wantCredits) {
		t.Errorf("film credits = %+v, want %+v", film.Credits, wantCredits)
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
		{"/m/film/film.mkv", 5_432_100, "", 0, true, 6},
	}; !slices.Equal(got, want) {
		t.Errorf("film sources = %+v, want %+v", got, want)
	}
	if got := film.Sources[2]; got.Container != "matroska,webm" || got.Bitrate != 24_000_000 {
		t.Errorf("film source = %q at %d bit/s, want matroska,webm at 24000000", got.Container, got.Bitrate)
	}
	if got, want := film.Sources[2].Streams, shown(streams); !slices.Equal(got, want) {
		t.Errorf("film streams = %+v, want %+v", got, want)
	}
	if got := film.Sources[2].Chapters; !slices.Equal(got, chapters) {
		t.Errorf("film chapters = %+v, want %+v", got, chapters)
	}
	if got := film.Sources[0].Chapters; len(got) != 0 {
		t.Errorf("a source with no chapters reads %+v", got)
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

	// An episode's still is its thumb ref, read with its row (#4476); a backdrop ref is not it.
	still, fanart := imageAt("ep1-still", at), imageAt("ep1-fanart", at)
	for _, owned := range []struct {
		img  clipstore.Image
		role string
	}{{still, "thumb"}, {fanart, "backdrop"}} {
		if err := st.PutImage(ctx, owned.img); err != nil {
			t.Fatal(err)
		}
		if err := st.PutImageRef(ctx, clipstore.ImageRef{ImageHash: owned.img.Hash, OwnerKind: librarystore.LibraryItemImageOwner,
			OwnerID: "ep1", Role: owned.role}); err != nil {
			t.Fatal(err)
		}
	}
	show, err := st.LibraryItemDetail(ctx, "show")
	if err != nil {
		t.Fatal(err)
	}
	if show.Overview != "A show." || !slices.Equal(show.Genres, []string{"Animation"}) {
		t.Errorf("show facts = %+v", show)
	}
	// An episode's length is its file's measured length (ep1), else its catalog's runtime while its
	// file is unread (ep-file), else unknown (ep-none).
	wantEpisodes := map[string]librarystore.LibraryDetailEpisode{
		"ep1": {Name: "Pilot", NameFrom: "tmdb", LibraryEpisodeNumbers: librarystore.LibraryEpisodeNumbers{
			Season: 1, Episode: 1, EpisodeEnd: 2}, Overview: "It begins.", LengthMs: 1_320_000, Still: still.Hash},
		"ep-file": {Name: "show - bonus", Unnumbered: true, LengthMs: 1_500_000},
		"ep-none": {Name: "Unknown length", LibraryEpisodeNumbers: librarystore.LibraryEpisodeNumbers{Season: 1, Episode: 4}},
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
	numbers := map[string]librarystore.LibraryEpisodeNumbers{}
	for id, e := range show.Episodes {
		numbers[id] = e.LibraryEpisodeNumbers
	}
	onDisk, err := st.LibraryEpisodeNumbers(ctx, "show")
	if err != nil || !maps.Equal(onDisk, numbers) {
		t.Errorf("episode numbers = %+v, %v; want the page's %+v", onDisk, err, numbers)
	}
	if len(show.Credits) != 0 || len(show.Studios) != 0 || show.Score != 0 {
		t.Errorf("a series with none reads credits %+v, studios %+v, score %v", show.Credits, show.Studios, show.Score)
	}

	if show.Still != "" {
		t.Errorf("series still = %q, want none", show.Still)
	}

	// An episode's own page reads its file with its tracks and chapters, and its still.
	ep, err := st.LibraryItemDetail(ctx, "ep1")
	if err != nil {
		t.Fatal(err)
	}
	if ep.Still != still.Hash {
		t.Errorf("episode page still = %q, want %q", ep.Still, still.Hash)
	}
	if ep.Overview != "It begins." || ep.RuntimeMs != 1_380_000 || len(ep.Sources) != 1 ||
		ep.Sources[0].Container != "matroska,webm" || !slices.Equal(ep.Sources[0].Streams, shown(streams[:2])) ||
		!slices.Equal(ep.Sources[0].Chapters, chapters[:1]) {
		t.Errorf("episode page = %+v", ep)
	}

	unwritten, err := st.LibraryItemDetail(ctx, "no-row")
	if err != nil || unwritten.Overview != "" || len(unwritten.Episodes) != 0 || len(unwritten.Sources) != 0 {
		t.Errorf("an id with no row = %+v, %v; want zero", unwritten, err)
	}
}
