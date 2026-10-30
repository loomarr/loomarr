//go:build ffmpeg

package playout

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

// The subtitles sidecar's extractor against real ffmpeg (#2088 step 7): a span deep in the file
// keeps source times, and a chunk keeps only the cues that start in it.
func TestFFmpegSubtitles_ExtractsAChunkInSourceTime(t *testing.T) {
	ffmpeg := ffmpegBin(t)
	dir := t.TempDir()
	srt := filepath.Join(dir, "eng.srt")
	if err := os.WriteFile(srt, []byte(
		"1\n00:00:02,000 --> 00:00:04,000\nfirst\n\n"+
			"2\n00:00:09,000 --> 00:00:11,000\nstraddles ten\n\n"+
			"3\n00:00:12,500 --> 00:00:14,000\n<i>second</i>\n\n"+
			"4\n00:00:21,000 --> 00:00:23,000\nafter\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	media := filepath.Join(dir, "in.mp4")
	if out, err := exec.Command(ffmpeg, "-v", "error", "-f", "lavfi", "-i", "testsrc2=s=320x180:r=24:d=30",
		"-i", srt, "-map", "0:v", "-map", "1", "-c:v", "libx264", "-g", "48", "-c:s", "mov_text",
		"-metadata:s:s:0", "language=eng", media).CombinedOutput(); err != nil {
		t.Fatalf("make media: %v\n%s", err, out)
	}

	extract := FFmpegSubtitlesWith(ffmpeg, nil)
	cues, err := extract(context.Background(), media, 0, 10*time.Second, 20*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if len(cues) != 1 || cues[0].Start != 12500*time.Millisecond || cues[0].End != 14*time.Second ||
		cues[0].Text != "<i>second</i>" {
		t.Fatalf("cues = %+v, want only the cue starting at 12.5s", cues)
	}
	head, err := extract(context.Background(), media, 0, 5*time.Second, 10*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if len(head) != 1 || head[0].Text != "straddles ten" {
		t.Fatalf("head = %+v, want the cue starting at 9s", head)
	}

	tracks, err := FFprobeTracksNextTo(ffmpeg)(context.Background(), media)
	if err != nil || len(tracks.Subtitles) != 1 || tracks.Subtitles[0].Codec != "mov_text" || !IsTextSubtitle(tracks.Subtitles[0].Codec) {
		t.Fatalf("tracks = %+v, err = %v", tracks, err)
	}
}
