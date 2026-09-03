package fillersafetycorpus

import (
	"bytes"
	"encoding/json"
	"fmt"
	"reflect"
	"sync"
	"time"

	"github.com/loomarr/loomarr/internal/fillersafety"
	"github.com/loomarr/loomarr/internal/fillersafetycert"
)

// CertificationEvidenceValidator caches shared decoded authorities while still
// validating the provenance and source binding of every certification case.
type CertificationEvidenceValidator struct {
	mu          sync.Mutex
	vctk        map[string]VCTKReleaseAuthority
	vctkCurrent map[string]struct{}
}

func NewCertificationEvidenceValidator() *CertificationEvidenceValidator {
	return &CertificationEvidenceValidator{
		vctk: make(map[string]VCTKReleaseAuthority), vctkCurrent: make(map[string]struct{}),
	}
}

// Validate recognizes every rights/provenance pair supported by the spoken
// certification corpus. Unknown formats and evidence that is current but does
// not govern the exact case source fail closed.
func (v *CertificationEvidenceValidator) Validate(
	rightsRaw, provenanceRaw []byte,
	item fillersafetycert.AuthorityDraftCase,
	at time.Time,
) error {
	if v == nil || len(rightsRaw) == 0 || int64(len(rightsRaw)) > maximumReleaseAuthorityBytes ||
		len(provenanceRaw) == 0 || int64(len(provenanceRaw)) > maximumReleaseAuthorityBytes || at.IsZero() {
		return fmt.Errorf("spoken certification evidence input is invalid")
	}
	if _, err := fillersafety.SourceAuthoritySHA256(item.SourceAuthority); err != nil {
		return fmt.Errorf("spoken certification source authority is invalid")
	}
	var header struct {
		ContractVersion string `json:"contractVersion"`
	}
	if err := json.Unmarshal(rightsRaw, &header); err != nil {
		return fmt.Errorf("spoken certification rights envelope is malformed")
	}
	switch header.ContractVersion {
	case KnownScriptRightsContractVersion:
		return validateKnownScriptCertificationEvidence(rightsRaw, provenanceRaw, item, at)
	case VCTKReleaseContractVersion:
		return v.validateVCTKCertificationEvidence(rightsRaw, provenanceRaw, item, at)
	default:
		return fmt.Errorf("spoken certification rights contract is unsupported")
	}
}

func (v *CertificationEvidenceValidator) validateVCTKCertificationEvidence(
	rightsRaw, provenanceRaw []byte,
	item fillersafetycert.AuthorityDraftCase,
	at time.Time,
) error {
	authority, err := v.vctkAuthority(rightsRaw, at)
	if err != nil {
		return err
	}
	provenance, err := decodeCanonicalCertificationJSON[vctkProvenance](provenanceRaw)
	if err != nil {
		return fmt.Errorf("spoken certification VCTK provenance is malformed or noncanonical")
	}
	source := item.SourceAuthority
	if provenance.SchemaVersion != PreparedCohortSchemaVersion || provenance.ContractVersion != PreparedCohortContractVersion ||
		provenance.PreparedAt.IsZero() || !provenance.PreparedAt.Equal(source.MeasuredAt) || provenance.PreparedAt.After(at) ||
		provenance.ReleaseAuthoritySHA256 != hashBytes(rightsRaw) || !validSHA256(provenance.RecipeSHA256) ||
		!vctkSpeakerID.MatchString(provenance.SpeakerID) || !vctkUtteranceID.MatchString(provenance.UtteranceID) ||
		(provenance.Microphone != "mic1" && provenance.Microphone != "mic2") ||
		!validFileAuthority(provenance.Audio) || !validFileAuthority(provenance.Transcript) ||
		!validFileAuthority(provenance.ScreeningEvidence) || provenance.OutputSHA256 != source.SourceSHA256 ||
		provenance.OutputBytes != source.SourceBytes || provenance.DurationMS != source.DurationMS ||
		item.Label != fillersafetycert.LabelClean || len(item.PositiveIntervals) != 0 {
		return fmt.Errorf("spoken certification VCTK provenance does not bind the case source")
	}
	for _, member := range authority.Members {
		if member.SpeakerID == provenance.SpeakerID && member.UtteranceID == provenance.UtteranceID &&
			member.Microphone == provenance.Microphone {
			if member.Locale != item.Locale || member.Audio != provenance.Audio || member.Transcript != provenance.Transcript ||
				member.ScreeningEvidence != provenance.ScreeningEvidence {
				return fmt.Errorf("spoken certification VCTK provenance does not match its release member")
			}
			return nil
		}
	}
	return fmt.Errorf("spoken certification VCTK provenance names no release member")
}

