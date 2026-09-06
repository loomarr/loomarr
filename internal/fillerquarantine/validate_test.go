package fillerquarantine

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/fillercorpus"
	"github.com/loomarr/loomarr/internal/fillerreview"
	"github.com/loomarr/loomarr/internal/mediatools"
)

func TestValidateAcceptsHeldIncompleteExposureAndRejectsAuthorityExpansion(t *testing.T) {
	report := validReportFixture()
	if err := Validate(report); err != nil {
		t.Fatal(err)
	}
	report.Authority.Training = true
	if err := Validate(report); err == nil {
		t.Fatal("training authority accepted")
	}
}

func TestValidateAcceptsProbeGroundedMissingVideoHold(t *testing.T) {
	report := validReportFixture()
	report.Cases[0].Media = MediaEvidence{DurationMS: 1_000, HasAudio: true}
	report.Cases[0].Fingerprint = FingerprintEvidence{}
	report.Cases[0].HoldReasons = []string{"fingerprint_unusable", "missing_video", "prior_perceptual_exposure_incomplete"}
	if err := Validate(report); err != nil {
		t.Fatal(err)
	}
}

func TestValidateRecomputesHoldReasonsFromEvidence(t *testing.T) {
	report := validReportFixture()
	report.Cases[0].HoldReasons = nil
	report.Cases[0].Disposition = DispositionEligibleForRightsReview
	report.Summary.Held = 0
	report.Summary.EligibleForRightsReview = 1
	if err := Validate(report); err == nil {
		t.Fatal("erased prior-exposure hold accepted")
	}
}

func TestComparePriorExposureHoldsAvailableIncomparablePriorSource(t *testing.T) {
	digest := strings.Repeat("a", 64)
	report := Report{
		PriorSources: []PriorSource{{SourceID: "prior", SourcePath: "prior.mp4", SourceSHA256: digest, DurationMS: 1_000, Available: true}},
		Cases:        []Case{{CaseID: "case"}},
	}
	prior := fingerprint{frames: []uint64{1}, audio: []uint32{1}}
	if err := comparePriorExposure(context.Background(), &report, map[string]fingerprint{"case": {frames: []uint64{1, 2, 3}, audio: []uint32{1, 2, 3}}}, map[string]fingerprint{digest: prior}); err != nil {
		t.Fatal(err)
	}
	if !slicesContains(report.Cases[0].HoldReasons, "prior_perceptual_exposure_incomplete") {
		t.Fatalf("available incomparable prior source did not hold candidate: %+v", report.Cases[0])
	}
}

func TestValidateRejectsEligibleCaseWithAvailableIncomparablePriorSource(t *testing.T) {
	report := validReportFixture()
	report.Summary = Summary{Cases: 1, EligibleForRightsReview: 1, PriorSources: 1}
	report.PriorSources[0].Available = true
	report.PriorSources[0].Fingerprint = FingerprintEvidence{FrameCount: 1, FrameSHA256: strings.Repeat("a", 64), AudioBinCount: 1, AudioRMSSHA256: strings.Repeat("a", 64)}
	report.Cases[0].Disposition = DispositionEligibleForRightsReview
	report.Cases[0].HoldReasons = nil
	report.Comparisons = []Comparison{{Scope: ComparisonPrior, CaseA: "case", CaseB: "prior"}}
	if err := Validate(report); err == nil {
		t.Fatal("eligible case with an available incomparable prior source was accepted")
	}
}

func TestValidateAcceptsEligibleCaseWithAvailableVisuallyComparablePriorSource(t *testing.T) {
	report := validReportFixture()
	report.Summary = Summary{Cases: 1, EligibleForRightsReview: 1, PriorSources: 1}
	report.PriorSources[0].Available = true
	report.PriorSources[0].Fingerprint = FingerprintEvidence{FrameCount: 1, FrameSHA256: strings.Repeat("a", 64), AudioBinCount: 1, AudioRMSSHA256: strings.Repeat("a", 64), VisualComparable: true}
	report.Cases[0].Disposition = DispositionEligibleForRightsReview
	report.Cases[0].HoldReasons = nil
	report.Comparisons = []Comparison{{Scope: ComparisonPrior, CaseA: "case", CaseB: "prior"}}
	if err := Validate(report); err != nil {
		t.Fatal(err)
	}
}

