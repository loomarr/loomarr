package store

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/platform/store/librarystore"
)

// testLibraryFilters pins the wall's filters (#4476): studio, person, the tracks of the copy
// that plays (a series through any episode), how they combine, and choices and people whose
// counts agree with the list, a member's leaving out what a member doesn't read.
func testLibraryFilters(t *testing.T, newStore NewStoreFunc) {
	ctx := context.Background()
	st := newStore(t)
	at := time.UnixMilli(1_790_000_000_000).UTC()
	video := func(resolution, hdr string) librarystore.LibraryStream {
		return librarystore.LibraryStream{Kind: "video", Resolution: resolution, HDR: hdr}
	}
	audio := librarystore.LibraryStream{Index: 1, Kind: "audio", Language: "fr", LanguageName: "French"}
	english := librarystore.LibraryStream{Index: 1, Kind: "audio", Language: "en", LanguageName: "English"}
	subs := librarystore.LibraryStream{Index: 2, Kind: "subtitle", Language: "de", LanguageName: "German"}
	source := func(path string, plays bool, streams ...librarystore.LibraryStream) librarystore.LibrarySource {
		return librarystore.LibrarySource{Path: path, Plays: plays, Streams: streams}
	}
	actor := librarystore.LibraryCredit{Name: "Ada Quill", Role: "actor", ID: librarystore.LibraryID{Namespace: "tmdb", Value: "90000901"}}
	director := librarystore.LibraryCredit{Name: "Bram Fen", Role: "director"}
	grounded := librarystore.LibraryMatch{Basis: "embedded_id"}
	named := librarystore.LibraryFrom{Name: "tmdb"}
	if err := st.SaveLibraryItems(ctx, []librarystore.LibraryItem{
		// A 4K Dolby Vision film with French sound and German subtitles.
		{ID: "a", Kind: "movie", Name: "Alpha", Match: grounded, From: named, Studios: []string{"Larkfield"},
			Credits: []librarystore.LibraryCredit{actor, director},
			Sources: []librarystore.LibrarySource{source("/m/a.mkv", true, video("4K", "Dolby Vision"), audio, subs)}},
		// Its 4K copy never plays: only the playing 1080p one counts.
		{ID: "b", Kind: "movie", Name: "Bravo", Match: grounded, From: named, Studios: []string{"LARKFIELD", "Oxbow"},
			Credits: []librarystore.LibraryCredit{actor},
			Sources: []librarystore.LibrarySource{source("/m/b.mkv", true, video("1080p", ""), english),
				source("/m/b-extra.mkv", false, video("4K", "HDR10"), audio, subs)}},
		// Its only 4K file can't be played, and its French one is gone.
		{ID: "c", Kind: "movie", Name: "Charlie", Match: grounded, From: named, Sources: []librarystore.LibrarySource{
			source("/m/c.mkv", true, video("4K", "HLG")), source("/m/c-gone.mkv", true, audio)}},
		// Waiting in Needs you: a member never sees it, its studio or its person.
		{ID: "u", Kind: "movie", Name: "unread folder", Studios: []string{"Hidden House"},
			Credits: []librarystore.LibraryCredit{{Name: "Unseen Person", Role: "actor"}},
			Sources: []librarystore.LibrarySource{source("/m/u.mkv", true, video("4K", "HDR10"), audio)}},
		// A series: one episode is 4K, another carries the French sound.
		{ID: "s", Kind: "series", Name: "Delta Show", Match: grounded, From: named, Studios: []string{"Oxbow"},
			Credits: []librarystore.LibraryCredit{director}},
		{ID: "s1", Kind: "episode", Name: "One", SeriesID: "s", Season: 1, Episode: 1, From: named,
			Sources: []librarystore.LibrarySource{source("/m/s/1.mkv", true, video("4K", "HDR10"), english)}},
		{ID: "s2", Kind: "episode", Name: "Two", SeriesID: "s", Season: 1, Episode: 2, From: named,
			Sources: []librarystore.LibrarySource{source("/m/s/2.mkv", true, video("720p", ""), audio)}},
	}, at); err != nil {
		t.Fatal(err)
	}
	file := func(path, item, series, header string, missing time.Time) librarystore.LibraryFile {
		return librarystore.LibraryFile{Path: path, Root: "/m", Size: 1, ModifiedNs: 1, Identity: "digest", Kind: "movie",
			Header: header, SeenAt: at, ItemID: item, SeriesID: series, MissingAt: missing}
	}
	read, failed := librarystore.LibraryHeaderRead, librarystore.LibraryHeaderFailed
	var here time.Time
	if err := st.SaveLibraryFiles(ctx, []librarystore.LibraryFile{
		file("/m/a.mkv", "a", "", read, here), file("/m/b.mkv", "b", "", read, here), file("/m/b-extra.mkv", "b", "", read, here),
		file("/m/c.mkv", "c", "", failed, here), file("/m/c-gone.mkv", "c", "", read, at), file("/m/u.mkv", "u", "", read, here),
		file("/m/s/1.mkv", "s1", "s", read, here), file("/m/s/2.mkv", "s2", "s", read, here),
	}); err != nil {
		t.Fatal(err)
	}

	lists := func(q librarystore.LibraryItemQuery, want ...string) {
		t.Helper()
		q.Limit, q.Letters = 50, true
		page, err := st.ListLibraryItems(ctx, q)
		if err != nil {
			t.Fatal(err)
		}
		var got []string
		for _, it := range page.Items {
			got = append(got, it.ID)
		}
		if !slices.Equal(got, want) || page.Total != len(want) {
			t.Errorf("%+v lists %v (total %d), want %v", q, got, page.Total, want)
		}
		// Letters read the same filters, so there is a stop exactly when there is a title.
		if (len(page.Letters) == 0) != (len(want) == 0) {
			t.Errorf("%+v letters = %v for %v", q, page.Letters, want)
		}
	}
	type q = librarystore.LibraryItemQuery
	lists(q{Studio: "larkfield"}, "a", "b")
	lists(q{Studio: "Nowhere"})
	lists(q{Person: "tmdb:90000901"}, "a", "b")
	lists(q{Person: "name:Bram Fen"}, "a", "s")
	lists(q{Person: "tmdb:0"})
	lists(q{Audio: "FR"}, "a", "s", "u")
	lists(q{Audio: "FR", Member: true}, "a", "s")
	lists(q{Subtitles: "de"}, "a")
	lists(q{Subtitles: "ja"})
	lists(q{UHD: true, Member: true}, "a", "s")
	lists(q{HDR: librarystore.LibraryHDRAny, Member: true}, "a", "s")
	lists(q{HDR: "HDR10", Member: true}, "s")
	lists(q{HDR: "HLG"})
	// Filters AND together; each track filter may be met by a different episode.
	lists(q{UHD: true, Audio: "fr", Member: true}, "a", "s")
	lists(q{UHD: true, Audio: "fr", Studio: "oxbow"}, "s")
	lists(q{Studio: "larkfield", Person: "name:Bram Fen", Kind: "movie", Search: "alpha"}, "a")
	lists(q{Studio: "oxbow", Subtitles: "de"})

	same := func(what string, got, want []librarystore.LibraryFacet) {
		t.Helper()
		if !slices.Equal(got, want) {
			t.Errorf("%s = %+v, want %+v", what, got, want)
		}
	}
	type f = librarystore.LibraryFacet
	got, err := st.LibraryFacets(ctx, "", true)
	if err != nil {
		t.Fatal(err)
	}
	// A studio saved in two cases is one choice; which spelling names it is the backend's MIN.
	if len(got.Studios) != 2 || got.Studios[0].Titles != 2 || got.Studios[1] != (f{Value: "Oxbow", Name: "Oxbow", Titles: 2}) {
		t.Errorf("member studios = %+v", got.Studios)
	}
	choice := func(value, name string, titles int) f { return f{Value: value, Name: name, Titles: titles} }
	same("member audio", got.Audio, []f{choice("en", "English", 2), choice("fr", "French", 2)})
	same("member subtitles", got.Subtitles, []f{choice("de", "German", 1)})
	same("member hdr", got.HDR, []f{choice("Dolby Vision", "Dolby Vision", 1), choice("HDR10", "HDR10", 1)})
	if got.UHD != 2 || got.HDRAny != 2 {
		t.Errorf("member 4K, HDR = %d, %d; want 2, 2", got.UHD, got.HDRAny)
	}
	if got, err = st.LibraryFacets(ctx, "movie", false); err != nil {
		t.Fatal(err)
	}
	if got.UHD != 2 || len(got.Studios) != 3 {
		t.Errorf("admin movie facets = %+v", got)
	}

	people, err := st.LibraryPeople(ctx, "", "", true, 10)
	if err != nil {
		t.Fatal(err)
	}
	want := []librarystore.LibraryPerson{{ID: "tmdb:90000901", Name: "Ada Quill", Titles: 2}, {ID: "name:Bram Fen", Name: "Bram Fen", Titles: 2}}
	if !slices.Equal(people, want) {
		t.Errorf("member people = %+v, want %+v", people, want)
	}
	if people, err = st.LibraryPeople(ctx, "QUILL", "series", false, 10); err != nil || len(people) != 0 {
		t.Errorf("series people named quill = %+v, %v", people, err)
	}
	if p, err := st.LibraryPerson(ctx, "tmdb:90000901", true); err != nil || p != want[0] {
		t.Errorf("person = %+v, %v", p, err)
	}
	if _, err := st.LibraryPerson(ctx, "name:Unseen Person", true); !errors.Is(err, ErrNotFound) {
		t.Errorf("a member reads a person only a hidden title credits: %v", err)
	}
	if p, err := st.LibraryPerson(ctx, "name:Unseen Person", false); err != nil || p.Titles != 1 {
		t.Errorf("admin's unseen person = %+v, %v", p, err)
	}
}
