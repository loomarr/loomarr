package fillerreview

import (
	"encoding/json"
	"fmt"
	"sort"
	"time"

	"github.com/loomarr/loomarr/internal/fillereval"
)

const (
	TemporalSuitabilityComparisonSchemaVersion   = 1
	TemporalSuitabilityComparisonContractVersion = "filler-suitability-comparison-v1"
)

type TemporalSuitabilityComparisonConfig struct {
	EvidenceManifestPath string
	FirstResultPath      string
	SecondResultPath     string
	ComparedAt           time.Time
	ExpectedCases        int
	OutputPath           string
}

type TemporalSuitabilityComparisonReport struct {
	SchemaVersion                 int                                 `json:"schemaVersion"`
	ContractVersion               string                              `json:"contractVersion"`
	ComparedAt                    time.Time                           `json:"comparedAt"`
	EvidenceManifestSHA256        string                              `json:"evidenceManifestSha256"`
	SelectionSHA256               string                              `json:"selectionSha256"`
	FirstResultSHA256             string                              `json:"firstResultSha256"`
	SecondResultSHA256            string                              `json:"secondResultSha256"`
	FirstAssessor                 fillereval.TemporalAssessorIdentity `json:"firstAssessor"`
	SecondAssessor                fillereval.TemporalAssessorIdentity `json:"secondAssessor"`
	Cases                         int                                 `json:"cases"`
	FirstOperationalFailures      int                                 `json:"firstOperationalFailures"`
	SecondOperationalFailures     int                                 `json:"secondOperationalFailures"`
	FlaggedUnionCases             int                                 `json:"flaggedUnionCases"`
	CorroboratedProhibitedCases   int                                 `json:"corroboratedProhibitedCases"`
	UncorroboratedProhibitedCases int                                 `json:"uncorroboratedProhibitedCases"`
	OperationalHoldCases          int                                 `json:"operationalHoldCases"`
	CoverageHoldCases             int                                 `json:"coverageHoldCases"`
	CandidateNoSignalCases        int                                 `json:"candidateNoSignalCases"`
	CaseComparisons               []TemporalSuitabilityCaseComparison `json:"caseComparisons"`
	ProductionAdmissionAllowed    bool                                `json:"productionAdmissionAllowed"`
	NextAction                    string                              `json:"nextAction"`
}

type TemporalSuitabilityCaseComparison struct {
	EvidenceAlias     string                           `json:"evidenceAlias"`
	FirstOutcome      string                           `json:"firstOutcome"`
	SecondOutcome     string                           `json:"secondOutcome"`
	UnionFlags        []SuitabilityFlag                `json:"unionFlags,omitempty"`
	CorroboratedFlags []SuitabilityFlag                `json:"corroboratedFlags,omitempty"`
	FirstOnlyFlags    []TemporalSuitabilityObservation `json:"firstOnlyFlags,omitempty"`
	SecondOnlyFlags   []TemporalSuitabilityObservation `json:"secondOnlyFlags,omitempty"`
	Disposition       string                           `json:"disposition"`
}

func CompareTemporalSuitabilityResults(first, second TemporalSuitabilityResult, evidenceSHA, selectionSHA, firstSHA, secondSHA string, comparedAt time.Time) (TemporalSuitabilityComparisonReport, error) {
	if comparedAt.IsZero() || !reviewSHA256(evidenceSHA) || !reviewSHA256(selectionSHA) || !reviewSHA256(firstSHA) || !reviewSHA256(secondSHA) || firstSHA == secondSHA || len(first.Assessments) == 0 || len(first.Assessments) != len(second.Assessments) || len(first.SelectionAliases) != len(first.Assessments) || len(second.SelectionAliases) != len(second.Assessments) || first.EvidenceManifestSHA256 != evidenceSHA || second.EvidenceManifestSHA256 != evidenceSHA || first.SelectionSHA256 != selectionSHA || second.SelectionSHA256 != selectionSHA || first.Assessor.ID == second.Assessor.ID || first.Assessor.ModelFamily == second.Assessor.ModelFamily || first.Assessor.Provider != "openrouter" || second.Assessor.Provider != "openrouter" {
		return TemporalSuitabilityComparisonReport{}, fmt.Errorf("suitability comparison requires two distinct complete results over one evidence selection")
	}
	report := TemporalSuitabilityComparisonReport{
		SchemaVersion: TemporalSuitabilityComparisonSchemaVersion, ContractVersion: TemporalSuitabilityComparisonContractVersion,
		ComparedAt: comparedAt.UTC(), EvidenceManifestSHA256: evidenceSHA, SelectionSHA256: selectionSHA,
		FirstResultSHA256: firstSHA, SecondResultSHA256: secondSHA, FirstAssessor: first.Assessor, SecondAssessor: second.Assessor,
		Cases: len(first.Assessments), ProductionAdmissionAllowed: false, NextAction: "certify_suitability_recall_before_admission",
	}
	firstByAlias := suitabilityAssessmentIndex(first.Assessments)
	secondByAlias := suitabilityAssessmentIndex(second.Assessments)
	aliases := append([]string(nil), first.SelectionAliases...)
	sort.Strings(aliases)
	for _, alias := range aliases {
		firstAssessment, firstExists := firstByAlias[alias]
		secondAssessment, secondExists := secondByAlias[alias]
		if !firstExists || !secondExists {
			return TemporalSuitabilityComparisonReport{}, fmt.Errorf("suitability result alias sets differ")
		}
		comparison := compareTemporalSuitabilityCase(firstAssessment, secondAssessment)
		report.CaseComparisons = append(report.CaseComparisons, comparison)
		if firstAssessment.OperationalFailure != nil {
			report.FirstOperationalFailures++
		}
		if secondAssessment.OperationalFailure != nil {
			report.SecondOperationalFailures++
		}
		if len(comparison.UnionFlags) > 0 {
			report.FlaggedUnionCases++
			if len(comparison.CorroboratedFlags) > 0 {
				report.CorroboratedProhibitedCases++
			} else {
				report.UncorroboratedProhibitedCases++
			}
		}
		switch comparison.Disposition {
		case "operational_hold":
			report.OperationalHoldCases++
		case "coverage_hold":
			report.CoverageHoldCases++
		case "candidate_no_signal_observed":
			report.CandidateNoSignalCases++
		}
	}
	if report.FlaggedUnionCases+report.OperationalHoldCases+report.CoverageHoldCases+report.CandidateNoSignalCases != report.Cases {
		return TemporalSuitabilityComparisonReport{}, fmt.Errorf("suitability comparison dispositions are not exhaustive")
	}
	return report, nil
}

