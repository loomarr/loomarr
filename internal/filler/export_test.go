package filler

import "testing"

// CertifiedShadowFixture exposes the certified two-segment proposal and its policies to the
// external filler_test package, which alone may import the real SQLite store without an import
// cycle.
func CertifiedShadowFixture(t *testing.T) (SplitProposal, *AutoSplitPolicy, *StructureMaterializationPolicy) {
	t.Helper()
	return certifiedStructureProposal(t), certifiedAutoPolicy(), allowCertifiedStructure(t)
}
