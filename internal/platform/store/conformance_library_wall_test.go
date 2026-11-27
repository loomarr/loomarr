package store

import (
	"context"
	"fmt"
	"reflect"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/platform/store/librarystore"
)

// testLibraryWallText pins how the wall reads text: a title with no sort title sorts and is
// bucketed by its name without accents, a search's % and _ are literal, a search under the recent
// sort keeps the name's matches first, and a genre outside ASCII matches in any case.
func testLibraryWallText(t *testing.T, newStore NewStoreFunc) {
	ctx := context.Background()
	st := newStore(t)
	at := time.UnixMilli(1_790_000_000_000).UTC()
	save := func(at time.Time, id, name string, genres ...string) {
		t.Helper()
		item := librarystore.LibraryItem{ID: id, Kind: "movie", Name: name, Genres: genres,
			Origin: librarystore.LibraryOrigin{Authority: "library:/m", Key: "/m/" + id},
			Match:  librarystore.LibraryMatch{Basis: "embedded_id"}, From: librarystore.LibraryFrom{Name: "tmdb"}}
		if err := st.SaveLibraryItems(ctx, []librarystore.LibraryItem{item}, at); err != nil {
			t.Fatal(err)
		}
		if err := st.SaveLibraryFiles(ctx, []librarystore.LibraryFile{{Path: "/m/" + id + ".mkv", Root: "/m",
			Size: 1, ModifiedNs: 1, Kind: "movie", Header: librarystore.LibraryHeaderRead, SeenAt: at, ItemID: id}}); err != nil {
			t.Fatal(err)
		}
	}
	save(at, "ember", "Ember Row")
	save(at, "ebene", "Ébène Quay")
	save(at, "erne", "Erne Walk", "Épouvante")
	save(at, "fallow", "Fallow Mire")
	save(at, "under", "Fen_Gate")
	save(at, "cross", "FenXGate")
	save(at, "percent", "Salt 50% Pier")
	save(at, "digits", "Salt 500 Pier")
	save(at, "old", "Old Marsh")
	save(at.Add(time.Hour), "new", "New Marsh")
	save(at.Add(2*time.Hour), "heron", "Heron", "Marsh")

	list := func(q librarystore.LibraryItemQuery) []string {
		t.Helper()
		page, err := st.ListLibraryItems(ctx, q)
		if err != nil {
			t.Fatal(err)
		}
		out := []string{}
		for _, it := range page.Items {
			out = append(out, it.ID)
		}
		return out
	}

	page, err := st.ListLibraryItems(ctx, librarystore.LibraryItemQuery{Letters: true, Limit: 3})
	if err != nil {
		t.Fatal(err)
	}
	var first []string
	for _, it := range page.Items {
		first = append(first, it.ID)
	}
	if want := []string{"ebene", "ember", "erne"}; !reflect.DeepEqual(first, want) {
		t.Errorf("first page = %v, want %v", first, want)
	}
	var letters []string
	for _, l := range page.Letters {
		letters = append(letters, fmt.Sprintf("%s%d", l.Letter, l.Offset))
	}
	if want := []string{"E0", "F3", "H6", "N7", "O8", "S9"}; !reflect.DeepEqual(letters, want) {
		t.Errorf("letters = %v, want %v", letters, want)
	}

	for _, tc := range []struct {
		name string
		q    librarystore.LibraryItemQuery
		want []string
	}{
		{"literal percent", librarystore.LibraryItemQuery{Search: "50%"}, []string{"percent"}},
		{"literal underscore", librarystore.LibraryItemQuery{Search: "n_g"}, []string{"under"}},
		{"search under recent", librarystore.LibraryItemQuery{Search: "marsh", Recent: true}, []string{"new", "old", "heron"}},
		{"non-ASCII genre", librarystore.LibraryItemQuery{Genre: "ÉPOUVANTE"}, []string{"erne"}},
		{"non-ASCII genre search", librarystore.LibraryItemQuery{Search: "épouvante"}, []string{"erne"}},
	} {
		if got := list(tc.q); !reflect.DeepEqual(got, tc.want) {
			t.Errorf("%s = %v, want %v", tc.name, got, tc.want)
		}
	}
}

