package store

import (
	"context"
	"database/sql"
	"time"

	"github.com/loomarr/loomarr/internal/library/inventory"
	"github.com/loomarr/loomarr/internal/platform/store/librarystore"
)

// LibraryProjector says how an inventory item is listed in the Library (#2151 decision 43): its
// library_items row, or false when it has none. It is pure, and the scanner supplies it, since
// only the scanner can read what it wrote in an origin's extension.
type LibraryProjector func(inventory.Item) (LibraryItem, bool)

// LibraryProjection is how the Library's rows are kept in step with the inventory.
type LibraryProjection interface {
	// ProjectLibraryWith installs the projector. From then on each inventory write that changes
	// what it reads (a snapshot, an origin marked unseen) rewrites the items it touched in its own
	// transaction, whoever writes. Before it is installed nothing is projected.
	ProjectLibraryWith(LibraryProjector)
}

func (s *sqlStore) ProjectLibraryWith(p LibraryProjector) { s.libraryProjector.Store(&p) }

// projecting reports whether a projector is installed, so an inventory write can skip reading the
// items it would hand over.
func (s *sqlStore) projecting() bool {
	p := s.libraryProjector.Load()
	return p != nil && *p != nil
}

// projectLibrary rewrites the library rows of each item in tx. The items come from the inventory
// write that changed them, so this side never reads the inventory. An item the inventory no longer
// has is not passed in, and one the projector does not list keeps whatever row it had: presence
// comes from library_files, and a title that comes back finds its row, its collections and its
// answer.
func (s *sqlStore) projectLibrary(ctx context.Context, tx *sql.Tx, at time.Time, items []inventory.Item) error {
	p := s.libraryProjector.Load()
	if p == nil || *p == nil {
		return nil
	}
	for _, item := range items {
		row, ok := (*p)(item)
		if !ok {
			continue
		}
		if err := librarystore.SaveItemIn(ctx, s.libraryCluster, tx, row, at); err != nil {
			return err
		}
	}
	return nil
}
