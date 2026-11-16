//go:build ffmpeg

// Tests that EXECUTE ffmpeg, behind the same build tag as playout's: unit tests must not depend on
// an installed ffmpeg.
package subtitles

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

// Both a subtitle file beside a video and a track inside one read back as the same cues.
func TestFFmpegReaderReadsTextTracks(t *testing.T) {
	ffmpeg, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip("no ffmpeg on PATH")
	}
	dir := t.TempDir()
	srt := filepath.Join(dir, "clip.en.srt")
	if err := os.WriteFile(srt, []byte("1\n00:00:01,000 --> 00:00:02,500\n<i>Hello</i> there\n\n2\n00:00:03,000 --> 00:00:04,000\nBye\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	mkv := filepath.Join(dir, "clip.mkv")
	fixture := exec.Command(ffmpeg, "-nostdin", "-loglevel", "error", "-f", "lavfi", "-i", "color=c=black:s=64x36:d=5",
		"-i", srt, "-map", "0:v", "-map", "1:s", "-c:v", "libx264", "-c:s", "ass", "-metadata:s:s:0", "language=eng", mkv)
	if out, err := fixture.CombinedOutput(); err != nil {
		t.Fatalf("make the fixture: %v: %s", err, out)
	}

	read := FFmpegReader(ffmpeg)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	for name, src := range map[string]Source{
		"beside": {Input: srt, Stream: -1},
		"inside": {Input: mkv, Stream: 1},
	} {
		doc, err := read(ctx, src)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		cues, err := ParseWebVTT(doc)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if len(cues) != 2 || cues[0].Text != "Hello there" || cues[0].Start != time.Second ||
			cues[0].End != 2500*time.Millisecond || cues[1].Text != "Bye" {
			t.Errorf("%s: cues = %+v", name, cues)
		}
	}
}
