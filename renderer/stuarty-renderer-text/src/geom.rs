//! Polylines, curves, rounded rectangles, ellipses and strokes: the port of
//! `engine/raster/geom.go` and `detmath.go`.
//!
//! The expressions keep the order of the Go code and never use a fused multiply-add (Rust does not
//! contract `a * b + c`), so the bits are the same as `engine/raster` on every target.

use std::f64::consts::PI;

use crate::math::{mul, sincos};

use crate::scan::Rasterizer;

/// A position in bitmap pixels, y down.
#[derive(Clone, Copy, Debug, Default, PartialEq)]
pub struct Point {
	pub x: f64,
	pub y: f64,
}

pub const fn pt(x: f64, y: f64) -> Point {
	Point { x, y }
}

/// A sequence of points. A closed polyline repeats no point: the last joins the first.
#[derive(Clone, Debug, Default, PartialEq)]
pub struct Polyline {
	pub pts: Vec<Point>,
	pub closed: bool,
}

const TWO_PI: f64 = 2.0 * PI;
const HALF_PI: f64 = PI / 2.0;
/// `3 * math.Pi / 2` evaluated exactly, as Go does for a constant, then rounded once.
const THREE_HALF_PI: f64 = 4.712_388_980_384_689_857_693_965_074_919_254_3;

/// The length of `(x, y)`.
pub fn hypot(x: f64, y: f64) -> f64 {
	(mul(x, x) + mul(y, y)).sqrt()
}

/// The arc tangent of `t`, in `[-pi/2, pi/2]`, from exactly rounded operations only.
pub fn atan(t: f64) -> f64 {
	if t < 0.0 {
		return -atan(-t);
	}
	if t > 1.0 {
		return HALF_PI - atan(1.0 / t);
	}
	let mut t = t;
	for _ in 0..2 {
		t /= 1.0 + (1.0 + mul(t, t)).sqrt();
	}
	let t2 = mul(t, t);
	let (mut sum, mut term) = (t, t);
	for k in 1..=20_i32 {
		term = -mul(term, t2);
		sum += term / f64::from(2 * k + 1);
	}
	4.0 * sum
}

/// The angle of the vector `(x, y)`, in `(-pi, pi]`.
pub fn atan2(y: f64, x: f64) -> f64 {
	if x > 0.0 {
		atan(y / x)
	} else if x < 0.0 && y >= 0.0 {
		atan(y / x) + PI
	} else if x < 0.0 {
		atan(y / x) - PI
	} else if y > 0.0 {
		HALF_PI
	} else if y < 0.0 {
		-HALF_PI
	} else {
		0.0
	}
}

fn lerp(a: f64, b: f64, t: f64) -> f64 {
	a + mul(t, b - a)
}

fn lerp_point(a: Point, b: Point, t: f64) -> Point {
	pt(lerp(a.x, b.x, t), lerp(a.y, b.y, t))
}

/// How far, in pixels, a flattened curve may stray from the true curve.
const FLAT_TOLERANCE: f64 = 0.1;

fn curve_steps(second_diff: f64) -> i64 {
	let n = (second_diff / (4.0 * FLAT_TOLERANCE)).sqrt().ceil() as i64;
	n.clamp(2, 64)
}

fn second_diff(p0: Point, p1: Point, p2: Point) -> f64 {
	hypot(p0.x - 2.0 * p1.x + p2.x, p0.y - 2.0 * p1.y + p2.y)
}

fn quad_at(p0: Point, p1: Point, p2: Point, t: f64) -> Point {
	let (a, b) = (lerp_point(p0, p1, t), lerp_point(p1, p2, t));
	lerp_point(a, b, t)
}

fn cube_at(p0: Point, p1: Point, p2: Point, p3: Point, t: f64) -> Point {
	let (a, b, c) = (
		lerp_point(p0, p1, t),
		lerp_point(p1, p2, t),
		lerp_point(p2, p3, t),
	);
	lerp_point(lerp_point(a, b, t), lerp_point(b, c, t), t)
}

/// Appends the points of a quadratic curve after `p0` (already in the path).
pub fn append_quad(pts: &mut Vec<Point>, p0: Point, p1: Point, p2: Point) {
	let n = curve_steps(second_diff(p0, p1, p2));
	for i in 1..=n {
		pts.push(quad_at(p0, p1, p2, i as f64 / n as f64));
	}
}

