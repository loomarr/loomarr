package scan

import (
	"cmp"
	"context"
	"encoding/json"
	"slices"

	mp "github.com/loomarr/loomarr/internal/metadataprovider"
	"github.com/loomarr/loomarr/internal/platform/store"
)

// Set is a ready-made set (#2151 decision 28): a TMDB collection the library has at least one movie
// of. It can't be changed, only copied into the household's own collections.
type Set struct {
	// ID is the TMDB collection id.
	ID   string
	Name string
	// Total is every movie in the set; Titles are the ones in the library, in the set's order.
	Total  int
	Titles []store.LibraryItemSummary
}

// SetRecords reads the cached catalog records of sets.
type SetRecords interface {
	LibraryCatalogRecord(ctx context.Context, key store.LibraryCatalogKey) (string, bool, error)
}

// Sets lists the ready-made sets the library has a movie of, by name. A set whose own record has
// not been fetched yet is left out until it has.
func (l *Library) Sets(ctx context.Context) ([]Set, error) {
	if l.SetRecords == nil {
		return []Set{}, nil
	}
	page, err := l.ItemStore.ListLibraryItems(ctx, store.LibraryItemQuery{Kind: string(kindMovie), InSet: true})
	if err != nil {
		return nil, err
	}
	owned := map[store.LibraryCatalogKey][]store.LibraryItemSummary{}
	var keys []store.LibraryCatalogKey
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
func (l *Library) set(ctx context.Context, key store.LibraryCatalogKey, titles []store.LibraryItemSummary) (Set, bool, error) {
	raw, fetched, err := l.SetRecords.LibraryCatalogRecord(ctx, key)
	if err != nil || !fetched || raw == "" {
		return Set{}, false, err
	}
	var rec mp.Record
	if err := json.Unmarshal([]byte(raw), &rec); err != nil || rec.Descriptive == nil {
		// A record that can't be read says nothing, as it does for a title.
		return Set{}, false, nil //nolint:nilerr // an unreadable cached record is skipped, not fatal
	}
	// A movie's place is found by the catalog row its details come from, the key its part names.
	place := map[store.LibraryCatalogKey]int{}
	for i, part := range rec.Children {
		if key := catalogKey(mp.KindMovie, part.IDs); key.String() != "" {
			place[key] = i
		}
	}
	at := func(it store.LibraryItemSummary) int {
		if i, ok := place[it.Catalog]; ok {
			return i
		}
		return len(rec.Children)
	}
	ordered := slices.Clone(titles)
	slices.SortStableFunc(ordered, func(a, b store.LibraryItemSummary) int {
		return cmp.Or(cmp.Compare(at(a), at(b)), cmp.Compare(a.Year, b.Year), cmp.Compare(a.ID, b.ID))
	})
	return Set{
		ID: key.ExternalID, Name: rec.Descriptive.Title,
		Total: max(len(rec.Children), len(titles)), Titles: ordered,
	}, true, nil
}
