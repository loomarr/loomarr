package fillerreview

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/loomarr/loomarr/internal/fillereval"
)

const (
	TemporalPanelComparisonSchemaVersion   = 1
	TemporalPanelComparisonContractVersion = "filler-temporal-panel-comparison-v1"
	TemporalPanelMaximumTargetedCases      = 7
)

type TemporalPanelComparisonConfig struct {
	HumanAssessmentPath   string
	HumanAttestationPath  string
	FirstAssessmentPath   string
	FirstAttestationPath  string
	SecondAssessmentPath  string
	SecondAttestationPath string
	ExpectedCases         int
	ComparedAt            time.Time
	OutputPath            string
}

type TemporalPanelComparisonReport struct {
	SchemaVersion               int                                 `json:"schemaVersion"`
	ContractVersion             string                              `json:"contractVersion"`
	ComparedAt                  time.Time                           `json:"comparedAt"`
	EvidenceManifestSHA256      string                              `json:"evidenceManifestSha256"`
	SelectionSHA256             string                              `json:"selectionSha256"`
	HumanAssessmentSetSHA256    string                              `json:"humanAssessmentSetSha256"`
	HumanAttestationFileSHA256  string                              `json:"humanAttestationFileSha256"`
	HumanAttestationSHA256      string                              `json:"humanAttestationSha256"`
	FirstAssessmentSetSHA256    string                              `json:"firstAssessmentSetSha256"`
	FirstAttestationFileSHA256  string                              `json:"firstAttestationFileSha256"`
	FirstAttestationSHA256      string                              `json:"firstAttestationSha256"`
	SecondAssessmentSetSHA256   string                              `json:"secondAssessmentSetSha256"`
	SecondAttestationFileSHA256 string                              `json:"secondAttestationFileSha256"`
	SecondAttestationSHA256     string                              `json:"secondAttestationSha256"`
	HumanReviewerID             string                              `json:"humanReviewerId"`
	FirstAssessor               fillereval.TemporalAssessorIdentity `json:"firstAssessor"`
	SecondAssessor              fillereval.TemporalAssessorIdentity `json:"secondAssessor"`
	Cases                       int                                 `json:"cases"`
	HumanTimestampsAtStart      int                                 `json:"humanTimestampsAtStart"`
	HumanTimestampsInformative  int                                 `json:"humanTimestampsInformative"`
	PairSummaries               []TemporalPanelPairSummary          `json:"pairSummaries"`
	ThreeWayExactUnitAgreement  int                                 `json:"threeWayExactUnitAgreement"`
	ThreeWayStandaloneAgreement int                                 `json:"threeWayStandaloneAgreement"`
	ThreeWayRoleComparable      int                                 `json:"threeWayRoleComparable"`
	ThreeWayRoleAgreement       int                                 `json:"threeWayRoleAgreement"`
	CaseComparisons             []TemporalPanelCaseComparison       `json:"caseComparisons"`
	DiagnosticCandidates        []TemporalPanelDiagnosticCandidate  `json:"diagnosticCandidates,omitempty"`
	Disposition                 TemporalPanelDisposition            `json:"disposition"`
}

type TemporalPanelPairSummary struct {
	Pair                      string                          `json:"pair"`
	Cases                     int                             `json:"cases"`
	OperationalFailures       int                             `json:"operationalFailures"`
	ExactUnitComparable       int                             `json:"exactUnitComparable"`
	ExactUnitAgreement        int                             `json:"exactUnitAgreement"`
	StandaloneClassComparable int                             `json:"standaloneClassComparable"`
	StandaloneClassAgreement  int                             `json:"standaloneClassAgreement"`
	RoleComparable            int                             `json:"roleComparable"`
	RoleAgreement             int                             `json:"roleAgreement"`
	ExactLabelAgreement       int                             `json:"exactLabelAgreement"`
	UnitEvidenceDistance      TemporalEvidenceDistanceSummary `json:"unitEvidenceDistance"`
	RoleEvidenceDistance      TemporalEvidenceDistanceSummary `json:"roleEvidenceDistance"`
}

type TemporalEvidenceDistanceSummary struct {
	Comparable   int `json:"comparable"`
	Within2000MS int `json:"within2000Ms"`
	Within5000MS int `json:"within5000Ms"`
}

type TemporalPanelLabel struct {
	Unit    fillereval.UnitKind            `json:"unit,omitempty"`
	Role    fillereval.TemporalRole        `json:"role,omitempty"`
	Failure fillereval.TemporalFailureCode `json:"failure,omitempty"`
}

