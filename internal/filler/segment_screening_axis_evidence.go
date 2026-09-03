package filler

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"
)

const (
	SegmentScreeningAxisEvidenceSchemaVersion   = 1
	SegmentScreeningAxisEvidenceContractVersion = "filler-segment-screening-axis-evidence-v1"
)

// SegmentScreeningAxisProfile is the immutable evaluator identity locked by a release authority.
// The certificate owns model/route details when an axis uses inference; deterministic axes bind
// their measurement certification through the same small interface.
type SegmentScreeningAxisProfile struct {
	Axis                 SegmentScreeningAxis `json:"axis"`
	EvidenceContract     string               `json:"evidenceContract"`
	PolicySHA256         string               `json:"policySha256"`
	CertificationSHA256  string               `json:"certificationSha256"`
	ImplementationSHA256 string               `json:"implementationSha256"`
}

// SegmentScreeningAxisEvidence is the provider-neutral closed result for one axis and exact span.
// RawEvidenceSHA256 points at private bounded bytes stored before this record is published.
type SegmentScreeningAxisEvidence struct {
	SchemaVersion     int                         `json:"schemaVersion"`
	ContractVersion   string                      `json:"contractVersion"`
	Source            SplitSourceAsset            `json:"source"`
	StartMs           int64                       `json:"startMs"`
	EndMs             int64                       `json:"endMs"`
	Profile           SegmentScreeningAxisProfile `json:"profile"`
	Outcome           SegmentScreeningOutcome     `json:"outcome"`
	ReasonCode        string                      `json:"reasonCode"`
	RawEvidenceSHA256 string                      `json:"rawEvidenceSha256"`
	AssessedAt        time.Time                   `json:"assessedAt"`
	SHA256            string                      `json:"sha256"`
}

type RecordedSegmentScreeningAxisEvidence struct {
	Evidence    SegmentScreeningAxisEvidence
	RawEvidence []byte
}

func NewSegmentScreeningAxisEvidence(source SplitSourceAsset, startMs, endMs int64, profile SegmentScreeningAxisProfile, outcome SegmentScreeningOutcome, reasonCode string, rawEvidence []byte, assessedAt time.Time) (RecordedSegmentScreeningAxisEvidence, error) {
	record := RecordedSegmentScreeningAxisEvidence{
		Evidence: SegmentScreeningAxisEvidence{
			SchemaVersion: SegmentScreeningAxisEvidenceSchemaVersion, ContractVersion: SegmentScreeningAxisEvidenceContractVersion,
			Source: source, StartMs: startMs, EndMs: endMs, Profile: profile,
			Outcome: outcome, ReasonCode: reasonCode, RawEvidenceSHA256: screeningBytesSHA256(rawEvidence), AssessedAt: assessedAt.UTC(),
		},
		RawEvidence: append([]byte(nil), rawEvidence...),
	}
	record.Evidence.SHA256 = SegmentScreeningAxisEvidenceSHA256(record.Evidence)
	if err := ValidateRecordedSegmentScreeningAxisEvidence(record); err != nil {
		return RecordedSegmentScreeningAxisEvidence{}, err
	}
	return record, nil
}

func ValidateRecordedSegmentScreeningAxisEvidence(record RecordedSegmentScreeningAxisEvidence) error {
	if err := ValidateSegmentScreeningAxisEvidence(record.Evidence); err != nil {
		return err
	}
	if len(record.RawEvidence) == 0 || screeningBytesSHA256(record.RawEvidence) != record.Evidence.RawEvidenceSHA256 {
		return fmt.Errorf("segment screening axis raw evidence is missing or drifted")
	}
	return nil
}

func ValidateSegmentScreeningAxisEvidence(evidence SegmentScreeningAxisEvidence) error {
	projected := SegmentScreeningResult{
		Axis: evidence.Profile.Axis, Outcome: evidence.Outcome,
		AuthoritySHA256: evidence.SHA256, ReasonCode: evidence.ReasonCode,
	}
	if evidence.SchemaVersion != SegmentScreeningAxisEvidenceSchemaVersion ||
		evidence.ContractVersion != SegmentScreeningAxisEvidenceContractVersion ||
		evidence.Source.validate() != nil || evidence.StartMs < 0 || evidence.EndMs <= evidence.StartMs ||
		evidence.EndMs > evidence.Source.DurationMs || ValidateSegmentScreeningAxisProfile(evidence.Profile) != nil ||
		validateSegmentScreeningResult(projected) != nil || !isContentHash(evidence.RawEvidenceSHA256) ||
		evidence.AssessedAt.IsZero() || evidence.SHA256 != SegmentScreeningAxisEvidenceSHA256(evidence) {
		return fmt.Errorf("segment screening axis evidence is invalid")
	}
	return nil
}

func ValidateSegmentScreeningAxisProfile(profile SegmentScreeningAxisProfile) error {
	if validateSegmentScreeningAxis(profile.Axis) != nil || !validScreeningReasonCode(profile.EvidenceContract) ||
		!isContentHash(profile.PolicySHA256) || !isContentHash(profile.CertificationSHA256) || !isContentHash(profile.ImplementationSHA256) {
		return fmt.Errorf("segment screening axis profile is invalid")
	}
	return nil
}

func (e SegmentScreeningAxisEvidence) Result() SegmentScreeningResult {
	return SegmentScreeningResult{Axis: e.Profile.Axis, Outcome: e.Outcome, AuthoritySHA256: e.SHA256, ReasonCode: e.ReasonCode}
}

func SegmentScreeningAxisEvidenceSHA256(evidence SegmentScreeningAxisEvidence) string {
	evidence.SHA256 = ""
	raw, err := json.Marshal(evidence)
	if err != nil {
		return ""
	}
	return screeningBytesSHA256(raw)
}

func screeningBytesSHA256(raw []byte) string {
	digest := sha256.Sum256(raw)
	return hex.EncodeToString(digest[:])
}
