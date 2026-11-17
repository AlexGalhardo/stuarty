package compositor

import (
	"fmt"
	"math"

	"github.com/AlexGalhardo/vukem-video/core/plan"
	"github.com/AlexGalhardo/vukem-video/engine/lut"
)

// LUTSource returns the lookup table of a lut asset (SDD-004 section 3.6).
type LUTSource func(asset string) (*lut.Table, error)

// lutDen is the denominator of the trilinear sum: 255 per axis for the weights, and 257 turning the 16-bit entries
// (65535 = 255 x 257) into the 8 bits of a channel.
const lutDen = 255 * 255 * 255 * 257

// lutMix is the intensity in thousandths; NaN is 0.
func lutMix(intensity float64) int64 {
	if intensity != intensity {
		return 0
	}

	return int64(math.Round(min(max(mul(intensity, 1000), 0), 1000)))
}

// lutKernel is a table prepared for the pixels of one frame: for each value of a channel, where it sits on an axis
// of the table (the offsets of the entry below and of the one above, in the 16-bit values of Data) and its weight
// out of 255. It holds what the per-pixel division and modulo of the interpolation would recompute, and nothing else,
// so the result is the same integer as the plain formula.
type lutKernel struct {
	data   []uint16
	im     int64
	lo, hi [3][256]int32
	frac   [3][256]uint64
}

func newLUTKernel(t *lut.Table, im int64) *lutKernel {
	k := &lutKernel{data: t.Data, im: im}
	n := int64(t.Size)
	n1 := n - 1

	// The red index varies fastest in Data, then green, then blue: the offset of one step is 3, 3n and 3n^2.
	step := [3]int64{3, 3 * n, 3 * n * n}

	for a := range k.lo {
		for v := range int64(256) {
			p := v * n1
			i0 := p / 255
			i1 := min(i0+1, n1)
			k.lo[a][v] = int32(i0 * step[a]) //nolint:gosec // G115: at most 3 x 65^3, far below 2^31
			k.hi[a][v] = int32(i1 * step[a]) //nolint:gosec // G115: at most 3 x 65^3, far below 2^31
			k.frac[a][v] = uint64(p % 255)   //nolint:gosec // G115: a remainder, not negative
		}
	}

	return k
}

// pixel is the colour of one straight pixel through the table: trilinear interpolation of the eight entries around
// it in integer arithmetic, then the mix with the pixel by the intensity im (thousandths).
func (k *lutKernel) pixel(r, g, b int64) (int64, int64, int64) {
	fr, fg, fb := k.frac[0][r], k.frac[1][g], k.frac[2][b]
	gr, gg, gb := 255-fr, 255-fg, 255-fb

	rl, rh := k.lo[0][r], k.hi[0][r]
	gl, gh := k.lo[1][g], k.hi[1][g]
	bl, bh := k.lo[2][b], k.hi[2][b]

	d := k.data
	e0, e1 := (*[3]uint16)(d[rl+gl+bl:]), (*[3]uint16)(d[rh+gl+bl:])
	e2, e3 := (*[3]uint16)(d[rl+gh+bl:]), (*[3]uint16)(d[rh+gh+bl:])
	e4, e5 := (*[3]uint16)(d[rl+gl+bh:]), (*[3]uint16)(d[rh+gl+bh:])
	e6, e7 := (*[3]uint16)(d[rl+gh+bh:]), (*[3]uint16)(d[rh+gh+bh:])

	w0, w1, w2, w3 := gr*gg*gb, fr*gg*gb, gr*fg*gb, fr*fg*gb
	w4, w5, w6, w7 := gr*gg*fb, fr*gg*fb, gr*fg*fb, fr*fg*fb

	// Every term is non-negative, so the unsigned division by a constant is a multiplication and a shift.
	im, rest := uint64(k.im), uint64(1000-k.im) //nolint:gosec // G115: the intensity is 1 to 1000

	sr := w0*uint64(e0[0]) + w1*uint64(e1[0]) + w2*uint64(e2[0]) + w3*uint64(e3[0]) +
		w4*uint64(e4[0]) + w5*uint64(e5[0]) + w6*uint64(e6[0]) + w7*uint64(e7[0])
	sg := w0*uint64(e0[1]) + w1*uint64(e1[1]) + w2*uint64(e2[1]) + w3*uint64(e3[1]) +
		w4*uint64(e4[1]) + w5*uint64(e5[1]) + w6*uint64(e6[1]) + w7*uint64(e7[1])
	sb := w0*uint64(e0[2]) + w1*uint64(e1[2]) + w2*uint64(e2[2]) + w3*uint64(e3[2]) +
		w4*uint64(e4[2]) + w5*uint64(e5[2]) + w6*uint64(e6[2]) + w7*uint64(e7[2])

	vr, vg, vb := (sr+lutDen/2)/lutDen, (sg+lutDen/2)/lutDen, (sb+lutDen/2)/lutDen

	ur, ug, ub := uint64(r), uint64(g), uint64(b) //nolint:gosec // G115: channels are 0 to 255

	mr, mg, mb := (ur*rest+vr*im+500)/1000, (ug*rest+vg*im+500)/1000, (ub*rest+vb*im+500)/1000

	return int64(mr), int64(mg), int64(mb) //nolint:gosec // G115: at most 255
}

