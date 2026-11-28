package store

import (
	"context"
	"time"

	"github.com/loomarr/loomarr/internal/library/inventory"
	"github.com/loomarr/loomarr/internal/platform/store/storedb"
)

// libraryProjection is the inventory cluster's view of the projector ProjectLibraryWith installs.
type libraryProjection struct{ s *sqlStore }

func (p libraryProjection) Projecting() bool { return p.s.projecting() }

func (p libraryProjection) Project(ctx context.Context, tx storedb.Tx, at time.Time, items []inventory.Item) error {
	return p.s.projectLibrary(ctx, tx, at, items)
}
