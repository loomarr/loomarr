//go:build amd64 && !noasm

package raster

import (
	"bytes"
	"fmt"
	"image"
	"image/draw"
	"math/rand/v2"
	"testing"

	"golang.org/x/image/vector"
	"golang.org/x/sys/cpu"
)

// pather is the path-building half of both rasterisers.
type pather interface {
	MoveTo(ax, ay float32)
	LineTo(bx, by float32)
	QuadTo(bx, by, cx, cy float32)
	CubeTo(bx, by, cx, cy, dx, dy float32)
	ClosePath()
}

// randomPath draws a few closed subpaths of lines, quadratics and cubics, reaching a little past
// the mask on every side, as glyphs and shapes clipped by their bounds do.
func randomPath(rng *rand.Rand, w, h int, p pather) {
	pt := func() (float32, float32) {
		return float32(rng.Float64()*1.2-0.1) * float32(w), float32(rng.Float64()*1.2-0.1) * float32(h)
	}
	for range 1 + rng.IntN(4) {
		p.MoveTo(pt())
		for range 2 + rng.IntN(8) {
			switch rng.IntN(3) {
			case 0:
				p.LineTo(pt())
			case 1:
				bx, by := pt()
				cx, cy := pt()
				p.QuadTo(bx, by, cx, cy)
			default:
				bx, by := pt()
				cx, cy := pt()
				dx, dy := pt()
				p.CubeTo(bx, by, cx, cy, dx, dy)
			}
		}
		p.ClosePath()
	}
}

// TestRasterizer_MatchesUpstreamSIMD pins the fork to golang.org/x/image/vector's amd64 SSE4.1
// output, which renders the watermark goldens: the same paths give the same bytes, for both
// operators, over a blank and a painted mask, whole and in part, with fixed and floating point
// math (either side of 512 px) and odd widths. Other architectures run the fork's pure Go, so
// this equality is what makes their renders match amd64's.
func TestRasterizer_MatchesUpstreamSIMD(t *testing.T) {
	if !cpu.X86.HasSSE41 {
		t.Skip("upstream uses its SSE4.1 accumulator only on CPUs that have it")
	}
	sizes := []image.Point{{37, 21}, {129, 64}, {511, 512}, {512, 513}, {513, 97}, {701, 640}, {1031, 333}}
	rng := rand.New(rand.NewPCG(4646, 1))
	for _, size := range sizes {
		for _, op := range []draw.Op{draw.Over, draw.Src} {
			for _, painted := range []bool{false, true} {
				for _, part := range []bool{false, true} {
					seed := rng.Uint64()
					name := fmt.Sprintf("%dx%d/op%d/painted=%t/part=%t", size.X, size.Y, op, painted, part)
					t.Run(name, func(t *testing.T) {
						t.Parallel()
						want := image.NewAlpha(image.Rectangle{Max: size})
						got := image.NewAlpha(image.Rectangle{Max: size})
						if painted {
							for i := range want.Pix {
								want.Pix[i] = uint8(i * 7)
							}
							copy(got.Pix, want.Pix)
						}
						r := want.Rect
						if part {
							r = image.Rect(size.X/5, size.Y/7, size.X-size.X/3, size.Y-1)
						}

						up := vector.NewRasterizer(size.X, size.Y)
						up.DrawOp = op
						randomPath(rand.New(rand.NewPCG(seed, 2)), size.X, size.Y, up)
						up.Draw(want, r, image.Opaque, image.Point{})

						fork := NewRasterizer(size.X, size.Y)
						fork.DrawOp = op
						randomPath(rand.New(rand.NewPCG(seed, 2)), size.X, size.Y, fork)
						fork.Draw(got, r)

						if !bytes.Equal(got.Pix, want.Pix) {
							n := 0
							for i := range got.Pix {
								if got.Pix[i] != want.Pix[i] {
									n++
								}
							}
							t.Errorf("%d of %d pixels differ from upstream", n, len(got.Pix))
						}
					})
				}
			}
		}
	}
}
