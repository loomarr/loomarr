package fillerreview

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/loomarr/loomarr/internal/fillereval"
)

type TemporalModelReviewPackageConfig struct {
	EvidenceManifestPath   string
	EvidencePrivateMapPath string
	SelectionPath          string
	PanelSlot              string
	BatchID                string
	Seed                   string
	PreparedAt             time.Time
	OutputDir              string
	Materialization        TemporalHumanReviewMaterialization
}

type TemporalModelReviewPackageResult struct {
	Cases         int
	Files         int
	Bytes         int64
	PackageSHA256 string
	MapSHA256     string
}

// BuildTemporalModelReviewPackage is the sole preparation interface. It
// publishes a complete public package and private map atomically only after
// exact evidence joins, content bindings, and a full-byte leakage audit pass.
func BuildTemporalModelReviewPackage(config TemporalModelReviewPackageConfig) (TemporalModelReviewPackageResult, error) {
	if err := validateTemporalModelReviewPackageConfig(config); err != nil {
		return TemporalModelReviewPackageResult{}, err
	}
	evidence, evidenceSHA, err := LoadTemporalTruthEvidence(config.EvidenceManifestPath)
	if err != nil {
		return TemporalModelReviewPackageResult{}, err
	}
	privateMap, err := readStrictJSON[TemporalTruthEvidencePrivateMap](config.EvidencePrivateMapPath)
	if err != nil {
		return TemporalModelReviewPackageResult{}, fmt.Errorf("read temporal truth private map: %w", err)
	}
	if err := validateTemporalHumanEvidenceJoin(evidence, evidenceSHA, privateMap); err != nil {
		return TemporalModelReviewPackageResult{}, err
	}
	selectionRaw, err := os.ReadFile(config.SelectionPath)
	if err != nil {
		return TemporalModelReviewPackageResult{}, fmt.Errorf("read temporal truth selection: %w", err)
	}
	selection, err := fillereval.DecodeTemporalTruthSelection(selectionRaw)
	if err != nil {
		return TemporalModelReviewPackageResult{}, err
	}
	if hashBytes(selectionRaw) != evidence.SelectionSHA256 {
		return TemporalModelReviewPackageResult{}, fmt.Errorf("temporal truth selection does not bind the exact evidence set")
	}
	if err := validateTemporalHumanSelectionJoin(selection, privateMap); err != nil {
		return TemporalModelReviewPackageResult{}, err
	}

	stage, err := beginTemporalHumanReviewStage(config.OutputDir)
	if err != nil {
		return TemporalModelReviewPackageResult{}, err
	}
	defer stage.Cleanup()
	publicRoot := filepath.Join(stage.path, "public")
	privateRoot := filepath.Join(stage.path, "private")
	if err := os.MkdirAll(publicRoot, 0o750); err != nil {
		return TemporalModelReviewPackageResult{}, err
	}
	if err := os.MkdirAll(privateRoot, 0o700); err != nil {
		return TemporalModelReviewPackageResult{}, err
	}

	pack := TemporalModelReviewPackage{
		SchemaVersion: TemporalModelReviewSchemaVersion, ContractVersion: TemporalModelReviewContractVersion,
		QuestionVersion: TemporalHumanReviewQuestionVersion, EvidenceViewVersion: TemporalModelReviewEvidenceViewVersion,
		PanelSlot: config.PanelSlot, BatchID: config.BatchID, PreparedAt: config.PreparedAt.UTC(),
		EvidenceManifestSHA256: evidenceSHA, SelectionSHA256: evidence.SelectionSHA256,
		SeedSHA256: temporalTruthHash([]byte(config.Seed)),
	}
	mapping := TemporalModelReviewMap{
		SchemaVersion: TemporalModelReviewSchemaVersion, ContractVersion: TemporalModelReviewContractVersion,
		PanelSlot: config.PanelSlot, BatchID: config.BatchID, PreparedAt: config.PreparedAt.UTC(), Seed: config.Seed,
		EvidenceManifestSHA256: evidenceSHA, SelectionSHA256: evidence.SelectionSHA256,
	}
	ordered := append([]TemporalTruthEvidenceCase(nil), evidence.Cases...)
	sort.Slice(ordered, func(i, j int) bool {
		return temporalModelReviewRank(config.Seed, config.BatchID, config.PanelSlot, ordered[i].Alias) < temporalModelReviewRank(config.Seed, config.BatchID, config.PanelSlot, ordered[j].Alias)
	})
	evidenceRoot := filepath.Dir(config.EvidenceManifestPath)
	result := TemporalModelReviewPackageResult{Cases: len(ordered)}
	for _, sourceCase := range ordered {
		alias := temporalModelReviewAlias(config.Seed, config.BatchID, config.PanelSlot, sourceCase.Alias)
		publicCase, files, bytesWritten, err := materializeTemporalModelReviewCase(evidenceRoot, publicRoot, alias, sourceCase, config.Materialization)
		if err != nil {
			return TemporalModelReviewPackageResult{}, fmt.Errorf("materialize model alias %q: %w", alias, err)
		}
		pack.Cases = append(pack.Cases, publicCase)
		mapping.Entries = append(mapping.Entries, TemporalModelReviewMapEntry{Alias: alias, EvidenceAlias: sourceCase.Alias})
		result.Files += files
		result.Bytes += bytesWritten
	}

	packageRaw, err := json.MarshalIndent(pack, "", "  ")
	if err != nil {
		return TemporalModelReviewPackageResult{}, err
	}
	packageRaw = append(packageRaw, '\n')
	result.PackageSHA256 = hashBytes(packageRaw)
	if err := writeTemporalTruthNew(filepath.Join(publicRoot, "manifest.json"), packageRaw, 0o640); err != nil {
		return TemporalModelReviewPackageResult{}, err
	}
	mapping.PackageSHA256 = result.PackageSHA256
	mapRaw, err := json.MarshalIndent(mapping, "", "  ")
	if err != nil {
		return TemporalModelReviewPackageResult{}, err
	}
	mapRaw = append(mapRaw, '\n')
	result.MapSHA256 = hashBytes(mapRaw)
	if err := writeTemporalTruthNew(filepath.Join(privateRoot, "map.json"), mapRaw, 0o600); err != nil {
		return TemporalModelReviewPackageResult{}, err
	}
	if err := auditTemporalHumanReviewLeakage(publicRoot, privateMap, selection); err != nil {
		return TemporalModelReviewPackageResult{}, err
	}
	if err := stage.Publish(); err != nil {
		return TemporalModelReviewPackageResult{}, err
	}
	return result, nil
}

