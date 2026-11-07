package scan

import (
	"context"
	"errors"
	"testing"
	"time"

	mp "github.com/loomarr/loomarr/internal/metadataprovider"
	"github.com/loomarr/loomarr/internal/store"
	"github.com/loomarr/loomarr/internal/tmdb"
)

// fakeCatalog answers fetches from a table; an unknown key is not found, and err fails every fetch.
type fakeCatalog struct {
	records map[string]mp.Record
	err     error
	fetched []string
}

func (f *fakeCatalog) ID() mp.ID { return mp.IDTMDB }

func (f *fakeCatalog) Capabilities() mp.Capabilities {
	return mp.Capabilities{Basis: mp.BasisCatalog, Catalog: mp.NamespaceTMDB}
}

func (f *fakeCatalog) Fetch(_ context.Context, ref mp.Ref) (mp.Record, error) {
	f.fetched = append(f.fetched, string(ref.Kind)+":"+ref.ID.Value)
	if f.err != nil {
		return mp.Record{}, f.err
	}
	rec, ok := f.records[ref.ID.Value]
	if !ok {
		return mp.Record{}, mp.ErrNotFound
	}
	return rec, nil
}

// A title grounded by an id in its name is written from its file name first; the Enricher
// fetches its catalog record, and the title is written again with the catalog's details. A
// record fetched once is not fetched again until the refresh, and a catalog with no key fetches
// nothing and holds nothing back.
func TestEnricherWritesTheCatalogsDetails(t *testing.T) {
	ctx := context.Background()
	h := newHarness(t)
	h.im.Catalog = h.st
	h.file("Golf (2013) {tmdb-90000201}/Golf (2013) {tmdb-90000201}.mkv", 10)
	h.run()

	page, err := h.st.ListLibraryItems(ctx, store.LibraryItemQuery{})
	if err != nil || len(page.Items) != 1 {
		t.Fatalf("items = %+v, %v", page.Items, err)
	}
	if page.Items[0].Name != "Golf (2013)" {
		t.Fatalf("before the fetch the title is named %q, want its file name", page.Items[0].Name)
	}

	catalog := &fakeCatalog{records: map[string]mp.Record{"90000201": {
		Source: mp.Source{Provider: mp.IDTMDB, Basis: mp.BasisCatalog, Catalog: mp.NamespaceTMDB},
		Kind:   mp.KindMovie, IDs: []mp.ExternalID{{Namespace: mp.NamespaceTMDB, Value: "90000201"}},
		Descriptive: &mp.Descriptive{Title: "Golf Course", Overview: "A round that never ends.", Year: 2013},
	}}}
	var rewrites [][]string
	e := &Enricher{
		Store: h.st, Provider: catalog,
		Rewrite: func(root string, dirs ...string) {
			rewrites = append(rewrites, dirs)
			if err := h.im.RunFolders(ctx, root, dirs); err != nil {
				t.Fatal(err)
			}
		},
	}
	if err := e.Tick(ctx); err != nil {
		t.Fatal(err)
	}
	if len(catalog.fetched) != 1 || catalog.fetched[0] != "movie:90000201" || len(rewrites) != 1 {
		t.Fatalf("fetched %v, rewrote %v", catalog.fetched, rewrites)
	}
	page, err = h.st.ListLibraryItems(ctx, store.LibraryItemQuery{})
	if err != nil || len(page.Items) != 1 || page.Items[0].Name != "Golf Course" {
		t.Fatalf("after the fetch items = %+v, %v", page.Items, err)
	}
	origin := fileOrigin(h.root, "Golf (2013) {tmdb-90000201}/Golf (2013) {tmdb-90000201}.mkv")
	if got := h.origin(h.item(origin), origin).Observation.Facts.Overview; got != "A round that never ends." {
		t.Fatalf("overview = %q", got)
	}

	// Fetched: nothing is due until the refresh.
	if err := e.Tick(ctx); err != nil || len(catalog.fetched) != 1 {
		t.Fatalf("second tick fetched %v, %v", catalog.fetched, err)
	}

	// A new title while TMDB has no key: it is asked for, nothing is held back, and it is fetched
	// once the key is set.
	h.file("Hotel (2014) {tmdb-90000202}/Hotel (2014) {tmdb-90000202}.mkv", 10)
	h.run()
	catalog.err = tmdb.ErrAPIKeyRequired
	if err := e.Tick(ctx); err != nil {
		t.Fatal(err)
	}
	catalog.err = nil
	if err := e.Tick(ctx); err != nil {
		t.Fatal(err)
	}
	if last := catalog.fetched[len(catalog.fetched)-1]; last != "movie:90000202" {
		t.Fatalf("fetched %v, want the new title once the key is set", catalog.fetched)
	}

	// A catalog that fails holds that title back, without failing the tick.
	h.file("India (2015) {tmdb-90000203}/India (2015) {tmdb-90000203}.mkv", 10)
	h.run()
	catalog.err = errors.New("tmdb: 503")
	if err := e.Tick(ctx); err != nil {
		t.Fatal(err)
	}
	catalog.err = nil
	before := len(catalog.fetched)
	if err := e.Tick(ctx); err != nil || len(catalog.fetched) != before {
		t.Fatalf("a held-back title was fetched again at once: %v, %v", catalog.fetched[before:], err)
	}
	e.Now = func() time.Time { return time.Now().Add(enrichRetry + time.Minute) }
	if err := e.Tick(ctx); err != nil || len(catalog.fetched) != before+1 {
		t.Fatalf("after the hold the title was not fetched: %v, %v", catalog.fetched[before:], err)
	}
}
