package store

import "github.com/loomarr/loomarr/internal/platform/store/operationstore"

// operationCluster names the embedded interactive-operation store; see identityCluster for why an
// alias. The implementation is internal/platform/store/operationstore.
type operationCluster = operationstore.Store

// bindOperations installs the interactive-operation cluster on s; bindClusters calls it on every
// construction.
func bindOperations(s *sqlStore) *sqlStore {
	s.operationCluster = operationstore.New(s.kernel())
	return s
}
