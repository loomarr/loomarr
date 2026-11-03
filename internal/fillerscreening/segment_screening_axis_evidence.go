package fillerscreening

import (
	"encoding/json"
	"fmt"
	"reflect"
	"slices"
	"time"

	"github.com/loomarr/loomarr/internal/filler"
	"github.com/loomarr/loomarr/internal/fillerairworthiness"
)

const (
	SegmentScreeningAxisEvidenceSchemaVersion   = 3
	SegmentScreeningAxisEvidenceContractVersion = "filler-rendered-child-screening-axis-evidence-v3"
)

// SegmentScreeningAxisProfile is the immutable evaluator identity locked by a release authority.
// The certificate owns model/route details when an axis uses inference; deterministic axes bind
// their measurement certification through the same small interface.
type SegmentScreeningAxisProfile struct {
	Axis                      filler.SegmentScreeningAxis `json:"axis"`
	EvidenceContract          string                      `json:"evidenceContract"`
	PolicySHA256              string                      `json:"policySha256"`
	CertificationSHA256       string                      `json:"certificationSha256"`
	ImplementationSHA256      string                      `json:"implementationSha256"`
	CertifiedSuitabilityFlags []fillerairworthiness.Flag  `json:"certifiedSuitabilityFlags"`
}

// SegmentScreeningAxisEvidence is the provider-neutral closed result for one axis and rendered child.
// RawEvidenceSHA256 points at private bounded bytes stored before this record is published.
type SegmentScreeningAxisEvidence struct {
	SchemaVersion     int                               `json:"schemaVersion"`
	ContractVersion   string                            `json:"contractVersion"`
	SubjectSHA256     string                            `json:"subjectSha256"`
	Profile           SegmentScreeningAxisProfile       `json:"profile"`
	Outcome           filler.SegmentScreeningOutcome    `json:"outcome"`
	ReasonCode        string                            `json:"reasonCode"`
	RawEvidenceSHA256 string                            `json:"rawEvidenceSha256"`
	Suitability       *fillerairworthiness.AxisEvidence `json:"suitability,omitempty"`
	AssessedAt        time.Time                         `json:"assessedAt"`
	SHA256            string                            `json:"sha256"`
}

type RecordedSegmentScreeningAxisEvidence struct {
	Evidence    SegmentScreeningAxisEvidence
	RawEvidence []byte
}

func NewSegmentScreeningAxisEvidence(subject filler.SegmentScreeningSubject, profile SegmentScreeningAxisProfile, outcome filler.SegmentScreeningOutcome, reasonCode string, suitability *fillerairworthiness.AxisEvidence, rawEvidence []byte, assessedAt time.Time) (RecordedSegmentScreeningAxisEvidence, error) {
	if err := filler.ValidateSegmentScreeningSubject(subject); err != nil {
		return RecordedSegmentScreeningAxisEvidence{}, err
	}
	record := RecordedSegmentScreeningAxisEvidence{
		Evidence: SegmentScreeningAxisEvidence{
			SchemaVersion: SegmentScreeningAxisEvidenceSchemaVersion, ContractVersion: SegmentScreeningAxisEvidenceContractVersion,
			SubjectSHA256: subject.SHA256, Profile: profile,
			Outcome: outcome, ReasonCode: reasonCode, RawEvidenceSHA256: filler.ScreeningBytesSHA256(rawEvidence),
			Suitability: cloneSuitabilityAxisEvidence(suitability), AssessedAt: assessedAt.UTC(),
		},
		RawEvidence: append([]byte(nil), rawEvidence...),
	}
	record.Evidence.Profile = cloneSegmentScreeningAxisProfile(profile)
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
	if len(record.RawEvidence) == 0 || filler.ScreeningBytesSHA256(record.RawEvidence) != record.Evidence.RawEvidenceSHA256 {
		return fmt.Errorf("segment screening axis raw evidence is missing or drifted")
	}
	return nil
}

func ValidateSegmentScreeningAxisEvidence(evidence SegmentScreeningAxisEvidence) error {
	projected := filler.SegmentScreeningResult{
		Axis: evidence.Profile.Axis, Outcome: evidence.Outcome,
		AuthoritySHA256: evidence.SHA256, ReasonCode: evidence.ReasonCode,
	}
	if evidence.SchemaVersion != SegmentScreeningAxisEvidenceSchemaVersion ||
		evidence.ContractVersion != SegmentScreeningAxisEvidenceContractVersion ||
		!filler.IsContentHash(evidence.SubjectSHA256) || ValidateSegmentScreeningAxisProfile(evidence.Profile) != nil ||
		filler.ValidateSegmentScreeningResult(projected) != nil || !filler.IsContentHash(evidence.RawEvidenceSHA256) ||
		evidence.AssessedAt.IsZero() || evidence.SHA256 != SegmentScreeningAxisEvidenceSHA256(evidence) {
		return fmt.Errorf("segment screening axis evidence is invalid")
	}
	if err := validateSegmentSuitabilityEvidence(evidence); err != nil {
		return err
	}
	return nil
}

