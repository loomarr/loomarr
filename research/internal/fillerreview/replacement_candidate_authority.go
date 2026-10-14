package fillerreview

import (
	"fmt"
	"os"
	"sort"
	"time"

	"github.com/loomarr/loomarr/internal/fillereval"
)

// ReplacementCandidateReviewConfig names the immutable review authorities
// that jointly own semantic, technical, audience, family, and transition
// decisions. SourceRoot is used only to make reviewed media paths portable.
type ReplacementCandidateReviewConfig struct {
	SelectionPath               string
	EvidenceManifestPath        string
	EvidencePrivateMapPath      string
	HumanAssessmentPath         string
	HumanAttestationPath        string
	MediaQualityPath            string
	SuitabilityPath             string
	ReferenceAuditPath          string
	ReferenceDownloadLedgerPath string
	FamilyAuditPath             string
	TransitionAuthorityPath     string
	SourceRoot                  string
	OpenedAt                    time.Time
}

type ReplacementCandidateReviewAuthority struct {
	Inputs []TemporalStructureHoldoutInput
	Cases  []ReplacementCandidateReviewCase
}

type ReplacementCandidateReviewCase struct {
	CaseID              string
	EvidenceAlias       string
	InspectedSourceFile string
	InspectedSourceSHA  string
	ReviewedMediaPath   string
	ReviewedMediaSHA    string
	ReviewedMediaBytes  int64
	DurationMS          int64
	Unit                fillereval.UnitKind
	Role                *fillereval.TemporalRole
	TechnicalFailure    string
	TechnicalVerdict    string
	HadAudio            bool
	FullDecodeMeasured  bool
	Suitability         string
	FamilyID            string
	Transition          TemporalTransitionAuthorityCase
}

