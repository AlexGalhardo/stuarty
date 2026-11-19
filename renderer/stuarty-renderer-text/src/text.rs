//! Text and captions: the port of `engine/raster/render.go` and `draw.go` (SDD-004 section 5.1).

use std::fmt;

use crate::math::mul;
use skrifa::GlyphId;
use skrifa::font::glyph::outline::{
	DrawMemory, DrawOptions, OutlineGlyph, PathSink, UnscaledOutlineSink,
};
use skrifa::raw::tables::glyf::PointFlags;

use crate::canvas::{Canvas, Mask, Painter, box_blur3};
use crate::fonts::{Face, FontSet, LookupError};
use crate::geom::{Point, Polyline, append_cube, append_quad, pt, rounded_rect, stroke_shapes};
use crate::layout::{Layout, PlacedGlyph, layout_text};
use crate::shape::finish;
use crate::{Bitmap, Colour, InvalidSpec, Rect};

/// The alignment of the lines of a text block.
#[derive(Clone, Copy, Debug, PartialEq, Eq)]
pub enum Align {
	Left,
	Center,
	Right,
}

/// The resolved style of a text or caption (`raster.style`).
#[derive(Clone, Debug)]
pub struct Style {
	pub font_family: String,
	pub font_weight: u32,
	pub size: f64,
	pub color: Colour,
	pub stroke_color: Colour,
	pub stroke_width: f64,
	pub shadow_color: Colour,
	pub shadow_dx: f64,
	pub shadow_dy: f64,
	pub shadow_blur: f64,
	/// `None` when there is no box behind the text.
	pub background: Option<Colour>,
	pub background_padding: f64,
	pub background_radius: f64,
	pub align: Align,
	pub line_height: f64,
	/// In pixels: lines wrap at this width.
	pub max_width: f64,
	pub uppercase: bool,
	/// False when the style says "none": the active word is then drawn like the others.
	pub highlight: bool,
	pub highlight_color: Colour,
	pub highlight_background: Option<Colour>,
}

/// A text or a caption.
#[derive(Clone, Debug)]
pub enum Content {
	Text(String),
	Caption {
		words: Vec<String>,
		/// The indices of the words that start a new line.
		breaks: Vec<usize>,
		/// The highlighted word, or `None`.
		active_word: Option<usize>,
	},
}

/// A text or caption bitmap to draw (`plan.BitmapSpec` of kind `text` or `caption`).
#[derive(Clone, Debug)]
pub struct TextSpec {
	pub content: Content,
	pub style: Style,
	pub pixel_scale: f64,
}

/// Why a text cannot be drawn.
#[derive(Clone, Debug, PartialEq, Eq)]
pub enum RenderError {
	Invalid(InvalidSpec),
	Font(LookupError),
}

impl fmt::Display for RenderError {
	fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
		match self {
			Self::Invalid(e) => e.fmt(f),
			Self::Font(e) => e.fmt(f),
		}
	}
}

impl std::error::Error for RenderError {}

impl From<InvalidSpec> for RenderError {
	fn from(e: InvalidSpec) -> Self {
		Self::Invalid(e)
	}
}

impl From<LookupError> for RenderError {
	fn from(e: LookupError) -> Self {
		Self::Font(e)
	}
}

/// Refuses a spec the rasterizer cannot draw (`validate`).
fn validate(spec: &TextSpec) -> Result<(), InvalidSpec> {
	if !(spec.pixel_scale > 0.0 && spec.pixel_scale <= 8.0) {
		return Err(InvalidSpec(format!("pixel scale {}", spec.pixel_scale)));
	}
	let st = &spec.style;
	if !(st.size > 0.0 && st.line_height > 0.0 && st.max_width > 0.0) {
		return Err(InvalidSpec(
			"size, line height and max width must be positive".to_owned(),
		));
	}
	if let Content::Caption {
		words, active_word, ..
	} = &spec.content
	{
		if words.is_empty() {
			return Err(InvalidSpec("a caption has no words".to_owned()));
		}
		if let Some(a) = active_word
			&& *a >= words.len()
		{
			return Err(InvalidSpec(format!("active word {a} of {}", words.len())));
		}
	}
	Ok(())
}

