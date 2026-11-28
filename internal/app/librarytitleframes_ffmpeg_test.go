//go:build ffmpeg

// Executes ffmpeg: run with `make test-ffmpeg`.
package app

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/loomarr/loomarr/internal/playout/hls"
)

// The pinned still path takes a home video's frame about a tenth of the way in: the fixture is red
// for its first 1.5 of 20 seconds, green to 3 seconds and blue after, with a keyframe each second.
func TestLiveHomeVideoPosterFrame(t *testing.T) {
	ctx := context.Background()
	ffmpeg, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Fatal(err)
	}
	video := filepath.Join(t.TempDir(), "home.mp4")
	filter := "color=c=red:s=320x180:r=10:d=1.5[a];color=c=0x00ff00:s=320x180:r=10:d=1.5[b];" +
		"color=c=blue:s=320x180:r=10:d=17[c];[a][b][c]concat=n=3:v=1:a=0[v]"
	if out, err := exec.CommandContext(ctx, ffmpeg, "-v", "error", "-filter_complex", filter, "-map", "[v]",
		"-c:v", "mpeg4", "-g", "5", "-y", video).CombinedOutput(); err != nil {
		t.Fatalf("make the fixture: %v: %s", err, out)
	}
	if _, err := os.Stat(video); err != nil {
		t.Fatal(err)
	}
	p, keep, item := homePosterFixture(t, video, 20_000)
	p.still = hls.FFmpegSourceStill(ffmpeg, nil)
	keep()
	if err := p.pass(ctx); err != nil {
		t.Fatal(err)
	}
	rec, err := p.images.Get(ctx, item().Poster)
	if err != nil || rec.Width != 320 || rec.Height != 180 {
		t.Fatalf("poster = %+v, %v", rec, err)
	}
	// The dominant colour of the frame at 2s is the green stretch's.
	var r, g, b int
	if _, err := fmt.Sscanf(rec.DominantHex, "#%02x%02x%02x", &r, &g, &b); err != nil || g < 200 || r > 60 || b > 60 {
		t.Fatalf("dominant colour = %q, %v; want green", rec.DominantHex, err)
	}
}
