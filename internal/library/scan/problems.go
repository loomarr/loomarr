package scan

import (
	"context"

	"github.com/loomarr/loomarr/internal/platform/store"
)

// Problems lists the titles that need a look (#2151 decision 24), by name: files that can't be
// played, or a movie's extra copies, which never play and take space (ExtraBytes).
func (l *Library) Problems(ctx context.Context) ([]store.LibraryItemSummary, error) {
	page, err := l.ItemStore.ListLibraryItems(ctx, store.LibraryItemQuery{Problems: true})
	return page.Items, err
}