func (v *CertificationEvidenceValidator) vctkAuthority(raw []byte, at time.Time) (VCTKReleaseAuthority, error) {
	digest := hashBytes(raw)
	v.mu.Lock()
	authority, found := v.vctk[digest]
	v.mu.Unlock()
	if !found {
		decoded, err := decodeCanonicalCertificationJSON[VCTKReleaseAuthority](raw)
		if err != nil {
			return VCTKReleaseAuthority{}, fmt.Errorf("spoken certification VCTK rights envelope is malformed or noncanonical")
		}
		v.mu.Lock()
		if existing, ok := v.vctk[digest]; ok {
			authority = existing
		} else {
			v.vctk[digest] = decoded
			authority = decoded
		}
		v.mu.Unlock()
	}
	currentKey := digest + "\x00" + at.UTC().Format(time.RFC3339Nano)
	v.mu.Lock()
	defer v.mu.Unlock()
	if _, ok := v.vctkCurrent[currentKey]; ok {
		return authority, nil
	}
	if err := validateRelease(authority, at); err != nil {
		return VCTKReleaseAuthority{}, fmt.Errorf("spoken certification VCTK rights are not current")
	}
	v.vctkCurrent[currentKey] = struct{}{}
	return authority, nil
}

func validateKnownScriptCertificationEvidence(
	rightsRaw, provenanceRaw []byte,
	item fillersafetycert.AuthorityDraftCase,
	at time.Time,
) error {
	rights, err := validateKnownScriptRights(rightsRaw, at)
	if err != nil {
		return fmt.Errorf("spoken certification participant rights are not current")
	}
	provenance, err := decodeCanonicalCertificationJSON[knownScriptProvenance](provenanceRaw)
	if err != nil {
		return fmt.Errorf("spoken certification participant provenance is malformed or noncanonical")
	}
	source := item.SourceAuthority
	transformation := provenance.Transformation
	if provenance.SchemaVersion != KnownScriptAuthoritySchemaVersion ||
		provenance.ContractVersion != KnownScriptAuthorityContractVersion || provenance.PreparedAt.IsZero() ||
		!provenance.PreparedAt.Equal(rights.PreparedAt) || !provenance.PreparedAt.Equal(source.MeasuredAt) ||
		provenance.PreparedAt.After(at) || provenance.AuthoritySHA256 != rights.AuthoritySHA256 ||
		provenance.ParticipantID != rights.ParticipantID || !boundedID(provenance.SessionID) ||
		!boundedID(provenance.TakeID) || !boundedID(provenance.ScriptID) || !validFileAuthority(provenance.Script) ||
		!validFileAuthority(provenance.PolicyMapping) || !validFileAuthority(provenance.MasterAudio) ||
		!validFileAuthority(provenance.SelectedAudio) || !boundedID(transformation.RecipeID) ||
		!validSHA256(transformation.RecipeSHA256) || transformation.RenderedAt.IsZero() ||
		transformation.RenderedAt.Before(rights.Consent.SignedAt) || transformation.RenderedAt.After(provenance.PreparedAt) ||
		!validTool(transformation.Tool) || transformation.MasterSHA256 != provenance.MasterAudio.SHA256 ||
		transformation.OutputSHA256 != provenance.SelectedAudio.SHA256 ||
		!reflect.DeepEqual(transformation.Assets, rights.Assets) || provenance.OutputSHA256 != source.SourceSHA256 ||
		provenance.OutputBytes != source.SourceBytes || provenance.DurationMS != source.DurationMS ||
		item.Label != fillersafetycert.LabelPositive || !equalPositiveIntervals(provenance.PositiveIntervals, item.PositiveIntervals) {
		return fmt.Errorf("spoken certification participant provenance does not bind the case source")
	}
	if _, err := validateKnownScriptAssets(transformation.Assets, at); err != nil {
		return fmt.Errorf("spoken certification participant transformation rights are not current")
	}
	return nil
}

func equalPositiveIntervals(actual []PreparedPositiveInterval, expected []fillersafetycert.PositiveInterval) bool {
	if len(actual) != len(expected) {
		return false
	}
	for index := range actual {
		if actual[index].RuleID != expected[index].RuleID || actual[index].StartMS != expected[index].StartMS ||
			actual[index].EndMS != expected[index].EndMS {
			return false
		}
	}
	return true
}

func decodeCanonicalCertificationJSON[T any](raw []byte) (T, error) {
	var zero T
	value, err := decodeKnownScriptJSON[T](raw)
	if err != nil {
		return zero, err
	}
	canonical, err := marshalPrivateJSON(value)
	if err != nil || !bytes.Equal(raw, canonical) {
		return zero, fmt.Errorf("private certification document is not canonical")
	}
	return value, nil
}
