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

type TemporalHumanReviewMaterialization string

const (
	TemporalHumanReviewHardlink TemporalHumanReviewMaterialization = "hardlink"
	TemporalHumanReviewCopy     TemporalHumanReviewMaterialization = "copy"
)

// TemporalHumanReviewPackageConfig is the sole preparation interface. The
// output path must not exist and is published atomically only after the public
// package, viewer, private map, materialized files, and leakage audit agree.
type TemporalHumanReviewPackageConfig struct {
	EvidenceManifestPath   string
	EvidencePrivateMapPath string
	SelectionPath          string
	BatchID                string
	Seed                   string
	PreparedAt             time.Time
	OutputDir              string
	Materialization        TemporalHumanReviewMaterialization
}

type TemporalHumanReviewPackageResult struct {
	Cases         int
	Files         int
	Bytes         int64
	PackageSHA256 string
	ViewerSHA256  string
	MapSHA256     string
}

func BuildTemporalHumanReviewPackage(config TemporalHumanReviewPackageConfig) (TemporalHumanReviewPackageResult, error) {
	if err := validateTemporalHumanReviewPackageConfig(config); err != nil {
		return TemporalHumanReviewPackageResult{}, err
	}
	evidence, evidenceSHA, err := LoadTemporalTruthEvidence(config.EvidenceManifestPath)
	if err != nil {
		return TemporalHumanReviewPackageResult{}, err
	}
	privateMap, err := readStrictJSON[TemporalTruthEvidencePrivateMap](config.EvidencePrivateMapPath)
	if err != nil {
		return TemporalHumanReviewPackageResult{}, fmt.Errorf("read temporal truth private map: %w", err)
	}
	if err := validateTemporalHumanEvidenceJoin(evidence, evidenceSHA, privateMap); err != nil {
		return TemporalHumanReviewPackageResult{}, err
	}
	selectionRaw, err := os.ReadFile(config.SelectionPath)
	if err != nil {
		return TemporalHumanReviewPackageResult{}, fmt.Errorf("read temporal truth selection: %w", err)
	}
	selection, err := fillereval.DecodeTemporalTruthSelection(selectionRaw)
	if err != nil {
		return TemporalHumanReviewPackageResult{}, err
	}
	if hashBytes(selectionRaw) != evidence.SelectionSHA256 {
		return TemporalHumanReviewPackageResult{}, fmt.Errorf("temporal truth selection does not bind the exact evidence set")
	}
	if err := validateTemporalHumanSelectionJoin(selection, privateMap); err != nil {
		return TemporalHumanReviewPackageResult{}, err
	}

	stage, err := beginTemporalHumanReviewStage(config.OutputDir)
	if err != nil {
		return TemporalHumanReviewPackageResult{}, err
	}
	defer stage.Cleanup()
	publicRoot := filepath.Join(stage.path, "public")
	privateRoot := filepath.Join(stage.path, "private")
	if err := os.MkdirAll(publicRoot, 0o750); err != nil {
		return TemporalHumanReviewPackageResult{}, err
	}
	if err := os.MkdirAll(privateRoot, 0o700); err != nil {
		return TemporalHumanReviewPackageResult{}, err
	}

	seedSHA := temporalTruthHash([]byte(config.Seed))
	pack := TemporalHumanReviewPackage{
		SchemaVersion: TemporalHumanReviewSchemaVersion, ContractVersion: TemporalHumanReviewContractVersion,
		QuestionVersion: TemporalHumanReviewQuestionVersion, BatchID: config.BatchID, PreparedAt: config.PreparedAt.UTC(),
		EvidenceManifestSHA256: evidenceSHA, SelectionSHA256: evidence.SelectionSHA256, SeedSHA256: seedSHA,
	}
	mapping := TemporalHumanReviewMap{
		SchemaVersion: TemporalHumanReviewSchemaVersion, ContractVersion: TemporalHumanReviewContractVersion,
		BatchID: config.BatchID, PreparedAt: config.PreparedAt.UTC(), Seed: config.Seed,
		EvidenceManifestSHA256: evidenceSHA, SelectionSHA256: evidence.SelectionSHA256,
	}
	ordered := append([]TemporalTruthEvidenceCase(nil), evidence.Cases...)
	sort.Slice(ordered, func(i, j int) bool {
		return temporalHumanReviewRank(config.Seed, config.BatchID, ordered[i].Alias) < temporalHumanReviewRank(config.Seed, config.BatchID, ordered[j].Alias)
	})
	evidenceRoot := filepath.Dir(config.EvidenceManifestPath)
	result := TemporalHumanReviewPackageResult{Cases: len(ordered)}
	for _, sourceCase := range ordered {
		alias := temporalHumanReviewAlias(config.Seed, config.BatchID, sourceCase.Alias)
		publicCase, files, bytesWritten, err := materializeTemporalHumanReviewCase(evidenceRoot, publicRoot, alias, sourceCase, config.Materialization)
		if err != nil {
			return TemporalHumanReviewPackageResult{}, fmt.Errorf("materialize review alias %q: %w", alias, err)
		}
		pack.Cases = append(pack.Cases, publicCase)
		mapping.Entries = append(mapping.Entries, TemporalHumanReviewMapEntry{Alias: alias, EvidenceAlias: sourceCase.Alias})
		result.Files += files
		result.Bytes += bytesWritten
	}

	packageRaw, err := json.MarshalIndent(pack, "", "  ")
	if err != nil {
		return TemporalHumanReviewPackageResult{}, err
	}
	packageRaw = append(packageRaw, '\n')
	result.PackageSHA256 = hashBytes(packageRaw)
	if err := writeTemporalTruthNew(filepath.Join(publicRoot, "manifest.json"), packageRaw, 0o640); err != nil {
		return TemporalHumanReviewPackageResult{}, err
	}
	viewerRaw, err := renderTemporalHumanReviewer(pack, result.PackageSHA256)
	if err != nil {
		return TemporalHumanReviewPackageResult{}, err
	}
	result.ViewerSHA256 = hashBytes(viewerRaw)
	if err := writeTemporalTruthNew(filepath.Join(publicRoot, "index.html"), viewerRaw, 0o640); err != nil {
		return TemporalHumanReviewPackageResult{}, err
	}
	mapping.PackageSHA256 = result.PackageSHA256
	mapping.ViewerSHA256 = result.ViewerSHA256
	mapRaw, err := json.MarshalIndent(mapping, "", "  ")
	if err != nil {
		return TemporalHumanReviewPackageResult{}, err
	}
	mapRaw = append(mapRaw, '\n')
	result.MapSHA256 = hashBytes(mapRaw)
	if err := writeTemporalTruthNew(filepath.Join(privateRoot, "map.json"), mapRaw, 0o600); err != nil {
		return TemporalHumanReviewPackageResult{}, err
	}
	if err := auditTemporalHumanReviewLeakage(publicRoot, privateMap, selection); err != nil {
		return TemporalHumanReviewPackageResult{}, err
	}
	if err := stage.Publish(); err != nil {
		return TemporalHumanReviewPackageResult{}, err
	}
	return result, nil
}

