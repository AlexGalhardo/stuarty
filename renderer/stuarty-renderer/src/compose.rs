//! The CPU path of the renderer: the port of `engine/compositor` (SDD-013 section 3).
//!
//! Every pixel is computed with integer arithmetic from numbers that are rounded once per layer,
//! so the result is the same on every system and does not depend on how many bands the frame is
//! split into: the result of a row never depends on another row.

use core::ops::Range;
use std::borrow::Cow;

use crate::blend::{BlendMode, mix_pixel};
use crate::blur::{blur_radius, blurred};
use crate::chroma::{ChromaKeySpec, apply_chroma_key};
use crate::colour::apply_matrix;
use crate::effects::{Effect, EffectGeom, FrameInfo, Transition, TransitionKind, apply_effects};
use crate::filters::{FilterOp, apply_filters};
use crate::grade::{Grade, GradeSpec, apply_grade};
use crate::image::Image;
use crate::lut::{Lut, apply_lut, lut_mix};
use crate::mask::{MaskSpec, MaskState, clamped_q, scale};
use crate::math::{div255, mul, round_half_away, round_half_up, sincos, u8_of};
use crate::resize::resize;
use crate::tone::{TONE_ENTRIES, apply_tone};

/// Where a source is drawn: a `w` by `h` rectangle centred on `(cx, cy)`, turned by `rot` degrees
/// clockwise, all in output pixels.
#[derive(Clone, Copy, Debug, PartialEq)]
pub struct Placement {
	pub cx: f64,
	pub cy: f64,
	pub w: f64,
	pub h: f64,
	pub rot: f64,
}

/// The pixels of a layer.
#[derive(Clone, Copy, Debug)]
pub enum Source<'a> {
	/// The decoded frame of a media layer, cropped and premultiplied (SDD-013 section 5.1).
	Media(&'a Image),
	/// A bitmap of the rasterizer, already premultiplied once, with its metrics at pixel scale 1.
	Bitmap {
		image: &'a Image,
		natural_w: f64,
		center_dx: f64,
		center_dy: f64,
	},
	/// A layer already prepared (SDD-013 section 4.2.2): its pixels hold every look, so only the mask, the
	/// wipe, the opacity and the blend mode are applied when it is drawn. `grown_w` and `grown_h` are the size of
	/// the rectangle that draws them, `off_x` and `off_y` the offset of the centre a bitmap's metrics give, and
	/// `scale` the output scale it was prepared at.
	Prepared {
		image: &'a Image,
		grown_w: f64,
		grown_h: f64,
		off_x: f64,
		off_y: f64,
		scale: f64,
	},
}

/// One layer of the frame plan, in project pixels, with its pixels.
#[derive(Clone, Debug)]
pub struct Layer<'a> {
	pub source: Source<'a>,
	pub cx: f64,
	pub cy: f64,
	pub w: f64,
	pub h: f64,
	pub rotation: f64,
	pub opacity: f64,
	pub color_matrix: Option<[f64; 12]>,
	pub filter: Option<[f64; 12]>,
	/// The advanced colour, after the colour matrix and before the filter (SDD-004 section 3.14).
	pub grade: Option<GradeSpec>,
	/// The lookup table and its intensity, after the grade and before the filter (SDD-004 section 3.6).
	pub lut: Option<(&'a Lut, f64)>,
	pub blur: f64,
	/// Applied to the layer's own pixels before the colour matrix (SDD-004 section 3.7).
	pub chroma_key: Option<ChromaKeySpec>,
	/// The mix with the destination of the last step of the draw (SDD-004 section 3.8).
	pub blend: BlendMode,
	/// Limits where the layer is drawn, in its own frame (SDD-004 section 3.9).
	pub mask: Option<MaskSpec>,
	/// Mirror the layer's own pixels across its vertical and horizontal axes (SDD-004 section 3.10).
	pub flip_x: bool,
	pub flip_y: bool,
	/// The pixel effects, in order, after the filter, the blur and the flip (SDD-004 section 3.13).
	pub effects: Vec<Effect>,
	/// The whip, dip or wipe in progress (SDD-004 section 3.2).
	pub transition: Option<Transition>,
	/// The plan's frame number and the project's size, for the grain and the whip.
	pub frame: FrameInfo,
	/// The drop shadow and the contour, drawn under the picture (SDD-004 section 3.15).
	pub shadow: Option<ShadowSpec>,
	pub outline: Option<OutlineSpec>,
	/// What an image layer adds (SDD-013 section 3.11).
	pub ext: LayerExt,
}

/// The parts of a layer that only an image document sets (SDD-013 section 3.11); all absent for a
/// video layer.
#[derive(Clone, Debug, Default)]
pub struct LayerExt {
	/// The tone table, applied before the colour matrix (section 3.11.1).
	pub tone: Option<Box<[u8; TONE_ENTRIES]>>,
	/// The ordered filters, applied after the blur (section 3.11.3).
	pub filters: Vec<FilterOp>,
	/// The index, in the list of layers drawn, of the layer whose alpha limits this one, and whether
	/// the coverage is inverted (section 3.11.4).
	pub clip: Option<(usize, bool)>,
	/// The layer is read by a clip and not drawn.
	pub stencil: bool,
}

/// A drop shadow. `color` is `#RRGGBB` or `#RRGGBBAA`; lengths are project pixels.
#[derive(Clone, Debug, PartialEq)]
pub struct ShadowSpec {
	pub color: String,
	pub opacity: f64,
	pub dx: f64,
	pub dy: f64,
	pub blur: f64,
}