/// The characters to shape and the character range of each word of a caption. Upper-casing maps one
/// character to one character, so ranges survive it.
fn text_runes(spec: &TextSpec) -> (Vec<char>, Vec<(usize, usize)>) {
	let mut runes: Vec<char> = Vec::new();
	let mut spans = Vec::new();
	match &spec.content {
		Content::Caption { words, breaks, .. } => {
			for (i, w) in words.iter().enumerate() {
				if i > 0 {
					runes.push(if breaks.contains(&i) { '\n' } else { ' ' });
				}
				let start = runes.len();
				runes.extend(w.chars());
				spans.push((start, runes.len()));
			}
		}
		Content::Text(t) => runes.extend(t.replace("\r\n", "\n").chars()),
	}
	if spec.style.uppercase {
		for r in &mut runes {
			*r = upper(*r);
		}
	}
	(runes, spans)
}

/// Go's `unicode.ToUpper`: the simple one-to-one mapping.
fn upper(c: char) -> char {
	let mut it = c.to_uppercase();
	match (it.next(), it.next()) {
		(Some(u), None) => u,
		_ => c,
	}
}

/// A rectangle given by its edges, in bitmap pixels.
#[derive(Clone, Copy)]
struct Extent {
	x0: f64,
	y0: f64,
	x1: f64,
	y1: f64,
}

impl Extent {
	const EMPTY: Self = Self {
		x0: f64::INFINITY,
		y0: f64::INFINITY,
		x1: f64::NEG_INFINITY,
		y1: f64::NEG_INFINITY,
	};

	fn union(self, o: Self) -> Self {
		Self {
			x0: self.x0.min(o.x0),
			y0: self.y0.min(o.y0),
			x1: self.x1.max(o.x1),
			y1: self.y1.max(o.y1),
		}
	}

	fn grow(self, by: f64) -> Self {
		Self {
			x0: self.x0 - by,
			y0: self.y0 - by,
			x1: self.x1 + by,
			y1: self.y1 + by,
		}
	}

	fn shift(self, dx: f64, dy: f64) -> Self {
		Self {
			x0: self.x0 + dx,
			y0: self.y0 + dy,
			x1: self.x1 + dx,
			y1: self.y1 + dy,
		}
	}
}

fn lines_extent(lines: &[Polyline]) -> Extent {
	let mut e = Extent::EMPTY;
	for l in lines {
		for p in &l.pts {
			e = e.union(Extent {
				x0: p.x,
				y0: p.y,
				x1: p.x,
				y1: p.y,
			});
		}
	}
	e
}

/// A glyph outline segment in font units, y up.
#[derive(Clone, Copy)]
enum Seg {
	Move(f64, f64),
	Line(f64, f64),
	Quad([f64; 4]),
	Cube([f64; 6]),
}

/// The points of a `glyf` contour into segments, the way go-text's `buildSegments` does it: the
/// contour starts at its first on-curve point, or at the midpoint of its first two off-curve points.
#[derive(Default)]
struct GlyfSegments {
	out: Vec<Seg>,
	contour: Vec<(f64, f64, bool)>,
}

impl UnscaledOutlineSink for GlyfSegments {
	fn start_contour(&mut self) {
		self.contour.clear();
	}

	fn point(&mut self, x: i32, y: i32, flags: PointFlags) {
		self.contour
			.push((f64::from(x), f64::from(y), flags.is_on_curve()));
	}

	fn end_contour(&mut self) {
		let pts = std::mem::take(&mut self.contour);
		build_segments(&pts, &mut self.out);
	}
}

fn mid(p: (f64, f64), q: (f64, f64)) -> (f64, f64) {
	((p.0 + q.0) / 2.0, (p.1 + q.1) / 2.0)
}

fn build_segments(points: &[(f64, f64, bool)], out: &mut Vec<Seg>) {
	let mut first_on: Option<(f64, f64)> = None;
	let mut first_off: Option<(f64, f64)> = None;
	let mut last_off: Option<(f64, f64)> = None;
	let n = points.len();
	for (i, &(x, y, on)) in points.iter().enumerate() {
		let p = (x, y);
		let is_end = i + 1 == n;
		if first_on.is_none() {
			if on {
				first_on = Some(p);
				out.push(Seg::Move(p.0, p.1));
			} else if first_off.is_none() {
				first_off = Some(p);
				if !is_end {
					continue;
				}
			} else {
				let m = mid(first_off.unwrap_or(p), p);
				first_on = Some(m);
				last_off = Some(p);
				out.push(Seg::Move(m.0, m.1));
			}
		} else if last_off.is_none() {
			if !on {
				last_off = Some(p);
				if !is_end {
					continue;
				}
			} else {
				out.push(Seg::Line(p.0, p.1));
			}
		} else if let Some(lo) = last_off {
			if !on {
				let m = mid(lo, p);
				out.push(Seg::Quad([lo.0, lo.1, m.0, m.1]));
				last_off = Some(p);
			} else {
				out.push(Seg::Quad([lo.0, lo.1, p.0, p.1]));
				last_off = None;
			}
		}

		if is_end {
			let fon = first_on.unwrap_or(p);
			match (first_off, last_off) {
				(None, None) => out.push(Seg::Line(fon.0, fon.1)),
				(None, Some(lo)) => out.push(Seg::Quad([lo.0, lo.1, fon.0, fon.1])),
				(Some(fo), None) => out.push(Seg::Quad([fo.0, fo.1, fon.0, fon.1])),
				(Some(fo), Some(lo)) => {
					let m = mid(lo, fo);
					out.push(Seg::Quad([lo.0, lo.1, m.0, m.1]));
					out.push(Seg::Quad([fo.0, fo.1, fon.0, fon.1]));
				}
			}
			first_on = None;
			first_off = None;
			last_off = None;
		}
	}
}

