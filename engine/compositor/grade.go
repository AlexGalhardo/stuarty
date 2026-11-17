package compositor

import (
	"math"

	"github.com/AlexGalhardo/vukem-video/core/plan"
	"github.com/AlexGalhardo/vukem-video/core/timeline"
)

// The advanced colour of SDD-004 section 3.14 in integer arithmetic: HSL bands, then the three colour
// wheels, then the tone curves, on straight RGB values 0..255. The Rust renderer (renderer/vukem-renderer/src/grade.rs)
// does the same operations in the same order, so a frame has the same bytes in both.

const (
	hueTurn  = 6 * 65536 // a full turn of the hue, 65536 per 60 degrees
	hslBands = 8
	// maxSlope is the largest slope of a curve, 1024 in 16.16, so no product of the spline leaves an i64.
	maxSlope = 1 << 26
)

// hueCentres are the hues of the eight HSL ranges, in 65536ths of 60 degrees: red, orange, yellow, green,
// cyan, blue, purple and magenta, then red again a turn later.
var hueCentres = [hslBands + 1]int64{0, 32768, 65536, 131072, 196608, 262144, 300373, 349525, hueTurn}

type hslBand struct {
	hue, sat, light int64 // 1/1024 of the range of the parameter
}

type wheelQ struct {
	b [3]int64 // the pixel's offset of each channel, scaled by 2^28 (see apply)
}

// grade is a layer's advanced colour with every parameter already in integers.
type grade struct {
	bands  [hslBands]hslBand
	hasHSL bool

	wheels   [3]*wheelQ // shadows, midtones, highlights
	hasWheel bool

	curves    [3][256]uint8 // red, green and blue after the master curve
	hasCurves bool
}

// q1024 is a parameter in 1/1024 of its range -1..1; NaN is 0.
func q1024(v float64) int64 {
	if v != v {
		return 0
	}

	return int64(math.Round(min(max(mul(v, 1024), -1024), 1024)))
}

// newGrade makes the grade of a spec, or nil when it changes no pixel.
func newGrade(spec *plan.GradeSpec) *grade {
	if spec == nil {
		return nil
	}

	g := &grade{}

	for _, b := range spec.Hsl {
		for i, name := range timeline.HslRanges {
			if name == b.Range {
				g.bands[i] = hslBand{q1024(b.Hue), q1024(b.Saturation), q1024(b.Lightness)}
				g.hasHSL = g.hasHSL || g.bands[i] != hslBand{}
			}
		}
	}

	if w := spec.Wheels; w != nil {
		for i, wh := range []*timeline.Wheel{w.Shadows, w.Midtones, w.Highlights} {
			if q := newWheel(wh); q != nil {
				g.wheels[i], g.hasWheel = q, true
			}
		}
	}

	if c := spec.Curves; c != nil {
		g.hasCurves = g.buildCurves(c)
	}

	if !g.hasHSL && !g.hasWheel && !g.hasCurves {
		return nil
	}

	return g
}

// newWheel precomputes B[c] = amount × tint[c] × 4 + luma × 65536, with the amount and the luma in 1/1024 and the
// tint of the hue (its RGB at full saturation and value minus their mean) in 16.16; nil when the wheel does nothing.
func newWheel(w *timeline.Wheel) *wheelQ {
	if w == nil {
		return nil
	}

	amount, luma := q1024(w.Amount), q1024(w.Luma)
	if amount == 0 && luma == 0 {
		return nil
	}

	hue := int64(0)
	if w.Hue == w.Hue {
		hue = int64(math.Round(min(max(mul(w.Hue, 1000), 0), 360000))) % 360000
	}

	h6 := hue * 6
	sector, f := h6/360000, (h6%360000)*65536/360000
	one := int64(65536)

	var rgb [3]int64

	switch sector {
	case 0:
		rgb = [3]int64{one, f, 0}
	case 1:
		rgb = [3]int64{one - f, one, 0}
	case 2:
		rgb = [3]int64{0, one, f}
	case 3:
		rgb = [3]int64{0, one - f, one}
	case 4:
		rgb = [3]int64{f, 0, one}
	default:
		rgb = [3]int64{one, 0, one - f}
	}

	mean := (rgb[0] + rgb[1] + rgb[2]) / 3

	q := &wheelQ{}
	for c := range rgb {
		q.b[c] = amount*(rgb[c]-mean)*4 + luma*65536
	}

	return q
}

// pointQ is a point of a curve in 16.16.
type pointQ struct{ x, y int64 }

