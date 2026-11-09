package filler

import (
	"strings"
	"testing"

	"github.com/loomarr/loomarr/internal/filler/asset"
	"github.com/loomarr/loomarr/internal/filler/split"
	"github.com/loomarr/loomarr/internal/media/tools"
)

func screeningSubjectManifest(t *testing.T) asset.MediaAssetManifest {
	t.Helper()
	tool := tools.MediaToolIdentity{
		Name: "ffmpeg", Version: "ffmpeg version fixture", ExecutableSHA256: strings.Repeat("9", 64),
	}
	evidenceRecipe := tools.EvidenceDerivativeRecipe(tools.DefaultMezzanine())
	evidenceRecipeSHA, err := evidenceRecipe.Digest()
	if err != nil {
		t.Fatal(err)
	}
	playbackRecipe := tools.PlaybackDerivativeRecipe(tools.DefaultMezzanine(), 0)
	playbackRecipeSHA, err := playbackRecipe.Digest()
	if err != nil {
		t.Fatal(err)
	}
	input := Probed{DurationMs: 30_000, Height: 480}
	output := Probed{DurationMs: 30_000, Height: 480}
	quality := MediaQuality{
		EvidenceVersion: tools.MediaQualityEvidenceV1,
		Provenance:      tools.MediaQualityProvenanceFFmpegDetectors,
		DurationMs:      30_000,
	}
	source := asset.MediaAssetIdentity{
		Role: asset.MediaAssetSourceMaster, SHA256: strings.Repeat("1", 64), Bytes: 1_000,
		ClipHash: strings.Repeat("2", 64), Path: ".loomarr-media/masters/11/11/source.mp4",
	}
	manifest := asset.MediaAssetManifest{
		Version: asset.MediaAssetManifestVersion, SourceMaster: source,
		Evidence: &asset.MediaDerivativeLineage{
			Asset: asset.MediaAssetIdentity{
				Role: asset.MediaAssetEvidence, SHA256: strings.Repeat("3", 64), Bytes: 900,
				ClipHash: strings.Repeat("4", 64), Path: ".loomarr-media/evidence/11/11/recipe/evidence.mp4",
			},
			InputSHA256: source.SHA256, Recipe: evidenceRecipe, RecipeSHA256: evidenceRecipeSHA,
			Tool: tool, DurationMs: 30_000, QC: fixtureDerivativeQC(30_000, evidenceRecipe.KeyframeSeconds, true, 0),
			InputProbe: input, OutputProbe: output, Quality: quality,
		},
		Playback: &asset.MediaDerivativeLineage{
			Asset: asset.MediaAssetIdentity{
				Role: asset.MediaAssetPlayback, SHA256: strings.Repeat("5", 64), Bytes: 800,
				ClipHash: strings.Repeat("6", 64), Path: "66/66/playback.mp4",
			},
			InputSHA256: source.SHA256, Recipe: playbackRecipe, RecipeSHA256: playbackRecipeSHA,
			Tool: tool, DurationMs: 30_000, QC: fixtureDerivativeQC(30_000, playbackRecipe.KeyframeSeconds, true, 0),
			InputProbe: input, OutputProbe: output, Quality: quality,
		},
	}
	if err := manifest.Validate(); err != nil {
		t.Fatal(err)
	}
	return manifest
}

func TestSegmentScreeningSubjectBindsRenderedChildAndExactParentInterval(t *testing.T) {
	manifest := screeningSubjectManifest(t)
	lineage := asset.ConditioningLineage{
		ChildHash: manifest.SourceMaster.ClipHash, ParentHash: strings.Repeat("7", 64),
		ParentAssetRole: string(split.SourceEvidence), ParentAssetSHA256: strings.Repeat("8", 64),
		StructureDecisionSHA256: strings.Repeat("a", 64), StructureRole: split.RoleCommercial,
		IntendedStartMs: 1_000, IntendedEndMs: 31_000,
	}
	measurement := completeConditioningMeasurement(-23)
	conditioning := asset.ConditioningEvidence{
		BeforeRewriteHash: lineage.ChildHash, AfterRewriteHash: manifest.Playback.Asset.ClipHash,
		BeforeRewrite: measurement, AfterRewrite: measurement,
		DerivedParentEdgesAfterRewrite: measurement.Cuts[0],
	}
	subject, err := asset.NewSegmentScreeningSubject(manifest.Playback.Asset.ClipHash, asset.SidecarTags{
		SourceID: "archive:commercials", AcquisitionID: "acq-17", MediaAssets: &manifest,
		ConditioningLineage: &lineage, Conditioning: &conditioning,
	})
	if err != nil {
		t.Fatal(err)
	}
	if asset.ValidateSegmentScreeningSubject(subject) != nil || subject.Lineage == nil ||
		subject.Lineage.IntendedStartMs != 1_000 || subject.Lineage.IntendedEndMs != 31_000 ||
		subject.CatalogHash != manifest.Playback.Asset.ClipHash || subject.ConditioningSHA256 != asset.ConditioningEvidenceSHA256(conditioning) {
		t.Fatalf("subject = %+v", subject)
	}
}

