package fillerscreening

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"

	"github.com/loomarr/loomarr/internal/filler"
	"github.com/loomarr/loomarr/internal/mediatools"
)

// Test-only fixtures. They mirror helpers of the same name in internal/filler's tests, which an
// external package cannot import; production code exports nothing for them.

// mediaAssetManifestVersion is filler's manifest version. A wrong value makes the manifest fail
// Validate, which fails the tests that depend on it rather than passing silently.
const mediaAssetManifestVersion = 1

// screeningBlobPath is where FileSegmentScreeningEvidenceRepository (rooted at root) keeps the
// blob of the given kind and digest. Tests that tamper with or remove stored evidence need it; the
// production code derives the same path inside filler.
func screeningBlobPath(root, kind, digest string) string {
	return filepath.Join(root, kind, digest[:2], digest+".json")
}

func screeningSubjectManifest(t *testing.T) filler.MediaAssetManifest {
	t.Helper()
	tool := mediatools.MediaToolIdentity{
		Name: "ffmpeg", Version: "ffmpeg version fixture", ExecutableSHA256: strings.Repeat("9", 64),
	}
	evidenceRecipe := mediatools.EvidenceDerivativeRecipe()
	evidenceRecipeSHA, err := evidenceRecipe.Digest()
	if err != nil {
		t.Fatal(err)
	}
	playbackRecipe := mediatools.PlaybackDerivativeRecipe(mediatools.DefaultMezzanine(), 0)
	playbackRecipeSHA, err := playbackRecipe.Digest()
	if err != nil {
		t.Fatal(err)
	}
	input := filler.Probed{DurationMs: 30_000, Height: 480}
	output := filler.Probed{DurationMs: 30_000, Height: 480}
	quality := filler.MediaQuality{
		EvidenceVersion: mediatools.MediaQualityEvidenceV1,
		Provenance:      mediatools.MediaQualityProvenanceFFmpegDetectors,
		DurationMs:      30_000,
	}
	source := filler.MediaAssetIdentity{
		Role: filler.MediaAssetSourceMaster, SHA256: strings.Repeat("1", 64), Bytes: 1_000,
		ClipHash: strings.Repeat("2", 64), Path: ".loomarr-media/masters/11/11/source.mp4",
	}
	manifest := filler.MediaAssetManifest{
		Version: mediaAssetManifestVersion, SourceMaster: source,
		Evidence: &filler.MediaDerivativeLineage{
			Asset: filler.MediaAssetIdentity{
				Role: filler.MediaAssetEvidence, SHA256: strings.Repeat("3", 64), Bytes: 900,
				ClipHash: strings.Repeat("4", 64), Path: ".loomarr-media/evidence/11/11/recipe/evidence.mp4",
			},
			InputSHA256: source.SHA256, Recipe: evidenceRecipe, RecipeSHA256: evidenceRecipeSHA,
			Tool: tool, DurationMs: 30_000, QC: fixtureDerivativeQC(30_000, evidenceRecipe.KeyframeSeconds, true, 0),
			InputProbe: input, OutputProbe: output, Quality: quality,
		},
		Playback: &filler.MediaDerivativeLineage{
			Asset: filler.MediaAssetIdentity{
				Role: filler.MediaAssetPlayback, SHA256: strings.Repeat("5", 64), Bytes: 800,
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

func fixtureDerivativeQC(durationMs int64, keyframeSeconds int, hadAudio bool, targetLUFS float64) mediatools.DerivativeQC {
	loudness := mediatools.ConditioningLoudness{TruePeak: mediatools.ConditioningTruePeak{State: mediatools.TruePeakUnavailable}}
	if hadAudio {
		integrated := -21.0
		if targetLUFS != 0 {
			integrated = targetLUFS
		}
		loudness = mediatools.ConditioningLoudness{
			IntegratedLUFS: integrated, Available: true,
			TruePeak: mediatools.ConditioningTruePeak{State: mediatools.TruePeakFinite, DBTP: -2},
		}
	}
	gap := int64(keyframeSeconds) * 1000
	return mediatools.DerivativeQC{
		Version: mediatools.DerivativeQCVersion, FastStart: true, CompleteDecode: true, Seekable: true,
		MaxVideoKeyframeGapMs: gap, TerminalKeyframeGapMs: min(gap, durationMs), Loudness: loudness,
	}
}

func completeConditioningMeasurement(lufs float64) mediatools.ConditioningMeasurement {
	available := func(ms int64) mediatools.OptionalMilliseconds {
		return mediatools.OptionalMilliseconds{Milliseconds: ms, Available: true}
	}
	return mediatools.ConditioningMeasurement{
		ContainerDurationMs: 30_000,
		Streams: []mediatools.ConditioningStream{
			{Kind: mediatools.StreamVideo, Index: 0, Start: available(0), Duration: available(30_000),
				Cadence: &mediatools.Rational{Numerator: 30_000, Denominator: 1001}},
			{Kind: mediatools.StreamAudio, Index: 1, Start: available(120), Duration: available(29_880)},
		},
		AVSkew: mediatools.ConditioningSkew{Start: available(120), End: available(0)},
		Loudness: mediatools.ConditioningLoudness{
			IntegratedLUFS: lufs, Available: true,
			TruePeak: mediatools.ConditioningTruePeak{State: mediatools.TruePeakFinite, DBTP: -2.1},
		},
		Quality: filler.MediaQuality{
			EvidenceVersion: mediatools.MediaQualityEvidenceV1,
			Provenance:      mediatools.MediaQualityProvenanceFFmpegDetectors,
			DurationMs:      30_000,
		},
		Cuts: []mediatools.ConditioningCutMeasurement{{Intended: mediatools.Interval{StartMs: 1_000, EndMs: 31_000}, Streams: []mediatools.ConditioningCutStream{
			{Kind: mediatools.StreamVideo, Index: 0, StartError: available(3), EndError: available(-4)},
			{Kind: mediatools.StreamAudio, Index: 1, StartError: available(5), EndError: available(-2)},
		}}},
	}
}

func sparseIdentityCollision() ([]byte, []byte) {
	const window = 64 << 10
	head := bytes.Repeat([]byte{'h'}, window)
	tail := bytes.Repeat([]byte{'t'}, window)
	left := append(append(append(make([]byte, 0, window*3), head...), bytes.Repeat([]byte{'a'}, window)...), tail...)
	right := append(append(append(make([]byte, 0, window*3), head...), bytes.Repeat([]byte{'b'}, window)...), tail...)
	return left, right
}
