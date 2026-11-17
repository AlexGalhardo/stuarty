package compositor

import "math"

// applyMatrix returns src with the 3x4 colour matrix applied to its non-linear RGB values
// (SDD-004 section 3.5), clamped. Coefficients are rounded to 16.16 fixed point once, so the
// per-pixel work is integer arithmetic. The result lives in scratch memory that the next call
// reuses.
func (c *Compositor) applyMatrix(src Image, m *[12]float64) Image {
	var q [12]int64
	for i, v := range m {
		q[i] = int64(math.Round(mul(v, 65536)))
	}

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
			nr := clamp8((q[0]*r + q[1]*g + q[2]*b + q[3]*255 + 32768) >> 16)
			ng := clamp8((q[4]*r + q[5]*g + q[6]*b + q[7]*255 + 32768) >> 16)
			nb := clamp8((q[8]*r + q[9]*g + q[10]*b + q[11]*255 + 32768) >> 16)
			if a != 255 {
				nr, ng, nb = div255(nr*a), div255(ng*a), div255(nb*a)
			}
			out.Pix[i], out.Pix[i+1], out.Pix[i+2], out.Pix[i+3] = u8(nr), u8(ng), u8(nb), src.Pix[i+3]
		}
	})
	return out
}

func clamp8(v int64) int64 { return min(max(v, 0), 255) }

// colourMemo remembers the last colours a band of a frame went through a pure per-pixel colour function: video has
// runs and areas of one colour, so a hit returns the same integers as calling the function.
type colourMemo struct {
	key [colourMemoSize]uint32
	val [colourMemoSize]uint32
}

const colourMemoSize = 4096

// pixel is fn(r, g, b), where fn returns channels of 0 to 255.
func (m *colourMemo) pixel(fn func(r, g, b int64) (int64, int64, int64), r, g, b int64) (int64, int64, int64) {
	// The key has a bit above the colour, so an empty slot (0) never matches.
	key := uint32(r)<<16 | uint32(g)<<8 | uint32(b) | 1<<24 //nolint:gosec // G115: channels are 0 to 255
	h := (key * 2654435761) >> (32 - 12)

	if m.key[h] == key {
		v := m.val[h]

		return int64(v >> 16), int64(v >> 8 & 255), int64(v & 255)
	}

	nr, ng, nb := fn(r, g, b)
	m.key[h], m.val[h] = key, uint32(nr)<<16|uint32(ng)<<8|uint32(nb) //nolint:gosec // G115: channels are 0 to 255

	return nr, ng, nb
}
