package fillerreview

import (
	"sort"

	"github.com/loomarr/loomarr/internal/fillereval"
)

const (
	temporalStructureDecisionReasonAgreement          = "independent_model_family_agreement"
	temporalStructureDecisionReasonOperationalFailure = "operational_failure"
	temporalStructureDecisionReasonUnitDisagreement   = "unit_disagreement"
	temporalStructureDecisionReasonUnsupportedUnit    = "unsupported_unit"
	temporalStructureDecisionReasonRoleDisagreement   = "role_disagreement"
	temporalStructureDecisionReasonIntervalCount      = "interval_count_disagreement"
	temporalStructureDecisionReasonIntervalRole       = "interval_role_disagreement"
	temporalStructureDecisionReasonBoundary           = "boundary_disagreement"
	temporalStructureDecisionReasonUnresolvedInterval = "unresolved_interval"
)

type temporalStructureDecisionCandidate struct {
	family     string
	assessorID string
	assessment TemporalStructureAssessment
}

func reduceTemporalStructureDecision(alias string, durationMS int64, candidates []temporalStructureDecisionCandidate) TemporalStructureCaseDecision {
	decision := TemporalStructureCaseDecision{Alias: alias, DurationMS: durationMS, Status: TemporalStructureDecisionHeld}
	for _, candidate := range candidates {
		decision.Candidates = append(decision.Candidates, temporalStructureDecisionObservation(candidate))
	}
	reasons := make(map[string]struct{})
	for _, candidate := range candidates {
		if candidate.assessment.OperationalFailure != nil {
			reasons[temporalStructureDecisionReasonOperationalFailure] = struct{}{}
		}
	}
	if len(reasons) != 0 {
		decision.ReasonCodes = sortedTemporalStructureDecisionReasons(reasons)
		return decision
	}

	unit := candidates[0].assessment.Unit.Kind
	for _, candidate := range candidates[1:] {
		if candidate.assessment.Unit.Kind != unit {
			reasons[temporalStructureDecisionReasonUnitDisagreement] = struct{}{}
		}
	}
	if unit == fillereval.UnitUnclear || unit == fillereval.UnitUnusable {
		reasons[temporalStructureDecisionReasonUnsupportedUnit] = struct{}{}
	}
	role := fillereval.TemporalRole("")
	if unit == fillereval.UnitStandalone {
		role = candidates[0].assessment.Role.Kind
		for _, candidate := range candidates[1:] {
			if candidate.assessment.Role == nil || candidate.assessment.Role.Kind != role {
				reasons[temporalStructureDecisionReasonRoleDisagreement] = struct{}{}
			}
		}
	}

	intervals := len(candidates[0].assessment.Segments)
	for _, candidate := range candidates[1:] {
		if len(candidate.assessment.Segments) != intervals {
			reasons[temporalStructureDecisionReasonIntervalCount] = struct{}{}
		}
	}
	if len(reasons) != 0 {
		decision.ReasonCodes = sortedTemporalStructureDecisionReasons(reasons)
		return decision
	}

	for index := 0; index < intervals; index++ {
		role := candidates[0].assessment.Segments[index].Role
		if role == fillereval.TemporalSegmentAmbiguous || role == fillereval.TemporalSegmentUnusable {
			reasons[temporalStructureDecisionReasonUnresolvedInterval] = struct{}{}
		}
		for _, candidate := range candidates[1:] {
			candidateRole := candidate.assessment.Segments[index].Role
			if candidateRole != role {
				reasons[temporalStructureDecisionReasonIntervalRole] = struct{}{}
			}
			if candidateRole == fillereval.TemporalSegmentAmbiguous || candidateRole == fillereval.TemporalSegmentUnusable {
				reasons[temporalStructureDecisionReasonUnresolvedInterval] = struct{}{}
			}
		}
	}
	if len(reasons) != 0 {
		decision.ReasonCodes = sortedTemporalStructureDecisionReasons(reasons)
		return decision
	}

	boundaries := []int64{0}
	for index := 0; index < intervals-1; index++ {
		byFamily := make(map[string][]int64)
		for _, candidate := range candidates {
			byFamily[candidate.family] = append(byFamily[candidate.family], candidate.assessment.Segments[index].EndMS)
		}
		familyBoundaries := make([]int64, 0, len(byFamily))
		for _, values := range byFamily {
			minimum, maximum := temporalStructureDecisionRange(values)
			if maximum-minimum > TemporalStructureNearBoundaryMS {
				reasons[temporalStructureDecisionReasonBoundary] = struct{}{}
			}
			familyBoundaries = append(familyBoundaries, minimum+(maximum-minimum)/2)
		}
		minimum, maximum := temporalStructureDecisionRange(familyBoundaries)
		if maximum-minimum > TemporalStructureNearBoundaryMS {
			reasons[temporalStructureDecisionReasonBoundary] = struct{}{}
		}
		boundaries = append(boundaries, minimum+(maximum-minimum)/2)
	}
	boundaries = append(boundaries, durationMS)
	if len(reasons) != 0 {
		decision.ReasonCodes = sortedTemporalStructureDecisionReasons(reasons)
		return decision
	}

	decision.Status = TemporalStructureDecisionConfirmed
	decision.ReasonCodes = []string{temporalStructureDecisionReasonAgreement}
	decision.Unit = unit
	decision.Role = role
	for index := 0; index < intervals; index++ {
		segmentRole := candidates[0].assessment.Segments[index].Role
		decision.Segments = append(decision.Segments, TemporalStructureDecisionSegment{
			StartMS: boundaries[index], EndMS: boundaries[index+1],
			Disposition: temporalStructureDecisionDisposition(segmentRole), Role: segmentRole,
		})
	}
	return decision
}

func temporalStructureDecisionObservation(candidate temporalStructureDecisionCandidate) TemporalStructureDecisionCandidateObservation {
	observation := TemporalStructureDecisionCandidateObservation{AssessorID: candidate.assessorID, ModelFamily: candidate.family}
	if candidate.assessment.OperationalFailure != nil {
		observation.Failure = candidate.assessment.OperationalFailure.Code
		return observation
	}
	observation.Unit = candidate.assessment.Unit.Kind
	if candidate.assessment.Role != nil {
		observation.Role = candidate.assessment.Role.Kind
	}
	for _, segment := range candidate.assessment.Segments {
		observation.Segments = append(observation.Segments, TemporalStructurePredictedSegment{StartMS: segment.StartMS, EndMS: segment.EndMS, Role: segment.Role})
	}
	return observation
}

func temporalStructureDecisionDisposition(role fillereval.TemporalSegmentRole) string {
	if temporalStructureFillerSegmentRole(role) {
		return TemporalStructureDispositionFillerCandidate
	}
	if role == fillereval.TemporalSegmentProgrammeFragment || role == fillereval.TemporalSegmentNonFiller {
		return TemporalStructureDispositionNonFiller
	}
	return TemporalStructureDispositionUnresolved
}

func temporalStructureDecisionRange(values []int64) (int64, int64) {
	minimum, maximum := values[0], values[0]
	for _, value := range values[1:] {
		if value < minimum {
			minimum = value
		}
		if value > maximum {
			maximum = value
		}
	}
	return minimum, maximum
}

func sortedTemporalStructureDecisionReasons(reasons map[string]struct{}) []string {
	result := make([]string, 0, len(reasons))
	for reason := range reasons {
		result = append(result, reason)
	}
	sort.Strings(result)
	return result
}
