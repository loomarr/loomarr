package fillerreview

import (
	"encoding/hex"
	"fmt"
	"strings"
	"time"
)

const (
	TemporalModelReviewSchemaVersion       = 1
	TemporalModelReviewContractVersion     = "filler-temporal-model-review-v1"
	TemporalModelReviewEvidenceViewVersion = "filler-temporal-frames-ocr-transcript-v1"
)

// TemporalModelReviewPackage is the complete model-visible contract. It uses
// a fresh namespace and deliberately excludes stable evidence aliases, source
// identity, selection strata, historical labels, human answers, and model
// identity. Model identity belongs to the separately locked inference result.
type TemporalModelReviewPackage struct {
	SchemaVersion          int                  `json:"schemaVersion"`
	ContractVersion        string               `json:"contractVersion"`
	QuestionVersion        string               `json:"questionVersion"`
	EvidenceViewVersion    string               `json:"evidenceViewVersion"`
	PanelSlot              string               `json:"panelSlot"`
	BatchID                string               `json:"batchId"`
	PreparedAt             time.Time            `json:"preparedAt"`
	EvidenceManifestSHA256 string               `json:"evidenceManifestSha256"`
	SelectionSHA256        string               `json:"selectionSha256"`
	SeedSHA256             string               `json:"seedSha256"`
	Cases                  []TemporalReviewCase `json:"cases"`
}

// TemporalModelReviewMap is coordinator-only. It is the sole bridge from a
// model batch alias to the stable opaque evidence alias shared across batches.
type TemporalModelReviewMap struct {
	SchemaVersion          int                           `json:"schemaVersion"`
	ContractVersion        string                        `json:"contractVersion"`
	PanelSlot              string                        `json:"panelSlot"`
	BatchID                string                        `json:"batchId"`
	PreparedAt             time.Time                     `json:"preparedAt"`
	Seed                   string                        `json:"seed"`
	EvidenceManifestSHA256 string                        `json:"evidenceManifestSha256"`
	SelectionSHA256        string                        `json:"selectionSha256"`
	PackageSHA256          string                        `json:"packageSha256"`
	Entries                []TemporalModelReviewMapEntry `json:"entries"`
}

type TemporalModelReviewMapEntry struct {
	Alias         string `json:"alias"`
	EvidenceAlias string `json:"evidenceAlias"`
}

func temporalModelReviewAlias(seed, batchID, panelSlot, evidenceAlias string) string {
	return "model-" + temporalTruthHash([]byte(seed + "\x00alias\x00" + batchID + "\x00" + panelSlot + "\x00" + evidenceAlias))[:32]
}

func temporalModelReviewRank(seed, batchID, panelSlot, evidenceAlias string) string {
	return temporalTruthHash([]byte(seed + "\x00order\x00" + batchID + "\x00" + panelSlot + "\x00" + evidenceAlias))
}

func validTemporalModelAlias(alias string) bool {
	if len(alias) != len("model-")+32 || !strings.HasPrefix(alias, "model-") {
		return false
	}
	_, err := hex.DecodeString(strings.TrimPrefix(alias, "model-"))
	return err == nil
}

func validTemporalModelPanelSlot(slot string) bool {
	if len(slot) < 3 || len(slot) > 32 || strings.TrimSpace(slot) != slot {
		return false
	}
	for _, value := range slot {
		if value != '-' && value != '_' && (value < 'a' || value > 'z') && (value < '0' || value > '9') {
			return false
		}
	}
	return true
}

func temporalModelOCR(source []TemporalTruthOCRObservation) []TemporalReviewOCRObservation {
	result := make([]TemporalReviewOCRObservation, 0, len(source))
	for _, item := range source {
		result = append(result, TemporalReviewOCRObservation(item))
	}
	return result
}

func temporalModelSignalID(frameID string, hasOCR bool) string {
	if !hasOCR {
		return ""
	}
	return fmt.Sprintf("%s-ocr", frameID)
}