/// A charstring outline: a path in font units, as the segments go-text reads them.
#[derive(Default)]
struct PathSegments {
	out: Vec<Seg>,
}

impl PathSink for PathSegments {
	fn move_to(&mut self, x: f32, y: f32) {
		self.out.push(Seg::Move(f64::from(x), f64::from(y)));
	}
	fn line_to(&mut self, x: f32, y: f32) {
		self.out.push(Seg::Line(f64::from(x), f64::from(y)));
	}
	fn quad_to(&mut self, cx0: f32, cy0: f32, x: f32, y: f32) {
		self.out.push(Seg::Quad([
			f64::from(cx0),
			f64::from(cy0),
			f64::from(x),
			f64::from(y),
		]));
	}
	fn curve_to(&mut self, cx0: f32, cy0: f32, cx1: f32, cy1: f32, x: f32, y: f32) {
		self.out.push(Seg::Cube([
			f64::from(cx0),
			f64::from(cy0),
			f64::from(cx1),
			f64::from(cy1),
			f64::from(x),
			f64::from(y),
		]));
	}
	fn close(&mut self) {}
}

fn glyph_segments(face: &Face, gid: u32) -> Option<Vec<Seg>> {
	if gid > u32::from(u16::MAX) {
		return None;
	}
	let outlines = face.font.outline_glyphs();
	let glyph = outlines.get(GlyphId::new(gid))?;
	match &glyph {
		OutlineGlyph::Glyf(_) => {
			let mut sink = GlyfSegments::default();
			glyph.draw_unscaled(DrawMemory::Scratch, &mut sink).ok()?;
			Some(sink.out)
		}
		_ => {
			let mut sink = PathSegments::default();
			glyph
				.draw(&DrawOptions::new(), DrawMemory::Scratch, &mut sink)
				.ok()?;
			Some(sink.out)
		}
	}
}

/// The closed polylines of the glyph outlines, in block pixels.
fn outlines(face: &Face, glyphs: &[PlacedGlyph], k: f64) -> Vec<Polyline> {
	let mut out = Vec::new();
	for g in glyphs {
		if let Some(segs) = glyph_segments(face, g.gid) {
			out.extend(glyph_polylines(&segs, g.x, g.y, k));
		}
	}
	out
}

/// Converts an outline in font units (y up) into polylines at the glyph origin `(ox, oy)` in pixels
/// (y down). Every outline point is rounded to a whole font unit, as the points of a static font
/// are (see `glyphPolylines` in `engine/raster`).
fn glyph_polylines(segs: &[Seg], ox: f64, oy: f64, k: f64) -> Vec<Polyline> {
	let mut out = Vec::new();
	let mut cur: Vec<Point> = Vec::new();
	let at = |x: f64, y: f64| -> Point {
		let (x, y) = (x.round(), y.round());
		pt(ox + mul(x, k), oy - mul(y, k))
	};
	let mut flush = |cur: &mut Vec<Point>| {
		if cur.len() > 1 {
			out.push(Polyline {
				pts: std::mem::take(cur),
				closed: true,
			});
		}
		cur.clear();
	};
	for s in segs {
		match *s {
			Seg::Move(x, y) => {
				flush(&mut cur);
				cur.push(at(x, y));
			}
			Seg::Line(x, y) => cur.push(at(x, y)),
			Seg::Quad([a, b, c, d]) => {
				let p0 = *cur.last().unwrap_or(&Point::default());
				append_quad(&mut cur, p0, at(a, b), at(c, d));
			}
			Seg::Cube([a, b, c, d, e, f]) => {
				let p0 = *cur.last().unwrap_or(&Point::default());
				append_cube(&mut cur, p0, at(a, b), at(c, d), at(e, f));
			}
		}
	}
	flush(&mut cur);
	out
}

