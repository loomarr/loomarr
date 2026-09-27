//go:build ffmpeg

package playout

import (
	"image/png"
	"math"
	"os"
	"path/filepath"
	"testing"

	"github.com/loomarr/loomarr/internal/watermark"
)

// EVERY LOOK AIRS AS RENDERED (#1617). The self-check proves each host's blend with its own test
// square; this proves the four looks' real bugs through the packager's item command on NVENC: the
// parameter sets match a bug-off item's, the programme is unchanged outside the bug, and every bug
// pixel is the straight-alpha blend of the bug's own (limited-range) grey over the programme. Run
// it under the GPU lock; it skips where NVENC cannot encode.
func TestLive_PackagerWatermarkEveryLook(t *testing.T) {
	p := newLivePackager(t)
	brk, _ := p.encode(nil, "break")
	off, err := os.ReadFile(brk)
	if err != nil {
		t.Fatal(err)
	}
	offY, err := decodeLuma(p.ctx, p.bin, brk, checkFrame)
	if err != nil {
		t.Fatal(err)
	}
	for _, style := range watermark.Styles {
		mask, err := watermark.CallsignMask("RETRO", style)
		if err != nil {
			t.Fatal(err)
		}
		bug := watermark.Render(mask, p.out.Height, style.Adjust(watermark.Look{Size: 0.06, Opacity: 0.40, Shadow: true}))
		path := filepath.Join(p.dir, "bug-"+string(style)+".png")
		f, err := os.Create(path)
		if err != nil {
			t.Fatal(err)
		}
		if err := png.Encode(f, bug.Straight); err != nil {
			t.Fatal(err)
		}
		_ = f.Close()
		w, h := bug.Size()
		wm := &Watermark{Straight: path, Width: w, Height: h, Corner: CornerTopRight, MarginX: 96, MarginY: 54}

		programme, pl := p.encode(wm, "programme-"+string(style))
		if !pl.Watermark {
			t.Fatalf("%s: the programme item carries no overlay: %v", style, pl.Fallbacks)
		}
		on, err := os.ReadFile(programme)
		if err != nil {
			t.Fatal(err)
		}
		if err := sameParameterSets(off, on); err != nil {
			t.Errorf("%s: %v", style, err)
		}
		onY, err := decodeLuma(p.ctx, p.bin, programme, checkFrame)
		if err != nil {
			t.Fatal(err)
		}
		x, y := wm.position(p.facts, p.out)
		picture := compareBug(offY, onY, p.out.Width, Rect{X: x, Y: y, W: w, H: h}).pictureDiff
		// Where the bug's white is (premultiplied ≥ 0.3), coded luma against a·(16 + 219·grey) +
		// (1−a)·programme. On this host's NVENC a correct blend measured 0.9-2.4 (Outline's 2 px
		// strokes carry the most coding error) and a full-range white (#1541) 7.2-8.6; a wrong
		// alpha convention misses by far more.
		var errSum, n float64
		for by := 0; by < h; by++ {
			for bx := 0; bx < w; bx++ {
				c := bug.Straight.NRGBAAt(bx, by)
				a := float64(c.A) / 255
				if a*float64(c.R) < 0.3*255 {
					continue
				}
				i := (y+by)*p.out.Width + x + bx
				want := a*(16+219*float64(c.R)/255) + (1-a)*float64(offY[i])
				errSum += math.Abs(float64(onY[i]) - want)
				n++
			}
		}
		t.Logf("%s: %dx%d bug, picture ΔY %.2f outside it; mean |ΔY| from the blend %.2f over its %.0f white pixels",
			style, w, h, picture, errSum/n, n)
		if picture > pictureTolerance {
			t.Errorf("%s: the overlay lost the programme picture: mean |ΔY| %.1f outside the bug", style, picture)
		}
		if n == 0 || errSum/n > 4.5 {
			t.Errorf("%s: the bug is not the blend: mean |ΔY| %.2f from it over %.0f white pixels", style, errSum/n, n)
		}
	}
}
