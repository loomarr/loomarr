package api_test

import (
	"context"
	"net/http"
	"net/url"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/platform/store/librarystore"
)

// seedWall stores identified movies and a series with genres and years, one whose source sort
// title drops its article, one whose name starts with a digit, and a movie waiting in Needs you,
// the only title starting with Z and the only Western.
func seedWall(t *testing.T, h *apiHarness) {
	t.Helper()
	ctx := context.Background()
	at := time.UnixMilli(1_790_000_000_000).UTC()
	title := func(id, kind, name, sortName string, year int, genres ...string) librarystore.LibraryItem {
		return librarystore.LibraryItem{ID: id, Kind: kind, Name: name, SortName: sortName, Year: year, Genres: genres,
			Origin: librarystore.LibraryOrigin{Authority: "library:/media", Key: "/media/" + id},
			Match:  librarystore.LibraryMatch{Basis: "embedded_id"},
			From:   librarystore.LibraryFrom{Name: "tmdb"},
		}
	}
	hidden := title("90000108", "movie", "Zulu Hidden", "", 1979, "Western", "Comedy")
	hidden.Match = librarystore.LibraryMatch{}
	items := []librarystore.LibraryItem{
		title("90000101", "movie", "Alpha One", "", 1979, "Science Fiction", "Horror"),
		title("90000102", "movie", "Alpha Two", "", 1986, "Science Fiction", "Action"),
		title("90000103", "movie", "Bravo Comedy Night", "", 2001, "Drama"),
		title("90000104", "movie", "Charlie", "", 1979, "Comedy"),
		title("90000105", "series", "Delta Show", "", 2010, "Comedy", "Animation"),
		title("90000106", "movie", "1979 Echo", "", 2005, "Documentary"),
		title("90000107", "movie", "The Foxtrot", "foxtrot", 2003),
		hidden,
	}
	if err := h.Store.SaveLibraryItems(ctx, items, at); err != nil {
		t.Fatal(err)
	}
	files := []librarystore.LibraryFile{}
	for _, it := range items {
		files = append(files, librarystore.LibraryFile{Path: "/media/" + it.ID + ".mkv", Root: "/media", Size: 1, ModifiedNs: 1,
			Kind: "movie", Header: librarystore.LibraryHeaderRead, SeenAt: at, ItemID: it.ID})
	}
	if err := h.Store.SaveLibraryFiles(ctx, files); err != nil {
		t.Fatal(err)
	}
}

type wallLetter struct {
	Letter string `json:"letter"`
	Offset int    `json:"offset"`
}

type wallPage struct {
	Items []struct {
		Name string `json:"name"`
	} `json:"items"`
	Total   int          `json:"total"`
	Letters []wallLetter `json:"letters"`
}

func wallList(t *testing.T, h *apiHarness, token string, query url.Values) wallPage {
	t.Helper()
	var p wallPage
	decode(t, h.Do(http.MethodGet, "/v1/library/items?"+query.Encode(), token, ""), &p)
	return p
}

func wallNames(p wallPage) string {
	names := []string{}
	for _, it := range p.Items {
		names = append(names, it.Name)
	}
	return strings.Join(names, ", ")
}

