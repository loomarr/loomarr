package store

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/platform/store/librarystore"
)

// testLibraryMemberReads pins what a household member reads of the Library (#4476): a movie or
// series a catalogue identifies or the maintainer answered, never one they ignored, and an
// episode its show's catalog numbers, of such a show. A title waiting in Needs you is left out of
// a member's list, its search, its total, a set's movies and a collection's titles, and reads as
// not found by id. An admin's reads are unchanged.
func testLibraryMemberReads(t *testing.T, newStore NewStoreFunc) {
	ctx := context.Background()
	st := newStore(t)
	at := time.UnixMilli(1_790_000_000_000).UTC()
	origin := func(key string) librarystore.LibraryOrigin {
		return librarystore.LibraryOrigin{Authority: "library:/m", Key: key}
	}
	grounded := librarystore.LibraryMatch{Basis: "embedded_id"}
	set := librarystore.LibraryCatalogKey{Provider: "tmdb_collection", Kind: "collection", ExternalID: "90000811"}
	if err := st.SaveLibraryItems(ctx, []librarystore.LibraryItem{
		{ID: "g", Kind: "movie", Name: "Grounded Film", Origin: origin("/m/g.mkv"), Match: grounded, Set: set},
		{ID: "u", Kind: "movie", Name: "unknown folder name", Origin: origin("/m/unknown folder name/u.mkv"), Set: set},
		{ID: "a", Kind: "movie", Name: "Answered Film", Origin: origin("/m/a.mkv")},
		{ID: "i", Kind: "movie", Name: "ignored folder name", Origin: origin("/m/i.mkv"), Match: grounded},
		{ID: "s", Kind: "series", Name: "Grounded Show", Origin: origin("/m/s"), Match: librarystore.LibraryMatch{Basis: "manager", App: "sonarr"}},
		{ID: "s1", Kind: "episode", Name: "Pilot", SeriesID: "s", Season: 1, Episode: 1},
		{ID: "s2", Kind: "episode", Name: "s.s01e02.file name", SeriesID: "s", Season: 1, Episode: 2, Unnumbered: true},
		{ID: "x", Kind: "series", Name: "unknown show folder", Origin: origin("/m/x")},
		{ID: "x1", Kind: "episode", Name: "Numbered", SeriesID: "x", Season: 1, Episode: 1},
	}, at); err != nil {
		t.Fatal(err)
	}
	file := func(path, item, series string) librarystore.LibraryFile {
		return librarystore.LibraryFile{Path: path, Root: "/m", Size: 1, ModifiedNs: 1, Identity: "digest", Kind: "movie",
			Header: librarystore.LibraryHeaderRead, SeenAt: at, ItemID: item, SeriesID: series}
	}
	if err := st.SaveLibraryFiles(ctx, []librarystore.LibraryFile{
		file("/m/g.mkv", "g", ""), file("/m/unknown folder name/u.mkv", "u", ""), file("/m/a.mkv", "a", ""),
		file("/m/i.mkv", "i", ""), file("/m/s/e1.mkv", "s1", "s"), file("/m/s/e2.mkv", "s2", "s"),
		file("/m/x/e1.mkv", "x1", "x"),
	}); err != nil {
		t.Fatal(err)
	}
	for _, answer := range []librarystore.LibraryAnswer{
		{Origin: origin("/m/a.mkv"), Kind: "movie", ID: librarystore.LibraryID{Namespace: "tmdb", Value: "90000812"}, AnsweredAt: at},
		{Origin: origin("/m/i.mkv"), Kind: "movie", Ignored: true, AnsweredAt: at},
	} {
		if err := st.SaveLibraryAnswer(ctx, answer); err != nil {
			t.Fatal(err)
		}
	}

	ids := func(q librarystore.LibraryItemQuery) ([]string, int) {
		t.Helper()
		page, err := st.ListLibraryItems(ctx, q)
		if err != nil {
			t.Fatal(err)
		}
		out := []string{}
		for _, it := range page.Items {
			out = append(out, it.ID)
		}
		slices.Sort(out)
		return out, page.Total
	}
	for _, tc := range []struct {
		name  string
		q     librarystore.LibraryItemQuery
		want  []string
		total int
	}{
		{"member list", librarystore.LibraryItemQuery{Member: true, Limit: 10}, []string{"a", "g", "s"}, 3},
		{"admin list", librarystore.LibraryItemQuery{Limit: 10}, []string{"a", "g", "s", "u", "x"}, 5},
		{"member search", librarystore.LibraryItemQuery{Member: true, Search: "folder", Limit: 10}, []string{}, 0},
		{"admin search", librarystore.LibraryItemQuery{Search: "folder", Limit: 10}, []string{"u", "x"}, 2},
		{"member set", librarystore.LibraryItemQuery{Member: true, Kind: "movie", InSet: true}, []string{"g"}, 1},
		{"admin set", librarystore.LibraryItemQuery{Kind: "movie", InSet: true}, []string{"g", "u"}, 2},
		{"member ids", librarystore.LibraryItemQuery{Member: true, IDs: []string{"u", "g", "i", "x"}}, []string{"g"}, 1},
	} {
		if got, total := ids(tc.q); !slices.Equal(got, tc.want) || total != tc.total {
			t.Errorf("%s = %v (total %d), want %v (total %d)", tc.name, got, total, tc.want, tc.total)
		}
	}

	for _, id := range []string{"g", "a", "s", "s1"} {
		if got, err := st.MemberLibraryItem(ctx, id); err != nil || got.ID != id {
			t.Errorf("member reads %s = %+v, %v", id, got.ID, err)
		}
	}
	// Waiting in Needs you, ignored, of a show waiting in Needs you, still named by its file, or
	// nothing at all: each is the same not found.
	for _, id := range []string{"u", "i", "x", "x1", "s2", "nope"} {
		if _, err := st.MemberLibraryItem(ctx, id); !errors.Is(err, ErrNotFound) {
			t.Errorf("member reads %s: err = %v, want ErrNotFound", id, err)
		}
	}
	for _, id := range []string{"u", "i", "x", "s1", "s2"} {
		if got, err := st.LibraryItem(ctx, id); err != nil || got.ID != id {
			t.Errorf("admin reads %s = %+v, %v", id, got.ID, err)
		}
	}
}
