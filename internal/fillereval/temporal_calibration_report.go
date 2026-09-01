package fillereval

import (
	"fmt"
	"reflect"
	"slices"
	"sort"
	"strings"
)

const (
	TemporalCalibrationReportSchemaVersion   = 2
	TemporalCalibrationReportContractVersion = "filler-temporal-calibration-report-v2"
)

type TemporalCalibrationRelation string

const (
	TemporalCalibrationNotApplicable      TemporalCalibrationRelation = "not_applicable"
	TemporalCalibrationNotComparable      TemporalCalibrationRelation = "not_comparable"
	TemporalCalibrationOperationalFailure TemporalCalibrationRelation = "operational_failure"
	TemporalCalibrationAgreementPreserved TemporalCalibrationRelation = "agreement_preserved"
	TemporalCalibrationAgreementBroken    TemporalCalibrationRelation = "agreement_broken"
	TemporalCalibrationMatchedFirst       TemporalCalibrationRelation = "matched_first"
	TemporalCalibrationMatchedSecond      TemporalCalibrationRelation = "matched_second"
	TemporalCalibrationMatchedNeither     TemporalCalibrationRelation = "matched_neither"
)

type TemporalCalibrationReport struct {
	SchemaVersion              int                                `json:"schemaVersion"`
	ContractVersion            string                             `json:"contractVersion"`
	BatchID                    string                             `json:"batchId"`
	PackageSHA256              string                             `json:"packageSha256"`
	SelectionSHA256            string                             `json:"selectionSha256"`
	ComparisonSHA256           string                             `json:"comparisonSha256"`
	FirstAssessmentSHA256      string                             `json:"firstAssessmentSha256"`
	SecondAssessmentSHA256     string                             `json:"secondAssessmentSha256"`
	ThirdAssessmentSHA256      string                             `json:"thirdAssessmentSha256"`
	FirstAssessor              TemporalAssessorIdentity           `json:"firstAssessor"`
	SecondAssessor             TemporalAssessorIdentity           `json:"secondAssessor"`
	ThirdAssessor              TemporalAssessorIdentity           `json:"thirdAssessor"`
	Cases                      int                                `json:"cases"`
	OperationalFailures        int                                `json:"operationalFailures"`
	ThirdUnitUnclear           int                                `json:"thirdUnitUnclear"`
	ThirdRoleUnclear           int                                `json:"thirdRoleUnclear"`
	AgreementControls          int                                `json:"agreementControls"`
	AgreementControlsPreserved int                                `json:"agreementControlsPreserved"`
	AgreementControlsBroken    int                                `json:"agreementControlsBroken"`
	UnitRelations              []TemporalCalibrationRelationCount `json:"unitRelations"`
	RoleRelations              []TemporalCalibrationRelationCount `json:"roleRelations"`
	ThirdUnits                 []TemporalCalibrationValueCount    `json:"thirdUnits"`
	ThirdRoles                 []TemporalCalibrationValueCount    `json:"thirdRoles"`
	CaseResults                []TemporalCalibrationCaseResult    `json:"caseResults"`
	Disposition                TemporalCalibrationDisposition     `json:"disposition"`
}

type TemporalCalibrationRelationCount struct {
	Relation TemporalCalibrationRelation `json:"relation"`
	Count    int                         `json:"count"`
	Aliases  []string                    `json:"aliases"`
}

type TemporalCalibrationValueCount struct {
	Value   string   `json:"value"`
	Count   int      `json:"count"`
	Aliases []string `json:"aliases"`
}

type TemporalCalibrationCaseResult struct {
	Alias         string                      `json:"alias"`
	Reasons       []string                    `json:"reasons"`
	Strata        []string                    `json:"strata"`
	FirstUnit     string                      `json:"firstUnit,omitempty"`
	SecondUnit    string                      `json:"secondUnit,omitempty"`
	ThirdUnit     string                      `json:"thirdUnit,omitempty"`
	FirstRole     string                      `json:"firstRole,omitempty"`
	SecondRole    string                      `json:"secondRole,omitempty"`
	ThirdRole     string                      `json:"thirdRole,omitempty"`
	FirstFailure  string                      `json:"firstFailure,omitempty"`
	SecondFailure string                      `json:"secondFailure,omitempty"`
	ThirdFailure  *TemporalOperationalFailure `json:"thirdFailure,omitempty"`
	UnitRelation  TemporalCalibrationRelation `json:"unitRelation"`
	RoleRelation  TemporalCalibrationRelation `json:"roleRelation"`
}

