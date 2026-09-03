package filler

import (
	"context"
	"fmt"
	"slices"
	"time"
)

func proposalNeedsStructureScreening(proposal SplitProposal) bool {
	return len(missingStructureScreeningIntervals(proposal)) > 0
}

func missingStructureScreeningIntervals(proposal SplitProposal) []StructurePlanSegment {
	if proposal.Structure == nil || proposal.StructureDecision == nil ||
		ValidateStructureDecisionProjection(*proposal.Structure, *proposal.StructureDecision) != nil ||
		ValidateStructureScreeningSet(proposal) != nil {
		return nil
	}
	existing := make(map[screeningSpan]struct{}, len(proposal.StructureScreenings))
	for _, screening := range proposal.StructureScreenings {
		existing[screeningSpan{screening.StartMs, screening.EndMs}] = struct{}{}
	}
	missing := make([]StructurePlanSegment, 0)
	for _, planned := range proposal.Structure.Plan {
		if planned.Disposition != StructureKeep {
			continue
		}
		if _, found := existing[screeningSpan{planned.StartMs, planned.EndMs}]; !found {
			missing = append(missing, planned)
		}
	}
	return missing
}

func (sp *Splitter) ScreenProposalStructure(ctx context.Context, proposal SplitProposal, screener ExactSpanScreeningDecisioner) (SplitProposal, error) {
	if sp == nil || sp.store == nil || screener == nil || proposal.ID == "" || proposal.ClipHash == "" || !proposal.Ready() {
		return SplitProposal{}, fmt.Errorf("screen proposal structure: execution is unavailable or proposal is incomplete")
	}
	missing := missingStructureScreeningIntervals(proposal)
	if len(missing) == 0 {
		return proposal, nil
	}
	clip, found, err := sp.store.GetClip(ctx, proposal.ClipHash)
	if err != nil {
		return SplitProposal{}, fmt.Errorf("screen proposal structure: get compilation: %w", err)
	}
	if !found {
		return SplitProposal{}, fmt.Errorf("screen proposal structure: compilation is unavailable")
	}
	source, fullPath, err := sp.resolveSource(ctx, sp.dropDir, clip, proposal.Source)
	if err != nil {
		return SplitProposal{}, fmt.Errorf("screen proposal structure source: %w", err)
	}
	returned, err := screener.Screen(ctx, StructureScreeningMedia{Source: source, FullPath: fullPath, Intervals: slices.Clone(missing)})
	if err != nil {
		return SplitProposal{}, fmt.Errorf("screen decided structure intervals: %w", err)
	}
	if err := validateReturnedStructureScreenings(source, missing, returned); err != nil {
		return SplitProposal{}, err
	}
	updated := proposal
	updated.StructureScreenings = append(slices.Clone(proposal.StructureScreenings), returned...)
	slices.SortFunc(updated.StructureScreenings, compareStructureScreenings)
	if err := ValidateStructureScreeningSet(updated); err != nil {
		return SplitProposal{}, err
	}
	updateCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	if err := sp.store.UpdateSplitProposal(updateCtx, updated); err != nil {
		return SplitProposal{}, err
	}
	return updated, nil
}

func validateReturnedStructureScreenings(source SplitSourceAsset, requested []StructurePlanSegment, returned []SegmentScreeningEvidence) error {
	if len(returned) != len(requested) {
		return fmt.Errorf("screen decided structure intervals: result coverage is incomplete")
	}
	want := make(map[screeningSpan]struct{}, len(requested))
	for _, interval := range requested {
		want[screeningSpan{interval.StartMs, interval.EndMs}] = struct{}{}
	}
	for index, evidence := range returned {
		span := screeningSpan{evidence.StartMs, evidence.EndMs}
		if ValidateSegmentScreeningEvidence(evidence) != nil || evidence.Source != source {
			return fmt.Errorf("screen decided structure intervals: result %d is invalid or source-drifted", index)
		}
		if _, exists := want[span]; !exists {
			return fmt.Errorf("screen decided structure intervals: result %d was not requested", index)
		}
		delete(want, span)
	}
	if len(want) != 0 {
		return fmt.Errorf("screen decided structure intervals: result coverage is incomplete")
	}
	return nil
}