func validateTemporalHumanReviewPackageConfig(config TemporalHumanReviewPackageConfig) error {
	if strings.TrimSpace(config.EvidenceManifestPath) == "" || strings.TrimSpace(config.EvidencePrivateMapPath) == "" || strings.TrimSpace(config.SelectionPath) == "" || strings.TrimSpace(config.BatchID) == "" || strings.TrimSpace(config.Seed) == "" || config.PreparedAt.IsZero() || strings.TrimSpace(config.OutputDir) == "" {
		return fmt.Errorf("temporal human review package requires evidence, private map, selection, batch, seed, prepared time, and output")
	}
	if config.Materialization != TemporalHumanReviewHardlink && config.Materialization != TemporalHumanReviewCopy {
		return fmt.Errorf("temporal human review materialization must be hardlink or copy")
	}
	return nil
}

func validateTemporalHumanEvidenceJoin(evidence TemporalTruthEvidenceManifest, evidenceSHA string, privateMap TemporalTruthEvidencePrivateMap) error {
	if privateMap.SchemaVersion != TemporalTruthEvidenceSchemaVersion || privateMap.ContractVersion != TemporalTruthEvidenceContractVersion || privateMap.SelectionSHA256 != evidence.SelectionSHA256 || len(privateMap.Entries) != len(evidence.Cases) || !reviewSHA256(evidenceSHA) {
		return fmt.Errorf("temporal truth private map does not bind the exact evidence set")
	}
	publicAliases := make(map[string]struct{}, len(evidence.Cases))
	for _, item := range evidence.Cases {
		publicAliases[item.Alias] = struct{}{}
	}
	seen := make(map[string]struct{}, len(privateMap.Entries))
	for _, entry := range privateMap.Entries {
		if _, exists := publicAliases[entry.Alias]; !exists || strings.TrimSpace(entry.CaseID) == "" || !reviewSHA256(entry.ContentSHA256) || !reviewSHA256(entry.SourceSHA256) || !reviewSHA256(entry.PacketSHA256) {
			return fmt.Errorf("temporal truth private map contains an invalid evidence join")
		}
		if _, duplicate := seen[entry.Alias]; duplicate {
			return fmt.Errorf("temporal truth private map repeats evidence alias %q", entry.Alias)
		}
		seen[entry.Alias] = struct{}{}
	}
	return nil
}

