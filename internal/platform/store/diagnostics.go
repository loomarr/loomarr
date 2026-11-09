package store

import "github.com/loomarr/loomarr/internal/platform/store/diagnosticsstore"

// diagnosticsCluster names the embedded diagnostics store; see identityCluster for why an alias.
// The DiagnosticStore role it satisfies is declared in store.go; the implementation is
// internal/platform/store/diagnosticsstore.
type diagnosticsCluster = diagnosticsstore.Store

// bindDiagnostics installs the diagnostics cluster on s; bindClusters calls it on every
// construction.
func bindDiagnostics(s *sqlStore) *sqlStore {
	s.diagnosticsCluster = diagnosticsstore.New(s.kernel())
	return s
}
