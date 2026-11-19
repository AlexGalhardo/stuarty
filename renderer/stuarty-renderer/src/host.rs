//! The renderer state a host talks to (SDD-013 section 4.2): textures and bitmaps held between
//! calls, and one render of a frame. The C-ABI of the WebAssembly module and the native parity
//! process are thin wrappers over this type.

use std::collections::BTreeMap;

use crate::compose::{Layer, Source, Target, compose, frame_fill, layer_opacity, premultiply};
use crate::error::{Error, Reason};
use crate::image::Image;
use crate::lut::{Lut, lut_mix};
use crate::plan::{MAX_ID_BYTES, Pixels, RenderInput, decode};

/// The largest side of a texture or bitmap.
pub const MAX_TEXTURE_SIDE: usize = 8192;
/// The default budget of textures in memory.
pub const DEFAULT_MAX_TEXTURE_BYTES: usize = 512 << 20;
/// The default budget of bitmaps in memory.
pub const DEFAULT_MAX_BITMAP_BYTES: usize = 128 << 20;
/// The most lookup tables held at once, and the memory they may take (65 points on a side is 1.6 MiB).
pub const MAX_LUTS: usize = 32;
pub const MAX_LUT_BYTES: usize = 64 << 20;

/// The two budgets of `vr_init`.
#[derive(Clone, Copy, Debug, PartialEq, Eq)]
pub struct Limits {
	pub max_texture_bytes: usize,
	pub max_bitmap_bytes: usize,
}

impl Default for Limits {
	fn default() -> Self {
		Self {
			max_texture_bytes: DEFAULT_MAX_TEXTURE_BYTES,
			max_bitmap_bytes: DEFAULT_MAX_BITMAP_BYTES,
		}
	}
}

/// The metrics a bitmap travels with (SDD-013 section 2.5).
#[derive(Clone, Copy, Debug, PartialEq)]
pub struct BitmapMetrics {
	pub natural_w: f64,
	pub natural_h: f64,
	pub center_dx: f64,
	pub center_dy: f64,
}

struct StoredBitmap {
	image: Image,
	metrics: BitmapMetrics,
}

/// One renderer: the stored pixels and the budgets. It is single threaded.
pub struct Renderer {
	limits: Limits,
	textures: BTreeMap<String, Image>,
	bitmaps: BTreeMap<String, StoredBitmap>,
	luts: BTreeMap<String, Lut>,
	texture_bytes: usize,
	bitmap_bytes: usize,
}

impl Renderer {
	/// An empty renderer with its budgets (`vr_init`).
	pub fn new(limits: Limits) -> Self {
		Self {
			limits,
			textures: BTreeMap::new(),
			bitmaps: BTreeMap::new(),
			luts: BTreeMap::new(),
			texture_bytes: 0,
			bitmap_bytes: 0,
		}
	}

	/// Stores the premultiplied RGBA8 pixels of a media layer under the item id, replacing the
	/// texture of that id. The pixels are copied.
	pub fn put_texture(
		&mut self,
		id: &str,
		w: usize,
		h: usize,
		rgba: &[u8],
		opaque: bool,
	) -> Result<(), Error> {
		check_id(id)?;
		let n = check_pixels(w, h, rgba)?;
		let old = self.textures.get(id).map_or(0, |t| t.pix.len());
		if self.texture_bytes - old + n > self.limits.max_texture_bytes {
			return Err(Error::new(Reason::LimitExceeded, "render_texture_bytes"));
		}
		let image = Image {
			pix: rgba[..n].to_vec(),
			w,
			h,
			opaque,
		};
		self.texture_bytes = self.texture_bytes - old + n;
		self.textures.insert(id.to_string(), image);
		Ok(())
	}

	/// Stores a bitmap of the rasterizer, straight alpha, under its key; it is premultiplied once.
	pub fn put_bitmap(
		&mut self,
		key: &str,
		w: usize,
		h: usize,
		rgba: &[u8],
		metrics: BitmapMetrics,
	) -> Result<(), Error> {
		check_id(key)?;
		let n = check_pixels(w, h, rgba)?;
		let old = self.bitmaps.get(key).map_or(0, |b| b.image.pix.len());
		if self.bitmap_bytes - old + n > self.limits.max_bitmap_bytes {
			return Err(Error::new(Reason::LimitExceeded, "render_bitmap_bytes"));
		}
		let image = premultiply(&rgba[..n], w, h);
		self.bitmap_bytes = self.bitmap_bytes - old + n;
		self.bitmaps
			.insert(key.to_string(), StoredBitmap { image, metrics });
		Ok(())
	}