/// Appends the points of a cubic curve after `p0` (already in the path).
pub fn append_cube(pts: &mut Vec<Point>, p0: Point, p1: Point, p2: Point, p3: Point) {
	let n = curve_steps(second_diff(p0, p1, p2).max(second_diff(p1, p2, p3)));
	for i in 1..=n {
		pts.push(cube_at(p0, p1, p2, p3, i as f64 / n as f64));
	}
}

/// Appends the points of an arc around `c` from angle `a0` to `a1` (radians, y down), excluding the
/// start point.
fn append_arc(pts: &mut Vec<Point>, c: Point, r: f64, a0: f64, a1: f64) {
	let n = arc_steps((a1 - a0).abs(), r);
	for i in 1..=n {
		let a = lerp(a0, a1, i as f64 / n as f64);
		let (s, co) = sincos(a);
		pts.push(pt(c.x + mul(r, co), c.y + mul(r, s)));
	}
}

/// The outline of a rectangle with rounded corners, clockwise on screen (y down). A radius of 0 gives
/// a plain rectangle.
pub fn rounded_rect(x: f64, y: f64, w: f64, h: f64, radius: f64) -> Polyline {
	let r = radius.min(w.min(h) / 2.0);
	if r <= 0.0 {
		return Polyline {
			pts: vec![pt(x, y), pt(x + w, y), pt(x + w, y + h), pt(x, y + h)],
			closed: true,
		};
	}
	let mut pts = vec![pt(x + r, y), pt(x + w - r, y)];
	append_arc(&mut pts, pt(x + w - r, y + r), r, -PI / 2.0, 0.0);
	pts.push(pt(x + w, y + h - r));
	append_arc(&mut pts, pt(x + w - r, y + h - r), r, 0.0, PI / 2.0);
	pts.push(pt(x + r, y + h));
	append_arc(&mut pts, pt(x + r, y + h - r), r, PI / 2.0, PI);
	pts.push(pt(x, y + r));
	append_arc(&mut pts, pt(x + r, y + r), r, PI, THREE_HALF_PI);
	pts.pop();
	Polyline { pts, closed: true }
}

/// The outline of an ellipse inscribed in the rectangle, clockwise on screen.
pub fn ellipse(x: f64, y: f64, w: f64, h: f64) -> Polyline {
	let (rx, ry) = (w / 2.0, h / 2.0);
	let (cx, cy) = (x + rx, y + ry);
	let n = arc_steps(2.0 * PI, rx.max(ry));
	let mut pts = Vec::with_capacity(n as usize);
	for i in 0..n {
		let (s, c) = sincos(i as f64 * TWO_PI / n as f64);
		pts.push(pt(cx + mul(rx, c), cy + mul(ry, s)));
	}
	Polyline { pts, closed: true }
}

/// The number of segments of an arc of the given angle and radius that stays within the flat
/// tolerance of the circle.
fn arc_steps(angle: f64, r: f64) -> i64 {
	if r <= FLAT_TOLERANCE {
		return 2;
	}
	let step = 2.0 * (2.0 * FLAT_TOLERANCE / r).sqrt();
	((angle / step).ceil() as i64).clamp(2, 256)
}

/// Polygons whose union is the stroke of the polylines with the given width: a rectangle along every
/// segment and a round join on the outer side of every vertex where the direction changes. Every
/// polygon is clockwise on screen, so the non-zero accumulation fills their union.
pub fn stroke_shapes(lines: &[Polyline], width: f64) -> Vec<Vec<Point>> {
	let r = width / 2.0;
	if r <= 0.0 {
		return Vec::new();
	}
	let mut out = Vec::new();
	for pl in lines {
		let pts = dedupe(&pl.pts, pl.closed);
		let n = pts.len();
		if n == 1 {
			out.push(orient(circle_points(pts[0], r)));
			continue;
		}
		let segments = if pl.closed { n } else { n - 1 };
		for i in 0..segments {
			let (a, b) = (pts[i], pts[(i + 1) % n]);
			out.push(orient(segment_quad(a, b, r)));
			if pl.closed || i + 1 < segments {
				append_join(&mut out, a, b, pts[(i + 2) % n], r);
			}
		}
	}
	out
}

/// Drops consecutive equal points (and, for a closed line, a last point equal to the first).
fn dedupe(pts: &[Point], closed: bool) -> Vec<Point> {
	let mut out: Vec<Point> = Vec::with_capacity(pts.len());
	for p in pts {
		if out.last() != Some(p) {
			out.push(*p);
		}
	}
	if closed && out.len() > 1 && out[0] == out[out.len() - 1] {
		out.pop();
	}
	out
}

