package raster

import (
	"fmt"
	"math"
	"runtime"
	"sync"

	"github.com/go-text/typesetting/font"

	"github.com/AlexGalhardo/vukem-video/core/plan"
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

// textJob is a text or caption bitmap before any pixel is drawn: the laid out
// glyphs and the geometry of the bitmap. Measure and Render share it, so the
// size the plan places a bitmap by is the size Render produces.
type textJob struct {
	st    style
	s     float64 // pixel scale
	size  float64 // font size in bitmap pixels
	lay   layout
	words []plan.Rect

	baseLines, hiLines, allLines []polyline

	block, box, hiBox extent
	hasHi             bool
	shadow            bool
	blur              int

	// ox and oy are the origin of the bitmap in block coordinates, w and h its
	// size in pixels.
	ox, oy float64
	w, h   int
}

// newTextJob lays the text out and works out the geometry. active is the index
// of the highlighted word, or -1.
func newTextJob(st style, s float64, face *font.Face, lay layout, spans [][2]int, active int) *textJob {
	j := &textJob{st: st, s: s, size: mul(st.size, s), lay: lay, words: lay.words}
	k := lay.scale

	var hiGlyphs, baseGlyphs []placedGlyph
	if active >= 0 {
		hiGlyphs, baseGlyphs = split(lay.glyphs, spans[active])
	} else {
		baseGlyphs = lay.glyphs
	}

	j.baseLines, j.hiLines = outlines(face, baseGlyphs, k), outlines(face, hiGlyphs, k)
	j.allLines = append(append([]polyline{}, j.baseLines...), j.hiLines...)

	half := mul(st.strokeWidth, s) / 2
	j.block = extent{0, 0, lay.width, lay.height}
	ink := linesExtent(j.allLines).grow(half)
	content := j.block.grow(half).union(ink)
	j.box = content

	if st.background != nil {
		j.box = j.block.grow(mul(st.backgroundPadding, s))
		content = content.union(j.box)
	}

	if active >= 0 && st.highlightBackground != nil {
		w := lay.words[active]
		pad := mul(0.1, j.size)
		j.hiBox, j.hasHi = extent{w.X - pad, w.Y, w.X + w.W + pad, w.Y + w.H}, true
		content = content.union(j.hiBox)
	}

	j.shadow = st.shadowColor.A > 0 && (st.shadowDX != 0 || st.shadowDY != 0 || st.shadowBlur != 0)
	j.blur = int(math.Ceil(st.shadowBlur * s))

	if j.shadow {
		content = content.union(ink.shift(mul(st.shadowDX, s), mul(st.shadowDY, s)).grow(float64(3 * j.blur)))
	}

	// One clear pixel all around: the antialiased edge of the ink may touch the
	// last pixel the bounding box reaches, and the bitmap must never clip it.
	content = content.grow(1)

	// The plan places the centre of the text block and the browser draws a bitmap by its own centre, so
	// the bitmap reaches as far on each side of the block's centre as it does on the farther one: a
	// shadow no longer moves the bitmap's centre off the block's (CenterDX and CenterDY stay under half
	// a pixel) and the preview and the compositor put the text in the same place.
	cx, cy := (j.block.x0+j.block.x1)/2, (j.block.y0+j.block.y1)/2
	j.ox, j.w = symmetricSpan(cx, content.x0, content.x1)
	j.oy, j.h = symmetricSpan(cy, content.y0, content.y1)

	return j
}

// symmetricSpan is the whole-pixel origin and length of a canvas that holds [lo, hi] and whose centre is
// within half a pixel of c (lo <= c <= hi). The tight canvas is kept when it is already that close, so a
// bitmap without a lopsided shadow keeps its size.
func symmetricSpan(c, lo, hi float64) (origin float64, length int) {
	origin = math.Floor(lo)
	length = int(math.Ceil(hi - origin))

	if math.Abs(origin+float64(length)/2-c) <= 0.5 {
		return origin, length
	}

	origin = math.Floor(c - math.Max(c-lo, hi-c))

	return origin, int(math.Ceil(2 * (c - origin)))
}

// naturalSize is the size of the bitmap at PixelScale 1.
func (j *textJob) naturalSize() (w, h float64) {
	return float64(j.w) / j.s, float64(j.h) / j.s
}

// draw paints the bitmap.
func (j *textJob) draw() (Bitmap, error) {
	st, s := j.st, j.s

	cv, err := newCanvas(j.w, j.h)
	if err != nil {
		return Bitmap{}, err
	}

	pt := newPainter(j.w, j.h)
	origin := point{-j.ox, -j.oy}

	if st.background != nil {
		r := roundedRect(j.box.x0, j.box.y0, j.box.x1-j.box.x0, j.box.y1-j.box.y0, mul(st.backgroundRadius, s))
		cv.over(pt.fill(origin, r.pts), *st.background)
	}

	if j.hasHi {
		radius := math.Max(mul(st.backgroundRadius, s), mul(0.15, j.size))
		r := roundedRect(j.hiBox.x0, j.hiBox.y0, j.hiBox.x1-j.hiBox.x0, j.hiBox.y1-j.hiBox.y0, radius)
		cv.over(pt.fill(origin, r.pts), *st.highlightBackground)
	}

	strokes := strokeShapes(j.allLines, mul(st.strokeWidth, s))

	if j.shadow {
		off := point{origin.x + mul(st.shadowDX, s), origin.y + mul(st.shadowDY, s)}
		m := pt.fill(off, append(polygons(j.allLines), strokes...)...)
		boxBlur3(m, j.blur)
		cv.over(m, st.shadowColor)
	}

	if len(strokes) > 0 {
		cv.over(pt.fill(origin, strokes...), st.strokeColor)
	}

	cv.over(pt.fill(origin, polygons(j.baseLines)...), st.color)

	if len(j.hiLines) > 0 {
		cv.over(pt.fill(origin, polygons(j.hiLines)...), st.highlightColor)
	}

	return finish(cv, s, j.block.shift(-j.ox, -j.oy), j.words, j.ox, j.oy), nil
}

// finish builds the Bitmap with its metrics at PixelScale 1. block is the text
// block in canvas pixels; words are in block pixels, ox and oy the canvas origin.
func finish(cv *canvas, s float64, block extent, words []plan.Rect, ox, oy float64) Bitmap {
	b := Bitmap{
		Pix: cv.pix, Width: cv.w, Height: cv.h,
		NaturalW: float64(cv.w) / s, NaturalH: float64(cv.h) / s,
		CenterDX: ((block.x0+block.x1)/2 - float64(cv.w)/2) / s,
		CenterDY: ((block.y0+block.y1)/2 - float64(cv.h)/2) / s,
	}

	for _, w := range words {
		b.WordBoxes = append(b.WordBoxes, plan.Rect{X: (w.X - ox) / s, Y: (w.Y - oy) / s, W: w.W / s, H: w.H / s})
	}

	return b
}

// shapeBox is the canvas of a shape and the shape's box in it, in pixels.
type shapeBox struct {
	cw, ch     int
	x, y, w, h float64
}

// shapeGeometry sizes the canvas of a shape: the shape plus half its stroke on
// every side, with the shape centred.
func shapeGeometry(sh shape, s float64) shapeBox {
	w, h := mul(sh.width, s), mul(sh.height, s)
	half := mul(sh.strokeWidth, s) / 2
	cw, ch := int(math.Ceil(w+2*half)), int(math.Ceil(h+2*half))

	return shapeBox{cw: cw, ch: ch, x: (float64(cw) - w) / 2, y: (float64(ch) - h) / 2, w: w, h: h}
}

func renderShape(sh shape, s float64) (Bitmap, error) {
	g := shapeGeometry(sh, s)
	cw, ch, x, y, w, h := g.cw, g.ch, g.x, g.y, g.w, g.h

	cv, err := newCanvas(cw, ch)
	if err != nil {
		return Bitmap{}, err
	}

	var outline polyline

	switch sh.kind {
	case ShapeRect:
		outline = roundedRect(x, y, w, h, 0)
	case ShapeRoundedRect:
		outline = roundedRect(x, y, w, h, mul(sh.radius, s))
	case ShapeEllipse:
		outline = ellipse(x, y, w, h)
	default:
		return Bitmap{}, fmt.Errorf("%w: shape %q", ErrInvalidSpec, sh.kind)
	}

	pt := newPainter(cw, ch)
	cv.over(pt.fill(point{}, outline.pts), sh.fill)

	if strokes := strokeShapes([]polyline{outline}, mul(sh.strokeWidth, s)); len(strokes) > 0 {
		cv.over(pt.fill(point{}, strokes...), sh.strokeColor)
	}

	return finish(cv, s, extent{x, y, x + w, y + h}, nil, 0, 0), nil
}
