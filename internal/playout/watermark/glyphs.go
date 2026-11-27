package watermark

import (
	"image"
	"image/draw"

	"golang.org/x/image/font"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/font/sfnt"
	"golang.org/x/image/math/fixed"

	"github.com/loomarr/loomarr/internal/playout/watermark/raster"
)

// glyphFace is an opentype face whose glyphs are rasterised by the raster package, so text
// renders the same pixels on every CPU architecture (#4646). Metrics, kerning, bounds and
// advances come from the opentype face. Glyph is opentype's Face.Glyph (x/image v0.47.0) with
// the rasteriser swapped, so the placement stays exactly opentype's.
type glyphFace struct {
	font.Face
	f     *sfnt.Font
	scale fixed.Int26_6

	buf  sfnt.Buffer
	rast raster.Rasterizer
	mask image.Alpha
}

// newGlyphFace opens f at size points, 72 DPI and unhinted.
func newGlyphFace(f *sfnt.Font, size float64) (*glyphFace, error) {
	opts := &opentype.FaceOptions{Size: size, DPI: 72, Hinting: font.HintingNone}
	face, err := opentype.NewFace(f, opts)
	if err != nil {
		return nil, err
	}
	// This is opentype.NewFace's own scale.
	return &glyphFace{Face: face, f: f, scale: fixed.Int26_6(0.5 + (opts.Size * opts.DPI * 64 / 72))}, nil
}

// Glyph satisfies font.Face: the mask is valid until the next call.
func (g *glyphFace) Glyph(dot fixed.Point26_6, r rune) (dr image.Rectangle, mask image.Image, maskp image.Point, advance fixed.Int26_6, ok bool) {
	x, err := g.f.GlyphIndex(&g.buf, r)
	if err != nil {
		return image.Rectangle{}, nil, image.Point{}, 0, false
	}
	// GlyphAdvance comes before LoadGlyph, whose segments are invalid once the buffer is reused.
	advance, err = g.f.GlyphAdvance(&g.buf, x, g.scale, font.HintingNone)
	if err != nil {
		return image.Rectangle{}, nil, image.Point{}, 0, false
	}
	segments, err := g.f.LoadGlyph(&g.buf, x, g.scale, nil)
	if err != nil {
		return image.Rectangle{}, nil, image.Point{}, 0, false
	}

	// The glyph's sub-pixel bounds at the dot, quantised to whole pixels.
	dBounds := segments.Bounds().Add(dot)
	dr.Min.X = dBounds.Min.X.Floor()
	dr.Min.Y = dBounds.Min.Y.Floor()
	dr.Max.X = dBounds.Max.X.Ceil()
	dr.Max.Y = dBounds.Max.Y.Ceil()
	width, height := dr.Dx(), dr.Dy()
	if width < 0 || height < 0 {
		return image.Rectangle{}, nil, image.Point{}, 0, false
	}
	// The bias moves glyph space, origin at the dot, into the mask, origin at dr.Min.
	biasX := dot.X - fixed.Int26_6(dr.Min.X<<6)
	biasY := dot.Y - fixed.Int26_6(dr.Min.Y<<6)

	nPixels := width * height
	if cap(g.mask.Pix) < nPixels {
		g.mask.Pix = make([]uint8, 2*nPixels)
	}
	g.mask.Pix = g.mask.Pix[:nPixels]
	g.mask.Stride = width
	g.mask.Rect = image.Rect(0, 0, width, height)

	// The biased segments convert from 26.6 fixed point to float32 pixels.
	p := func(q fixed.Point26_6) (float32, float32) {
		return float32(q.X+biasX) / 64, float32(q.Y+biasY) / 64
	}
	g.rast.Reset(width, height)
	g.rast.DrawOp = draw.Src
	for _, seg := range segments {
		switch seg.Op {
		case sfnt.SegmentOpMoveTo:
			g.rast.MoveTo(p(seg.Args[0]))
		case sfnt.SegmentOpLineTo:
			g.rast.LineTo(p(seg.Args[0]))
		case sfnt.SegmentOpQuadTo:
			bx, by := p(seg.Args[0])
			cx, cy := p(seg.Args[1])
			g.rast.QuadTo(bx, by, cx, cy)
		case sfnt.SegmentOpCubeTo:
			bx, by := p(seg.Args[0])
			cx, cy := p(seg.Args[1])
			dx, dy := p(seg.Args[2])
			g.rast.CubeTo(bx, by, cx, cy, dx, dy)
		}
	}
	g.rast.Draw(&g.mask, g.mask.Bounds())
	return dr, &g.mask, g.mask.Rect.Min, advance, x != 0
}
