package fillereval

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"slices"
	"sort"
	"strings"
)

const (
	TemporalCalibrationSelectionSchemaVersion   = 1
	TemporalCalibrationSelectionContractVersion = "filler-temporal-calibration-selection-v1"
	TemporalCalibrationMaxCases                 = 15
)

// TemporalCalibrationSelection is the immutable bridge from a complete
// two-model diagnostic to one bounded stronger-model calibration. It contains
// no labels or source identity; its cases are only opaque aliases and strata.
type TemporalCalibrationSelection struct {
	SchemaVersion          int                       `json:"schemaVersion"`
	ContractVersion        string                    `json:"contractVersion"`
	BatchID                string                    `json:"batchId"`
	PackageSHA256          string                    `json:"packageSha256"`
	FirstAssessmentSHA256  string                    `json:"firstAssessmentSha256"`
	SecondAssessmentSHA256 string                    `json:"secondAssessmentSha256"`
	ComparisonSHA256       string                    `json:"comparisonSha256"`
	Cases                  []TemporalCalibrationCase `json:"cases"`
}

// BuildTemporalCalibrationSelection projects only the stratified candidate
// list from a deterministic comparison. The comparison itself remains the
// evidence for why each opaque alias was selected.
func BuildTemporalCalibrationSelection(report TemporalComparisonReport) (TemporalCalibrationSelection, error) {
	if report.SchemaVersion != TemporalAssessmentSchemaVersion || report.ContractVersion != TemporalAssessmentContractVersion || strings.TrimSpace(report.BatchID) == "" || !validSHA256(report.PackageSHA256) || !validSHA256(report.FirstAssessmentSHA256) || !validSHA256(report.SecondAssessmentSHA256) || report.Cases <= 0 || len(report.CalibrationCandidates) == 0 {
		return TemporalCalibrationSelection{}, fmt.Errorf("temporal comparison identity or calibration candidates are invalid")
	}
	selection := TemporalCalibrationSelection{
		SchemaVersion:          TemporalCalibrationSelectionSchemaVersion,
		ContractVersion:        TemporalCalibrationSelectionContractVersion,
		BatchID:                report.BatchID,
		PackageSHA256:          report.PackageSHA256,
		FirstAssessmentSHA256:  report.FirstAssessmentSHA256,
		SecondAssessmentSHA256: report.SecondAssessmentSHA256,
		ComparisonSHA256:       TemporalComparisonReportSHA256(report),
		Cases:                  slices.Clone(report.CalibrationCandidates),
	}
	for index := range selection.Cases {
		selection.Cases[index].Reasons = sortedUnique(selection.Cases[index].Reasons)
		selection.Cases[index].Strata = sortedUnique(selection.Cases[index].Strata)
	}
	sort.Slice(selection.Cases, func(i, j int) bool { return selection.Cases[i].Alias < selection.Cases[j].Alias })
	if err := ValidateTemporalCalibrationSelection(selection, report.BatchID, report.PackageSHA256, len(report.CalibrationCandidates)); err != nil {
		return TemporalCalibrationSelection{}, err
	}
	return selection, nil
}

func DecodeTemporalCalibrationSelection(data []byte) (TemporalCalibrationSelection, error) {
	var selection TemporalCalibrationSelection
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&selection); err != nil {
		return TemporalCalibrationSelection{}, fmt.Errorf("decode temporal calibration selection: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return TemporalCalibrationSelection{}, fmt.Errorf("decode temporal calibration selection: trailing JSON value")
		}
		return TemporalCalibrationSelection{}, fmt.Errorf("decode temporal calibration selection trailing value: %w", err)
	}
	return selection, nil
}

func ValidateTemporalCalibrationSelection(selection TemporalCalibrationSelection, batchID, packageSHA256 string, expectedCases int) error {
	if selection.SchemaVersion != TemporalCalibrationSelectionSchemaVersion || selection.ContractVersion != TemporalCalibrationSelectionContractVersion {
		return fmt.Errorf("temporal calibration selection schema or contract version is invalid")
	}
	if strings.TrimSpace(batchID) == "" || selection.BatchID != batchID || !validSHA256(packageSHA256) || selection.PackageSHA256 != packageSHA256 {
		return fmt.Errorf("temporal calibration selection package identity is invalid")
	}
	if !validSHA256(selection.FirstAssessmentSHA256) || !validSHA256(selection.SecondAssessmentSHA256) || !validSHA256(selection.ComparisonSHA256) || selection.FirstAssessmentSHA256 == selection.SecondAssessmentSHA256 {
		return fmt.Errorf("temporal calibration selection comparison identity is invalid")
	}
	if expectedCases <= 0 || expectedCases > TemporalCalibrationMaxCases || len(selection.Cases) != expectedCases {
		return fmt.Errorf("temporal calibration selection has %d cases; want exactly %d", len(selection.Cases), expectedCases)
	}
	seen := make(map[string]struct{}, len(selection.Cases))
	previous := ""
	for index, item := range selection.Cases {
		if strings.TrimSpace(item.Alias) == "" || len(item.Reasons) == 0 || len(item.Strata) == 0 || !strictSortedUnique(item.Reasons) || !strictSortedUnique(item.Strata) || !validTemporalCalibrationReasons(item.Reasons) || !validTemporalCalibrationStrata(item.Strata) || index > 0 && item.Alias <= previous {
			return fmt.Errorf("temporal calibration selection case %d is invalid or not canonically ordered", index)
		}
		if _, duplicate := seen[item.Alias]; duplicate {
			return fmt.Errorf("temporal calibration selection repeats alias %q", item.Alias)
		}
		seen[item.Alias] = struct{}{}
		previous = item.Alias
	}
	return nil
}

func validTemporalCalibrationReasons(reasons []string) bool {
	for _, reason := range reasons {
		if reason != "unit_disagreement" && reason != "role_disagreement" && reason != "agreement_control" {
			return false
		}
	}
	return true
}

func validTemporalCalibrationStrata(strata []string) bool {
	for _, stratum := range strata {
		if !strings.HasPrefix(stratum, "unit:") && !strings.HasPrefix(stratum, "role:") && !strings.HasPrefix(stratum, "agreement:") {
			return false
		}
	}
	return true
}

func TemporalComparisonReportSHA256(report TemporalComparisonReport) string {
	raw, err := json.Marshal(report)
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

func strictSortedUnique(values []string) bool {
	for index, value := range values {
		if strings.TrimSpace(value) == "" || index > 0 && value <= values[index-1] {
			return false
		}
	}
	return true
}