// lutPixel is the colour of one pixel through a table that is used once; a frame prepares the table once instead.
func lutPixel(t *lut.Table, im, r, g, b int64) (int64, int64, int64) {
	return newLUTKernel(t, im).pixel(r, g, b)
}

// applyLUT returns src with the table applied to its straight RGB values, mixed in by im thousandths (SDD-004
// section 3.6). The result lives in scratch memory that the next call reuses.
func (c *Compositor) applyLUT(src Image, t *lut.Table, im int64) Image {
	n := src.W * src.H * 4
	if cap(c.scratch) < n {
		c.scratch = make([]byte, n)
	}

	out := Image{Pix: c.scratch[:n], W: src.W, H: src.H, Opaque: src.Opaque}
	k := newLUTKernel(t, im)

	c.runBands(0, src.H, src.W, func(y0, y1 int) {
		var memo colourMemo

		for i := y0 * src.W * 4; i < y1*src.W*4; i += 4 {
			a := int64(src.Pix[i+3])
			r, g, b := int64(src.Pix[i]), int64(src.Pix[i+1]), int64(src.Pix[i+2])

			switch a {
			case 0:
				copy(out.Pix[i:i+4], src.Pix[i:i+4])
				continue
			case 255:
			default:
				r, g, b = min((r*255+a/2)/a, 255), min((g*255+a/2)/a, 255), min((b*255+a/2)/a, 255)
			}

			nr, ng, nb := memo.pixel(k.pixel, r, g, b)
			if a != 255 {
				nr, ng, nb = div255(nr*a), div255(ng*a), div255(nb*a)
			}

			out.Pix[i], out.Pix[i+1], out.Pix[i+2], out.Pix[i+3] = u8(nr), u8(ng), u8(nb), src.Pix[i+3]
		}
	})

	return out
}

// lutOf is the table of a layer and its intensity, or nil when the layer has none or draws none of it.
func (c *Compositor) lutOf(l *plan.Layer) (*lut.Table, int64, error) {
	if l.Lut == nil {
		return nil, 0, nil
	}

	im := lutMix(l.Lut.Intensity)
	if im == 0 {
		return nil, 0, nil
	}

	if c.LUTs == nil {
		return nil, 0, fmt.Errorf("%w: no lookup table source", ErrSource)
	}

	t, err := c.LUTs(l.Lut.Asset)
	if err != nil {
		return nil, 0, err
	}

	return t, im, nil
}