/// A contour. `width` is project pixels.
#[derive(Clone, Debug, PartialEq)]
pub struct OutlineSpec {
	pub color: String,
	pub width: f64,
}

/// The largest shadow offset in source pixels (a valid plan is far below it; it bounds the memory of
/// a plan that is not, SDD-013 section 3.11.3).
const MAX_SHADOW_OFFSET: i64 = 2048;

/// The largest outline radius in source pixels, the same bound as an effect length.
const MAX_OUTLINE_RADIUS: usize = 256;

/// Premultiplies a straight-alpha RGBA8 bitmap once.
pub fn premultiply(pix: &[u8], w: usize, h: usize) -> Image {
	let n = w.saturating_mul(h).saturating_mul(4).min(pix.len());
	let mut out = vec![0_u8; n];
	for (o, s) in out
		.as_chunks_mut::<4>()
		.0
		.iter_mut()
		.zip(pix.as_chunks::<4>().0)
	{
		let a = u32::from(s[3]);
		o[0] = u8_of(div255(u32::from(s[0]) * a));
		o[1] = u8_of(div255(u32::from(s[1]) * a));
		o[2] = u8_of(div255(u32::from(s[2]) * a));
		o[3] = s[3];
	}
	Image {
		pix: out,
		w,
		h,
		opaque: false,
	}
}

/// The colour uncovered pixels are filled with. A transparent plan keeps the named colour and
/// sets alpha to 0 (SDD-004 section 6). Any other plan is opaque.
pub fn frame_fill(colour: &str, transparent: bool) -> [u8; 4] {
	let mut bg = parse_colour(colour);
	if transparent {
		bg[3] = 0;
	}
	bg
}

/// Reads `#RRGGBB`; an alpha, if any, is ignored (the frame fill forces 255, then [`frame_fill`]
/// clears it when the plan is transparent). Anything else is black.
pub fn parse_colour(s: &str) -> [u8; 4] {
	let black = [0, 0, 0, 255];
	let b = s.as_bytes();
	if b.len() < 7 || b[0] != b'#' {
		return black;
	}
	let mut out = black;
	for (i, o) in out.iter_mut().take(3).enumerate() {
		let (Some(hi), Some(lo)) = (hex_val(b[1 + 2 * i]), hex_val(b[2 + 2 * i])) else {
			return black;
		};
		*o = hi << 4 | lo;
	}
	out
}

fn hex_val(c: u8) -> Option<u8> {
	match c {
		b'0'..=b'9' => Some(c - b'0'),
		b'a'..=b'f' => Some(c - b'a' + 10),
		b'A'..=b'F' => Some(c - b'A' + 10),
		_ => None,
	}
}

/// The frame a composition writes: `width x height` RGBA8, no padding.
pub struct Target<'a> {
	pub pix: &'a mut [u8],
	pub width: usize,
	pub height: usize,
	/// The rows to fill and draw (a band of the export); the rows outside are not touched.
	pub rows: Range<usize>,
}

/// Draws one frame plan into `dst`, scaling every position and size by `out_scale`. The rows of
/// `dst` are filled with `background` first, which is opaque unless the plan is transparent.
pub fn compose(dst: &mut Target<'_>, background: [u8; 4], layers: &[Layer<'_>], out_scale: f64) {
	dst.rows = dst.rows.start.min(dst.height)..dst.rows.end.min(dst.height);
	if dst.pix.len() < dst.width * dst.height * 4 || dst.rows.is_empty() {
		return;
	}
	let (a, b) = (dst.rows.start * dst.width * 4, dst.rows.end * dst.width * 4);
	for px in dst.pix[a..b].as_chunks_mut::<4>().0.iter_mut() {
		px.copy_from_slice(&background);
	}
	for (i, l) in layers.iter().enumerate() {
		if l.ext.stencil {
			continue;
		}
		if l.ext.clip.is_some() {
			if let Some(p) = prepare_clipped(layers, i, dst.width, dst.height, &dst.rows, out_scale)
			{
				draw(dst, &p.image, &p.placement, p.op, p.blend, None);
			}
		} else {
			draw_layer(dst, l, out_scale);
		}
	}
}