// The wall filters by genre with kind and search, finds a title by its year and its genre as well
// as its name with name matches first, and carries a letter index whose offsets land on their
// titles (#4476). A member reads all of it without the titles waiting in Needs you.
func TestLibraryWallFiltersSearchAndLetters(t *testing.T) {
	h := newLibraryHarness(t)
	seedWall(t, h)
	for _, tc := range []struct {
		token string
		query url.Values
		want  string
	}{
		{adminToken, url.Values{"genre": {"science fiction"}}, "Alpha One, Alpha Two"},
		{adminToken, url.Values{"genre": {"COMEDY"}, "kind": {"series"}}, "Delta Show"},
		{adminToken, url.Values{"genre": {"comedy"}, "search": {"char"}}, "Charlie"},
		{adminToken, url.Values{"genre": {"no such genre"}}, ""},
		{adminToken, url.Values{"search": {"1979"}}, "1979 Echo, Alpha One, Charlie, Zulu Hidden"},
		{adminToken, url.Values{"search": {"alpha 1979"}}, "Alpha One"},
		{adminToken, url.Values{"search": {"comedy"}}, "Bravo Comedy Night, Charlie, Delta Show, Zulu Hidden"},
		{memberToken, url.Values{"genre": {"comedy"}}, "Charlie, Delta Show"},
		{memberToken, url.Values{"search": {"1979"}}, "1979 Echo, Alpha One, Charlie"},
		{memberToken, url.Values{"search": {"comedy"}, "kind": {"movie"}}, "Bravo Comedy Night, Charlie"},
		// A member never learns of the hidden title's genre or name.
		{memberToken, url.Values{"genre": {"western"}}, ""},
		{memberToken, url.Values{"search": {"western"}}, ""},
		{memberToken, url.Values{"search": {"zulu"}}, ""},
	} {
		p := wallList(t, h, tc.token, tc.query)
		if got := wallNames(p); got != tc.want || p.Total != len(p.Items) {
			t.Errorf("%s ?%s = %q (total %d), want %q", tc.token, tc.query.Encode(), got, p.Total, tc.want)
		}
	}

	for _, tc := range []struct {
		token string
		query url.Values
		want  []wallLetter
	}{
		{adminToken, url.Values{"limit": {"1"}},
			[]wallLetter{{"#", 0}, {"A", 1}, {"B", 3}, {"C", 4}, {"D", 5}, {"F", 6}, {"Z", 7}}},
		// The only title starting with Z waits in Needs you: a member's rail has no Z.
		{memberToken, url.Values{"limit": {"1"}},
			[]wallLetter{{"#", 0}, {"A", 1}, {"B", 3}, {"C", 4}, {"D", 5}, {"F", 6}}},
		{memberToken, url.Values{"genre": {"comedy"}}, []wallLetter{{"C", 0}, {"D", 1}}},
		{adminToken, url.Values{"sort": {"recent"}}, nil},
	} {
		if got := wallList(t, h, tc.token, tc.query).Letters; !reflect.DeepEqual(got, tc.want) {
			t.Errorf("%s letters ?%s = %v, want %v", tc.token, tc.query.Encode(), got, tc.want)
		}
	}
	// A rail click asks for offset=<n>: it lands on the letter's first title.
	for letter, want := range map[string]string{"#": "1979 Echo", "A": "Alpha One", "F": "The Foxtrot", "Z": "Zulu Hidden"} {
		var offset int
		for _, l := range wallList(t, h, adminToken, url.Values{"limit": {"1"}}).Letters {
			if l.Letter == letter {
				offset = l.Offset
			}
		}
		p := wallList(t, h, adminToken, url.Values{"limit": {"1"}, "offset": {strconv.Itoa(offset)}})
		if got := wallNames(p); got != want {
			t.Errorf("offset for %s = %q, want %q", letter, got, want)
		}
	}
}

// The genre list counts each genre's titles by name, narrowed by kind, and a member's counts
// leave out the titles a member doesn't read: no Western, one fewer Comedy (#4476).
func TestLibraryGenres(t *testing.T) {
	h := newLibraryHarness(t)
	seedWall(t, h)
	type genre struct {
		Name   string `json:"name"`
		Titles int    `json:"titles"`
	}
	for _, tc := range []struct {
		token, query string
		want         []genre
	}{
		{adminToken, "", []genre{{"Action", 1}, {"Animation", 1}, {"Comedy", 3}, {"Documentary", 1}, {"Drama", 1},
			{"Horror", 1}, {"Science Fiction", 2}, {"Western", 1}}},
		{adminToken, "?kind=series", []genre{{"Animation", 1}, {"Comedy", 1}}},
		{memberToken, "", []genre{{"Action", 1}, {"Animation", 1}, {"Comedy", 2}, {"Documentary", 1}, {"Drama", 1},
			{"Horror", 1}, {"Science Fiction", 2}}},
		{memberToken, "?kind=movie", []genre{{"Action", 1}, {"Comedy", 1}, {"Documentary", 1}, {"Drama", 1},
			{"Horror", 1}, {"Science Fiction", 2}}},
	} {
		var body struct {
			Genres []genre `json:"genres"`
		}
		decode(t, h.Do(http.MethodGet, "/v1/library/genres"+tc.query, tc.token, ""), &body)
		if !reflect.DeepEqual(body.Genres, tc.want) {
			t.Errorf("%s genres%s = %v, want %v", tc.token, tc.query, body.Genres, tc.want)
		}
	}
	resp := h.Do(http.MethodGet, "/v1/library/genres", "", "")
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("signed-out genres = %d, want 401", resp.StatusCode)
	}
}
