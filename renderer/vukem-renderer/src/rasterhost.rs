//! The text and shape rasterizer as seen by a host (SDD-013 section 4.2): a request in, a bitmap
//! and its metrics out, and the bytes of the user fonts a request names, stored by their digest.
//! The C-ABI of the module (`ffi`) and the native tests are thin wrappers over [`TextHost`].
//!
//! The module cannot ask its host for a file, so a user font a request needs and the module does
//! not hold is reported as `font_needed` with the digest: the host puts the bytes
//! ([`TextHost::put_font`]) and repeats the request, or reports the font as not stored itself.

use std::collections::HashMap;
use std::sync::{Arc, Mutex, PoisonError};

use vukem_renderer_text::fonts::{
	DocFontFile, FontError, FontSet, FontSource, LookupError, SourceError,
};
use vukem_renderer_text::shape::{Shape, ShapeKind, render_shape, shape_geometry};
use vukem_renderer_text::text::{
	Align, Content, RenderError, Style, TextSpec, measure_text, render_text,
};
use vukem_renderer_text::validate::{MAX_FONT_BYTES, check_font};
use vukem_renderer_text::{Bitmap, Colour, MAX_PIXELS};

use crate::error::{Error, Reason};

/// The budget of the font files held between requests.
pub const MAX_FONT_STORE_BYTES: usize = 64 << 20;
/// The most families, files, words or breaks a request may carry.
const MAX_ITEMS: usize = 1 << 16;

/// What a request asks for.
#[derive(Clone, Copy, Debug, PartialEq, Eq)]
enum Mode {
	Render,
	Measure,
}

#[derive(Default)]
struct FontStore {
	/// By lower-case hex digest.
	files: HashMap<String, Vec<u8>>,
	bytes: usize,
	needed: Option<String>,
}

/// The state of the rasterizer between calls: the font set (and the fonts it parsed), the font files
/// the host put, and the result of the last request.
pub struct TextHost {
	fonts: FontSet,
	store: Arc<Mutex<FontStore>>,
	result: Vec<u8>,
}

impl Default for TextHost {
	fn default() -> Self {
		Self::new()
	}
}

struct Reader<'a>(&'a [u8]);

impl<'a> Reader<'a> {
	fn take(&mut self, n: usize) -> Result<&'a [u8], Error> {
		if n > self.0.len() {
			return Err(Error::new(
				Reason::InvalidArgument,
				"the request is truncated",
			));
		}
		let (head, tail) = self.0.split_at(n);
		self.0 = tail;
		Ok(head)
	}
	fn u8(&mut self) -> Result<u8, Error> {
		Ok(self.take(1)?[0])
	}
	fn u32(&mut self) -> Result<u32, Error> {
		let b = self.take(4)?;
		Ok(u32::from_le_bytes([b[0], b[1], b[2], b[3]]))
	}
	fn count(&mut self) -> Result<usize, Error> {
		let n = self.u32()? as usize;
		if n > MAX_ITEMS {
			return Err(Error::new(Reason::LimitExceeded, "request_items"));
		}
		Ok(n)
	}
	fn f64(&mut self) -> Result<f64, Error> {
		let b = self.take(8)?;
		let mut a = [0_u8; 8];
		a.copy_from_slice(b);
		Ok(f64::from_bits(u64::from_le_bytes(a)))
	}
	fn str(&mut self) -> Result<String, Error> {
		let n = self.u32()? as usize;
		let b = self.take(n)?;
		String::from_utf8(b.to_vec())
			.map_err(|_| Error::new(Reason::InvalidArgument, "a string is not UTF-8"))
	}
	fn flag(&mut self) -> Result<bool, Error> {
		Ok(self.u8()? != 0)
	}
	fn colour(&mut self) -> Result<Colour, Error> {
		let c = self.take(4)?;
		Ok(Colour {
			r: c[0],
			g: c[1],
			b: c[2],
			a: c[3],
		})
	}
	fn opt_colour(&mut self) -> Result<Option<Colour>, Error> {
		if self.flag()? {
			Ok(Some(self.colour()?))
		} else {
			Ok(None)
		}
	}
}

fn style(r: &mut Reader<'_>) -> Result<Style, Error> {
	Ok(Style {
		font_family: r.str()?,
		font_weight: r.u32()?,
		size: r.f64()?,
		color: r.colour()?,
		stroke_color: r.colour()?,
		stroke_width: r.f64()?,
		shadow_color: r.colour()?,
		shadow_dx: r.f64()?,
		shadow_dy: r.f64()?,
		shadow_blur: r.f64()?,
		background: r.opt_colour()?,
		background_padding: r.f64()?,
		background_radius: r.f64()?,
		align: match r.u8()? {
			0 => Align::Left,
			2 => Align::Right,
			_ => Align::Center,
		},
		line_height: r.f64()?,
		max_width: r.f64()?,
		uppercase: r.flag()?,
		highlight: r.flag()?,
		highlight_color: r.colour()?,
		highlight_background: r.opt_colour()?,
	})
}

