package asset

import (
	"fmt"

	"github.com/loomarr/loomarr/internal/filler/split"
	"github.com/loomarr/loomarr/internal/media/tools"
)

// EvidenceMatchesLineage reports whether the persisted conditioning evidence measured exactly the
// cut the lineage intended.
func EvidenceMatchesLineage(e ConditioningEvidence, lineage ConditioningLineage) bool {
	want := tools.Interval{StartMs: lineage.IntendedStartMs, EndMs: lineage.IntendedEndMs}
	return len(e.BeforeRewrite.Cuts) > 0 && len(e.AfterRewrite.Cuts) > 0 &&
		e.BeforeRewrite.Cuts[0].Intended == want && e.AfterRewrite.Cuts[0].Intended == want
}

// ValidateConditioningPair checks that e binds exactly the given before and after identities and
// that its post-rewrite parent edges follow from the measured timing.
func ValidateConditioningPair(e ConditioningEvidence, beforeHash, afterHash string) error {
	if !split.IsContentHash(beforeHash) || !split.IsContentHash(afterHash) ||
		e.BeforeRewriteHash != beforeHash || e.AfterRewriteHash != afterHash {
		return fmt.Errorf("persisted conditioning artifact identities are invalid")
	}
	if err := tools.ValidateConditioningPair(e.BeforeRewrite, e.AfterRewrite); err != nil {
		return err
	}
	derived, err := tools.DeriveConditioningParentEdges(e.BeforeRewrite, e.AfterRewrite)
	if err != nil || len(derived.Streams) != len(e.DerivedParentEdgesAfterRewrite.Streams) {
		return fmt.Errorf("persisted post-rewrite parent edges are invalid")
	}
	for i := range derived.Streams {
		if derived.Streams[i] != e.DerivedParentEdgesAfterRewrite.Streams[i] {
			return fmt.Errorf("persisted post-rewrite parent edges do not match measured timing")
		}
	}
	return nil
}

// ValidConditioningLineage reports whether lineage is a complete, correctly bound record for a
// child whose parent is childParentHash.
func ValidConditioningLineage(lineage *ConditioningLineage, childParentHash string) bool {
	if lineage == nil || !split.IsContentHash(lineage.ChildHash) || !split.IsContentHash(lineage.ParentHash) ||
		lineage.ParentHash != childParentHash || lineage.IntendedStartMs < 0 || lineage.IntendedEndMs <= lineage.IntendedStartMs {
		return false
	}
	assetBound := lineage.ParentAssetRole != "" || lineage.ParentAssetSHA256 != ""
	role := split.SourceRole(lineage.ParentAssetRole)
	decisionBound := lineage.StructureDecisionSHA256 == "" && lineage.StructureRole == "" ||
		split.IsContentHash(lineage.StructureDecisionSHA256) && split.CertifiedFillerRole(lineage.StructureRole)
	return decisionBound && (!assetBound || ((role == split.SourceEvidence || role == split.SourceLegacyPlayback) && split.IsContentHash(lineage.ParentAssetSHA256)))
}
