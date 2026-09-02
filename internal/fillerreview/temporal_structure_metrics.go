package fillerreview

import (
	"fmt"
	"sort"
	"time"

	"github.com/loomarr/loomarr/internal/fillereval"
)

const (
	temporalStructureBoundaryConstructedJoin = "constructed_join"
	temporalStructureBoundaryParentCutEdge   = "parent_cut_edge"
)

type temporalStructureTruthBoundary struct {
	kind string
	atMS int64
}

type temporalStructureSummaryKey struct {
	assessor string
	unit     fillereval.UnitKind
}

func buildTemporalStructureComparison(loaded temporalStructureComparisonLoaded, comparedAt time.Time) TemporalStructureComparisonReport {
	report := TemporalStructureComparisonReport{
		SchemaVersion: TemporalStructureComparisonSchemaVersion, ContractVersion: TemporalStructureComparisonContractVersion,
		ChallengeID: loaded.manifest.ChallengeID, PublicManifestSHA256: loaded.publicSHA, PrivateAuthoritySHA256: loaded.authoritySHA,
		ComparedAt: comparedAt, BoundaryTolerancesMS: []int64{TemporalStructureNearBoundaryMS, TemporalStructureBroadBoundaryMS},
		Cases: len(loaded.authority.Cases), ProductionAdmissionAllowed: false,
	}
	summaryByAssessor := make(map[string]*TemporalStructureAssessorSummary, len(loaded.assessments))
	constructionByKey := make(map[temporalStructureSummaryKey]*TemporalStructureConstructionSummary)
	boundaryDistances := make(map[string][]int64)
	constructionDistances := make(map[temporalStructureSummaryKey][]int64)
	for _, loadedAssessment := range loaded.assessments {
		set := loadedAssessment.set
		report.Assessors = append(report.Assessors, TemporalStructureAssessorReference{
			AssessmentSetSHA256: loadedAssessment.fileSHA, RawResultSHA256: set.RawResultSHA256,
			CapabilitySHA256: set.CapabilitySnapshotSHA256, CompletedAt: set.CompletedAt, Assessor: set.Assessor,
		})
		summaryByAssessor[set.Assessor.ID] = &TemporalStructureAssessorSummary{AssessorID: set.Assessor.ID, Cases: len(loaded.authority.Cases)}
		for _, unit := range []fillereval.UnitKind{fillereval.UnitStandalone, fillereval.UnitCompilation, fillereval.UnitProgrammeExcerpt} {
			key := temporalStructureSummaryKey{assessor: set.Assessor.ID, unit: unit}
			constructionByKey[key] = &TemporalStructureConstructionSummary{AssessorID: set.Assessor.ID, TruthUnit: unit}
		}
	}

	authorityCases := append([]TemporalStructureChallengeAuthorityCase(nil), loaded.authority.Cases...)
	sort.Slice(authorityCases, func(i, j int) bool { return authorityCases[i].Alias < authorityCases[j].Alias })
	for _, truthCase := range authorityCases {
		publicCase := temporalStructurePublicCase(loaded.manifest, truthCase.Alias)
		boundaries := temporalStructureTruthBoundaries(truthCase, publicCase.Video.DurationMS)
		comparison := TemporalStructureCaseComparison{
			Alias: truthCase.Alias, DurationMS: publicCase.Video.DurationMS,
			Truth: TemporalStructureTruthLabel{Unit: truthCase.Unit, Role: truthCase.Role},
		}
		allExact := true
		for _, loadedAssessment := range loaded.assessments {
			assessment := loadedAssessment.byAlias[truthCase.Alias]
			result := scoreTemporalStructureCase(loadedAssessment.set.Assessor.ID, truthCase, boundaries, assessment)
			comparison.Assessments = append(comparison.Assessments, result)
			allExact = allExact && result.ExactLabelCorrect
			assessorSummary := summaryByAssessor[result.AssessorID]
			constructionSummary := constructionByKey[temporalStructureSummaryKey{assessor: result.AssessorID, unit: truthCase.Unit}]
			accumulateTemporalStructureResult(assessorSummary, constructionSummary, result, assessment, len(boundaries))
			for _, distance := range result.BoundaryDistances {
				boundaryDistances[result.AssessorID] = append(boundaryDistances[result.AssessorID], distance.DistanceMS)
				key := temporalStructureSummaryKey{assessor: result.AssessorID, unit: truthCase.Unit}
				constructionDistances[key] = append(constructionDistances[key], distance.DistanceMS)
			}
		}
		if allExact {
			report.AllAssessorsExactCorrect++
		}
		if reasons := temporalStructureDiagnosticReasons(comparison); len(reasons) > 0 {
			report.DiagnosticCandidates = append(report.DiagnosticCandidates, TemporalStructureDiagnosticCandidate{Alias: comparison.Alias, Reasons: reasons})
		}
		report.CaseComparisons = append(report.CaseComparisons, comparison)
	}

	for _, loadedAssessment := range loaded.assessments {
		id := loadedAssessment.set.Assessor.ID
		summary := summaryByAssessor[id]
		summary.Boundary.MedianDistanceMS = temporalStructureMedian(boundaryDistances[id])
		report.AssessorSummaries = append(report.AssessorSummaries, *summary)
		for _, unit := range []fillereval.UnitKind{fillereval.UnitStandalone, fillereval.UnitCompilation, fillereval.UnitProgrammeExcerpt} {
			key := temporalStructureSummaryKey{assessor: id, unit: unit}
			constructionByKey[key].Boundary.MedianDistanceMS = temporalStructureMedian(constructionDistances[key])
			report.ConstructionSummaries = append(report.ConstructionSummaries, *constructionByKey[key])
		}
	}
	report.PairSummaries = buildTemporalStructurePairSummaries(report.CaseComparisons, loaded.assessments)
	report.Disposition = temporalStructureComparisonDisposition(report.DiagnosticCandidates)
	return report
}