/// The polygon clockwise on screen (y down), reversed when its signed area says otherwise.
fn orient(mut poly: Vec<Point>) -> Vec<Point> {
	let mut area = 0.0_f64;
	for (i, p) in poly.iter().enumerate() {
		let q = poly[(i + 1) % poly.len()];
		area += mul(p.x, q.y) - mul(q.x, p.y);
	}
	if area < 0.0 {
		poly.reverse();
	}
	poly
}

fn append_join(out: &mut Vec<Vec<Point>>, a: Point, b: Point, c: Point, r: f64) {
	let (d0x, d0y) = (b.x - a.x, b.y - a.y);
	let (d1x, d1y) = (c.x - b.x, c.y - b.y);
	let (l0, l1) = (hypot(d0x, d0y), hypot(d1x, d1y));
	if l0 == 0.0 || l1 == 0.0 {
		return;
	}
	let cross = mul(d0x, d1y) - mul(d0y, d1x);
	let dot = mul(d0x, d1x) + mul(d0y, d1y);
	let angle = atan2(cross, dot);
	if angle.abs() < 1e-6 {
		return;
	}
	let side = if cross < 0.0 { 1.0 } else { -1.0 };
	let a0 = atan2(side * d0x, -side * d0y);
	let n = arc_steps(angle.abs(), r);
	let mut pie = Vec::with_capacity(n as usize + 2);
	pie.push(b);
	for i in 0..=n {
		let (s, co) = sincos(a0 + lerp(0.0, angle, i as f64 / n as f64));
		pie.push(pt(b.x + mul(r, co), b.y + mul(r, s)));
	}
	out.push(orient(pie));
}

fn circle_points(c: Point, r: f64) -> Vec<Point> {
	let n = arc_steps(2.0 * PI, r);
	(0..n)
		.map(|i| {
			let (s, co) = sincos(i as f64 * TWO_PI / n as f64);
			pt(c.x + mul(r, co), c.y + mul(r, s))
		})
		.collect()
}

/// The rectangle of half-width `r` around the segment `a`-`b`.
fn segment_quad(a: Point, b: Point, r: f64) -> Vec<Point> {
	let (dx, dy) = (b.x - a.x, b.y - a.y);
	let l = hypot(dx, dy);
	let (nx, ny) = (mul(-dy / l, r), mul(dx / l, r));
	vec![
		pt(a.x - nx, a.y - ny),
		pt(b.x - nx, b.y - ny),
		pt(b.x + nx, b.y + ny),
		pt(a.x + nx, a.y + ny),
	]
}

/// Adds the polygon, moved by `off`, to the rasterizer.
pub fn add_polygon(rz: &mut Rasterizer, poly: &[Point], off: Point) {
	if poly.len() < 3 {
		return;
	}
	rz.move_to((poly[0].x + off.x) as f32, (poly[0].y + off.y) as f32);
	for q in &poly[1..] {
		rz.line_to((q.x + off.x) as f32, (q.y + off.y) as f32);
	}
	rz.close_path();
}

#[cfg(test)]
mod tests {
	use super::*;

	#[test]
	fn the_exact_constant_is_three_halves_of_pi_within_one_ulp() {
		let approx = 3.0 * PI / 2.0;
		assert!((THREE_HALF_PI.to_bits() as i64 - approx.to_bits() as i64).abs() <= 1);
	}

	#[test]
	// The platform library is the reference the exactly rounded series is checked against.
	#[allow(clippy::disallowed_methods)]
	fn atan2_is_within_a_few_ulps_of_the_library() {
		for (y, x) in [
			(1.0, 1.0),
			(-2.0, 0.5),
			(0.3, -4.0),
			(-0.2, -0.1),
			(5.0, 0.0),
		] {
			assert!((atan2(y, x) - f64::atan2(y, x)).abs() < 1e-14, "{y} {x}");
		}
	}

	#[test]
	fn a_plain_rectangle_has_four_points_and_a_rounded_one_has_more() {
		assert_eq!(rounded_rect(0.0, 0.0, 10.0, 8.0, 0.0).pts.len(), 4);
		assert!(rounded_rect(0.0, 0.0, 40.0, 30.0, 8.0).pts.len() > 8);
	}
}
