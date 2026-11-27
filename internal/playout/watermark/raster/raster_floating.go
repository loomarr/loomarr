// Copyright 2016 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package raster

// This file is the floating point math implementation of the rasterizer.

import (
	"math"
)

func floatingFloor(x float32) int32 { return int32(math.Floor(float64(x))) }
func floatingCeil(x float32) int32  { return int32(math.Ceil(float64(x))) }

func (z *Rasterizer) floatingLineTo(bx, by float32) {
	ax, ay := z.penX, z.penY
	z.penX, z.penY = bx, by
	dir := float32(1)
	if ay > by {
		dir, ax, ay, bx, by = -1, bx, by, ax, ay
	}
	// Horizontal line segments yield no change in coverage. Almost horizontal segments would
	// yield some change, in ideal math, but the computation further below, involving
	// 1 / (by - ay), is unstable in floating point math, so the segment counts as perfectly
	// horizontal.
	if by-ay <= 0.000001 {
		return
	}
	dxdy := (bx - ax) / (by - ay)

	x := ax
	y := floatingFloor(ay)
	yMax := floatingCeil(by)
	if yMax > int32(z.size.Y) {
		yMax = int32(z.size.Y)
	}
	width := int32(z.size.X)

	for ; y < yMax; y++ {
		dy := min(float32(y+1), by) - max(float32(y), ay)

		// The "float32" in expressions like "float32(foo*bar)" here and below looks redundant,
		// since foo and bar already have type float32, but it forbids a fused multiply-add, which
		// rounds differently, so every architecture computes the same coverage.
		xNext := x + float32(dy*dxdy)
		if y < 0 {
			x = xNext
			continue
		}
		buf := z.bufF32[y*width:]
		d := float32(dy * dir)
		x0, x1 := x, xNext
		if x > xNext {
			x0, x1 = x1, x0
		}
		x0i := floatingFloor(x0)
		x0Floor := float32(x0i)
		x1i := floatingCeil(x1)
		x1Ceil := float32(x1i)

		if x1i <= x0i+1 {
			xmf := float32(0.5*(x+xNext)) - x0Floor
			if i := clamp(x0i+0, width); i < uint(len(buf)) {
				buf[i] += d - float32(d*xmf)
			}
			if i := clamp(x0i+1, width); i < uint(len(buf)) {
				buf[i] += float32(d * xmf)
			}
		} else {
			s := 1 / (x1 - x0)
			x0f := x0 - x0Floor
			oneMinusX0f := 1 - x0f
			a0 := float32(0.5 * s * oneMinusX0f * oneMinusX0f)
			x1f := x1 - x1Ceil + 1
			am := float32(0.5 * s * x1f * x1f)

			if i := clamp(x0i, width); i < uint(len(buf)) {
				buf[i] += float32(d * a0)
			}

			if x1i == x0i+2 {
				if i := clamp(x0i+1, width); i < uint(len(buf)) {
					buf[i] += float32(d * (1 - a0 - am))
				}
			} else {
				a1 := float32(s * (1.5 - x0f))
				if i := clamp(x0i+1, width); i < uint(len(buf)) {
					buf[i] += float32(d * (a1 - a0))
				}
				dTimesS := float32(d * s)
				for xi := x0i + 2; xi < x1i-1; xi++ {
					if i := clamp(xi, width); i < uint(len(buf)) {
						buf[i] += dTimesS
					}
				}
				a2 := a1 + float32(s*float32(x1i-x0i-3))
				if i := clamp(x1i-1, width); i < uint(len(buf)) {
					buf[i] += float32(d * (1 - a2 - am))
				}
			}

			if i := clamp(x1i, width); i < uint(len(buf)) {
				buf[i] += float32(d * am)
			}
		}

		x = xNext
	}
}

const (
	// almost256 scales a floating point value in the range [0, 1] to a uint8 value in the range
	// [0x00, 0xff].
	//
	// 255 is too small. Floating point math accumulates rounding errors, so a fully covered src
	// value that would in ideal math be float32(1) might be float32(1-ε), and uint8(255 * (1-ε))
	// would be 0xfe instead of 0xff. The uint8 conversion rounds to zero, not to nearest.
	//
	// 256 is too big. A fully covered src value of float32(1) would translate to uint8(256 * 1),
	// which can be 0x00 instead of the maximal value 0xff.
	//
	// math.Float32bits(almost256) is 0x437fffff.
	almost256 = 255.99998

	// almost65536 scales a floating point value in the range [0, 1] to a uint16 value in the
	// range [0x0000, 0xffff].
	//
	// math.Float32bits(almost65536) is 0x477fffff.
	almost65536 = almost256 * 256
)

// runningSums writes into sums each running sum of src, the cumulative area of every pixel. The
// sum runs across the whole flat buffer, row after row, as upstream's does: each row's areas net
// to zero, so the total carries no coverage into the next row. It adds in the order of upstream's
// amd64 SSE4.1 accumulator, so that every architecture rounds as the goldens' amd64 renders do.
// Float addition is not associative, and a sequential sum rounds differently. That accumulator
// takes four values at a time: it adds the neighbouring pairs, then each lane the sum two lanes
// before it, then the last total of the previous four. A tail of fewer than four values at the
// end of the buffer adds one value at a time.
func runningSums(sums, src []float32) {
	var offset float32
	i := 0
	for ; i+4 <= len(src); i += 4 {
		s0, s1, s2, s3 := src[i], src[i+1], src[i+2], src[i+3]
		x0, x1, x2, x3 := s0, s1+s0, s2+s1, s3+s2
		x2, x3 = x2+x0, x3+x1
		sums[i], sums[i+1], sums[i+2], sums[i+3] = x0+offset, x1+offset, x2+offset, x3+offset
		offset = sums[i+3]
	}
	for ; i < len(src); i++ {
		offset = src[i] + offset
		sums[i] = offset
	}
}

// coverage is a running sum's coverage, its absolute value clamped to 1.
func coverage(acc float32) float32 {
	if acc < 0 {
		acc = -acc
	}
	if acc > 1 {
		acc = 1
	}
	return acc
}

func floatingAccumulateOpOver(dst []uint8, sums []float32) {
	// Sanity check that len(dst) >= len(sums).
	if len(dst) < len(sums) {
		return
	}
	for i, acc := range sums {
		// This algorithm comes from the standard library's image/draw package.
		dstA := uint32(dst[i]) * 0x101
		maskA := uint32(almost65536 * coverage(acc))
		outA := dstA*(0xffff-maskA)/0xffff + maskA
		dst[i] = uint8(outA >> 8)
	}
}

func floatingAccumulateOpSrc(dst []uint8, sums []float32) {
	// Sanity check that len(dst) >= len(sums).
	if len(dst) < len(sums) {
		return
	}
	for i, acc := range sums {
		dst[i] = uint8(almost256 * coverage(acc))
	}
}

func floatingAccumulateMask(dst []uint32, sums []float32) {
	// Sanity check that len(dst) >= len(sums).
	if len(dst) < len(sums) {
		return
	}
	for i, acc := range sums {
		dst[i] = uint32(almost65536 * coverage(acc))
	}
}