func validateTemporalModelReviewPackageConfig(config TemporalModelReviewPackageConfig) error {
	if strings.TrimSpace(config.EvidenceManifestPath) == "" || strings.TrimSpace(config.EvidencePrivateMapPath) == "" || strings.TrimSpace(config.SelectionPath) == "" || !validTemporalModelPanelSlot(config.PanelSlot) || strings.TrimSpace(config.BatchID) == "" || strings.TrimSpace(config.Seed) == "" || config.PreparedAt.IsZero() || strings.TrimSpace(config.OutputDir) == "" {
		return fmt.Errorf("temporal model review package requires evidence, private map, selection, panel slot, batch, seed, prepared time, and output")
	}
	if config.Materialization != TemporalHumanReviewHardlink && config.Materialization != TemporalHumanReviewCopy {
		return fmt.Errorf("temporal model review materialization must be hardlink or copy")
	}
	return nil
}

func materializeTemporalModelReviewCase(sourceRoot, publicRoot, alias string, source TemporalTruthEvidenceCase, mode TemporalHumanReviewMaterialization) (TemporalReviewCase, int, int64, error) {
	caseRoot := filepath.Join(publicRoot, "cases", alias)
	if err := os.MkdirAll(caseRoot, 0o750); err != nil {
		return TemporalReviewCase{}, 0, 0, err
	}
	result := TemporalReviewCase{Alias: alias, DurationMS: source.DurationMS}
	var total int64
	for index, sourceFrame := range source.Frames {
		name := fmt.Sprintf("frame-%02d.jpg", index+1)
		materialized, err := materializeTemporalHumanReviewFile(sourceRoot, publicRoot, caseRoot, TemporalTruthEvidenceFile{
			Path: sourceFrame.Path, SHA256: sourceFrame.SHA256, Bytes: sourceFrame.Bytes,
		}, name, mode)
		if err != nil {
			return TemporalReviewCase{}, 0, 0, err
		}
		ocr := temporalModelOCR(sourceFrame.OCR)
		result.Frames = append(result.Frames, TemporalReviewFrame{
			ID: sourceFrame.ID, Path: materialized.Path, SHA256: materialized.SHA256, Bytes: materialized.Bytes,
			Width: sourceFrame.Width, Height: sourceFrame.Height, AtMS: sourceFrame.AtMS,
			OCRSignalID: temporalModelSignalID(sourceFrame.ID, len(ocr) > 0), OCR: ocr,
		})
		total += materialized.Bytes
	}
	for index, segment := range source.TranscriptSegments {
		result.TranscriptSegments = append(result.TranscriptSegments, TemporalReviewTranscript{
			ID: fmt.Sprintf("transcript-%02d", index+1), StartMS: segment.StartMS, EndMS: segment.EndMS, Text: segment.Text,
		})
	}
	return result, len(result.Frames), total, nil
}