func validateTemporalHumanSelectionJoin(selection fillereval.TemporalTruthSelection, privateMap TemporalTruthEvidencePrivateMap) error {
	selected := make(map[string]string, len(selection.Cases))
	for _, item := range selection.Cases {
		selected[item.CaseID] = item.ContentSHA256
	}
	for _, entry := range privateMap.Entries {
		if selected[entry.CaseID] != entry.ContentSHA256 {
			return fmt.Errorf("temporal truth selection and private evidence map do not contain the same cases")
		}
		delete(selected, entry.CaseID)
	}
	if len(selected) != 0 {
		return fmt.Errorf("temporal truth selection and private evidence map do not contain the same cases")
	}
	return nil
}

func temporalHumanReviewAlias(seed, batchID, evidenceAlias string) string {
	return "review-" + temporalTruthHash([]byte(seed + "\x00alias\x00" + batchID + "\x00" + evidenceAlias))[:32]
}

func temporalHumanReviewRank(seed, batchID, evidenceAlias string) string {
	return temporalTruthHash([]byte(seed + "\x00order\x00" + batchID + "\x00" + evidenceAlias))
}

func materializeTemporalHumanReviewCase(sourceRoot, publicRoot, alias string, source TemporalTruthEvidenceCase, mode TemporalHumanReviewMaterialization) (TemporalHumanReviewCase, int, int64, error) {
	caseRoot := filepath.Join(publicRoot, "cases", alias)
	if err := os.MkdirAll(caseRoot, 0o750); err != nil {
		return TemporalHumanReviewCase{}, 0, 0, err
	}
	result := TemporalHumanReviewCase{Alias: alias, DurationMS: source.DurationMS}
	video, err := materializeTemporalHumanReviewFile(sourceRoot, publicRoot, caseRoot, source.Video, "review.mp4", mode)
	if err != nil {
		return TemporalHumanReviewCase{}, 0, 0, err
	}
	result.Video = video
	files := 1
	bytesWritten := video.Bytes
	for index, sourceFrame := range source.Frames {
		name := fmt.Sprintf("frame-%02d.jpg", index+1)
		materialized, err := materializeTemporalHumanReviewFile(sourceRoot, publicRoot, caseRoot, TemporalTruthEvidenceFile{
			Path: sourceFrame.Path, SHA256: sourceFrame.SHA256, Bytes: sourceFrame.Bytes,
		}, name, mode)
		if err != nil {
			return TemporalHumanReviewCase{}, 0, 0, err
		}
		result.Frames = append(result.Frames, TemporalTruthEvidenceFrame{
			ID: sourceFrame.ID, Path: materialized.Path, SHA256: materialized.SHA256, Bytes: materialized.Bytes,
			Width: sourceFrame.Width, Height: sourceFrame.Height, AtMS: sourceFrame.AtMS, OCR: sourceFrame.OCR,
		})
		files++
		bytesWritten += materialized.Bytes
	}
	for index, segment := range source.TranscriptSegments {
		result.TranscriptSegments = append(result.TranscriptSegments, TemporalReviewTranscript{
			ID: fmt.Sprintf("transcript-%02d", index+1), StartMS: segment.StartMS, EndMS: segment.EndMS, Text: segment.Text,
		})
	}
	return result, files, bytesWritten, nil
}

