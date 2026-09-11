package fillercorpus

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"
)

const (
	SoundtrackPresentExpected     = "present_expected"
	SoundtrackIntentionallySilent = "intentionally_silent"
	SoundtrackUnknown             = "unknown"

	SoundtrackEvidenceFirstPartyMetadata = "first_party_metadata"
	SoundtrackEvidenceEncodedStream      = "encoded_stream_metadata"
	SoundtrackEvidenceReviewedManifest   = "reviewed_source_manifest"

	QuarantinePurposeTemporalStructureReplacement = "temporal_structure_replacement_v1"

	HoldReasonSoundtrackIntentionallySilent = "soundtrack_intentionally_silent"
	HoldReasonSoundtrackUnknown             = "soundtrack_unknown"
)

// SoundtrackExpectation binds one pre-download soundtrack claim to frozen
// evidence and to the exact representation the claim describes.
type SoundtrackExpectation struct {
	Status               string `json:"status"`
	EvidenceKind         string `json:"evidenceKind"`
	EvidenceSHA256       string `json:"evidenceSha256"`
	EvidenceLocator      string `json:"evidenceLocator"`
	RepresentationSHA256 string `json:"representationSha256"`
}

// BindRepresentationSoundtrack binds a discovery-lane soundtrack claim to
// the exact remote representation that the lane can promote.
func BindRepresentationSoundtrack(representation Representation, status, evidenceKind, evidenceSHA256, evidenceLocator string) SoundtrackExpectation {
	return soundtrackExpectation(status, evidenceKind, evidenceSHA256, evidenceLocator, representationIdentitySHA256(representationIdentity{
		Transport: TransportHTTPS, Name: representation.Name, URL: representation.URL, MIMEType: representation.MIMEType, Bytes: representation.Bytes,
		SHA1: representation.SHA1, MD5: representation.MD5,
	}))
}

// BindInventorySoundtrack binds a soundtrack claim to one exact inventory
// representation, including local and post-download identity fields.
func BindInventorySoundtrack(representation InventoryRepresentation, status, evidenceKind, evidenceSHA256, evidenceLocator string) SoundtrackExpectation {
	return soundtrackExpectation(status, evidenceKind, evidenceSHA256, evidenceLocator, representationIdentitySHA256(representationIdentity{
		Transport: representation.Transport, Name: representation.Name, URL: representation.URL, Path: representation.Path,
		MIMEType: representation.MIMEType, Origin: representation.Origin, Bytes: representation.Bytes,
		SHA256: representation.SHA256, SHA1: representation.SHA1, MD5: representation.MD5,
		DurationMS: representation.DurationMS, Width: representation.Width, Height: representation.Height,
	}))
}

func soundtrackExpectation(status, evidenceKind, evidenceSHA256, evidenceLocator, representationSHA256 string) SoundtrackExpectation {
	return SoundtrackExpectation{
		Status: status, EvidenceKind: evidenceKind, EvidenceSHA256: evidenceSHA256,
		EvidenceLocator: evidenceLocator, RepresentationSHA256: representationSHA256,
	}
}

func validateRepresentationSoundtrack(representation InventoryRepresentation) bool {
	expectation := representation.Soundtrack
	if !knownSoundtrackStatus(expectation.Status) || !knownSoundtrackEvidenceKind(expectation.EvidenceKind) ||
		!digest(expectation.EvidenceSHA256, 64) || strings.TrimSpace(expectation.EvidenceLocator) == "" ||
		len(expectation.EvidenceLocator) > 512 {
		return false
	}
	expected := BindInventorySoundtrack(representation, expectation.Status, expectation.EvidenceKind, expectation.EvidenceSHA256, expectation.EvidenceLocator)
	return expectation.RepresentationSHA256 == expected.RepresentationSHA256
}

func validateLaneSoundtrack(representation Representation) bool {
	expectation := representation.Soundtrack
	if !knownSoundtrackStatus(expectation.Status) || !knownSoundtrackEvidenceKind(expectation.EvidenceKind) ||
		!digest(expectation.EvidenceSHA256, 64) || strings.TrimSpace(expectation.EvidenceLocator) == "" || len(expectation.EvidenceLocator) > 512 {
		return false
	}
	expected := BindRepresentationSoundtrack(representation, expectation.Status, expectation.EvidenceKind, expectation.EvidenceSHA256, expectation.EvidenceLocator)
	return expectation.RepresentationSHA256 == expected.RepresentationSHA256
}

func soundtrackEvidenceBound(item InventoryCase) bool {
	if item.Representation.Soundtrack.EvidenceSHA256 == item.MetadataSHA256 {
		return true
	}
	for _, evidence := range item.Evidence {
		if evidence.SHA256 == item.Representation.Soundtrack.EvidenceSHA256 {
			return true
		}
	}
	return false
}

func knownSoundtrackStatus(value string) bool {
	switch value {
	case SoundtrackPresentExpected, SoundtrackIntentionallySilent, SoundtrackUnknown:
		return true
	default:
		return false
	}
}

// KnownSoundtrackStatus reports whether value is in the closed soundtrack
// expectation vocabulary.
func KnownSoundtrackStatus(value string) bool { return knownSoundtrackStatus(value) }

// TemporalReplacementSoundtrackHoldReason returns the stable pre-download
// hold for a valid soundtrack expectation. An empty result means that the
// representation may enter the temporal-structure replacement plan.
func TemporalReplacementSoundtrackHoldReason(status string) string {
	switch status {
	case SoundtrackPresentExpected:
		return ""
	case SoundtrackIntentionallySilent:
		return HoldReasonSoundtrackIntentionallySilent
	case SoundtrackUnknown:
		return HoldReasonSoundtrackUnknown
	default:
		return "soundtrack_expectation_invalid"
	}
}

func knownSoundtrackEvidenceKind(value string) bool {
	switch value {
	case SoundtrackEvidenceFirstPartyMetadata, SoundtrackEvidenceEncodedStream, SoundtrackEvidenceReviewedManifest:
		return true
	default:
		return false
	}
}

type representationIdentity struct {
	Transport  string `json:"transport,omitempty"`
	Name       string `json:"name"`
	URL        string `json:"url,omitempty"`
	Path       string `json:"path,omitempty"`
	MIMEType   string `json:"mimeType"`
	Origin     string `json:"origin,omitempty"`
	Bytes      int64  `json:"bytes"`
	SHA256     string `json:"sha256,omitempty"`
	SHA1       string `json:"sha1,omitempty"`
	MD5        string `json:"md5,omitempty"`
	DurationMS int64  `json:"durationMs,omitempty"`
	Width      int    `json:"width,omitempty"`
	Height     int    `json:"height,omitempty"`
}

func representationIdentitySHA256(value representationIdentity) string {
	raw, err := json.Marshal(value)
	if err != nil {
		panic(err)
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}
