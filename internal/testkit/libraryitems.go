package testkit

import (
	"context"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/platform/store/librarystore"
)

// LibraryItemSaver is the store a fixture writes library items through.
type LibraryItemSaver interface {
	SaveLibraryItems(ctx context.Context, items []librarystore.LibraryItem, at time.Time) error
}

// SaveLibraryItems stores a bare movie for each id, so a title, airing or scheduled airing that
// names it satisfies its foreign key to library_items (#2793). A fixture whose library is a fake
// calls it with the ids the fake hands out; "" is skipped, since a title not in the library names
// no item.
func SaveLibraryItems(t testing.TB, s LibraryItemSaver, ids ...string) {
	t.Helper()
	items := make([]librarystore.LibraryItem, 0, len(ids))
	for _, id := range ids {
		if id != "" {
			items = append(items, librarystore.LibraryItem{ID: id, Kind: "movie", Name: id, SortName: id})
		}
	}
	if err := s.SaveLibraryItems(context.Background(), items, time.Unix(1, 0)); err != nil {
		t.Fatalf("save library items %v: %v", ids, err)
	}
}
