package compositor

import (
	"math"

	"github.com/AlexGalhardo/vukem-video/core/plan"
	"github.com/AlexGalhardo/vukem-video/core/timeline"
)

// This file is the pixel effects of SDD-004 section 3.13 and the whip blur and dip mix of a transition
// (section 3.2). Each works on the layer's own pixels, after the filter, the blur and the flip and
// before the mask; the wipe, which needs the destination pixel, is in mask.go. All arithmetic is
// integer, so the bytes do not depend on the number of bands and are those of the Rust renderer.

// maxEffectLen is the largest length, in pixels of the layer's own bitmap, of a motion blur, an RGB split
// or a whip blur: it bounds the memory a source drawn much smaller than it is stored can ask for.
const maxEffectLen = 256

// sinTable is T of section 3.13: round_half_up(1024 * sin(d degrees)) for d from 0 to 90.
var sinTable = [91]int64{
	0, 18, 36, 54, 71, 89, 107, 125, 143, 160,
	178, 195, 213, 230, 248, 265, 282, 299, 316, 333,
	350, 367, 384, 400, 416, 433, 449, 465, 481, 496,
	512, 527, 543, 558, 573, 587, 602, 616, 630, 644,
	658, 672, 685, 698, 711, 724, 737, 749, 761, 773,
	784, 796, 807, 818, 828, 839, 849, 859, 868, 878,
	887, 896, 904, 912, 920, 928, 935, 943, 949, 956,
	962, 968, 974, 979, 984, 989, 994, 998, 1002, 1005,
	1008, 1011, 1014, 1016, 1018, 1020, 1022, 1023, 1023, 1024,
	1024,
}

// angleCS is the cosine and the sine, times 1024, of an angle in degrees (clockwise on the screen).
func angleCS(deg float64) (c, s int64) {
	a := int64(math.Round(deg))
	a = min(max(a, -180), 180)
	q := abs64(a) % 180
	u := q

	if q > 90 {
		u = 180 - q
	}

	s, c = sinTable[u], sinTable[90-u]
	if a < 0 {
		s = -s
	}

	if abs64(a) > 90 {
		c = -c
	}

	return c, s
}

// roundDiv1024 is v / 1024 rounded half away from zero.
func roundDiv1024(v int64) int64 {
	if v < 0 {
		return -((-v + 512) >> 10)
	}

	return (v + 512) >> 10
}

// shiftAlong is the shift by k along the angle with cosine c and sine s (times 1024).
func shiftAlong(k, c, s int64) (dx, dy int) {
	return int(roundDiv1024(k * c)), int(roundDiv1024(k * s))
}

// effectGeom places the pixels of the layer's current image in the layer's own rectangle: the image
// started as sw by sh pixels filling a rectangle of wq by hq (24.8), and grown by padding it now has
// its first pixel at (offx, offy). A padding pixel has a position outside the rectangle.
type effectGeom struct {
	sw, sh     int
	offx, offy int
	wq, hq     int64
}

// lx is the position of the centre of column x from the left of the rectangle, in 24.8.
func (g *effectGeom) lx(x int) int64 { return int64(2*(x-g.offx)+1) * g.wq / int64(2*g.sw) }

// ly is the position of the centre of row y from the top of the rectangle, in 24.8.
func (g *effectGeom) ly(y int) int64 { return int64(2*(y-g.offy)+1) * g.hq / int64(2*g.sh) }

// effectCtx is what the effects of one layer share.
type effectCtx struct {
	c        *Compositor
	outScale float64
	// scale is the scale at which the layer's bitmap is drawn, in output pixels per bitmap pixel.
	scale float64
	frame int
	geom  effectGeom
}

// px is a length in frame pixels at project resolution as a length in pixels of the layer's bitmap, as the
// blur sigma of section 3.6: times the output scale, divided by the scale of the bitmap, rounded half up.
func (e *effectCtx) px(length float64) int {
	v := math.Floor(mul(length, e.outScale)/e.scale + 0.5)
	if !(v > 0) {
		return 0
	}

	return int(min(v, maxEffectLen))
}

// permille is a parameter x as the whole number of thousandths, rounded half away from zero.
func permille(x float64) int64 { return int64(math.Round(mul(x, 1000))) }

// effectParam reads a parameter of an effect, def when it is absent, held to its range (a plan that core
// accepts is always inside it).
func effectParam(ef *timeline.Effect, name string, def float64) float64 {
	v, ok := ef.Param(name)
	if !ok {
		v = def
	}

	lo, hi := timeline.EffectRange(name)

	return min(max(v, lo), hi)
}

