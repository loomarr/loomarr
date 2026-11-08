package scan

import (
	"context"

	"github.com/loomarr/loomarr/internal/platform/store"
)

// Collection is one of the household's collections (#2151 decision 28) with its titles in the
// household's order. A member whose title has left the library is not among Titles; it stays a
// member, so the title comes back in its place.
type Collection struct {
	store.LibraryCollection
	Titles []store.LibraryItemSummary
}

// Collections lists the household's collections, by name.
func (l *Library) Collections(ctx context.Context) ([]Collection, error) {
	rows, err := l.CollectionStore.ListLibraryCollections(ctx)
	if err != nil {
		return nil, err
	}
	return l.withTitles(ctx, rows)
}

// Collection reads one collection; store.ErrNotFound when there is none.
func (l *Library) Collection(ctx context.Context, id string) (Collection, error) {
	row, err := l.CollectionStore.LibraryCollection(ctx, id)
	if err != nil {
		return Collection{}, err
	}
	out, err := l.withTitles(ctx, []store.LibraryCollection{row})
	if err != nil {
		return Collection{}, err
	}
	return out[0], nil
}

// CreateCollection saves a new collection of items, in order, and returns its id.
func (l *Library) CreateCollection(ctx context.Context, name, description string, items []string) (string, error) {
	return l.CollectionStore.CreateLibraryCollection(ctx, store.LibraryCollection{
		Name: name, Description: description, Members: items,
	}, l.now())
}

// UpdateCollection replaces a collection's name, description and items; store.ErrNotFound when
// there is none.
func (l *Library) UpdateCollection(ctx context.Context, id, name, description string, items []string) error {
	return l.CollectionStore.UpdateLibraryCollection(ctx, store.LibraryCollection{
		ID: id, Name: name, Description: description, Members: items,
	}, l.now())
}

// DeleteCollection deletes a collection; store.ErrNotFound when there is none. Whether a channel
// still uses it is the caller's to ask first.
func (l *Library) DeleteCollection(ctx context.Context, id string) error {
	return l.CollectionStore.DeleteLibraryCollection(ctx, id)
}

// withTitles reads every collection's titles in one query and puts them in each one's order.
func (l *Library) withTitles(ctx context.Context, rows []store.LibraryCollection) ([]Collection, error) {
	var ids []string
	for _, c := range rows {
		ids = append(ids, c.Members...)
	}
	byID := map[string]store.LibraryItemSummary{}
	if len(ids) > 0 {
		page, err := l.ItemStore.ListLibraryItems(ctx, store.LibraryItemQuery{IDs: ids})
		if err != nil {
			return nil, err
		}
		for _, it := range page.Items {
			byID[it.ID] = it
		}
	}
	out := make([]Collection, 0, len(rows))
	for _, c := range rows {
		titles := []store.LibraryItemSummary{}
		for _, id := range c.Members {
			if it, ok := byID[id]; ok {
				titles = append(titles, it)
			}
		}
		out = append(out, Collection{LibraryCollection: c, Titles: titles})
	}
	return out, nil
}
