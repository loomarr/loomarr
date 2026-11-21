package store

import (
	"context"
	"time"

	"github.com/loomarr/loomarr/internal/platform/store/viewingstore"
)

// ViewingStore keeps the watch history. It has no retention: rows stay until an admin clears them.
// The implementation is internal/platform/store/viewingstore.
type ViewingStore interface {
	InsertViewing(ctx context.Context, v viewingstore.Viewing) error
	// ExtendViewing moves a viewing's end to endedAt (never earlier than it is) and, when programme
	// is not "", records the programme. It reports false when the row is gone (an admin cleared it).
	ExtendViewing(ctx context.Context, id, programme string, endedAt time.Time) (bool, error)
	ListViewings(ctx context.Context, q viewingstore.ViewingQuery) ([]viewingstore.Viewing, error)
	// DeleteViewings clears one person's history, or everyone's when userID is "", and reports how
	// many rows it removed.
	DeleteViewings(ctx context.Context, userID string) (int, error)
}

// viewingCluster names the embedded watch-history store; see identityCluster for why an alias.
type viewingCluster = viewingstore.Store

// bindViewings installs the watch-history cluster on s; bindClusters calls it on every
// construction.
func bindViewings(s *sqlStore) *sqlStore {
	s.viewingCluster = viewingstore.New(s.kernel())
	return s
}
