package split

import (
	"errors"
	"fmt"
	"slices"
	"strings"
)

func buildStructurePlan(source SourceAsset, boundaries []Boundary, claims []RoleClaim, discards []DiscardClaim, observations []Observation, authority *structureDecisionProjectionAuthority) ([]PlanSegment, error) {
	durationMs := source.DurationMs
	cutStatuses := map[int64]BoundaryStatus{0: BoundaryResolved, durationMs: BoundaryResolved}
	for _, boundary := range boundaries {
		if prior, exists := cutStatuses[boundary.AtMs]; !exists || prior == BoundaryResolved {
			cutStatuses[boundary.AtMs] = boundary.Status
		}
	}
	// A V34 shadow discard may use the legacy detector's exact cut while V67 fusion reports a
	// narrower uncertainty midpoint. Preserve the discarded interval as unresolved structure
	// instead of moving or losing time; unresolved edges keep the assessment out of auto-publish.
	for _, discard := range discards {
		if discard.StartMs > 0 && discard.StartMs < durationMs {
			if _, exists := cutStatuses[discard.StartMs]; !exists {
				cutStatuses[discard.StartMs] = BoundaryUnresolved
			}
		}
		if discard.EndMs > 0 && discard.EndMs < durationMs {
			if _, exists := cutStatuses[discard.EndMs]; !exists {
				cutStatuses[discard.EndMs] = BoundaryUnresolved
			}
		}
	}
	cuts := make([]int64, 0, len(cutStatuses))
	for cut := range cutStatuses {
		cuts = append(cuts, cut)
	}
	slices.Sort(cuts)
	claimBySpan := make(map[[2]int64]RoleClaim, len(claims))
	discardBySpan := make(map[[2]int64]DiscardClaim, len(discards))
	knownEvidence := make(map[string]Observation, len(observations))
	for _, o := range observations {
		knownEvidence[o.ID] = o
	}
	for _, claim := range claims {
		claim.Reason = strings.TrimSpace(claim.Reason)
		claim.EvidenceIDs = UniqueStrings(claim.EvidenceIDs)
		if claim.StartMs < 0 || claim.EndMs <= claim.StartMs || claim.EndMs > durationMs || !ValidRole(claim.Role) || claim.Reason == "" || len(claim.EvidenceIDs) == 0 {
			return nil, errors.New("source structure: invalid role claim")
		}
		for _, id := range claim.EvidenceIDs {
			if _, ok := knownEvidence[id]; !ok {
				return nil, fmt.Errorf("source structure: role claim cites unknown observation %q", id)
			}
		}
		key := [2]int64{claim.StartMs, claim.EndMs}
		if _, duplicate := claimBySpan[key]; duplicate {
			return nil, fmt.Errorf("source structure: duplicate role claim for %d..%d", claim.StartMs, claim.EndMs)
		}
		claimBySpan[key] = claim
	}
	for _, discard := range discards {
		discard.EvidenceIDs = UniqueStrings(discard.EvidenceIDs)
		if discard.StartMs < 0 || discard.EndMs <= discard.StartMs || discard.EndMs > durationMs || !validStructureDiscardReason(discard.Reason) || len(discard.EvidenceIDs) == 0 {
			return nil, errors.New("source structure: invalid discard claim")
		}
		for _, id := range discard.EvidenceIDs {
			if _, ok := knownEvidence[id]; !ok {
				return nil, fmt.Errorf("source structure: discard claim cites unknown observation %q", id)
			}
		}
		key := [2]int64{discard.StartMs, discard.EndMs}
		if _, duplicate := discardBySpan[key]; duplicate {
			return nil, fmt.Errorf("source structure: duplicate discard claim for %d..%d", discard.StartMs, discard.EndMs)
		}
		if _, roleClaimed := claimBySpan[key]; roleClaimed {
			return nil, fmt.Errorf("source structure: span %d..%d has both role and discard claims", discard.StartMs, discard.EndMs)
		}
		discardBySpan[key] = discard
	}
	plan := make([]PlanSegment, 0, len(cuts)-1)
	usedClaims, usedDiscards := 0, 0
	for i := 0; i+1 < len(cuts); i++ {
		segment := PlanSegment{
			Index: i, StartMs: cuts[i], EndMs: cuts[i+1], Disposition: DispositionUnresolved,
			StartStatus: cutStatuses[cuts[i]], EndStatus: cutStatuses[cuts[i+1]],
			Reason: "structure or role remains unresolved",
		}
		if claim, ok := claimBySpan[[2]int64{segment.StartMs, segment.EndMs}]; ok {
			usedClaims++
			segment.Role, segment.EvidenceIDs, segment.Reason = claim.Role, claim.EvidenceIDs, claim.Reason
			if segment.StartStatus == BoundaryResolved && segment.EndStatus == BoundaryResolved {
				switch claim.Role {
				case RoleCommercial, RolePromo, RoleBumper, RoleStationID, RolePSA, RoleTrailer, RoleInterstitial:
					if structureRoleClaimHasExactEvidence(claim, source, knownEvidence) || authority.authorizesKeep(claim, knownEvidence) {
						segment.Disposition, segment.Reason = DispositionKeep, claim.Reason
					}
				case RoleProgrammeFragment, RoleNonFiller:
					segment.Disposition, segment.Reason = DispositionDiscard, claim.Reason
					if claim.Role == RoleProgrammeFragment {
						segment.DiscardReason = DiscardProgrammeMaterial
					} else {
						segment.DiscardReason = DiscardNonFiller
					}
				}
			}
		}
		if discard, ok := discardBySpan[[2]int64{segment.StartMs, segment.EndMs}]; ok {
			usedDiscards++
			segment.Disposition, segment.Reason = DispositionDiscard, string(discard.Reason)
			segment.DiscardReason, segment.EvidenceIDs = discard.Reason, discard.EvidenceIDs
		}
		plan = append(plan, segment)
	}
	if usedClaims != len(claimBySpan) {
		return nil, errors.New("source structure: role claim does not align with fused segment bounds")
	}
	if usedDiscards != len(discardBySpan) {
		return nil, errors.New("source structure: discard claim does not align with fused segment bounds")
	}
	return plan, validateStructureCoverage(plan, durationMs)
}