// maxFrameSide holds the frame size a whip reads to what its arithmetic takes.
const maxFrameSide = 1 << 20

// progress is the 16.16 progress of a transition held to 0..65536.
func progress(p int) int64 { return int64(min(max(p, 0), 65536)) }

// applyEffects draws the pixel effects of l, then the whip blur and the dip of its transition, on src. It
// returns the image and the placement of the image after the effects that pad it.
func (c *Compositor) applyEffects(
	src Image, p placement, pre effectGeom, l *plan.Layer, fp *plan.FramePlan, outScale float64,
) (Image, placement) {
	tr := l.Transition
	if len(l.Effects) == 0 && (tr == nil || tr.Kind != "whip" && tr.Kind != "dip") {
		return src, p
	}

	e := &effectCtx{c: c, outScale: outScale, scale: p.w / float64(src.W), frame: fp.Frame, geom: pre}
	e.geom.offx, e.geom.offy = (src.W-pre.sw)/2, (src.H-pre.sh)/2

	for i := range l.Effects {
		ef := &l.Effects[i]

		switch ef.Kind {
		case timeline.EffectMotionBlur:
			am := permille(effectParam(ef, "amount", 0.5))
			c2, s2 := angleCS(effectParam(ef, "angle", 0))
			src, p = e.motionBlur(src, p, e.px(float64(am*60)/1000), c2, s2)
		case timeline.EffectGlow:
			src, p = e.glow(src, p, ef)
		case timeline.EffectVignette:
			src = e.vignette(src, ef)
		case timeline.EffectGrain:
			src = e.grain(src, ef)
		case timeline.EffectRGBSplit:
			am := permille(effectParam(ef, "amount", 0.3))
			c2, s2 := angleCS(effectParam(ef, "angle", 0))
			src, p = e.rgbSplit(src, p, e.px(float64(am*24)/1000), c2, s2)
		}
	}

	if tr == nil {
		return src, p
	}

	switch tr.Kind {
	case "whip":
		size := int64(min(max(fp.Width, 0), maxFrameSide))
		c2, s2 := int64(1024), int64(0)

		if tr.Direction == "up" || tr.Direction == "down" {
			size, c2, s2 = int64(min(max(fp.Height, 0), maxFrameSide)), 0, 1024
		}

		n := ((int64(65536)-progress(tr.P))*size>>4 + 32768) >> 16
		src, p = e.motionBlur(src, p, e.px(float64(n)), c2, s2)
	case "dip":
		src = dipMix(src, progress(tr.P), parseColour(tr.Color))
	}

	return src, p
}

// grown is the placement of an image that padding made w by h from one of sw by sh.
func grown(p placement, sw, sh, w, h int) placement {
	p.w, p.h = mul(p.w, float64(w))/float64(sw), mul(p.h, float64(h))/float64(sh)

	return p
}

// pad moves the geometry of the image when it grows by padding on every side.
func (e *effectCtx) pad(px, py int) {
	e.geom.offx += px
	e.geom.offy += py
}

// motionBlur is the directional box blur: the mean of the 2h+1 shifts of the pixel along the angle with
// cosine c and sine s, h = n / 2, over a layer padded by h on every side. n under 2 changes nothing.
func (e *effectCtx) motionBlur(src Image, p placement, n int, c, s int64) (Image, placement) {
	if n < 2 {
		return src, p
	}

	h := n / 2
	taps := 2*h + 1
	dxs, dys := make([]int, taps), make([]int, taps)

	for i := range taps {
		dxs[i], dys[i] = shiftAlong(int64(i-h), c, s)
	}

	w, ht := src.W+2*h, src.H+2*h
	out := make([]byte, w*ht*4)
	div := uint32(taps) //nolint:gosec // G115: taps is at most 2*128+1
	half := div / 2

	e.c.runBands(0, ht, w, func(y0, y1 int) {
		for y := y0; y < y1; y++ {
			for x := range w {
				var sum [4]uint32

				for i := range taps {
					sx, sy := x-h+dxs[i], y-h+dys[i]
					if sx < 0 || sy < 0 || sx >= src.W || sy >= src.H {
						continue
					}

					o := (sy*src.W + sx) * 4
					sum[0] += uint32(src.Pix[o])
					sum[1] += uint32(src.Pix[o+1])
					sum[2] += uint32(src.Pix[o+2])
					sum[3] += uint32(src.Pix[o+3])
				}

				o := (y*w + x) * 4
				out[o], out[o+1] = u8((sum[0]+half)/div), u8((sum[1]+half)/div)
				out[o+2], out[o+3] = u8((sum[2]+half)/div), u8((sum[3]+half)/div)
			}
		}
	})

	e.pad(h, h)

	return Image{Pix: out, W: w, H: ht}, grown(p, src.W, src.H, w, ht)
}

