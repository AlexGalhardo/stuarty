package compositor

import (
	"encoding/binary"
	"math"
	"sync"
)

// maxBlurRadius is the largest box radius of one pass, in source pixels: with the 255 of a channel it
// is the largest window whose sum fits a 16-bit lane (255 x 257 = 65535). The blur field is at most
// 100 px at project resolution (SDD-001 3.4); a source that is drawn smaller than it is stored, or
// an export above twice the project size, would ask for more and gets this radius.
const maxBlurRadius = 128

// blurRadius is the radius r of the box that three passes turn into a Gaussian of standard
// deviation sigma: three boxes of width 2r+1 have the variance r(r+1), so r is the positive root
// of r(r+1) = sigma^2, rounded. A sigma under about 0.8 pixels gives 0 and no blur. This is the
// method of the rasterizer for shadows (SDD-004 section 5.1 item 5), so a caption shadow and a
// blurred layer agree.
func blurRadius(sigma float64) int {
	if !(sigma > 0) {
		return 0
	}

	r := int(math.Round((math.Sqrt(mul(4, mul(sigma, sigma))+1) - 1) / 2))

	return min(r, maxBlurRadius)
}

// blurred returns src blurred by sigma, a standard deviation in output pixels (SDD-004 section
// 3.6), and the placement that draws it. The blur is of the layer's own premultiplied pixels with
// the outside counted as transparent, so the result is larger than src by three radii on every
// side and the placement grows by the same amount around the same centre: the layer softens past
// its edge. The radii are taken in source pixels, from the scale at which src is drawn, so the
// blur has the same reach on the frame whatever the size of the source. Three box passes along
// each axis, in integer arithmetic that rounds half up at every pass; rows (then columns) are
// independent, so the bytes do not depend on the number of bands.
func (c *Compositor) blurred(src Image, p placement, sigma float64) (Image, placement) {
	rx := blurRadius(sigma / (p.w / float64(src.W)))
	ry := blurRadius(sigma / (p.h / float64(src.H)))

	if rx == 0 && ry == 0 {
		return src, p
	}

	px, py := 3*rx, 3*ry
	w, h := src.W+2*px, src.H+2*py
	n := w * h * 4

	if cap(c.blurA) < n {
		c.blurA, c.blurB = make([]byte, n), make([]uint32, w*h)
	}

	rows, out := c.blurB[:w*h], c.blurA[:n]
	tx, ty := roundTable(rx), roundTable(ry)

	c.runBands(0, h, w, func(y0, y1 int) { blurRows3(src, rows, w, px, py, y0, y1, rx, tx) })
	c.runBands(0, w, h, func(x0, x1 int) { blurColumns3(rows, out, w, h, x0, x1, ry, ty) })

	grown := placement{
		cx: p.cx, cy: p.cy, rot: p.rot,
		w: mul(p.w, float64(w)) / float64(src.W), h: mul(p.h, float64(h)) / float64(src.H),
	}

	return Image{Pix: out, W: w, H: h}, grown
}

// meanTable maps the sum of a window of 2r+1 bytes, at most 65535, to its mean rounded half up, so
// a pass needs no division; its size lets a lane masked to 16 bits index it without a check.
type meanTable [1 << 16]uint8

func roundTable(r int) *meanTable {
	div := 2*r + 1
	t := new(meanTable)

	for s := range 255*div + 1 {
		t[s] = uint8((s + div/2) / div) //nolint:gosec // G115: a sum of at most 255*div over div is at most 255
	}

	return t
}

// A pixel is held in a uint32 with its bytes in the order of the image. The sum of a window is kept
// in one uint64 with a 16-bit lane for each channel: a lane holds at most 255 x (2 x 128 + 1), so no
// lane carries into the next, and one add or subtract moves four channels.

// widen spreads the four bytes of a pixel into the lanes of a sum.
func widen(p uint32) uint64 {
	x := uint64(p)
	x = (x | x<<16) & 0x0000FFFF0000FFFF
	x = (x | x<<8) & 0x00FF00FF00FF00FF

	return x
}

// narrow turns a sum into the pixel of its means, by the table of roundTable.
func narrow(t *meanTable, s uint64) uint32 {
	const m = 1<<16 - 1

	return uint32(t[s&m]) | uint32(t[s>>16&m])<<8 | uint32(t[s>>32&m])<<16 | uint32(t[s>>48])<<24
}

