package fillereval

import (
	"strings"
	"testing"
	"time"
)

func TestAnalyzeTemporalCalibrationReportsRelationsWithoutInventingTruth(t *testing.T) {
	cases := calibrationReportCases()
	first := calibrationReportSet("first", "family-a", cases, []calibrationFixtureClaim{
		{Unit: UnitStandalone, Role: TemporalRoleCommercial},
		{Unit: UnitStandalone, Role: TemporalRolePromo},
		{Unit: UnitStandalone, Role: TemporalRoleTrailer},
		{Unit: UnitCompilation},
	})
	second := calibrationReportSet("second", "family-b", cases, []calibrationFixtureClaim{
		{Unit: UnitCompilation},
		{Unit: UnitStandalone, Role: TemporalRoleCommercial},
		{Unit: UnitStandalone, Role: TemporalRoleTrailer},
		{Unit: UnitCompilation},
	})
	comparison, err := CompareTemporalAssessmentSets(first, second, cases)
	if err != nil {
		t.Fatal(err)
	}
	selection, err := BuildTemporalCalibrationSelection(comparison)
	if err != nil {
		t.Fatal(err)
	}
	selectedSignals, err := calibrationSignals(selection, cases)
	if err != nil {
		t.Fatal(err)
	}
	third := calibrationReportSet("third", "family-c", selectedSignals, []calibrationFixtureClaim{
		claimForAlias(selection.Cases[0].Alias, UnitStandalone, TemporalRoleCommercial),
		claimForAlias(selection.Cases[1].Alias, UnitStandalone, TemporalRoleCommercial),
		claimForAlias(selection.Cases[2].Alias, UnitStandalone, TemporalRoleTrailer),
		claimForAlias(selection.Cases[3].Alias, UnitStandalone, TemporalRoleBumper),
	})
	report, err := AnalyzeTemporalCalibration(selection, strings.Repeat("9", 64), first, second, third, cases)
	if err != nil {
		t.Fatal(err)
	}
	if report.Cases != 4 || report.AgreementControls != 2 || report.AgreementControlsPreserved != 1 || report.AgreementControlsBroken != 1 || report.OperationalFailures != 0 || len(report.CaseResults) != 4 {
		t.Fatalf("report summary = %+v", report)
	}
	if countCalibrationRelation(report.UnitRelations, TemporalCalibrationMatchedFirst) != 1 || countCalibrationRelation(report.RoleRelations, TemporalCalibrationMatchedSecond) != 1 || countCalibrationRelation(report.UnitRelations, TemporalCalibrationAgreementBroken) != 1 {
		t.Fatalf("relation counts = unit %+v role %+v", report.UnitRelations, report.RoleRelations)
	}
}

func TestAnalyzeTemporalCalibrationRejectsSelectionAndFamilyDrift(t *testing.T) {
	cases := calibrationReportCases()[:1]
	first := calibrationReportSet("first", "family-a", cases, []calibrationFixtureClaim{{Unit: UnitStandalone, Role: TemporalRoleCommercial}})
	second := calibrationReportSet("second", "family-b", cases, []calibrationFixtureClaim{{Unit: UnitCompilation}})
	comparison, err := CompareTemporalAssessmentSets(first, second, cases)
	if err != nil {
		t.Fatal(err)
	}
	selection, err := BuildTemporalCalibrationSelection(comparison)
	if err != nil {
		t.Fatal(err)
	}
	third := calibrationReportSet("third", "family-a", cases, []calibrationFixtureClaim{{Unit: UnitStandalone, Role: TemporalRoleCommercial}})
	if _, err := AnalyzeTemporalCalibration(selection, strings.Repeat("9", 64), first, second, third, cases); err == nil || !strings.Contains(err.Error(), "distinct") {
		t.Fatalf("family drift error = %v", err)
	}
	selection.Cases[0].Strata = []string{"unit:invented:invented"}
	third.Assessor.ModelFamily = "family-c"
	if _, err := AnalyzeTemporalCalibration(selection, strings.Repeat("9", 64), first, second, third, cases); err == nil || !strings.Contains(err.Error(), "does not match") {
		t.Fatalf("selection drift error = %v", err)
	}
}

type calibrationFixtureClaim struct {
	Alias string
	Unit  UnitKind
	Role  TemporalRole
}

func claimForAlias(alias string, unit UnitKind, role TemporalRole) calibrationFixtureClaim {
	return calibrationFixtureClaim{Alias: alias, Unit: unit, Role: role}
}

func calibrationReportCases() []TemporalCaseSignals {
	cases := make([]TemporalCaseSignals, 4)
	for index := range cases {
		cases[index] = TemporalCaseSignals{Alias: string(rune('a' + index)), DurationMS: 1_000, Signals: []TemporalSignal{{ID: "frame-01", Kind: "frame", AtMS: 100}}}
	}
	return cases
}

func calibrationReportSet(id, family string, cases []TemporalCaseSignals, claims []calibrationFixtureClaim) TemporalAssessmentSet {
	byAlias := make(map[string]calibrationFixtureClaim, len(claims))
	for index, claim := range claims {
		if claim.Alias == "" && index < len(cases) {
			claim.Alias = cases[index].Alias
		}
		byAlias[claim.Alias] = claim
	}
	set := TemporalAssessmentSet{
		SchemaVersion: TemporalAssessmentSchemaVersion, ContractVersion: TemporalAssessmentContractVersion,
		BatchID: "batch", PackageSHA256: strings.Repeat("1", 64),
		Assessor: TemporalAssessorIdentity{ID: id, Provider: "test", Model: id, ModelFamily: family, ModelDigest: strings.Repeat("a", 64), PromptVersion: "prompt-v1"},
	}
	for _, item := range cases {
		claim := byAlias[item.Alias]
		assessment := TemporalAssessment{Alias: item.Alias, Unit: &UnitAssessment{Kind: claim.Unit, DecisiveSignalIDs: []string{"frame-01"}, Reason: "bounded reason"}}
		axes := []string{"unit"}
		if claim.Unit == UnitStandalone {
			assessment.Role = &RoleAssessment{Kind: claim.Role, DecisiveSignalIDs: []string{"frame-01"}, Reason: "bounded reason"}
			axes = append(axes, "role")
		}
		assessment.Inference = temporalInference(axes...)
		assessment.Inference.AssessedAt = time.Unix(1, 0).UTC()
		set.Assessments = append(set.Assessments, assessment)
	}
	return set
}

func countCalibrationRelation(counts []TemporalCalibrationRelationCount, relation TemporalCalibrationRelation) int {
	for _, count := range counts {
		if count.Relation == relation {
			return count.Count
		}
	}
	return 0
}
