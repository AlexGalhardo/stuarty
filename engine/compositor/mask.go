package compositor

import (
	"math"

	"github.com/AlexGalhardo/vukem-video/core/plan"
)

// maskFix is the fractional bits of the position inside a mask's rectangle: 24.8 fixed point.
const maskFix = 8

// maskState is the mask of one layer (SDD-004 section 3.9): the shape in 24.8 fixed point in the
// layer's own unrotated rectangle, and the two ways of finding where a destination pixel is in it.
// Everything is integer, so the coverage is the same on every system and in every band.
type maskState struct {
	ellipse, invert bool
	// l, t, r, b is the rectangle of the shape and ha, hb its half-extents; fq is the feather.
	l, t, r, b, ha, hb, fq int64
	// left and top are the rounded corner of the copy path; the affine is the sampler path (32.32).
	left, top              int
	ax, bx, ox, ay, by, oy int64
}

// newMask prepares the mask of spec for a layer placed as p (the placement before any blur, so a
// blur halo never moves the mask).
func newMask(spec *plan.MaskSpec, p placement, outScale float64) *maskState {
	wq := int64(math.Round(mul(p.w, 256)))
	hq := int64(math.Round(mul(p.h, 256)))
	xm := int64(math.Round(mul(spec.X, 1000)))
	ym := int64(math.Round(mul(spec.Y, 1000)))
	wm := int64(math.Round(mul(spec.Width, 1000)))
	hm := int64(math.Round(mul(spec.Height, 1000)))

	m := &maskState{ellipse: spec.Shape == "ellipse", invert: spec.Invert}
	m.l = (xm*wq + 500) / 1000
	m.t = (ym*hq + 500) / 1000
	m.ha = max(1, (wm*wq+1000)/2000)
	m.hb = max(1, (hm*hq+1000)/2000)
	m.r, m.b = m.l+2*m.ha, m.t+2*m.hb
	m.fq = int64(math.Round(mul(mul(float64(spec.Feather), outScale), 256)))

	m.left, m.top = round(p.cx-p.w/2), round(p.cy-p.h/2)

	// The transform of the sampler with a source the size of the rectangle (kx = ky = 1).
	sin, cos := sincos(mul(p.rot, math.Pi/180))
	hw, hh := p.w/2, p.h/2
	dx0, dy0 := 0.5-p.cx, 0.5-p.cy
	one := float64(uint64(1) << fix)
	q := func(v float64) int64 { return int64(math.Round(mul(v, one))) }
	m.ax, m.bx = q(cos), q(sin)
	m.ay, m.by = q(-sin), q(cos)
	m.ox = q(mul(dx0, cos) + mul(dy0, sin) + hw)
	m.oy = q(-mul(dx0, sin) + mul(dy0, cos) + hh)

	return m
}

// copyCov is the coverage of the destination pixel (x, y) of a copied layer.
func (m *maskState) copyCov(x, y int) uint32 {
	return m.coverage(int64(x-m.left)<<maskFix+128, int64(y-m.top)<<maskFix+128)
}

// sampleCov is the coverage of the destination pixel (x, y) of a sampled layer.
func (m *maskState) sampleCov(x, y int) uint32 {
	const half = int64(1) << (fix - maskFix - 1)
	u := m.bx*int64(y) + m.ox + m.ax*int64(x)
	v := m.by*int64(y) + m.oy + m.ay*int64(x)
	return m.coverage((u+half)>>(fix-maskFix), (v+half)>>(fix-maskFix))
}

// coverage is the 0..255 coverage at the position (lx, ly) in 24.8 inside the rectangle.
func (m *maskState) coverage(lx, ly int64) uint32 {
	var d int64
	if m.ellipse {
		qx := min((abs64(lx-(m.l+m.ha))<<16)/m.ha, 1<<22)
		qy := min((abs64(ly-(m.t+m.hb))<<16)/m.hb, 1<<22)
		r := int64(isqrt(uint64(qx*qx + qy*qy))) //nolint:gosec // G115: the sum is non-negative and under 2^47
		d = ((65536 - r) * min(m.ha, m.hb)) >> 16
	} else {
		d = min(lx-m.l, m.r-lx, ly-m.t, m.b-ly)
	}

	var cov int64
	if m.fq == 0 {
		if d >= 0 {
			cov = 255
		}
	} else {
		t := min(max(2*d+m.fq, 0), 2*m.fq)
		cov = (255*t + m.fq) / (2 * m.fq)
	}
	if m.invert {
		cov = 255 - cov
	}
	return uint32(cov)
}

// scale multiplies the premultiplied pixel by the coverage, one div255 per channel.
func (m *maskState) scale(px *[4]uint32, cov uint32) {
	if cov == 255 {
		return
	}
	for k := range px {
		px[k] = div255(px[k] * cov)
	}
}

func abs64(v int64) int64 {
	if v < 0 {
		return -v
	}
	return v
}
