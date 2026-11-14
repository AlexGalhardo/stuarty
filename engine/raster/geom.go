package raster

import (
	"image"
	"image/draw"
	"math"
	"slices"

	"golang.org/x/image/vector"
)

// point is a position in bitmap pixels, y down.
type point struct{ x, y float64 }

// polyline is a sequence of points. A closed polyline repeats no point: the
// last point joins the first.
type polyline struct {
	pts    []point
	closed bool
}

// The geometry below avoids expressions of the form a*b+c on purpose: Go may
// fuse them into one instruction on arm64, which rounds differently, and the
// bitmaps must be byte-equal on every platform. float64() around a product
// forbids the fusion (Go specification, "Floating-point operators").

func mul(a, b float64) float64 { return float64(a * b) }

func lerp(a, b, t float64) float64 { return a + float64(t*(b-a)) }

func lerpPoint(a, b point, t float64) point {
	return point{lerp(a.x, b.x, t), lerp(a.y, b.y, t)}
}

// flatTolerance is how far, in pixels, a flattened curve may stray from the
// true curve.
const flatTolerance = 0.1

// curveSteps is the number of line segments of a curve whose control polygon
// has the given largest second difference (the distance of a control point from
// the chord of its neighbours): the deviation of a quadratic curve cut in n
// equal parameter steps is that value over 4n squared, and the same bound is
// used for a cubic with its larger second difference. At least 2, at most 64.
func curveSteps(secondDiff float64) int {
	n := int(math.Ceil(math.Sqrt(secondDiff / (4 * flatTolerance))))

	return min(max(n, 2), 64)
}

// secondDiff is the length of p0 - 2*p1 + p2.
func secondDiff(p0, p1, p2 point) float64 {
	return hypot(p0.x-2*p1.x+p2.x, p0.y-2*p1.y+p2.y)
}

func quadAt(p0, p1, p2 point, t float64) point {
	a, b := lerpPoint(p0, p1, t), lerpPoint(p1, p2, t)

	return lerpPoint(a, b, t)
}

func cubeAt(p0, p1, p2, p3 point, t float64) point {
	a, b, c := lerpPoint(p0, p1, t), lerpPoint(p1, p2, t), lerpPoint(p2, p3, t)

	return lerpPoint(lerpPoint(a, b, t), lerpPoint(b, c, t), t)
}

// appendQuad appends the points of a quadratic curve after p0 (p0 itself is
// already in the path).
func appendQuad(pts []point, p0, p1, p2 point) []point {
	n := curveSteps(secondDiff(p0, p1, p2))
	for i := 1; i <= n; i++ {
		pts = append(pts, quadAt(p0, p1, p2, float64(i)/float64(n)))
	}

	return pts
}

func appendCube(pts []point, p0, p1, p2, p3 point) []point {
	n := curveSteps(math.Max(secondDiff(p0, p1, p2), secondDiff(p1, p2, p3)))
	for i := 1; i <= n; i++ {
		pts = append(pts, cubeAt(p0, p1, p2, p3, float64(i)/float64(n)))
	}

	return pts
}

// appendArc appends the points of a circular arc around c from angle a0 to a1
// (radians, y down), excluding the start point.
func appendArc(pts []point, c point, r, a0, a1 float64) []point {
	n := arcSteps(math.Abs(a1-a0), r)
	for i := 1; i <= n; i++ {
		a := lerp(a0, a1, float64(i)/float64(n))
		s, co := sincos(a)
		pts = append(pts, point{c.x + mul(r, co), c.y + mul(r, s)})
	}

	return pts
}

// roundedRect is the outline of a rectangle with rounded corners, clockwise on
// screen (y down). A radius of 0 gives a plain rectangle.
func roundedRect(x, y, w, h, radius float64) polyline {
	r := math.Min(radius, math.Min(w, h)/2)
	if r <= 0 {
		return polyline{pts: []point{{x, y}, {x + w, y}, {x + w, y + h}, {x, y + h}}, closed: true}
	}

	var pts []point

	pts = append(pts, point{x + r, y}, point{x + w - r, y})
	pts = appendArc(pts, point{x + w - r, y + r}, r, -math.Pi/2, 0)
	pts = append(pts, point{x + w, y + h - r})
	pts = appendArc(pts, point{x + w - r, y + h - r}, r, 0, math.Pi/2)
	pts = append(pts, point{x + r, y + h})
	pts = appendArc(pts, point{x + r, y + h - r}, r, math.Pi/2, math.Pi)
	pts = append(pts, point{x, y + r})
	pts = appendArc(pts, point{x + r, y + r}, r, math.Pi, 3*math.Pi/2)

	return polyline{pts: pts[:len(pts)-1], closed: true}
}

// ellipse is the outline of an ellipse inscribed in the rectangle, clockwise
// on screen.
func ellipse(x, y, w, h float64) polyline {
	rx, ry := w/2, h/2
	cx, cy := x+rx, y+ry
	n := arcSteps(2*math.Pi, math.Max(rx, ry))
	pts := make([]point, 0, n)

	for i := range n {
		s, c := sincos(float64(i) * twoPi / float64(n))
		pts = append(pts, point{cx + mul(rx, c), cy + mul(ry, s)})
	}

	return polyline{pts: pts, closed: true}
}

// arcSteps is the number of segments of an arc of the given angle and radius
// that stays within flatTolerance of the circle.
func arcSteps(angle, r float64) int {
	if r <= flatTolerance {
		return 2
	}

	// Sagitta of a chord: the arc strays r*(1-cos(step/2)), about r*step^2/8.
	step := 2 * math.Sqrt(2*flatTolerance/r)

	return min(max(int(math.Ceil(angle/step)), 2), 256)
}

