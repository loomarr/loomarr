// Copyright 2016 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package raster is golang.org/x/image/vector v0.47.0 (commit c437a9130dcb), cut to the opaque
// alpha masks the watermark draws, so that every CPU architecture rounds the same way and the
// watermark render is bit-identical everywhere (#4646).
//
// Upstream rounds differently per architecture in two places. On amd64 its SSE4.1 assembly sums
// the coverage of each row four lanes at a time, while other architectures run a sequential loop.
// Float addition is not associative, so the two orders round differently. And its curve
// flattening (lerp, devSquared) leaves products unconverted, which arm64 fuses into multiply-adds.
// This package is pure Go. Its running sum adds in the SSE4.1 lane order (runningSums), and every
// float product that feeds an addition is converted explicitly, which forbids fusion. The
// watermark goldens are rendered on amd64, and every architecture computes the pixels amd64 does.
// raster_amd64_test.go checks this package against upstream's assembly bit for bit.
//
// The rasterizer's design follows
// https://medium.com/@raphlinus/inside-the-fastest-font-renderer-in-the-world-75ae5270c445
package raster

import (
	"image"
	"image/draw"
	"math"
)

// floatingPointMathThreshold is the width or height above which the rasterizer uses floating point
// math instead of fixed point math: fixed point is faster but overflows at large scales.
const floatingPointMathThreshold = 512

// lerp interpolates between p and q. The explicit float32 conversions forbid a fused multiply-add,
// so every architecture rounds the same.
func lerp(t, px, py, qx, qy float32) (x, y float32) {
	return px + float32(t*(qx-px)), py + float32(t*(qy-py))
}

func clamp(i, width int32) uint {
	if i < 0 {
		return 0
	}
	if i < width {
		return uint(i)
	}
	return uint(width)
}

// NewRasterizer returns a Rasterizer whose mask is bounded by the given width and height.
func NewRasterizer(w, h int) *Rasterizer {
	z := &Rasterizer{}
	z.Reset(w, h)
	return z
}

// Rasterizer is a 2-D vector graphics rasterizer. Its zero value has a zero-sized mask; Reset
// changes the bounds.
type Rasterizer struct {
	// bufF32 or bufU32 holds the individual area values, for floating or fixed point math.
	// bufU32 also holds the cumulative mask when Draw needs one, and bufSum the floating point
	// running sums.
	bufF32 []float32
	bufU32 []uint32
	bufSum []float32

	useFloatingPointMath bool

	size   image.Point
	firstX float32
	firstY float32
	penX   float32
	penY   float32

	// DrawOp is the operator Draw uses. The zero value is draw.Over.
	DrawOp draw.Op
}

// Reset resets the Rasterizer as if NewRasterizer had just returned it, DrawOp included.
func (z *Rasterizer) Reset(w, h int) {
	z.size = image.Point{w, h}
	z.firstX = 0
	z.firstY = 0
	z.penX = 0
	z.penY = 0
	z.DrawOp = draw.Over
	z.setUseFloatingPointMath(w > floatingPointMathThreshold || h > floatingPointMathThreshold)
}

func (z *Rasterizer) setUseFloatingPointMath(b bool) {
	z.useFloatingPointMath = b
	// Make z.bufF32 or z.bufU32 large enough to hold width * height samples, and zero them.
	if z.useFloatingPointMath {
		if n := z.size.X * z.size.Y; n > cap(z.bufF32) {
			z.bufF32 = make([]float32, n)
		} else {
			z.bufF32 = z.bufF32[:n]
			clear(z.bufF32)
		}
	} else {
		if n := z.size.X * z.size.Y; n > cap(z.bufU32) {
			z.bufU32 = make([]uint32, n)
		} else {
			z.bufU32 = z.bufU32[:n]
			clear(z.bufU32)
		}
	}
}

// Size returns the width and height of the mask.
func (z *Rasterizer) Size() image.Point { return z.size }

// Bounds returns the rectangle from (0, 0) to the mask's width and height.
func (z *Rasterizer) Bounds() image.Rectangle { return image.Rectangle{Max: z.size} }

// Pen returns the location of the path-drawing pen: the last argument to the most recent XxxTo
// call.
func (z *Rasterizer) Pen() (x, y float32) { return z.penX, z.penY }

// ClosePath closes the current path.
func (z *Rasterizer) ClosePath() { z.LineTo(z.firstX, z.firstY) }

// MoveTo starts a new path and moves the pen to (ax, ay). The coordinates are allowed to be out of
// the Rasterizer's bounds.
func (z *Rasterizer) MoveTo(ax, ay float32) {
	z.firstX = ax
	z.firstY = ay
	z.penX = ax
	z.penY = ay
}

// LineTo adds a line segment from the pen to (bx, by), and moves the pen to (bx, by). The
// coordinates are allowed to be out of the Rasterizer's bounds.
func (z *Rasterizer) LineTo(bx, by float32) {
	if z.useFloatingPointMath {
		z.floatingLineTo(bx, by)
	} else {
		z.fixedLineTo(bx, by)
	}
}

