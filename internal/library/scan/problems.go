package scan

import (
	"context"

	"github.com/loomarr/loomarr/internal/platform/store/librarystore"
)

// Problems lists the titles that need a look (#2151 decision 24), by name: files that can't be
// played, or a movie's extra copies, which never play and take space (ExtraBytes).
func (l *Library) Problems(ctx context.Context) ([]librarystore.LibraryItemSummary, error) {
	page, err := l.ItemStore.ListLibraryItems(ctx, librarystore.LibraryItemQuery{Problems: true})
	return page.Items, err
}
