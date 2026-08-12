package filler_test

import (
	"bytes"
	"image"
	"image/color"
	"image/jpeg"
	"testing"
)

// A solid 4:3 grayscale JPEG, for the vision stage's tests.
//
// ⚠ A LOCAL copy rather than an import. The original lived beside the frame-heuristic tests,
// which moved to internal/mediatools with the analysis they cover — and a test helper is not a
// reason to export something from a production package, nor to make one test package import
// another's internals. Six lines duplicated beats either.
//
// These frames only need to BE decodable frames: the vision stage under test never inspects
// their pixels, it passes them to a scripted double. The 4:3 grayscale shape is kept because one
// case asserts that a clip which would trip the pre-1970s frame hint on its own still does not
// get a grounded Era from the vision path.
func grayF(t *testing.T, v uint8) []byte {
	t.Helper()
	const w, h = 320, 240
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := range h {
		for x := range w {
			img.Set(x, y, color.RGBA{R: v, G: v, B: v, A: 255})
		}
	}
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, nil); err != nil {
		t.Fatalf("encode test frame: %v", err)
	}
	return buf.Bytes()
}