// QuadTo adds a quadratic Bézier segment from the pen via (bx, by) to (cx, cy), and moves the pen
// to (cx, cy). The coordinates are allowed to be out of the Rasterizer's bounds.
func (z *Rasterizer) QuadTo(bx, by, cx, cy float32) {
	ax, ay := z.penX, z.penY
	devsq := devSquared(ax, ay, bx, by, cx, cy)
	if devsq >= 0.333 {
		const tol = 3
		n := 1 + int(math.Sqrt(math.Sqrt(tol*float64(devsq))))
		t, nInv := float32(0), 1/float32(n)
		for i := 0; i < n-1; i++ {
			t += nInv
			abx, aby := lerp(t, ax, ay, bx, by)
			bcx, bcy := lerp(t, bx, by, cx, cy)
			z.LineTo(lerp(t, abx, aby, bcx, bcy))
		}
	}
	z.LineTo(cx, cy)
}

// CubeTo adds a cubic Bézier segment from the pen via (bx, by) and (cx, cy) to (dx, dy), and moves
// the pen to (dx, dy). The coordinates are allowed to be out of the Rasterizer's bounds.
func (z *Rasterizer) CubeTo(bx, by, cx, cy, dx, dy float32) {
	ax, ay := z.penX, z.penY
	devsq := devSquared(ax, ay, bx, by, dx, dy)
	if devsqAlt := devSquared(ax, ay, cx, cy, dx, dy); devsq < devsqAlt {
		devsq = devsqAlt
	}
	if devsq >= 0.333 {
		const tol = 3
		n := 1 + int(math.Sqrt(math.Sqrt(tol*float64(devsq))))
		t, nInv := float32(0), 1/float32(n)
		for i := 0; i < n-1; i++ {
			t += nInv
			abx, aby := lerp(t, ax, ay, bx, by)
			bcx, bcy := lerp(t, bx, by, cx, cy)
			cdx, cdy := lerp(t, cx, cy, dx, dy)
			abcx, abcy := lerp(t, abx, aby, bcx, bcy)
			bcdx, bcdy := lerp(t, bcx, bcy, cdx, cdy)
			z.LineTo(lerp(t, abcx, abcy, bcdx, bcdy))
		}
	}
	z.LineTo(dx, dy)
}

// devSquared measures how curvy the sequence (ax, ay) to (bx, by) to (cx, cy) is, which sets how
// many line segments approximate a Bézier segment. The explicit float32 conversions forbid a fused
// multiply-add, so every architecture picks the same count.
func devSquared(ax, ay, bx, by, cx, cy float32) float32 {
	devx := ax - float32(2*bx) + cx
	devy := ay - float32(2*by) + cy
	return float32(devx*devx) + float32(devy*devy)
}

// Draw fills r of dst with the path's coverage, an opaque mask combined by DrawOp (draw.Over or
// draw.Src).
func (z *Rasterizer) Draw(dst *image.Alpha, r image.Rectangle) {
	if r == dst.Bounds() && r == z.Bounds() {
		z.drawWhole(dst)
		return
	}
	z.accumulateMask()
	pix := dst.Pix[dst.PixOffset(r.Min.X, r.Min.Y):]
	for y, y1 := 0, r.Max.Y-r.Min.Y; y < y1; y++ {
		for x, x1 := 0, r.Max.X-r.Min.X; x < x1; x++ {
			ma := z.bufU32[y*z.size.X+x]
			i := y*dst.Stride + x
			if z.DrawOp == draw.Over {
				// This formula is like rasterizeOpOver's, simplified for the concrete dst type
				// and opaque src assumption.
				a := 0xffff - ma
				pix[i] = uint8((uint32(pix[i])*0x101*a/0xffff + ma) >> 8)
			} else {
				pix[i] = uint8(ma >> 8)
			}
		}
	}
}

// drawWhole draws when dst, r and the mask share bounds, accumulating straight into dst.Pix.
func (z *Rasterizer) drawWhole(dst *image.Alpha) {
	if !z.useFloatingPointMath {
		if z.DrawOp == draw.Over {
			fixedAccumulateOpOver(dst.Pix, z.bufU32)
		} else {
			fixedAccumulateOpSrc(dst.Pix, z.bufU32)
		}
		return
	}
	sums := z.runningSums()
	if z.DrawOp == draw.Over {
		floatingAccumulateOpOver(dst.Pix, sums)
	} else {
		floatingAccumulateOpSrc(dst.Pix, sums)
	}
}

// accumulateMask fills bufU32 with the cumulative coverage of each pixel, 0 to 0xffff.
func (z *Rasterizer) accumulateMask() {
	if !z.useFloatingPointMath {
		fixedAccumulateMask(z.bufU32)
		return
	}
	sums := z.runningSums()
	if n := z.size.X * z.size.Y; n > cap(z.bufU32) {
		z.bufU32 = make([]uint32, n)
	} else {
		z.bufU32 = z.bufU32[:n]
	}
	floatingAccumulateMask(z.bufU32, sums)
}

// runningSums returns the running sums of bufF32 in bufSum.
func (z *Rasterizer) runningSums() []float32 {
	if n := len(z.bufF32); n > cap(z.bufSum) {
		z.bufSum = make([]float32, n)
	} else {
		z.bufSum = z.bufSum[:n]
	}
	runningSums(z.bufSum, z.bufF32)
	return z.bufSum
}
