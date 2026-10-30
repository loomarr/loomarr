package playout

import (
	"encoding/binary"
	"image/color"
	"testing"
	"time"
)

// supWriter builds a .sup stream the way a Blu-ray authoring tool lays it out.
type supWriter struct{ b []byte }

func (w *supWriter) seg(at time.Duration, kind byte, data []byte) {
	pts := uint32(at * 90000 / time.Second)
	h := []byte{'P', 'G', 0, 0, 0, 0, 0, 0, 0, 0, kind, 0, 0}
	binary.BigEndian.PutUint32(h[2:6], pts)
	binary.BigEndian.PutUint16(h[11:13], uint16(len(data)))
	w.b = append(append(w.b, h...), data...)
}

// show is a display set: palette 0 (index 1 opaque white, 2 half-transparent black), one object
// placed at x,y, its rows run-length coded.
func (w *supWriter) show(at time.Duration, state byte, x, y int, rle []byte, ow, oh int) {
	pcs := []byte{0x07, 0x80, 0x04, 0x38, 0x10, 0, 1, state, 0, 0, 1, 0, 1, 0, 0, 0, 0, 0, 0}
	binary.BigEndian.PutUint16(pcs[15:17], uint16(x))
	binary.BigEndian.PutUint16(pcs[17:19], uint16(y))
	w.seg(at, pgsComposition, pcs)
	w.seg(at, pgsWindow, []byte{1, 0, 0, 0, 0, 0, 0, 0, 0, 0})
	w.seg(at, pgsPalette, []byte{0, 0, 1, 235, 128, 128, 255, 2, 16, 128, 128, 128})
	ods := []byte{0, 1, 0, 0xC0, 0, 0, 0, 0, 0, 0, 0}
	binary.BigEndian.PutUint16(ods[7:9], uint16(ow))
	binary.BigEndian.PutUint16(ods[9:11], uint16(oh))
	w.seg(at, pgsObject, append(ods, rle...))
	w.seg(at, pgsEnd, nil)
}

func (w *supWriter) clear(at time.Duration) {
	w.seg(at, pgsComposition, []byte{0x07, 0x80, 0x04, 0x38, 0x10, 0, 2, 0, 0, 0, 0})
	w.seg(at, pgsWindow, []byte{1, 0, 0, 0, 0, 0, 0, 0, 0, 0})
	w.seg(at, pgsEnd, nil)
}

// A 4x2 object: row 0 is two transparent then two white (a short zero run, then literal pixels);
// row 1 is a coded run of four half-black.
var twoRows = []byte{0, 0x02, 1, 1, 0, 0, 0, 0x84, 2, 0, 0}

func TestParsePGSDrawsEachDisplaySetOnTheCanvas(t *testing.T) {
	var w supWriter
	w.show(1*time.Second, pgsEpochStart, 100, 900, twoRows, 4, 2)
	w.clear(4 * time.Second)
	states, width, height, err := ParsePGS(w.b)
	if err != nil {
		t.Fatal(err)
	}
	if width != 1920 || height != 1080 {
		t.Fatalf("canvas %dx%d, want 1920x1080", width, height)
	}
	if len(states) != 2 || states[0].At != time.Second || states[1].At != 4*time.Second || states[1].Image != nil {
		t.Fatalf("states = %+v, want a still at 1s and a clear at 4s", states)
	}
	img := states[0].Image
	white := color.NRGBA{R: 255, G: 255, B: 255, A: 255}
	for _, c := range []struct {
		x, y int
		want color.NRGBA
	}{
		{100, 900, color.NRGBA{}}, // transparent index stays transparent
		{102, 900, white},
		{103, 900, white},
		{100, 901, color.NRGBA{A: 128}},
		{103, 901, color.NRGBA{A: 128}},
		{104, 900, color.NRGBA{}}, // outside the object
	} {
		if got := img.NRGBAAt(c.x, c.y); got != c.want {
			t.Errorf("pixel (%d,%d) = %v, want %v", c.x, c.y, got, c.want)
		}
	}
}

func TestParsePGSFoldsRepeatedClears(t *testing.T) {
	var w supWriter
	w.clear(0) // a stream often opens with an empty epoch
	w.show(time.Second, pgsEpochStart, 0, 0, twoRows, 4, 2)
	w.clear(2 * time.Second)
	w.clear(3 * time.Second)
	states, _, _, err := ParsePGS(w.b)
	if err != nil {
		t.Fatal(err)
	}
	if len(states) != 2 || states[0].Image == nil || states[1].At != 2*time.Second {
		t.Fatalf("states = %+v, want one still then one clear", states)
	}
}

func TestParsePGSLongRunsSpanTheirWidth(t *testing.T) {
	// A 300-pixel row: a long coded run of colour 1 (flag 0xC0 | 300>>8, then 300&0xFF).
	rle := []byte{0, 0xC0 | 1, 300 & 0xFF, 1, 0, 0}
	var w supWriter
	w.show(0, pgsEpochStart, 10, 10, rle, 300, 1)
	states, _, _, err := ParsePGS(w.b)
	if err != nil {
		t.Fatal(err)
	}
	img := states[0].Image
	if img.NRGBAAt(10, 10).A != 255 || img.NRGBAAt(309, 10).A != 255 || img.NRGBAAt(310, 10).A != 0 {
		t.Fatalf("run edges: %v %v %v", img.NRGBAAt(10, 10), img.NRGBAAt(309, 10), img.NRGBAAt(310, 10))
	}
}

func TestParsePGSRejectsATruncatedSegment(t *testing.T) {
	var w supWriter
	w.show(0, pgsEpochStart, 0, 0, twoRows, 4, 2)
	if _, _, _, err := ParsePGS(w.b[:len(w.b)-5]); err == nil {
		t.Fatal("a truncated stream parsed")
	}
}

func TestPGSColorRoundTripsBT709(t *testing.T) {
	if got := pgsColor(235, 128, 128, 255); got != (color.NRGBA{R: 255, G: 255, B: 255, A: 255}) {
		t.Errorf("white = %v", got)
	}
	if got := pgsColor(16, 128, 128, 255); got != (color.NRGBA{A: 255}) {
		t.Errorf("black = %v", got)
	}
	// BT.709 limited-range pure red: Y 63, Cb 102, Cr 240.
	if got := pgsColor(63, 240, 102, 255); got.R < 250 || got.G > 5 || got.B > 5 {
		t.Errorf("red = %v", got)
	}
}