type TemporalPanelPairComparison struct {
	Comparable               bool   `json:"comparable"`
	ExactUnitAgreement       bool   `json:"exactUnitAgreement"`
	StandaloneClassAgreement bool   `json:"standaloneClassAgreement"`
	RoleComparable           bool   `json:"roleComparable"`
	RoleAgreement            bool   `json:"roleAgreement"`
	ExactLabelAgreement      bool   `json:"exactLabelAgreement"`
	UnitEvidenceDistanceMS   *int64 `json:"unitEvidenceDistanceMs,omitempty"`
	RoleEvidenceDistanceMS   *int64 `json:"roleEvidenceDistanceMs,omitempty"`
}

type TemporalPanelCaseComparison struct {
	EvidenceAlias             string                      `json:"evidenceAlias"`
	Human                     TemporalPanelLabel          `json:"human"`
	First                     TemporalPanelLabel          `json:"first"`
	Second                    TemporalPanelLabel          `json:"second"`
	HumanTimestampInformative bool                        `json:"humanTimestampInformative"`
	HumanFirst                TemporalPanelPairComparison `json:"humanFirst"`
	HumanSecond               TemporalPanelPairComparison `json:"humanSecond"`
	FirstSecond               TemporalPanelPairComparison `json:"firstSecond"`
}

type TemporalPanelDiagnosticCandidate struct {
	EvidenceAlias string   `json:"evidenceAlias"`
	Reasons       []string `json:"reasons"`
}

type TemporalPanelDisposition struct {
	NextAction                 string   `json:"nextAction"`
	DiagnosticCandidateCases   int      `json:"diagnosticCandidateCases"`
	MaximumTargetedCases       int      `json:"maximumTargetedCases"`
	TargetedHumanReviewAllowed bool     `json:"targetedHumanReviewAllowed"`
	TargetedHumanReviewCases   []string `json:"targetedHumanReviewCases,omitempty"`
	ProductionAdmissionAllowed bool     `json:"productionAdmissionAllowed"`
}

type temporalPanelRater struct {
	label           TemporalPanelLabel
	unitDecisiveMS  []int64
	roleDecisiveMS  []int64
	humanDecisiveMS *int64
}

// CompareTemporalPanel validates and joins one human reference and two
// independently blinded model panels. Agreement is diagnostic only: one human
// reference is not truth, and temporal role evidence cannot certify media
// quality or airworthiness.
func CompareTemporalPanel(config TemporalPanelComparisonConfig) (TemporalPanelComparisonReport, error) {
	loaded, err := loadTemporalPanelComparison(config)
	if err != nil {
		return TemporalPanelComparisonReport{}, err
	}
	report := loaded.reportIdentity(config.ComparedAt.UTC())
	humanByAlias := temporalHumanAssessmentIndex(loaded.human.Assessments)
	firstByAlias := temporalModelAssessmentIndex(loaded.first.Assessments)
	secondByAlias := temporalModelAssessmentIndex(loaded.second.Assessments)

	aliases := make([]string, 0, len(humanByAlias))
	for alias := range humanByAlias {
		aliases = append(aliases, alias)
	}
	sort.Strings(aliases)
	pairSummaries := map[string]*TemporalPanelPairSummary{
		"human_first":  {Pair: "human_first", Cases: len(aliases)},
		"human_second": {Pair: "human_second", Cases: len(aliases)},
		"first_second": {Pair: "first_second", Cases: len(aliases)},
	}

	for _, alias := range aliases {
		human := temporalHumanPanelRater(humanByAlias[alias])
		first := temporalModelPanelRater(firstByAlias[alias])
		second := temporalModelPanelRater(secondByAlias[alias])
		informative := human.humanDecisiveMS != nil && *human.humanDecisiveMS > 0
		if informative {
			report.HumanTimestampsInformative++
		} else {
			report.HumanTimestampsAtStart++
		}
		humanFirst := compareTemporalPanelPair(human, first, informative)
		humanSecond := compareTemporalPanelPair(human, second, informative)
		firstSecond := compareTemporalPanelPair(first, second, true)
		accumulateTemporalPanelPair(pairSummaries["human_first"], humanFirst)
		accumulateTemporalPanelPair(pairSummaries["human_second"], humanSecond)
		accumulateTemporalPanelPair(pairSummaries["first_second"], firstSecond)

		if human.label.Failure == "" && first.label.Failure == "" && second.label.Failure == "" {
			if human.label.Unit == first.label.Unit && first.label.Unit == second.label.Unit {
				report.ThreeWayExactUnitAgreement++
			}
			if temporalStandalone(human.label) == temporalStandalone(first.label) && temporalStandalone(first.label) == temporalStandalone(second.label) {
				report.ThreeWayStandaloneAgreement++
			}
			if temporalStandalone(human.label) && temporalStandalone(first.label) && temporalStandalone(second.label) {
				report.ThreeWayRoleComparable++
				if human.label.Role == first.label.Role && first.label.Role == second.label.Role {
					report.ThreeWayRoleAgreement++
				}
			}
		}

		comparison := TemporalPanelCaseComparison{
			EvidenceAlias: alias, Human: human.label, First: first.label, Second: second.label,
			HumanTimestampInformative: informative, HumanFirst: humanFirst, HumanSecond: humanSecond, FirstSecond: firstSecond,
		}
		report.CaseComparisons = append(report.CaseComparisons, comparison)
		if reasons := temporalPanelDiagnosticReasons(comparison); len(reasons) > 0 {
			report.DiagnosticCandidates = append(report.DiagnosticCandidates, TemporalPanelDiagnosticCandidate{EvidenceAlias: alias, Reasons: reasons})
		}
	}
	report.PairSummaries = []TemporalPanelPairSummary{*pairSummaries["human_first"], *pairSummaries["human_second"], *pairSummaries["first_second"]}
	report.Disposition = temporalPanelDisposition(report.DiagnosticCandidates)
	return report, nil
}

