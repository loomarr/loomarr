package filler

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPlaybackLoudness_ReadsTheIngestMeasurementFromTheSidecar(t *testing.T) {
	manifest := screeningSubjectManifest(t)
	manifest.Playback.QC = fixtureDerivativeQC(30_000, manifest.Playback.Recipe.KeyframeSeconds, true, -23.4)
	media := filepath.Join(t.TempDir(), "playback.mp4")
	if err := os.WriteFile(media, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := WriteSidecarTags(media, SidecarTags{MediaAssets: &manifest}, false); err != nil {
		t.Fatal(err)
	}

	got, ok := PlaybackLoudness(media)
	if !ok || got != -23.4 {
		t.Fatalf("PlaybackLoudness = %v, %v; want -23.4, true", got, ok)
	}
}

// A clip with no sidecar, or one whose audio was never measured, reports NOT measured — the
// caller airs it at 0 dB rather than on a guessed number.
func TestPlaybackLoudness_UnmeasuredClipsReportNothing(t *testing.T) {
	dir := t.TempDir()
	bare := filepath.Join(dir, "bare.mp4")
	if _, ok := PlaybackLoudness(bare); ok {
		t.Fatal("a clip with no sidecar reported a measurement")
	}

	manifest := screeningSubjectManifest(t)
	manifest.Playback.QC = fixtureDerivativeQC(30_000, manifest.Playback.Recipe.KeyframeSeconds, false, 0)
	silent := filepath.Join(dir, "silent.mp4")
	if err := WriteSidecarTags(silent, SidecarTags{MediaAssets: &manifest}, false); err != nil {
		t.Fatal(err)
	}
	if _, ok := PlaybackLoudness(silent); ok {
		t.Fatal("a clip with no audio measurement reported one")
	}
}
