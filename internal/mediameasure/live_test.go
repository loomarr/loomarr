package mediameasure

import (
	"context"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"testing"
	"time"
)

// TestLive_MeasureFile times each measurement step on a real file. It is skipped unless
// MEDIAMEASURE_LIVE_FILE names one, so the supervisor can point it at library files:
//
//	MEDIAMEASURE_LIVE_FILE=/path/to/film.mkv go test ./internal/mediameasure -run Live -v
func TestLive_MeasureFile(t *testing.T) {
	path := os.Getenv("MEDIAMEASURE_LIVE_FILE")
	if path == "" {
		t.Skip("set MEDIAMEASURE_LIVE_FILE to measure a real file")
	}
	ctx := context.Background()
	tools := DefaultTools(os.Getenv("MEDIAMEASURE_FFMPEG"), os.Getenv("MEDIAMEASURE_FFPROBE"))
	out, err := exec.Command(tools.FFprobe, "-v", "error", "-show_entries", "format=duration", "-of", "csv=p=0", path).Output()
	if err != nil {
		t.Fatal(err)
	}
	seconds, err := strconv.ParseFloat(strings.TrimSpace(string(out)), 64)
	if err != nil {
		t.Fatal(err)
	}
	durationMs := int64(seconds * 1000)

	start := time.Now()
	frames, err := tools.Keyframes(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("keyframes: %d in %s", len(frames), time.Since(start).Round(time.Millisecond))

	start = time.Now()
	quality, loudness, err := tools.Decode(ctx, path, durationMs, true, true)
	if err != nil {
		t.Fatal(err)
	}
	breaks := BreakCandidates(quality.Black, quality.Silence, frames, durationMs)
	t.Logf("decode pass: %s for %.0f s of media; black %d, silence %d, breaks %d; %.1f LUFS, true peak %+v",
		time.Since(start).Round(time.Millisecond), seconds, len(quality.Black), len(quality.Silence), len(breaks),
		loudness.IntegratedLUFS, loudness.TruePeak)
}