// testLibraryWall pins the wall's filters (#4476): the list filters by genre in any case, a
// search finds a title by its year and its genres as well as its name with the name's matches
// first, the letter index names each initial's first offset under the same filters, and the
// genre counts group names in any case. A member's genre counts and letters leave out the titles
// a member doesn't read.
func testLibraryWall(t *testing.T, newStore NewStoreFunc) {
	ctx := context.Background()
	st := newStore(t)
	at := time.UnixMilli(1_790_000_000_000).UTC()
	origin := func(key string) librarystore.LibraryOrigin {
		return librarystore.LibraryOrigin{Authority: "library:/m", Key: key}
	}
	grounded := librarystore.LibraryMatch{Basis: "embedded_id"}
	named := librarystore.LibraryFrom{Name: "tmdb"}
	title := func(id, kind, name, sortName string, year int, genres ...string) librarystore.LibraryItem {
		return librarystore.LibraryItem{ID: id, Kind: kind, Name: name, SortName: sortName, Year: year, Genres: genres,
			Origin: origin("/m/" + id), Match: grounded, From: named}
	}
	hidden := title("h", "movie", "Zulu Hidden", "", 1979, "Western", "comedy")
	hidden.Match = librarystore.LibraryMatch{}
	items := []librarystore.LibraryItem{
		title("a1", "movie", "Alpha One", "", 1979, "Science Fiction", "Horror"),
		title("a2", "movie", "Alpha Two", "", 1986, "science fiction", "Action"),
		title("b", "movie", "Bravo Comedy Night", "", 2001, "Drama"),
		title("c", "movie", "Charlie", "", 1979, "Comedy"),
		title("d", "series", "Delta Show", "", 2010, "Comedy", "Animation"),
		title("n", "movie", "1979 Echo", "", 2005, "Documentary"),
		// The source's sort title drops the article; the list sorts by it.
		title("f", "movie", "The Foxtrot", "foxtrot", 2003),
		hidden,
	}
	if err := st.SaveLibraryItems(ctx, items, at); err != nil {
		t.Fatal(err)
	}
	files := []librarystore.LibraryFile{}
	for _, it := range items {
		files = append(files, librarystore.LibraryFile{Path: "/m/" + it.ID + ".mkv", Root: "/m", Size: 1, ModifiedNs: 1,
			Kind: "movie", Header: librarystore.LibraryHeaderRead, SeenAt: at, ItemID: it.ID})
	}
	if err := st.SaveLibraryFiles(ctx, files); err != nil {
		t.Fatal(err)
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
		return out, page.Total
	}
	for _, tc := range []struct {
		name string
		q    librarystore.LibraryItemQuery
		want []string
	}{
		{"genre any case", librarystore.LibraryItemQuery{Genre: "SCIENCE fiction", Limit: 60}, []string{"a1", "a2"}},
		{"genre and kind", librarystore.LibraryItemQuery{Genre: "comedy", Kind: "series", Limit: 60}, []string{"d"}},
		{"genre and search", librarystore.LibraryItemQuery{Genre: "comedy", Search: "char", Limit: 60}, []string{"c"}},
		{"unknown genre", librarystore.LibraryItemQuery{Genre: "no such genre", Limit: 60}, []string{}},
		{"year, name first", librarystore.LibraryItemQuery{Search: "1979", Limit: 60}, []string{"n", "a1", "c", "h"}},
		{"name and year", librarystore.LibraryItemQuery{Search: "alpha 1979", Limit: 60}, []string{"a1"}},
		{"genre, name first", librarystore.LibraryItemQuery{Search: "Comedy", Limit: 60}, []string{"b", "c", "d", "h"}},
		{"two-word genre", librarystore.LibraryItemQuery{Search: "science fiction", Limit: 60}, []string{"a1", "a2"}},
		{"member genre", librarystore.LibraryItemQuery{Genre: "western", Member: true, Limit: 60}, []string{}},
		{"member search", librarystore.LibraryItemQuery{Search: "1979", Member: true, Limit: 60}, []string{"n", "a1", "c"}},
	} {
		got, total := ids(tc.q)
		if !reflect.DeepEqual(got, tc.want) || total != len(tc.want) {
			t.Errorf("%s = %v (total %d), want %v", tc.name, got, total, tc.want)
		}
	}

	// L is a letter as the test writes it; letters reads the store's into it.
	type L struct {
		Letter string
		Offset int
	}
	letters := func(q librarystore.LibraryItemQuery) []L {
		t.Helper()
		q.Letters = true
		page, err := st.ListLibraryItems(ctx, q)
		if err != nil {
			t.Fatal(err)
		}
		var out []L
		for _, l := range page.Letters {
			out = append(out, L{Letter: l.Letter, Offset: l.Offset})
		}
		return out
	}
	all := letters(librarystore.LibraryItemQuery{Limit: 1})
	wantAll := []L{{"#", 0}, {"A", 1}, {"B", 3}, {"C", 4}, {"D", 5}, {"F", 6}, {"Z", 7}}
	if !reflect.DeepEqual(all, wantAll) {
		t.Fatalf("letters = %v, want %v", all, wantAll)
	}
	// Each offset lands on the title it names.
	for _, l := range all {
		got, _ := ids(librarystore.LibraryItemQuery{Limit: 1, Offset: l.Offset})
		want := map[string]string{"#": "n", "A": "a1", "B": "b", "C": "c", "D": "d", "F": "f", "Z": "h"}[l.Letter]
		if len(got) != 1 || got[0] != want {
			t.Errorf("offset %d for %s = %v, want %s", l.Offset, l.Letter, got, want)
		}
	}
	for _, tc := range []struct {
		name string
		q    librarystore.LibraryItemQuery
		want []L
	}{
		{"member skips a hidden initial", librarystore.LibraryItemQuery{Member: true},
			[]L{{"#", 0}, {"A", 1}, {"B", 3}, {"C", 4}, {"D", 5}, {"F", 6}}},
		{"under a genre", librarystore.LibraryItemQuery{Genre: "comedy"}, []L{{"C", 0}, {"D", 1}, {"Z", 2}}},
		{"under a search", librarystore.LibraryItemQuery{Search: "comedy"}, []L{{"B", 0}, {"C", 1}, {"D", 2}, {"Z", 3}}},
		{"none under the recent sort", librarystore.LibraryItemQuery{Recent: true}, nil},
	} {
		if got := letters(tc.q); !reflect.DeepEqual(got, tc.want) {
			t.Errorf("letters %s = %v, want %v", tc.name, got, tc.want)
		}
	}
	if page, err := st.ListLibraryItems(ctx, librarystore.LibraryItemQuery{}); err != nil || page.Letters != nil {
		t.Errorf("letters unasked = %v (%v), want none", page.Letters, err)
	}

	type G struct {
		Name   string
		Titles int
	}
	for _, tc := range []struct {
		name   string
		kind   string
		member bool
		want   []G
	}{
		{"admin", "", false, []G{{"Action", 1}, {"Animation", 1}, {"Comedy", 3}, {"Documentary", 1}, {"Drama", 1},
			{"Horror", 1}, {"Science Fiction", 2}, {"Western", 1}}},
		{"admin movies", "movie", false, []G{{"Action", 1}, {"Comedy", 2}, {"Documentary", 1}, {"Drama", 1},
			{"Horror", 1}, {"Science Fiction", 2}, {"Western", 1}}},
		{"member", "", true, []G{{"Action", 1}, {"Animation", 1}, {"Comedy", 2}, {"Documentary", 1}, {"Drama", 1},
			{"Horror", 1}, {"Science Fiction", 2}}},
		{"member series", "series", true, []G{{"Animation", 1}, {"Comedy", 1}}},
	} {
		genres, err := st.LibraryGenres(ctx, tc.kind, tc.member)
		if err != nil {
			t.Fatal(err)
		}
		got := []G{}
		for _, g := range genres {
			got = append(got, G{Name: g.Name, Titles: g.Titles})
		}
		if !reflect.DeepEqual(got, tc.want) {
			t.Errorf("genres %s = %v, want %v", tc.name, got, tc.want)
		}
	}
}
