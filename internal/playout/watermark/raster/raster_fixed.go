// Copyright 2016 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package raster

// This file is the fixed point math implementation of the rasterizer. Its accumulation is integer
// arithmetic, which is associative, so every architecture and summing order agrees.

const (
	// ϕ is the number of binary digits after the fixed point: 22.9 fixed point math.
	ϕ = 9

	fxOne          int1ϕ = 1 << ϕ
	fxOneAndAHalf  int1ϕ = 1<<ϕ + 1<<(ϕ-1)
	fxOneMinusIota int1ϕ = 1<<ϕ - 1 // Used for rounding up.
)

// int1ϕ is a signed fixed-point number with 1*ϕ binary digits after the fixed point.
type int1ϕ int32

// int2ϕ is a signed fixed-point number with 2*ϕ binary digits after the fixed point. The
// Rasterizer's bufU32, nominally []uint32, holds int2ϕ values during fixedLineTo.
type int2ϕ int32

func fixedFloor(x int1ϕ) int32 { return int32(x >> ϕ) }
func fixedCeil(x int1ϕ) int32  { return int32((x + fxOneMinusIota) >> ϕ) }

func (z *Rasterizer) fixedLineTo(bx, by float32) {
	ax, ay := z.penX, z.penY
	z.penX, z.penY = bx, by
	dir := int1ϕ(1)
	if ay > by {
		dir, ax, ay, bx, by = -1, bx, by, ax, ay
	}
	// Horizontal line segments yield no change in coverage. Almost horizontal segments would
	// yield some change, in ideal math, but the computation further below, involving
	// 1 / (by - ay), is unstable in fixed point math, so the segment counts as perfectly
	// horizontal.
	if by-ay <= 0.000001 {
		return
	}
	dxdy := (bx - ax) / (by - ay)

	ayϕ := int1ϕ(ay * float32(fxOne))
	byϕ := int1ϕ(by * float32(fxOne))

	x := int1ϕ(ax * float32(fxOne))
	y := fixedFloor(ayϕ)
	yMax := fixedCeil(byϕ)
	if yMax > int32(z.size.Y) {
		yMax = int32(z.size.Y)
	}
	width := int32(z.size.X)

	for ; y < yMax; y++ {
		dy := min(int1ϕ(y+1)<<ϕ, byϕ) - max(int1ϕ(y)<<ϕ, ayϕ)
		xNext := x + int1ϕ(float32(dy)*dxdy)
		if y < 0 {
			x = xNext
			continue
		}
		buf := z.bufU32[y*width:]
		d := dy * dir // d ranges up to ±1<<(1*ϕ).
		x0, x1 := x, xNext
		if x > xNext {
			x0, x1 = x1, x0
		}
		x0i := fixedFloor(x0)
		x0Floor := int1ϕ(x0i) << ϕ
		x1i := fixedCeil(x1)
		x1Ceil := int1ϕ(x1i) << ϕ

		if x1i <= x0i+1 {
			xmf := (x+xNext)>>1 - x0Floor
			if i := clamp(x0i+0, width); i < uint(len(buf)) {
				buf[i] += uint32(d * (fxOne - xmf))
			}
			if i := clamp(x0i+1, width); i < uint(len(buf)) {
				buf[i] += uint32(d * xmf)
			}
		} else {
			oneOverS := x1 - x0
			twoOverS := 2 * oneOverS
			x0f := x0 - x0Floor
			oneMinusX0f := fxOne - x0f
			oneMinusX0fSquared := oneMinusX0f * oneMinusX0f
			x1f := x1 - x1Ceil + fxOne
			x1fSquared := x1f * x1f

			// The division by oneOverS comes as late as possible, which minimises rounding
			// errors. In ideal math, a0 = ((oneMinusX0f * oneMinusX0f) >> 1) / oneOverS and
			// am = ((x1f * x1f) >> 1) / oneOverS, as in the floating point version.

			if i := clamp(x0i, width); i < uint(len(buf)) {
				// In ideal math: buf[i] += uint32(d * a0)
				D := oneMinusX0fSquared // D ranges up to ±1<<(2*ϕ).
				D *= d                  // D ranges up to ±1<<(3*ϕ).
				D /= twoOverS
				buf[i] += uint32(D)
			}

			if x1i == x0i+2 {
				if i := clamp(x0i+1, width); i < uint(len(buf)) {
					// In ideal math: buf[i] += uint32(d * (fxOne - a0 - am))
					D := twoOverS<<ϕ - oneMinusX0fSquared - x1fSquared // D ranges up to ±1<<(2*ϕ+2).
					D *= d                                             // D ranges up to ±1<<(3*ϕ+2).
					D /= twoOverS
					buf[i] += uint32(D)
				}
			} else {
				// In ideal math, a1 = ((fxOneAndAHalf - x0f) << ϕ) / oneOverS.

				if i := clamp(x0i+1, width); i < uint(len(buf)) {
					// In ideal math: buf[i] += uint32(d * (a1 - a0)), computed as
					// uint32(A * d / twoOverS) where A = (a1 - a0) * twoOverS, which
					// ranges up to ±1<<(2*ϕ+2).
					D := (fxOneAndAHalf-x0f)<<(ϕ+1) - oneMinusX0fSquared // D ranges up to ±1<<(2*ϕ+2).
					D *= d                                               // D ranges up to ±1<<(3*ϕ+2).
					D /= twoOverS
					buf[i] += uint32(D)
				}
				dTimesS := uint32((d << (2 * ϕ)) / oneOverS)
				for xi := x0i + 2; xi < x1i-1; xi++ {
					if i := clamp(xi, width); i < uint(len(buf)) {
						buf[i] += dTimesS
					}
				}

				// In ideal math, a2 = a1 + (s * (x1i - x0i - 3)) and
				// buf[x1i-1] += uint32(d * (fxOne - a2 - am)).
				if i := clamp(x1i-1, width); i < uint(len(buf)) {
					const C = 1<<(ϕ+2) - fxOneAndAHalf<<1
					D := x1f<<1 + C // D ranges up to ±1<<(1*ϕ+2).
					D <<= ϕ         // D ranges up to ±1<<(2*ϕ+2).
					D -= x1fSquared // D ranges up to ±1<<(2*ϕ+3).
					D *= d          // D ranges up to ±1<<(3*ϕ+3).
					D /= twoOverS
					buf[i] += uint32(D)
				}
			}

			if i := clamp(x1i, width); i < uint(len(buf)) {
				// In ideal math: buf[i] += uint32(d * am)
				D := x1fSquared // D ranges up to ±1<<(2*ϕ).
				D *= d          // D ranges up to ±1<<(3*ϕ).
				D /= twoOverS
				buf[i] += uint32(D)
			}
		}

		x = xNext
	}
}

