package fillerreview

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/loomarr/loomarr/internal/fillereval"
)

// LoadTemporalModelReviewPackage strictly decodes and content-verifies the
// public package, then exposes only the evidence namespace models may cite.
func LoadTemporalModelReviewPackage(path string, expectedCases int) (TemporalModelReviewPackage, []fillereval.TemporalCaseSignals, string, error) {
	if strings.TrimSpace(path) == "" || expectedCases <= 0 {
		return TemporalModelReviewPackage{}, nil, "", fmt.Errorf("model package path and positive expected case count are required")
	}
	pack, err := readStrictJSON[TemporalModelReviewPackage](path)
	if err != nil {
		return TemporalModelReviewPackage{}, nil, "", fmt.Errorf("read temporal model review package: %w", err)
	}
	digest, err := hashFile(path)
	if err != nil {
		return TemporalModelReviewPackage{}, nil, "", err
	}
	if err := validateTemporalModelReviewPackage(filepath.Dir(path), pack, expectedCases); err != nil {
		return TemporalModelReviewPackage{}, nil, "", err
	}
	signals := make([]fillereval.TemporalCaseSignals, 0, len(pack.Cases))
	for _, item := range pack.Cases {
		caseSignals := fillereval.TemporalCaseSignals{Alias: item.Alias, DurationMS: item.DurationMS}
		for _, frame := range item.Frames {
			caseSignals.Signals = append(caseSignals.Signals, fillereval.TemporalSignal{ID: frame.ID, Kind: "frame", AtMS: frame.AtMS})
			if frame.OCRSignalID != "" {
				caseSignals.Signals = append(caseSignals.Signals, fillereval.TemporalSignal{ID: frame.OCRSignalID, Kind: "ocr", AtMS: frame.AtMS})
			}
		}
		for _, segment := range item.TranscriptSegments {
			caseSignals.Signals = append(caseSignals.Signals, fillereval.TemporalSignal{ID: segment.ID, Kind: "transcript", AtMS: segment.StartMS})
		}
		signals = append(signals, caseSignals)
	}
	return pack, signals, digest, nil
}

func validateTemporalModelReviewPackage(root string, pack TemporalModelReviewPackage, expectedCases int) error {
	if pack.SchemaVersion != TemporalModelReviewSchemaVersion || pack.ContractVersion != TemporalModelReviewContractVersion || pack.QuestionVersion != TemporalHumanReviewQuestionVersion || pack.EvidenceViewVersion != TemporalModelReviewEvidenceViewVersion || !validTemporalModelPanelSlot(pack.PanelSlot) || strings.TrimSpace(pack.BatchID) == "" || pack.PreparedAt.IsZero() || !reviewSHA256(pack.EvidenceManifestSHA256) || !reviewSHA256(pack.SelectionSHA256) || !reviewSHA256(pack.SeedSHA256) || len(pack.Cases) != expectedCases {
		return fmt.Errorf("temporal model review package identity or case count is invalid")
	}
	aliases := make(map[string]struct{}, len(pack.Cases))
	for _, item := range pack.Cases {
		if !validTemporalModelAlias(item.Alias) || item.DurationMS <= 0 || len(item.Frames) == 0 || len(item.Frames) > TemporalEvidenceMaxFrames {
			return fmt.Errorf("temporal model review package contains an invalid case")
		}
		if _, duplicate := aliases[item.Alias]; duplicate {
			return fmt.Errorf("temporal model review package repeats alias %q", item.Alias)
		}
		aliases[item.Alias] = struct{}{}
		if err := validateTemporalReviewCase(root, item); err != nil {
			return fmt.Errorf("temporal model review alias %q: %w", item.Alias, err)
		}
	}
	return nil
}

func validateTemporalModelReviewMap(pack TemporalModelReviewPackage, packageSHA string, mapping TemporalModelReviewMap) (map[string]string, error) {
	if mapping.SchemaVersion != TemporalModelReviewSchemaVersion || mapping.ContractVersion != TemporalModelReviewContractVersion || mapping.PanelSlot != pack.PanelSlot || mapping.BatchID != pack.BatchID || !mapping.PreparedAt.Equal(pack.PreparedAt) || temporalTruthHash([]byte(mapping.Seed)) != pack.SeedSHA256 || mapping.EvidenceManifestSHA256 != pack.EvidenceManifestSHA256 || mapping.SelectionSHA256 != pack.SelectionSHA256 || mapping.PackageSHA256 != packageSHA || len(mapping.Entries) != len(pack.Cases) {
		return nil, fmt.Errorf("temporal model review map does not bind the exact package")
	}
	publicAliases := make(map[string]struct{}, len(pack.Cases))
	for _, item := range pack.Cases {
		publicAliases[item.Alias] = struct{}{}
	}
	result := make(map[string]string, len(mapping.Entries))
	evidenceAliases := make(map[string]struct{}, len(mapping.Entries))
	for _, entry := range mapping.Entries {
		if _, exists := publicAliases[entry.Alias]; !exists || !strings.HasPrefix(entry.EvidenceAlias, "evidence-") {
			return nil, fmt.Errorf("temporal model review map contains an unknown alias")
		}
		if _, duplicate := result[entry.Alias]; duplicate {
			return nil, fmt.Errorf("temporal model review map repeats alias %q", entry.Alias)
		}
		if _, duplicate := evidenceAliases[entry.EvidenceAlias]; duplicate {
			return nil, fmt.Errorf("temporal model review map repeats evidence alias %q", entry.EvidenceAlias)
		}
		result[entry.Alias] = entry.EvidenceAlias
		evidenceAliases[entry.EvidenceAlias] = struct{}{}
	}
	return result, nil
}
