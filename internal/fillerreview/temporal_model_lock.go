package fillerreview

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/loomarr/loomarr/internal/fillerbakeoff"
	"github.com/loomarr/loomarr/internal/fillereval"
)

const (
	TemporalModelAssessmentSchemaVersion   = 1
	TemporalModelAssessmentContractVersion = "filler-temporal-model-assessment-v1"
)

type TemporalModelAssessmentLockConfig struct {
	PackagePath          string
	PrivateMapPath       string
	ResultPath           string
	SnapshotPath         string
	HumanAssessmentPath  string
	HumanAttestationPath string
	ExpectedCases        int
	ReleasedAt           time.Time
	OutputDir            string
}

type TemporalModelAssessmentLockResult struct {
	Assessments         int
	AssessmentSetSHA256 string
	AttestationSHA256   string
}

// TemporalModelAssessmentSet is canonical across independently blinded model
// batches. Stable evidence aliases remain opaque; per-axis signal timestamps
// make comparison independent of a batch's temporary alias namespace.
type TemporalModelAssessmentSet struct {
	SchemaVersion              int                                 `json:"schemaVersion"`
	ContractVersion            string                              `json:"contractVersion"`
	PanelSlot                  string                              `json:"panelSlot"`
	BatchID                    string                              `json:"batchId"`
	EvidenceManifestSHA256     string                              `json:"evidenceManifestSha256"`
	SelectionSHA256            string                              `json:"selectionSha256"`
	PackageSHA256              string                              `json:"packageSha256"`
	MapSHA256                  string                              `json:"mapSha256"`
	RawResultSHA256            string                              `json:"rawResultSha256"`
	SnapshotFileSHA256         string                              `json:"snapshotFileSha256"`
	CapabilitySnapshotSHA256   string                              `json:"capabilitySnapshotSha256"`
	HumanAssessmentSetSHA256   string                              `json:"humanAssessmentSetSha256"`
	HumanAttestationFileSHA256 string                              `json:"humanAttestationFileSha256"`
	HumanAttestationSHA256     string                              `json:"humanAttestationSha256"`
	ReleasedAt                 time.Time                           `json:"releasedAt"`
	Assessor                   fillereval.TemporalAssessorIdentity `json:"assessor"`
	Assessments                []TemporalLockedModelAssessment     `json:"assessments"`
}

type TemporalLockedModelAssessment struct {
	EvidenceAlias      string                                 `json:"evidenceAlias"`
	Unit               *fillereval.UnitAssessment             `json:"unit,omitempty"`
	UnitDecisiveAtMS   []int64                                `json:"unitDecisiveAtMs,omitempty"`
	Role               *fillereval.RoleAssessment             `json:"role,omitempty"`
	RoleDecisiveAtMS   []int64                                `json:"roleDecisiveAtMs,omitempty"`
	OperationalFailure *fillereval.TemporalOperationalFailure `json:"operationalFailure,omitempty"`
	Inference          fillereval.TemporalInference           `json:"inference"`
}

type TemporalModelAssessmentAttestation struct {
	SchemaVersion              int       `json:"schemaVersion"`
	ContractVersion            string    `json:"contractVersion"`
	PanelSlot                  string    `json:"panelSlot"`
	BatchID                    string    `json:"batchId"`
	CompletedAt                time.Time `json:"completedAt"`
	ReleasedAt                 time.Time `json:"releasedAt"`
	PackageSHA256              string    `json:"packageSha256"`
	MapSHA256                  string    `json:"mapSha256"`
	RawResultSHA256            string    `json:"rawResultSha256"`
	SnapshotFileSHA256         string    `json:"snapshotFileSha256"`
	CapabilitySnapshotSHA256   string    `json:"capabilitySnapshotSha256"`
	HumanAssessmentSetSHA256   string    `json:"humanAssessmentSetSha256"`
	HumanAttestationFileSHA256 string    `json:"humanAttestationFileSha256"`
	HumanAttestationSHA256     string    `json:"humanAttestationSha256"`
	AssessmentSetSHA256        string    `json:"assessmentSetSha256"`
	AttestationSHA256          string    `json:"attestationSha256"`
}

