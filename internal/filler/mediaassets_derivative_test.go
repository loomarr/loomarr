package filler

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/loomarr/loomarr/internal/filler/asset"
	"github.com/loomarr/loomarr/internal/filler/split"
	"github.com/loomarr/loomarr/internal/media/tools"
	"github.com/loomarr/loomarr/internal/platform/storagegovernor"
)

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

func TestBuildEvidenceDerivativeUsesMasterAndPublishesHiddenLineage(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "source.mkv")
	if err := os.WriteFile(source, []byte("source master bytes"), 0o644); err != nil {
		t.Fatal(err)
	}
	clipHash, err := asset.ClipID(source)
	if err != nil {
		t.Fatal(err)
	}
	master, err := preserveSourceMaster(context.Background(), root, source, clipHash, SidecarTags{})
	if err != nil {
		t.Fatal(err)
	}
	tool := tools.MediaToolIdentity{Name: "ffmpeg", Version: "ffmpeg version fixture", ExecutableSHA256: string(make([]byte, 64))}
	tool.ExecutableSHA256 = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	lineage, err := buildMediaDerivative(context.Background(), mediaDerivativeRequest{
		ClipDir: root, Source: master, Input: Probed{DurationMs: 30_000, Height: 480},
		Recipe: tools.EvidenceDerivativeRecipe(tools.DefaultMezzanine()), Tool: tool,
		Probe: func(context.Context, string) (Probed, error) { return Probed{DurationMs: 30_000, Height: 480}, nil },
		Verify: func(_ context.Context, _, _ string, durationMs int64, keyframeSeconds int, hadAudio bool, targetLUFS float64) (tools.DerivativeQC, error) {
			return fixtureDerivativeQC(durationMs, keyframeSeconds, hadAudio, targetLUFS), nil
		},
		Transcode: func(_ context.Context, request tools.TranscodeRequest, _ func(int)) (MediaQuality, error) {
			if request.In != filepath.Join(root, filepath.FromSlash(master.Path)) || request.TargetLUFS != 0 || request.Profile.KeyframeSeconds != 1 {
				t.Fatalf("evidence request = %+v", request)
			}
			return MediaQuality{DurationMs: 30_000}, os.WriteFile(request.Out, []byte("high quality evidence bytes"), 0o600)
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if lineage.Asset.Role != MediaAssetEvidence || lineage.InputSHA256 != master.SHA256 || lineage.Recipe.ID != "filler-evidence-v2" {
		t.Fatalf("evidence lineage = %+v", lineage)
	}
	if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(lineage.Asset.Path))); err != nil {
		t.Fatalf("hidden evidence derivative missing: %v", err)
	}
}

func TestBuildEvidenceDerivativeStopsOutputThatExceedsItsLease(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "source.mkv")
	if err := os.WriteFile(source, []byte("source master bytes"), 0o644); err != nil {
		t.Fatal(err)
	}
	clipHash, err := asset.ClipID(source)
	if err != nil {
		t.Fatal(err)
	}
	master, err := preserveSourceMaster(context.Background(), root, source, clipHash, SidecarTags{})
	if err != nil {
		t.Fatal(err)
	}
	governor := storagegovernor.New(intakeCapacityMeter{measurement: storagegovernor.Measurement{
		ID: "clips", TotalBytes: 64 * storagegovernor.GiB, FreeBytes: 60 * storagegovernor.GiB,
	}}, func(storagegovernor.Domain) storagegovernor.Policy {
		return storagegovernor.Policy{SoftBudgetBytes: storagegovernor.GiB}
	})
	lease, decision := governor.Reserve(t.Context(), storagegovernor.Request{
		Path: root, Domain: storagegovernor.DomainFiller, EstimatedBytes: 10,
	})
	if lease == nil || !decision.Allowed {
		t.Fatalf("reserve = %+v", decision)
	}
	defer lease.Release()
	tool := tools.MediaToolIdentity{
		Name: "ffmpeg", Version: "ffmpeg version fixture",
		ExecutableSHA256: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
	}
	_, err = buildMediaDerivative(context.Background(), mediaDerivativeRequest{
		ClipDir: root, Source: master, Input: Probed{DurationMs: 30_000, Height: 480},
		Recipe: tools.EvidenceDerivativeRecipe(tools.DefaultMezzanine()), Tool: tool,
		Probe: func(context.Context, string) (Probed, error) { return Probed{DurationMs: 30_000, Height: 480}, nil },
		Verify: func(_ context.Context, _, _ string, durationMs int64, keyframeSeconds int, hadAudio bool, targetLUFS float64) (tools.DerivativeQC, error) {
			return fixtureDerivativeQC(durationMs, keyframeSeconds, hadAudio, targetLUFS), nil
		},
		Transcode: func(_ context.Context, request tools.TranscodeRequest, _ func(int)) (MediaQuality, error) {
			return MediaQuality{DurationMs: 30_000}, os.WriteFile(request.Out, make([]byte, 11), 0o600)
		},
		Storage: storagegovernor.NewWriteTracker(lease),
	})
	if err == nil || !strings.Contains(err.Error(), string(storagegovernor.ReasonEstimateUnknown)) {
		t.Fatalf("buildMediaDerivative error = %v, want estimate ceiling", err)
	}
	evidenceRoot := filepath.Join(root, split.MediaAssetRootName, split.MediaEvidenceDirName)
	if _, err := os.Stat(evidenceRoot); !os.IsNotExist(err) {
		t.Fatalf("oversized evidence was published: %v", err)
	}
}