/// The layer `i`, which has a clip, drawn into a transparent frame-sized image, multiplied by the
/// coverage of the clip (section 3.11.4) and ready to draw as a copy at the origin with the
/// opacity and the blend mode of the layer. The GPU path draws the same image. `None` when the
/// layer draws nothing.
pub fn prepare_clipped(
	layers: &[Layer<'_>],
	i: usize,
	width: usize,
	height: usize,
	rows: &Range<usize>,
	out_scale: f64,
) -> Option<Prepared<'static>> {
	let l = layers.get(i)?;
	let (source, invert) = l.ext.clip?;
	let op = layer_opacity(l.opacity);
	if op == 0 || source >= layers.len() || width == 0 || height == 0 {
		return None;
	}

	// The layer itself, with its own mask, on a transparent frame, source-over, full opacity.
	let mut own = l.clone();
	own.opacity = 1.0;
	own.blend = BlendMode::Normal;
	own.ext.clip = None;
	let mut image = Image::new(width, height);
	{
		let mut t = Target {
			pix: &mut image.pix,
			width,
			height,
			rows: rows.clone(),
		};
		draw_layer(&mut t, &own, out_scale);
	}

	// The alpha of the source, drawn alone the same way.
	let mut src = layers[source].clone();
	src.opacity = 1.0;
	src.blend = BlendMode::Normal;
	src.ext.clip = None;
	let mut plane = vec![0_u8; width * height * 4];
	{
		let mut t = Target {
			pix: &mut plane,
			width,
			height,
			rows: rows.clone(),
		};
		draw_layer(&mut t, &src, out_scale);
	}

	for (px, cv) in image
		.pix
		.as_chunks_mut::<4>()
		.0
		.iter_mut()
		.zip(plane.as_chunks::<4>().0)
	{
		let cov = if invert {
			255 - u32::from(cv[3])
		} else {
			u32::from(cv[3])
		};
		if cov == 255 {
			continue;
		}
		for v in px.iter_mut() {
			*v = u8_of(div255(u32::from(*v) * cov));
		}
	}

	Some(Prepared {
		image: Cow::Owned(image),
		placement: Placement {
			cx: width as f64 / 2.0,
			cy: height as f64 / 2.0,
			w: width as f64,
			h: height as f64,
			rot: 0.0,
		},
		op,
		blend: l.blend,
		mask: None,
	})
}

/// The opacity of a layer as the 0..255 weight of the blend (SDD-013 section 3.3 step 2.1). Zero
/// means the layer is not drawn.
pub fn layer_opacity(opacity: f64) -> u32 {
	round_half_away(opacity.clamp(0.0, 1.0) * 255.0) as u32
}

/// A layer ready to draw: its pixels after the chroma key, the matrices and the blur, where the
/// rectangle is, and the opacity. The GPU path draws the same prepared layers.
#[derive(Debug)]
pub struct Prepared<'a> {
	pub image: Cow<'a, Image>,
	pub placement: Placement,
	pub op: u32,
	pub blend: BlendMode,
	/// The mask, placed on the layer's rectangle before the blur grew it.
	pub mask: Option<MaskState>,
}

/// Steps 1 to 5 of SDD-013 section 3.3 for one layer; `None` when the layer draws nothing.
pub fn prepare_layer<'a>(l: &Layer<'a>, out_scale: f64) -> Option<Prepared<'a>> {
	prepare_layer_offset(l, out_scale).map(|(p, _, _)| p)
}

/// [`prepare_layer`] and the offset of the centre that a bitmap's metrics gave the placement (0 and 0 for a
/// media layer), which a prepared layer keeps to be placed again (SDD-013 section 4.2.2).
pub fn prepare_layer_offset<'a>(l: &Layer<'a>, out_scale: f64) -> Option<(Prepared<'a>, f64, f64)> {
	let op = layer_opacity(l.opacity);
	if op == 0 {
		return None;
	}

	let (image, p, is_bitmap, off) = match l.source {
		Source::Media(img) => {
			let p = Placement {
				cx: l.cx * out_scale,
				cy: l.cy * out_scale,
				w: l.w * out_scale,
				h: l.h * out_scale,
				rot: l.rotation,
			};
			(img, p, false, (0.0, 0.0))
		}
		Source::Prepared {
			image,
			grown_w,
			grown_h,
			off_x,
			off_y,
			..
		} => {
			let p = Placement {
				cx: (l.cx - off_x) * out_scale,
				cy: (l.cy - off_y) * out_scale,
				w: l.w * out_scale,
				h: l.h * out_scale,
				rot: l.rotation,
			};
			if !(p.w > 0.0 && p.h > 0.0) {
				return None;
			}
			let wipe = l.transition.filter(|t| t.kind == TransitionKind::Wipe);
			let mask = (l.mask.is_some() || wipe.is_some())
				.then(|| MaskState::new(l.mask.as_ref(), wipe.as_ref(), &p, out_scale));
			let placement = Placement {
				w: grown_w,
				h: grown_h,
				..p
			};
			return Some((
				Prepared {
					image: Cow::Borrowed(image),
					placement,
					op,
					blend: l.blend,
					mask,
				},
				off_x,
				off_y,
			));
		}
		Source::Bitmap {
			image,
			natural_w,
			center_dx,
			center_dy,
		} => {
			// The plan places the centre of the text block; the bitmap reaches further on the
			// side of a shadow, so its own centre is offset by the metrics of the rasterizer.
			let sc = if natural_w > 0.0 {
				l.w / natural_w
			} else {
				1.0
			};
			let off = (mul(center_dx, sc), mul(center_dy, sc));
			let p = Placement {
				cx: (l.cx - off.0) * out_scale,
				cy: (l.cy - off.1) * out_scale,
				w: l.w * out_scale,
				h: l.h * out_scale,
				rot: l.rotation,
			};
			(image, p, true, off)
		}
	};

	if !(p.w > 0.0 && p.h > 0.0) || !image.is_drawable() {
		return None;
	}

	// A layer that needs no processing is drawn from the stored pixels, never from a copy.
	let mut src: Cow<'a, Image> = shrunk(image, is_bitmap, &p);
	if let Some(k) = &l.chroma_key {
		src = Cow::Owned(apply_chroma_key(&src, k));
	}
	if let Some(t) = &l.ext.tone {
		src = Cow::Owned(apply_tone(&src, t));
	}
	if let Some(m) = &l.color_matrix {
		src = Cow::Owned(apply_matrix(&src, m));
	}
	if let Some(g) = l.grade.as_ref().and_then(Grade::new) {
		src = Cow::Owned(apply_grade(&src, &g));
	}
	if let Some((table, intensity)) = l.lut
		&& lut_mix(intensity) > 0
	{
		src = Cow::Owned(apply_lut(&src, table, lut_mix(intensity)));
	}
	if let Some(m) = &l.filter {
		src = Cow::Owned(apply_matrix(&src, m));
	}
	// The mask keeps the frame of the layer before the blur grows it; a wipe is a coverage of the same
	// kind, multiplied with it (SDD-004 sections 3.2 and 3.9).
	let wipe = l.transition.filter(|t| t.kind == TransitionKind::Wipe);
	let mask = (l.mask.is_some() || wipe.is_some())
		.then(|| MaskState::new(l.mask.as_ref(), wipe.as_ref(), &p, out_scale));
	let pre = EffectGeom {
		sw: src.w as i64,
		sh: src.h as i64,
		offx: 0,
		offy: 0,
		wq: clamped_q(p.w),
		hq: clamped_q(p.h),
	};
	let mut p = p;
	if l.blur > 0.0
		&& let Some((img, grown)) = blurred(&src, &p, mul(l.blur, out_scale))
	{
		src = Cow::Owned(img);
		p = grown;
	}
	if !l.ext.filters.is_empty() {
		let (img, grown) = apply_filters(
			src.into_owned(),
			p,
			pre,
			&l.ext.filters,
			&l.frame,
			out_scale,
		);
		src = Cow::Owned(img);
		p = grown;
	}
	if l.flip_x || l.flip_y {
		src = Cow::Owned(flipped(&src, l.flip_x, l.flip_y));
	}
	if let Some((img, grown)) = apply_effects(
		&src,
		&p,
		pre,
		&l.effects,
		l.transition.as_ref(),
		&l.frame,
		out_scale,
	) {
		src = Cow::Owned(img);
		p = grown;
	}
	// The shadow and the outline sit under the picture and are cut by the mask with it.
	if let Some((img, grown)) = shadowed(&src, &p, l.shadow.as_ref(), l.outline.as_ref(), out_scale)
	{
		src = Cow::Owned(img);
		p = grown;
	}
	Some((
		Prepared {
			image: src,
			placement: p,
			op,
			blend: l.blend,
			mask,
		},
		off.0,
		off.1,
	))
}

