package filler

import "time"

// StructureCertificationPolicy is the release boundary between a valid assessment and unattended
// publication. AssessmentCertified owns locked source/signal slices. ScreeningCertified verifies
// that all four content-addressed screening artifacts still exist in their owning ledgers. The
// decisions themselves are durable exact-span proposal evidence, not injectable booleans.
type StructureCertificationPolicy struct {
	AssessmentCertified func(SourceStructureAssessment) bool
	ScreeningCertified  func(SegmentScreeningEvidence) bool
}

const (
	RejectStructureMissing     AutoSplitReject = "the source has no complete structure assessment"
	RejectStructureInvalid     AutoSplitReject = "the source structure assessment is invalid"
	RejectStructureAmbiguous   AutoSplitReject = "the source structure or a segment role remains unresolved"
	RejectStructureMismatch    AutoSplitReject = "the proposed cuts do not match the complete structure plan"
	RejectStructureUncertified AutoSplitReject = "this source and signal path is not certified for automatic splitting"
	RejectSegmentUnscreened    AutoSplitReject = "a segment has not passed the required safety and rights screens"
)

// CertifiedAutoConfirmable applies V67's complete-plan rules before the compatibility V54 gate.
// It never returns a partial confirmation: if any keep interval fails the older tag/boundary rules,
// every keep interval remains together for one concise structure review. Explained discard spans
// are returned separately and never masquerade as held filler.
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
	type span struct{ start, end int64 }
	segments := make(map[span]SplitSegment, len(p.Segments))
	for _, segment := range p.Segments {
		key := span{segment.StartMs, segment.EndMs}
		if _, duplicate := segments[key]; duplicate {
			return certifiedSplitReject(p.Segments, RejectStructureMismatch)
		}
		segments[key] = segment
	}
	var keep, discard []SplitSegment
	for _, planned := range assessment.Plan {
		if planned.Disposition == StructureUnresolved {
			return certifiedSplitReject(p.Segments, RejectStructureAmbiguous)
		}
		segment, exists := segments[span{planned.StartMs, planned.EndMs}]
		switch planned.Disposition {
		case StructureKeep:
			if !exists || !certifiedFillerRole(planned.Role) {
				return certifiedSplitReject(p.Segments, RejectStructureMismatch)
			}
			keep = append(keep, segment)
			delete(segments, span{planned.StartMs, planned.EndMs})
		case StructureDiscard:
			if exists {
				discard = append(discard, segment)
				delete(segments, span{planned.StartMs, planned.EndMs})
			}
		}
	}
	if len(segments) != 0 || len(keep) == 0 {
		return certifiedSplitReject(p.Segments, RejectStructureMismatch)
	}
	if certification == nil || certification.AssessmentCertified == nil || !certification.AssessmentCertified(assessment) {
		return SplitPartition{Reject: RejectStructureUncertified, Hold: keep, Discard: discard}
	}
	for _, segment := range keep {
		if segment.Screening == nil || segment.Screening.Source != assessment.Source || segment.Screening.StartMs != segment.StartMs || segment.Screening.EndMs != segment.EndMs || !segment.Screening.Passes() || certification.ScreeningCertified == nil || !certification.ScreeningCertified(*segment.Screening) {
			return SplitPartition{Reject: RejectSegmentUnscreened, Hold: keep, Discard: discard}
		}
	}
	legacy := AutoConfirmable(SplitProposal{Segments: keep}, auto, minClipDuration)
	if legacy.Reject != AutoSplitOK {
		legacy.Discard = discard
		return legacy
	}
	if len(legacy.Hold) > 0 {
		return SplitPartition{Reject: legacy.Verdict(), Hold: keep, Discard: discard}
	}
	legacy.Discard = discard
	return legacy
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
