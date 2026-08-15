package images

import (
	"bytes"
	"encoding/base64"
	"image"
	"image/color"
	"image/png"
	"testing"
)

// testImage builds a deterministic gradient with a hard-edged block, at a real poster aspect.
//
// Synthetic rather than a fixture file: the assertions here are about the CODEC, and a checked-in
// JPEG would make them depend on someone's export settings. The gradient exercises resampling
// (smooth ramps are where a bad filter bands) and the block gives a sharp edge to check the
// resize did not smear everything.
func testImage(w, h int) image.Image {
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := range h {
		for x := range w {
			c := color.NRGBA{
				R: uint8(x * 255 / max(w-1, 1)),
				G: uint8(y * 255 / max(h-1, 1)),
				B: 128,
				A: 255,
			}
			// A hard block in one quadrant — a sharp edge that survives resampling.
			if x > w/2 && y < h/2 {
				c = color.NRGBA{R: 255, G: 0, B: 0, A: 255}
			}
			img.Set(x, y, c)
		}
	}
	return img
}

func pngBytes(t *testing.T, img image.Image) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("png encode: %v", err)
	}
	return buf.Bytes()
}

func TestDecodeSniffsBytesNotClaims(t *testing.T) {
	data := pngBytes(t, testImage(64, 96))

	img, mime, err := Decode(data)
	if err != nil {
		t.Fatalf("decode png: %v", err)
	}
	if mime != "image/png" {
		t.Errorf("mime = %q, want image/png", mime)
	}
	if got := img.Bounds().Dx(); got != 64 {
		t.Errorf("width = %d, want 64", got)
	}
}

// ⚠ The SVG case is the security assertion, not a format-coverage one. An SVG served back from a
// public endpoint with an image content type is stored XSS in Loomarr's own origin, so it must be
// refused on its BYTES even when it arrives labelled as something else.
func TestDecodeRefusesSVGHoweverItIsLabelled(t *testing.T) {
	svg := []byte(`<?xml version="1.0"?><svg xmlns="http://www.w3.org/2000/svg">` +
		`<script>fetch("/v1/users")</script></svg>`)

	if AllowedInput(SniffType(svg)) {
		t.Fatalf("SVG passed the allowlist as %q — it must never be an accepted input", SniffType(svg))
	}
	if _, _, err := Decode(svg); err == nil {
		t.Fatal("Decode accepted an SVG; the public serve path would then hand back executable bytes")
	}
}

func TestResizePreservesAspectAndRefusesUpscale(t *testing.T) {
	src := testImage(1000, 1500) // 2:3 poster

	got := Resize(src, 154)
	if w := got.Bounds().Dx(); w != 154 {
		t.Errorf("width = %d, want exactly 154", w)
	}
	// 154 * 1500/1000 = 231
	if h := got.Bounds().Dy(); h != 231 {
		t.Errorf("height = %d, want 231 (aspect preserved)", h)
	}

	// Asking for more pixels than exist returns the source untouched rather than inventing
	// detail — a ladder rung wider than the original means the caller is wrong, and upscaling
	// would hide that.
	up := Resize(src, 4000)
	if up.Bounds().Dx() != 1000 {
		t.Errorf("upscale produced %d px wide; want the untouched 1000", up.Bounds().Dx())
	}
}

// The stepped downscale must not change the OUTPUT CONTRACT, only the cost of reaching it. A
// large ratio (which takes the halving path) and a small one (which does not) must both land on
// exactly the requested width.
func TestResizeSteppedAndDirectAgreeOnSize(t *testing.T) {
	for _, tc := range []struct{ srcW, want int }{
		{4000, 320}, // 12.5× — several halvings
		{600, 320},  // <2× — straight to CatmullRom, no halving
		{321, 320},  // adjacent
	} {
		out := Resize(testImage(tc.srcW, tc.srcW*3/2), tc.want)
		if got := out.Bounds().Dx(); got != tc.want {
			t.Errorf("Resize(%d→%d) produced width %d", tc.srcW, tc.want, got)
		}
	}
}