func ValidateSegmentScreeningAxisProfile(profile SegmentScreeningAxisProfile) error {
	if filler.ValidateSegmentScreeningAxis(profile.Axis) != nil || !filler.ValidScreeningReasonCode(profile.EvidenceContract) ||
		!filler.IsContentHash(profile.PolicySHA256) || !filler.IsContentHash(profile.CertificationSHA256) || !filler.IsContentHash(profile.ImplementationSHA256) {
		return fmt.Errorf("segment screening axis profile is invalid")
	}
	if axis, safety := airworthinessAxis(profile.Axis); safety {
		normalized, err := fillerairworthiness.NormalizeAxisProfile(fillerairworthiness.AxisProfile{
			Axis: axis, EvidenceContract: profile.EvidenceContract, PolicySHA256: profile.PolicySHA256,
			CertificationSHA256: profile.CertificationSHA256, ImplementationSHA256: profile.ImplementationSHA256,
			CertifiedFlags: profile.CertifiedSuitabilityFlags,
		})
		if err != nil {
			return fmt.Errorf("segment screening safety profile: %w", err)
		}
		if !reflect.DeepEqual(profile.CertifiedSuitabilityFlags, normalized.CertifiedFlags) {
			return fmt.Errorf("segment screening safety profile flags are non-canonical")
		}
	} else if len(profile.CertifiedSuitabilityFlags) != 0 {
		return fmt.Errorf("non-safety screening profile cannot certify suitability flags")
	}
	return nil
}

func cloneSegmentScreeningAxisProfile(profile SegmentScreeningAxisProfile) SegmentScreeningAxisProfile {
	cloned := profile
	cloned.CertifiedSuitabilityFlags = slices.Clone(profile.CertifiedSuitabilityFlags)
	return cloned
}

func validateSegmentSuitabilityEvidence(evidence SegmentScreeningAxisEvidence) error {
	axis, safety := airworthinessAxis(evidence.Profile.Axis)
	if !safety {
		if evidence.Suitability != nil {
			return fmt.Errorf("non-safety screening evidence cannot carry suitability evidence")
		}
		return nil
	}
	if evidence.Suitability == nil || evidence.Suitability.SubjectSHA256 != evidence.SubjectSHA256 ||
		evidence.Suitability.EvidenceSHA256 != evidence.RawEvidenceSHA256 ||
		evidence.Suitability.Profile.Axis != axis {
		return fmt.Errorf("safety screening evidence lacks bound suitability evidence")
	}
	wantProfile, err := segmentAirworthinessProfile(evidence.Profile)
	if err != nil || !reflect.DeepEqual(evidence.Suitability.Profile, wantProfile) ||
		fillerairworthiness.ValidateAxisEvidence(*evidence.Suitability, fillerairworthiness.MaximumRenderedDurationMS) != nil {
		return fmt.Errorf("safety screening suitability evidence is invalid or profile-drifted")
	}
	return nil
}

func segmentAirworthinessProfile(profile SegmentScreeningAxisProfile) (fillerairworthiness.AxisProfile, error) {
	axis, safety := airworthinessAxis(profile.Axis)
	if !safety {
		return fillerairworthiness.AxisProfile{}, fmt.Errorf("screening axis is not an Airworthiness input")
	}
	return fillerairworthiness.NormalizeAxisProfile(fillerairworthiness.AxisProfile{
		Axis: axis, EvidenceContract: profile.EvidenceContract, PolicySHA256: profile.PolicySHA256,
		CertificationSHA256: profile.CertificationSHA256, ImplementationSHA256: profile.ImplementationSHA256,
		CertifiedFlags: profile.CertifiedSuitabilityFlags,
	})
}

func airworthinessAxis(axis filler.SegmentScreeningAxis) (fillerairworthiness.Axis, bool) {
	switch axis {
	case filler.ScreenVisualSafety:
		return fillerairworthiness.AxisVisual, true
	case filler.ScreenSpokenSafety:
		return fillerairworthiness.AxisSpoken, true
	case filler.ScreenWrittenSafety:
		return fillerairworthiness.AxisWritten, true
	default:
		return "", false
	}
}

func cloneSuitabilityAxisEvidence(value *fillerairworthiness.AxisEvidence) *fillerairworthiness.AxisEvidence {
	if value == nil {
		return nil
	}
	cloned := *value
	cloned.Profile.CertifiedFlags = slices.Clone(value.Profile.CertifiedFlags)
	cloned.Observations = slices.Clone(value.Observations)
	return &cloned
}

func (e SegmentScreeningAxisEvidence) Result() filler.SegmentScreeningResult {
	return filler.SegmentScreeningResult{Axis: e.Profile.Axis, Outcome: e.Outcome, AuthoritySHA256: e.SHA256, ReasonCode: e.ReasonCode}
}

func SegmentScreeningAxisEvidenceSHA256(evidence SegmentScreeningAxisEvidence) string {
	evidence.SHA256 = ""
	raw, err := json.Marshal(evidence)
	if err != nil {
		return ""
	}
	return filler.ScreeningBytesSHA256(raw)
}