/// `src` mirrored across its vertical axis, its horizontal axis or both: an exact permutation of
/// pixels (SDD-004 section 3.10).
fn flipped(src: &Image, flip_x: bool, flip_y: bool) -> Image {
	let stride = src.w * 4;
	let mut out = Image {
		pix: vec![0; src.h * stride],
		w: src.w,
		h: src.h,
		opaque: src.opaque,
	};
	for y in 0..src.h {
		let sy = if flip_y { src.h - 1 - y } else { y };
		let row = &src.pix[sy * stride..(sy + 1) * stride];
		let dst = &mut out.pix[y * stride..(y + 1) * stride];
		if flip_x {
			for (d, s) in dst
				.as_chunks_mut::<4>()
				.0
				.iter_mut()
				.zip(row.as_chunks::<4>().0.iter().rev())
			{
				*d = *s;
			}
		} else {
			dst.copy_from_slice(row);
		}
	}
	out
}

/// Reads `#RRGGBB` (alpha 255) or `#RRGGBBAA`. Anything else is opaque black.
pub fn parse_rgba(s: &str) -> [u8; 4] {
	let mut out = [0, 0, 0, 255];
	let b = s.as_bytes();
	if (b.len() != 7 && b.len() != 9) || b[0] != b'#' {
		return out;
	}
	for i in 0..(b.len() - 1) / 2 {
		let (Some(hi), Some(lo)) = (hex_val(b[1 + 2 * i]), hex_val(b[2 + 2 * i])) else {
			return [0, 0, 0, 255];
		};
		out[i] = hi << 4 | lo;
	}
	out
}

fn src_len_signed(length: f64, out_scale: f64, src: usize, drawn: f64) -> i64 {
	if drawn <= 0.0 || drawn.is_nan() {
		return 0;
	}
	round_half_up(length * out_scale * src as f64 / drawn)
}

fn src_len(length: f64, out_scale: f64, src: usize, drawn: f64) -> usize {
	let n = src_len_signed(length, out_scale, src, drawn);
	if n < 0 { 0 } else { n as usize }
}

fn iabs(v: i64) -> usize {
	usize::try_from(v.unsigned_abs()).unwrap_or(usize::MAX)
}

fn in_disc(dx: i64, dy: i64, ox: i64, oy: i64) -> bool {
	if ox == 0 && oy == 0 {
		return dx == 0 && dy == 0;
	}
	if ox == 0 {
		return dx == 0 && dy * dy <= oy * oy;
	}
	if oy == 0 {
		return dy == 0 && dx * dx <= ox * ox;
	}
	let (dx2, dy2) = (dx * dx, dy * dy);
	let (ox2, oy2) = (ox * ox, oy * oy);
	dx2 * oy2 + dy2 * ox2 <= ox2 * oy2
}

/// The largest `dx` with the offset `(dx, dy)` inside the disc of radii `(ox, oy)`; the disc is
/// symmetric, so the window of a row of the source is `[x - k, x + k]`.
fn disc_half_width(dy: i64, ox: i64, oy: i64) -> i64 {
	(0..=ox)
		.rev()
		.find(|&dx| in_disc(dx, dy, ox, oy))
		.unwrap_or(0)
}

