//go:build ffmpeg

package hls

import (
	"context"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"testing"
	"time"
)

func ffmpegBin(t *testing.T) string {
	t.Helper()
	if p := os.Getenv("FFMPEG_PATH"); p != "" {
		return p
	}
	p, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip("no ffmpeg on PATH")
	}
	return p
}

// A tone-mapped picture sits inside these signalstats bounds; black is 16/16 (#1516).
const (
	pictureMinYMAX = 100
	pictureMinYAVG = 60
	pictureMaxYAVG = 200
)

func assertPicture(t *testing.T, bin, path string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	b, err := exec.CommandContext(ctx, bin, "-hide_banner", "-nostats", "-i", path, "-map", "0:v:0",
		"-vf", "signalstats,metadata=print:file=-", "-f", "null", "-").Output()
	if err != nil {
		t.Fatalf("signalstats: %v", err)
	}
	// The mean YAVG, and the LOWEST per-frame YMAX so one black frame anywhere fails.
	var sum float64
	frames, lowest := 0, -1
	for line := range strings.Lines(string(b)) {
		k, v, ok := strings.Cut(strings.TrimSpace(line), "=")
		if !ok {
			continue
		}
		switch k {
		case "lavfi.signalstats.YAVG":
			f, err := strconv.ParseFloat(v, 64)
			if err != nil {
				t.Fatalf("YAVG %q: %v", v, err)
			}
			sum += f
			frames++
		case "lavfi.signalstats.YMAX":
			n, err := strconv.Atoi(v)
			if err != nil {
				t.Fatalf("YMAX %q: %v", v, err)
			}
			if lowest < 0 || n < lowest {
				lowest = n
			}
		}
	}
	if frames == 0 {
		t.Fatalf("signalstats read no frames from %s", path)
	}
	avg := sum / float64(frames)
	t.Logf("YAVG mean %.1f, lowest per-frame YMAX %d", avg, lowest)
	if lowest < pictureMinYMAX || avg < pictureMinYAVG || avg > pictureMaxYAVG {
		t.Errorf("tone-mapped output is not a picture: YAVG mean %.1f (want %d-%d), lowest YMAX %d (want >= %d)",
			avg, pictureMinYAVG, pictureMaxYAVG, lowest, pictureMinYMAX)
	}
}
