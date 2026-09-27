package playout

import (
	"math"
	"testing"
)

// frame is a flat luma frame with an optional square painted in.
func frame(bg byte, sq Rect, v byte) []byte {
	y := make([]byte, checkWidth*checkHeight)
	for i := range y {
		x, r := i%checkWidth, i/checkWidth
		if x >= sq.X && x < sq.X+sq.W && r >= sq.Y && r < sq.Y+sq.H {
			y[i] = v
		} else {
			y[i] = bg
		}
	}
	return y
}

// The n8.1.2 overlay_cuda failure (#1532 finding 4): the bug is drawn, the programme is gone (all
// green: Y 0), exit 0. The gate's measurement must see the lost picture, not the drawn bug.
func TestCompareBug_SeesTheLostProgramme(t *testing.T) {
	bug := Rect{X: 1680, Y: 54, W: 64, H: 64}
	want := byte(math.Round(checkAlpha*235 + (1-checkAlpha)*120))
	good := compareBug(frame(120, Rect{}, 0), frame(120, bug, want), checkWidth, bug)
	if good.pictureDiff > pictureTolerance || math.Abs(good.bugLuma-good.bugWant) > bugTolerance {
		t.Fatalf("a correct overlay fails the measurement: %+v", good)
	}
	green := compareBug(frame(120, Rect{}, 0), frame(0, bug, want), checkWidth, bug)
	if green.pictureDiff <= pictureTolerance {
		t.Errorf("all-green programme passes: ΔY %.1f", green.pictureDiff)
	}
	// A straight bug through a premultiplied blend (or the reverse) misses the expected luma.
	// So does a full-range white (#1541): 255 where limited-range white is 235, off by 0.651×20 ≈ 13.
	for _, wrong := range []byte{235, byte(math.Round(checkAlpha*checkAlpha*235 + (1-checkAlpha)*120)),
		byte(math.Round(checkAlpha*255 + (1-checkAlpha)*120))} {
		m := compareBug(frame(120, Rect{}, 0), frame(120, bug, wrong), checkWidth, bug)
		if math.Abs(m.bugLuma-m.bugWant) <= bugTolerance {
			t.Errorf("bug luma %d passes against %.1f", wrong, m.bugWant)
		}
	}
	// No bug at the placed position.
	if m := compareBug(frame(120, Rect{}, 0), frame(120, Rect{}, 0), checkWidth, bug); math.Abs(m.bugLuma-m.bugWant) <= bugTolerance {
		t.Error("a missing bug passes")
	}
}

func TestSameParameterSets(t *testing.T) {
	sps := []byte{0x67, 0x64, 0x00, 0x28, 0xac, 0xd9, 0x40, 0x78, 0x02, 0x27, 0xe5, 0x80}
	sps1088 := []byte{0x67, 0x64, 0x00, 0x28, 0xac, 0xd9, 0x40, 0x78, 0x04, 0x4f, 0xcb, 0x80}
	pps := []byte{0x68, 0xeb, 0xe3, 0xcb, 0x22, 0xc0}
	stream := func(s []byte) []byte {
		out := append([]byte{0, 0, 0, 1}, s...)
		out = append(append(out, 0, 0, 0, 1), pps...)
		return append(append(out, 0, 0, 1, 0x65), 0x88, 0x84)
	}
	if err := sameParameterSets(stream(sps), stream(sps)); err != nil {
		t.Errorf("identical streams: %v", err)
	}
	if err := sameParameterSets(stream(sps), stream(sps1088)); err == nil {
		t.Error("a 1088-line SPS passes as identical")
	}
	if err := sameParameterSets(stream(sps), []byte{0, 0, 1, 0x65, 0x88}); err == nil {
		t.Error("a stream without parameter sets passes")
	}
}

func TestWatermarkCheck_NoGPUOverlayFamilies(t *testing.T) {
	for _, f := range []Family{FamilySoftware, FamilyGeneric, FamilyVideoToolbox} {
		if r := WatermarkCheck(t.Context(), "ffmpeg-not-run", HostProfile{Family: f}, t.TempDir()); r.Works {
			t.Errorf("%s: the watermark passed without a GPU overlay", f)
		}
	}
}