func scoreTemporalStructureCase(assessorID string, truth TemporalStructureChallengeAuthorityCase, boundaries []temporalStructureTruthBoundary, assessment TemporalStructureAssessment) TemporalStructureAssessorCaseResult {
	result := TemporalStructureAssessorCaseResult{AssessorID: assessorID}
	if assessment.OperationalFailure != nil {
		result.Prediction.Failure = assessment.OperationalFailure.Code
		return result
	}
	result.Prediction.Unit = assessment.Unit.Kind
	if assessment.Role != nil {
		result.Prediction.Role = assessment.Role.Kind
	}
	result.UnitCorrect = result.Prediction.Unit == truth.Unit
	result.StandaloneClassCorrect = (result.Prediction.Unit == fillereval.UnitStandalone) == (truth.Unit == fillereval.UnitStandalone)
	result.RoleComparable = truth.Unit == fillereval.UnitStandalone && result.Prediction.Unit == fillereval.UnitStandalone
	result.RoleCorrect = result.RoleComparable && result.Prediction.Role == truth.Role
	result.ExactLabelCorrect = result.UnitCorrect && (truth.Unit != fillereval.UnitStandalone || result.RoleCorrect)
	if result.UnitCorrect && len(boundaries) > 0 {
		for _, boundary := range boundaries {
			nearest, distance := nearestTemporalStructureTime(boundary.atMS, assessment.Unit.DecisiveAtMS)
			result.BoundaryDistances = append(result.BoundaryDistances, TemporalStructureBoundaryDistance{
				Kind: boundary.kind, TruthAtMS: boundary.atMS, NearestDecisiveMS: nearest, DistanceMS: distance,
				Within2000MS: distance <= TemporalStructureNearBoundaryMS, Within5000MS: distance <= TemporalStructureBroadBoundaryMS,
			})
		}
	}
	return result
}

func accumulateTemporalStructureResult(summary *TemporalStructureAssessorSummary, construction *TemporalStructureConstructionSummary, result TemporalStructureAssessorCaseResult, assessment TemporalStructureAssessment, truthBoundaries int) {
	construction.Cases++
	summary.Boundary.TruthTargets += truthBoundaries
	construction.Boundary.TruthTargets += truthBoundaries
	if result.Prediction.Failure != "" {
		summary.OperationalFailures++
		construction.OperationalFailures++
		return
	}
	summary.UnitComparable++
	if assessment.Unit.Kind == fillereval.UnitUnclear {
		summary.SemanticAbstentions++
	}
	if assessment.Unit.Kind == fillereval.UnitUnusable {
		summary.UnusableClaims++
	}
	if result.UnitCorrect {
		summary.ExactUnitCorrect++
		construction.ExactUnitCorrect++
	}
	if result.StandaloneClassCorrect {
		summary.StandaloneClassCorrect++
		construction.StandaloneClassCorrect++
	}
	if result.RoleComparable {
		summary.RoleComparable++
		construction.RoleComparable++
	}
	if result.RoleCorrect {
		summary.RoleCorrect++
		construction.RoleCorrect++
	}
	if result.ExactLabelCorrect {
		summary.ExactLabelCorrect++
		construction.ExactLabelCorrect++
	}
	for _, distance := range result.BoundaryDistances {
		summary.Boundary.ComparableTargets++
		construction.Boundary.ComparableTargets++
		if distance.Within2000MS {
			summary.Boundary.Within2000MS++
			construction.Boundary.Within2000MS++
		}
		if distance.Within5000MS {
			summary.Boundary.Within5000MS++
			construction.Boundary.Within5000MS++
		}
	}
}

func temporalStructureTruthBoundaries(item TemporalStructureChallengeAuthorityCase, durationMS int64) []temporalStructureTruthBoundary {
	switch item.Unit {
	case fillereval.UnitCompilation:
		result := make([]temporalStructureTruthBoundary, 0, len(item.JoinTimesMS))
		for _, atMS := range item.JoinTimesMS {
			result = append(result, temporalStructureTruthBoundary{kind: temporalStructureBoundaryConstructedJoin, atMS: atMS})
		}
		return result
	case fillereval.UnitProgrammeExcerpt:
		return []temporalStructureTruthBoundary{{kind: temporalStructureBoundaryParentCutEdge, atMS: 0}, {kind: temporalStructureBoundaryParentCutEdge, atMS: durationMS}}
	default:
		return nil
	}
}

func nearestTemporalStructureTime(target int64, candidates []int64) (int64, int64) {
	nearest := candidates[0]
	distance := absoluteInt64(nearest - target)
	for _, candidate := range candidates[1:] {
		candidateDistance := absoluteInt64(candidate - target)
		if candidateDistance < distance || candidateDistance == distance && candidate < nearest {
			nearest, distance = candidate, candidateDistance
		}
	}
	return nearest, distance
}

func temporalStructureMedian(values []int64) *int64 {
	if len(values) == 0 {
		return nil
	}
	ordered := append([]int64(nil), values...)
	sort.Slice(ordered, func(i, j int) bool { return ordered[i] < ordered[j] })
	median := ordered[len(ordered)/2]
	if len(ordered)%2 == 0 {
		median = (ordered[len(ordered)/2-1] + median) / 2
	}
	return &median
}

func temporalStructurePublicCase(manifest TemporalStructureChallengeManifest, alias string) TemporalStructureChallengePublicCase {
	for _, item := range manifest.Cases {
		if item.Alias == alias {
			return item
		}
	}
	panic(fmt.Sprintf("validated temporal structure alias %q disappeared", alias))
}
