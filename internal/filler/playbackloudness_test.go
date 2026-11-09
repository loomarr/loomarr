package filler

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/loomarr/loomarr/internal/filler/asset"
)

func TestPlaybackLoudness_ReadsTheIngestMeasurementFromTheSidecar(t *testing.T) {
	manifest := screeningSubjectManifest(t)
	manifest.Playback.QC = fixtureDerivativeQC(30_000, manifest.Playback.Recipe.KeyframeSeconds, true, -23.4)
	media := filepath.Join(t.TempDir(), "playback.mp4")
	if err := os.WriteFile(media, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := asset.WriteSidecarTags(media, asset.SidecarTags{MediaAssets: &manifest}, false); err != nil {
		t.Fatal(err)
	}

	got, peak, ok := asset.PlaybackLoudness(media)
	if !ok || got != -23.4 {
		t.Fatalf("PlaybackLoudness = %v, %v; want -23.4, true", got, ok)
	}
	// The peak rides along so a boost can stop short of clipping (#2032).
	if peak == nil || *peak != -2 {
		t.Fatalf("true peak = %v, want -2 dBTP", peak)
	}
}

// A clip with no sidecar, or one whose audio was never measured, reports NOT measured — the
// caller airs it at 0 dB rather than on a guessed number.
func TestPlaybackLoudness_UnmeasuredClipsReportNothing(t *testing.T) {
	dir := t.TempDir()
	bare := filepath.Join(dir, "bare.mp4")
	if _, _, ok := asset.PlaybackLoudness(bare); ok {
		t.Fatal("a clip with no sidecar reported a measurement")
	}

	manifest := screeningSubjectManifest(t)
	manifest.Playback.QC = fixtureDerivativeQC(30_000, manifest.Playback.Recipe.KeyframeSeconds, false, 0)
	silent := filepath.Join(dir, "silent.mp4")
	if err := asset.WriteSidecarTags(silent, asset.SidecarTags{MediaAssets: &manifest}, false); err != nil {
		t.Fatal(err)
	}
	if _, _, ok := asset.PlaybackLoudness(silent); ok {
		t.Fatal("a clip with no audio measurement reported one")
	}
}
