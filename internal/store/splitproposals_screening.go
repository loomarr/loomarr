package store

import (
	"cmp"
	"fmt"
	"slices"

	"github.com/loomarr/loomarr/internal/filler"
)

type splitScreeningSpan struct{ start, end int64 }

func splitProposalScreenings(p filler.SplitProposal) []filler.SegmentScreeningEvidence {
	screenings := append([]filler.SegmentScreeningEvidence(nil), p.StructureScreenings...)
	seen := make(map[splitScreeningSpan]struct{}, len(screenings))
	for _, screening := range screenings {
		seen[splitScreeningSpan{screening.StartMs, screening.EndMs}] = struct{}{}
	}
	for _, segment := range p.Segments {
		key := splitScreeningSpan{segment.StartMs, segment.EndMs}
		if segment.Screening == nil {
			continue
		}
		if _, exists := seen[key]; exists {
			continue
		}
		screenings = append(screenings, *segment.Screening)
		seen[key] = struct{}{}
	}
	slices.SortFunc(screenings, func(left, right filler.SegmentScreeningEvidence) int {
		if left.StartMs != right.StartMs {
			return cmp.Compare(left.StartMs, right.StartMs)
		}
		return cmp.Compare(left.EndMs, right.EndMs)
	})
	return screenings
}

func attachSplitProposalScreenings(p *filler.SplitProposal, screenings []filler.SegmentScreeningEvidence) error {
	bySpan := make(map[splitScreeningSpan]int, len(p.Segments))
	for index, segment := range p.Segments {
		bySpan[splitScreeningSpan{segment.StartMs, segment.EndMs}] = index
	}
	for i := range screenings {
		key := splitScreeningSpan{screenings[i].StartMs, screenings[i].EndMs}
		index, matchesSegment := bySpan[key]
		matchesStructure := screeningNamesStructureKeep(*p, screenings[i])
		if matchesStructure {
			p.StructureScreenings = append(p.StructureScreenings, screenings[i])
		}
		if !matchesSegment {
			if matchesStructure {
				continue
			}
			return fmt.Errorf("screening %d does not name a proposal segment or structure keep interval", i)
		}
		if p.Segments[index].Screening != nil {
			return fmt.Errorf("screening %d repeats a proposal segment", i)
		}
		screening := screenings[i]
		p.Segments[index].Screening = &screening
	}
	return nil
}

func validateSplitProposalScreenings(p filler.SplitProposal) error {
	if err := filler.ValidateStructureScreeningSet(p); err != nil {
		return err
	}
	structureBySpan := make(map[[2]int64]string, len(p.StructureScreenings))
	for _, screening := range p.StructureScreenings {
		structureBySpan[[2]int64{screening.StartMs, screening.EndMs}] = screening.SHA256
	}
	for index, segment := range p.Segments {
		if segment.Screening == nil {
			continue
		}
		if err := filler.ValidateSegmentScreeningEvidence(*segment.Screening); err != nil {
			return fmt.Errorf("segment %d has invalid screening evidence: %w", index, err)
		}
		if segment.Screening.Source != p.Source || segment.Screening.StartMs != segment.StartMs || segment.Screening.EndMs != segment.EndMs {
			return fmt.Errorf("segment %d screening does not bind the proposal source and span", index)
		}
		if digest, exists := structureBySpan[[2]int64{segment.StartMs, segment.EndMs}]; exists && digest != segment.Screening.SHA256 {
			return fmt.Errorf("segment %d screening conflicts with structure screening", index)
		}
	}
	return nil
}

func screeningNamesStructureKeep(p filler.SplitProposal, screening filler.SegmentScreeningEvidence) bool {
	if p.Structure == nil || p.StructureDecision == nil ||
		filler.ValidateStructureDecisionProjection(*p.Structure, *p.StructureDecision) != nil {
		return false
	}
	for _, planned := range p.Structure.Plan {
		if planned.Disposition == filler.StructureKeep && planned.StartMs == screening.StartMs && planned.EndMs == screening.EndMs {
			return true
		}
	}
	return false
}
