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

// lutPixel is the colour of one straight pixel through the table: trilinear interpolation of the eight entries
// around it in integer arithmetic, then the mix with the pixel by the intensity im (thousandths).
func lutPixel(t *lut.Table, im, r, g, b int64) (int64, int64, int64) {
	n := int64(t.Size)
	n1 := n - 1

	// A channel v sits at v x (n - 1) / 255 on the axis: the index below it and the weight of the one above, out of 255.
	pr, pg, pb := r*n1, g*n1, b*n1
	r0, g0, b0 := pr/255, pg/255, pb/255
	fr, fg, fb := pr%255, pg%255, pb%255
	r1, g1, b1 := min(r0+1, n1), min(g0+1, n1), min(b0+1, n1)

	at := func(ri, gi, bi int64) []uint16 {
		i := 3 * (ri + n*(gi+n*bi))
		return t.Data[i : i+3]
	}

	corners := [8]struct {
		e []uint16
		w int64
	}{
		{at(r0, g0, b0), (255 - fr) * (255 - fg) * (255 - fb)},
		{at(r1, g0, b0), fr * (255 - fg) * (255 - fb)},
		{at(r0, g1, b0), (255 - fr) * fg * (255 - fb)},
		{at(r1, g1, b0), fr * fg * (255 - fb)},
		{at(r0, g0, b1), (255 - fr) * (255 - fg) * fb},
		{at(r1, g0, b1), fr * (255 - fg) * fb},
		{at(r0, g1, b1), (255 - fr) * fg * fb},
		{at(r1, g1, b1), fr * fg * fb},
	}

	var sum [3]int64

	for _, c := range corners {
		for k := range sum {
			sum[k] += c.w * int64(c.e[k])
		}
	}

	in := [3]int64{r, g, b}

	var out [3]int64

	for k := range out {
		v := (sum[k] + lutDen/2) / lutDen
		out[k] = (in[k]*(1000-im) + v*im + 500) / 1000
	}

	return out[0], out[1], out[2]
}

// applyLUT returns src with the table applied to its straight RGB values, mixed in by im thousandths (SDD-004
// section 3.6). The result lives in scratch memory that the next call reuses.
func (c *Compositor) applyLUT(src Image, t *lut.Table, im int64) Image {
	n := src.W * src.H * 4
	if cap(c.scratch) < n {
		c.scratch = make([]byte, n)
	}

	out := Image{Pix: c.scratch[:n], W: src.W, H: src.H, Opaque: src.Opaque}

	c.runBands(0, src.H, src.W, func(y0, y1 int) {
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

			nr, ng, nb := lutPixel(t, im, r, g, b)
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