	/// Stores a lookup table under the id of its asset, replacing the one of that id: `size` points on a side and
	/// `size^3 x 3` little-endian 16-bit entries (red varies fastest). At most `MAX_LUTS` tables and
	/// `MAX_LUT_BYTES` are held.
	pub fn put_lut(&mut self, id: &str, size: usize, entries: &[u8]) -> Result<(), Error> {
		check_id(id)?;
		let lut = Lut::from_bytes(size, entries)?;
		let old = self.luts.get(id).map_or(0, Lut::byte_len);
		let held: usize = self.luts.values().map(Lut::byte_len).sum();
		if (self.luts.len() >= MAX_LUTS && !self.luts.contains_key(id))
			|| held - old + lut.byte_len() > MAX_LUT_BYTES
		{
			return Err(Error::new(Reason::LimitExceeded, "render_luts"));
		}
		self.luts.insert(id.to_string(), lut);
		Ok(())
	}

	/// Forgets the texture, the bitmap or the lookup table with this id or key; an unknown one is not an error.
	pub fn release(&mut self, id: &str) {
		self.luts.remove(id);
		if let Some(t) = self.textures.remove(id) {
			self.texture_bytes -= t.pix.len();
		}
		if let Some(b) = self.bitmaps.remove(id) {
			self.bitmap_bytes -= b.image.pix.len();
		}
	}

	/// Draws the render input into `out`, which must be exactly `width x height x 4` bytes
	/// (`vr_render`). Only the rows of the input's band are written.
	pub fn render(&self, input: &[u8], out: &mut [u8]) -> Result<(), Error> {
		let r = decode(input)?;
		let (width, height) = (r.output.width, r.output.height);
		if out.len() != width * height * 4 {
			return Err(Error::new(
				Reason::InvalidArgument,
				"out_len is not the frame size",
			));
		}

		let layers = self.layers(&r)?;

		let mut target = Target {
			pix: out,
			width,
			height,
			rows: r.output.rows.clone(),
		};
		compose(
			&mut target,
			frame_fill(&r.background, r.transparent),
			&layers,
			r.output.scale,
		);
		Ok(())
	}

	/// The layers of a decoded render input with the stored pixels they draw, in plan order. A
	/// layer that rounds to no opacity is left out; a texture or bitmap the host did not store is
	/// an error. The CPU and the GPU path both draw this list.
	pub fn layers<'a>(&'a self, r: &RenderInput) -> Result<Vec<Layer<'a>>, Error> {
		let mut layers = Vec::with_capacity(r.layers.len());
		for l in &r.layers {
			// A layer that rounds to no opacity is not drawn, so the host need not have uploaded it.
			if layer_opacity(l.opacity) == 0 {
				continue;
			}
			let source = match &l.pixels {
				Pixels::Media(id) => match self.textures.get(id) {
					Some(img) => Source::Media(img),
					None => return Err(Error::new(Reason::MissingTexture, id.clone())),
				},
				Pixels::Bitmap(key) => match self.bitmaps.get(key) {
					Some(b) => Source::Bitmap {
						image: &b.image,
						natural_w: b.metrics.natural_w,
						center_dx: b.metrics.center_dx,
						center_dy: b.metrics.center_dy,
					},
					None => return Err(Error::new(Reason::MissingTexture, key.clone())),
				},
			};
			let lut = match &l.lut {
				// A table mixed in at no thousandth is not drawn, so the host need not have uploaded it.
				Some(r) if lut_mix(r.intensity) > 0 => match self.luts.get(&r.asset) {
					Some(t) => Some((t, r.intensity)),
					None => return Err(Error::new(Reason::MissingTexture, r.asset.clone())),
				},
				_ => None,
			};
			layers.push(Layer {
				source,
				cx: l.cx,
				cy: l.cy,
				w: l.w,
				h: l.h,
				rotation: l.rotation,
				opacity: l.opacity,
				color_matrix: l.color_matrix,
				filter: l.filter,
				grade: l.grade.clone(),
				lut,
				blur: l.blur,
				chroma_key: l.chroma_key.clone(),
				blend: l.blend,
				mask: l.mask,
				flip_x: l.flip_x,
				flip_y: l.flip_y,
				effects: l.effects.clone(),
				transition: l.transition,
				frame: r.frame,
				shadow: l.shadow.clone(),
				outline: l.outline.clone(),
			});
		}
		Ok(layers)
	}
}

fn check_id(id: &str) -> Result<(), Error> {
	if id.is_empty() || id.len() > MAX_ID_BYTES || !id.is_ascii() {
		return Err(Error::new(
			Reason::InvalidArgument,
			"an id is not 1 to 64 ASCII bytes",
		));
	}
	Ok(())
}

