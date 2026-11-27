package api_test

import (
	"context"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/platform/store/librarystore"
)

// The wall's filters, their choices and a person's titles over HTTP (#4476): a member reads all
// of it, and nothing a title waiting in Needs you carries reaches them through any of it.
func TestLibraryFiltersFacetsAndPeople(t *testing.T) {
	h := newLibraryHarness(t)
	ctx := context.Background()
	at := time.UnixMilli(1_790_000_000_000).UTC()
	title := func(id, name, studio, person, language string, grounded bool) librarystore.LibraryItem {
		it := librarystore.LibraryItem{ID: id, Kind: "movie", Name: name, Studios: []string{studio},
			Origin:  librarystore.LibraryOrigin{Authority: "library:/media", Key: "/media/" + id},
			Credits: []librarystore.LibraryCredit{{Name: person, Role: "actor", Character: "Lead"}},
			Sources: []librarystore.LibrarySource{{Path: "/media/" + id + ".mkv", Plays: true, Streams: []librarystore.LibraryStream{
				{Kind: "video", Resolution: "4K", HDR: "Dolby Vision"},
				{Index: 1, Kind: "audio", Language: language, LanguageName: language + " name"},
				{Index: 2, Kind: "subtitle", Language: language, LanguageName: language + " name"},
			}}},
		}
		if grounded {
			it.Match, it.From = librarystore.LibraryMatch{Basis: "embedded_id"}, librarystore.LibraryFrom{Name: "tmdb"}
		}
		return it
	}
	// A person's id may hold a slash: an unlinked person is kept by name.
	items := []librarystore.LibraryItem{
		title("90000201", "Alpha", "Larkfield", "Ada/Quill", "fr", true),
		title("90000202", "folder name", "Hidden House", "Unseen Person", "ja", false),
	}
	// A person a catalog identifies, whose title carries the credit by that id.
	items[0].Credits = append(items[0].Credits, librarystore.LibraryCredit{Name: "Bram Fen", Role: "director",
		ID: librarystore.LibraryID{Namespace: "tmdb", Value: "90000901"}})
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
	body := func(route, token string, want int) string {
		t.Helper()
		resp := h.Do(http.MethodGet, route, token, "")
		if resp.StatusCode != want {
			t.Errorf("%s = %d, want %d", route, resp.StatusCode, want)
		}
		return readBody(t, resp)
	}

	for query, want := range map[string]string{
		"studio=larkfield": "Alpha",
		"studio=nowhere":   "",
		"person=" + url.QueryEscape("name:Ada/Quill"): "Alpha",
		"person=nobody":                          "",
		"audio=fr":                               "Alpha",
		"subtitles=fr":                           "Alpha",
		"uhd=true":                               "Alpha",
		"hdr=any":                                "Alpha",
		"hdr=" + url.QueryEscape("Dolby Vision"): "Alpha",
		"hdr=HLG":                                "",
		"uhd=true&audio=ja":                      "",
	} {
		values, err := url.ParseQuery(query)
		if err != nil {
			t.Fatal(err)
		}
		if p := wallList(t, h, memberToken, values); wallNames(p) != want || p.Total != len(strings.Fields(want)) {
			t.Errorf("member %s = %q (total %d), want %q", query, wallNames(p), p.Total, want)
		}
	}

	facets := body("/v1/library/facets", memberToken, http.StatusOK)
	for _, want := range []string{`"Larkfield"`, `"fr name"`, `"uhd":1`, `"hdrAny":1`, `"Dolby Vision"`} {
		if !strings.Contains(facets, want) {
			t.Errorf("member facets lack %s: %s", want, facets)
		}
	}
	people := body("/v1/library/people?q=a", memberToken, http.StatusOK)
	person := body("/v1/library/people/"+url.PathEscape("name:Ada/Quill"), memberToken, http.StatusOK)
	detail := body("/v1/library/items/90000201", memberToken, http.StatusOK)
	for what, got := range map[string]string{"people": people, "person": person, "title": detail} {
		if !strings.Contains(got, `name:Ada/Quill`) {
			t.Errorf("member %s lacks the person's id: %s", what, got)
		}
	}
	for route, got := range map[string]string{"facets": facets, "people": people, "person": person} {
		assertMemberSafe(t, route, got)
		for _, hidden := range []string{"Hidden House", "Unseen Person", "ja name", "/media", ".mkv", "folder name"} {
			if strings.Contains(got, hidden) {
				t.Errorf("member %s carries %q: %s", route, hidden, got)
			}
		}
	}
	// A search only a hidden title's person matches finds them for an admin alone.
	if got := body("/v1/library/people?q=unseen", memberToken, http.StatusOK); !strings.Contains(got, `"people":[]`) {
		t.Errorf("member search for a hidden title's person = %s", got)
	}
	if got := body("/v1/library/people?q=unseen", adminToken, http.StatusOK); !strings.Contains(got, "Unseen Person") {
		t.Errorf("admin search for the same person = %s", got)
	}
	// A linked person's credit carries their catalog id, which reads them and lists their titles.
	linked := "tmdb:90000901"
	if !strings.Contains(detail, `"personId":"`+linked+`"`) {
		t.Errorf("title lacks the linked person's id: %s", detail)
	}
	body("/v1/library/people/"+linked, memberToken, http.StatusOK)
	if p := wallList(t, h, memberToken, url.Values{"person": {linked}}); wallNames(p) != "Alpha" {
		t.Errorf("linked person's titles = %q", wallNames(p))
	}
	body("/v1/library/people/"+url.PathEscape("name:Unseen Person"), memberToken, http.StatusNotFound)
	body("/v1/library/people/"+url.PathEscape("name:Unseen Person"), adminToken, http.StatusOK)
	for _, route := range []string{"/v1/library/facets", "/v1/library/people", "/v1/library/people/nobody"} {
		body(route, "", http.StatusUnauthorized)
	}
}
