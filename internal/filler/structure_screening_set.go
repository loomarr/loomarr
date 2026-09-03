package filler

import (
	"fmt"
	"slices"
)

type screeningSpan struct{ start, end int64 }

func ValidateStructureScreeningSet(proposal SplitProposal) error {
	if len(proposal.StructureScreenings) == 0 {
		return nil
	}
	if proposal.Structure == nil || proposal.StructureDecision == nil ||
		proposal.Structure.Source != proposal.Source ||
		ValidateStructureDecisionProjection(*proposal.Structure, *proposal.StructureDecision) != nil {
		return fmt.Errorf("structure screenings require an exact confirmed structure decision")
	}
	keep := make(map[screeningSpan]struct{})
	for _, planned := range proposal.Structure.Plan {
		if planned.Disposition == StructureKeep {
			keep[screeningSpan{planned.StartMs, planned.EndMs}] = struct{}{}
		}
	}
	seen := make(map[screeningSpan]struct{}, len(proposal.StructureScreenings))
	for index, evidence := range proposal.StructureScreenings {
		span := screeningSpan{evidence.StartMs, evidence.EndMs}
		if ValidateSegmentScreeningEvidence(evidence) != nil || evidence.Source != proposal.Source {
			return fmt.Errorf("structure screening %d is invalid or source-drifted", index)
		}
		if _, exists := keep[span]; !exists {
			return fmt.Errorf("structure screening %d does not name a decided keep interval", index)
		}
		if _, duplicate := seen[span]; duplicate {
			return fmt.Errorf("structure screening %d repeats a decided interval", index)
		}
		seen[span] = struct{}{}
	}
	if !slices.IsSortedFunc(proposal.StructureScreenings, compareStructureScreenings) {
		return fmt.Errorf("structure screenings are not canonically ordered")
	}
	return nil
}

func compareStructureScreenings(left, right SegmentScreeningEvidence) int {
	if left.StartMs != right.StartMs {
		return intCompare64(left.StartMs, right.StartMs)
	}
	return intCompare64(left.EndMs, right.EndMs)
}

func intCompare64(left, right int64) int {
	switch {
	case left < right:
		return -1
	case left > right:
		return 1
	default:
		return 0
	}
}

func screeningForStructureInterval(proposal SplitProposal, segment SplitSegment) *SegmentScreeningEvidence {
	for index := range proposal.StructureScreenings {
		evidence := &proposal.StructureScreenings[index]
		if evidence.StartMs == segment.StartMs && evidence.EndMs == segment.EndMs {
			return evidence
		}
	}
	return segment.Screening
}