/// The alpha of `src` dilated by the disc of radii `(ox, oy)`: each pixel of the result, in the
/// frame grown by the radii, is the largest source alpha inside the disc around it. One pass per
/// row of the disc takes the maximum over a window of that row of the source with a monotonic
/// queue, so the cost is the pixels times the height of the disc and not its area.
fn dilate_alpha(src: &Image, ox: usize, oy: usize) -> Image {
	let (w, h) = (src.w + 2 * ox, src.h + 2 * oy);
	let mut out = Image::new(w, h);
	let (oxi, oyi) = (ox as i64, oy as i64);
	let mut queue: std::collections::VecDeque<usize> = std::collections::VecDeque::new();
	let mut row = vec![0_u8; src.w];

	for dy in -oyi..=oyi {
		let k = disc_half_width(dy, oxi, oyi);
		for y in 0..h {
			let sy = y as i64 - oyi + dy;
			if sy < 0 || sy >= src.h as i64 {
				continue;
			}
			for (a, px) in row
				.iter_mut()
				.zip(src.pix[sy as usize * src.w * 4..].as_chunks::<4>().0)
			{
				*a = px[3];
			}
			queue.clear();
			let mut next = 0_usize; // the next source column to push
			for x in 0..w {
				let cx = x as i64 - oxi;
				let hi = (cx + k).min(src.w as i64 - 1);
				while (next as i64) <= hi {
					while queue.back().is_some_and(|&b| row[b] <= row[next]) {
						queue.pop_back();
					}
					queue.push_back(next);
					next += 1;
				}
				while queue.front().is_some_and(|&f| (f as i64) < cx - k) {
					queue.pop_front();
				}
				if let Some(&f) = queue.front() {
					let o = (y * w + x) * 4 + 3;
					out.pix[o] = out.pix[o].max(row[f]);
				}
			}
		}
	}
	out
}

fn shadow_silhouette(src: &Image, dilated: Option<&Image>, ox: usize, oy: usize) -> Image {
	if ox == 0 && oy == 0 {
		let mut pix = vec![0_u8; src.w * src.h * 4];
		for (i, px) in src.pix.as_chunks::<4>().0.iter().enumerate() {
			pix[i * 4 + 3] = px[3];
		}
		return Image {
			pix,
			w: src.w,
			h: src.h,
			opaque: false,
		};
	}
	let Some(dil) = dilated else {
		return shadow_silhouette(src, None, 0, 0);
	};
	let mut pix = vec![0_u8; dil.w * dil.h * 4];
	for y in 0..dil.h {
		for x in 0..dil.w {
			let ao = u32::from(dil.pix[(y * dil.w + x) * 4 + 3]);
			let (sx, sy) = (x as i64 - ox as i64, y as i64 - oy as i64);
			let ap = if sx >= 0 && sy >= 0 && (sx as usize) < src.w && (sy as usize) < src.h {
				u32::from(src.pix[(sy as usize * src.w + sx as usize) * 4 + 3])
			} else {
				0
			};
			pix[(y * dil.w + x) * 4 + 3] = (ap + div255(ao * (255 - ap))) as u8;
		}
	}
	Image {
		pix,
		w: dil.w,
		h: dil.h,
		opaque: false,
	}
}

fn paint_colour(dst: &mut Image, alpha: &Image, x0: i64, y0: i64, color: [u8; 4], factor: u32) {
	if factor == 0 {
		return;
	}
	for y in 0..alpha.h {
		let dy = y0 + y as i64;
		if dy < 0 || dy >= dst.h as i64 {
			continue;
		}
		for x in 0..alpha.w {
			let dx = x0 + x as i64;
			if dx < 0 || dx >= dst.w as i64 {
				continue;
			}
			let a = div255(u32::from(alpha.pix[(y * alpha.w + x) * 4 + 3]) * factor);
			if a == 0 {
				continue;
			}
			let o = (dy as usize * dst.w + dx as usize) * 4;
			over_pixel(
				&mut dst.pix[o..o + 4],
				div255(u32::from(color[0]) * a),
				div255(u32::from(color[1]) * a),
				div255(u32::from(color[2]) * a),
				a,
			);
		}
	}
}

fn paint_image(dst: &mut Image, src: &Image, x0: i64, y0: i64) {
	for y in 0..src.h {
		let dy = y0 + y as i64;
		if dy < 0 || dy >= dst.h as i64 {
			continue;
		}
		for x in 0..src.w {
			let dx = x0 + x as i64;
			if dx < 0 || dx >= dst.w as i64 {
				continue;
			}
			let s = &src.pix[(y * src.w + x) * 4..];
			let a = u32::from(s[3]);
			if a == 0 {
				continue;
			}
			let o = (dy as usize * dst.w + dx as usize) * 4;
			over_pixel(
				&mut dst.pix[o..o + 4],
				u32::from(s[0]),
				u32::from(s[1]),
				u32::from(s[2]),
				a,
			);
		}
	}
}

fn over_pixel(d: &mut [u8], r: u32, g: u32, b: u32, a: u32) {
	let ia = 255 - a;
	d[0] = u8_of(r + div255(u32::from(d[0]) * ia));
	d[1] = u8_of(g + div255(u32::from(d[1]) * ia));
	d[2] = u8_of(b + div255(u32::from(d[2]) * ia));
	d[3] = u8_of(a + div255(u32::from(d[3]) * ia));
}

