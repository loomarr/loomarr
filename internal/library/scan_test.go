package library

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

// RecentlyAdded must sort by DateCreated, request provider ids, and bound the window with
// MinDateLastSaved so a large library costs one call. It parses the /Items shape into
// SearchResults carrying the external ids the scan correlates on.
func TestRecentlyAdded_QueryAndParse(t *testing.T) {
	var gotQuery url.Values
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.Query()
		_, _ = w.Write([]byte(`{"Items":[
			{"Id":"lib-1","Name":"The Matrix","Type":"Movie","ProductionYear":1999,
			 "ProviderIds":{"Tmdb":"603"}},
			{"Id":"lib-2","Name":"Breaking Bad","Type":"Series","ProductionYear":2008,
			 "ProviderIds":{"Tvdb":"81189"}}
		],"TotalRecordCount":2}`))
	}))
	defer srv.Close()

	c := New(Emby, srv.URL, "tok", "dev-1")
	since := time.Date(2026, 7, 23, 12, 0, 0, 0, time.UTC)
	got, err := c.RecentlyAdded(context.Background(), since)
	if err != nil {
		t.Fatal(err)
	}
	if sb := gotQuery.Get("SortBy"); !strings.Contains(sb, "DateCreated") {
		t.Errorf("SortBy = %q, want DateCreated", sb)
	}
	if f := gotQuery.Get("Fields"); !strings.Contains(f, "ProviderIds") {
		t.Errorf("Fields = %q, must request ProviderIds", f)
	}
	if mds := gotQuery.Get("MinDateLastSaved"); mds != "2026-07-23T12:00:00Z" {
		t.Errorf("MinDateLastSaved = %q, want the RFC3339 since", mds)
	}
	if len(got) != 2 {
		t.Fatalf("got %d results, want 2", len(got))
	}
	if got[0].TMDBID != 603 || got[0].MediaType != Movie || got[0].LibraryItemID != "lib-1" {
		t.Errorf("movie result = %+v", got[0])
	}
	if got[1].TVDBID != 81189 || got[1].MediaType != Series {
		t.Errorf("series result = %+v", got[1])
	}
}

// AllItems is the full sweep: same query WITHOUT MinDateLastSaved.
func TestAllItems_NoDateBound(t *testing.T) {
	var hadMDS bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, hadMDS = r.URL.Query()["MinDateLastSaved"]
		_, _ = w.Write([]byte(`{"Items":[{"Id":"x","Name":"n","Type":"Movie","ProviderIds":{"Tmdb":"1"}}],"TotalRecordCount":1}`))
	}))
	defer srv.Close()

	c := New(Emby, srv.URL, "tok", "dev-1")
	got, err := c.AllItems(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if hadMDS {
		t.Error("AllItems must not send MinDateLastSaved")
	}
	if len(got) != 1 || got[0].TMDBID != 1 {
		t.Errorf("got %+v", got)
	}
}

// A zero `since` degenerates to no MinDateLastSaved (the whole library) rather than sending an
// epoch-zero timestamp.
func TestRecentlyAdded_ZeroSinceOmitsBound(t *testing.T) {
	var hadMDS bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, hadMDS = r.URL.Query()["MinDateLastSaved"]
		_, _ = w.Write([]byte(`{"Items":[],"TotalRecordCount":0}`))
	}))
	defer srv.Close()

	c := New(Emby, srv.URL, "tok", "dev-1")
	if _, err := c.RecentlyAdded(context.Background(), time.Time{}); err != nil {
		t.Fatal(err)
	}
	if hadMDS {
		t.Error("zero since must omit MinDateLastSaved")
	}
}