func PublishTemporalPanelComparison(config TemporalPanelComparisonConfig) (TemporalPanelComparisonReport, string, error) {
	if strings.TrimSpace(config.OutputPath) == "" {
		return TemporalPanelComparisonReport{}, "", fmt.Errorf("temporal panel comparison output path is required")
	}
	report, err := CompareTemporalPanel(config)
	if err != nil {
		return TemporalPanelComparisonReport{}, "", err
	}
	raw, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return TemporalPanelComparisonReport{}, "", err
	}
	raw = append(raw, '\n')
	if err := writeTemporalTruthNew(config.OutputPath, raw, 0o600); err != nil {
		return TemporalPanelComparisonReport{}, "", fmt.Errorf("publish temporal panel comparison: %w", err)
	}
	return report, hashBytes(raw), nil
}

func temporalHumanAssessmentIndex(items []TemporalHumanReviewAssessment) map[string]TemporalHumanReviewAssessment {
	result := make(map[string]TemporalHumanReviewAssessment, len(items))
	for _, item := range items {
		result[item.EvidenceAlias] = item
	}
	return result
}

func temporalModelAssessmentIndex(items []TemporalLockedModelAssessment) map[string]TemporalLockedModelAssessment {
	result := make(map[string]TemporalLockedModelAssessment, len(items))
	for _, item := range items {
		result[item.EvidenceAlias] = item
	}
	return result
}

func temporalHumanPanelRater(item TemporalHumanReviewAssessment) temporalPanelRater {
	role := fillereval.TemporalRole("")
	if item.Role != nil {
		role = *item.Role
	}
	decisive := item.DecisiveAtMS
	return temporalPanelRater{label: TemporalPanelLabel{Unit: item.Unit, Role: role}, humanDecisiveMS: &decisive}
}

func temporalModelPanelRater(item TemporalLockedModelAssessment) temporalPanelRater {
	result := temporalPanelRater{unitDecisiveMS: item.UnitDecisiveAtMS, roleDecisiveMS: item.RoleDecisiveAtMS}
	if item.OperationalFailure != nil {
		result.label.Failure = item.OperationalFailure.Code
		return result
	}
	result.label.Unit = item.Unit.Kind
	if item.Role != nil {
		result.label.Role = item.Role.Kind
	}
	return result
}

func compareTemporalPanelPair(left, right temporalPanelRater, useHumanTimestamp bool) TemporalPanelPairComparison {
	result := TemporalPanelPairComparison{Comparable: left.label.Failure == "" && right.label.Failure == ""}
	if !result.Comparable {
		return result
	}
	result.ExactUnitAgreement = left.label.Unit == right.label.Unit
	result.StandaloneClassAgreement = temporalStandalone(left.label) == temporalStandalone(right.label)
	result.RoleComparable = temporalStandalone(left.label) && temporalStandalone(right.label)
	result.RoleAgreement = result.RoleComparable && left.label.Role == right.label.Role
	result.ExactLabelAgreement = result.ExactUnitAgreement && (!temporalStandalone(left.label) || result.RoleAgreement)
	if useHumanTimestamp {
		result.UnitEvidenceDistanceMS = temporalPanelEvidenceDistance(left, right, false)
		if result.RoleComparable {
			result.RoleEvidenceDistanceMS = temporalPanelEvidenceDistance(left, right, true)
		}
	}
	return result
}

