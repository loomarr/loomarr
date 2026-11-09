package defectfloor

import (
	"os"
	"path"
	"path/filepath"
	"strings"
	"testing"

	"github.com/loomarr/loomarr/internal/filler/asset"
	"github.com/loomarr/loomarr/internal/filler/cliplibrary"
	"github.com/loomarr/loomarr/internal/filler/split"
	"github.com/loomarr/loomarr/internal/media/tools"
)

// Test-only fixtures. They mirror helpers of the same name in internal/filler's tests, which an
// external package cannot import; production code exports nothing for them.

// mediaAssetManifestVersion is filler's manifest version. A wrong value makes filler read the
// sidecar as invalid, which fails the tests that depend on it rather than passing silently.
const mediaAssetManifestVersion = 1

// writeContentAddressedClip stores body at its content-addressed catalog path under dir and
// returns the clip hash.
func writeContentAddressedClip(t *testing.T, dir string, body []byte, ext string) string {
	t.Helper()
	tmp := filepath.Join(dir, "source"+ext)
	if err := os.WriteFile(tmp, body, 0o644); err != nil {
		t.Fatal(err)
	}
	hash, err := asset.ClipID(tmp)
	if err != nil {
		t.Fatal(err)
	}
	dst := filepath.Join(dir, cliplibrary.ClipRelPath(hash, ext))
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(tmp, dst); err != nil {
		t.Fatal(err)
	}
	return hash
}

// preserveSourceMaster files a copy of sourcePath in the hidden master tree of clipDir, where
// filler's pipeline keeps it, and returns the identity a sidecar names it by. The real routine is
// unexported; the defect floor only needs the file to exist at a valid path with a valid identity.
func preserveSourceMaster(clipDir, sourcePath, clipHash string) (asset.Identity, error) {
	digest, size, err := asset.FileSHA256(sourcePath)
	if err != nil {
		return asset.Identity{}, err
	}
	rel := path.Join(split.MediaAssetRootName, "masters", digest[:2], digest[2:4], digest+strings.ToLower(filepath.Ext(sourcePath)))
	full := filepath.Join(clipDir, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		return asset.Identity{}, err
	}
	body, err := os.ReadFile(sourcePath)
	if err != nil {
		return asset.Identity{}, err
	}
	if err := os.WriteFile(full, body, 0o644); err != nil {
		return asset.Identity{}, err
	}
	return asset.Identity{
		Role: asset.RoleSourceMaster, SHA256: digest, Bytes: size, ClipHash: clipHash, Path: rel,
	}, nil
}

func fixtureDerivativeQC(durationMs int64, keyframeSeconds int, hadAudio bool, targetLUFS float64) tools.DerivativeQC {
	loudness := tools.ConditioningLoudness{TruePeak: tools.ConditioningTruePeak{State: tools.TruePeakUnavailable}}
	if hadAudio {
		integrated := -21.0
		if targetLUFS != 0 {
			integrated = targetLUFS
		}
		loudness = tools.ConditioningLoudness{
			IntegratedLUFS: integrated, Available: true,
			TruePeak: tools.ConditioningTruePeak{State: tools.TruePeakFinite, DBTP: -2},
		}
	}
	gap := int64(keyframeSeconds) * 1000
	return tools.DerivativeQC{
		Version: tools.DerivativeQCVersion, FastStart: true, CompleteDecode: true, Seekable: true,
		MaxVideoKeyframeGapMs: gap, TerminalKeyframeGapMs: min(gap, durationMs), Loudness: loudness,
	}
}

// screeningSubjectManifest is a complete media-asset manifest (source master, evidence and
// playback lineage). filler's sidecar reader validates it, so an incomplete one reads as invalid.
func screeningSubjectManifest(t *testing.T) asset.Manifest {
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
	input := tools.Probed{DurationMs: 30_000, Height: 480}
	output := tools.Probed{DurationMs: 30_000, Height: 480}
	quality := tools.MediaQuality{
		EvidenceVersion: tools.MediaQualityEvidenceV1,
		Provenance:      tools.MediaQualityProvenanceFFmpegDetectors,
		DurationMs:      30_000,
	}
	source := asset.Identity{
		Role: asset.RoleSourceMaster, SHA256: strings.Repeat("1", 64), Bytes: 1_000,
		ClipHash: strings.Repeat("2", 64), Path: ".loomarr-media/masters/11/11/source.mp4",
	}
	return asset.Manifest{
		Version: mediaAssetManifestVersion, SourceMaster: source,
		Evidence: &asset.DerivativeLineage{
			Asset: asset.Identity{
				Role: asset.RoleEvidence, SHA256: strings.Repeat("3", 64), Bytes: 900,
				ClipHash: strings.Repeat("4", 64), Path: ".loomarr-media/evidence/11/11/recipe/evidence.mp4",
			},
			InputSHA256: source.SHA256, Recipe: evidenceRecipe, RecipeSHA256: evidenceRecipeSHA,
			Tool: tool, DurationMs: 30_000, QC: fixtureDerivativeQC(30_000, evidenceRecipe.KeyframeSeconds, true, 0),
			InputProbe: input, OutputProbe: output, Quality: quality,
		},
		Playback: &asset.DerivativeLineage{
			Asset: asset.Identity{
				Role: asset.RolePlayback, SHA256: strings.Repeat("5", 64), Bytes: 800,
				ClipHash: strings.Repeat("6", 64), Path: "66/66/playback.mp4",
			},
			InputSHA256: source.SHA256, Recipe: playbackRecipe, RecipeSHA256: playbackRecipeSHA,
			Tool: tool, DurationMs: 30_000, QC: fixtureDerivativeQC(30_000, playbackRecipe.KeyframeSeconds, true, 0),
			InputProbe: input, OutputProbe: output, Quality: quality,
		},
	}
}
