package fillerreference

import (
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/loomarr/loomarr/internal/mediatools"
)

type InspectionSeed struct {
	SchemaVersion        int                     `json:"schemaVersion"`
	Kind                 string                  `json:"kind"`
	Status               string                  `json:"status"`
	ContractVersion      string                  `json:"contractVersion"`
	SourceAuditSHA256    string                  `json:"sourceAuditSha256"`
	DuplicateAuditSHA256 string                  `json:"duplicateAuditSha256"`
	TriageEvidence       TriageEvidence          `json:"triageEvidence"`
	SelectionPolicy      InspectionSelection     `json:"selectionPolicy"`
	SelectedCaseIDs      []string                `json:"selectedCaseIds"`
	ExplicitFindings     []ExplicitTriageFinding `json:"explicitTriageFindings"`
	RequiredNextGate     string                  `json:"requiredNextGate"`
}

type TriageEvidence struct {
	Method                 string `json:"method"`
	SampledFramesPerSource int    `json:"sampledFramesPerSource"`
	Limitations            string `json:"limitations"`
}

type InspectionSelection struct {
	TargetSources      int      `json:"targetSources"`
	FinalAcceptedClips int      `json:"finalAcceptedClips"`
	Prefer             []string `json:"prefer"`
	DoNotBackfill      []string `json:"doNotBackfill"`
	CoverageShortfall  string   `json:"coverageShortfall"`
}

type ExplicitTriageFinding struct {
	CaseID          string `json:"caseId,omitempty"`
	CaseIDPattern   string `json:"caseIdPattern,omitempty"`
	Disposition     string `json:"disposition"`
	PreferredCaseID string `json:"preferredCaseId,omitempty"`
	Evidence        string `json:"evidence"`
}

type MeasurementInputIdentity struct {
	AuditSHA256  string `json:"auditSha256"`
	FamilySHA256 string `json:"familyAuditSha256"`
	SeedSHA256   string `json:"inspectionSeedSha256"`
}

type InspectionMeasurement struct {
	CaseID           string                              `json:"caseId"`
	ContentSHA256    string                              `json:"contentSha256"`
	LocalFile        string                              `json:"localFile"`
	Status           string                              `json:"status"`
	Measurement      *mediatools.ConditioningMeasurement `json:"measurement,omitempty"`
	MeasurementError string                              `json:"measurementError,omitempty"`
}

type MeasurementSummary struct {
	Cases          int `json:"cases"`
	Measured       int `json:"measured"`
	TechnicalHolds int `json:"technicalHolds"`
}

type MeasurementArtifact struct {
	SchemaVersion int                      `json:"schemaVersion"`
	GeneratedAt   time.Time                `json:"generatedAt"`
	Inputs        MeasurementInputIdentity `json:"inputs"`
	Summary       MeasurementSummary       `json:"summary"`
	Cases         []InspectionMeasurement  `json:"cases"`
}

// InspectionCases validates that the hand-authored seed is bound to the exact
// audit and duplicate evidence and returns the selected cases in seed order.
func InspectionCases(seed InspectionSeed, audit Audit, families FamilyAudit, inputs MeasurementInputIdentity) ([]Case, error) {
	if seed.SchemaVersion != 1 || seed.Kind != "filler_reference_inspection_seed" || seed.Status != "preliminary_requires_full_playback" || seed.ContractVersion != ContractVersion {
		return nil, fmt.Errorf("inspection seed identity is invalid")
	}
	if inputs.AuditSHA256 == "" || inputs.FamilySHA256 == "" || inputs.SeedSHA256 == "" || seed.SourceAuditSHA256 != inputs.AuditSHA256 || seed.DuplicateAuditSHA256 != inputs.FamilySHA256 || families.SourceAudit != inputs.AuditSHA256 {
		return nil, fmt.Errorf("inspection seed input binding is invalid")
	}
	if seed.SelectionPolicy.TargetSources != 50 || seed.SelectionPolicy.FinalAcceptedClips != 32 || len(seed.SelectedCaseIDs) != seed.SelectionPolicy.TargetSources || seed.TriageEvidence.SampledFramesPerSource != 4 || strings.TrimSpace(seed.RequiredNextGate) == "" {
		return nil, fmt.Errorf("inspection seed size or gate is invalid")
	}
	byID := make(map[string]Case, len(audit.Cases))
	for _, item := range audit.Cases {
		byID[item.CaseID] = item
	}
	seen := map[string]struct{}{}
	selected := make([]Case, 0, len(seed.SelectedCaseIDs))
	for _, id := range seed.SelectedCaseIDs {
		item, ok := byID[id]
		if !ok || item.Disposition == DispositionExclude || item.ContentSHA256 == "" || item.SourceLocalFile == "" || item.Media.SourceDurationMS <= 0 || item.Media.SourceDurationMS > mediatools.ConditioningMaxDurationMs {
			return nil, fmt.Errorf("inspection case %q is absent, excluded, or outside the measurement contract", id)
		}
		if _, duplicate := seen[id]; duplicate {
			return nil, fmt.Errorf("inspection case %q repeats", id)
		}
		seen[id] = struct{}{}
		selected = append(selected, item)
	}
	if !slices.IsSortedFunc(families.Fingerprints, func(a, b FamilyFingerprint) int { return strings.Compare(a.CaseID, b.CaseID) }) {
		return nil, fmt.Errorf("duplicate fingerprints are not in canonical order")
	}
	return selected, nil
}
