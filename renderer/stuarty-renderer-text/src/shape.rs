//! Shapes: the port of `renderShape` and `shapeGeometry` of `engine/raster/draw.go`.

use crate::math::mul;

use crate::canvas::{Canvas, Painter};
use crate::geom::{Point, Polyline, ellipse, rounded_rect, stroke_shapes};
use crate::{Bitmap, Colour, InvalidSpec};

/// The kind of a shape (`plan.ShapeSpec.Shape`, SDD-001 section 3.6).
#[derive(Clone, Copy, Debug, PartialEq, Eq)]
pub enum ShapeKind {
	Rect,
	RoundedRect,
	Ellipse,
}

/// A shape with the colours parsed (`raster.shape`).
#[derive(Clone, Copy, Debug)]
pub struct Shape {
	pub kind: ShapeKind,
	pub width: f64,
	pub height: f64,
	pub radius: f64,
	pub fill: Colour,
	pub stroke_color: Colour,
	pub stroke_width: f64,
}

/// The canvas of a shape and the box of the shape in it, in pixels.
#[derive(Clone, Copy, Debug)]
pub struct ShapeBox {
	pub cw: usize,
	pub ch: usize,
	pub x: f64,
	pub y: f64,
	pub w: f64,
	pub h: f64,
}

/// Sizes the canvas of a shape: the shape plus half its stroke on every side, with the shape centred.
pub fn shape_geometry(sh: &Shape, s: f64) -> ShapeBox {
	let (w, h) = (mul(sh.width, s), mul(sh.height, s));
	let half = mul(sh.stroke_width, s) / 2.0;
	let (cw, ch) = (
		(w + 2.0 * half).ceil() as usize,
		(h + 2.0 * half).ceil() as usize,
	);
	ShapeBox {
		cw,
		ch,
		x: (cw as f64 - w) / 2.0,
		y: (ch as f64 - h) / 2.0,
		w,
		h,
	}
}

/// Draws a shape at the pixel scale `s`. The caller refuses a pixel scale outside `(0, 8]` and a
/// non-positive size, as `validate` does in Go.
pub fn render_shape(sh: &Shape, s: f64) -> Result<Bitmap, InvalidSpec> {
	let g = shape_geometry(sh, s);
	let mut cv = Canvas::new(g.cw, g.ch)?;
	let outline: Polyline = match sh.kind {
		ShapeKind::Rect => rounded_rect(g.x, g.y, g.w, g.h, 0.0),
		ShapeKind::RoundedRect => rounded_rect(g.x, g.y, g.w, g.h, mul(sh.radius, s)),
		ShapeKind::Ellipse => ellipse(g.x, g.y, g.w, g.h),
	};
	let mut pt = Painter::new(g.cw, g.ch);
	let origin = Point::default();
	cv.over(pt.fill(origin, &[&outline.pts]), sh.fill);
	let strokes = stroke_shapes(std::slice::from_ref(&outline), mul(sh.stroke_width, s));
	if !strokes.is_empty() {
		let polys: Vec<&[Point]> = strokes.iter().map(Vec::as_slice).collect();
		cv.over(pt.fill(origin, &polys), sh.stroke_color);
	}
	Ok(finish(cv, s, (g.x, g.y, g.x + g.w, g.y + g.h)))
}

/// Builds the bitmap with its metrics at pixel scale 1. `block` is `(x0, y0, x1, y1)` in canvas
/// pixels.
pub(crate) fn finish(cv: Canvas, s: f64, block: (f64, f64, f64, f64)) -> Bitmap {
	Bitmap {
		natural_w: cv.w as f64 / s,
		natural_h: cv.h as f64 / s,
		center_dx: ((block.0 + block.2) / 2.0 - cv.w as f64 / 2.0) / s,
		center_dy: ((block.1 + block.3) / 2.0 - cv.h as f64 / 2.0) / s,
		width: cv.w,
		height: cv.h,
		pix: cv.pix,
		word_boxes: Vec::new(),
	}
}
