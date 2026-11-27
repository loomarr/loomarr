package store

import "github.com/loomarr/loomarr/internal/platform/store/qualitystore"

// The suggestion-quality ledger lives in internal/platform/store/qualitystore.

// qualityCluster names the embedded quality-ledger store; see identityCluster for why an alias.
type qualityCluster = qualitystore.Store

// bindQuality installs the quality-ledger cluster on s; bindClusters calls it on every construction.
func bindQuality(s *sqlStore) *sqlStore {
	s.qualityCluster = qualitystore.New(s.kernel())
	return s
}
