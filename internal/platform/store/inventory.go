package store

import (
	"context"
	"time"

	"github.com/loomarr/loomarr/internal/library/inventory"
	"github.com/loomarr/loomarr/internal/platform/store/inventorystore"
	"github.com/loomarr/loomarr/internal/platform/store/storedb"
)

// inventoryCluster names the embedded inventory store; see identityCluster for why an alias. The
// implementation is internal/platform/store/inventorystore.
type inventoryCluster = inventorystore.Store

// bindInventory installs the inventory cluster on s; bindClusters calls it on every construction.
// The cluster reaches the Library only through libraryProjection, so it never imports this
// package.
func bindInventory(s *sqlStore) *sqlStore {
	s.inventoryCluster = inventorystore.New(s.kernel(), libraryProjection{s})
	return s
}

// libraryProjection is the inventory cluster's view of the projector ProjectLibraryWith installs.
type libraryProjection struct{ s *sqlStore }

func (p libraryProjection) Projecting() bool { return p.s.projecting() }

func (p libraryProjection) Project(ctx context.Context, tx storedb.Tx, at time.Time, items []inventory.Item) error {
	return p.s.projectLibrary(ctx, tx, at, items)
}