func fixedAccumulateOpOver(dst []uint8, src []uint32) {
	// Sanity check that len(dst) >= len(src).
	if len(dst) < len(src) {
		return
	}

	acc := int2ϕ(0)
	for i, v := range src {
		acc += int2ϕ(v)
		a := acc
		if a < 0 {
			a = -a
		}
		a >>= 2*ϕ - 16
		if a > 0xffff {
			a = 0xffff
		}
		// This algorithm comes from the standard library's image/draw package.
		dstA := uint32(dst[i]) * 0x101
		maskA := uint32(a)
		outA := dstA*(0xffff-maskA)/0xffff + maskA
		dst[i] = uint8(outA >> 8)
	}
}

func fixedAccumulateOpSrc(dst []uint8, src []uint32) {
	// Sanity check that len(dst) >= len(src).
	if len(dst) < len(src) {
		return
	}

	acc := int2ϕ(0)
	for i, v := range src {
		acc += int2ϕ(v)
		a := acc
		if a < 0 {
			a = -a
		}
		a >>= 2*ϕ - 8
		if a > 0xff {
			a = 0xff
		}
		dst[i] = uint8(a)
	}
}

func fixedAccumulateMask(buf []uint32) {
	acc := int2ϕ(0)
	for i, v := range buf {
		acc += int2ϕ(v)
		a := acc
		if a < 0 {
			a = -a
		}
		a >>= 2*ϕ - 16
		if a > 0xffff {
			a = 0xffff
		}
		buf[i] = uint32(a)
	}
}
