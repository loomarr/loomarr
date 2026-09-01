package fillereval

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestBuildTemporalCalibrationSelectionIsDeterministicAndStrict(t *testing.T) {
	report := TemporalComparisonReport{
		SchemaVersion: TemporalAssessmentSchemaVersion, ContractVersion: TemporalAssessmentContractVersion,
		BatchID: "temporal-batch", PackageSHA256: strings.Repeat("1", 64), FirstAssessmentSHA256: strings.Repeat("2", 64), SecondAssessmentSHA256: strings.Repeat("3", 64), Cases: 32,
		CalibrationCandidates: []TemporalCalibrationCase{
			{Alias: "opaque-b", Reasons: []string{"unit_disagreement"}, Strata: []string{"unit:standalone:compilation"}},
			{Alias: "opaque-a", Reasons: []string{"agreement_control", "agreement_control"}, Strata: []string{"agreement:standalone:commercial"}},
		},
	}
	first, err := BuildTemporalCalibrationSelection(report)
	if err != nil {
		t.Fatal(err)
	}
	second, err := BuildTemporalCalibrationSelection(report)
	if err != nil {
		t.Fatal(err)
	}
	firstRaw, _ := json.Marshal(first)
	secondRaw, _ := json.Marshal(second)
	if string(firstRaw) != string(secondRaw) || first.Cases[0].Alias != "opaque-a" || len(first.Cases[0].Reasons) != 1 || len(first.ComparisonSHA256) != 64 {
		t.Fatalf("selection is not canonical: %+v", first)
	}
	decoded, err := DecodeTemporalCalibrationSelection(firstRaw)
	if err != nil {
		t.Fatal(err)
	}
	if err := ValidateTemporalCalibrationSelection(decoded, report.BatchID, report.PackageSHA256, 2); err != nil {
		t.Fatal(err)
	}
}

func TestDecodeTemporalCalibrationSelectionRejectsUnknownAndDriftedFields(t *testing.T) {
	raw := []byte(`{"schemaVersion":1,"contractVersion":"filler-temporal-calibration-selection-v1","batchId":"b","packageSha256":"` + strings.Repeat("1", 64) + `","firstAssessmentSha256":"` + strings.Repeat("2", 64) + `","secondAssessmentSha256":"` + strings.Repeat("3", 64) + `","comparisonSha256":"` + strings.Repeat("4", 64) + `","cases":[],"extra":true}`)
	if _, err := DecodeTemporalCalibrationSelection(raw); err == nil {
		t.Fatal("unknown field was accepted")
	}
	selection := TemporalCalibrationSelection{
		SchemaVersion: TemporalCalibrationSelectionSchemaVersion, ContractVersion: TemporalCalibrationSelectionContractVersion,
		BatchID: "b", PackageSHA256: strings.Repeat("1", 64), FirstAssessmentSHA256: strings.Repeat("2", 64), SecondAssessmentSHA256: strings.Repeat("3", 64), ComparisonSHA256: strings.Repeat("4", 64),
		Cases: []TemporalCalibrationCase{{Alias: "opaque-a", Reasons: []string{"z", "a"}, Strata: []string{"unit:a:b"}}},
	}
	if err := ValidateTemporalCalibrationSelection(selection, "b", selection.PackageSHA256, 1); err == nil {
		t.Fatal("non-canonical reason order was accepted")
	}
	selection.Cases[0].Reasons = []string{"invented_reason"}
	selection.Cases[0].Strata = []string{"invented:stratum"}
	if err := ValidateTemporalCalibrationSelection(selection, "b", selection.PackageSHA256, 1); err == nil {
		t.Fatal("open-ended selection vocabulary was accepted")
	}
	if err := ValidateTemporalCalibrationSelection(selection, "b", selection.PackageSHA256, TemporalCalibrationMaxCases+1); err == nil {
		t.Fatal("selection case ceiling was not enforced")
	}
}