func TestSegmentScreeningSubjectRejectsIncompleteOrDriftedChildIdentity(t *testing.T) {
	manifest := screeningSubjectManifest(t)
	standalone, err := asset.NewSegmentScreeningSubject(manifest.Playback.Asset.ClipHash, asset.SidecarTags{MediaAssets: &manifest})
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name   string
		mutate func(*asset.SegmentScreeningSubject)
	}{
		{name: "playback", mutate: func(s *asset.SegmentScreeningSubject) { s.PlaybackSHA256 = "" }},
		{name: "manifest", mutate: func(s *asset.SegmentScreeningSubject) { s.MediaManifestSHA256 = "" }},
		{name: "source id", mutate: func(s *asset.SegmentScreeningSubject) { s.SourceID = " padded " }},
		{name: "conditioning without lineage", mutate: func(s *asset.SegmentScreeningSubject) { s.ConditioningSHA256 = strings.Repeat("e", 64) }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			candidate := standalone
			test.mutate(&candidate)
			candidate.SHA256 = asset.SegmentScreeningSubjectSHA256(candidate)
			if err := asset.ValidateSegmentScreeningSubject(candidate); err == nil {
				t.Fatal("drifted subject was accepted")
			}
		})
	}
}

func TestSegmentScreeningSubjectRequiresMeasuredDerivativeQuality(t *testing.T) {
	manifest := screeningSubjectManifest(t)
	manifest.Playback.Quality = MediaQuality{}
	if _, err := asset.NewSegmentScreeningSubject(manifest.Playback.Asset.ClipHash, asset.SidecarTags{MediaAssets: &manifest}); err == nil {
		t.Fatal("subject accepted playback without measured media quality")
	}
}

func TestVerifySegmentScreeningSubjectRejectsCurrentArtifactDrift(t *testing.T) {
	manifest := screeningSubjectManifest(t)
	tags := asset.SidecarTags{MediaAssets: &manifest}
	subject, err := asset.NewSegmentScreeningSubject(manifest.Playback.Asset.ClipHash, tags)
	if err != nil {
		t.Fatal(err)
	}
	if err := asset.VerifySegmentScreeningSubject(subject, manifest.Playback.Asset.ClipHash, tags); err != nil {
		t.Fatal(err)
	}
	drifted := manifest
	playback := *manifest.Playback
	playback.Asset.SHA256 = strings.Repeat("f", 64)
	drifted.Playback = &playback
	if err := asset.VerifySegmentScreeningSubject(subject, playback.Asset.ClipHash, asset.SidecarTags{MediaAssets: &drifted}); err == nil {
		t.Fatal("current playback drift was accepted")
	}
}

func TestMediaAssetManifestAuthorityIdentityIgnoresOnlyPaths(t *testing.T) {
	manifest := screeningSubjectManifest(t)
	want := asset.MediaAssetManifestAuthoritySHA256(manifest)
	manifest.SourceMaster.Path = ".loomarr-media/masters/11/11/relocated.mp4"
	manifest.Evidence.Asset.Path = ".loomarr-media/evidence/11/11/recipe/relocated.mp4"
	manifest.Playback.Asset.Path = "66/66/relocated.mp4"
	if got := asset.MediaAssetManifestAuthoritySHA256(manifest); got != want {
		t.Fatalf("path relocation changed media authority: got %s want %s", got, want)
	}
	manifest.Playback.Asset.SHA256 = strings.Repeat("f", 64)
	if got := asset.MediaAssetManifestAuthoritySHA256(manifest); got == want {
		t.Fatal("playback byte drift did not change media authority")
	}
}
