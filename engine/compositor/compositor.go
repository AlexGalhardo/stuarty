// Package compositor draws a frame plan into an RGBA frame (SDD-004 section 6). It is the export
// executor and the truth: FFmpeg decodes and encodes, nothing else places a pixel.
//
// Every pixel is computed with integer arithmetic from numbers that are rounded once per layer, so
// the result is the same on every architecture (NFR-010) and does not depend on how many bands the
// work is split into (SDD-004 section 12, rule 3).
package compositor

import (
	"errors"
	"fmt"
	"image"
	"math"
	"runtime"
	"strconv"
	"sync"

	"github.com/AlexGalhardo/stuarty/core/plan"
	"github.com/AlexGalhardo/stuarty/engine/bitmap"
)

// MaxBands is the largest number of bands (SDD-004 section 12, rule 5).
const MaxBands = 8

// bandMinPixels is the work under which splitting into goroutines costs more than it saves. It
// never changes a pixel.
const bandMinPixels = 1 << 14

// Image is a source of pixels: RGBA, 8 bits, premultiplied alpha, W*H*4 bytes. The compositor never
// writes to it.
type Image struct {
	Pix  []byte
	W, H int
	// Opaque says every alpha is 255, which lets a copy skip blending (a decoded video frame).
	Opaque bool
	// FullW and FullH, when not zero, say the pixels are only the visible window of a layer drawn FullW by
	// FullH, whose top-left corner is at (OffX, OffY) of that full image. The decoder delivers a window when
	// a cover fit lets most of the layer fall outside the frame, so it does not scale pixels nobody sees.
	// The compositor copies a window only for a layer that is not rotated, flipped, blurred, masked or
	// changed by an effect; the frame source gives a window to no other.
	FullW, FullH, OffX, OffY int
}

// MediaSource returns the pixels of a media layer: the decoded frame of the source or the decoded
// image, already cropped. When its size is the rounded size of the drawn layer and the layer is not
// rotated, the compositor copies it (SDD-004 section 6 rule 3); otherwise it samples it.
type MediaSource func(l *plan.Layer) (Image, error)

// BitmapSource renders (or fetches from a cache) the bitmap of a text, caption or shape layer.
type BitmapSource func(spec plan.BitmapSpec) (bitmap.Bitmap, error)

// ErrSource is wrapped by the error of a layer whose pixels could not be had.
var ErrSource = errors.New("compositor: a layer has no pixels")

// Compositor draws frames. It keeps scratch memory between frames, so one value serves one
// goroutine at a time.
type Compositor struct {
	Media   MediaSource
	Bitmaps BitmapSource
	// LUTs returns the lookup table of a lut asset; it is needed only when a layer has a lut.
	LUTs LUTSource
	// Bands is the number of bands for per-pixel work; zero is DefaultBands.
	Bands int

	scratch []byte
	// flipBuf holds the mirrored pixels of a flipped layer.
	flipBuf []byte
	// blurA holds the blurred pixels and blurB the rows between the two directions of a blur.
	blurA   []byte
	blurB   []uint32
	premul  map[string]Image
	premulN int
}

// premulBudget is the bytes of premultiplied bitmaps kept between frames.
const premulBudget = 48 << 20

// DefaultBands is the number of CPUs, at most MaxBands.
func DefaultBands() int { return max(1, min(runtime.NumCPU(), MaxBands)) }

func (c *Compositor) bands() int {
	if c.Bands > 0 {
		return min(c.Bands, MaxBands)
	}
	return DefaultBands()
}

// Compose draws one frame plan into dst, scaling every position and size by outScale. dst is RGBA,
// 8 bits, with no stride padding. It is filled with the background first. A transparent plan fills
// with alpha 0, so a pixel no layer covers stays clear; any other plan ends opaque.
func (c *Compositor) Compose(dst *image.RGBA, fp plan.FramePlan, outScale float64) error {
	b := dst.Bounds()
	if b.Min != (image.Point{}) || dst.Stride != 4*b.Dx() {
		return errors.New("compositor: dst must start at the origin and have no stride padding")
	}
	if !(outScale > 0) {
		return fmt.Errorf("compositor: output scale %v", outScale)
	}

	fill(dst.Pix, frameFill(fp))

	for i := range fp.Layers {
		l := &fp.Layers[i]
		if err := c.layer(dst, l, &fp, outScale); err != nil {
			return fmt.Errorf("layer %s: %w", l.ItemID, err)
		}
	}
	return nil
}

