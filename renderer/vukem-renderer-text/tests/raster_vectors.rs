//! The Rust rasterizer against `engine/raster`, byte for byte.
//!
//! `renderer/testdata/raster-vectors.bin` is written by `engine/raster/vectors_test.go`: each case
//! holds a spec and the bitmap the Go rasterizer draws for it. A single differing byte is a failure
//! (rule `.agents/skills/renderer-parity`).

use vukem_renderer_text::fonts::FontSet;
use vukem_renderer_text::shape::{Shape, ShapeKind, render_shape};
use vukem_renderer_text::text::{Align, Content, Style, TextSpec, render_text};
use vukem_renderer_text::{Bitmap, Colour, Rect};

const VECTORS: &[u8] = include_bytes!("../../testdata/raster-vectors.bin");

/// The vectors the port does not draw byte for byte like `engine/raster`, and why (TASK-0220 waits
/// for the author's decision on them). The test requires exactly these to differ: a vector that
/// starts to match must leave the list, one that starts to differ must not be added without a review.
///
/// Every one is a text in a variable font (Inter, Montserrat, Playfair Display, JetBrains Mono) or
/// reaches a kerning pair whose GPOS value go-text rounds differently:
///
/// - go-text 0.3.5 does not apply the kerning of most pairs of the variable fonts (`Wo`, `VA`, `To`
///   in Inter), which `harfrust` (HarfBuzz 15.1) applies, so the lines are a few pixels narrower and
///   the sizes of the bitmaps differ;
/// - go-text scales a GPOS value with a truncating integer division (`emScaleX`) where HarfBuzz and
///   `harfrust` round, so a kerned advance can differ by 1/64 pixel (`anton-text-ligature-and-kerning`);
/// - go-text computes the points of a variable glyph in `float32` (HarfBuzz `gvar` code) and rounds
///   the end result to a font unit, `skrifa` in 16.16 fixed point: a point can land on the other side
///   of a rounding boundary, one font unit away (`jetbrains-mono-code`, `text-stroke-only-thick`).
const KNOWN_DIVERGENT: &[&str] = &[
	"inter-plain",
	"inter-stroke-shadow-box",
	"montserrat-uppercase",
	"playfair-accents",
	"jetbrains-mono-code",
	"inter-cyrillic-greek",
	"inter-wrapped-paragraph",
	"caption-plain",
	"caption-active-word",
	"emoji-sequence-and-notdef",
	"right-to-left-mixed",
	"missing-glyph-no-fallback",
	"caption-active-first",
	"caption-active-last-with-box",
	"text-empty-lines",
	"text-trailing-space",
	"text-unbreakable-word-over-width",
	"text-align-left-multiline",
	"text-align-center-multiline",
	"text-fractional-size-scale-1.5",
	"text-shadow-blur-offset",
	"text-box-padding",
	"text-stroke-only-thick",
	"text-uppercase-accents",
	"text-ligature-and-kerning",
	"caption-break-and-highlight-colour",
	"caption-highlight-none",
	"caption-forced-break",
	"caption-wrapped-narrow",
	"anton-text-ligature-and-kerning",
];

struct Reader<'a>(&'a [u8]);

impl<'a> Reader<'a> {
	fn take(&mut self, n: usize) -> &'a [u8] {
		let (head, tail) = self.0.split_at(n);
		self.0 = tail;
		head
	}
	fn u32(&mut self) -> usize {
		u32::from_le_bytes(self.take(4).try_into().unwrap()) as usize
	}
	fn f64(&mut self) -> f64 {
		f64::from_bits(u64::from_le_bytes(self.take(8).try_into().unwrap()))
	}
	fn str(&mut self) -> String {
		let n = self.u32();
		String::from_utf8(self.take(n).to_vec()).unwrap()
	}
	fn flag(&mut self) -> bool {
		self.take(1)[0] != 0
	}
	fn colour(&mut self) -> Colour {
		let c = self.take(4);
		Colour {
			r: c[0],
			g: c[1],
			b: c[2],
			a: c[3],
		}
	}
	fn opt_colour(&mut self) -> Option<Colour> {
		self.flag().then(|| self.colour())
	}
	/// The pixels as runs of zero bytes and of other bytes (see `vectorWriter.pixels`).
	fn pixels(&mut self, len: usize) -> Vec<u8> {
		let mut out = Vec::with_capacity(len);
		while out.len() < len {
			let zeros = self.u32();
			let lit = self.u32();
			out.resize(out.len() + zeros, 0);
			out.extend_from_slice(self.take(lit));
		}
		assert_eq!(out.len(), len, "pixel runs");
		out
	}
}

struct Expected {
	width: usize,
	height: usize,
	metrics: (f64, f64, f64, f64),
	word_boxes: Vec<Rect>,
	pix: Vec<u8>,
}

fn expected(r: &mut Reader<'_>, text: bool) -> Expected {
	let (width, height) = (r.u32(), r.u32());
	let metrics = (r.f64(), r.f64(), r.f64(), r.f64());
	let mut word_boxes = Vec::new();
	if text {
		for _ in 0..r.u32() {
			word_boxes.push(Rect {
				x: r.f64(),
				y: r.f64(),
				w: r.f64(),
				h: r.f64(),
			});
		}
	}
	let pix = r.pixels(width * height * 4);
	Expected {
		width,
		height,
		metrics,
		word_boxes,
		pix,
	}
}

