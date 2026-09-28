package demolibrary

import (
	"context"
	"image/png"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

func testGenerator(t *testing.T) generator {
	t.Helper()
	ffmpeg, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip("ffmpeg not on PATH")
	}
	l := Layout{Dir: t.TempDir()}
	if err := os.MkdirAll(l.work(), 0o755); err != nil {
		t.Fatal(err)
	}
	return generator{ffmpeg: ffmpeg, l: l, log: slog.New(slog.DiscardHandler)}
}

// The filler quality gate (internal/mediatools/quality.go) decides whether a clip can air. The first
// generator failed it twice, so no demo break aired: its silent track was rejected as
// silent_content ("99% of the 10.0s audio is silent"), and its static cards were held for review
// ("The picture is unchanged for 20.0s"). Each interstitial must carry real sound, a moving
// picture, and clear the gate's 10-second floor, measured with the gate's own detector settings.
func TestFillerClearsTheQualityGate(t *testing.T) {
	g := testGenerator(t)
	for i, f := range Fillers {
		if f.Duration <= 10 {
			t.Errorf("%s is %ds; the quality gate's floor is 10s", f.ID, f.Duration)
		}
		out := filepath.Join(g.l.Dir, f.ID+".mp4")
		if err := g.filler(context.Background(), f, chroma[i%len(chroma)], out); err != nil {
			t.Fatal(err)
		}
		stats, err := exec.Command(g.ffmpeg, "-hide_banner", "-nostdin", "-i", out, "-vn", "-af", "volumedetect", "-f", "null", "-").CombinedOutput()
		if err != nil {
			t.Fatalf("volumedetect %s: %v: %s", f.ID, err, stats)
		}
		m := regexp.MustCompile(`mean_volume: (-?[0-9.]+|-inf) dB`).FindStringSubmatch(string(stats))
		if m == nil {
			t.Fatalf("%s: no mean_volume in ffmpeg output", f.ID)
		}
		if mean, err := strconv.ParseFloat(m[1], 64); err != nil || mean < -40 {
			t.Errorf("%s mean volume %s dB; the clip is effectively silent", f.ID, m[1])
		}
		detect, err := exec.Command(g.ffmpeg, "-hide_banner", "-nostdin", "-i", out,
			"-vf", "blackdetect=d=0.1:pix_th=0.20,freezedetect=n=-60dB:d=2",
			"-af", "silencedetect=n=-35dB:d=0.3", "-f", "null", "-").CombinedOutput()
		if err != nil {
			t.Fatalf("detectors %s: %v: %s", f.ID, err, detect)
		}
		for _, marker := range []string{"freeze_start", "black_start", "silence_start"} {
			if strings.Contains(string(detect), marker) {
				t.Errorf("%s trips the quality gate's detector: %s", f.ID, marker)
			}
		}
	}
}

// Playout draws a custom watermark by its alpha and refuses an image with no visible shape
// ("airing without it"). The first generator drew opaque bars onto a transparent canvas, but
// drawbox never writes alpha, so the file was entirely transparent and no demo channel had a bug.
func TestWatermarkHasAVisibleShape(t *testing.T) {
	g := testGenerator(t)
	out := filepath.Join(g.l.Dir, "wm.png")
	if err := g.watermark(context.Background(), out); err != nil {
		t.Fatal(err)
	}
	f, err := os.Open(out)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	img, err := png.Decode(f)
	if err != nil {
		t.Fatal(err)
	}
	opaque, clear := 0, 0
	b := img.Bounds()
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			switch _, _, _, a := img.At(x, y).RGBA(); a {
			case 0xffff:
				opaque++
			case 0:
				clear++
			}
		}
	}
	total := b.Dx() * b.Dy()
	if opaque < total/2 {
		t.Fatalf("watermark has %d/%d opaque pixels; the bars must be visible", opaque, total)
	}
	if clear == 0 {
		t.Fatal("watermark has no transparent pixels; its alpha must carry the bars' shape")
	}
}
