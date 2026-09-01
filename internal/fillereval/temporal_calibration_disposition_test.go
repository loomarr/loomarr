package fillereval

import (
	"reflect"
	"testing"
)

func TestTemporalCalibrationDispositionNeverAuthorizesFullCorpusRelabel(t *testing.T) {
	tests := []struct {
		name       string
		mutate     func(*TemporalCalibrationReport)
		nextAction TemporalCalibrationNextAction
		reasons    []string
	}{
		{
			name:       "clean axis-specific support repeats diagnostic",
			nextAction: TemporalCalibrationRepeatDiagnostic,
			reasons:    []string{"clean_controls_and_unanimous_axis_support"},
		},
		{
			name: "underspecified selection is repaired",
			mutate: func(report *TemporalCalibrationReport) {
				report.AgreementControls = 2
				report.AgreementControlsPreserved = 2
			},
			nextAction: TemporalCalibrationRepairSelection,
			reasons:    []string{"fewer_than_three_agreement_controls"},
		},
		{
			name: "operational failure repeats bounded calibration",
			mutate: func(report *TemporalCalibrationReport) {
				report.OperationalFailures = 1
				report.CaseResults[3].UnitRelation = TemporalCalibrationOperationalFailure
			},
			nextAction: TemporalCalibrationRepeatHosted,
			reasons:    []string{"hosted_operational_failure"},
		},
		{
			name: "broken control repairs evidence",
			mutate: func(report *TemporalCalibrationReport) {
				report.AgreementControlsPreserved = 2
				report.AgreementControlsBroken = 1
				report.CaseResults[0].UnitRelation = TemporalCalibrationAgreementBroken
			},
			nextAction: TemporalCalibrationRepairEvidence,
			reasons:    []string{"agreement_control_broken"},
		},
		{
			name: "semantic abstention repairs evidence",
			mutate: func(report *TemporalCalibrationReport) {
				report.ThirdUnitUnclear = 1
			},
			nextAction: TemporalCalibrationRepairEvidence,
			reasons:    []string{"hosted_semantic_abstention"},
		},
		{
			name: "novel claim repairs evidence",
			mutate: func(report *TemporalCalibrationReport) {
				report.CaseResults[3].UnitRelation = TemporalCalibrationMatchedNeither
			},
			nextAction: TemporalCalibrationRepairEvidence,
			reasons:    []string{"unit_novel_or_not_comparable"},
		},
		{
			name: "mixed support repairs evidence",
			mutate: func(report *TemporalCalibrationReport) {
				report.CaseResults = append(report.CaseResults, TemporalCalibrationCaseResult{
					Alias: "unit-second", Reasons: []string{"unit_disagreement"}, UnitRelation: TemporalCalibrationMatchedSecond,
				})
			},
			nextAction: TemporalCalibrationRepairEvidence,
			reasons:    []string{"unit_mixed_local_support"},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			report := cleanTemporalCalibrationDispositionReport()
			if test.mutate != nil {
				test.mutate(&report)
			}
			got := temporalCalibrationDisposition(report)
			if got.NextAction != test.nextAction || got.FullCorpusRelabelAllowed || !reflect.DeepEqual(got.Reasons, test.reasons) {
				t.Fatalf("disposition = %+v", got)
			}
		})
	}
}

func TestTemporalCalibrationDispositionReportsAxisSupportSeparately(t *testing.T) {
	disposition := temporalCalibrationDisposition(cleanTemporalCalibrationDispositionReport())
	if len(disposition.Axes) != 2 {
		t.Fatalf("axes = %+v", disposition.Axes)
	}
	unit, role := disposition.Axes[0], disposition.Axes[1]
	if unit.Axis != "unit" || unit.Disputes != 1 || unit.MatchedFirst != 1 || unit.Outcome != TemporalCalibrationAxisMatchedFirst {
		t.Fatalf("unit = %+v", unit)
	}
	if role.Axis != "role" || role.Disputes != 1 || role.MatchedSecond != 1 || role.Outcome != TemporalCalibrationAxisMatchedSecond {
		t.Fatalf("role = %+v", role)
	}
}

func cleanTemporalCalibrationDispositionReport() TemporalCalibrationReport {
	return TemporalCalibrationReport{
		AgreementControls: 3, AgreementControlsPreserved: 3,
		CaseResults: []TemporalCalibrationCaseResult{
			{Alias: "control-one", Reasons: []string{"agreement_control"}, UnitRelation: TemporalCalibrationAgreementPreserved, RoleRelation: TemporalCalibrationAgreementPreserved},
			{Alias: "control-two", Reasons: []string{"agreement_control"}, UnitRelation: TemporalCalibrationAgreementPreserved, RoleRelation: TemporalCalibrationAgreementPreserved},
			{Alias: "control-three", Reasons: []string{"agreement_control"}, UnitRelation: TemporalCalibrationAgreementPreserved, RoleRelation: TemporalCalibrationAgreementPreserved},
			{Alias: "unit-first", Reasons: []string{"unit_disagreement"}, UnitRelation: TemporalCalibrationMatchedFirst},
			{Alias: "role-second", Reasons: []string{"role_disagreement"}, RoleRelation: TemporalCalibrationMatchedSecond},
		},
	}
}
