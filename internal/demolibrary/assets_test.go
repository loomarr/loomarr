package demolibrary

import (
	"context"
	"image/png"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
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