// rgbSplit moves the red channel by d along the angle one way and the blue the other, over a layer padded by
// the shift; the alpha is the largest of the three.
func (e *effectCtx) rgbSplit(src Image, p placement, d int, c, s int64) (Image, placement) {
	dx, dy := shiftAlong(int64(d), c, s)
	if dx == 0 && dy == 0 {
		return src, p
	}

	ax, ay := max(dx, -dx), max(dy, -dy)
	w, h := src.W+2*ax, src.H+2*ay
	out := make([]byte, w*h*4)

	at := func(x, y int) (px [4]uint8) {
		if x < 0 || y < 0 || x >= src.W || y >= src.H {
			return px
		}

		copy(px[:], src.Pix[(y*src.W+x)*4:])

		return px
	}

	e.c.runBands(0, h, w, func(y0, y1 int) {
		for y := y0; y < y1; y++ {
			for x := range w {
				sx, sy := x-ax, y-ay
				r, g, b := at(sx-dx, sy-dy), at(sx, sy), at(sx+dx, sy+dy)
				o := (y*w + x) * 4
				out[o], out[o+1], out[o+2] = r[0], g[1], b[2]
				out[o+3] = max(r[3], g[3], b[3])
			}
		}
	})

	e.pad(ax, ay)

	return Image{Pix: out, W: w, H: h}, grown(p, src.W, src.H, w, h)
}

// glow adds the blurred bright part of the layer back to it.
func (e *effectCtx) glow(src Image, p placement, ef *timeline.Effect) (Image, placement) {
	aq := uint32(math.Round(mul(effectParam(ef, "amount", 0.5), 255)))
	t := int64(math.Round(mul(effectParam(ef, "threshold", 0.6), 255)))
	sigma := mul(effectParam(ef, "radius", 12), e.outScale)

	bright := make([]byte, len(src.Pix))

	for i := 0; i+3 < len(src.Pix); i += 4 {
		r, g, b, a := int64(src.Pix[i]), int64(src.Pix[i+1]), int64(src.Pix[i+2]), int64(src.Pix[i+3])
		y := (54*r + 183*g + 19*b + 128) >> 8

		if t >= 255 || y <= t {
			continue
		}

		k := min(255, ((y-t)*255+(255-t)/2)/(255-t))
		bright[i], bright[i+1] = u8(div255(r*k)), u8(div255(g*k))
		bright[i+2], bright[i+3] = u8(div255(b*k)), u8(div255(a*k))
	}

	// The blur works in memory that the next blur reuses, and src may be that memory.
	own := append([]byte(nil), src.Pix...)

	blurred, gp := e.c.blurred(Image{Pix: bright, W: src.W, H: src.H}, p, sigma)
	ox, oy := (blurred.W-src.W)/2, (blurred.H-src.H)/2
	out := make([]byte, len(blurred.Pix))

	e.c.runBands(0, blurred.H, blurred.W, func(y0, y1 int) {
		for y := y0; y < y1; y++ {
			for x := range blurred.W {
				o := (y*blurred.W + x) * 4
				sx, sy := x-ox, y-oy
				inside := sx >= 0 && sy >= 0 && sx < src.W && sy < src.H

				for k := range 4 {
					v := uint32(0)
					if inside {
						v = uint32(own[(sy*src.W+sx)*4+k])
					}

					out[o+k] = u8(v + div255(uint32(blurred.Pix[o+k])*aq))
				}
			}
		}
	})

	e.pad(ox, oy)

	return Image{Pix: out, W: blurred.W, H: blurred.H}, gp
}

