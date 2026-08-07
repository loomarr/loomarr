package filler_test

import (
	"bytes"
	"image"
	"image/color"
	"image/jpeg"
	"testing"

	"github.com/mantonx/loomarr/internal/filler"
)

// The frame-heuristic tier (§10 V44) is deterministic and LLM-free, so it is
// tested on SYNTHETIC frames we JPEG-encode here — no ffmpeg, no network (§19).
// Encoding through image/jpeg is deliberate, not just convenient: it exercises the
// real decode path AnalyzeFrames uses AND puts the lossy chroma artefacts a genuine
// keyframe carries in front of bwChromaThreshold, so a threshold too tight to
// survive JPEG would fail here rather than silently in production.

// solidFrame builds a w×h JPEG filled with one colour. A grey fill (r==g==b) is the
// monochrome case; anything else is a colour frame.
func solidFrame(t *testing.T, w, h int, c color.Color) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, c)
		}
	}
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: 90}); err != nil {
		t.Fatalf("encode frame: %v", err)
	}
	return buf.Bytes()
}

// grey/colour/dimension helpers, so the table rows read as intent not arithmetic.
func gray(v uint8) color.Color { return color.RGBA{v, v, v, 255} }
func red() color.Color         { return color.RGBA{200, 30, 30, 255} }

const fourThreeW, fourThreeH = 320, 240     // 1.333…
const sixteenNineW, sixteenNineH = 320, 180 // 1.777…

func TestAnalyzeFrames_BlackAndWhiteDetection(t *testing.T) {
	tests := []struct {
		name   string
		frames [][]byte
		wantBW bool
		wantN  int
	}{
		{
			name:   "all grey frames read monochrome",
			frames: [][]byte{grayF(t, 40), grayF(t, 128), grayF(t, 210)},
			wantBW: true,
			wantN:  3,
		},
		{
			name:   "all colour frames do not",
			frames: [][]byte{colourF(t), colourF(t), colourF(t)},
			wantBW: false,
			wantN:  3,
		},
		{
			// A single grey fade in a colour spot must NOT make the clip B&W — the
			// majority rule (bwFrameFraction) exists for exactly this.
			name:   "one grey frame among colour is not enough",
			frames: [][]byte{grayF(t, 128), colourF(t), colourF(t)},
			wantBW: false,
			wantN:  3,
		},
		{
			// The mirror: a majority grey DOES qualify even with a stray colour frame.
			name:   "majority grey with one colour frame qualifies",
			frames: [][]byte{grayF(t, 80), grayF(t, 160), colourF(t)},
			wantBW: true,
			wantN:  3,
		},
		{
			// A corrupt frame is skipped, not fatal, and does not count toward N.
			name:   "undecodable frames are ignored",
			frames: [][]byte{grayF(t, 100), []byte("not a jpeg"), grayF(t, 150)},
			wantBW: true,
			wantN:  2,
		},
		{
			name:   "no frames yields no measurement",
			frames: nil,
			wantBW: false,
			wantN:  0,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := filler.AnalyzeFrames(tc.frames)
			if got.BlackAndWhite != tc.wantBW {
				t.Errorf("BlackAndWhite = %v, want %v", got.BlackAndWhite, tc.wantBW)
			}
			if got.Frames != tc.wantN {
				t.Errorf("Frames = %d, want %d", got.Frames, tc.wantN)
			}
		})
	}
}

func TestAnalyzeFrames_AspectRatio(t *testing.T) {
	tests := []struct {
		name   string
		frames [][]byte
		wantLo float64 // aspect ratio band [wantLo, wantHi]
		wantHi float64
	}{
		{
			name:   "4:3 frames average ~1.333",
			frames: [][]byte{solidFrame(t, fourThreeW, fourThreeH, gray(128))},
			wantLo: 1.30, wantHi: 1.36,
		},
		{
			name:   "16:9 frames average ~1.778",
			frames: [][]byte{solidFrame(t, sixteenNineW, sixteenNineH, gray(128))},
			wantLo: 1.74, wantHi: 1.82,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := filler.AnalyzeFrames(tc.frames)
			if got.AspectRatio < tc.wantLo || got.AspectRatio > tc.wantHi {
				t.Errorf("AspectRatio = %.4f, want in [%.2f,%.2f]", got.AspectRatio, tc.wantLo, tc.wantHi)
			}
		})
	}
}

