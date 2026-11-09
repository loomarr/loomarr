package split

import (
	"fmt"
	"slices"
	"time"

	"github.com/loomarr/loomarr/internal/filler/structure"
)

// Reassess adds the segments' retained per-span role evidence to the immutable detector facts of
// current. Evidence for a span that does not exactly match the deterministic plan remains
// auditable but cannot become a role claim. decision is the replayable complete-timeline artifact
// when current is its projection rather than a strictly reducible detector assessment.
func Reassess(source SourceAsset, current *Assessment, decision *structure.Artifact, segments []Segment, assessedAt time.Time) (Assessment, error) {
	if current == nil {
		return Assessment{}, fmt.Errorf("source structure: proposal has no assessment to enrich")
	}
	var authority *structureDecisionProjectionAuthority
	if err := ValidateAssessment(*current); err != nil {
		if decision == nil {
			return Assessment{}, err
		}
		authority, err = newStructureDecisionProjectionAuthority(source, *decision)
		if err != nil {
			return Assessment{}, err
		}
		if err := validateAssessment(*current, authority); err != nil {
			return Assessment{}, err
		}
	}
	replacedSpans := make(map[[2]int64]struct{}, len(segments))
	for _, segment := range segments {
		if segment.RoleEvidence != nil {
			replacedSpans[[2]int64{segment.StartMs, segment.EndMs}] = struct{}{}
		}
	}
	observations := make([]Observation, 0, len(current.Observations)+len(segments))
	for _, observation := range current.Observations {
		if observation.Kind == ObservationSegmentRole {
			if _, replaced := replacedSpans[[2]int64{observation.StartMs, observation.EndMs}]; replaced {
				continue
			}
		}
		observations = append(observations, observation)
	}
	discards := discardClaims(current.Plan)
	planSpans := make(map[[2]int64]struct{}, len(current.Plan))
	claimBySpan := make(map[[2]int64]RoleClaim, len(current.Plan))
	for _, segment := range current.Plan {
		span := [2]int64{segment.StartMs, segment.EndMs}
		planSpans[span] = struct{}{}
		if segment.Role != "" {
			claimBySpan[span] = RoleClaim{
				StartMs: segment.StartMs, EndMs: segment.EndMs, Role: segment.Role,
				EvidenceIDs: slices.Clone(segment.EvidenceIDs), Reason: segment.Reason,
			}
		}
	}
	for _, segment := range segments {
		if segment.RoleEvidence == nil {
			continue
		}
		evidence := CloneRoleEvidence(*segment.RoleEvidence)
		if evidence.Source != source || evidence.StartMs != segment.StartMs || evidence.EndMs != segment.EndMs {
			return Assessment{}, fmt.Errorf("source structure: segment %d role evidence binds another source or span", segment.Index)
		}
		observation, err := NewRoleObservation("role-"+evidence.SHA256, evidence)
		if err != nil {
			return Assessment{}, err
		}
		observations = append(observations, observation)
		span := [2]int64{segment.StartMs, segment.EndMs}
		if _, exact := planSpans[span]; exact {
			claimBySpan[span] = RoleClaim{
				StartMs: segment.StartMs, EndMs: segment.EndMs, Role: evidence.Role,
				EvidenceIDs: []string{observation.ID}, Reason: evidence.Reason,
			}
		}
	}
	claims := make([]RoleClaim, 0, len(claimBySpan))
	for _, segment := range current.Plan {
		if claim, exists := claimBySpan[[2]int64{segment.StartMs, segment.EndMs}]; exists {
			claims = append(claims, claim)
		}
	}
	assessment, err := assess(Input{
		Source: source, Observations: observations, RoleClaims: claims, DiscardClaims: discards,
		AssessedAt: assessedAt, UnusableReason: current.UnusableReason,
	}, authority)
	if err != nil {
		return Assessment{}, err
	}
	if authority != nil {
		if err := ValidateDecisionProjection(assessment, *decision); err != nil {
			return Assessment{}, err
		}
	}
	return assessment, nil
}

func discardClaims(plan []PlanSegment) []DiscardClaim {
	var claims []DiscardClaim
	for _, segment := range plan {
		if segment.Disposition == DispositionDiscard && segment.Role == "" {
			claims = append(claims, DiscardClaim{
				StartMs: segment.StartMs, EndMs: segment.EndMs, Reason: segment.DiscardReason,
				EvidenceIDs: slices.Clone(segment.EvidenceIDs),
			})
		}
	}
	return claims
}
