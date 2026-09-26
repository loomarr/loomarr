package mediameasure

import (
	"os/exec"
	"path/filepath"
	"testing"
)

// fadeFixture is a 7 s Matroska file with a 1 s scene fade in the middle: black video AND silent
// audio from 3.0 s to 4.0 s, tone and test pattern either side, one keyframe per second.
func fadeFixture(t *testing.T) string {
	t.Helper()
	ffmpeg, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip("ffmpeg unavailable")
	}
	path := filepath.Join(t.TempDir(), "fade.mkv")
	out, err := exec.Command(ffmpeg, "-nostdin", "-v", "error", "-y",
		"-f", "lavfi", "-i", "testsrc2=s=320x240:r=25:d=3",
		"-f", "lavfi", "-i", "color=c=black:s=320x240:r=25:d=1",
		"-f", "lavfi", "-i", "testsrc2=s=320x240:r=25:d=3",
		"-f", "lavfi", "-i", "sine=f=440:r=48000:d=3",
		"-f", "lavfi", "-i", "anullsrc=r=48000:cl=stereo:d=1",
		"-f", "lavfi", "-i", "sine=f=440:r=48000:d=3",
		"-filter_complex", "[0:v][1:v][2:v]concat=n=3:v=1:a=0[v];[3:a][4:a][5:a]concat=n=3:v=0:a=1[a]",
		"-map", "[v]", "-map", "[a]", "-c:v", "mpeg4", "-g", "25", "-keyint_min", "25", "-sc_threshold", "0",
		"-c:a", "aac", path).CombinedOutput()
	if err != nil {
		t.Fatalf("fixture: %v: %s", err, out)
	}
	return path
}
