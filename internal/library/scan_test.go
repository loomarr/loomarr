package library

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/demolibrary"
	"github.com/loomarr/loomarr/internal/testkit"
)

func demoClient(t *testing.T, flavor Flavor) (*Client, *testkit.DemoLibrary) {
	t.Helper()
	demo := testkit.NewDemoLibrary(t)
	return New(flavor, demo.URL, testkit.DemoLibraryToken, "dev-1"), demo
}

// RecentlyAdded must sort by DateCreated, request provider ids, and bound the window with
// MinDateLastSaved so a large library costs one call. It parses the /Items shape into
// SearchResults carrying the external ids the scan correlates on.
func TestRecentlyAdded_QueryAndParse(t *testing.T) {
	c, demo := demoClient(t, Emby)
	// The stand-in's catalogue was saved on 2026-01-01; a window from before then sees it all.
	since := time.Date(2025, 7, 23, 12, 0, 0, 0, time.UTC)
	got, err := c.RecentlyAdded(context.Background(), since)
	if err != nil {
		t.Fatal(err)
	}
	q := demo.LastQuery(t)
	if sb := q.Get("SortBy"); !strings.Contains(sb, "DateCreated") {
		t.Errorf("SortBy = %q, want DateCreated", sb)
	}
	if f := q.Get("Fields"); !strings.Contains(f, "ProviderIds") {
		t.Errorf("Fields = %q, must request ProviderIds", f)
	}
	if mds := q.Get("MinDateLastSaved"); mds != "2025-07-23T12:00:00Z" {
		t.Errorf("MinDateLastSaved = %q, want the RFC3339 since", mds)
	}
	if len(got) != len(demolibrary.Catalogue) {
		t.Fatalf("got %d results, want the %d catalogue titles", len(got), len(demolibrary.Catalogue))
	}
	byID := map[string]SearchResult{}
	for _, r := range got {
		byID[r.LibraryItemID] = r
	}
	movie, series := testkit.FirstMovie(), testkit.FirstSeries()
	if r := byID[movie.ID()]; r.TMDBID != movie.ProviderID() || r.MediaType != Movie || r.Year != movie.Year {
		t.Errorf("movie result = %+v, want tmdb %d movie %d", r, movie.ProviderID(), movie.Year)
	}
	if r := byID[series.ID()]; r.TVDBID != series.ProviderID() || r.MediaType != Series {
		t.Errorf("series result = %+v, want tvdb %d series", r, series.ProviderID())
	}
}

// A window that starts after the last save returns nothing: the bound is applied server-side.
func TestRecentlyAdded_WindowAfterLastSaveIsEmpty(t *testing.T) {
	c, _ := demoClient(t, Emby)
	got, err := c.RecentlyAdded(context.Background(), time.Date(2026, 7, 23, 12, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Fatalf("got %d results from a window after the last save, want 0", len(got))
	}
}

// AllItems is the full sweep: same query WITHOUT MinDateLastSaved.
func TestAllItems_NoDateBound(t *testing.T) {
	c, demo := demoClient(t, Emby)
	got, err := c.AllItems(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if _, had := demo.LastQuery(t)["MinDateLastSaved"]; had {
		t.Error("AllItems must not send MinDateLastSaved")
	}
	if len(got) != len(demolibrary.Catalogue) {
		t.Errorf("got %d results, want the %d catalogue titles", len(got), len(demolibrary.Catalogue))
	}
}

// A zero `since` degenerates to no MinDateLastSaved (the whole library) rather than sending an
// epoch-zero timestamp.
func TestRecentlyAdded_ZeroSinceOmitsBound(t *testing.T) {
	c, demo := demoClient(t, Emby)
	if _, err := c.RecentlyAdded(context.Background(), time.Time{}); err != nil {
		t.Fatal(err)
	}
	if _, had := demo.LastQuery(t)["MinDateLastSaved"]; had {
		t.Error("zero since must omit MinDateLastSaved")
	}
}