// LockTemporalModelAssessment validates, unblinds, and releases one complete
// model result. It deliberately supports only post-human inference: every paid
// request must be timestamped at or after the immutable human lock.
func LockTemporalModelAssessment(config TemporalModelAssessmentLockConfig) (TemporalModelAssessmentLockResult, error) {
	if err := validateTemporalModelAssessmentLockConfig(config); err != nil {
		return TemporalModelAssessmentLockResult{}, err
	}
	pack, signals, packageSHA, err := LoadTemporalModelReviewPackage(config.PackagePath, config.ExpectedCases)
	if err != nil {
		return TemporalModelAssessmentLockResult{}, err
	}
	mapping, err := readStrictJSON[TemporalModelReviewMap](config.PrivateMapPath)
	if err != nil {
		return TemporalModelAssessmentLockResult{}, fmt.Errorf("read temporal model review map: %w", err)
	}
	mapSHA, err := hashFile(config.PrivateMapPath)
	if err != nil {
		return TemporalModelAssessmentLockResult{}, err
	}
	aliasMap, err := validateTemporalModelReviewMap(pack, packageSHA, mapping)
	if err != nil {
		return TemporalModelAssessmentLockResult{}, err
	}

	resultRaw, err := os.ReadFile(config.ResultPath)
	if err != nil {
		return TemporalModelAssessmentLockResult{}, fmt.Errorf("read temporal model result: %w", err)
	}
	result, err := DecodeOpenRouterTemporalResult(resultRaw)
	if err != nil {
		return TemporalModelAssessmentLockResult{}, err
	}
	loaded := temporalInferencePackage{
		BatchID: pack.BatchID, Cases: pack.Cases, Signals: signals, PackageSHA256: packageSHA,
		SelectionSHA256: pack.SelectionSHA256, PackageCaseCount: len(pack.Cases),
	}
	if err := validateOpenRouterTemporalInferenceResult(result, loaded); err != nil {
		return TemporalModelAssessmentLockResult{}, err
	}
	rawResultSHA := hashBytes(resultRaw)

	snapshotRaw, err := os.ReadFile(config.SnapshotPath)
	if err != nil {
		return TemporalModelAssessmentLockResult{}, fmt.Errorf("read OpenRouter snapshot: %w", err)
	}
	var snapshot fillerbakeoff.OpenRouterSnapshot
	if err := decodeStrictReviewJSON(snapshotRaw, &snapshot); err != nil {
		return TemporalModelAssessmentLockResult{}, fmt.Errorf("decode OpenRouter snapshot: %w", err)
	}
	if err := fillerbakeoff.ValidateOpenRouterSnapshot(snapshot); err != nil {
		return TemporalModelAssessmentLockResult{}, err
	}
	capabilitySHA := fillerbakeoff.OpenRouterSnapshotSHA256(snapshot)
	if capabilitySHA != result.CapabilitySnapshotSHA256 {
		return TemporalModelAssessmentLockResult{}, fmt.Errorf("OpenRouter snapshot does not bind the model result")
	}
	snapshotFileSHA := hashBytes(snapshotRaw)

	humanSet, humanAttestation, humanSetSHA, humanAttestationFileSHA, err := loadTemporalHumanLockAuthority(config.HumanAssessmentPath, config.HumanAttestationPath)
	if err != nil {
		return TemporalModelAssessmentLockResult{}, err
	}
	if err := validateTemporalModelPostHumanOrder(result, humanAttestation, config.ReleasedAt); err != nil {
		return TemporalModelAssessmentLockResult{}, err
	}
	if err := validateTemporalHumanReferenceForModel(humanSet, humanAttestation, pack, aliasMap); err != nil {
		return TemporalModelAssessmentLockResult{}, err
	}

	canonical, err := buildTemporalModelAssessmentSet(pack, signals, packageSHA, mapSHA, rawResultSHA, snapshotFileSHA, humanSetSHA, humanAttestationFileSHA, humanAttestation, aliasMap, result, config.ReleasedAt.UTC())
	if err != nil {
		return TemporalModelAssessmentLockResult{}, err
	}
	stage, err := beginTemporalHumanReviewStage(config.OutputDir)
	if err != nil {
		return TemporalModelAssessmentLockResult{}, err
	}
	defer stage.Cleanup()
	assessmentRaw, err := json.MarshalIndent(canonical, "", "  ")
	if err != nil {
		return TemporalModelAssessmentLockResult{}, err
	}
	assessmentRaw = append(assessmentRaw, '\n')
	assessmentSHA := hashBytes(assessmentRaw)
	attestation := TemporalModelAssessmentAttestation{
		SchemaVersion: TemporalModelAssessmentSchemaVersion, ContractVersion: TemporalModelAssessmentContractVersion,
		PanelSlot: pack.PanelSlot, BatchID: pack.BatchID, CompletedAt: result.CompletedAt.UTC(), ReleasedAt: config.ReleasedAt.UTC(),
		PackageSHA256: packageSHA, MapSHA256: mapSHA, RawResultSHA256: rawResultSHA,
		SnapshotFileSHA256: snapshotFileSHA, CapabilitySnapshotSHA256: capabilitySHA,
		HumanAssessmentSetSHA256: humanSetSHA, HumanAttestationFileSHA256: humanAttestationFileSHA,
		HumanAttestationSHA256: humanAttestation.AttestationSHA256, AssessmentSetSHA256: assessmentSHA,
	}
	attestation.AttestationSHA256 = temporalModelAssessmentAttestationSHA256(attestation)
	attestationRaw, err := json.MarshalIndent(attestation, "", "  ")
	if err != nil {
		return TemporalModelAssessmentLockResult{}, err
	}
	attestationRaw = append(attestationRaw, '\n')
	if err := writeTemporalTruthNew(filepath.Join(stage.path, "assessment-set.json"), assessmentRaw, 0o600); err != nil {
		return TemporalModelAssessmentLockResult{}, err
	}
	if err := writeTemporalTruthNew(filepath.Join(stage.path, "attestation.json"), attestationRaw, 0o600); err != nil {
		return TemporalModelAssessmentLockResult{}, err
	}
	if err := stage.Publish(); err != nil {
		return TemporalModelAssessmentLockResult{}, err
	}
	return TemporalModelAssessmentLockResult{Assessments: len(canonical.Assessments), AssessmentSetSHA256: assessmentSHA, AttestationSHA256: attestation.AttestationSHA256}, nil
}