/// The shadow and the outline of SDD-004 section 3.15. `None` when they change no pixel, so the
/// source is sampled as it was.
pub(crate) fn shadowed(
	src: &Image,
	p: &Placement,
	shadow: Option<&ShadowSpec>,
	outline: Option<&OutlineSpec>,
	out_scale: f64,
) -> Option<(Image, Placement)> {
	if shadow.is_none() && outline.is_none() || !(p.w > 0.0 && p.h > 0.0) || src.w < 1 || src.h < 1
	{
		return None;
	}
	let mut oc = [0_u8; 4];
	let (mut ox, mut oy) = (0_usize, 0_usize);
	if let Some(ol) = outline {
		oc = parse_rgba(&ol.color);
		if oc[3] != 0 {
			ox = src_len(ol.width, out_scale, src.w, p.w).min(MAX_OUTLINE_RADIUS);
			oy = src_len(ol.width, out_scale, src.h, p.h).min(MAX_OUTLINE_RADIUS);
		}
	}
	let mut sc = [0_u8; 4];
	let mut factor = 0_u32;
	let (mut dxs, mut dys) = (0_i64, 0_i64);
	let (mut rx, mut ry) = (0_usize, 0_usize);
	let sigma = shadow
		.map(|sh| mul(sh.blur, out_scale) / 2.0)
		.unwrap_or(0.0);
	if let Some(sh) = shadow {
		sc = parse_rgba(&sh.color);
		let op = round_half_away(sh.opacity.clamp(0.0, 1.0) * 255.0).clamp(0, 255) as u32;
		factor = div255(op * u32::from(sc[3]));
		if factor != 0 {
			dxs = src_len_signed(sh.dx, out_scale, src.w, p.w)
				.clamp(-MAX_SHADOW_OFFSET, MAX_SHADOW_OFFSET);
			dys = src_len_signed(sh.dy, out_scale, src.h, p.h)
				.clamp(-MAX_SHADOW_OFFSET, MAX_SHADOW_OFFSET);
			rx = blur_radius(sigma / (p.w / src.w as f64));
			ry = blur_radius(sigma / (p.h / src.h as f64));
		}
	}
	if factor == 0 && ox == 0 && oy == 0 {
		return None;
	}
	let (blur_pad_x, blur_pad_y) = (3 * rx, 3 * ry);
	let pad_x = ox + blur_pad_x + iabs(dxs);
	let pad_y = oy + blur_pad_y + iabs(dys);
	let (cw, ch) = (src.w + 2 * pad_x, src.h + 2 * pad_y);
	let mut out = Image::new(cw, ch);
	let dilated = (ox != 0 || oy != 0).then(|| dilate_alpha(src, ox, oy));
	if factor != 0 {
		let sil = shadow_silhouette(src, dilated.as_ref(), ox, oy);
		let (shadow_img, bpx, bpy) = if rx != 0 || ry != 0 {
			let sil_p = Placement {
				cx: 0.0,
				cy: 0.0,
				rot: 0.0,
				w: p.w * sil.w as f64 / src.w as f64,
				h: p.h * sil.h as f64 / src.h as f64,
			};
			let (img, _) = blurred(&sil, &sil_p, sigma).unwrap_or((sil.clone(), sil_p));
			let bpx = img.w.saturating_sub(sil.w) / 2;
			let bpy = img.h.saturating_sub(sil.h) / 2;
			(img, bpx, bpy)
		} else {
			(sil, 0, 0)
		};
		paint_colour(
			&mut out,
			&shadow_img,
			pad_x as i64 - ox as i64 - bpx as i64 + dxs,
			pad_y as i64 - oy as i64 - bpy as i64 + dys,
			sc,
			factor,
		);
	}
	if let Some(dil) = &dilated {
		paint_colour(
			&mut out,
			dil,
			pad_x as i64 - ox as i64,
			pad_y as i64 - oy as i64,
			oc,
			u32::from(oc[3]),
		);
	}
	paint_image(&mut out, src, pad_x as i64, pad_y as i64);
	Some((
		out,
		Placement {
			cx: p.cx,
			cy: p.cy,
			rot: p.rot,
			w: mul(p.w, cw as f64) / src.w as f64,
			h: mul(p.h, ch as f64) / src.h as f64,
		},
	))
}

fn draw_layer(dst: &mut Target<'_>, l: &Layer<'_>, out_scale: f64) {
	if let Some(p) = prepare_layer(l, out_scale) {
		draw(dst, &p.image, &p.placement, p.op, p.blend, p.mask.as_ref());
	}
}

/// A bitmap box-filtered down to its drawn size when it is at least twice as large in both
/// directions, so a small frame is not point-sampled from a large bitmap.
fn shrunk<'a>(src: &'a Image, is_bitmap: bool, p: &Placement) -> Cow<'a, Image> {
	if is_bitmap {
		let (w, h) = (round_half_up(p.w), round_half_up(p.h));
		if w >= 1 && h >= 1 {
			let (w, h) = (w as usize, h as usize);
			if src.w >= w.saturating_mul(2) && src.h >= h.saturating_mul(2) {
				return Cow::Owned(resize(src, w, h));
			}
		}
	}
	Cow::Borrowed(src)
}