// OpenReplacementCandidateReviewAuthority reuses the same strict loaders as
// genesis planning and exposes one normalized, read-only join to the pool
// builder. It performs no media work or policy projection.
func OpenReplacementCandidateReviewAuthority(config ReplacementCandidateReviewConfig) (ReplacementCandidateReviewAuthority, error) {
	selectionRaw, err := os.ReadFile(config.SelectionPath)
	if err != nil {
		return ReplacementCandidateReviewAuthority{}, fmt.Errorf("read replacement selection: %w", err)
	}
	selection, err := fillereval.DecodeTemporalTruthSelection(selectionRaw)
	if err != nil {
		return ReplacementCandidateReviewAuthority{}, err
	}
	evidence, evidenceSHA, err := LoadTemporalTruthEvidence(config.EvidenceManifestPath)
	if err != nil {
		return ReplacementCandidateReviewAuthority{}, err
	}
	privateRaw, err := os.ReadFile(config.EvidencePrivateMapPath)
	if err != nil {
		return ReplacementCandidateReviewAuthority{}, fmt.Errorf("read replacement evidence map: %w", err)
	}
	privateMap, err := readStrictJSON[TemporalTruthEvidencePrivateMap](config.EvidencePrivateMapPath)
	if err != nil {
		return ReplacementCandidateReviewAuthority{}, fmt.Errorf("decode replacement evidence map: %w", err)
	}
	if err := validateTemporalHumanEvidenceJoin(evidence, evidenceSHA, privateMap); err != nil {
		return ReplacementCandidateReviewAuthority{}, err
	}
	if err := validateTemporalHumanSelectionJoin(selection, privateMap); err != nil {
		return ReplacementCandidateReviewAuthority{}, err
	}
	human, attestation, humanSHA, attestationSHA, err := loadTemporalHumanLockAuthority(config.HumanAssessmentPath, config.HumanAttestationPath)
	if err != nil {
		return ReplacementCandidateReviewAuthority{}, err
	}
	if err := validateTemporalStructureHoldoutHuman(human, attestation, evidence, evidenceSHA); err != nil {
		return ReplacementCandidateReviewAuthority{}, err
	}
	quality, _, qualitySHA, err := loadMediaIntegrityQualityReport(config.MediaQualityPath)
	if err != nil {
		return ReplacementCandidateReviewAuthority{}, err
	}
	if err := validateTemporalStructureHoldoutQuality(quality, human, attestation, humanSHA, attestationSHA, evidence, evidenceSHA, config.OpenedAt); err != nil {
		return ReplacementCandidateReviewAuthority{}, err
	}
	suitability, suitabilitySHA, err := loadTemporalStructureHoldoutSuitability(config.SuitabilityPath, evidence, evidenceSHA)
	if err != nil {
		return ReplacementCandidateReviewAuthority{}, err
	}
	reference, referenceSHA, err := loadTemporalStructureHoldoutReferenceAudit(config.ReferenceAuditPath, config.OpenedAt)
	if err != nil {
		return ReplacementCandidateReviewAuthority{}, err
	}
	ledgerSHA, _, err := loadTemporalStructureHoldoutReferenceDownloadLedger(config.ReferenceDownloadLedgerPath, reference)
	if err != nil {
		return ReplacementCandidateReviewAuthority{}, err
	}
	family, familySHA, err := loadTemporalStructureHoldoutFamily(config.FamilyAuditPath, selection, reference, referenceSHA, config.OpenedAt)
	if err != nil {
		return ReplacementCandidateReviewAuthority{}, err
	}
	transition, transitionSHA, err := loadTemporalTransitionAuthority(config.TransitionAuthorityPath, evidence, evidenceSHA, privateMap, hashBytes(privateRaw), config.OpenedAt)
	if err != nil {
		return ReplacementCandidateReviewAuthority{}, err
	}

	authority := ReplacementCandidateReviewAuthority{Inputs: []TemporalStructureHoldoutInput{
		{Name: "evidence_manifest", SHA256: evidenceSHA}, {Name: "evidence_private_map", SHA256: hashBytes(privateRaw)},
		{Name: "family_audit", SHA256: familySHA}, {Name: "human_assessment", SHA256: humanSHA},
		{Name: "human_attestation", SHA256: attestationSHA}, {Name: "media_quality", SHA256: qualitySHA},
		{Name: "reference_audit", SHA256: referenceSHA}, {Name: "reference_download_ledger", SHA256: ledgerSHA},
		{Name: "selection", SHA256: hashBytes(selectionRaw)}, {Name: "suitability", SHA256: suitabilitySHA},
		{Name: "transition_authority", SHA256: transitionSHA},
	}}
	sort.Slice(authority.Inputs, func(i, j int) bool { return authority.Inputs[i].Name < authority.Inputs[j].Name })

	evidenceByAlias := make(map[string]TemporalTruthEvidenceCase, len(evidence.Cases))
	for _, item := range evidence.Cases {
		evidenceByAlias[item.Alias] = item
	}
	mapByAlias := make(map[string]TemporalTruthEvidencePrivateEntry, len(privateMap.Entries))
	for _, item := range privateMap.Entries {
		mapByAlias[item.Alias] = item
	}
	qualityByAlias := make(map[string]TemporalMediaQualityCase, len(quality.CaseMeasurements))
	for _, item := range quality.CaseMeasurements {
		qualityByAlias[item.EvidenceAlias] = item
	}
	suitabilityByAlias := make(map[string]TemporalSuitabilityCaseComparison, len(suitability.CaseComparisons))
	for _, item := range suitability.CaseComparisons {
		suitabilityByAlias[item.EvidenceAlias] = item
	}
	transitionByAlias := make(map[string]TemporalTransitionAuthorityCase, len(transition.Cases))
	for _, item := range transition.Cases {
		transitionByAlias[item.EvidenceAlias] = item
	}
	familyByCase := make(map[string]string)
	for _, group := range family.Families {
		for _, member := range group.Members {
			familyByCase[member] = group.FamilyID
		}
	}
	fingerprintByCase := make(map[string]temporalStructureHoldoutFingerprint, len(family.Fingerprints))
	for _, item := range family.Fingerprints {
		fingerprintByCase[item.CaseID] = item
	}

	for _, assessment := range human.Assessments {
		evidenceCase, evidenceOK := evidenceByAlias[assessment.EvidenceAlias]
		mapping, mappingOK := mapByAlias[assessment.EvidenceAlias]
		measurement, qualityOK := qualityByAlias[assessment.EvidenceAlias]
		safety, suitabilityOK := suitabilityByAlias[assessment.EvidenceAlias]
		edges, transitionOK := transitionByAlias[assessment.EvidenceAlias]
		fingerprint, familyOK := fingerprintByCase[mapping.CaseID]
		if !evidenceOK || !mappingOK || !qualityOK || !suitabilityOK || !transitionOK || !familyOK ||
			fingerprint.ContentSHA256 != mapping.ContentSHA256 || measurement.SourceMediaSHA256 != evidenceCase.Video.SHA256 {
			return ReplacementCandidateReviewAuthority{}, fmt.Errorf("replacement review case %q lacks complete bound authority", assessment.EvidenceAlias)
		}
		relative, err := temporalStructureHoldoutRelativeEvidencePath(config.SourceRoot, config.EvidenceManifestPath, evidenceCase.Video.Path)
		if err != nil {
			return ReplacementCandidateReviewAuthority{}, err
		}
		familyID := familyByCase[mapping.CaseID]
		if familyID == "" {
			familyID = "singleton-" + hashBytes([]byte(mapping.CaseID))[:24]
		}
		authority.Cases = append(authority.Cases, ReplacementCandidateReviewCase{
			CaseID: mapping.CaseID, EvidenceAlias: assessment.EvidenceAlias,
			InspectedSourceFile: mapping.SourceLocalFile, InspectedSourceSHA: mapping.SourceSHA256,
			ReviewedMediaPath: relative, ReviewedMediaSHA: evidenceCase.Video.SHA256,
			ReviewedMediaBytes: evidenceCase.Video.Bytes, DurationMS: evidenceCase.Video.DurationMS,
			Unit: assessment.Unit, Role: assessment.Role,
			TechnicalFailure: measurement.OperationalFailure, TechnicalVerdict: measurement.PolicyVerdict,
			HadAudio: measurement.HadAudio, FullDecodeMeasured: measurement.Measurement != nil,
			Suitability: safety.Disposition, FamilyID: familyID, Transition: edges,
		})
	}
	sort.Slice(authority.Cases, func(i, j int) bool { return authority.Cases[i].CaseID < authority.Cases[j].CaseID })
	return authority, nil
}
