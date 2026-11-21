//go:build ffmpeg

package cliplibrary

import (
	"context"
	"os/exec"
	"path/filepath"
	"testing"
)

// The probe records the stream's declared rate beside its measured average, which the transcode's
// preservation check compares when a source's uneven timestamps move the average.
func TestProbeRecordsTheNominalFrameRate(t *testing.T) {
	ffmpeg, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Fatal("ffmpeg build-tag test requires ffmpeg")
	}
	ffprobe, err := exec.LookPath("ffprobe")
	if err != nil {
		t.Fatal("ffmpeg build-tag test requires ffprobe")
	}
	clip := filepath.Join(t.TempDir(), "ntsc.mp4")
	if out, err := exec.Command(ffmpeg, "-v", "error", "-f", "lavfi", "-i", "testsrc=size=320x240:rate=30000/1001:duration=2",
		"-c:v", "libx264", "-pix_fmt", "yuv420p", clip).CombinedOutput(); err != nil {
		t.Fatalf("make fixture: %v: %s", err, out)
	}
	got, err := ffprobeWith(context.Background(), ffprobe, clip)
	if err != nil {
		t.Fatal(err)
	}
	if got.NominalCadence != "30000/1001" || got.Cadence == "" {
		t.Fatalf("probe = cadence %q nominal %q, want a measured average and nominal 30000/1001", got.Cadence, got.NominalCadence)
	}
}
