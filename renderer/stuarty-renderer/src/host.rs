//! The renderer state a host talks to (SDD-013 section 4.2): textures and bitmaps held between
//! calls, and one render of a frame. The C-ABI of the WebAssembly module and the native parity
//! process are thin wrappers over this type.

use std::collections::BTreeMap;

use crate::compose::{
	Layer, LayerExt, Source, Target, compose, frame_fill, layer_opacity, premultiply,
	prepare_layer_offset,
};
use crate::error::{Error, Reason};
use crate::image::Image;
use crate::lut::{Lut, lut_mix};
use crate::plan::{LayerSpec, MAX_ID_BYTES, Pixels, RenderInput, decode};

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

/// What a prepared layer keeps beside its pixels (SDD-013 section 4.2.2).
#[derive(Clone, Copy, Debug, PartialEq)]
struct PreparedMeta {
	grown_w: f64,
	grown_h: f64,
	off_x: f64,
	off_y: f64,
	scale: f64,
}

/// The bytes before the pixels of a prepared block: width, height, flags and a zero (4 x `u32`), then the
/// five `f64` of [`PreparedMeta`].
const PREPARED_HEADER: usize = 16 + 40;

struct StoredBitmap {
	image: Image,
	metrics: BitmapMetrics,
}

/// One renderer: the stored pixels and the budgets. It is single threaded.
pub struct Renderer {
	limits: Limits,
	textures: BTreeMap<String, Image>,
	/// The textures that are prepared layers (`vr_put_prepared`), with what places them again.
	prepared: BTreeMap<String, PreparedMeta>,
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
			prepared: BTreeMap::new(),
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
		self.prepared.remove(id);
		Ok(())
	}

	/// Prepares the one layer of the render input (steps 1 to 5 of SDD-004 section 3.3 at opacity 1) and gives
	/// the block `put_prepared` stores: `width`, `height`, flags (bit 0: opaque) and 0 as `u32`, then the grown
	/// width and height, the offset of the centre and the scale as `f64`, then the pixels, all little endian.
	/// The block is empty when the layer draws nothing.
	pub fn prepare(&self, input: &[u8]) -> Result<Vec<u8>, Error> {
		let mut r = decode(input)?;
		if r.layers.len() != 1 {
			return Err(Error::new(
				Reason::InvalidPlan,
				"a prepare takes exactly one layer",
			));
		}
		r.layers[0].opacity = 1.0;
		r.layers[0].clip = None;
		let layers = self.layers(&r)?;
		let scale = r.output.scale;
		let Some((p, off_x, off_y)) = prepare_layer_offset(&layers[0], scale) else {
			return Ok(Vec::new());
		};
		let img = &*p.image;
		let mut block = Vec::with_capacity(PREPARED_HEADER + img.pix.len());
		for v in [img.w as u32, img.h as u32, u32::from(img.opaque), 0] {
			block.extend_from_slice(&v.to_le_bytes());
		}
		for v in [p.placement.w, p.placement.h, off_x, off_y, scale] {
			block.extend_from_slice(&v.to_le_bytes());
		}
		block.extend_from_slice(&img.pix);
		Ok(block)
	}

	/// Stores a block of [`Renderer::prepare`] as the texture of `id`: a media layer that names `id` is then
	/// drawn from it with no look (SDD-013 section 4.2.2).
	pub fn put_prepared(&mut self, id: &str, block: &[u8]) -> Result<(), Error> {
		check_id(id)?;
		let bad = |d: &str| Error::new(Reason::InvalidArgument, d.to_string());
		if block.len() < PREPARED_HEADER {
			return Err(bad("a prepared block is shorter than its header"));
		}
		let u = |i: usize| u32::from_le_bytes(block[i..i + 4].try_into().unwrap_or([0; 4]));
		let f = |i: usize| f64::from_le_bytes(block[i..i + 8].try_into().unwrap_or([0; 8]));
		let (w, h, flags) = (u(0) as usize, u(4) as usize, u(8));
		let meta = PreparedMeta {
			grown_w: f(16),
			grown_h: f(24),
			off_x: f(32),
			off_y: f(40),
			scale: f(48),
		};
		let finite = [
			meta.grown_w,
			meta.grown_h,
			meta.off_x,
			meta.off_y,
			meta.scale,
		]
		.iter()
		.all(|v| v.is_finite());
		if !finite || !(meta.grown_w > 0.0 && meta.grown_h > 0.0) || flags > 1 {
			return Err(bad(
				"a prepared block has a size or a flag that is not valid",
			));
		}
		let n = check_pixels(w, h, &block[PREPARED_HEADER..])?;
		if block.len() != PREPARED_HEADER + n {
			return Err(bad("a prepared block is longer than its pixels"));
		}
		self.put_texture(
			id,
			w,
			h,
			&block[PREPARED_HEADER..PREPARED_HEADER + n],
			flags == 1,
		)?;
		self.prepared.insert(id.to_string(), meta);
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
		self.prepared.remove(id);
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
		// A buffer is the whole frame, or only the rows of the band (SDD-013 section 4.2).
		let band = r
			.output
			.rows
			.end
			.min(height)
			.saturating_sub(r.output.rows.start);
		if out.len() != width * height * 4 && !(band > 0 && out.len() == width * band * 4) {
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
		let mut kept: Vec<&LayerSpec> = Vec::with_capacity(r.layers.len());
		for l in &r.layers {
			// A layer that rounds to no opacity is not drawn, so the host need not have uploaded it.
			if layer_opacity(l.opacity) == 0 {
				continue;
			}
			let source = match &l.pixels {
				Pixels::Media(id) => match (self.textures.get(id), self.prepared.get(id)) {
					(Some(img), Some(m)) => {
						if m.scale != r.output.scale {
							return Err(Error::new(Reason::InvalidPlan, "prepared_scale"));
						}
						Source::Prepared {
							image: img,
							grown_w: m.grown_w,
							grown_h: m.grown_h,
							off_x: m.off_x,
							off_y: m.off_y,
							scale: m.scale,
						}
					}
					(Some(img), None) => Source::Media(img),
					(None, _) => return Err(Error::new(Reason::MissingTexture, id.clone())),
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
				ext: LayerExt {
					tone: l.tone.clone(),
					filters: l.filters.clone(),
					clip: None,
					stencil: l.stencil,
				},
			});
			kept.push(l);
		}
		// A clip names its source by item id: the index of that layer in the list drawn. The source
		// must be there and must not be clipped itself (section 3.11.4).
		for (i, l) in kept.iter().enumerate() {
			let Some(c) = &l.clip else { continue };
			let source = kept
				.iter()
				.position(|k| k.item == c.source)
				.filter(|&j| j != i && kept[j].clip.is_none())
				.ok_or_else(|| {
					Error::new(
						Reason::InvalidPlan,
						format!("layer {}: the clip source is not a layer to read", l.item),
					)
				})?;
			layers[i].ext.clip = Some((source, c.invert));
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

	fn bitmap_renderer() -> Renderer {
		let mut r = Renderer::new(Limits::default());
		let mut px = vec![0_u8; 10 * 6 * 4];
		for (i, p) in px.chunks_mut(4).enumerate() {
			p.copy_from_slice(&[
				200,
				(i % 10 * 20) as u8,
				90,
				if i % 7 == 0 { 120 } else { 255 },
			]);
		}
		let m = BitmapMetrics {
			natural_w: 8.0,
			natural_h: 4.0,
			center_dx: 1.5,
			center_dy: -0.5,
		};
		r.put_bitmap("k", 10, 6, &px, m).unwrap();
		r
	}

	/// A bitmap layer with a shadow and a blur whose centre its metrics move, at `(cx, cy)`; the same layer as a
	/// media layer of item `t` when `prepared`.
	fn bitmap_input(cx: f64, cy: f64, prepared: bool) -> String {
		let pixels = if prepared {
			r#""kind":"media","item":"t""#
		} else {
			r#""kind":"bitmap","item":"t","bitmap":{"kind":"shape"},"bitmap_key":"k""#
		};
		format!(
			r##"{{"version":1,"output":{{"width":40,"height":30,"scale":1}},"plan":{{"background":"#204060","layers":[{{{pixels},"cx":{cx},"cy":{cy},"w":16,"h":8,"rotation":7,"opacity":0.6,"blend":"soft_light","blur":0.8,"filters":[{{"kind":"shadow","color":"#000000","opacity":0.6,"dx":2,"dy":3,"blur":2}},{{"kind":"outer_glow","color":"#FFAA00","radius":2,"amount":1.5}}],"mask":{{"shape":"rounded_rect","x":0,"y":0,"width":1,"height":1,"radius":3}}}}]}}}}"##
		)
	}

	#[test]
	fn a_prepared_bitmap_is_the_bitmap_drawn_whole_wherever_it_is() {
		let mut r = bitmap_renderer();
		let block = r
			.prepare(bitmap_input(20.0, 15.0, false).as_bytes())
			.unwrap();
		assert!(block.len() > PREPARED_HEADER);
		let whole = bitmap_renderer();
		r.put_prepared("t", &block).unwrap();
		for (cx, cy) in [(20.0, 15.0), (14.5, 11.0), (27.0, 19.5)] {
			let (mut a, mut b) = (vec![0_u8; 40 * 30 * 4], vec![0_u8; 40 * 30 * 4]);
			whole
				.render(bitmap_input(cx, cy, false).as_bytes(), &mut a)
				.unwrap();
			r.render(bitmap_input(cx, cy, true).as_bytes(), &mut b)
				.unwrap();
			assert!(a == b, "moved to {cx},{cy}");
			assert!(
				a.chunks(4).any(|p| p[..3] != [32, 64, 96]),
				"something is drawn"
			);
		}
	}

	#[test]
	fn a_prepared_layer_refuses_another_scale_and_a_bad_block() {
		let mut r = bitmap_renderer();
		let block = r
			.prepare(bitmap_input(20.0, 15.0, false).as_bytes())
			.unwrap();
		r.put_prepared("t", &block).unwrap();
		let mut out = vec![0_u8; 40 * 30 * 4];
		let half = bitmap_input(20.0, 15.0, true).replace("\"scale\":1", "\"scale\":0.5");
		assert_eq!(
			r.render(half.as_bytes(), &mut out).unwrap_err(),
			Error::new(Reason::InvalidPlan, "prepared_scale")
		);
		assert_eq!(
			r.put_prepared("u", &block[..PREPARED_HEADER - 1])
				.unwrap_err()
				.reason,
			Reason::InvalidArgument
		);
		assert_eq!(
			r.put_prepared("u", &block[..block.len() - 1])
				.unwrap_err()
				.reason,
			Reason::InvalidArgument
		);
		let mut long = block.clone();
		long.push(0);
		assert_eq!(
			r.put_prepared("u", &long).unwrap_err().reason,
			Reason::InvalidArgument
		);
		let mut nan = block;
		nan[16..24].copy_from_slice(&f64::NAN.to_le_bytes());
		assert_eq!(
			r.put_prepared("u", &nan).unwrap_err().reason,
			Reason::InvalidArgument
		);
	}

	#[test]
	fn a_layer_that_draws_nothing_prepares_to_an_empty_block() {
		let r = bitmap_renderer();
		let none = bitmap_input(20.0, 15.0, false).replace("\"w\":16", "\"w\":0");
		assert!(r.prepare(none.as_bytes()).unwrap().is_empty());
		let two = bitmap_input(20.0, 15.0, false).replace(
			"\"layers\":[",
			"\"layers\":[{\"kind\":\"bitmap\",\"item\":\"s\",\"bitmap\":{\"kind\":\"shape\"},\"bitmap_key\":\"k\",\"cx\":1,\"cy\":1,\"w\":2,\"h\":2},",
		);
		assert_eq!(
			r.prepare(two.as_bytes()).unwrap_err().reason,
			Reason::InvalidPlan
		);
	}
}
