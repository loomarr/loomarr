package scan

import (
	"cmp"
	"context"
	"encoding/json"
	"slices"

	mp "github.com/loomarr/loomarr/internal/metadataprovider"
	"github.com/loomarr/loomarr/internal/platform/store"
	"github.com/loomarr/loomarr/internal/platform/store/librarystore"
)

// Set is a ready-made set (#2151 decision 28): a TMDB collection the library has at least one movie
// of. It can't be changed, only copied into the household's own collections.
type Set struct {
	// ID is the TMDB collection id.
	ID   string
	Name string
	// Total is every movie in the set; Titles are the ones in the library, in the set's order.
	Total  int
	Titles []librarystore.LibraryItemSummary
}

// RecordReader reads cached catalog records: the ready-made sets', and a show's episode orders.
type RecordReader interface {
	LibraryCatalogRecord(ctx context.Context, key librarystore.LibraryCatalogKey) (string, bool, error)
}

// record is the cached catalog record for key. ok is false when it hasn't been fetched yet, the
// catalog has no such title, or the cached record can't be read: each says nothing, as it does
// for a title.
func (l *Library) record(ctx context.Context, key librarystore.LibraryCatalogKey) (rec mp.Record, ok bool, err error) {
	if l.Records == nil || key.String() == "" {
		return mp.Record{}, false, nil
	}
	raw, fetched, err := l.Records.LibraryCatalogRecord(ctx, key)
	if err != nil || !fetched || raw == "" {
		return mp.Record{}, false, err
	}
	if err := json.Unmarshal([]byte(raw), &rec); err != nil || rec.Descriptive == nil {
		return mp.Record{}, false, nil //nolint:nilerr // an unreadable cached record is skipped, not fatal
	}
	return rec, true, nil
}

// Sets lists the ready-made sets the library has a movie of, by name. A set whose own record has
// not been fetched yet is left out until it has.
func (l *Library) Sets(ctx context.Context) ([]Set, error) {
	if l.Records == nil {
		return []Set{}, nil
	}
	page, err := l.ItemStore.ListLibraryItems(ctx, librarystore.LibraryItemQuery{Kind: string(kindMovie), InSet: true})
	if err != nil {
		return nil, err
	}
	owned := map[librarystore.LibraryCatalogKey][]librarystore.LibraryItemSummary{}
	var keys []librarystore.LibraryCatalogKey
	for _, it := range page.Items {
		if _, seen := owned[it.Set]; !seen {
			keys = append(keys, it.Set)
		}
		owned[it.Set] = append(owned[it.Set], it)
	}
	out := []Set{}
	for _, key := range keys {
		set, ok, err := l.set(ctx, key, owned[key])
		if err != nil {
			return nil, err
		}
		if ok {
			out = append(out, set)
		}
	}
	slices.SortFunc(out, func(a, b Set) int { return cmp.Or(cmp.Compare(a.Name, b.Name), cmp.Compare(a.ID, b.ID)) })
	return out, nil
}

// CopySet makes a collection of the household's own from a ready-made set: its name and the
// library's movies of it, in its order. It returns the new collection's id; store.ErrNotFound
// when the library has no movie of the set.
func (l *Library) CopySet(ctx context.Context, id string) (string, error) {
	sets, err := l.Sets(ctx)
	if err != nil {
		return "", err
	}
	for _, s := range sets {
		if s.ID != id {
			continue
		}
		items := make([]string, 0, len(s.Titles))
		for _, it := range s.Titles {
			items = append(items, it.ID)
		}
		return l.CreateCollection(ctx, s.Name, "", items)
	}
	return "", store.ErrNotFound
}

// set reads one set's record and puts the library's movies of it in its order. A movie the record
// doesn't list (the set changed since) goes last, by year.
func (l *Library) set(ctx context.Context, key librarystore.LibraryCatalogKey, titles []librarystore.LibraryItemSummary) (Set, bool, error) {
	rec, ok, err := l.record(ctx, key)
	if err != nil || !ok {
		return Set{}, false, err
	}
	// A movie's place is found by the catalog row its details come from, the key its part names.
	place := map[librarystore.LibraryCatalogKey]int{}
	for i, part := range rec.Children {
		if key := l.Keys.For(mp.KindMovie, part.IDs); key.String() != "" {
			place[key] = i
		}
	}
	at := func(it librarystore.LibraryItemSummary) int {
		if i, ok := place[it.Catalog]; ok {
			return i
		}
		return len(rec.Children)
	}
	ordered := slices.Clone(titles)
	slices.SortStableFunc(ordered, func(a, b librarystore.LibraryItemSummary) int {
		return cmp.Or(cmp.Compare(at(a), at(b)), cmp.Compare(a.Year, b.Year), cmp.Compare(a.ID, b.ID))
	})
	return Set{
		ID: key.ExternalID, Name: rec.Descriptive.Title,
		Total: max(len(rec.Children), len(titles)), Titles: ordered,
	}, true, nil
}
