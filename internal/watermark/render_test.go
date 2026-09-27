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

// testLook is the approved Plate look at the approved opacity (#1617). internal/schedule owns the
// product defaults; these tests exercise the renderer at the same values.
var testLook = Look{Size: 0.06, Opacity: 0.40, Shadow: true}

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
		w, h := sizeFor(c.w, c.h, 1080, testLook.Size)
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
	bug := Render(mask, 1080, testLook)
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
			// The bug's own opacity is its premultiplied white, R×A (the shadow beneath adds alpha,
			// not light).
			maxA = max(maxA, uint8((int(c.R)*int(c.A)+127)/255))
			if c.A > 0 && c.R < 64 {
				shadowSeen = true
			}
		}
	}
	// The plate is the look's opacity at most: it is baked in, and the bug never reaches opaque.
	if want := uint8(testLook.Opacity*255 + 0.5); maxA < want-1 || maxA > want+1 {
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
	again := Render(mask, 1080, testLook)
	if !bytes.Equal(again.Straight.Pix, bug.Straight.Pix) {
		t.Error("rendering is not deterministic")
	}
}

// THE CANDIDATE DESIGNS (#1617) share the Plate's geometry, so each is the Plate's letters at the
// Plate's size and position: Text is the knockout drawn positive with no plate, Outline is Text
// hollowed to a stroke around each letter.
func TestCallsignMask_CandidatesShareThePlatesLetters(t *testing.T) {
	plate, err := PlateMask("RETRO")
	if err != nil {
		t.Fatal(err)
	}
	masks := map[Design]*image.Alpha{}
	for _, d := range []Design{DesignPlate, DesignText, DesignOutline} {
		if masks[d], err = CallsignMask("RETRO", d); err != nil {
			t.Fatalf("%s: %v", d, err)
		}
		if masks[d].Rect != plate.Rect {
			t.Errorf("%s: %v, want the Plate's %v", d, masks[d].Rect, plate.Rect)
		}
	}
	if !bytes.Equal(masks[DesignPlate].Pix, plate.Pix) {
		t.Error("DesignPlate is not the Plate")
	}
	text, outline := masks[DesignText], masks[DesignOutline]
	var knocked, drawn, inside, hollow, stroke int
	for i := range plate.Pix {
		if plate.Pix[i] < 40 && i%plate.Stride > plate.Stride/10 && i%plate.Stride < plate.Stride*9/10 {
			knocked++ // a letter pixel, inside the plate's padding
			if text.Pix[i] > 215 {
				drawn++
			}
		}
		if text.Pix[i] == 255 {
			inside++
			if outline.Pix[i] == 0 {
				hollow++
			}
		} else if text.Pix[i] == 0 && outline.Pix[i] > 200 {
			stroke++
		}
	}
	if knocked == 0 || drawn < knocked*95/100 {
		t.Errorf("Text draws %d of the Plate's %d knocked-out letter pixels", drawn, knocked)
	}
	if mid := text.Pix[text.PixOffset(text.Rect.Dx()/20, text.Rect.Dy()/2)]; mid != 0 {
		t.Errorf("Text has a plate: alpha %d in its padding", mid)
	}
	// The stroke is centred on the letter's edge, so it covers the outer half-stroke of each letter
	// (~half its pixels at Geist Bold's stem width) and as much again outside.
	if inside == 0 || hollow < inside/3 || stroke == 0 {
		t.Errorf("Outline is not a hollow stroke: %d of %d letter pixels hollow, %d stroke pixels outside", hollow, inside, stroke)
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
	look := testLook
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
		s := Legibility(mask, 1080, testLook)
		w, h := Render(mask, 1080, testLook).Size()
		t.Logf("%s: survival %.2f at %dx%d", name, s, w, h)
		if (s >= legibleSurvival) != legible {
			t.Errorf("%s: survival %.2f, sample sheet says legible=%v", name, s, legible)
		}
	}
}