fn polygons(lines: &[Polyline]) -> Vec<&[Point]> {
	lines.iter().map(|l| l.pts.as_slice()).collect()
}

/// The glyphs inside the character span and the others.
fn split(glyphs: &[PlacedGlyph], span: (usize, usize)) -> (Vec<PlacedGlyph>, Vec<PlacedGlyph>) {
	glyphs
		.iter()
		.copied()
		.partition(|g| g.text >= span.0 && g.text < span.1)
}

/// A text or caption bitmap before any pixel is drawn: the laid out glyphs and the geometry of the
/// bitmap. `measure` and `render` share it, so the size the plan places a bitmap by is the size the
/// render produces.
struct TextJob {
	st: Style,
	s: f64,
	size: f64,
	words: Vec<Rect>,
	base_lines: Vec<Polyline>,
	hi_lines: Vec<Polyline>,
	all_lines: Vec<Polyline>,
	block: Extent,
	boxx: Extent,
	hi_box: Extent,
	has_hi: bool,
	shadow: bool,
	blur: usize,
	ox: f64,
	oy: f64,
	w: usize,
	h: usize,
}

impl TextJob {
	fn new(
		st: &Style,
		s: f64,
		face: &Face,
		lay: &Layout,
		spans: &[(usize, usize)],
		active: Option<usize>,
	) -> Self {
		let size = mul(st.size, s);
		let k = lay.scale;
		let (hi_glyphs, base_glyphs) = match active {
			Some(a) => split(&lay.glyphs, spans[a]),
			None => (Vec::new(), lay.glyphs.clone()),
		};
		let base_lines = outlines(face, &base_glyphs, k);
		let hi_lines = outlines(face, &hi_glyphs, k);
		let mut all_lines = base_lines.clone();
		all_lines.extend(hi_lines.iter().cloned());

		let half = mul(st.stroke_width, s) / 2.0;
		let block = Extent {
			x0: 0.0,
			y0: 0.0,
			x1: lay.width,
			y1: lay.height,
		};
		let ink = lines_extent(&all_lines).grow(half);
		let mut content = block.grow(half).union(ink);
		let mut boxx = content;

		if st.background.is_some() {
			boxx = block.grow(mul(st.background_padding, s));
			content = content.union(boxx);
		}

		let mut hi_box = Extent::EMPTY;
		let mut has_hi = false;
		if let Some(a) = active
			&& st.highlight_background.is_some()
		{
			let w = lay.words[a];
			let pad = mul(0.1, size);
			hi_box = Extent {
				x0: w.x - pad,
				y0: w.y,
				x1: w.x + w.w + pad,
				y1: w.y + w.h,
			};
			has_hi = true;
			content = content.union(hi_box);
		}

		let shadow = st.shadow_color.a > 0
			&& (st.shadow_dx != 0.0 || st.shadow_dy != 0.0 || st.shadow_blur != 0.0);
		let blur = mul(st.shadow_blur, s).ceil().max(0.0) as usize;
		if shadow {
			content = content.union(
				ink.shift(mul(st.shadow_dx, s), mul(st.shadow_dy, s))
					.grow((3 * blur) as f64),
			);
		}

		// One clear pixel all around: the antialiased edge of the ink may touch the last pixel the
		// bounding box reaches, and the bitmap must never clip it.
		content = content.grow(1.0);

		// The plan places the centre of the text block and the browser draws a bitmap by its own
		// centre, so the bitmap reaches as far on each side of the block's centre as it does on the
		// farther one.
		let (cx, cy) = ((block.x0 + block.x1) / 2.0, (block.y0 + block.y1) / 2.0);
		let (ox, w) = symmetric_span(cx, content.x0, content.x1);
		let (oy, h) = symmetric_span(cy, content.y0, content.y1);

		Self {
			st: st.clone(),
			s,
			size,
			words: lay.words.clone(),
			base_lines,
			hi_lines,
			all_lines,
			block,
			boxx,
			hi_box,
			has_hi,
			shadow,
			blur,
			ox,
			oy,
			w,
			h,
		}
	}

	/// The size of the bitmap at pixel scale 1.
	fn natural_size(&self) -> (f64, f64) {
		(self.w as f64 / self.s, self.h as f64 / self.s)
	}

