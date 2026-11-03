package mediatools

import (
	"testing"

	"github.com/loomarr/loomarr/internal/encode"
)

func TestDerivativeRecipesAreStableAndRoleSeparated(t *testing.T) {
	evidence := EvidenceDerivativeRecipe(DefaultMezzanine())
	playback := PlaybackDerivativeRecipe(DefaultMezzanine(), -23)
	evidenceDigest, err := evidence.Digest()
	if err != nil {
		t.Fatal(err)
	}
	playbackDigest, err := playback.Digest()
	if err != nil {
		t.Fatal(err)
	}
	if evidenceDigest == playbackDigest || evidence.Role == playback.Role {
		t.Fatalf("evidence and playback recipes collapsed: %s / %s", evidenceDigest, playbackDigest)
	}
	if evidence.TargetLUFS != 0 || playback.TargetLUFS != -23 {
		t.Fatalf("loudness policy crossed roles: evidence=%v playback=%v", evidence.TargetLUFS, playback.TargetLUFS)
	}
	// One measured level for both roles (#2121): the evidence picture is the one that airs.
	if evidence.CRF != playback.CRF || evidence.VideoEncoder != playback.VideoEncoder {
		t.Fatalf("evidence %s/%d and playback %s/%d encode differently", evidence.VideoEncoder, evidence.CRF, playback.VideoEncoder, playback.CRF)
	}

	changed := evidence
	changed.CRF++
	changedDigest, err := changed.Digest()
	if err != nil {
		t.Fatal(err)
	}
	if changedDigest == evidenceDigest {
		t.Fatal("semantic recipe change retained the same identity")
	}
}

// A hardware family's recipe names its own encoder and carries no x264 preset; the libx264 records
// written before #2121 (CRF 14 `slow`) stay valid, so no stored clip's digest breaks.
func TestDerivativeRecipeAcceptsEveryFamilyAndPriorRecords(t *testing.T) {
	nvenc := EvidenceDerivativeRecipe(FileMezzanine(encode.HostProfile{Family: encode.FamilyNVENC}))
	if nvenc.VideoEncoder != "h264_nvenc" || nvenc.Preset != "" {
		t.Fatalf("NVENC recipe %s preset %q", nvenc.VideoEncoder, nvenc.Preset)
	}
	if got := nvenc.Profile().Family; got != encode.FamilyNVENC {
		t.Fatalf("NVENC recipe profile family %q", got)
	}
	prior := EvidenceDerivativeRecipe(DefaultMezzanine())
	prior.ID, prior.CRF, prior.Preset = "filler-evidence-v1", 14, "slow"
	for name, r := range map[string]DerivativeRecipe{"nvenc": nvenc, "prior libx264": prior} {
		if err := r.Validate(); err != nil {
			t.Errorf("%s: %v", name, err)
		}
	}
	for name, mutate := range map[string]func(*DerivativeRecipe){
		"hardware with a preset": func(r *DerivativeRecipe) { r.VideoEncoder, r.Preset = "h264_nvenc", "slow" },
		"x264 without a preset":  func(r *DerivativeRecipe) { r.Preset = "" },
		"unknown encoder":        func(r *DerivativeRecipe) { r.VideoEncoder = "h264_qsv" },
	} {
		r := prior
		mutate(&r)
		if r.Validate() == nil {
			t.Errorf("%s: accepted", name)
		}
	}
}

func TestEvidenceRecipeRefusesPlaybackLoudness(t *testing.T) {
	recipe := EvidenceDerivativeRecipe(DefaultMezzanine())
	recipe.TargetLUFS = -23
	if err := recipe.Validate(); err == nil {
		t.Fatal("evidence recipe accepted a loudness rewrite")
	}
}