fn first_difference(name: &str, got: &Bitmap, want: &Expected) -> Option<String> {
	if (got.width, got.height) != (want.width, want.height) {
		return Some(format!(
			"{name}: size {}x{}, Go {}x{}",
			got.width, got.height, want.width, want.height
		));
	}
	let m = (got.natural_w, got.natural_h, got.center_dx, got.center_dy);
	if m != want.metrics {
		return Some(format!("{name}: metrics {m:?}, Go {:?}", want.metrics));
	}
	if got.word_boxes != want.word_boxes {
		return Some(format!(
			"{name}: word boxes {:?}, Go {:?}",
			got.word_boxes, want.word_boxes
		));
	}
	let diff: Vec<usize> = got
		.pix
		.iter()
		.zip(&want.pix)
		.enumerate()
		.filter_map(|(i, (a, b))| (a != b).then_some(i))
		.collect();
	let max = diff
		.iter()
		.map(|&i| got.pix[i].abs_diff(want.pix[i]))
		.max()
		.unwrap_or(0);
	diff.first().map(|&i| {
		format!(
			"{name}: {} bytes differ (at most by {max}), the first at pixel ({}, {}) channel {}: {} but Go draws {}",
			diff.len(),
			(i / 4) % got.width,
			(i / 4) / got.width,
			i % 4,
			got.pix[i],
			want.pix[i]
		)
	})
}

fn text_style(r: &mut Reader<'_>) -> Style {
	Style {
		font_family: r.str(),
		font_weight: r.u32() as u32,
		size: r.f64(),
		color: r.colour(),
		stroke_color: r.colour(),
		stroke_width: r.f64(),
		shadow_color: r.colour(),
		shadow_dx: r.f64(),
		shadow_dy: r.f64(),
		shadow_blur: r.f64(),
		background: r.opt_colour(),
		background_padding: r.f64(),
		background_radius: r.f64(),
		align: match r.str().as_str() {
			"left" => Align::Left,
			"right" => Align::Right,
			_ => Align::Center,
		},
		line_height: r.f64(),
		max_width: r.f64(),
		uppercase: r.flag(),
		highlight: r.flag(),
		highlight_color: r.colour(),
		highlight_background: r.opt_colour(),
	}
}

#[test]
fn every_vector_is_the_bytes_of_the_go_rasterizer() {
	let fonts = FontSet::bundled();
	let mut r = Reader(VECTORS);
	assert_eq!(r.take(4), b"VRAS");
	assert_eq!(r.u32(), 2, "vector file version");
	let n = r.u32();
	assert!(n >= 50, "{n} vectors");
	let (mut shapes, mut texts) = (0, 0);
	let mut failures: Vec<(String, String)> = Vec::new();
	for _ in 0..n {
		let name = r.str();
		let kind = r.u32();
		let (got, want) = if kind == 0 {
			let shape_kind = match r.u32() {
				0 => ShapeKind::Rect,
				1 => ShapeKind::RoundedRect,
				_ => ShapeKind::Ellipse,
			};
			let (width, height, radius, stroke_width) = (r.f64(), r.f64(), r.f64(), r.f64());
			let (fill, stroke_color) = (r.colour(), r.colour());
			let scale = r.f64();
			let sh = Shape {
				kind: shape_kind,
				width,
				height,
				radius,
				fill,
				stroke_color,
				stroke_width,
			};
			let want = expected(&mut r, false);
			shapes += 1;
			(render_shape(&sh, scale).unwrap(), want)
		} else {
			let style = text_style(&mut r);
			let content = if kind == 2 {
				let words = (0..r.u32()).map(|_| r.str()).collect();
				let breaks = (0..r.u32()).map(|_| r.u32()).collect();
				let active = r.u32();
				Content::Caption {
					words,
					breaks,
					active_word: active.checked_sub(1),
				}
			} else {
				Content::Text(r.str())
			};
			let pixel_scale = r.f64();
			let want = expected(&mut r, true);
			texts += 1;
			let spec = TextSpec {
				content,
				style,
				pixel_scale,
			};
			(render_text(&spec, &fonts).unwrap(), want)
		};
		if let Some(d) = first_difference(&name, &got, &want) {
			failures.push((name, d));
		}
	}
	assert!(r.0.is_empty(), "trailing bytes");
	assert!(
		shapes >= 10 && texts >= 20,
		"{shapes} shapes, {texts} texts"
	);
	let differing: Vec<&str> = failures.iter().map(|(n, _)| n.as_str()).collect();
	let unexpected: Vec<&String> = failures
		.iter()
		.filter(|(n, _)| !KNOWN_DIVERGENT.contains(&n.as_str()))
		.map(|(_, d)| d)
		.collect();
	assert!(
		unexpected.is_empty(),
		"{} vectors differ from Go and are not known to:
{}",
		unexpected.len(),
		unexpected
			.iter()
			.map(|d| d.as_str())
			.collect::<Vec<_>>()
			.join(
				"
"
			)
	);
	let fixed: Vec<&&str> = KNOWN_DIVERGENT
		.iter()
		.filter(|n| !differing.contains(n))
		.collect();
	assert!(
		fixed.is_empty(),
		"these vectors now match Go: remove them from KNOWN_DIVERGENT: {fixed:?}"
	);
}
