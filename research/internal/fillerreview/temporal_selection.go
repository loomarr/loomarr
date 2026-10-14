package fillerreview

import (
	"fmt"
	"os"

	"github.com/loomarr/loomarr/internal/fillereval"
)

// TemporalCalibrationPackage is a fully verified temporal package narrowed by
// an immutable, identity-bound calibration selection. Paths remain relative to
// the original package root; no media is copied or relabeled.
type TemporalCalibrationPackage struct {
	Package         TemporalReviewPackage
	Cases           []TemporalReviewCase
	Signals         []fillereval.TemporalCaseSignals
	PackageSHA256   string
	Selection       fillereval.TemporalCalibrationSelection
	SelectionSHA256 string
}

// temporalInferencePackage is the verified seam consumed by inference. The
// legacy calibration selection and a fresh model-panel package are two real
// adapters: both must reduce to the same closed case and signal contract
// before any paid transport or checkpoint state exists.
type temporalInferencePackage struct {
	BatchID          string
	Cases            []TemporalReviewCase
	Signals          []fillereval.TemporalCaseSignals
	PackageSHA256    string
	SelectionSHA256  string
	PackageCaseCount int
}

func LoadTemporalCalibrationPackage(packagePath, selectionPath string, expectedPackageCases, expectedCalibrationCases int) (TemporalCalibrationPackage, error) {
	pack, allSignals, packageSHA256, err := LoadTemporalReviewPackage(packagePath, expectedPackageCases)
	if err != nil {
		return TemporalCalibrationPackage{}, err
	}
	selectionRaw, err := os.ReadFile(selectionPath)
	if err != nil {
		return TemporalCalibrationPackage{}, fmt.Errorf("read temporal calibration selection: %w", err)
	}
	selection, err := fillereval.DecodeTemporalCalibrationSelection(selectionRaw)
	if err != nil {
		return TemporalCalibrationPackage{}, err
	}
	if err := fillereval.ValidateTemporalCalibrationSelection(selection, pack.BatchID, packageSHA256, expectedCalibrationCases); err != nil {
		return TemporalCalibrationPackage{}, err
	}
	caseByAlias := make(map[string]TemporalReviewCase, len(pack.Cases))
	signalsByAlias := make(map[string]fillereval.TemporalCaseSignals, len(allSignals))
	for index, item := range pack.Cases {
		caseByAlias[item.Alias] = item
		signalsByAlias[item.Alias] = allSignals[index]
	}
	selectedCases := make([]TemporalReviewCase, 0, len(selection.Cases))
	selectedSignals := make([]fillereval.TemporalCaseSignals, 0, len(selection.Cases))
	for _, selected := range selection.Cases {
		item, exists := caseByAlias[selected.Alias]
		if !exists {
			return TemporalCalibrationPackage{}, fmt.Errorf("temporal calibration selection names unknown alias %q", selected.Alias)
		}
		selectedCases = append(selectedCases, item)
		selectedSignals = append(selectedSignals, signalsByAlias[selected.Alias])
	}
	return TemporalCalibrationPackage{
		Package: pack, Cases: selectedCases, Signals: selectedSignals, PackageSHA256: packageSHA256,
		Selection: selection, SelectionSHA256: hashBytes(selectionRaw),
	}, nil
}

func (loaded TemporalCalibrationPackage) inferencePackage() temporalInferencePackage {
	return temporalInferencePackage{
		BatchID: loaded.Package.BatchID, Cases: loaded.Cases, Signals: loaded.Signals,
		PackageSHA256: loaded.PackageSHA256, SelectionSHA256: loaded.SelectionSHA256,
		PackageCaseCount: len(loaded.Package.Cases),
	}
}

func loadTemporalModelInferencePackage(packagePath string, expectedCases int) (temporalInferencePackage, error) {
	pack, signals, packageSHA, err := LoadTemporalModelReviewPackage(packagePath, expectedCases)
	if err != nil {
		return temporalInferencePackage{}, err
	}
	return temporalInferencePackage{
		BatchID: pack.BatchID, Cases: pack.Cases, Signals: signals, PackageSHA256: packageSHA,
		SelectionSHA256: pack.SelectionSHA256, PackageCaseCount: len(pack.Cases),
	}, nil
}
