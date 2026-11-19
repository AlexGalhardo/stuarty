//! The Rust rasterizer against its own vectors, byte for byte.
//!
//! `renderer/testdata/raster-vectors.bin` holds each case: a spec and the bitmap this rasterizer draws
//! for it. Rust is the reference (ADR-0012, decision of 2026-11-16): the file is written from this
//! crate with `VUKEM_WRITE_RASTER_VECTORS=1 cargo test -p stuarty-renderer-text --test raster_vectors`
//! (a reviewed change of output, rule `.agents/skills/renderer-parity`), and `engine/rasterize`
//! draws the same specs through the renderer module under `wazero` and must get the same bytes
//! (`TestRasterVectorsAreCurrent`). The specs are encoded by that Go test; this one keeps them as
//! they are and replaces only the bitmaps.

use stuarty_renderer_text::fonts::FontSet;
use stuarty_renderer_text::shape::{Shape, ShapeKind, render_shape};
use stuarty_renderer_text::text::{Align, Content, Style, TextSpec, render_text};
use stuarty_renderer_text::{Bitmap, Colour, Rect};

const VECTORS: &[u8] = include_bytes!("../../testdata/raster-vectors.bin");

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

fn put_u32(out: &mut Vec<u8>, v: usize) {
	out.extend_from_slice(&(v as u32).to_le_bytes());
}

fn put_f64(out: &mut Vec<u8>, v: f64) {
	out.extend_from_slice(&v.to_bits().to_le_bytes());
}

/// The pixels as runs of zero bytes and of other bytes (see `Reader::pixels`): the length of a run of
/// zeros, the length of the run that follows and those bytes, until the bitmap is written. A literal
/// run keeps a single zero that sits between two other bytes.
fn put_pixels(out: &mut Vec<u8>, pix: &[u8]) {
	let mut i = 0;
	while i < pix.len() {
		let zeros = pix[i..].iter().take_while(|&&b| b == 0).count();
		let mut lit = 0;
		while i + zeros + lit < pix.len()
			&& (pix[i + zeros + lit] != 0
				|| (i + zeros + lit + 1 < pix.len() && pix[i + zeros + lit + 1] != 0))
		{
			lit += 1;
		}
		put_u32(out, zeros);
		put_u32(out, lit);
		out.extend_from_slice(&pix[i + zeros..i + zeros + lit]);
		i += zeros + lit;
	}
}

/// The expected part of a case as the file holds it.
fn put_expected(out: &mut Vec<u8>, b: &Bitmap, text: bool) {
	put_u32(out, b.width);
	put_u32(out, b.height);
	for v in [b.natural_w, b.natural_h, b.center_dx, b.center_dy] {
		put_f64(out, v);
	}
	if text {
		put_u32(out, b.word_boxes.len());
		for w in &b.word_boxes {
			for v in [w.x, w.y, w.w, w.h] {
				put_f64(out, v);
			}
		}
	}
	put_pixels(out, &b.pix);
}

#[test]
fn every_vector_is_what_the_rasterizer_draws() {
	let fonts = FontSet::bundled();
	let mut r = Reader(VECTORS);
	assert_eq!(r.take(4), b"VRAS");
	assert_eq!(r.u32(), 2, "vector file version");
	let n = r.u32();
	assert!(n >= 50, "{n} vectors");
	let (mut shapes, mut texts) = (0, 0);
	let mut failures: Vec<String> = Vec::new();
	let mut written: Vec<u8> = VECTORS[..12].to_vec();
	for _ in 0..n {
		let name = r.str();
		let spec_start = VECTORS.len() - r.0.len();
		let kind = r.u32();
		let spec_bytes;
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
			let spec_end = VECTORS.len() - r.0.len();
			let want = expected(&mut r, false);
			spec_bytes = spec_start..spec_end;
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
			let spec_end = VECTORS.len() - r.0.len();
			let want = expected(&mut r, true);
			spec_bytes = spec_start..spec_end;
			texts += 1;
			let spec = TextSpec {
				content,
				style,
				pixel_scale,
			};
			(render_text(&spec, &fonts).unwrap(), want)
		};
		if let Some(d) = first_difference(&name, &got, &want) {
			failures.push(d);
		}
		put_u32(&mut written, name.len());
		written.extend_from_slice(name.as_bytes());
		written.extend_from_slice(&VECTORS[spec_bytes]);
		put_expected(&mut written, &got, kind != 0);
	}
	assert!(r.0.is_empty(), "trailing bytes");
	assert!(
		shapes >= 10 && texts >= 20,
		"{shapes} shapes, {texts} texts"
	);
	if std::env::var("VUKEM_WRITE_RASTER_VECTORS").is_ok_and(|v| v == "1") {
		let path = concat!(
			env!("CARGO_MANIFEST_DIR"),
			"/../testdata/raster-vectors.bin"
		);
		std::fs::write(path, &written).unwrap();
		return;
	}
	assert!(
		failures.is_empty(),
		"{} vectors differ from the file (regenerate it with VUKEM_WRITE_RASTER_VECTORS=1 and review 		 the change):
{}",
		failures.len(),
		failures.join("
")
	);
}