func curvePoints(pts []timeline.CurvePoint) []pointQ {
	out := make([]pointQ, 0, len(pts))

	for _, p := range pts {
		x, y := int64(0), int64(0)
		if p.X == p.X {
			x = int64(math.Round(min(max(mul(p.X, 65536), 0), 65536)))
		}

		if p.Y == p.Y {
			y = int64(math.Round(min(max(mul(p.Y, 65536), 0), 65536)))
		}
		// Two points that share a 16.16 position keep the first.
		if n := len(out); n > 0 && x <= out[n-1].x {
			continue
		}

		out = append(out, pointQ{x, y})
	}

	return out
}

func sign(v int64) int {
	switch {
	case v > 0:
		return 1
	case v < 0:
		return -1
	}

	return 0
}

// curveTable is the 256 entries of the monotone cubic through the points (Fritsch-Carlson with the box limit
// of 3 times the secant on each side), flat before the first point and after the last one; nil for no points.
func curveTable(pts []pointQ) *[256]uint8 {
	n := len(pts)
	if n == 0 {
		return nil
	}

	delta := make([]int64, max(n-1, 0))
	for k := range delta {
		delta[k] = ((pts[k+1].y - pts[k].y) << 16) / (pts[k+1].x - pts[k].x)
	}

	m := make([]int64, n)

	if n > 1 {
		m[0], m[n-1] = delta[0], delta[n-2]

		for k := 1; k < n-1; k++ {
			if sign(delta[k-1]) == sign(delta[k]) && sign(delta[k]) != 0 {
				m[k] = (delta[k-1] + delta[k]) / 2
			}
		}

		for k, d := range delta {
			for _, e := range []int{k, k + 1} {
				switch {
				case d == 0 || sign(m[e]) != sign(d):
					m[e] = 0
				case abs64(m[e]) > 3*abs64(d):
					m[e] = int64(sign(d)) * 3 * abs64(d)
				}

				m[e] = min(max(m[e], -maxSlope), maxSlope)
			}
		}
	}

	var t [256]uint8

	for v := range t {
		x := (int64(v)*65536 + 127) / 255

		var y int64

		switch {
		case x <= pts[0].x:
			y = pts[0].y
		case x >= pts[n-1].x:
			y = pts[n-1].y
		default:
			k := 0
			for x >= pts[k+1].x {
				k++
			}

			h := pts[k+1].x - pts[k].x
			u := (x - pts[k].x) * 65536 / h
			u2 := (u * u) >> 16
			u3 := (u2 * u) >> 16
			h00, h10, h01, h11 := 2*u3-3*u2+65536, u3-2*u2+u, -2*u3+3*u2, u3-u2
			y = (h00*pts[k].y + h01*pts[k+1].y + h10*((h*m[k])>>16) + h11*((h*m[k+1])>>16) + 32768) >> 16
		}

		y = min(max(y, 0), 65536)
		t[v] = uint8((y*255 + 32768) >> 16)
	}

	return &t
}

// buildCurves fills g.curves and says whether any curve is set.
func (g *grade) buildCurves(c *timeline.Curves) bool {
	master := curveTable(curvePoints(c.Master))
	chans := [3]*[256]uint8{
		curveTable(curvePoints(c.Red)), curveTable(curvePoints(c.Green)), curveTable(curvePoints(c.Blue)),
	}

	if master == nil && chans[0] == nil && chans[1] == nil && chans[2] == nil {
		return false
	}

	for ch := range chans {
		for v := range 256 {
			o := uint8(v)
			if master != nil {
				o = master[o]
			}

			if chans[ch] != nil {
				o = chans[ch][o]
			}

			g.curves[ch][v] = o
		}
	}

	return true
}