// strokeShapes returns polygons whose union is the stroke of the polylines
// with the given width: a rectangle along every segment and a round join (a
// pie slice of the circle) on the outer side of every vertex where the
// direction changes. Every polygon is clockwise on screen, so the non-zero
// accumulation of the rasterizer fills their union.
func strokeShapes(lines []polyline, width float64) [][]point {
	r := width / 2
	if r <= 0 {
		return nil
	}

	var out [][]point

	for _, pl := range lines {
		pts := dedupe(pl.pts, pl.closed)
		n := len(pts)

		if n == 1 {
			out = append(out, orient(circlePoints(pts[0], r)))

			continue
		}

		segments := n
		if !pl.closed {
			segments = n - 1
		}

		for i := range segments {
			a, b := pts[i], pts[(i+1)%n]
			out = append(out, orient(segmentQuad(a, b, r)))

			// The join at b, between this segment and the next one.
			if pl.closed || i+1 < segments {
				out = appendJoin(out, a, b, pts[(i+2)%n], r)
			}
		}
	}

	return out
}

// dedupe drops consecutive equal points (and, for a closed line, a last point
// equal to the first), which have no direction.
func dedupe(pts []point, closed bool) []point {
	out := make([]point, 0, len(pts))

	for _, p := range pts {
		if len(out) == 0 || p != out[len(out)-1] {
			out = append(out, p)
		}
	}

	if closed && len(out) > 1 && out[0] == out[len(out)-1] {
		out = out[:len(out)-1]
	}

	return out
}

// orient returns the polygon clockwise on screen (y down), reversing it when
// its signed area says otherwise.
func orient(poly []point) []point {
	var area float64

	for i, p := range poly {
		q := poly[(i+1)%len(poly)]
		area += float64(p.x*q.y) - float64(q.x*p.y)
	}

	if area < 0 {
		slices.Reverse(poly)
	}

	return poly
}

// appendJoin adds the round join at b of the path a, b, c: the pie between the
// normals of the two segments, on the outer side of the turn.
func appendJoin(out [][]point, a, b, c point, r float64) [][]point {
	d0x, d0y := b.x-a.x, b.y-a.y
	d1x, d1y := c.x-b.x, c.y-b.y
	l0 := hypot(d0x, d0y)
	l1 := hypot(d1x, d1y)

	if l0 == 0 || l1 == 0 {
		return out
	}

	cross := float64(d0x*d1y) - float64(d0y*d1x)
	dot := float64(d0x*d1x) + float64(d0y*d1y)
	angle := atan2(cross, dot)

	if math.Abs(angle) < 1e-6 {
		return out
	}

	// The outer side of a turn is the one the path turns away from.
	side := -1.0
	if cross < 0 {
		side = 1
	}

	a0 := atan2(side*d0x, -side*d0y)

	n := arcSteps(math.Abs(angle), r)
	pie := make([]point, 0, n+2)
	pie = append(pie, b)

	for i := 0; i <= n; i++ {
		s, co := sincos(a0 + lerp(0, angle, float64(i)/float64(n)))
		pie = append(pie, point{b.x + mul(r, co), b.y + mul(r, s)})
	}

	return append(out, orient(pie))
}

func circlePoints(c point, r float64) []point {
	n := arcSteps(2*math.Pi, r)
	pts := make([]point, n)

	for i := range n {
		s, co := sincos(float64(i) * twoPi / float64(n))
		pts[i] = point{c.x + mul(r, co), c.y + mul(r, s)}
	}

	return pts
}

// segmentQuad is the rectangle of half-width r around the segment a-b.
func segmentQuad(a, b point, r float64) []point {
	dx, dy := b.x-a.x, b.y-a.y
	l := hypot(dx, dy)
	// The normal, scaled to r.
	nx, ny := mul(-dy/l, r), mul(dx/l, r)

	return []point{{a.x - nx, a.y - ny}, {b.x - nx, b.y - ny}, {b.x + nx, b.y + ny}, {a.x + nx, a.y + ny}}
}

// mask is an 8-bit coverage image.
type mask struct {
	w, h int
	pix  []uint8
}

// painter rasterizes polygons into one reusable mask: a render fills many
// masks in a row, and allocating each of them dominates the cost of a small
// bitmap. The mask it returns is valid until the next call.
type painter struct {
	w, h int
	rz   vector.Rasterizer
	m    mask
}

func newPainter(w, h int) *painter {
	return &painter{w: w, h: h, m: mask{w: w, h: h, pix: make([]uint8, w*h)}}
}

// fill rasterizes the polygons, moved by off, into the mask. The polygons
// share one accumulation buffer, so overlapping polygons of the same
// orientation union and opposite ones cut holes.
func (p *painter) fill(off point, polys ...[]point) *mask {
	clear(p.m.pix)

	if p.w == 0 || p.h == 0 {
		return &p.m
	}

	p.rz.Reset(p.w, p.h)

	for _, poly := range polys {
		addPolygon(&p.rz, poly, off)
	}

	img := &image.Alpha{Pix: p.m.pix, Stride: p.w, Rect: image.Rect(0, 0, p.w, p.h)}
	p.rz.DrawOp = draw.Src
	p.rz.Draw(img, img.Rect, image.Opaque, image.Point{})

	return &p.m
}

func addPolygon(rz *vector.Rasterizer, poly []point, off point) {
	if len(poly) < 3 {
		return
	}

	rz.MoveTo(float32(poly[0].x+off.x), float32(poly[0].y+off.y))

	for _, q := range poly[1:] {
		rz.LineTo(float32(q.x+off.x), float32(q.y+off.y))
	}

	rz.ClosePath()
}
