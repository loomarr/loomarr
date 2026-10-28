package fillerstructure

import (
	"slices"
	"sort"
	"strings"
)

func Reduce(request Request) Decision {
	candidates := slices.Clone(request.Candidates)
	for index := range candidates {
		candidates[index].Assessor.ModelFamily = strings.ToLower(strings.TrimSpace(candidates[index].Assessor.ModelFamily))
		candidates[index].Segments = slices.Clone(candidates[index].Segments)
	}
	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].Assessor.ID != candidates[j].Assessor.ID {
			return candidates[i].Assessor.ID < candidates[j].Assessor.ID
		}
		return candidates[i].Assessor.AssessmentSHA256 < candidates[j].Assessor.AssessmentSHA256
	})
	decision := Decision{Source: request.Source, Input: request.Input, Status: StatusHeld, Candidates: candidates}
	if invalidCandidates(Request{Source: request.Source, Input: request.Input, BoundaryToleranceMS: request.BoundaryToleranceMS, Candidates: candidates}) {
		decision.ReasonCodes = []string{ReasonInvalidCandidate}
		return decision
	}
	reasons := make(map[string]struct{})
	for _, candidate := range candidates {
		if candidate.Failure != "" {
			reasons[ReasonOperationalFailure] = struct{}{}
		}
	}
	if len(reasons) != 0 {
		decision.ReasonCodes = sortedReasons(reasons)
		return decision
	}

	unit := candidates[0].Unit
	for _, candidate := range candidates[1:] {
		if candidate.Unit != unit {
			reasons[ReasonUnitDisagreement] = struct{}{}
		}
	}
	if unit == UnitUnclear || unit == UnitUnusable {
		reasons[ReasonUnsupportedUnit] = struct{}{}
	}
	// A standalone role is always a filler role, so a subtype disagreement leaves the role absent
	// rather than holding the structure; the exact role is the final child's to establish (#1069).
	role := Role("")
	if unit == UnitStandalone {
		role = unanimousRole(candidates, func(candidate Candidate) Role { return candidate.Role })
	}
	intervals := len(candidates[0].Segments)
	for _, candidate := range candidates[1:] {
		if len(candidate.Segments) != intervals {
			reasons[ReasonIntervalCount] = struct{}{}
		}
	}
	if len(reasons) != 0 {
		decision.ReasonCodes = sortedReasons(reasons)
		return decision
	}

	// Intervals must agree on disposition, never on subtype: commercial against PSA is still one
	// filler candidate, while filler against programme or an unresolved role always holds.
	for index := range intervals {
		disposition := DispositionForRole(candidates[0].Segments[index].Role)
		for _, candidate := range candidates {
			candidateDisposition := DispositionForRole(candidate.Segments[index].Role)
			if candidateDisposition == DispositionUnresolved {
				reasons[ReasonUnresolvedInterval] = struct{}{}
			}
			if candidateDisposition != disposition {
				reasons[ReasonIntervalDisposition] = struct{}{}
			}
		}
	}
	if len(reasons) != 0 {
		decision.ReasonCodes = sortedReasons(reasons)
		return decision
	}

	boundaries := []int64{0}
	for index := 0; index < intervals-1; index++ {
		byFamily := make(map[string][]int64)
		for _, candidate := range candidates {
			family := strings.ToLower(strings.TrimSpace(candidate.Assessor.ModelFamily))
			byFamily[family] = append(byFamily[family], candidate.Segments[index].EndMS)
		}
		familyBoundaries := make([]int64, 0, len(byFamily))
		for _, values := range byFamily {
			minimum, maximum := valueRange(values)
			if maximum-minimum > request.BoundaryToleranceMS {
				reasons[ReasonBoundary] = struct{}{}
			}
			familyBoundaries = append(familyBoundaries, minimum+(maximum-minimum)/2)
		}
		minimum, maximum := valueRange(familyBoundaries)
		if maximum-minimum > request.BoundaryToleranceMS {
			reasons[ReasonBoundary] = struct{}{}
		}
		boundaries = append(boundaries, minimum+(maximum-minimum)/2)
	}
	boundaries = append(boundaries, request.Source.DurationMS)
	if len(reasons) != 0 {
		decision.ReasonCodes = sortedReasons(reasons)
		return decision
	}

	decision.Status = StatusConfirmed
	decision.ReasonCodes = []string{ReasonAgreement}
	decision.Unit, decision.Role = unit, role
	for index := range intervals {
		decision.Segments = append(decision.Segments, DecisionSegment{
			StartMS: boundaries[index], EndMS: boundaries[index+1],
			Disposition: DispositionForRole(candidates[0].Segments[index].Role),
			Role:        unanimousRole(candidates, func(candidate Candidate) Role { return candidate.Segments[index].Role }),
		})
	}
	return decision
}

// unanimousRole is a diagnostic: the role every candidate gave, or absent. It never picks the first
// answer or a majority, because no certified module has established a role the assessors dispute.
func unanimousRole(candidates []Candidate, roleOf func(Candidate) Role) Role {
	role := roleOf(candidates[0])
	for _, candidate := range candidates[1:] {
		if roleOf(candidate) != role {
			return ""
		}
	}
	return role
}

// DispositionForRole projects one interval role without weakening unresolved roles.
func DispositionForRole(role Role) Disposition {
	if fillerRole(role) {
		return DispositionFillerCandidate
	}
	if role == RoleProgrammeFragment || role == RoleNonFiller {
		return DispositionNonFiller
	}
	return DispositionUnresolved
}

func valueRange(values []int64) (int64, int64) {
	minimum, maximum := values[0], values[0]
	for _, value := range values[1:] {
		minimum, maximum = min(minimum, value), max(maximum, value)
	}
	return minimum, maximum
}

func sortedReasons(reasons map[string]struct{}) []string {
	result := make([]string, 0, len(reasons))
	for reason := range reasons {
		result = append(result, reason)
	}
	sort.Strings(result)
	return result
}