// AnalyzeTemporalCalibration verifies the original two-model comparison and
// describes how a distinct third assessor relates to each selected case. It
// deliberately does not treat any model as truth or emit a recommendation.
func AnalyzeTemporalCalibration(selection TemporalCalibrationSelection, selectionSHA256 string, first, second, third TemporalAssessmentSet, allCases []TemporalCaseSignals) (TemporalCalibrationReport, error) {
	if !validSHA256(selectionSHA256) {
		return TemporalCalibrationReport{}, fmt.Errorf("temporal calibration selection artifact SHA-256 is invalid")
	}
	recomputed, err := CompareTemporalAssessmentSets(first, second, allCases)
	if err != nil {
		return TemporalCalibrationReport{}, err
	}
	recomputedSelection, err := BuildTemporalCalibrationSelection(recomputed)
	if err != nil {
		return TemporalCalibrationReport{}, err
	}
	if !reflect.DeepEqual(selection, recomputedSelection) {
		return TemporalCalibrationReport{}, fmt.Errorf("temporal calibration selection does not match the bound two-model comparison")
	}
	selectedCases, err := calibrationSignals(selection, allCases)
	if err != nil {
		return TemporalCalibrationReport{}, err
	}
	if err := ValidateTemporalAssessmentSet(third, selection.BatchID, selection.PackageSHA256, selectedCases); err != nil {
		return TemporalCalibrationReport{}, fmt.Errorf("third assessment set: %w", err)
	}
	if third.Assessor.ID == first.Assessor.ID || third.Assessor.ID == second.Assessor.ID || strings.EqualFold(third.Assessor.ModelFamily, first.Assessor.ModelFamily) || strings.EqualFold(third.Assessor.ModelFamily, second.Assessor.ModelFamily) {
		return TemporalCalibrationReport{}, fmt.Errorf("third assessment set requires a distinct assessor and model family")
	}

	report := TemporalCalibrationReport{
		SchemaVersion: TemporalCalibrationReportSchemaVersion, ContractVersion: TemporalCalibrationReportContractVersion,
		BatchID: selection.BatchID, PackageSHA256: selection.PackageSHA256, SelectionSHA256: selectionSHA256,
		ComparisonSHA256: selection.ComparisonSHA256, FirstAssessmentSHA256: selection.FirstAssessmentSHA256,
		SecondAssessmentSHA256: selection.SecondAssessmentSHA256, ThirdAssessmentSHA256: TemporalAssessmentSetSHA256(third),
		FirstAssessor: first.Assessor, SecondAssessor: second.Assessor, ThirdAssessor: third.Assessor, Cases: len(selection.Cases),
	}
	firstByAlias, secondByAlias, thirdByAlias := temporalAssessmentIndex(first.Assessments), temporalAssessmentIndex(second.Assessments), temporalAssessmentIndex(third.Assessments)
	unitRelations := map[TemporalCalibrationRelation][]string{}
	roleRelations := map[TemporalCalibrationRelation][]string{}
	thirdUnits := map[string][]string{}
	thirdRoles := map[string][]string{}
	for _, selected := range selection.Cases {
		a, b, c := firstByAlias[selected.Alias], secondByAlias[selected.Alias], thirdByAlias[selected.Alias]
		result := calibrationCaseResult(selected, a, b, c)
		report.CaseResults = append(report.CaseResults, result)
		unitRelations[result.UnitRelation] = append(unitRelations[result.UnitRelation], result.Alias)
		roleRelations[result.RoleRelation] = append(roleRelations[result.RoleRelation], result.Alias)
		if c.OperationalFailure != nil {
			report.OperationalFailures++
		} else {
			thirdUnits[string(c.Unit.Kind)] = append(thirdUnits[string(c.Unit.Kind)], c.Alias)
			if c.Unit.Kind == UnitUnclear {
				report.ThirdUnitUnclear++
			}
			if c.Role != nil {
				thirdRoles[string(c.Role.Kind)] = append(thirdRoles[string(c.Role.Kind)], c.Alias)
				if c.Role.Kind == TemporalRoleUnclear {
					report.ThirdRoleUnclear++
				}
			}
		}
		if slices.Contains(selected.Reasons, "agreement_control") {
			report.AgreementControls++
			preserved := result.UnitRelation == TemporalCalibrationAgreementPreserved && (result.RoleRelation == TemporalCalibrationAgreementPreserved || result.RoleRelation == TemporalCalibrationNotApplicable)
			if preserved {
				report.AgreementControlsPreserved++
			} else {
				report.AgreementControlsBroken++
			}
		}
	}
	report.UnitRelations = calibrationRelationCounts(unitRelations)
	report.RoleRelations = calibrationRelationCounts(roleRelations)
	report.ThirdUnits = calibrationValueCounts(thirdUnits)
	report.ThirdRoles = calibrationValueCounts(thirdRoles)
	report.Disposition = temporalCalibrationDisposition(report)
	return report, nil
}

