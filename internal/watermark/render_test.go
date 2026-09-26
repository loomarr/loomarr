package watermark

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"
)

// The approved sizes, from the sample sheets (PR #1532): at 1080p every bug covers ~2·65² px², so
// HBO's 2.4:1 mark is 143x59, NBC's 4.1:1 is 185x45 and the 6.9:1 Nickelodeon wordmark 238x35.
func TestSizeFor_EqualArea(t *testing.T) {
	for _, c := range []struct {
		w, h         int
		wantW, wantH int
	}{
		{1430, 590, 143, 59}, // the HBO sample
		{1850, 450, 186, 45}, // the NBC sample (185 there: its trimmed logo's own rounding)
		{2380, 350, 239, 35}, // the Nickelodeon wordmark sample (238)
		{100, 100, 65, 65},   // square: reaches H
		{100, 300, 22, 65},   // tall: capped at H, never taller
	} {
		w, h := sizeFor(c.w, c.h, 1080, DefaultSize)
		if w != c.wantW || h != c.wantH {
			t.Errorf("%dx%d: %dx%d, want %dx%d", c.w, c.h, w, h, c.wantW, c.wantH)
		}
	}
}

func TestRender_WhiteSilhouetteBakedOpacityShadowEvenSize(t *testing.T) {
	mask, err := PlateMask("RETRO")
	if err != nil {
		t.Fatal(err)
	}
	bug := Render(mask, 1080, DefaultLook())
	w, h := bug.Size()
	if w%2 != 0 || h%2 != 0 {
		t.Fatalf("odd size %dx%d", w, h)
	}
	var maxA uint8
	var shadowSeen, knockout bool
	s := bug.Straight
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			c := s.NRGBAAt(x, y)
			// The bug's own opacity is its premultiplied white (the shadow beneath adds alpha, not light).
			maxA = max(maxA, bug.Premultiplied.NRGBAAt(x, y).R)
			if c.A > 0 && c.R < 64 {
				shadowSeen = true
			}
			p := bug.Premultiplied.NRGBAAt(x, y)
			if p.A != c.A || int(p.R) > int(c.A)+1 {
				t.Fatalf("(%d,%d) premultiplied %v vs straight %v", x, y, p, c)
			}
		}
	}
	// The plate is 65% at most: the opacity is baked in, and the bug never reaches opaque.
	if want := uint8(166); maxA < want-1 || maxA > want+1 { // 0.65 × 255
		t.Errorf("peak alpha %d, want %d", maxA, want)
	}
	if !shadowSeen {
		t.Error("no drop shadow")
	}
	// The knocked-out callsign: transparent-ish pixels inside the plate's middle row.
	mid := h / 2
	for x := w / 4; x < 3*w/4; x++ {
		if s.NRGBAAt(x, mid).A < 40 {
			knockout = true
		}
	}
	if !knockout {
		t.Error("callsign not knocked out of the plate")
	}
	// Deterministic: the same input renders the same bytes.
	again := Render(mask, 1080, DefaultLook())
	if !bytes.Equal(again.Straight.Pix, bug.Straight.Pix) {
		t.Error("rendering is not deterministic")
	}
}

// stroke draws a ring of the given stroke width (at supersampled scale) or a filled disc.
func strokeLogo(size, stroke int) *image.NRGBA {
	img := image.NewNRGBA(image.Rect(0, 0, size*3, size))
	for y := 0; y < size; y++ {
		for x := 0; x < size*3; x++ {
			// Three vertical bars of the stroke width, like a thin wordmark's stems.
			if (x%size) < stroke && y > 0 {
				img.SetNRGBA(x, y, color.NRGBA{200, 30, 30, 255})
			}
		}
	}
	return img
}

// THE MEASURED VARIANT RULE: a thin primary loses to a legible alternative; a legible primary wins.
func TestPickVariant_PrefersTheFirstLegibleVariant(t *testing.T) {
	thin, _ := LogoMask(strokeLogo(800, 12))  // stems ~1 px at airing size
	bold, _ := LogoMask(strokeLogo(800, 300)) // stems ~24 px
	look := DefaultLook()
	if s := Legibility(thin, 1080, look); s >= legibleSurvival {
		t.Errorf("thin strokes measured legible: %.2f", s)
	}
	if s := Legibility(bold, 1080, look); s < legibleSurvival {
		t.Errorf("bold strokes measured illegible: %.2f", s)
	}
	if got := PickVariant([]*image.Alpha{thin, bold}, 1080, look); got != 1 {
		t.Errorf("thin primary picked over a legible wordmark: %d", got)
	}
	if got := PickVariant([]*image.Alpha{bold, thin}, 1080, look); got != 0 {
		t.Errorf("a legible primary lost: %d", got)
	}
	if got := PickVariant([]*image.Alpha{thin}, 1080, look); got != 0 {
		t.Errorf("the only variant must still air: %d", got)
	}
}

// Calibration on the real TMDB logos from the sample sheets (not in the repo: trademarks). Set
// WATERMARK_TEST_LOGOS to a directory holding hbo.png, nbc.png, nick.png (the splat) and nickw.png.
func TestLegibility_SampleLogos(t *testing.T) {
	dir := os.Getenv("WATERMARK_TEST_LOGOS")
	if dir == "" {
		t.Skip("WATERMARK_TEST_LOGOS not set")
	}
	want := map[string]bool{"hbo": true, "nbc": true, "nick": false, "nickw": true}
	for name, legible := range want {
		f, err := os.Open(filepath.Join(dir, name+".png"))
		if err != nil {
			t.Fatal(err)
		}
		img, err := png.Decode(f)
		_ = f.Close()
		if err != nil {
			t.Fatal(err)
		}
		mask, err := LogoMask(img)
		if err != nil {
			t.Fatal(err)
		}
		s := Legibility(mask, 1080, DefaultLook())
		w, h := Render(mask, 1080, DefaultLook()).Size()
		t.Logf("%s: survival %.2f at %dx%d", name, s, w, h)
		if (s >= legibleSurvival) != legible {
			t.Errorf("%s: survival %.2f, sample sheet says legible=%v", name, s, legible)
		}
	}
}