// placement is where a source is drawn: a w by h rectangle centred on (cx, cy), turned by rot
// degrees clockwise, all in output pixels.
type placement struct {
	cx, cy, w, h, rot float64
}

func (c *Compositor) layer(dst *image.RGBA, l *plan.Layer, fp *plan.FramePlan, outScale float64) error {
	op := uint32(math.Round(min(max(l.Opacity, 0), 1) * 255))
	if op == 0 {
		return nil
	}

	var (
		src Image
		p   placement
	)
	switch l.Kind {
	case plan.Media:
		if c.Media == nil {
			return fmt.Errorf("%w: no media source", ErrSource)
		}
		img, err := c.Media(l)
		if err != nil {
			return err
		}
		src = img
		p = placement{cx: l.CX * outScale, cy: l.CY * outScale, w: l.W * outScale, h: l.H * outScale, rot: l.Rotation}
	case plan.Bitmap:
		if c.Bitmaps == nil || l.Bitmap == nil {
			return fmt.Errorf("%w: no bitmap source", ErrSource)
		}
		bm, err := c.Bitmaps(*l.Bitmap)
		if err != nil {
			return err
		}
		src = c.premultiplied(l.Bitmap.Key(), bm)
		// The plan places the centre of the text block; the bitmap reaches further on the side of a
		// shadow, so its own centre is offset by the metrics of the rasterizer.
		sc := 1.0
		if bm.NaturalW > 0 {
			sc = l.W / bm.NaturalW
		}
		p = placement{
			cx: (l.CX - mul(bm.CenterDX, sc)) * outScale, cy: (l.CY - mul(bm.CenterDY, sc)) * outScale,
			w: l.W * outScale, h: l.H * outScale, rot: l.Rotation,
		}
	default:
		return fmt.Errorf("%w: kind %q", ErrSource, l.Kind)
	}

	if !(p.w > 0 && p.h > 0) || src.W <= 0 || src.H <= 0 || len(src.Pix) < src.W*src.H*4 {
		return nil
	}
	if src.FullW != 0 && !windowDrawable(src, l, p) {
		return fmt.Errorf("%w: a window of a layer that is transformed", ErrSource)
	}
	if l.Kind == plan.Bitmap {
		src = c.shrunk(l.Bitmap.Key(), src, round(p.w), round(p.h))
	}
	if l.ChromaKey != nil {
		src = c.applyChromaKey(src, l.ChromaKey)
	}
	if l.ColorMatrix != nil {
		src = c.applyMatrix(src, l.ColorMatrix)
	}
	if g := newGrade(l.Grade); g != nil {
		src = c.applyGrade(src, g)
	}
	table, mix, err := c.lutOf(l)
	if err != nil {
		return err
	}
	if table != nil {
		src = c.applyLUT(src, table, mix)
	}
	if l.Filter != nil {
		src = c.applyMatrix(src, l.Filter)
	}
	// The mask keeps the frame of the layer before the blur grows it (SDD-004 section 3.9). A wipe is a
	// coverage of the same kind, multiplied with it (section 3.2).
	wipe := wipeOf(l.Transition)

	var mask *maskState
	if l.Mask != nil || wipe != nil {
		mask = newMask(l.Mask, wipe, p, outScale)
	}

	pre := effectGeom{
		sw: src.W,
		sh: src.H,
		wq: clamped(mul(p.w, 256), maskLimit),
		hq: clamped(mul(p.h, 256), maskLimit),
	}

	if l.Blur > 0 {
		src, p = c.blurred(src, p, mul(l.Blur, outScale))
	}
	// The flip mirrors the layer's own pixels; the mask above keeps the item's frame (SDD-004 section 3.10).
	src = c.flipped(src, l.FlipX, l.FlipY)
	// The effects and the pixels of a transition come after the flip, so a direction is a direction on the
	// screen, and before the mask (section 3.13).
	src, p = c.applyEffects(src, p, pre, l, fp, outScale)
	// The shadow and the outline sit under the picture and are cut by the mask with it (SDD-004 §3.15).
	src, p = c.shadowed(src, p, l, outScale)
	c.draw(dst, src, p, op, blendOf(l.Blend), mask)
	return nil
}

// windowDrawable says a windowed source can be copied as it is: the layer is drawn at the size of the
// full image, not turned, and nothing reads the pixels outside the window.
func windowDrawable(src Image, l *plan.Layer, p placement) bool {
	return l.Kind == plan.Media && p.rot == 0 && src.FullW == round(p.w) && src.FullH == round(p.h) &&
		!l.FlipX && !l.FlipY && l.Blur == 0 && l.Mask == nil && l.Transition == nil && len(l.Effects) == 0 &&
		l.Shadow == nil && l.Outline == nil
}