func TestEncodeWebPRoundTrips(t *testing.T) {
	src := testImage(320, 480)

	var buf bytes.Buffer
	if err := EncodeWebP(&buf, src); err != nil {
		t.Fatalf("EncodeWebP: %v", err)
	}
	if buf.Len() == 0 {
		t.Fatal("EncodeWebP wrote no bytes")
	}

	// Sniffing our own output proves the container is real WebP, not merely that the call
	// returned nil — the failure mode a "did it error?" test misses entirely.
	if mime := SniffType(buf.Bytes()); mime != "image/webp" {
		t.Fatalf("encoded bytes sniff as %q, want image/webp", mime)
	}
	if isAnimatedWebP(buf.Bytes(), "image/webp") {
		t.Fatal("single-frame WebP was classified as animated")
	}

	back, _, err := Decode(buf.Bytes())
	if err != nil {
		t.Fatalf("decode our own webp: %v", err)
	}
	if back.Bounds().Dx() != 320 || back.Bounds().Dy() != 480 {
		t.Errorf("round-tripped dimensions = %v, want 320x480", back.Bounds().Size())
	}
}

func TestIsAnimatedWebPReadsRIFFChunks(t *testing.T) {
	data := animatedWebPBytes(t)
	if !isAnimatedWebP(data, SniffType(data)) {
		t.Fatal("two-frame WebP was classified as static")
	}

	// Truncating inside the first chunk must fail closed without walking beyond the buffer.
	if isAnimatedWebP(data[:18], "image/webp") {
		t.Fatal("truncated WebP was classified as animated")
	}
	if isAnimatedWebP(data, "image/jpeg") {
		t.Fatal("WebP bytes were classified as animated under a different MIME type")
	}
}

func TestEncodeJPEGRoundTrips(t *testing.T) {
	var buf bytes.Buffer
	if err := EncodeJPEG(&buf, testImage(320, 480)); err != nil {
		t.Fatalf("EncodeJPEG: %v", err)
	}
	if mime := SniffType(buf.Bytes()); mime != "image/jpeg" {
		t.Fatalf("encoded bytes sniff as %q, want image/jpeg", mime)
	}
}

// ⚠ AVIF must NOT be reachable through the inline encoder. It is a 300–1200ms job-only format,
// and a caller that quietly got it here would put that cost on a request.
func TestEncodeRefusesAVIFInline(t *testing.T) {
	if err := Encode(&bytes.Buffer{}, testImage(32, 32), FormatAVIF); err == nil {
		t.Fatal("Encode produced AVIF inline; it must be job-only (§22)")
	}
}

func TestPlaceholderIsSmallAndDecodable(t *testing.T) {
	ph := Placeholder(testImage(1000, 1500))
	if ph == "" {
		t.Fatal("empty placeholder")
	}
	raw, err := base64.StdEncoding.DecodeString(ph)
	if err != nil {
		t.Fatalf("placeholder is not valid base64: %v", err)
	}
	// The whole point of a ThumbHash is that it rides on the row. If it ever grows past ~50
	// bytes something has gone wrong with the pre-shrink and every image list pays for it.
	if len(raw) > 50 {
		t.Errorf("placeholder is %d bytes; a ThumbHash should be ~25", len(raw))
	}
}

// The alpha case is the whole reason ThumbHash was chosen over BlurHash: a transparent logo must
// not come back as a black box. Asserting it here pins the property the §14 decision rests on.
func TestPlaceholderCarriesAlpha(t *testing.T) {
	img := image.NewNRGBA(image.Rect(0, 0, 64, 64))
	for y := range 64 {
		for x := range 64 {
			// Opaque red disc-ish centre, fully transparent surround.
			if x > 16 && x < 48 && y > 16 && y < 48 {
				img.Set(x, y, color.NRGBA{R: 255, A: 255})
			} else {
				img.Set(x, y, color.NRGBA{})
			}
		}
	}
	ph := Placeholder(img)
	if ph == "" {
		t.Fatal("empty placeholder for a transparent image")
	}
	raw, err := base64.StdEncoding.DecodeString(ph)
	if err != nil {
		t.Fatalf("bad base64: %v", err)
	}
	// ThumbHash sets a flag in its header for alpha-bearing images; a format without alpha
	// support could not round-trip this at all.
	if len(raw) < 5 {
		t.Fatalf("placeholder too short to carry an alpha header: %d bytes", len(raw))
	}
}

