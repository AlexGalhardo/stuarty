package raster

import (
	"fmt"
	"math"
	"runtime"
	"sync"

	"github.com/go-text/typesetting/font"
)

// extent is a rectangle given by its edges, in bitmap pixels.
type extent struct{ x0, y0, x1, y1 float64 }

func emptyExtent() extent { return extent{math.Inf(1), math.Inf(1), math.Inf(-1), math.Inf(-1)} }

func (e extent) union(o extent) extent {
	return extent{math.Min(e.x0, o.x0), math.Min(e.y0, o.y0), math.Max(e.x1, o.x1), math.Max(e.y1, o.y1)}
}

func (e extent) grow(by float64) extent { return extent{e.x0 - by, e.y0 - by, e.x1 + by, e.y1 + by} }

func (e extent) shift(dx, dy float64) extent {
	return extent{e.x0 + dx, e.y0 + dy, e.x1 + dx, e.y1 + dy}
}

func linesExtent(lines []polyline) extent {
	e := emptyExtent()

	for _, l := range lines {
		for _, p := range l.pts {
			e = e.union(extent{p.x, p.y, p.x, p.y})
		}
	}

	return e
}

// canvas is an RGBA bitmap with straight alpha.
type canvas struct {
	w, h int
	pix  []byte
}

func newCanvas(w, h int) (*canvas, error) {
	if w <= 0 || h <= 0 || w*h > MaxPixels {
		return nil, fmt.Errorf("%w: bitmap of %d by %d pixels", ErrInvalidSpec, w, h)
	}

	return &canvas{w: w, h: h, pix: make([]byte, w*h*4)}, nil
}

// over composites the colour through the coverage mask onto the canvas.
func (c *canvas) over(m *mask, col Colour) {
	if col.A == 0 {
		return
	}

	for i, cov := range m.pix {
		if cov == 0 {
			continue
		}

		sa := (uint32(cov)*uint32(col.A) + 127) / 255
		c.blend(i*4, col, sa)
	}
}

// blend composites the colour with alpha sa (0 to 255) onto the pixel at
// offset o, straight alpha, integer arithmetic only.
func (c *canvas) blend(o int, col Colour, sa uint32) {
	if sa == 0 {
		return
	}

	d := c.pix[o : o+4 : o+4]
	da := uint32(d[3])

	// Over an empty pixel, or with an opaque source, the result is the source:
	// the general formula below gives the same bytes, without three divisions.
	if da == 0 || sa == 255 {
		d[0], d[1], d[2], d[3] = col.R, col.G, col.B, u8(sa)

		return
	}

	keep := (da*(255-sa) + 127) / 255
	oa := sa + keep

	d[0] = mix(uint32(col.R), sa, uint32(d[0]), keep, oa)
	d[1] = mix(uint32(col.G), sa, uint32(d[1]), keep, oa)
	d[2] = mix(uint32(col.B), sa, uint32(d[2]), keep, oa)
	d[3] = u8(oa)
}

// u8 narrows a value that is at most 255 by construction: every caller
// divides a weighted sum of 8-bit values by the sum of the weights.
func u8[T ~int | ~uint32](v T) uint8 {
	return uint8(v)
}

func mix(s, sa, d, keep, oa uint32) uint8 {
	return u8((s*sa + d*keep + oa/2) / oa)
}

// boxBlur3 blurs the mask with a box blur of the given radius applied three
// times, in place (SDD-004 section 5.1 item 5). The rows (then the columns) are
// independent, so bands of them run in parallel; the bytes do not depend on the
// number of bands.
func boxBlur3(m *mask, radius int) {
	if radius <= 0 {
		return
	}

	tmp := make([]uint8, len(m.pix))
	bands := min(runtime.GOMAXPROCS(0), 4)

	for range 3 {
		parallel(bands, m.h, func(y0, y1 int) { blurRows(m.pix, tmp, m.w, y0, y1, radius) })
		parallel(bands, m.w, func(x0, x1 int) { blurColumns(tmp, m.pix, m.w, m.h, x0, x1, radius) })
	}
}