/// The top-left corner of the one-to-one copy of SDD-013 section 3.6, when the placement is one:
/// no rotation and a rectangle that rounds to the size of the source.
pub fn copy_origin(src_w: usize, src_h: usize, p: &Placement) -> Option<(i64, i64)> {
	if p.rot == 0.0 && src_w as i64 == round_half_up(p.w) && src_h as i64 == round_half_up(p.h) {
		return Some((
			round_half_up(p.cx - p.w / 2.0),
			round_half_up(p.cy - p.h / 2.0),
		));
	}
	None
}

fn draw(
	dst: &mut Target<'_>,
	src: &Image,
	p: &Placement,
	op: u32,
	mode: BlendMode,
	mask: Option<&MaskState>,
) {
	match copy_origin(src.w, src.h, p) {
		Some((x0, y0)) => blit(dst, src, x0, y0, op, mode, mask),
		None => sample(dst, src, p, op, mode, mask),
	}
}

/// Draws `src` with its top-left corner at `(x0, y0)`, pixel for pixel.
fn blit(
	dst: &mut Target<'_>,
	src: &Image,
	x0: i64,
	y0: i64,
	op: u32,
	mode: BlendMode,
	mask: Option<&MaskState>,
) {
	let dw = dst.width;
	let cx0 = x0.max(0);
	let cx1 = x0.saturating_add(src.w as i64).min(dw as i64);
	let cy0 = y0.max(dst.rows.start as i64);
	let cy1 = y0.saturating_add(src.h as i64).min(dst.rows.end as i64);
	if cx0 >= cx1 || cy0 >= cy1 {
		return;
	}
	let n = (cx1 - cx0) as usize * 4;

	for y in cy0..cy1 {
		let so = ((y - y0) as usize * src.w + (cx0 - x0) as usize) * 4;
		let d = (y as usize * dw + cx0 as usize) * 4;
		if let Some(m) = mask {
			masked_row(
				&mut dst.pix[d..d + n],
				&src.pix[so..so + n],
				op,
				mode,
				m,
				cx0,
				y,
			);
		} else if src.opaque && op == 255 && mode == BlendMode::Normal {
			dst.pix[d..d + n].copy_from_slice(&src.pix[so..so + n]);
		} else {
			over_row(&mut dst.pix[d..d + n], &src.pix[so..so + n], op, mode);
		}
	}
}

/// Blends a row of premultiplied pixels over `d` with opacity `op` (0..255).
fn over_row(d: &mut [u8], s: &[u8], op: u32, mode: BlendMode) {
	for (dp, sp) in d
		.as_chunks_mut::<4>()
		.0
		.iter_mut()
		.zip(s.as_chunks::<4>().0)
	{
		let mut a = u32::from(sp[3]);
		if op == 255 && a == 255 && mode == BlendMode::Normal {
			dp.copy_from_slice(sp);
			continue;
		}
		if op != 255 {
			a = div255(a * op);
		}
		if a == 0 {
			continue;
		}
		let (mut r, mut g, mut b) = (u32::from(sp[0]), u32::from(sp[1]), u32::from(sp[2]));
		if op != 255 {
			r = div255(r * op);
			g = div255(g * op);
			b = div255(b * op);
		}
		mix_pixel(dp, r, g, b, a, mode);
	}
}

/// [`over_row`] for a masked layer: each pixel is multiplied by its coverage first, then by the
/// opacity, and a masked layer never takes the copy shortcut. `x` is the destination column of the
/// first pixel of the row.
fn masked_row(d: &mut [u8], s: &[u8], op: u32, mode: BlendMode, m: &MaskState, x: i64, y: i64) {
	for (i, (dp, sp)) in d
		.as_chunks_mut::<4>()
		.0
		.iter_mut()
		.zip(s.as_chunks::<4>().0)
		.enumerate()
	{
		let mut px = [
			u32::from(sp[0]),
			u32::from(sp[1]),
			u32::from(sp[2]),
			u32::from(sp[3]),
		];
		scale(&mut px, m.copy_cov(x + i as i64, y));
		if op != 255 {
			for v in &mut px {
				*v = div255(*v * op);
			}
		}
		if px[3] == 0 {
			continue;
		}
		mix_pixel(dp, px[0], px[1], px[2], px[3], mode);
	}
}

/// Fractional bits of the sampler coordinates.
const FIX: u32 = 32;