func materializeTemporalHumanReviewFile(sourceRoot, publicRoot, caseRoot string, source TemporalTruthEvidenceFile, name string, mode TemporalHumanReviewMaterialization) (TemporalTruthEvidenceFile, error) {
	sourcePath, err := resolveWithin(sourceRoot, source.Path)
	if err != nil {
		return TemporalTruthEvidenceFile{}, err
	}
	target := filepath.Join(caseRoot, name)
	if mode == TemporalHumanReviewHardlink {
		if err := os.Link(sourcePath, target); err != nil {
			return TemporalTruthEvidenceFile{}, fmt.Errorf("hard-link %q (use copy mode across filesystems): %w", source.Path, err)
		}
	} else if err := copyFile(sourcePath, target); err != nil {
		return TemporalTruthEvidenceFile{}, err
	}
	info, err := os.Stat(target)
	if err != nil || !info.Mode().IsRegular() || info.Size() != source.Bytes {
		return TemporalTruthEvidenceFile{}, fmt.Errorf("materialized file %q fails byte binding", name)
	}
	digest, err := hashFile(target)
	if err != nil || digest != source.SHA256 {
		return TemporalTruthEvidenceFile{}, fmt.Errorf("materialized file %q fails content binding", name)
	}
	result := source
	result.Path = temporalTruthRelative(publicRoot, target)
	return result, nil
}

func renderTemporalHumanReviewer(pack TemporalHumanReviewPackage, packageSHA string) ([]byte, error) {
	data, err := json.Marshal(pack)
	if err != nil {
		return nil, err
	}
	replacer := strings.NewReplacer(
		"__LOOMARR_TEMPORAL_REVIEW_PACKAGE_JSON__", string(data),
		"__LOOMARR_TEMPORAL_REVIEW_PACKAGE_SHA256__", packageSHA,
	)
	rendered := replacer.Replace(temporalHumanReviewerTemplate)
	if strings.Contains(rendered, "__LOOMARR_TEMPORAL_REVIEW_") {
		return nil, fmt.Errorf("temporal reviewer template contains an unresolved marker")
	}
	return []byte(rendered), nil
}

type temporalHumanReviewStage struct {
	path      string
	output    string
	published bool
}

func beginTemporalHumanReviewStage(output string) (*temporalHumanReviewStage, error) {
	resolved, err := filepath.Abs(output)
	if err != nil {
		return nil, err
	}
	if _, err := os.Lstat(resolved); err == nil || !os.IsNotExist(err) {
		return nil, fmt.Errorf("output already exists: %s", resolved)
	}
	if err := os.MkdirAll(filepath.Dir(resolved), 0o750); err != nil {
		return nil, err
	}
	path, err := os.MkdirTemp(filepath.Dir(resolved), ".filler-temporal-human-review-*")
	if err != nil {
		return nil, err
	}
	return &temporalHumanReviewStage{path: path, output: resolved}, nil
}

func (stage *temporalHumanReviewStage) Cleanup() {
	if !stage.published {
		_ = os.RemoveAll(stage.path)
	}
}

func (stage *temporalHumanReviewStage) Publish() error {
	if err := os.Rename(stage.path, stage.output); err != nil {
		return err
	}
	stage.published = true
	return nil
}