/// The byte length of a `w x h` RGBA8 block, after the size and the block are checked.
fn check_pixels(w: usize, h: usize, rgba: &[u8]) -> Result<usize, Error> {
	if w == 0 || h == 0 {
		return Err(Error::new(
			Reason::InvalidArgument,
			"a texture has no pixels",
		));
	}
	if w > MAX_TEXTURE_SIDE || h > MAX_TEXTURE_SIDE {
		return Err(Error::new(Reason::LimitExceeded, "render_texture_side"));
	}
	let n = w * h * 4;
	if rgba.len() < n {
		return Err(Error::new(
			Reason::InvalidArgument,
			"the pixels are shorter than width x height x 4",
		));
	}
	Ok(n)
}

#[cfg(test)]
mod tests {
	use super::*;

	const INPUT: &str = r##"{"version":1,"output":{"width":4,"height":4,"scale":1},"plan":{"background":"#000000","layers":[
		{"item":"a","kind":"media","cx":2,"cy":2,"w":2,"h":2,"opacity":1}]}}"##;

	fn white(w: usize, h: usize) -> Vec<u8> {
		vec![255; w * h * 4]
	}

	#[test]
	fn renders_a_stored_texture() {
		let mut r = Renderer::new(Limits::default());
		r.put_texture("a", 2, 2, &white(2, 2), true).unwrap();
		let mut out = vec![0_u8; 4 * 4 * 4];
		r.render(INPUT.as_bytes(), &mut out).unwrap();
		let px = |x: usize, y: usize| out[(y * 4 + x) * 4..(y * 4 + x) * 4 + 4].to_vec();
		assert_eq!(px(1, 1), [255; 4]);
		assert_eq!(px(0, 0), [0, 0, 0, 255]);
	}

	#[test]
	fn a_band_writes_only_its_rows() {
		let mut r = Renderer::new(Limits::default());
		r.put_texture("a", 2, 2, &white(2, 2), true).unwrap();
		let band = INPUT.replace("\"scale\":1", "\"scale\":1,\"y0\":2,\"y1\":3");
		let mut out = vec![7_u8; 4 * 4 * 4];
		r.render(band.as_bytes(), &mut out).unwrap();
		assert!(out[..2 * 16].iter().all(|&b| b == 7));
		assert!(out[3 * 16..].iter().all(|&b| b == 7));
		assert_eq!(out[2 * 16 + 4..2 * 16 + 8], [255; 4]);
	}

	#[test]
	fn reports_what_is_missing_or_wrong() {
		let mut r = Renderer::new(Limits::default());
		let mut out = vec![0_u8; 64];
		assert_eq!(
			r.render(INPUT.as_bytes(), &mut out),
			Err(Error::new(Reason::MissingTexture, "a"))
		);
		r.put_texture("a", 2, 2, &white(2, 2), true).unwrap();
		assert_eq!(
			r.render(INPUT.as_bytes(), &mut out[..63])
				.unwrap_err()
				.reason,
			Reason::InvalidArgument
		);
		assert_eq!(
			r.put_texture("a", 2, 2, &white(2, 2)[..15], true)
				.unwrap_err()
				.reason,
			Reason::InvalidArgument
		);
		assert_eq!(
			r.put_texture("a", 8193, 1, &[], true).unwrap_err().reason,
			Reason::LimitExceeded
		);
		assert_eq!(
			r.put_texture(&"x".repeat(65), 1, 1, &white(1, 1), true)
				.unwrap_err()
				.reason,
			Reason::InvalidArgument
		);
	}

	#[test]
	fn budgets_refuse_without_changing_anything() {
		let mut r = Renderer::new(Limits {
			max_texture_bytes: 20,
			max_bitmap_bytes: 8,
		});
		r.put_texture("a", 2, 2, &white(2, 2), true).unwrap();
		r.put_texture("a", 2, 2, &white(2, 2), true).unwrap();
		assert_eq!(
			r.put_texture("b", 2, 2, &white(2, 2), true)
				.unwrap_err()
				.detail,
			"render_texture_bytes"
		);
		r.release("a");
		r.put_texture("b", 2, 2, &white(2, 2), true).unwrap();
		let m = BitmapMetrics {
			natural_w: 2.0,
			natural_h: 2.0,
			center_dx: 0.0,
			center_dy: 0.0,
		};
		assert_eq!(
			r.put_bitmap("k", 2, 2, &white(2, 2), m).unwrap_err().detail,
			"render_bitmap_bytes"
		);
	}
}
