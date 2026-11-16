//! The CPU path of the renderer: the port of `engine/compositor` (SDD-013 section 3).
//!
//! Every pixel is computed with integer arithmetic from numbers that are rounded once per layer,
//! so the result is the same on every system and does not depend on how many bands the frame is
//! split into: the result of a row never depends on another row.

use core::ops::Range;
use std::borrow::Cow;

use crate::blend::{BlendMode, mix_pixel};
use crate::blur::blurred;
use crate::chroma::{ChromaKeySpec, apply_chroma_key};
use crate::colour::apply_matrix;
use crate::effects::{Effect, EffectGeom, FrameInfo, Transition, TransitionKind, apply_effects};
use crate::image::Image;
use crate::mask::{MaskSpec, MaskState, clamped_q, scale};
use crate::math::{div255, mul, round_half_away, round_half_up, sincos, u8_of};
use crate::resize::resize;

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
}

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

/// Reads `#RRGGBB`; an alpha, if any, is ignored (the frame is opaque). Anything else is black.
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
/// `dst` end opaque.
pub fn compose(dst: &mut Target<'_>, background: [u8; 4], layers: &[Layer<'_>], out_scale: f64) {
	dst.rows = dst.rows.start.min(dst.height)..dst.rows.end.min(dst.height);
	if dst.pix.len() < dst.width * dst.height * 4 || dst.rows.is_empty() {
		return;
	}
	let (a, b) = (dst.rows.start * dst.width * 4, dst.rows.end * dst.width * 4);
	for px in dst.pix[a..b].as_chunks_mut::<4>().0.iter_mut() {
		px.copy_from_slice(&background);
	}
	for l in layers {
		draw_layer(dst, l, out_scale);
	}
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
	let op = layer_opacity(l.opacity);
	if op == 0 {
		return None;
	}

	let (image, p, is_bitmap) = match l.source {
		Source::Media(img) => {
			let p = Placement {
				cx: l.cx * out_scale,
				cy: l.cy * out_scale,
				w: l.w * out_scale,
				h: l.h * out_scale,
				rot: l.rotation,
			};
			(img, p, false)
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
			let p = Placement {
				cx: (l.cx - mul(center_dx, sc)) * out_scale,
				cy: (l.cy - mul(center_dy, sc)) * out_scale,
				w: l.w * out_scale,
				h: l.h * out_scale,
				rot: l.rotation,
			};
			(image, p, true)
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
	if let Some(m) = &l.color_matrix {
		src = Cow::Owned(apply_matrix(&src, m));
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
	Some(Prepared {
		image: src,
		placement: p,
		op,
		blend: l.blend,
		mask,
	})
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
