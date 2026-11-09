package archive_test

import (
	"context"
	"log/slog"
	"path/filepath"
	"slices"
	"testing"

	"github.com/loomarr/loomarr/internal/filler/fetch"
	"github.com/loomarr/loomarr/internal/filler/source"
	"github.com/loomarr/loomarr/internal/filler/source/archive"
)

// askingCatalog lists items without runtimes, as Archive.org's one-request search does, and fills
// them in only when asked for per-item metadata, remembering which items it was asked about.
type askingCatalog struct {
	runtimes map[string]int
	listed   []string
	// asked is every item id whose metadata was read, across calls.
	asked []string
}

func (f *askingCatalog) EnumerateCollection(context.Context, string, int) (fetch.DiscoveryResult, error) {
	var out fetch.DiscoveryResult
	for _, id := range f.listed {
		out.Items = append(out.Items, fetch.DiscoveredItem{ID: id})
	}
	out.Total = len(out.Items)
	return out, nil
}

func (f *askingCatalog) SearchCollection(context.Context, string, string, int) (fetch.DiscoveryResult, error) {
	return fetch.DiscoveryResult{}, nil
}

func (f *askingCatalog) Enrich(_ context.Context, items []fetch.DiscoveredItem) {
	for i := range items {
		f.asked = append(f.asked, items[i].ID)
		items[i].DurationMS = f.runtimes[items[i].ID]
		if items[i].DurationMS > 0 {
			items[i].Height = 480
		}
	}
}

// #1773 (supervisor decision): each Archive.org item's runtime is read once and remembered on
// disk, so a later pull (and a restarted server) reuses it instead of paying Archive.org's
// per-item throttle again. Only what is still unknown is asked for.
func TestProvider_RuntimesAreReadOnce(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".loomarr-remote-runtimes.json")
	catalog := &askingCatalog{runtimes: map[string]int{"reel": 30 * 60 * 1000, "spot": 45 * 1000}, listed: []string{"reel", "spot", "unprobed"}}
	enumerate := func() map[string]source.Item {
		t.Helper()
		// A fresh cache over the same file each time: what a restarted server would see.
		cached := archive.NewCachedCatalog(catalog, archive.NewRuntimeCache(path, slog.New(slog.DiscardHandler)))
		provider := archive.New(archive.Config{Catalog: cached, Runtimes: func() bool { return true }})
		listing, err := provider.List(t.Context(), source.Source{ID: "s", URI: "reels_collection"},
			source.ListRequest{Take: 10, List: 10})
		if err != nil {
			t.Fatal(err)
		}
		got := map[string]source.Item{}
		for _, item := range listing.Items {
			got[item.ID] = item
		}
		return got
	}

	enumerate()
	if !slices.Equal(catalog.asked, []string{"reel", "spot", "unprobed"}) {
		t.Fatalf("first listing asked for %v, want every item", catalog.asked)
	}
	catalog.asked = nil
	second := enumerate()
	// A runtime Archive.org could not give is asked for again: it may have been probed since.
	if !slices.Equal(catalog.asked, []string{"unprobed"}) {
		t.Errorf("second listing asked for %v, want only the item whose runtime is still unknown", catalog.asked)
	}
	if second["reel"].DurationMS != 30*60*1000 || second["spot"].DurationMS != 45*1000 || second["spot"].Height != 480 {
		t.Errorf("remembered items came back as reel=%+v spot=%+v, want their runtimes and quality", second["reel"], second["spot"])
	}
}
