package scan

import (
	"context"
	"errors"
	"slices"
	"testing"

	mp "github.com/loomarr/loomarr/internal/metadataprovider"
	"github.com/loomarr/loomarr/internal/platform/store"
)

// A movie whose catalog record names its set asks for the set's own record; once fetched, the set
// is listed with how many of its movies the library has, in its order, and can be copied into the
// household's own collections.
func TestReadyMadeSets(t *testing.T) {
	ctx := context.Background()
	h := newHarness(t)
	h.im.Catalog = h.st
	h.file("Lantern Keeper Returns (2004) {tmdb-90000613}/Returns.mkv", 10)
	h.file("Lantern Keeper (2001) {tmdb-90000612}/Keeper.mkv", 10)
	h.file("Harbour (2010) {tmdb-90000620}/Harbour.mkv", 10)
	h.run()

	source := mp.Source{Provider: mp.IDTMDB, Basis: mp.BasisCatalog, Catalog: mp.NamespaceTMDB}
	set := &mp.CollectionRef{ID: mp.ExternalID{Namespace: mp.NamespaceTMDBCollection, Value: "90000611"}, Name: "Lantern Keeper Collection"}
	movie := func(id, title string, year int, in *mp.CollectionRef) mp.Record {
		return mp.Record{Source: source, Kind: mp.KindMovie, IDs: []mp.ExternalID{{Namespace: mp.NamespaceTMDB, Value: id}},
			Descriptive: &mp.Descriptive{Title: title, Year: year, Collection: in}}
	}
	catalog := &fakeCatalog{records: map[string]mp.Record{
		"90000612": movie("90000612", "The Lantern Keeper", 2001, set),
		"90000613": movie("90000613", "The Lantern Keeper Returns", 2004, set),
		"90000620": movie("90000620", "Harbour", 2010, nil),
		"90000611": {Source: source, Kind: mp.KindCollection,
			IDs:         []mp.ExternalID{set.ID},
			Descriptive: &mp.Descriptive{Title: "The Lantern Keeper Collection"},
			Children: []mp.Record{
				movie("90000612", "The Lantern Keeper", 2001, nil),
				movie("90000613", "The Lantern Keeper Returns", 2004, nil),
				movie("90000614", "The Lantern Keeper Forever", 2009, nil),
			}},
	}}
	e := &Enricher{Store: h.st, Provider: catalog, Rewrite: func(root string, dirs ...string) {
		if err := h.im.RunFolders(ctx, root, dirs); err != nil {
			t.Fatal(err)
		}
	}}
	lib := &Library{ItemStore: h.st, CollectionStore: h.st, Records: h.st, Keys: fakeCatalogKeys}

	// The movies' records arrive first: their set is asked for, and listed only once it arrives.
	if err := e.Tick(ctx); err != nil {
		t.Fatal(err)
	}
	if sets, err := lib.Sets(ctx); err != nil || len(sets) != 0 {
		t.Fatalf("sets before the set's record = %+v, %v", sets, err)
	}
	if err := e.Tick(ctx); err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(catalog.fetched, "collection:90000611") {
		t.Fatalf("fetched %v, want the set", catalog.fetched)
	}

	sets, err := lib.Sets(ctx)
	if err != nil || len(sets) != 1 {
		t.Fatalf("sets = %+v, %v", sets, err)
	}
	s := sets[0]
	var names []string
	for _, it := range s.Titles {
		names = append(names, it.Name)
	}
	if s.ID != "90000611" || s.Name != "The Lantern Keeper Collection" || s.Total != 3 ||
		!slices.Equal(names, []string{"The Lantern Keeper", "The Lantern Keeper Returns"}) {
		t.Fatalf("set = %+v (titles %v), want 2 of 3 in release order", s, names)
	}

	id, err := lib.CopySet(ctx, "90000611")
	if err != nil {
		t.Fatal(err)
	}
	c, err := lib.Collection(ctx, id)
	if err != nil || c.Name != "The Lantern Keeper Collection" || len(c.Titles) != 2 || c.Titles[0].Name != "The Lantern Keeper" {
		t.Fatalf("copy = %+v, %v", c, err)
	}
	if _, err := lib.CopySet(ctx, "90000699"); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("copying a set the library has none of: %v, want ErrNotFound", err)
	}
}
