package filler

// structureDecisionSHA256ForInterval returns provenance only when the proposal is an exact
// projection of one confirmed artifact and this child is one of that decision's keep spans.
// Manual boundary edits and detector-only cuts deliberately return no model-decision identity.
func structureDecisionSHA256ForInterval(proposal SplitProposal, segment SplitSegment) string {
	if proposal.Structure == nil || proposal.StructureDecision == nil ||
		proposal.Structure.Source != proposal.Source ||
		ValidateStructureDecisionProjection(*proposal.Structure, *proposal.StructureDecision) != nil {
		return ""
	}
	for _, planned := range proposal.Structure.Plan {
		if planned.Disposition == StructureKeep && planned.StartMs == segment.StartMs && planned.EndMs == segment.EndMs {
			return proposal.StructureDecision.SHA256
		}
	}
	return ""
}

func segmentScreeningSHA256ForInterval(proposal SplitProposal, segment SplitSegment) string {
	evidence := screeningForStructureInterval(proposal, segment)
	if evidence == nil || ValidateSegmentScreeningEvidence(*evidence) != nil || evidence.Source != proposal.Source ||
		evidence.StartMs != segment.StartMs || evidence.EndMs != segment.EndMs {
		return ""
	}
	return evidence.SHA256
}
