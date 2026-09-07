//go:build ffmpeg

package playoutcertfixture

import (
	"context"
	"os/exec"
	"path/filepath"
	"testing"
)

// SignalDecoderFixtures are deterministic H.264/AAC transport streams with
// black/440 Hz A followed by white/880 Hz B, plus a video-only control.
type SignalDecoderFixtures struct {
	AV        string
	VideoOnly string
}

// SignalDecoderMedia generates the small media pair used by decoded-signal
// component tests. The supplied deadline bounds fixture generation.
func SignalDecoderMedia(t testing.TB, ctx context.Context, dir string) SignalDecoderFixtures {
	t.Helper()
	ffmpeg, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Fatal("ffmpeg build-tag fixture requires ffmpeg")
	}
	fixtures := SignalDecoderFixtures{AV: filepath.Join(dir, "signal-av.ts"), VideoOnly: filepath.Join(dir, "signal-video-only.ts")}
	base := []string{"-nostdin", "-v", "error", "-threads", "1", "-filter_threads", "1",
		"-f", "lavfi", "-i", "color=c=black:size=64x64:rate=25:duration=2",
		"-f", "lavfi", "-i", "color=c=white:size=64x64:rate=25:duration=2"}
	runSignalFixture(t, ctx, ffmpeg, append(append([]string{}, base...),
		"-f", "lavfi", "-i", "sine=frequency=440:sample_rate=48000:duration=2",
		"-f", "lavfi", "-i", "sine=frequency=880:sample_rate=48000:duration=2",
		"-filter_complex", "[0:v][1:v]concat=n=2:v=1:a=0[v];[2:a][3:a]concat=n=2:v=0:a=1[a]",
		"-map", "[v]", "-map", "[a]", "-c:v", "libx264", "-preset", "ultrafast", "-pix_fmt", "yuv420p",
		"-c:a", "aac", "-b:a", "128k", "-ar", "48000", "-ac", "2", "-f", "mpegts", "-y", fixtures.AV))
	runSignalFixture(t, ctx, ffmpeg, append(append([]string{}, base...),
		"-filter_complex", "[0:v][1:v]concat=n=2:v=1:a=0[v]", "-map", "[v]",
		"-c:v", "libx264", "-preset", "ultrafast", "-pix_fmt", "yuv420p", "-f", "mpegts", "-y", fixtures.VideoOnly))
	return fixtures
}

func runSignalFixture(t testing.TB, ctx context.Context, executable string, args []string) {
	t.Helper()
	if output, err := exec.CommandContext(ctx, executable, args...).CombinedOutput(); err != nil {
		t.Fatalf("generate decoded signal fixture: %v: %s", err, output)
	}
}