func buildTemporalModelAssessmentSet(pack TemporalModelReviewPackage, signals []fillereval.TemporalCaseSignals, packageSHA, mapSHA, rawResultSHA, snapshotFileSHA, humanSetSHA, humanAttestationFileSHA string, humanAttestation TemporalHumanReviewAttestation, aliasMap map[string]string, result OpenRouterTemporalResult, releasedAt time.Time) (TemporalModelAssessmentSet, error) {
	signalTimes := make(map[string]map[string]int64, len(signals))
	for _, item := range signals {
		times := make(map[string]int64, len(item.Signals))
		for _, signal := range item.Signals {
			times[signal.ID] = signal.AtMS
		}
		signalTimes[item.Alias] = times
	}
	set := TemporalModelAssessmentSet{
		SchemaVersion: TemporalModelAssessmentSchemaVersion, ContractVersion: TemporalModelAssessmentContractVersion,
		PanelSlot: pack.PanelSlot, BatchID: pack.BatchID, EvidenceManifestSHA256: pack.EvidenceManifestSHA256,
		SelectionSHA256: pack.SelectionSHA256, PackageSHA256: packageSHA, MapSHA256: mapSHA, RawResultSHA256: rawResultSHA,
		SnapshotFileSHA256: snapshotFileSHA, CapabilitySnapshotSHA256: result.CapabilitySnapshotSHA256,
		HumanAssessmentSetSHA256: humanSetSHA, HumanAttestationFileSHA256: humanAttestationFileSHA,
		HumanAttestationSHA256: humanAttestation.AttestationSHA256, ReleasedAt: releasedAt, Assessor: result.AssessmentSet.Assessor,
	}
	for _, assessment := range result.AssessmentSet.Assessments {
		evidenceAlias, exists := aliasMap[assessment.Alias]
		if !exists {
			return TemporalModelAssessmentSet{}, fmt.Errorf("model result names unknown alias %q", assessment.Alias)
		}
		locked := TemporalLockedModelAssessment{
			EvidenceAlias: evidenceAlias, Unit: assessment.Unit, Role: assessment.Role,
			OperationalFailure: assessment.OperationalFailure, Inference: assessment.Inference,
		}
		if assessment.Unit != nil {
			locked.UnitDecisiveAtMS = temporalModelDecisiveTimes(signalTimes[assessment.Alias], assessment.Unit.DecisiveSignalIDs)
		}
		if assessment.Role != nil {
			locked.RoleDecisiveAtMS = temporalModelDecisiveTimes(signalTimes[assessment.Alias], assessment.Role.DecisiveSignalIDs)
		}
		set.Assessments = append(set.Assessments, locked)
	}
	sort.Slice(set.Assessments, func(i, j int) bool { return set.Assessments[i].EvidenceAlias < set.Assessments[j].EvidenceAlias })
	return set, nil
}

func temporalModelDecisiveTimes(signalTimes map[string]int64, signalIDs []string) []int64 {
	result := make([]int64, 0, len(signalIDs))
	for _, id := range signalIDs {
		result = append(result, signalTimes[id])
	}
	sort.Slice(result, func(i, j int) bool { return result[i] < result[j] })
	return result
}

func validateTemporalModelAssessmentLockConfig(config TemporalModelAssessmentLockConfig) error {
	if strings.TrimSpace(config.PackagePath) == "" || strings.TrimSpace(config.PrivateMapPath) == "" || strings.TrimSpace(config.ResultPath) == "" || strings.TrimSpace(config.SnapshotPath) == "" || strings.TrimSpace(config.HumanAssessmentPath) == "" || strings.TrimSpace(config.HumanAttestationPath) == "" || config.ExpectedCases <= 0 || config.ReleasedAt.IsZero() || strings.TrimSpace(config.OutputDir) == "" {
		return fmt.Errorf("temporal model assessment lock requires package, map, result, snapshot, human authority, expected cases, release time, and output")
	}
	return nil
}

func temporalModelAssessmentAttestationSHA256(attestation TemporalModelAssessmentAttestation) string {
	attestation.AttestationSHA256 = ""
	return temporalTruthJSONSHA(attestation)
}