// shrunk is src box-filtered down to w by h when it is at least twice as large in both directions, as a
// bitmap is when a small frame (an agent's view) is composed from the full-size spec: bilinear alone
// would point-sample the text and make it shimmer. The result is cached with the bitmap.
func (c *Compositor) shrunk(key string, src Image, w, h int) Image {
	if w < 1 || h < 1 || src.W < 2*w || src.H < 2*h {
		return src
	}
	key += "@" + strconv.Itoa(w) + "x" + strconv.Itoa(h)
	if img, ok := c.premul[key]; ok {
		return img
	}
	img := Resize(src, w, h)
	c.premul[key] = img
	c.premulN += len(img.Pix)
	return img
}

// round is round-half-up, which is what the copy path uses for offsets and sizes.
func round(v float64) int { return int(math.Floor(v + 0.5)) }

func (c *Compositor) draw(dst *image.RGBA, src Image, p placement, op uint32, mode blendMode, mask *maskState) {
	if src.FullW != 0 {
		c.blit(dst, src, round(p.cx-p.w/2)+src.OffX, round(p.cy-p.h/2)+src.OffY, op, mode, mask)
		return
	}
	if p.rot == 0 && src.W == round(p.w) && src.H == round(p.h) {
		c.blit(dst, src, round(p.cx-p.w/2), round(p.cy-p.h/2), op, mode, mask)
		return
	}
	c.sample(dst, src, p, op, mode, mask)
}

// premultiplied converts a straight-alpha bitmap of the rasterizer, once per key.
func (c *Compositor) premultiplied(key string, bm bitmap.Bitmap) Image {
	if img, ok := c.premul[key]; ok {
		return img
	}
	pix := make([]byte, len(bm.Pix))
	for i := 0; i+3 < len(bm.Pix); i += 4 {
		a := uint32(bm.Pix[i+3])
		pix[i] = u8(div255(uint32(bm.Pix[i]) * a))
		pix[i+1] = u8(div255(uint32(bm.Pix[i+1]) * a))
		pix[i+2] = u8(div255(uint32(bm.Pix[i+2]) * a))
		pix[i+3] = bm.Pix[i+3]
	}
	if c.premul == nil || c.premulN+len(pix) > premulBudget {
		c.premul, c.premulN = map[string]Image{}, 0
	}
	img := Image{Pix: pix, W: bm.Width, H: bm.Height}
	c.premul[key] = img
	c.premulN += len(pix)
	return img
}

// div255 is x/255 rounded to nearest, exact for x from 0 to 65535.
func div255[T uint32 | int64](x T) T {
	x += 128
	return (x + x>>8) >> 8
}

// u8 is the byte of a value that is at most 255 by construction; the min keeps it so.
func u8[T uint32 | int64](v T) uint8 {
	return uint8(min(v, 255))
}

// frac8 is the 8 bits of weight of a 32.32 coordinate, the 8 bits under the integer part.
func frac8(v int64) uint32 {
	return uint32(v >> (fix - 8) & 0xFF)
}

// mul is a product the compiler may not fuse with a following addition: the explicit conversion
// rounds it, and the Go specification forbids fusing across it (arm64 would otherwise differ from
// amd64 in the last bit).
func mul(a, b float64) float64 { return float64(a * b) }

// runBands calls fn on disjoint ranges of rows [y0, y1). The ranges are chosen by the row count
// only; what fn writes never depends on them.
func (c *Compositor) runBands(y0, y1, width int, fn func(y0, y1 int)) {
	rows := y1 - y0
	n := min(c.bands(), rows)
	if n <= 1 || rows*width < bandMinPixels {
		if rows > 0 {
			fn(y0, y1)
		}
		return
	}
	var wg sync.WaitGroup
	for i := range n {
		a, b := y0+rows*i/n, y0+rows*(i+1)/n
		wg.Go(func() { fn(a, b) })
	}
	wg.Wait()
}