// parallel splits [0, n) into at most bands ranges and runs f on each.
func parallel(bands, n int, f func(lo, hi int)) {
	bands = max(min(bands, n/64), 1)
	if bands == 1 {
		f(0, n)

		return
	}

	var wg sync.WaitGroup

	for b := range bands {
		wg.Go(func() { f(b*n/bands, (b+1)*n/bands) })
	}

	wg.Wait()
}

// blurRows blurs the rows [y0, y1) of src into dst along x, treating pixels
// outside the mask as zero.
func blurRows(src, dst []uint8, w, y0, y1, r int) {
	div := 2*r + 1

	for y := y0; y < y1; y++ {
		row, out := src[y*w:(y+1)*w], dst[y*w:(y+1)*w]

		sum := 0
		for x := 0; x <= r && x < w; x++ {
			sum += int(row[x])
		}

		for x := range w {
			out[x] = u8((sum + div/2) / div)

			if old := x - r; old >= 0 {
				sum -= int(row[old])
			}

			if in := x + r + 1; in < w {
				sum += int(row[in])
			}
		}
	}
}

// blurColumns blurs the columns [x0, x1) of src into dst along y. It walks the
// rows in order with one running sum per column, which keeps the memory access
// sequential.
func blurColumns(src, dst []uint8, w, h, x0, x1, r int) {
	div := 2*r + 1
	sums := make([]int, x1-x0)

	for y := 0; y <= r && y < h; y++ {
		for i, v := range src[y*w+x0 : y*w+x1] {
			sums[i] += int(v)
		}
	}

	for y := range h {
		out := dst[y*w+x0 : y*w+x1]
		for i := range out {
			out[i] = u8((sums[i] + div/2) / div)
		}

		if old := y - r; old >= 0 {
			for i, v := range src[old*w+x0 : old*w+x1] {
				sums[i] -= int(v)
			}
		}

		if in := y + r + 1; in < h {
			for i, v := range src[in*w+x0 : in*w+x1] {
				sums[i] += int(v)
			}
		}
	}
}

// split divides glyphs into the ones inside the rune span and the others.
func split(glyphs []placedGlyph, span [2]int) (inside, outside []placedGlyph) {
	for _, g := range glyphs {
		if g.text >= span[0] && g.text < span[1] {
			inside = append(inside, g)
		} else {
			outside = append(outside, g)
		}
	}

	return inside, outside
}

// drawText draws a text or caption bitmap. active is the index of the
// highlighted word, or -1.
func drawText(spec BitmapSpec, face *font.Face, lay layout, spans [][2]int, active int) (Bitmap, error) {
	st, s := spec.Style, spec.PixelScale
	size := mul(st.Size, s)
	k := lay.scale

	var hiGlyphs, baseGlyphs []placedGlyph
	if active >= 0 {
		hiGlyphs, baseGlyphs = split(lay.glyphs, spans[active])
	} else {
		baseGlyphs = lay.glyphs
	}

	baseLines, hiLines := outlines(face, baseGlyphs, k), outlines(face, hiGlyphs, k)
	allLines := append(append([]polyline{}, baseLines...), hiLines...)

	half := mul(st.StrokeWidth, s) / 2
	block := extent{0, 0, lay.width, lay.height}
	ink := linesExtent(allLines).grow(half)
	content := block.grow(half).union(ink)
	box := content

	if st.BackgroundColor != nil {
		box = block.grow(mul(st.BackgroundPadding, s))
		content = content.union(box)
	}

	var hiBox extent

	if active >= 0 && st.HighlightBackground != nil {
		w := lay.words[active]
		pad := mul(0.1, size)
		hiBox = extent{w.X - pad, w.Y, w.X + w.W + pad, w.Y + w.H}
		content = content.union(hiBox)
	}

	shadow := st.ShadowColor.A > 0 && (st.ShadowDX != 0 || st.ShadowDY != 0 || st.ShadowBlur != 0)
	blur := int(math.Ceil(st.ShadowBlur * s))

	if shadow {
		content = content.union(ink.shift(mul(st.ShadowDX, s), mul(st.ShadowDY, s)).grow(float64(3 * blur)))
	}

	// One clear pixel all around: the antialiased edge of the ink may touch the
	// last pixel the bounding box reaches, and the bitmap must never clip it.
	content = content.grow(1)
	ox, oy := math.Floor(content.x0), math.Floor(content.y0)
	w, h := int(math.Ceil(content.x1-ox)), int(math.Ceil(content.y1-oy))

	cv, err := newCanvas(w, h)
	if err != nil {
		return Bitmap{}, err
	}

	pt := newPainter(w, h)
	origin := point{-ox, -oy}

	if st.BackgroundColor != nil {
		r := roundedRect(box.x0, box.y0, box.x1-box.x0, box.y1-box.y0, mul(st.BackgroundRadius, s))
		cv.over(pt.fill(origin, r.pts), *st.BackgroundColor)
	}

	if hiBox != (extent{}) {
		radius := math.Max(mul(st.BackgroundRadius, s), mul(0.15, size))
		r := roundedRect(hiBox.x0, hiBox.y0, hiBox.x1-hiBox.x0, hiBox.y1-hiBox.y0, radius)
		cv.over(pt.fill(origin, r.pts), *st.HighlightBackground)
	}

	strokes := strokeShapes(allLines, mul(st.StrokeWidth, s))

	if shadow {
		off := point{origin.x + mul(st.ShadowDX, s), origin.y + mul(st.ShadowDY, s)}
		m := pt.fill(off, append(polygons(allLines), strokes...)...)
		boxBlur3(m, blur)
		cv.over(m, st.ShadowColor)
	}

	if len(strokes) > 0 {
		cv.over(pt.fill(origin, strokes...), st.StrokeColor)
	}

	cv.over(pt.fill(origin, polygons(baseLines)...), st.Color)

	if len(hiLines) > 0 {
		cv.over(pt.fill(origin, polygons(hiLines)...), st.HighlightColor)
	}

	return finish(cv, s, block.shift(-ox, -oy), lay.words, ox, oy), nil
}