fn put_u32(out: &mut Vec<u8>, v: usize) {
	out.extend_from_slice(&u32::try_from(v).unwrap_or(u32::MAX).to_le_bytes());
}

fn put_f64(out: &mut Vec<u8>, v: f64) {
	out.extend_from_slice(&v.to_bits().to_le_bytes());
}

fn put_str(out: &mut Vec<u8>, s: &str) {
	put_u32(out, s.len());
	out.extend_from_slice(s.as_bytes());
}

/// The result of a successful request: width, height, the four metrics, the word boxes, the pixels.
fn encode_bitmap(b: &Bitmap) -> Vec<u8> {
	let mut out = Vec::with_capacity(44 + b.word_boxes.len() * 32 + b.pix.len());
	put_u32(&mut out, b.width);
	put_u32(&mut out, b.height);
	for v in [b.natural_w, b.natural_h, b.center_dx, b.center_dy] {
		put_f64(&mut out, v);
	}
	put_u32(&mut out, b.word_boxes.len());
	for w in &b.word_boxes {
		for v in [w.x, w.y, w.w, w.h] {
			put_f64(&mut out, v);
		}
	}
	out.extend_from_slice(&b.pix);
	out
}

/// The detail of `font_unknown`: family, weight, the weights that exist, the digest not stored.
fn font_unknown(e: &FontError) -> (Error, Vec<u8>) {
	let mut out = Vec::new();
	put_str(&mut out, &e.family);
	put_u32(&mut out, e.weight as usize);
	match &e.weights {
		Some(ws) => {
			out.push(1);
			put_u32(&mut out, ws.len());
			for w in ws {
				put_u32(&mut out, *w as usize);
			}
		}
		None => out.push(0),
	}
	match &e.hash {
		Some(h) => {
			out.push(1);
			put_str(&mut out, h);
		}
		None => out.push(0),
	}
	(Error::new(Reason::FontUnknown, e.to_string()), out)
}

/// A measure result: the natural size with no pixels.
fn measured(w: f64, h: f64) -> Bitmap {
	Bitmap {
		pix: Vec::new(),
		width: 0,
		height: 0,
		natural_w: w,
		natural_h: h,
		center_dx: 0.0,
		center_dy: 0.0,
		word_boxes: Vec::new(),
	}
}

impl TextHost {
	/// A host with the bundled fonts and no user font.
	pub fn new() -> Self {
		let store = Arc::new(Mutex::new(FontStore::default()));
		let reader = Arc::clone(&store);
		let src: FontSource = Arc::new(move |hash: &str| {
			let mut s = reader.lock().unwrap_or_else(PoisonError::into_inner);
			match s.files.get(hash) {
				Some(d) => Ok(d.clone()),
				None => {
					s.needed = Some(hash.to_owned());
					Err(SourceError::NotStored)
				}
			}
		});
		Self {
			fonts: FontSet::new(&[], Some(src)),
			store,
			result: Vec::new(),
		}
	}