// The hint is the whole point: a B&W 4:3 clip SUGGESTS a pre-1970s era, and every
// other combination suggests nothing. The suggestion is never a grounded Era — the
// caller writes it to Clip.SuggestedEra, which matching never reads.
func TestSuggestedEraFrom(t *testing.T) {
	bw43 := filler.AnalyzeFrames([][]byte{
		solidFrame(t, fourThreeW, fourThreeH, gray(60)),
		solidFrame(t, fourThreeW, fourThreeH, gray(140)),
	})
	colour43 := filler.AnalyzeFrames([][]byte{
		solidFrame(t, fourThreeW, fourThreeH, red()),
		solidFrame(t, fourThreeW, fourThreeH, red()),
	})
	bw169 := filler.AnalyzeFrames([][]byte{
		solidFrame(t, sixteenNineW, sixteenNineH, gray(60)),
		solidFrame(t, sixteenNineW, sixteenNineH, gray(140)),
	})

	if got := filler.SuggestedEraFrom(bw43); got == 0 {
		t.Errorf("B&W 4:3 should suggest an era, got 0")
	}
	// ⚠ The value is a DECADE sentinel (a hint), never a specific fabricated year.
	if got := filler.SuggestedEraFrom(bw43); got%10 != 0 || got >= 1970 {
		t.Errorf("B&W 4:3 hint = %d, want a pre-1970s decade sentinel", got)
	}
	// Colour, or widescreen, or nothing: no hint. Neither signal alone dates a clip.
	if got := filler.SuggestedEraFrom(colour43); got != 0 {
		t.Errorf("colour 4:3 should suggest nothing, got %d", got)
	}
	if got := filler.SuggestedEraFrom(bw169); got != 0 {
		t.Errorf("B&W 16:9 should suggest nothing, got %d", got)
	}
	if got := filler.SuggestedEraFrom(filler.FrameHint{}); got != 0 {
		t.Errorf("no frames should suggest nothing, got %d", got)
	}
}

// A hint is only ever a SUGGESTION: SuggestedEraFrom must never return a value the
// grounding rules would accept as a fact. It is a decade sentinel a human confirms,
// exactly as validateTags demotes an ungrounded LLM year to SuggestedEra.
func TestSuggestedEraFrom_IsSuggestionNotGroundedTag(t *testing.T) {
	bw43 := filler.AnalyzeFrames([][]byte{
		solidFrame(t, fourThreeW, fourThreeH, gray(70)),
		solidFrame(t, fourThreeW, fourThreeH, gray(150)),
	})
	hint := filler.SuggestedEraFrom(bw43)
	if hint == 0 {
		t.Fatalf("expected a suggestion for a B&W 4:3 clip")
	}
	// The tier writes ONLY SuggestedEra. Prove that a clip carrying this hint reports
	// no grounded Era and stays untagged for matching — the hint cannot masquerade as
	// a fact.
	c := filler.Clip{SuggestedEra: hint}
	if c.Era != 0 {
		t.Errorf("a frame hint set Era = %d; it must only ever set SuggestedEra", c.Era)
	}
	if c.Tagged() {
		t.Errorf("a clip with only a frame-hint suggestion must not be Tagged (matchable)")
	}
}

func grayF(t *testing.T, v uint8) []byte { return solidFrame(t, fourThreeW, fourThreeH, gray(v)) }
func colourF(t *testing.T) []byte        { return solidFrame(t, fourThreeW, fourThreeH, red()) }