// blit draws src with its top-left corner at (x0, y0), pixel for pixel.
func (c *Compositor) blit(dst *image.RGBA, src Image, x0, y0 int, op uint32, mode blendMode, mask *maskState) {
	b := dst.Bounds()
	cx0, cy0 := max(x0, 0), max(y0, 0)
	cx1, cy1 := min(x0+src.W, b.Dx()), min(y0+src.H, b.Dy())
	if cx0 >= cx1 || cy0 >= cy1 {
		return
	}
	n := (cx1 - cx0) * 4

	if src.Opaque && op == 255 && mode == blendNormal && mask == nil {
		for y := cy0; y < cy1; y++ {
			so := ((y-y0)*src.W + (cx0 - x0)) * 4
			do := (y*b.Dx() + cx0) * 4
			copy(dst.Pix[do:do+n], src.Pix[so:so+n])
		}
		return
	}

	c.runBands(cy0, cy1, cx1-cx0, func(ya, yb int) {
		for y := ya; y < yb; y++ {
			so := ((y-y0)*src.W + (cx0 - x0)) * 4
			do := (y*b.Dx() + cx0) * 4
			if mask != nil {
				maskedRow(dst.Pix[do:do+n], src.Pix[so:so+n], op, mode, mask, cx0, y)
				continue
			}
			overRow(dst.Pix[do:do+n], src.Pix[so:so+n], op, mode)
		}
	})
}

// overRow blends a row of premultiplied pixels over d with opacity op (0..255).
func overRow(d, s []byte, op uint32, mode blendMode) {
	for i := 0; i+3 < len(s); i += 4 {
		a := uint32(s[i+3])
		if op == 255 && a == 255 && mode == blendNormal {
			copy(d[i:i+4], s[i:i+4])
			continue
		}
		if op != 255 {
			a = div255(a * op)
		}
		if a == 0 {
			continue
		}
		r, g, b := uint32(s[i]), uint32(s[i+1]), uint32(s[i+2])
		if op != 255 {
			r, g, b = div255(r*op), div255(g*op), div255(b*op)
		}
		mixPixel(d[i:i+4:i+4], r, g, b, a, mode)
	}
}

// maskedRow is overRow for a masked layer: each pixel is multiplied by its coverage first, then by
// the opacity, and a masked layer never takes the copy shortcut. x is the destination column of the
// first pixel of the row.
func maskedRow(d, s []byte, op uint32, mode blendMode, m *maskState, x, y int) {
	for i := 0; i+3 < len(s); i += 4 {
		px := [4]uint32{uint32(s[i]), uint32(s[i+1]), uint32(s[i+2]), uint32(s[i+3])}
		m.scale(&px, m.copyCov(x+i/4, y))
		if op != 255 {
			for k := range px {
				px[k] = div255(px[k] * op)
			}
		}
		if px[3] == 0 {
			continue
		}
		mixPixel(d[i:i+4:i+4], px[0], px[1], px[2], px[3], mode)
	}
}

func overPixel(d []byte, r, g, b, a uint32) {
	ia := 255 - a
	d[0] = u8(r + div255(uint32(d[0])*ia))
	d[1] = u8(g + div255(uint32(d[1])*ia))
	d[2] = u8(b + div255(uint32(d[2])*ia))
	d[3] = u8(a + div255(uint32(d[3])*ia))
}

const fix = 32 // fractional bits of the sampler coordinates