// hsl moves the colour of one pixel by the bands.
func (g *grade) hsl(r, gr, b int64) (int64, int64, int64) {
	mx, mn := max(r, gr, b), min(r, gr, b)

	d := mx - mn
	if d == 0 {
		return r, gr, b
	}

	sum := mx + mn
	den := sum

	if sum > 255 {
		den = 510 - sum
	}

	sat := d * 65536 / den

	var h int64

	switch mx {
	case r:
		h = (gr - b) * 65536 / d
		if h < 0 {
			h += hueTurn
		}
	case gr:
		h = (b-r)*65536/d + 131072
	default:
		h = (r-gr)*65536/d + 262144
	}

	i := 0
	for h >= hueCentres[i+1] {
		i++
	}

	t := (h - hueCentres[i]) * 65536 / (hueCentres[i+1] - hueCentres[i])
	wa, wb := (65536-t)*sat>>16, t*sat>>16
	ba, bb := g.bands[i], g.bands[(i+1)%hslBands]

	hs := (wa*ba.hue + wb*bb.hue) / 2048
	ds := (wa*ba.sat + wb*bb.sat) / 1024
	dl := (wa*ba.light + wb*bb.light) / 1024

	if hs == 0 && ds == 0 && dl == 0 {
		return r, gr, b
	}

	h = ((h+hs)%hueTurn + hueTurn) % hueTurn
	s2 := min(max(sat*(65536+ds)>>16, 0), 65536)
	l := (sum*65536 + 255) / 510

	if dl >= 0 {
		l += (65536 - l) * dl >> 16
	} else {
		l -= l * -dl >> 16
	}

	c := (65536 - abs64(2*l-65536)) * s2 >> 16
	sector, f := h/65536, h%65536

	x := c * f >> 16
	if sector%2 == 1 {
		x = c * (65536 - f) >> 16
	}

	m := l - c>>1

	var v [3]int64

	switch sector {
	case 0:
		v = [3]int64{c, x, 0}
	case 1:
		v = [3]int64{x, c, 0}
	case 2:
		v = [3]int64{0, c, x}
	case 3:
		v = [3]int64{0, x, c}
	case 4:
		v = [3]int64{x, 0, c}
	default:
		v = [3]int64{c, 0, x}
	}

	return clamp8(
			((v[0]+m)*255 + 32768) >> 16,
		), clamp8(
			((v[1]+m)*255 + 32768) >> 16,
		), clamp8(
			((v[2]+m)*255 + 32768) >> 16,
		)
}

// wheel adds the three wheels to one pixel: the weight of a tonal range is (1 - Y)^2, 1 - both and Y^2 of the luma
// Y of the pixel, in 16.16, and the offset of a channel is 255 × Σ weight × B / 2^44.
func (g *grade) wheel(r, gr, b int64) (int64, int64, int64) {
	y := (54*r + 183*gr + 19*b + 128) >> 8
	s, hi := (255-y)*(255-y), y*y
	ws := [3]int64{s, 65025 - s - hi, hi}

	var n [3]int64

	for k, w := range g.wheels {
		if w == nil {
			continue
		}

		w16 := (ws[k]*65536 + 32512) / 65025
		for c := range n {
			n[c] += w16 * w.b[c]
		}
	}

	const half = int64(1) << 43

	return clamp8(r + (n[0]*255+half)>>44), clamp8(gr + (n[1]*255+half)>>44), clamp8(b + (n[2]*255+half)>>44)
}

// pixel is the whole grade of one straight pixel.
func (g *grade) pixel(r, gr, b int64) (int64, int64, int64) {
	if g.hasHSL {
		r, gr, b = g.hsl(r, gr, b)
	}

	if g.hasWheel {
		r, gr, b = g.wheel(r, gr, b)
	}

	if g.hasCurves {
		r, gr, b = int64(g.curves[0][r]), int64(g.curves[1][gr]), int64(g.curves[2][b])
	}

	return r, gr, b
}

// applyGrade returns src with the grade applied to its straight RGB values (SDD-004 section 3.14). The result lives
// in scratch memory that the next call reuses.
func (c *Compositor) applyGrade(src Image, g *grade) Image {
	n := src.W * src.H * 4
	if cap(c.scratch) < n {
		c.scratch = make([]byte, n)
	}

	out := Image{Pix: c.scratch[:n], W: src.W, H: src.H, Opaque: src.Opaque}

	c.runBands(0, src.H, src.W, func(y0, y1 int) {
		var memo colourMemo

		for i := y0 * src.W * 4; i < y1*src.W*4; i += 4 {
			a := int64(src.Pix[i+3])
			r, gr, b := int64(src.Pix[i]), int64(src.Pix[i+1]), int64(src.Pix[i+2])

			switch a {
			case 0:
				copy(out.Pix[i:i+4], src.Pix[i:i+4])
				continue
			case 255:
			default:
				r, gr, b = min((r*255+a/2)/a, 255), min((gr*255+a/2)/a, 255), min((b*255+a/2)/a, 255)
			}

			nr, ng, nb := memo.pixel(g.pixel, r, gr, b)
			if a != 255 {
				nr, ng, nb = div255(nr*a), div255(ng*a), div255(nb*a)
			}

			out.Pix[i], out.Pix[i+1], out.Pix[i+2], out.Pix[i+3] = u8(nr), u8(ng), u8(nb), src.Pix[i+3]
		}
	})

	return out
}
