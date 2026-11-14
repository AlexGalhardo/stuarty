// Copyright 2016 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package scan is a scanline rasterizer for polygons. It is the floating-point
// path of golang.org/x/image/vector (version 0.23.0), copied here with these
// changes: only MoveTo, LineTo and ClosePath are kept, the buffer is always
// floating point, and the accumulation into 8-bit coverage is always the
// portable Go loop.
//
// Why a copy: vector.Rasterizer.Draw uses an SSE4.1 accumulation on amd64 and
// the Go loop everywhere else. The two add the float32 areas in a different
// order, so their last bits differ, and a coverage that lands on either side
// of an 8-bit rounding changes a pixel. The bitmaps of engine/raster must be
// byte-equal on every architecture (NFR-010, SDD-004 section 11), so the Go loop
// is used everywhere. The geometry code does not use fused multiply-add (see
// LineTo below), so the Go loop gives the same bits on amd64 and arm64.
//
// The licence of the original is in the file LICENSE of this folder.
package scan

import "math"

// Rasterizer accumulates the signed area of the line segments of polygons, then
// converts it into coverage. Overlapping polygons of the same orientation union
// and opposite ones cut holes (non-zero winding, approximately).
type Rasterizer struct {
	buf    []float32
	w, h   int
	firstX float32
	firstY float32
	penX   float32
	penY   float32
}

// Reset clears the rasterizer and sets its size in pixels.
func (z *Rasterizer) Reset(w, h int) {
	z.w, z.h = w, h
	z.firstX, z.firstY, z.penX, z.penY = 0, 0, 0, 0

	if n := w * h; n > cap(z.buf) {
		z.buf = make([]float32, n)
	} else {
		z.buf = z.buf[:n]
		clear(z.buf)
	}
}

// MoveTo starts a new path and moves the pen to (ax, ay). The coordinates may
// be out of the bounds.
func (z *Rasterizer) MoveTo(ax, ay float32) {
	z.firstX, z.firstY = ax, ay
	z.penX, z.penY = ax, ay
}

// ClosePath closes the current path.
func (z *Rasterizer) ClosePath() {
	z.LineTo(z.firstX, z.firstY)
}

// Coverage writes the coverage of every pixel, 0 to 255, into dst, which holds
// w*h values in rows. It reports false when dst is shorter.
func (z *Rasterizer) Coverage(dst []uint8) bool {
	if len(dst) < len(z.buf) {
		return false
	}

	accumulateOpSrc(dst, z.buf)

	return true
}

func floatingMax(x, y float32) float32 {
	if x > y {
		return x
	}
	return y
}

func floatingMin(x, y float32) float32 {
	if x < y {
		return x
	}
	return y
}

func floatingFloor(x float32) int32 { return int32(math.Floor(float64(x))) }
func floatingCeil(x float32) int32  { return int32(math.Ceil(float64(x))) }

// LineTo adds a line segment, from the pen to (bx, by), and moves the pen to
// (bx, by). The coordinates may be out of the bounds.
func (z *Rasterizer) LineTo(bx, by float32) { //nolint:gocognit // copied unchanged from golang.org/x/image/vector
	ax, ay := z.penX, z.penY
	z.penX, z.penY = bx, by
	dir := float32(1)
	if ay > by {
		dir, ax, ay, bx, by = -1, bx, by, ax, ay
	}
	// Horizontal line segments yield no change in coverage. Almost horizontal
	// segments would yield some change, in ideal math, but the computation
	// further below, involving 1 / (by - ay), is unstable in floating point
	// math, so we treat the segment as if it was perfectly horizontal.
	if by-ay <= 0.000001 {
		return
	}
	dxdy := (bx - ax) / (by - ay)

	x := ax
	y := floatingFloor(ay)
	yMax := floatingCeil(by)
	yMax = min(yMax, int32(z.h)) //nolint:gosec // the canvas is at most MaxPixels, far below 2^31
	width := int32(z.w)          //nolint:gosec // same

	for ; y < yMax; y++ {
		dy := floatingMin(float32(y+1), by) - floatingMax(float32(y), ay)

		// The "float32" in expressions like "float32(foo*bar)" here and below
		// look redundant, since foo and bar already have type float32, but are
		// explicit in order to disable the compiler's Fused Multiply Add (FMA)
		// instruction selection, which can improve performance but can result
		// in different rounding errors in floating point computations.
		//
		// This package aims to have bit-exact identical results across all
		// GOARCHes, and across pure Go code and assembly, so it disables FMA.
		//
		// See the discussion at
		// https://groups.google.com/d/topic/golang-dev/Sti0bl2xUXQ/discussion
		xNext := x + float32(dy*dxdy)
		if y < 0 {
			x = xNext
			continue
		}
		buf := z.buf[y*width:]
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
	// almost256 scales a floating point value in the range [0, 1] to a uint8
	// value in the range [0x00, 0xff].
	//
	// 255 is too small. Floating point math accumulates rounding errors, so a
	// fully covered src value that would in ideal math be float32(1) might be
	// float32(1-ε), and uint8(255 * (1-ε)) would be 0xfe instead of 0xff. The
	// uint8 conversion rounds to zero, not to nearest.
	//
	// 256 is too big. If we multiplied by 256, below, then a fully covered src
	// value of float32(1) would translate to uint8(256 * 1), which can be 0x00
	// instead of the maximal value 0xff.
	//
	// math.Float32bits(almost256) is 0x437fffff.
	almost256 = 255.99998
)

func accumulateOpSrc(dst []uint8, src []float32) {
	// Sanity check that len(dst) >= len(src).
	if len(dst) < len(src) {
		return
	}

	acc := float32(0)
	for i, v := range src {
		acc += v
		a := acc
		if a < 0 {
			a = -a
		}
		if a > 1 {
			a = 1
		}
		dst[i] = uint8(almost256 * a)
	}
}

func clamp(i, width int32) uint {
	if i < 0 {
		return 0
	}
	if i < width {
		return uint(i)
	}
	return uint(width) //nolint:gosec // width is not negative
}