	fn draw(&self) -> Result<Bitmap, InvalidSpec> {
		let (st, s) = (&self.st, self.s);
		let mut cv = Canvas::new(self.w, self.h)?;
		let mut painter = Painter::new(self.w, self.h);
		let origin = pt(-self.ox, -self.oy);

		if let Some(bg) = st.background {
			let b = self.boxx;
			let r = rounded_rect(
				b.x0,
				b.y0,
				b.x1 - b.x0,
				b.y1 - b.y0,
				mul(st.background_radius, s),
			);
			cv.over(painter.fill(origin, &[&r.pts]), bg);
		}

		if self.has_hi
			&& let Some(hb) = st.highlight_background
		{
			let radius = mul(st.background_radius, s).max(mul(0.15, self.size));
			let b = self.hi_box;
			let r = rounded_rect(b.x0, b.y0, b.x1 - b.x0, b.y1 - b.y0, radius);
			cv.over(painter.fill(origin, &[&r.pts]), hb);
		}

		let strokes = stroke_shapes(&self.all_lines, mul(st.stroke_width, s));

		if self.shadow {
			let off = pt(
				origin.x + mul(st.shadow_dx, s),
				origin.y + mul(st.shadow_dy, s),
			);
			let mut polys = polygons(&self.all_lines);
			polys.extend(strokes.iter().map(Vec::as_slice));
			let m: &mut Mask = painter.fill(off, &polys);
			box_blur3(m, self.blur);
			cv.over(m, st.shadow_color);
		}

		if !strokes.is_empty() {
			let polys: Vec<&[Point]> = strokes.iter().map(Vec::as_slice).collect();
			cv.over(painter.fill(origin, &polys), st.stroke_color);
		}

		cv.over(painter.fill(origin, &polygons(&self.base_lines)), st.color);

		if !self.hi_lines.is_empty() {
			cv.over(
				painter.fill(origin, &polygons(&self.hi_lines)),
				st.highlight_color,
			);
		}

		let blk = self.block.shift(-self.ox, -self.oy);
		let mut bm = finish(cv, s, (blk.x0, blk.y0, blk.x1, blk.y1));
		bm.word_boxes = self
			.words
			.iter()
			.map(|w| Rect {
				x: (w.x - self.ox) / s,
				y: (w.y - self.oy) / s,
				w: w.w / s,
				h: w.h / s,
			})
			.collect();
		Ok(bm)
	}
}

/// The whole-pixel origin and length of a canvas that holds `[lo, hi]` and whose centre is within
/// half a pixel of `c` (`lo <= c <= hi`). The tight canvas is kept when it is already that close, so a
/// bitmap without a lopsided shadow keeps its size.
fn symmetric_span(c: f64, lo: f64, hi: f64) -> (f64, usize) {
	let origin = lo.floor();
	let length = (hi - origin).ceil() as usize;
	if (origin + length as f64 / 2.0 - c).abs() <= 0.5 {
		return (origin, length);
	}
	let origin = (c - (c - lo).max(hi - c)).floor();
	(origin, (2.0 * (c - origin)).ceil() as usize)
}

/// Resolves the style, lays the text out and works out the geometry.
fn prepare(spec: &TextSpec, fonts: &FontSet) -> Result<TextJob, RenderError> {
	let st = &spec.style;
	let face = fonts.lookup(&st.font_family, st.font_weight)?;
	let (runes, spans) = text_runes(spec);
	let lay = layout_text(&runes, &spans, &face, st, spec.pixel_scale);
	let active = match &spec.content {
		Content::Caption { active_word, .. } if st.highlight && st.highlight_color.a > 0 => {
			*active_word
		}
		_ => None,
	};
	Ok(TextJob::new(
		st,
		spec.pixel_scale,
		&face,
		&lay,
		&spans,
		active,
	))
}

/// Draws the bitmap of a text or caption. It is deterministic: the same spec gives the same bytes on
/// every platform.
pub fn render_text(spec: &TextSpec, fonts: &FontSet) -> Result<Bitmap, RenderError> {
	validate(spec)?;
	Ok(prepare(spec, fonts)?.draw()?)
}

/// The natural size of a text or caption bitmap, in frame pixels at pixel scale 1: exactly the
/// `natural_w` and `natural_h` [`render_text`] produces for the same spec, without drawing it
/// (SDD-004 section 4.2). A spec the rasterizer cannot draw measures as 0 by 0.
pub fn measure_text(spec: &TextSpec, fonts: &FontSet) -> (f64, f64) {
	if validate(spec).is_err() {
		return (0.0, 0.0);
	}
	prepare(spec, fonts).map_or((0.0, 0.0), |j| j.natural_size())
}
