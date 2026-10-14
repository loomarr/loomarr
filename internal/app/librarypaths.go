package app

import (
	"context"

	"github.com/loomarr/loomarr/internal/store"
)

// libraryPathCache adapts the store's path table to library.PathCache (#1456), so playout
// resolves a scheduled item's file without a media-server request at airtime.
type libraryPathCache struct{ st store.LibraryPathStore }

func (p libraryPathCache) ItemPath(ctx context.Context, itemID string) (string, bool, error) {
	return p.st.LibraryItemPath(ctx, itemID)
}

func (p libraryPathCache) SetItemPath(ctx context.Context, itemID, serverPath string) error {
	return p.st.SetLibraryItemPath(ctx, itemID, serverPath)
}
