package filler

import (
	"fmt"
	"time"

	"github.com/loomarr/loomarr/internal/fillerstructure"
)

// StructureCertificationPolicy is the release boundary between a valid assessment and unattended
// publication. Authority owns locked source/signal slices. ScreeningCertified verifies that all
// four content-addressed screening artifacts still exist in their owning ledgers.
type StructureCertificationPolicy struct {
	Authority          *fillerstructure.Authority
	ScreeningCertified func(SegmentScreeningEvidence) bool
}

const (
	RejectStructureMissing     AutoSplitReject = "the source has no complete structure assessment"
	RejectStructureInvalid     AutoSplitReject = "the source structure assessment is invalid"
	RejectStructureAmbiguous   AutoSplitReject = "the source structure or a segment role remains unresolved"
	RejectStructureMismatch    AutoSplitReject = "the proposed cuts do not match the complete structure plan"
	RejectStructureUncertified AutoSplitReject = "this source and signal path is not certified for automatic splitting"
	RejectSegmentUnscreened    AutoSplitReject = "a segment has not passed the required safety and rights screens"
)

// CertifiedAutoConfirmable applies V67's complete-plan rules independently of the compatibility
// detector's cut coordinates and confidence score. It never returns a partial confirmation: if any
// keep interval fails metadata admission, every keep interval remains together for one review.
func CertifiedAutoConfirmable(p SplitProposal, auto *AutoSplitPolicy, certification *StructureCertificationPolicy, minClipDuration time.Duration) SplitPartition {
	if p.Structure == nil {
		return certifiedSplitReject(p.Segments, RejectStructureMissing)
	}
	assessment := *p.Structure
	if ValidateSourceStructureAssessment(assessment) != nil || assessment.Source != p.Source {
		return certifiedSplitReject(p.Segments, RejectStructureInvalid)
	}
	if assessment.Kind != StructureCompilationBreak && assessment.Kind != StructureProgrammeSpots {
		return certifiedSplitReject(p.Segments, RejectStructureAmbiguous)
	}
	if len(p.Spawned) > 0 {
		// V54 proposals that already published a partial generation lack source-span lineage for
		// those older children. They stay reviewable; V67 never guesses which plan spans they own.
		return certifiedSplitReject(p.Segments, RejectStructureMismatch)
	}
	keep, discard, err := projectCertifiedStructureSegments(p.Segments, assessment.Plan)
	if err != nil || len(keep) == 0 {
		return certifiedSplitReject(p.Segments, RejectStructureMismatch)
	}
	if p.StructureDecision == nil {
		return SplitPartition{Reject: RejectStructureUncertified, Hold: keep, Discard: discard}
	}
	if ValidateStructureDecisionProjection(assessment, *p.StructureDecision) != nil {
		return SplitPartition{Reject: RejectStructureMismatch, Hold: keep, Discard: discard}
	}
	if certification == nil || certification.Authority == nil || fillerstructure.VerifyAuthority(*p.StructureDecision, *certification.Authority) != nil {
		return SplitPartition{Reject: RejectStructureUncertified, Hold: keep, Discard: discard}
	}
	for _, segment := range keep {
		if segment.Screening == nil || segment.Screening.Source != assessment.Source || segment.Screening.StartMs != segment.StartMs || segment.Screening.EndMs != segment.EndMs || !segment.Screening.Passes() || certification.ScreeningCertified == nil || !certification.ScreeningCertified(*segment.Screening) {
			return SplitPartition{Reject: RejectSegmentUnscreened, Hold: keep, Discard: discard}
		}
	}
	certified := certifiedPlanConfirmable(keep, auto, minClipDuration)
	if certified.Reject != AutoSplitOK {
		certified.Discard = discard
		return certified
	}
	if len(certified.Hold) > 0 {
		return SplitPartition{Reject: certified.Verdict(), Hold: certified.Hold, Discard: discard}
	}
	certified.Discard = discard
	return certified
}

func projectCertifiedStructureSegments(existing []SplitSegment, plan []StructurePlanSegment) ([]SplitSegment, []SplitSegment, error) {
	type span struct{ start, end int64 }
	metadata := make(map[span]SplitSegment, len(existing))
	for _, segment := range existing {
		key := span{segment.StartMs, segment.EndMs}
		if _, duplicate := metadata[key]; duplicate {
			return nil, nil, fmt.Errorf("certified split projection repeats a detector span")
		}
		metadata[key] = segment
	}
	var keep, discard []SplitSegment
	for _, planned := range plan {
		segment := metadata[span{planned.StartMs, planned.EndMs}]
		segment.Index, segment.StartMs, segment.EndMs = planned.Index, planned.StartMs, planned.EndMs
		segment.HoldReason = ""
		switch planned.Disposition {
		case StructureKeep:
			if !certifiedFillerRole(planned.Role) {
				return nil, nil, fmt.Errorf("certified split projection has a non-filler keep interval")
			}
			keep = append(keep, segment)
		case StructureDiscard:
			discard = append(discard, segment)
		default:
			return nil, nil, fmt.Errorf("certified split projection has an unresolved interval")
		}
	}
	return keep, discard, nil
}

func certifiedPlanConfirmable(segments []SplitSegment, policy *AutoSplitPolicy, minClipDuration time.Duration) SplitPartition {
	if policy == nil || policy.Enabled == nil || !policy.Enabled() {
		return SplitPartition{Reject: RejectDisabled, Hold: segments}
	}
	if len(segments) == 0 {
		return SplitPartition{Reject: RejectNoSegments}
	}
	maxDuration := 120 * time.Second
	if policy.MaxDuration != nil {
		if configured := policy.MaxDuration(); configured > 0 {
			maxDuration = configured
		}
	}
	partition := SplitPartition{}
	for _, segment := range segments {
		if reason := segmentContentVerdict(segment, policy, minClipDuration, maxDuration); reason != AutoSplitOK {
			segment.HoldReason = string(reason)
			partition.Hold = append(partition.Hold, segment)
			continue
		}
		partition.Confirm = append(partition.Confirm, segment)
	}
	if len(partition.Hold) > 0 {
		reject := partition.Verdict()
		type span struct{ start, end int64 }
		reasons := make(map[span]string, len(partition.Hold))
		for _, held := range partition.Hold {
			reasons[span{held.StartMs, held.EndMs}] = held.HoldReason
		}
		allHeld := append([]SplitSegment(nil), segments...)
		for index := range allHeld {
			allHeld[index].HoldReason = reasons[span{allHeld[index].StartMs, allHeld[index].EndMs}]
			if allHeld[index].HoldReason == "" {
				allHeld[index].HoldReason = string(reject)
			}
		}
		partition.Confirm = nil
		partition.Hold = allHeld
		partition.Reject = reject
	}
	return partition
}

func certifiedSplitReject(segments []SplitSegment, reason AutoSplitReject) SplitPartition {
	hold := append([]SplitSegment(nil), segments...)
	for i := range hold {
		hold[i].HoldReason = string(reason)
	}
	return SplitPartition{Reject: reason, Hold: hold}
}

func certifiedFillerRole(role StructureSegmentRole) bool {
	switch role {
	case SegmentRoleCommercial, SegmentRolePromo, SegmentRoleBumper, SegmentRoleStationID, SegmentRolePSA, SegmentRoleTrailer:
		return true
	default:
		return false
	}
}