// blurRows3 writes the rows [y0, y1) of dst, w pixels wide, as the source image src placed at
// (px, py) in a transparent field and blurred three times along x. A row is a few kilobytes, so the
// three passes of one row stay in the first-level cache and the image is read and written once.
func blurRows3(src Image, dst []uint32, w, px, py, y0, y1, r int, t *meanTable) {
	a, b := make([]uint32, w), make([]uint32, w)

	for y := y0; y < y1; y++ {
		out := dst[y*w : (y+1)*w]

		sy := y - py
		if sy < 0 || sy >= src.H {
			clear(out)

			continue
		}

		clear(a)

		line := src.Pix[sy*src.W*4 : (sy+1)*src.W*4]
		for x := range src.W {
			a[px+x] = binary.LittleEndian.Uint32(line[4*x:])
		}

		blurRow(a, b, r, t)
		blurRow(b, a, r, t)
		blurRow(a, out, r, t)
	}
}

// blurRow blurs one row of pixels into out; the pixels outside the row count as zero.
func blurRow(row, out []uint32, r int, t *meanTable) {
	w := len(row)
	out = out[:w]

	var sum uint64

	for x := 0; x <= r && x < w; x++ {
		sum += widen(row[x])
	}

	if w <= 2*r+1 {
		for x := range w {
			out[x] = narrow(t, sum)

			if old := x - r; old >= 0 {
				sum -= widen(row[old])
			}

			if in := x + r + 1; in < w {
				sum += widen(row[in])
			}
		}

		return
	}

	// Away from the ends the window gains a pixel and loses one; at each end only one of the two.
	for x := range r {
		out[x] = narrow(t, sum)
		sum += widen(row[x+r+1])
	}

	for x := r; x < w-r-1; x++ {
		out[x] = narrow(t, sum)
		sum = sum + widen(row[x+r+1]) - widen(row[x-r])
	}

	for x := w - r - 1; x < w; x++ {
		out[x] = narrow(t, sum)
		sum -= widen(row[x-r])
	}
}

// stripeWidth is the number of pixel columns a vertical pass handles together. A stripe of a
// 1920 row image is 1920 x 32 x 4 bytes, which two buffers keep in the second-level cache.
const stripeWidth = 32

var stripes = sync.Pool{New: func() any { return new([2][]uint32) }}

// blurColumns3 blurs the pixel columns [x0, x1) of src three times along y into dst. It works one
// stripe of columns at a time on a copy of the stripe, so the three passes read memory that stays
// cached and the rows of the image are walked once.
func blurColumns3(src []uint32, dst []byte, w, h, x0, x1, r int, t *meanTable) {
	buf, ok := stripes.Get().(*[2][]uint32)
	if !ok {
		buf = new([2][]uint32)
	}

	defer stripes.Put(buf)

	sums := make([]uint64, stripeWidth)

	for sx := x0; sx < x1; sx += stripeWidth {
		ex := min(sx+stripeWidth, x1)
		sw := ex - sx
		n := h * sw

		if cap(buf[0]) < n {
			buf[0], buf[1] = make([]uint32, n), make([]uint32, n)
		}

		a, b := buf[0][:n], buf[1][:n]
		for y := range h {
			copy(a[y*sw:(y+1)*sw], src[y*w+sx:y*w+ex])
		}

		blurStripe(a, b, h, sw, r, t, sums[:sw])
		blurStripe(b, a, h, sw, r, t, sums[:sw])
		blurStripe(a, b, h, sw, r, t, sums[:sw])

		for y := range h {
			line := dst[(y*w+sx)*4 : (y*w+ex)*4]
			for i, p := range b[y*sw : (y+1)*sw] {
				binary.LittleEndian.PutUint32(line[4*i:], p)
			}
		}
	}
}

// blurStripe is one pass along y over rows rows of sw pixels, with one running sum for each pixel
// of a row.
func blurStripe(in, out []uint32, rows, sw, r int, t *meanTable, sums []uint64) {
	clear(sums)

	for y := 0; y <= r && y < rows; y++ {
		for i, p := range in[y*sw : (y+1)*sw] {
			sums[i] += widen(p)
		}
	}

	for y := range rows {
		o := out[y*sw : (y+1)*sw]
		for i := range o {
			o[i] = narrow(t, sums[i])
		}

		if next := y + r + 1; next < rows {
			for i, p := range in[next*sw : (next+1)*sw] {
				sums[i] += widen(p)
			}
		}

		if last := y - r; last >= 0 {
			for i, p := range in[last*sw : (last+1)*sw] {
				sums[i] -= widen(p)
			}
		}
	}
}