/// Draws `src` into the rectangle `p` with bilinear interpolation: every destination pixel whose
/// centre is inside the rectangle takes the four nearest source pixels. Edges are not smoothed.
/// Only the destination rows of `dst.rows` are drawn.
pub fn sample(
	dst: &mut Target<'_>,
	src: &Image,
	p: &Placement,
	op: u32,
	mode: BlendMode,
	mask: Option<&MaskState>,
) {
	let (sin, cos) = sincos(mul(p.rot, core::f64::consts::PI / 180.0));
	let dw = dst.width;

	let (hw, hh) = (p.w / 2.0, p.h / 2.0);
	let ex = mul(hw, cos.abs()) + mul(hh, sin.abs());
	let ey = mul(hw, sin.abs()) + mul(hh, cos.abs());
	let bx0 = ((p.cx - ex).floor() as i64).max(0);
	let bx1 = ((p.cx + ex).ceil() as i64).min(dw as i64);
	let by0 = ((p.cy - ey).floor() as i64)
		.max(0)
		.max(dst.rows.start as i64);
	let by1 = ((p.cy + ey).ceil() as i64).min(dst.rows.end as i64);
	if bx0 >= bx1 || by0 >= by1 {
		return;
	}

	// The inverse transform of a destination pixel centre to source coordinates, as 32.32 fixed
	// point: sx = ax*px + bx*py + ox, sy = ay*px + by*py + oy.
	let (kx, ky) = (src.w as f64 / p.w, src.h as f64 / p.h);
	let (dx0, dy0) = (0.5 - p.cx, 0.5 - p.cy);
	let one = (1_u64 << FIX) as f64;
	let q = |v: f64| round_half_away(mul(v, one));
	let (ax, bxx) = (q(mul(cos, kx)), q(mul(sin, kx)));
	let (ay, byy) = (q(-mul(sin, ky)), q(mul(cos, ky)));
	let ox = q(mul(mul(dx0, cos) + mul(dy0, sin) + hw, kx));
	let oy = q(mul(-mul(dx0, sin) + mul(dy0, cos) + hh, ky));

	let (lim_x, lim_y) = ((src.w as i64) << FIX, (src.h as i64) << FIX);
	let half = 1_i64 << (FIX - 1);
	let (max_x, max_y) = (src.w as i64 - 1, src.h as i64 - 1);
	let stride = src.w * 4;

	for y in by0..by1 {
		let row_x = bxx.wrapping_mul(y).wrapping_add(ox);
		let row_y = byy.wrapping_mul(y).wrapping_add(oy);
		let mut d = (y as usize * dw + bx0 as usize) * 4;
		for x in bx0..bx1 {
			let sx = row_x.wrapping_add(ax.wrapping_mul(x));
			let sy = row_y.wrapping_add(ay.wrapping_mul(x));
			let here = d;
			d += 4;
			if sx < 0 || sy < 0 || sx >= lim_x || sy >= lim_y {
				continue;
			}
			let (fx, fy) = (sx - half, sy - half);
			let (x0, y0) = (fx >> FIX, fy >> FIX);
			let (wx, wy) = (frac8(fx), frac8(fy));
			let (x1, y1) = ((x0 + 1).min(max_x) as usize, (y0 + 1).min(max_y) as usize);
			let (x0, y0) = (x0.max(0) as usize, y0.max(0) as usize);
			let o00 = y0 * stride + x0 * 4;
			let o10 = y0 * stride + x1 * 4;
			let o01 = y1 * stride + x0 * 4;
			let o11 = y1 * stride + x1 * 4;

			let mut px = [0_u32; 4];
			for (k, v) in px.iter_mut().enumerate() {
				let top =
					u32::from(src.pix[o00 + k]) * (256 - wx) + u32::from(src.pix[o10 + k]) * wx;
				let bot =
					u32::from(src.pix[o01 + k]) * (256 - wx) + u32::from(src.pix[o11 + k]) * wx;
				*v = (top * (256 - wy) + bot * wy + 32768) >> 16;
			}
			if let Some(m) = mask {
				scale(&mut px, m.sample_cov(x, y));
			}
			let mut a = px[3];
			if op != 255 {
				a = div255(a * op);
				px[0] = div255(px[0] * op);
				px[1] = div255(px[1] * op);
				px[2] = div255(px[2] * op);
			}
			if a == 0 {
				continue;
			}
			mix_pixel(&mut dst.pix[here..here + 4], px[0], px[1], px[2], a, mode);
		}
	}
}

/// The 8 bits of weight of a 32.32 coordinate, the 8 bits under the integer part.
fn frac8(v: i64) -> u32 {
	((v >> (FIX - 8)) & 0xFF) as u32
}

#[cfg(test)]
mod tests {
	use super::*;

	/// The definition of the dilation, one disc at a time.
	fn dilate_alpha_by_definition(src: &Image, ox: usize, oy: usize) -> Image {
		let (w, h) = (src.w + 2 * ox, src.h + 2 * oy);
		let mut out = Image::new(w, h);
		let (ox, oy) = (ox as i64, oy as i64);
		for y in 0..h {
			for x in 0..w {
				let (cx, cy) = (x as i64 - ox, y as i64 - oy);
				let mut max_a = 0_u8;
				for dy in -oy..=oy {
					for dx in -ox..=ox {
						if !in_disc(dx, dy, ox, oy) {
							continue;
						}
						let (sx, sy) = (cx + dx, cy + dy);
						if sx < 0 || sy < 0 || sx >= src.w as i64 || sy >= src.h as i64 {
							continue;
						}
						max_a = max_a.max(src.pix[(sy as usize * src.w + sx as usize) * 4 + 3]);
					}
				}
				out.pix[(y * w + x) * 4 + 3] = max_a;
			}
		}
		out
	}

	#[test]
	fn the_fast_dilation_is_the_definition() {
		let mut seed = 12345_u32;
		let mut next = move || {
			seed = seed.wrapping_mul(1_664_525).wrapping_add(1_013_904_223);
			(seed >> 16) as usize
		};
		for _ in 0..40 {
			let (w, h) = (1 + next() % 13, 1 + next() % 13);
			let mut src = Image::new(w, h);
			for px in src.pix.as_chunks_mut::<4>().0 {
				px[3] = if next() % 4 == 0 {
					(next() % 256) as u8
				} else {
					0
				};
			}
			let (ox, oy) = (next() % 6, next() % 6);
			assert_eq!(
				dilate_alpha(&src, ox, oy),
				dilate_alpha_by_definition(&src, ox, oy),
				"{w}x{h} radii {ox},{oy}"
			);
		}
	}
}