func compareTemporalSuitabilityCase(first, second TemporalSuitabilityAssessment) TemporalSuitabilityCaseComparison {
	comparison := TemporalSuitabilityCaseComparison{
		EvidenceAlias: first.EvidenceAlias, FirstOutcome: suitabilityAssessmentOutcome(first), SecondOutcome: suitabilityAssessmentOutcome(second),
	}
	matchedSecond := make(map[int]struct{})
	union := make(map[SuitabilityFlag]struct{})
	corroborated := make(map[SuitabilityFlag]struct{})
	for _, flag := range first.Flags {
		union[flag.Kind] = struct{}{}
		matched := false
		for secondIndex, secondFlag := range second.Flags {
			if flag.Kind == secondFlag.Kind && temporalSuitabilityRangesOverlap(flag, secondFlag) {
				matched = true
				matchedSecond[secondIndex] = struct{}{}
				corroborated[flag.Kind] = struct{}{}
			}
		}
		if !matched {
			comparison.FirstOnlyFlags = append(comparison.FirstOnlyFlags, flag)
		}
	}
	for index, flag := range second.Flags {
		union[flag.Kind] = struct{}{}
		if _, matched := matchedSecond[index]; !matched {
			comparison.SecondOnlyFlags = append(comparison.SecondOnlyFlags, flag)
		}
	}
	comparison.UnionFlags = sortedSuitabilityFlags(union)
	comparison.CorroboratedFlags = sortedSuitabilityFlags(corroborated)
	switch {
	case len(comparison.UnionFlags) > 0:
		comparison.Disposition = "prohibited_hold"
	case first.OperationalFailure != nil || second.OperationalFailure != nil:
		comparison.Disposition = "operational_hold"
	case first.VisualAssessment != suitabilityVisualCompleted || first.SpokenLanguageAssessment != suitabilityLanguageCompleted || second.VisualAssessment != suitabilityVisualCompleted || second.SpokenLanguageAssessment != suitabilityLanguageCompleted:
		comparison.Disposition = "coverage_hold"
	default:
		comparison.Disposition = "candidate_no_signal_observed"
	}
	return comparison
}

func temporalSuitabilityRangesOverlap(first, second TemporalSuitabilityObservation) bool {
	return first.StartMS < second.EndMS && second.StartMS < first.EndMS
}

func suitabilityAssessmentOutcome(assessment TemporalSuitabilityAssessment) string {
	if assessment.OperationalFailure != nil {
		return "failure:" + string(assessment.OperationalFailure.Code)
	}
	return string(assessment.Outcome)
}

func suitabilityAssessmentIndex(items []TemporalSuitabilityAssessment) map[string]TemporalSuitabilityAssessment {
	result := make(map[string]TemporalSuitabilityAssessment, len(items))
	for _, item := range items {
		result[item.EvidenceAlias] = item
	}
	return result
}

func sortedSuitabilityFlags(values map[SuitabilityFlag]struct{}) []SuitabilityFlag {
	result := make([]SuitabilityFlag, 0, len(values))
	for value := range values {
		result = append(result, value)
	}
	sort.Slice(result, func(i, j int) bool { return result[i] < result[j] })
	return result
}

func PublishTemporalSuitabilityComparison(config TemporalSuitabilityComparisonConfig) (TemporalSuitabilityComparisonReport, string, error) {
	loaded, err := loadTemporalSuitabilityComparison(config)
	if err != nil {
		return TemporalSuitabilityComparisonReport{}, "", err
	}
	report, err := CompareTemporalSuitabilityResults(loaded.first, loaded.second, loaded.evidenceSHA, loaded.selectionSHA, loaded.firstSHA, loaded.secondSHA, config.ComparedAt)
	if err != nil {
		return TemporalSuitabilityComparisonReport{}, "", err
	}
	raw, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return TemporalSuitabilityComparisonReport{}, "", err
	}
	raw = append(raw, '\n')
	if err := writeTemporalTruthNew(config.OutputPath, raw, 0o600); err != nil {
		return TemporalSuitabilityComparisonReport{}, "", err
	}
	return report, hashBytes(raw), nil
}
