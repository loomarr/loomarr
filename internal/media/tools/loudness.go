package tools

import (
	"bytes"
	"context"
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/loomarr/loomarr/internal/media/bgexec"
)

// SilentDb is the level a window of digital silence (or no audio) reads as: astats reports -inf.
const SilentDb = -90.0

// LoudnessWindows is the RMS level, in dB, of each consecutive winMs window of a file's first
// audio track over [startMs, endMs). A file with no audio errors: there is no sound to judge.
func (t *FFmpegTools) LoudnessWindows(ctx context.Context, file string, startMs, endMs, winMs int64) ([]float64, error) {
	if startMs < 0 || endMs <= startMs || winMs <= 0 {
		return nil, fmt.Errorf("invalid loudness span %d..%d by %d", startMs, endMs, winMs)
	}
	var stdout, stderr bytes.Buffer
	cmd := bgexec.FFmpeg(ctx, t.FFmpegPath,
		"-nostdin", "-hide_banner", "-nostats", "-v", "error",
		"-ss", msToSeconds(startMs), "-t", msToSeconds(endMs-startMs),
		"-i", file, "-map", "0:a:0", "-ac", "1", "-ar", "48000",
		"-af", fmt.Sprintf("asetnsamples=n=%d:p=0,astats=metadata=1:reset=1,ametadata=print:key=lavfi.astats.Overall.RMS_level:file=-", 48*winMs),
		"-f", "null", "-")
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("ffmpeg loudness %s at %d..%d: %w: %s", file, startMs, endMs, err, stderr.String())
	}
	levels := parseLoudness(stdout.String())
	if len(levels) == 0 {
		return nil, fmt.Errorf("ffmpeg loudness %s at %d..%d: no audio measured", file, startMs, endMs)
	}
	return levels, nil
}

// parseLoudness reads astats' per-window RMS lines. -inf and anything below SilentDb read as
// SilentDb, so silence compares as a number.
func parseLoudness(out string) []float64 {
	var levels []float64
	for _, line := range strings.Split(out, "\n") {
		_, v, ok := strings.Cut(line, "lavfi.astats.Overall.RMS_level=")
		if !ok {
			continue
		}
		db, err := strconv.ParseFloat(strings.TrimSpace(v), 64)
		if err != nil || math.IsNaN(db) || db < SilentDb {
			db = SilentDb
		}
		levels = append(levels, db)
	}
	return levels
}