	fn store(&self) -> std::sync::MutexGuard<'_, FontStore> {
		self.store.lock().unwrap_or_else(PoisonError::into_inner)
	}

	/// Stores the bytes of a user font under its lower-case hex SHA-256; empty bytes forget it. The
	/// module trusts the host's digest only to name the file: bytes that do not hash to it are
	/// refused when the font is parsed.
	pub fn put_font(&mut self, hash: &str, data: &[u8]) -> Result<(), Error> {
		let hex = hash
			.bytes()
			.all(|b| b.is_ascii_digit() || (b'a'..=b'f').contains(&b));
		if hash.len() != 64 || !hex {
			return Err(Error::new(
				Reason::InvalidArgument,
				"a font digest is 64 lower-case hex characters",
			));
		}
		if data.len() > MAX_FONT_BYTES {
			return Err(Error::new(Reason::LimitExceeded, "font_bytes"));
		}
		let mut s = self.store();
		let old = s.files.get(hash).map_or(0, Vec::len);
		if s.bytes - old + data.len() > MAX_FONT_STORE_BYTES {
			return Err(Error::new(Reason::LimitExceeded, "font_store_bytes"));
		}
		s.bytes = s.bytes - old + data.len();
		if data.is_empty() {
			s.files.remove(hash);
		} else {
			s.files.insert(hash.to_owned(), data.to_vec());
		}
		Ok(())
	}

	/// Validates a user font file (`raster.ValidateFont`). On a refusal the result holds the reason
	/// and the message of the file.
	pub fn validate_font(&mut self, data: &[u8]) -> Result<(), Error> {
		self.result = Vec::new();
		check_font(data).map_err(|e| {
			put_str(&mut self.result, e.reason.as_str());
			put_str(&mut self.result, &e.message);
			Error::new(Reason::FontInvalid, e.to_string())
		})
	}

	/// The result of the last request: the encoded bitmap, or the structured detail of a failure.
	pub fn result(&self) -> &[u8] {
		&self.result
	}

	/// Forgets the result so its memory can be reused.
	pub fn clear_result(&mut self) {
		self.result = Vec::new();
	}

	/// Runs one request: draws (or measures) a shape, a text or a caption. The result is
	/// [`result`](Self::result).
	pub fn run(&mut self, request: &[u8]) -> Result<(), Error> {
		self.result = Vec::new();
		self.store().needed = None;
		let mut r = Reader(request);
		let mode = match r.u8()? {
			0 => Mode::Render,
			1 => Mode::Measure,
			_ => return Err(Error::new(Reason::InvalidArgument, "unknown mode")),
		};
		let kind = r.u8()?;
		let pixel_scale = r.f64()?;
		let bitmap = match kind {
			0 => self.shape(&mut r, mode, pixel_scale)?,
			1 | 2 => self.text(&mut r, mode, kind == 2, pixel_scale)?,
			_ => return Err(Error::new(Reason::InvalidSpec, "unknown kind")),
		};
		self.result = encode_bitmap(&bitmap);
		Ok(())
	}

	fn shape(&self, r: &mut Reader<'_>, mode: Mode, scale: f64) -> Result<Bitmap, Error> {
		let kind = match r.u8()? {
			0 => ShapeKind::Rect,
			1 => ShapeKind::RoundedRect,
			2 => ShapeKind::Ellipse,
			_ => return Err(Error::new(Reason::InvalidSpec, "unknown shape")),
		};
		let (width, height, radius, stroke_width) = (r.f64()?, r.f64()?, r.f64()?, r.f64()?);
		let (fill, stroke_color) = (r.colour()?, r.colour()?);
		let sh = Shape {
			kind,
			width,
			height,
			radius,
			fill,
			stroke_color,
			stroke_width,
		};
		let invalid = |m: String| Error::new(Reason::InvalidSpec, m);
		if !(scale > 0.0 && scale <= 8.0) {
			return Err(invalid(format!("pixel scale {scale}")));
		}
		if !(width > 0.0 && height > 0.0) {
			return Err(invalid(
				"shape needs a positive width and height".to_owned(),
			));
		}
		if mode == Mode::Measure {
			let g = shape_geometry(&sh, scale);
			return Ok(measured(g.cw as f64 / scale, g.ch as f64 / scale));
		}
		render_shape(&sh, scale).map_err(|e| invalid(e.0))
	}

	fn text(
		&mut self,
		r: &mut Reader<'_>,
		mode: Mode,
		caption: bool,
		pixel_scale: f64,
	) -> Result<Bitmap, Error> {
		let mut docs: Vec<(String, Vec<DocFontFile>)> = Vec::new();
		for _ in 0..r.count()? {
			let name = r.str()?;
			let mut files = Vec::new();
			for _ in 0..r.count()? {
				files.push(DocFontFile {
					weight: r.u32()?,
					hash: r.str()?,
				});
			}
			docs.push((name, files));
		}
		let style = style(r)?;
		let content = if caption {
			let mut words = Vec::new();
			for _ in 0..r.count()? {
				words.push(r.str()?);
			}
			let mut breaks = Vec::new();
			for _ in 0..r.count()? {
				breaks.push(r.u32()? as usize);
			}
			let active = r.u32()? as usize;
			Content::Caption {
				words,
				breaks,
				active_word: active.checked_sub(1),
			}
		} else {
			Content::Text(r.str()?)
		};
		let spec = TextSpec {
			content,
			style,
			pixel_scale,
		};
		self.fonts.set_doc_fonts(&docs);
		if mode == Mode::Measure {
			let (w, h) = measure_text(&spec, &self.fonts);
			return Ok(measured(w, h));
		}
		match render_text(&spec, &self.fonts) {
			Ok(b) if b.width * b.height <= MAX_PIXELS => Ok(b),
			Ok(_) => Err(Error::new(Reason::LimitExceeded, "bitmap_pixels")),
			Err(e) => Err(self.text_failure(e)),
		}
	}

	fn text_failure(&mut self, e: RenderError) -> Error {
		match e {
			RenderError::Invalid(i) => Error::new(Reason::InvalidSpec, i.0),
			RenderError::Font(LookupError::Font(f)) => {
				if let Some(hash) = self.store().needed.take() {
					return Error::new(Reason::FontNeeded, hash);
				}
				let (err, detail) = font_unknown(&f);
				self.result = detail;
				err
			}
			RenderError::Font(LookupError::Invalid(i)) => {
				put_str(&mut self.result, i.reason.as_str());
				put_str(&mut self.result, &i.message);
				Error::new(Reason::FontInvalid, i.to_string())
			}
			RenderError::Font(LookupError::Source(m)) => Error::new(Reason::TextFailed, m),
		}
	}
}

