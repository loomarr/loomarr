package store

import "github.com/loomarr/loomarr/internal/platform/store/activitystore"

// The Dashboard's Recent activity feed lives in internal/platform/store/activitystore. Its row type
// is re-exported here by alias, for the store's own contract.
type (
	// Activity is one row in the Recent activity feed (activitystore.Activity).
	Activity = activitystore.Activity
)

// activityCluster names the embedded activity store; see identityCluster for why an alias. The
// ActivityStore role it satisfies is declared in store.go.
type activityCluster = activitystore.Store

// bindActivity installs the activity cluster on s; bindClusters calls it on every construction.
func bindActivity(s *sqlStore) *sqlStore {
	s.activityCluster = activitystore.New(s.kernel())
	return s
}