// sample draws src into the rectangle p with bilinear interpolation: every destination pixel whose
// centre is inside the rectangle takes the four nearest source pixels. Edges are not smoothed.
func (c *Compositor) sample(dst *image.RGBA, src Image, p placement, op uint32, mode blendMode, mask *maskState) {
	b := dst.Bounds()
	sin, cos := sincos(mul(p.rot, math.Pi/180))

	hw, hh := p.w/2, p.h/2
	ex := mul(hw, abs(cos)) + mul(hh, abs(sin))
	ey := mul(hw, abs(sin)) + mul(hh, abs(cos))
	bx0, bx1 := max(int(math.Floor(p.cx-ex)), 0), min(int(math.Ceil(p.cx+ex)), b.Dx())
	by0, by1 := max(int(math.Floor(p.cy-ey)), 0), min(int(math.Ceil(p.cy+ey)), b.Dy())
	if bx0 >= bx1 || by0 >= by1 {
		return
	}

	// The inverse transform of a destination pixel centre to source coordinates, as 32.32 fixed
	// point: sx = ax*px + bx*py + ox, sy = ay*px + by*py + oy.
	kx, ky := float64(src.W)/p.w, float64(src.H)/p.h
	dx0, dy0 := 0.5-p.cx, 0.5-p.cy
	one := float64(uint64(1) << fix)
	q := func(v float64) int64 { return int64(math.Round(mul(v, one))) }
	ax, bxx := q(mul(cos, kx)), q(mul(sin, kx))
	ay, byy := q(-mul(sin, ky)), q(mul(cos, ky))
	ox := q(mul(mul(dx0, cos)+mul(dy0, sin)+hw, kx))
	oy := q(mul(-mul(dx0, sin)+mul(dy0, cos)+hh, ky))

	limX, limY := int64(src.W)<<fix, int64(src.H)<<fix
	half := int64(1) << (fix - 1)
	maxX, maxY := int64(src.W-1), int64(src.H-1)
	stride := src.W * 4

	c.runBands(by0, by1, bx1-bx0, func(ya, yb int) {
		for y := ya; y < yb; y++ {
			rowX, rowY := bxx*int64(y)+ox, byy*int64(y)+oy
			do := (y*b.Dx() + bx0) * 4
			for x := bx0; x < bx1; x, do = x+1, do+4 {
				sx, sy := rowX+ax*int64(x), rowY+ay*int64(x)
				if sx < 0 || sy < 0 || sx >= limX || sy >= limY {
					continue
				}
				fx, fy := sx-half, sy-half
				x0, y0 := fx>>fix, fy>>fix
				wx, wy := frac8(fx), frac8(fy)
				x1, y1 := min(x0+1, maxX), min(y0+1, maxY)
				x0, y0 = max(x0, 0), max(y0, 0)
				o00, o10 := int(y0)*stride+int(x0)*4, int(y0)*stride+int(x1)*4
				o01, o11 := int(y1)*stride+int(x0)*4, int(y1)*stride+int(x1)*4

				var px [4]uint32
				for k := range 4 {
					top := uint32(src.Pix[o00+k])*(256-wx) + uint32(src.Pix[o10+k])*wx
					bot := uint32(src.Pix[o01+k])*(256-wx) + uint32(src.Pix[o11+k])*wx
					px[k] = (top*(256-wy) + bot*wy + 32768) >> 16
				}
				if mask != nil {
					mask.scale(&px, mask.sampleCov(x, y))
				}
				a := px[3]
				if op != 255 {
					a = div255(a * op)
					px[0], px[1], px[2] = div255(px[0]*op), div255(px[1]*op), div255(px[2]*op)
				}
				if a == 0 {
					continue
				}
				mixPixel(dst.Pix[do:do+4:do+4], px[0], px[1], px[2], a, mode)
			}
		}
	})
}

func abs(v float64) float64 {
	if v < 0 {
		return -v
	}
	return v
}

// frameFill is the colour of uncovered pixels. The planner names black for a transparent project so
// an executor that ignores the flag stays black; this one paints that colour with alpha 0 (SDD-004 §6).
func frameFill(fp plan.FramePlan) [4]uint8 {
	c := parseColour(fp.Background)
	if fp.Transparent {
		c[3] = 0
	}
	return c
}

// fill paints the whole frame with c, which is opaque unless the plan is transparent.
func fill(pix []byte, c [4]uint8) {
	if len(pix) < 4 {
		return
	}
	copy(pix, c[:])
	for n := 4; n < len(pix); n *= 2 {
		copy(pix[n:], pix[:n])
	}
}

// parseColour reads #RRGGBB (an alpha, if any, is ignored: the frame is opaque). Anything else is black.
func parseColour(s string) [4]uint8 {
	out := [4]uint8{0, 0, 0, 255}
	if len(s) < 7 || s[0] != '#' {
		return out
	}
	for i := range 3 {
		hi, ok1 := hexVal(s[1+2*i])
		lo, ok2 := hexVal(s[2+2*i])
		if !ok1 || !ok2 {
			return [4]uint8{0, 0, 0, 255}
		}
		out[i] = hi<<4 | lo
	}
	return out
}

func hexVal(c byte) (uint8, bool) {
	switch {
	case c >= '0' && c <= '9':
		return c - '0', true
	case c >= 'a' && c <= 'f':
		return c - 'a' + 10, true
	case c >= 'A' && c <= 'F':
		return c - 'A' + 10, true
	}
	return 0, false
}

const (
	twoPi = 2 * math.Pi
)

// sincos returns the sine and cosine of x radians using only operations IEEE 754 rounds exactly (see
// mul), so it gives the same bits on every architecture; math.Sin and math.Cos are compiled with fused
// multiply-add on arm64.
func sincos(x float64) (sin, cos float64) {
	x -= mul(twoPi, math.Floor(x/twoPi+0.5))
	x2 := mul(x, x)
	sin, cos = x, 1.0
	ts, tc := x, 1.0
	for k := 1; k <= 24; k++ {
		tc = -mul(tc, x2) / float64((2*k-1)*(2*k))
		ts = -mul(ts, x2) / float64((2*k)*(2*k+1))
		cos += tc
		sin += ts
	}
	return sin, cos
}