func (authority *structureDecisionProjectionAuthority) authorizesKeep(claim RoleClaim, observations map[string]Observation) bool {
	if authority == nil {
		return false
	}
	interval, ok := authority.intervals[[2]int64{claim.StartMs, claim.EndMs}]
	if !ok || interval.disposition != DispositionKeep || interval.role != claim.Role {
		return false
	}
	for _, id := range claim.EvidenceIDs {
		observation, exists := observations[id]
		if exists && id == interval.observationID && observation.Kind == ObservationCompleteTimelineDecision &&
			observation.Effect == ObservationContextOnly && observation.StartMs == claim.StartMs && observation.EndMs == claim.EndMs &&
			observation.Producer == authority.reducerVersion && observation.EvidenceSHA256 == authority.artifactSHA256 && observation.RoleEvidence == nil {
			return true
		}
	}
	return false
}

func structureRoleClaimHasExactEvidence(claim RoleClaim, source SourceAsset, observations map[string]Observation) bool {
	for _, id := range claim.EvidenceIDs {
		observation := observations[id]
		if observation.Kind != ObservationSegmentRole || observation.RoleEvidence == nil {
			continue
		}
		evidence := observation.RoleEvidence
		if evidence.Source == source && evidence.StartMs == claim.StartMs && evidence.EndMs == claim.EndMs && evidence.Role == claim.Role {
			return true
		}
	}
	return false
}

func reduceStructureKind(assessment Assessment) Kind {
	if assessment.UnusableReason != "" {
		return KindUnusable
	}
	for _, boundary := range assessment.Boundaries {
		if boundary.Status != BoundaryResolved {
			return KindAmbiguous
		}
	}
	allResolved := true
	fillers, programmes := 0, 0
	for _, segment := range assessment.Plan {
		if segment.Disposition == DispositionUnresolved || segment.Role == RoleAmbiguous || segment.Role == RoleUnusable || segment.Disposition == DispositionKeep && segment.Role == "" {
			allResolved = false
		}
		if segment.Disposition == DispositionKeep {
			fillers++
		}
		if segment.Role == RoleProgrammeFragment {
			programmes++
		}
	}
	if !allResolved {
		return KindAmbiguous
	}
	if programmes > 0 && fillers > 0 {
		return KindProgrammeSpots
	}
	if len(assessment.Plan) == 1 {
		return KindSingleUnit
	}
	if fillers >= 2 && programmes == 0 {
		return KindCompilationBreak
	}
	return KindAmbiguous
}

func validateStructureCoverage(plan []PlanSegment, durationMs int64) error {
	if durationMs <= 0 || len(plan) == 0 || plan[0].StartMs != 0 || plan[len(plan)-1].EndMs != durationMs {
		return errors.New("source structure: plan does not cover the complete source")
	}
	for i, segment := range plan {
		if segment.Index != i || segment.StartMs < 0 || segment.EndMs <= segment.StartMs || i > 0 && segment.StartMs != plan[i-1].EndMs || !validStructureDisposition(segment.Disposition) || !validStructureBoundaryStatus(segment.StartStatus) || !validStructureBoundaryStatus(segment.EndStatus) {
			return fmt.Errorf("source structure: invalid plan segment %d", i)
		}
		if segment.Disposition != DispositionUnresolved && strings.TrimSpace(segment.Reason) == "" {
			return fmt.Errorf("source structure: resolved plan segment %d lacks a reason", i)
		}
		if segment.Disposition == DispositionKeep && segment.Role == "" {
			return fmt.Errorf("source structure: kept plan segment %d lacks a role", i)
		}
		if segment.Disposition == DispositionDiscard && !validStructureDiscardReason(segment.DiscardReason) {
			return fmt.Errorf("source structure: discarded plan segment %d lacks a closed reason", i)
		}
	}
	return nil
}

func validSourceStructureKind(kind Kind) bool {
	return kind == KindSingleUnit || kind == KindCompilationBreak || kind == KindProgrammeSpots || kind == KindAmbiguous || kind == KindUnusable
}

// ValidRole reports whether role is one of the closed segment roles.
func ValidRole(role Role) bool {
	switch role {
	case RoleCommercial, RolePromo, RoleBumper, RoleStationID, RolePSA, RoleTrailer, RoleInterstitial, RoleProgrammeFragment, RoleNonFiller, RoleAmbiguous, RoleUnusable:
		return true
	default:
		return false
	}
}

func validStructureDisposition(disposition Disposition) bool {
	return disposition == DispositionKeep || disposition == DispositionDiscard || disposition == DispositionUnresolved
}

func validStructureBoundaryStatus(status BoundaryStatus) bool {
	return status == BoundaryResolved || status == BoundaryUnresolved || status == BoundaryConflicted
}

func validStructureDiscardReason(reason DiscardReason) bool {
	return reason == DiscardBelowClipFloor || reason == DiscardDuplicate || reason == DiscardProgrammeMaterial || reason == DiscardNonFiller || reason == DiscardUnusableFragment
}
