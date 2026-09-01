package fillerreview

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/loomarr/loomarr/internal/fillereval"
)

func TestLoadTemporalCalibrationPackageBindsSelectionToPackage(t *testing.T) {
	packagePath := writeTemporalTestPackage(t)
	pack, _, packageSHA256, err := LoadTemporalReviewPackage(packagePath, 1)
	if err != nil {
		t.Fatal(err)
	}
	selection := fillereval.TemporalCalibrationSelection{
		SchemaVersion: fillereval.TemporalCalibrationSelectionSchemaVersion, ContractVersion: fillereval.TemporalCalibrationSelectionContractVersion,
		BatchID: pack.BatchID, PackageSHA256: packageSHA256, FirstAssessmentSHA256: strings.Repeat("1", 64), SecondAssessmentSHA256: strings.Repeat("2", 64), ComparisonSHA256: strings.Repeat("3", 64),
		Cases: []fillereval.TemporalCalibrationCase{{Alias: "opaque", Reasons: []string{"agreement_control"}, Strata: []string{"agreement:standalone:commercial"}}},
	}
	raw, err := json.Marshal(selection)
	if err != nil {
		t.Fatal(err)
	}
	selectionPath := filepath.Join(t.TempDir(), "selection.json")
	if err := os.WriteFile(selectionPath, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	selected, err := LoadTemporalCalibrationPackage(packagePath, selectionPath, 1, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(selected.Cases) != 1 || selected.Cases[0].Alias != "opaque" || len(selected.Signals) != 1 || !reviewSHA256(selected.SelectionSHA256) {
		t.Fatalf("selected package = %+v", selected)
	}

	selection.PackageSHA256 = strings.Repeat("f", 64)
	raw, _ = json.Marshal(selection)
	if err := os.WriteFile(selectionPath, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadTemporalCalibrationPackage(packagePath, selectionPath, 1, 1); err == nil || !strings.Contains(err.Error(), "package identity") {
		t.Fatalf("package drift error = %v", err)
	}
}