// vignette darkens the colour channels towards the edges and corners of the layer's rectangle.
func (e *effectCtx) vignette(src Image, ef *timeline.Effect) Image {
	const r1 = 92682

	amq := int64(math.Round(mul(effectParam(ef, "amount", 0.5), 255)))
	r0 := int64(r1) - (int64(r1)*permille(effectParam(ef, "softness", 0.5))+500)/1000
	hw, hh := max(1, e.geom.wq/2), max(1, e.geom.hq/2)

	nx := make([]int64, src.W)
	for x := range nx {
		nx[x] = min((abs64(e.geom.lx(x)-e.geom.wq/2)<<16)/hw, 1<<22)
	}

	out := Image{Pix: make([]byte, len(src.Pix)), W: src.W, H: src.H, Opaque: src.Opaque}

	e.c.runBands(0, src.H, src.W, func(y0, y1 int) {
		for y := y0; y < y1; y++ {
			ny := min((abs64(e.geom.ly(y)-e.geom.hq/2)<<16)/hh, 1<<22)

			for x := range src.W {
				r := radius(nx[x], ny)

				var f int64

				switch {
				case r >= r1:
					f = 65536
				case r <= r0:
					f = 0
				default:
					f = (r - r0) * 65536 / (r1 - r0)
				}

				f2 := (f*f + 32768) >> 16
				m := uint32(255 - ((f2*amq + 32768) >> 16)) //nolint:gosec // G115: f2 and amq are at most 65536 and 255
				o := (y*src.W + x) * 4
				out.Pix[o], out.Pix[o+1] = u8(div255(uint32(src.Pix[o])*m)), u8(div255(uint32(src.Pix[o+1])*m))
				out.Pix[o+2], out.Pix[o+3] = u8(div255(uint32(src.Pix[o+2])*m)), src.Pix[o+3]
			}
		}
	})

	return out
}

// radius is the length of (nx, ny), both at most 2^22, so the sum of squares is under 2^47.
func radius(nx, ny int64) int64 {
	return int64(isqrt(uint64(nx*nx + ny*ny))) //nolint:gosec // G115: non-negative and under 2^47
}

// wrap32 is the low 32 bits of v: the wrap of a negative cell is part of the definition of the grain.
func wrap32(v int64) uint32 { return uint32(v) } //nolint:gosec // G115: the wrap is the definition

// floorDiv is a / b rounded down, for b > 0.
func floorDiv(a, b int64) int64 {
	q := a / b
	if a%b < 0 {
		q--
	}

	return q
}

// grain adds the same hashed noise to the three colour channels, a new pattern every frame.
func (e *effectCtx) grain(src Image, ef *timeline.Effect) Image {
	am := permille(effectParam(ef, "amount", 0.3))
	sz := max(1, int64(math.Floor(mul(effectParam(ef, "size", 1), e.outScale)+0.5)))
	seed := int64(effectParam(ef, "seed", 0))
	amq := (am*64 + 500) / 1000
	frame := uint32(e.frame) //nolint:gosec // G115: a frame number is small and non-negative

	cxs := make([]int64, src.W)
	for x := range cxs {
		cxs[x] = floorDiv(e.geom.lx(x)>>8, sz)
	}

	out := Image{Pix: make([]byte, len(src.Pix)), W: src.W, H: src.H, Opaque: src.Opaque}

	e.c.runBands(0, src.H, src.W, func(y0, y1 int) {
		for y := y0; y < y1; y++ {
			cy := wrap32(floorDiv(e.geom.ly(y)>>8, sz))

			for x := range src.W {
				o := (y*src.W + x) * 4
				a := int64(src.Pix[o+3])
				v := plan.Hash3(wrap32(cxs[x]+seed<<16), cy, frame) >> 24
				delta := ((int64(v)-128)*amq + 64) >> 7

				var dl int64

				switch {
				case delta > 0:
					dl = (delta*a + 127) / 255
				case delta < 0:
					dl = -((-delta*a + 127) / 255)
				}

				for k := range 3 {
					out.Pix[o+k] = u8(min(max(int64(src.Pix[o+k])+dl, 0), a))
				}

				out.Pix[o+3] = src.Pix[o+3]
			}
		}
	})

	return out
}

// dipMix mixes the layer with the solid colour col: its own pixels at the progress P (16.16), the colour,
// inside the layer's own shape, at the rest.
func dipMix(src Image, progress int64, col [4]uint8) Image {
	pq := uint32((progress*255 + 32768) >> 16) //nolint:gosec // G115: progress is at most 65536
	out := Image{Pix: make([]byte, len(src.Pix)), W: src.W, H: src.H, Opaque: src.Opaque}

	for i := 0; i+3 < len(src.Pix); i += 4 {
		a := uint32(src.Pix[i+3])

		for k := range 3 {
			out.Pix[i+k] = u8(div255(uint32(src.Pix[i+k])*pq) + div255(div255(uint32(col[k])*a)*(255-pq)))
		}

		out.Pix[i+3] = src.Pix[i+3]
	}

	return out
}