// finish builds the Bitmap with its metrics at PixelScale 1. block is the text
// block in canvas pixels; words are in block pixels, ox and oy the canvas origin.
func finish(cv *canvas, s float64, block extent, words []Rect, ox, oy float64) Bitmap {
	b := Bitmap{
		Pix: cv.pix, Width: cv.w, Height: cv.h,
		NaturalW: float64(cv.w) / s, NaturalH: float64(cv.h) / s,
		CenterDX: ((block.x0+block.x1)/2 - float64(cv.w)/2) / s,
		CenterDY: ((block.y0+block.y1)/2 - float64(cv.h)/2) / s,
	}

	for _, w := range words {
		b.WordBoxes = append(b.WordBoxes, Rect{X: (w.X - ox) / s, Y: (w.Y - oy) / s, W: w.W / s, H: w.H / s})
	}

	return b
}

func renderShape(spec BitmapSpec) (Bitmap, error) {
	sh, s := spec.Shape, spec.PixelScale
	w, h := mul(sh.Width, s), mul(sh.Height, s)
	half := mul(sh.StrokeWidth, s) / 2

	cw, ch := int(math.Ceil(w+2*half)), int(math.Ceil(h+2*half))

	cv, err := newCanvas(cw, ch)
	if err != nil {
		return Bitmap{}, err
	}

	// The shape is centred in the canvas.
	x, y := (float64(cw)-w)/2, (float64(ch)-h)/2

	var outline polyline

	switch sh.Shape {
	case ShapeRect:
		outline = roundedRect(x, y, w, h, 0)
	case ShapeRoundedRect:
		outline = roundedRect(x, y, w, h, mul(sh.Radius, s))
	case ShapeEllipse:
		outline = ellipse(x, y, w, h)
	default:
		return Bitmap{}, fmt.Errorf("%w: shape %q", ErrInvalidSpec, sh.Shape)
	}

	pt := newPainter(cw, ch)
	cv.over(pt.fill(point{}, outline.pts), sh.Fill)

	if strokes := strokeShapes([]polyline{outline}, mul(sh.StrokeWidth, s)); len(strokes) > 0 {
		cv.over(pt.fill(point{}, strokes...), sh.StrokeColor)
	}

	return finish(cv, s, extent{x, y, x + w, y + h}, nil, 0, 0), nil
}
