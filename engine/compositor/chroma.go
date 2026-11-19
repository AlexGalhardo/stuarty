package compositor

import (
	"math"
	"math/bits"

	"github.com/AlexGalhardo/stuarty/core/plan"
)

// chromaScale is the chroma distance that a similarity or a smoothness of 1 stands for, in 1/256 of an 8-bit
// chroma unit: 128 units, half the width of the Cb and Cr range.
const chromaScale = 128 * 256

// chromaKey is a key colour and its three settings reduced to integers once per layer (SDD-004 section 3.7).
type chromaKey struct {
	cb, cr int64 // the chroma of the key colour, in 1/256 units
	// t0 is the distance up to which a pixel is removed, t1 the width of the soft edge beyond it.
	t0, t1 int64
	// spill is the 16.16 fraction of the excess of the key's dominant channel taken out of the pixels that stay,
	// and dominant is that channel (0 red, 1 green, 2 blue) or -1 when the key colour has none.
	spill    int64
	dominant int
}

func newChromaKey(spec *plan.ChromaKeySpec) chromaKey {
	c := parseColour(spec.Color)
	k := chromaKey{
		t0:       int64(math.Round(mul(min(max(spec.Similarity, 0), 1), chromaScale))),
		t1:       int64(math.Round(mul(min(max(spec.Smoothness, 0), 1), chromaScale))),
		spill:    int64(math.Round(mul(min(max(spec.Spill, 0), 1), 65536))),
		dominant: -1,
	}
	k.cb, k.cr = chroma(int64(c[0]), int64(c[1]), int64(c[2]))

	// The dominant channel is the one that is strictly above both others: a green screen is green, a grey or a
	// white key has none and nothing to despill.
	for ch := range 3 {
		if int(c[ch]) > int(c[(ch+1)%3]) && int(c[ch]) > int(c[(ch+2)%3]) {
			k.dominant = ch
		}
	}

	return k
}

// chroma is the BT.601 Cb and Cr of a colour in 1/256 of a unit, each from -32640 to 32640.
func chroma(r, g, b int64) (cb, cr int64) {
	return -43*r - 85*g + 128*b, 128*r - 107*g - 21*b
}

// isqrt is the integer square root: the largest x with x*x <= n.
func isqrt(n uint64) uint64 {
	if n < 2 {
		return n
	}

	x := uint64(1) << ((bits.Len64(n) + 1) / 2)

	for {
		y := (x + n/x) / 2
		if y >= x {
			return x
		}

		x = y
	}
}

// keep is the opacity, 0 to 255, that a pixel of straight colour (r, g, b) keeps: 0 within t0 of the key colour
// in the chroma plane, 255 beyond t0 + t1 and a linear ramp between.
func (k *chromaKey) keep(r, g, b int64) int64 {
	cb, cr := chroma(r, g, b)
	dcb, dcr := cb-k.cb, cr-k.cr
	d := int64(isqrt(uint64(dcb*dcb + dcr*dcr))) //nolint:gosec // G115: the sum of squares is far below 2^63

	switch {
	case d <= k.t0:
		return 0
	case d >= k.t0+k.t1:
		return 255
	default:
		return ((d-k.t0)*255 + k.t1/2) / k.t1
	}
}

// despill pulls the dominant channel of the key colour down towards the mean of the two others, by the spill
// fraction of its excess, so that the green of a screen does not tint the edges of what stays.
func (k *chromaKey) despill(c *[3]int64) {
	if k.dominant < 0 || k.spill == 0 {
		return
	}

	limit := (c[(k.dominant+1)%3] + c[(k.dominant+2)%3]) / 2
	if excess := c[k.dominant] - limit; excess > 0 {
		c[k.dominant] -= (excess*k.spill + 32768) >> 16
	}
}

// applyChromaKey returns src with the key applied to its own pixels: alpha goes down for the pixels near the key
// colour and the key colour is taken out of the others (SDD-004 section 3.7). The pixels are premultiplied, so a
// pixel is returned to straight colour, keyed and premultiplied again with its new alpha. The result is never
// opaque, whatever src was, and lives in scratch memory that the next call reuses.
func (c *Compositor) applyChromaKey(src Image, spec *plan.ChromaKeySpec) Image {
	k := newChromaKey(spec)

	n := src.W * src.H * 4
	if cap(c.scratch) < n {
		c.scratch = make([]byte, n)
	}

	out := Image{Pix: c.scratch[:n], W: src.W, H: src.H}

	c.runBands(0, src.H, src.W, func(y0, y1 int) {
		for i := y0 * src.W * 4; i < y1*src.W*4; i += 4 {
			a := int64(src.Pix[i+3])
			if a == 0 {
				copy(out.Pix[i:i+4], src.Pix[i:i+4])

				continue
			}

			px := [3]int64{int64(src.Pix[i]), int64(src.Pix[i+1]), int64(src.Pix[i+2])}
			if a != 255 {
				for ch := range px {
					px[ch] = min((px[ch]*255+a/2)/a, 255)
				}
			}

			f := k.keep(px[0], px[1], px[2])
			if f == 0 {
				out.Pix[i], out.Pix[i+1], out.Pix[i+2], out.Pix[i+3] = 0, 0, 0, 0

				continue
			}

			k.despill(&px)

			na := div255(a * f)
			out.Pix[i], out.Pix[i+1], out.Pix[i+2] = u8(div255(px[0]*na)), u8(div255(px[1]*na)), u8(div255(px[2]*na))
			out.Pix[i+3] = u8(na)
		}
	})

	return out
}