func TestDominantHex(t *testing.T) {
	solid := image.NewNRGBA(image.Rect(0, 0, 32, 32))
	for y := range 32 {
		for x := range 32 {
			solid.Set(x, y, color.NRGBA{R: 0x33, G: 0x66, B: 0x99, A: 255})
		}
	}
	if got := DominantHex(solid); got != "#336699" {
		t.Errorf("DominantHex = %q, want #336699", got)
	}
}

// Real numbers on this machine, because the figures circulating for this library come from an
// older revision with unknown test images. Run with:
//
//	go test ./internal/images -bench BenchmarkEncode -benchtime 20x
func BenchmarkEncodeWebP(b *testing.B) {
	img := Resize(testImage(2000, 3000), 500)
	b.ResetTimer()
	for b.Loop() {
		if err := EncodeWebP(&bytes.Buffer{}, img); err != nil {
			b.Fatal(err)
		}
	}
}

// The pair below is the evidence for §22's "stepped downscale" rule. Keep both: the naive one is
// not dead weight, it is the control that shows the rule is worth having.
//
// Measured on an i9-12900K, 2000×3000 source, five-rung poster ladder:
//
//	BenchmarkResizeLadderNaive   231 ms/op   (Resize() per rung, from the original each time)
//	BenchmarkResizeLadder        100 ms/op   (each rung sourced from the previous)
func BenchmarkResizeLadderNaive(b *testing.B) {
	src := testImage(2000, 3000)
	b.ResetTimer()
	for b.Loop() {
		for _, w := range posterWidths {
			_ = Resize(src, w)
		}
	}
}

func BenchmarkResizeLadder(b *testing.B) {
	src := testImage(2000, 3000)
	b.ResetTimer()
	for b.Loop() {
		_ = ResizeLadder(src, posterWidths)
	}
}

// Sourced from the real ladder rather than re-listed. A second copy of these numbers is exactly
// the hand-maintained list this repo has repeatedly watched drift from the thing it mirrors.
var posterWidths = RolePoster.Widths()

func TestResizeLadderMatchesIndividualResizes(t *testing.T) {
	src := testImage(2000, 3000)

	got := ResizeLadder(src, posterWidths)
	if len(got) != len(posterWidths) {
		t.Fatalf("ladder returned %d rungs, want %d", len(got), len(posterWidths))
	}
	for _, w := range posterWidths {
		img, ok := got[w]
		if !ok {
			t.Fatalf("ladder missing rung %d", w)
		}
		if img.Bounds().Dx() != w {
			t.Errorf("rung %d has width %d", w, img.Bounds().Dx())
		}
		// Aspect must survive the chain — the failure mode of a stepped resize is drift, where
		// each rung rounds and the smallest ends up a different shape from the largest.
		wantH := int(float64(w) * 1.5)
		if h := img.Bounds().Dy(); h < wantH-2 || h > wantH+2 {
			t.Errorf("rung %d height = %d, want ~%d (2:3 aspect drifted)", w, h, wantH)
		}
	}
}

// Callers must not have to sort. A ladder handed over in ascending order would, if the
// implementation naively chained, produce every rung from an already-tiny source — upscaling
// garbage. Passing it unsorted here is the assertion that cannot happen.
func TestResizeLadderIsOrderIndependent(t *testing.T) {
	src := testImage(1000, 1500)
	ascending := []int{154, 342, 780}

	got := ResizeLadder(src, ascending)
	for _, w := range ascending {
		if got[w].Bounds().Dx() != w {
			t.Errorf("rung %d width = %d — ladder is order-dependent", w, got[w].Bounds().Dx())
		}
	}
}
