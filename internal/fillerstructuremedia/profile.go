// Package fillerstructuremedia owns the exact media contract shared by
// complete-timeline structure qualification and production assessment.
package fillerstructuremedia

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"

	"github.com/loomarr/loomarr/internal/encode"
	"github.com/loomarr/loomarr/internal/fillerstructure"
)

const (
	ProfileSchemaVersion   = 3
	ProfileContractVersion = "filler-structure-assessment-media-v3"
	MaximumVideoBytes      = fillerstructure.AssessmentMediaMaximumBytes
)

// Profile is the path-free identity of one assessment-media recipe. Media-tool binary identities
// are bound separately by the caller.
type Profile struct {
	SchemaVersion   int    `json:"schemaVersion"`
	ContractVersion string `json:"contractVersion"`
	// Family is the encoder family the cut runs on (#2121). A certification covers one profile, so
	// a cut on another family is a different profile that needs its own certification.
	Family                 encode.Family `json:"family"`
	MIMEType               string        `json:"mimeType"`
	Container              string        `json:"container"`
	VideoCodec             string        `json:"videoCodec"`
	PixelFormat            string        `json:"pixelFormat"`
	Width                  int           `json:"width"`
	Height                 int           `json:"height"`
	FrameRate              string        `json:"frameRate"`
	SampleAspectRatio      string        `json:"sampleAspectRatio"`
	AudioCodec             string        `json:"audioCodec"`
	AudioSampleRate        int           `json:"audioSampleRate"`
	AudioChannels          int           `json:"audioChannels"`
	VideoTrackTimescale    int           `json:"videoTrackTimescale"`
	MaximumVideoBytes      int64         `json:"maximumVideoBytes"`
	MaximumTimelineDriftMS int64         `json:"maximumTimelineDriftMs"`
	PartRecipeSHA256       string        `json:"partRecipeSha256"`
	ConcatRecipeSHA256     string        `json:"concatRecipeSha256"`
	SHA256                 string        `json:"sha256"`
}

// cutLevel is the family's constant-quality level for a cut: the level whose picture matches the
// software cut's, x264 `medium` CRF 23, measured as VMAF against the same scaled source
// (docs/design/filler-structure.md). ok is false for a family not measured for cuts.
func cutLevel(f encode.Family) (level int, ok bool) {
	switch f {
	case encode.FamilySoftware:
		return 23, true
	case encode.FamilyNVENC:
		return 29, true
	}
	return 0, false
}

// cutFamilies is every family with a measured cut level, software first.
var cutFamilies = []encode.Family{encode.FamilySoftware, encode.FamilyNVENC}

// CutFamily is the family this host cuts on: its own when that family has a measured cut level,
// else software.
func CutFamily(host encode.HostProfile) encode.Family {
	if _, ok := cutLevel(host.Family); ok {
		return host.Family
	}
	return encode.FamilySoftware
}

// ProfileFor returns a fresh profile for the family, so callers cannot mutate shared process state
// and silently change the recipe identity. ok is false for a family with no measured cut level.
func ProfileFor(family encode.Family) (Profile, bool) {
	if _, ok := cutLevel(family); !ok {
		return Profile{}, false
	}
	profile := Profile{
		SchemaVersion: ProfileSchemaVersion, ContractVersion: ProfileContractVersion, Family: family,
		MIMEType: "video/mp4", Container: "mp4", VideoCodec: "h264", PixelFormat: "yuv420p",
		Width: 960, Height: 720, FrameRate: "30/1", SampleAspectRatio: "1/1",
		AudioCodec: "aac", AudioSampleRate: 48_000, AudioChannels: 2,
		VideoTrackTimescale: 90_000, MaximumVideoBytes: MaximumVideoBytes,
		MaximumTimelineDriftMS: fillerstructure.AssessmentMediaMaximumTimelineDriftMS,
		PartRecipeSHA256:       recipeSHA256(partArgumentTemplate(family)),
		ConcatRecipeSHA256:     recipeSHA256(concatArgumentTemplate()),
	}
	profile.SHA256 = ProfileSHA256(profile)
	return profile, true
}

// SoftwareProfile is the cut on the CPU, which every host can run.
func SoftwareProfile() Profile {
	profile, _ := ProfileFor(encode.FamilySoftware)
	return profile
}

// ProfileBySHA256 resolves a pinned profile digest, as an authority or a plan records it, back to
// the profile it names. ok is false for a digest no supported recipe produces.
func ProfileBySHA256(digest string) (Profile, bool) {
	for _, family := range cutFamilies {
		if profile, _ := ProfileFor(family); digest != "" && profile.SHA256 == digest {
			return profile, true
		}
	}
	return Profile{}, false
}

// Supported reports whether profile is exactly one of the supported recipes, digest included.
func Supported(profile Profile) bool {
	want, ok := ProfileFor(profile.Family)
	return ok && profile == want
}

func ProfileSHA256(profile Profile) string {
	profile.SHA256 = ""
	raw, err := json.Marshal(profile)
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

func recipeSHA256(arguments []string) string {
	raw, err := json.Marshal(arguments)
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}