#[cfg(test)]
mod tests {
	use super::*;

	fn str_(out: &mut Vec<u8>, s: &str) {
		put_str(out, s);
	}

	fn colour(out: &mut Vec<u8>, c: [u8; 4]) {
		out.extend_from_slice(&c);
	}

	/// A text request: mode, kind 1, scale, `docs` (family and one file each), Inter 400, size 40.
	fn text_request(mode: u8, family: &str, docs: &[(&str, &str)], text: &str) -> Vec<u8> {
		let mut b = vec![mode, 1];
		put_f64(&mut b, 1.0);
		put_u32(&mut b, docs.len());
		for (name, hash) in docs {
			str_(&mut b, name);
			put_u32(&mut b, 1);
			put_u32(&mut b, 400);
			str_(&mut b, hash);
		}
		str_(&mut b, family);
		put_u32(&mut b, 400);
		put_f64(&mut b, 40.0);
		colour(&mut b, [255, 255, 255, 255]);
		colour(&mut b, [0, 0, 0, 0]);
		put_f64(&mut b, 0.0);
		colour(&mut b, [0, 0, 0, 0]);
		for _ in 0..3 {
			put_f64(&mut b, 0.0);
		}
		b.push(0);
		put_f64(&mut b, 0.0);
		put_f64(&mut b, 0.0);
		b.push(1);
		put_f64(&mut b, 1.2);
		put_f64(&mut b, 800.0);
		b.extend_from_slice(&[0, 0]);
		colour(&mut b, [0, 0, 0, 0]);
		b.push(0);
		str_(&mut b, text);
		b
	}

	#[test]
	fn renders_and_measures_the_same_size() {
		let mut h = TextHost::new();
		h.run(&text_request(0, "Inter", &[], "Wow")).unwrap();
		let drawn = h.result().to_vec();
		let (w, ht) = (
			u32::from_le_bytes(drawn[0..4].try_into().unwrap()) as usize,
			u32::from_le_bytes(drawn[4..8].try_into().unwrap()) as usize,
		);
		assert!(w > 0 && ht > 0);
		assert_eq!(drawn.len(), 44 + w * ht * 4);
		h.run(&text_request(1, "Inter", &[], "Wow")).unwrap();
		let m = h.result();
		assert_eq!(m.len(), 44);
		assert_eq!(m[8..24], drawn[8..24], "natural size");
	}

	#[test]
	fn an_unknown_family_reports_it_and_a_missing_user_font_is_asked_for() {
		let mut h = TextHost::new();
		let e = h.run(&text_request(0, "Nope", &[], "x")).unwrap_err();
		assert_eq!(e.reason, Reason::FontUnknown);
		assert!(!h.result().is_empty());

		let hash = "ab".repeat(32);
		let req = text_request(0, "Mine", &[("Mine", &hash)], "x");
		let e = h.run(&req).unwrap_err();
		assert_eq!(
			(e.reason, e.detail.as_str()),
			(Reason::FontNeeded, hash.as_str())
		);

		let e = h
			.put_font(&hash, b"not a font")
			.and_then(|()| h.run(&req))
			.unwrap_err();
		assert_eq!(
			e.reason,
			Reason::TextFailed,
			"bytes that do not hash to the digest"
		);
		h.put_font(&hash, &[]).unwrap();
		assert_eq!(
			h.run(&req).unwrap_err().reason,
			Reason::FontNeeded,
			"forgotten"
		);
	}

	#[test]
	fn refuses_what_it_cannot_read() {
		let mut h = TextHost::new();
		assert_eq!(h.run(&[]).unwrap_err().reason, Reason::InvalidArgument);
		assert_eq!(
			h.run(&[0, 1, 0]).unwrap_err().reason,
			Reason::InvalidArgument
		);
		assert_eq!(h.run(&[9, 1]).unwrap_err().reason, Reason::InvalidArgument);
		assert_eq!(
			h.put_font("zz", &[1]).unwrap_err().reason,
			Reason::InvalidArgument
		);
		assert_eq!(
			h.validate_font(b"not a font").unwrap_err().reason,
			Reason::FontInvalid
		);
		assert!(!h.result().is_empty());
	}
}
