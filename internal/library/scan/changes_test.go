package scan

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	mp "github.com/loomarr/loomarr/internal/library/metadataprovider"
	"github.com/loomarr/loomarr/internal/platform/store/librarystore"
)

// fakeFeed answers Changes from a list and notes each time it was asked from.
type fakeFeed struct {
	changed []mp.Ref
	err     error
	asked   []time.Time
}

func (f *fakeFeed) Window() time.Duration { return 14 * 24 * time.Hour }

func (f *fakeFeed) Changes(_ context.Context, since time.Time) ([]mp.Ref, error) {
	f.asked = append(f.asked, since)
	return f.changed, f.err
}

// A record TMDB changed is fetched again within one nightly read, ahead of the refresh, and an
// untouched one is not. The first read only starts the cursor, a failed read keeps it, and a
// cursor older than the feed's window moves on without marking anything: the refresh catches up.
func TestChangeFollowerRefetchesWhatTheCatalogChanged(t *testing.T) {
	ctx := context.Background()
	h := newHarness(t)
	night := time.UnixMilli(1_800_000_000_000)
	edited := librarystore.LibraryCatalogKey{Provider: "tmdb", Kind: "movie", ExternalID: "90000201"}
	untouched := librarystore.LibraryCatalogKey{Provider: "tmdb", Kind: "movie", ExternalID: "90000202"}
	if err := h.st.WantLibraryCatalog(ctx, []librarystore.LibraryCatalogKey{edited, untouched}); err != nil {
		t.Fatal(err)
	}
	for _, k := range []librarystore.LibraryCatalogKey{edited, untouched} {
		if _, err := h.st.SaveLibraryCatalog(ctx, k, `{"Kind":"movie"}`, catalogVersions[k.Kind], "", night); err != nil {
			t.Fatal(err)
		}
	}
	feed := &fakeFeed{changed: []mp.Ref{
		{Kind: mp.KindMovie, ID: mp.ExternalID{Namespace: mp.NamespaceTMDB, Value: "90000201"}},
		// Most of what TMDB changed is not in this library.
		{Kind: mp.KindMovie, ID: mp.ExternalID{Namespace: mp.NamespaceTMDB, Value: "90009999"}},
	}}
	now := night
	follower := &ChangeFollower{Store: h.st, Feed: feed, Catalog: "tmdb", Now: func() time.Time { return now }}
	catalog := &fakeCatalog{records: map[string]mp.Record{
		"90000201": {Kind: mp.KindMovie}, "90000202": {Kind: mp.KindMovie},
	}}
	enricher := &Enricher{Store: h.st, Provider: catalog, Rewrite: func(string, ...string) {},
		Now: func() time.Time { return now }}
	tick := func() {
		t.Helper()
		if err := follower.Tick(ctx); err != nil {
			t.Fatal(err)
		}
		if err := enricher.Tick(ctx); err != nil {
			t.Fatal(err)
		}
	}

	tick() // starts the cursor
	now = night.Add(time.Hour)
	tick() // not a day yet
	if len(feed.asked) != 0 || len(catalog.fetched) != 0 {
		t.Fatalf("feed asked %v, fetched %v before a day passed; want neither", feed.asked, catalog.fetched)
	}

	now = night.Add(ChangeEvery)
	feed.err = errors.New("tmdb: 503")
	if err := follower.Tick(ctx); err == nil {
		t.Fatal("a failed read reports its error")
	}
	feed.err = nil
	tick()
	if len(feed.asked) != 2 || !feed.asked[1].Equal(night) {
		t.Fatalf("feed asked from %v, want the night it was last read through, again after the failure", feed.asked)
	}
	if !slices.Equal(catalog.fetched, []string{"movie:90000201"}) {
		t.Fatalf("fetched %v, want only the record TMDB changed", catalog.fetched)
	}
	tick()
	if len(catalog.fetched) != 1 {
		t.Fatalf("fetched %v; a refetched record is no longer changed", catalog.fetched)
	}

	now = now.Add(20 * 24 * time.Hour)
	tick()
	if len(feed.asked) != 2 {
		t.Errorf("feed asked from %v after 20 days off; past its window there is nothing to ask", feed.asked[2:])
	}
	if read, _, err := h.st.LibraryCatalogFeedRead(ctx, "tmdb"); err != nil || !read.Equal(now) {
		t.Errorf("cursor = %v, %v; want it moved on to now", read, err)
	}
}
