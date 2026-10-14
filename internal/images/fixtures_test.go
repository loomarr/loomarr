package images

import (
	"bytes"
	"encoding/binary"
	"image"
	"image/color"
	"image/png"
	"testing"
)

// testImage builds deterministic raster input for service tests. Production image inspection,
// resizing, encoding, placeholder generation, and animation detection all live in loomarr-image.
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
			if x > w/2 && y < h/2 {
				c = color.NRGBA{R: 255, A: 255}
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

// isAnimatedWebP is deliberately test-only. It verifies that the Rust worker emitted an animated
// container without restoring animation interpretation to production Go code.
func isAnimatedWebP(data []byte, mime string) bool {
	if mime != "image/webp" || len(data) < 20 || string(data[:4]) != "RIFF" || string(data[8:12]) != "WEBP" {
		return false
	}
	end := min(uint64(binary.LittleEndian.Uint32(data[4:8]))+8, uint64(len(data)))
	for offset := uint64(12); offset+8 <= end; {
		chunkSize := uint64(binary.LittleEndian.Uint32(data[offset+4 : offset+8]))
		next := offset + 8 + chunkSize + chunkSize%2
		if next <= offset || next > end {
			return false
		}
		chunk := string(data[offset : offset+4])
		if (chunk == "ANIM" && chunkSize >= 6) || (chunk == "ANMF" && chunkSize >= 16) {
			return true
		}
		offset = next
	}
	return false
}