func temporalPanelEvidenceDistance(left, right temporalPanelRater, role bool) *int64 {
	leftTimes := left.unitDecisiveMS
	rightTimes := right.unitDecisiveMS
	if role {
		leftTimes, rightTimes = left.roleDecisiveMS, right.roleDecisiveMS
	}
	if left.humanDecisiveMS != nil {
		leftTimes = []int64{*left.humanDecisiveMS}
	}
	if right.humanDecisiveMS != nil {
		rightTimes = []int64{*right.humanDecisiveMS}
	}
	if len(leftTimes) == 0 || len(rightTimes) == 0 {
		return nil
	}
	minimum := absTemporalDistance(leftTimes[0], rightTimes[0])
	for _, leftTime := range leftTimes {
		for _, rightTime := range rightTimes {
			minimum = min(minimum, absTemporalDistance(leftTime, rightTime))
		}
	}
	return &minimum
}

func absTemporalDistance(left, right int64) int64 {
	if left >= right {
		return left - right
	}
	return right - left
}

func temporalStandalone(label TemporalPanelLabel) bool {
	return label.Unit == fillereval.UnitStandalone
}

func accumulateTemporalPanelPair(summary *TemporalPanelPairSummary, comparison TemporalPanelPairComparison) {
	if !comparison.Comparable {
		summary.OperationalFailures++
		return
	}
	summary.ExactUnitComparable++
	summary.StandaloneClassComparable++
	if comparison.ExactUnitAgreement {
		summary.ExactUnitAgreement++
	}
	if comparison.StandaloneClassAgreement {
		summary.StandaloneClassAgreement++
	}
	if comparison.RoleComparable {
		summary.RoleComparable++
		if comparison.RoleAgreement {
			summary.RoleAgreement++
		}
	}
	if comparison.ExactLabelAgreement {
		summary.ExactLabelAgreement++
	}
	accumulateTemporalEvidenceDistance(&summary.UnitEvidenceDistance, comparison.UnitEvidenceDistanceMS)
	accumulateTemporalEvidenceDistance(&summary.RoleEvidenceDistance, comparison.RoleEvidenceDistanceMS)
}

func accumulateTemporalEvidenceDistance(summary *TemporalEvidenceDistanceSummary, distance *int64) {
	if distance == nil {
		return
	}
	summary.Comparable++
	if *distance <= 2_000 {
		summary.Within2000MS++
	}
	if *distance <= 5_000 {
		summary.Within5000MS++
	}
}

func temporalPanelDiagnosticReasons(comparison TemporalPanelCaseComparison) []string {
	reasons := make([]string, 0, 5)
	if comparison.First.Failure != "" || comparison.Second.Failure != "" {
		reasons = append(reasons, "operational_failure")
	}
	if !comparison.HumanFirst.ExactUnitAgreement || !comparison.HumanSecond.ExactUnitAgreement || !comparison.FirstSecond.ExactUnitAgreement {
		reasons = append(reasons, "unit_disagreement")
	}
	if !comparison.HumanFirst.StandaloneClassAgreement || !comparison.HumanSecond.StandaloneClassAgreement || !comparison.FirstSecond.StandaloneClassAgreement {
		reasons = append(reasons, "standalone_class_disagreement")
	}
	if comparison.HumanFirst.RoleComparable && !comparison.HumanFirst.RoleAgreement || comparison.HumanSecond.RoleComparable && !comparison.HumanSecond.RoleAgreement || comparison.FirstSecond.RoleComparable && !comparison.FirstSecond.RoleAgreement {
		reasons = append(reasons, "role_disagreement")
	}
	if comparison.Human.Unit == fillereval.UnitUnusable && comparison.First.Unit != fillereval.UnitUnusable && comparison.Second.Unit != fillereval.UnitUnusable {
		reasons = append(reasons, "human_unusable_model_miss")
	}
	sort.Strings(reasons)
	return reasons
}

func temporalPanelDisposition(candidates []TemporalPanelDiagnosticCandidate) TemporalPanelDisposition {
	result := TemporalPanelDisposition{
		DiagnosticCandidateCases: len(candidates), MaximumTargetedCases: TemporalPanelMaximumTargetedCases,
		ProductionAdmissionAllowed: false,
	}
	switch {
	case len(candidates) == 0:
		result.NextAction = "proceed_to_quality_and_suitability_evaluation"
	case len(candidates) > TemporalPanelMaximumTargetedCases:
		result.NextAction = "improve_pipeline"
	default:
		result.NextAction = "targeted_review_optional"
		result.TargetedHumanReviewAllowed = true
		for _, candidate := range candidates {
			result.TargetedHumanReviewCases = append(result.TargetedHumanReviewCases, candidate.EvidenceAlias)
		}
	}
	return result
}