func TestValidateRejectsMissingMediaWallTimeCeiling(t *testing.T) {
	report := validReportFixture()
	report.Ceilings.MaxMediaWallTimeMS = 0
	if err := Validate(report); err == nil {
		t.Fatal("report without media wall-time ceiling accepted")
	}
}

func TestHashFileHonorsCancellation(t *testing.T) {
	path := filepath.Join(t.TempDir(), "source.mp4")
	if err := os.WriteFile(path, []byte("fixture"), 0o600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, _, err := hashFile(ctx, path); err == nil {
		t.Fatal("hashing ignored cancellation")
	}
}

func TestResolveBeneathRejectsLexicalAndSymlinkEscape(t *testing.T) {
	root := t.TempDir()
	if _, err := resolveBeneath(root, "../escape.mp4"); err == nil {
		t.Fatal("lexical escape accepted")
	}
	outside := filepath.Join(t.TempDir(), "outside.mp4")
	if err := os.WriteFile(outside, []byte("outside"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, "inside.mp4")); err != nil {
		t.Fatal(err)
	}
	if _, err := resolveBeneath(root, "inside.mp4"); err == nil {
		t.Fatal("symlink escape accepted")
	}
}

func TestTechnicalHoldReasonsSeparatePlaybackAndSignalFailures(t *testing.T) {
	quality := mediatools.MediaQuality{DurationMs: 1_000, Black: []mediatools.Interval{{StartMs: 0, EndMs: 1_000}}}
	reasons := technicalHoldReasons(structuredRepresentation(), mediatools.Probed{DurationMs: 1_000, NoVideo: true, Silent: true}, quality, fingerprint{})
	for _, expected := range []string{"fingerprint_unusable", "missing_audio", "missing_video", "mostly_black"} {
		if !slicesContains(reasons, expected) {
			t.Fatalf("reasons=%v missing %q", reasons, expected)
		}
	}
}

func structuredRepresentation() fillercorpus.InventoryRepresentation {
	return fillercorpus.InventoryRepresentation{}
}

func validReportFixture() Report {
	digest := strings.Repeat("a", 64)
	tool := fillerreview.TemporalTruthToolIdentity{Path: "/fixture/tool", Version: "fixture", BinarySHA256: digest}
	return Report{
		SchemaVersion: SchemaVersion, ContractVersion: ContractVersion, GeneratedAt: time.Date(2026, 9, 5, 12, 0, 0, 0, time.UTC),
		Inputs:     InputIdentity{InventorySHA256: digest, DownloadLedgerSHA256: digest, PriorPublicManifestSHA256: digest, PriorAuthoritySHA256: digest},
		MediaTools: fillerreview.TemporalTruthMediaIdentity{FFmpeg: tool, FFprobe: tool}, Ceilings: Ceilings{MaxMediaWallTimeMS: 1_000}, Algorithm: "fixture",
		Summary:      Summary{Cases: 1, Held: 1, PriorSources: 1, UnavailablePriorSources: 1},
		PriorSources: []PriorSource{{SourceID: "prior", SourcePath: "prior.mp4", SourceSHA256: digest, DurationMS: 1_000}},
		Cases: []Case{{
			CaseID: "case", LocalFile: "case.mp4", ContentSHA256: digest, Bytes: 1,
			ExpectedMedia: MediaExpectation{Bytes: 1},
			Media: MediaEvidence{DurationMS: 1_000, Width: 640, Height: 480, HasVideo: true, HasAudio: true, Quality: mediatools.MediaQuality{
				EvidenceVersion: mediatools.MediaQualityEvidenceV1, Provenance: mediatools.MediaQualityProvenanceFFmpegDetectors, DurationMs: 1_000,
			}},
			Fingerprint: FingerprintEvidence{FrameCount: 1, FrameSHA256: digest, AudioBinCount: 1, AudioRMSSHA256: digest, VisualComparable: true},
			Disposition: DispositionHold, HoldReasons: []string{"prior_perceptual_exposure_incomplete"},
		}},
		Authority: AuthorityDisposition{CopyAndStorage: true, LocalTechnicalInspection: true},
	}
}

func slicesContains(values []string, expected string) bool {
	for _, value := range values {
		if value == expected {
			return true
		}
	}
	return false
}