func calibrationSignals(selection TemporalCalibrationSelection, allCases []TemporalCaseSignals) ([]TemporalCaseSignals, error) {
	byAlias := make(map[string]TemporalCaseSignals, len(allCases))
	for _, item := range allCases {
		byAlias[item.Alias] = item
	}
	selected := make([]TemporalCaseSignals, 0, len(selection.Cases))
	for _, item := range selection.Cases {
		signals, exists := byAlias[item.Alias]
		if !exists {
			return nil, fmt.Errorf("temporal calibration selection names unknown alias %q", item.Alias)
		}
		selected = append(selected, signals)
	}
	return selected, nil
}

func calibrationCaseResult(selected TemporalCalibrationCase, first, second, third TemporalAssessment) TemporalCalibrationCaseResult {
	result := TemporalCalibrationCaseResult{Alias: selected.Alias, Reasons: slices.Clone(selected.Reasons), Strata: slices.Clone(selected.Strata)}
	result.FirstUnit, result.FirstRole, result.FirstFailure = calibrationClaim(first)
	result.SecondUnit, result.SecondRole, result.SecondFailure = calibrationClaim(second)
	result.ThirdUnit, result.ThirdRole, _ = calibrationClaim(third)
	result.ThirdFailure = third.OperationalFailure
	if third.OperationalFailure != nil {
		result.UnitRelation, result.RoleRelation = TemporalCalibrationOperationalFailure, TemporalCalibrationOperationalFailure
		return result
	}
	result.UnitRelation = calibrationRelation(result.FirstUnit, result.SecondUnit, result.ThirdUnit)
	if result.FirstRole == "" || result.SecondRole == "" {
		result.RoleRelation = TemporalCalibrationNotApplicable
	} else if result.ThirdRole == "" {
		result.RoleRelation = TemporalCalibrationNotComparable
	} else {
		result.RoleRelation = calibrationRelation(result.FirstRole, result.SecondRole, result.ThirdRole)
	}
	return result
}

func calibrationClaim(assessment TemporalAssessment) (unit, role, failure string) {
	if assessment.OperationalFailure != nil {
		return "", "", string(assessment.OperationalFailure.Code)
	}
	unit = string(assessment.Unit.Kind)
	if assessment.Role != nil {
		role = string(assessment.Role.Kind)
	}
	return unit, role, ""
}

func calibrationRelation(first, second, third string) TemporalCalibrationRelation {
	if first == "" || second == "" {
		return TemporalCalibrationNotComparable
	}
	if first == second {
		if third == first {
			return TemporalCalibrationAgreementPreserved
		}
		return TemporalCalibrationAgreementBroken
	}
	if third == first {
		return TemporalCalibrationMatchedFirst
	}
	if third == second {
		return TemporalCalibrationMatchedSecond
	}
	return TemporalCalibrationMatchedNeither
}

func calibrationRelationCounts(values map[TemporalCalibrationRelation][]string) []TemporalCalibrationRelationCount {
	keys := make([]string, 0, len(values))
	for value := range values {
		keys = append(keys, string(value))
	}
	sort.Strings(keys)
	counts := make([]TemporalCalibrationRelationCount, 0, len(keys))
	for _, key := range keys {
		aliases := slices.Clone(values[TemporalCalibrationRelation(key)])
		sort.Strings(aliases)
		counts = append(counts, TemporalCalibrationRelationCount{Relation: TemporalCalibrationRelation(key), Count: len(aliases), Aliases: aliases})
	}
	return counts
}

func calibrationValueCounts(values map[string][]string) []TemporalCalibrationValueCount {
	keys := make([]string, 0, len(values))
	for value := range values {
		keys = append(keys, value)
	}
	sort.Strings(keys)
	counts := make([]TemporalCalibrationValueCount, 0, len(keys))
	for _, key := range keys {
		aliases := slices.Clone(values[key])
		sort.Strings(aliases)
		counts = append(counts, TemporalCalibrationValueCount{Value: key, Count: len(aliases), Aliases: aliases})
	}
	return counts
}
