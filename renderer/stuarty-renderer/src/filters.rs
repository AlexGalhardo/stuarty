//! The ordered pixel filters of an image layer (SDD-013 section 3.11.3).
//!
//! Each filter works on the layer's own premultiplied pixels and may pad them, like the blur: the
//! placement grows by the padding around the same centre. They run in the order of the plan, so a
//! shadow before a blur is blurred with the picture.

use crate::blur::{blur_radius, blurred, blurred_replicate};
use crate::colour::apply_matrix;
use crate::compose::{OutlineSpec, Placement, ShadowSpec, parse_rgba, shadowed};
use crate::effects::{Effect, EffectGeom, FrameInfo, apply_effects};
use crate::image::Image;
use crate::math::{div255, mul, round_half_away, u8_of};

/// One filter of the plan, with every member filled in.
#[derive(Clone, Debug, PartialEq)]
pub enum FilterOp {
	/// A colour filter: the 3x4 matrix of section 3.4.
	Matrix([f64; 12]),
	/// The Gaussian blur of section 3.5; `sigma` is in project pixels.
	Blur {
		sigma: f64,
	},
	/// The unsharp mask: `amount` 0 to 3, `radius` the standard deviation in project pixels.
	Sharpen {
		amount: f64,
		radius: f64,
	},
	Vignette {
		amount: f64,
		softness: f64,
	},
	Grain {
		amount: f64,
		size: f64,
		seed: i64,
	},
	Shadow(ShadowSpec),
	Stroke(OutlineSpec),
	/// The outer glow: `color`, the standard deviation `radius` and the strength `amount` 0 to 4.
	OuterGlow {
		color: String,
		radius: f64,
		amount: f64,
	},
}

/// Draws the filters of a layer in order. `pre` is the geometry of the layer before any of its
/// padding (the effects read the position of a pixel in the layer's rectangle from it).
pub fn apply_filters(
	src: Image,
	p: Placement,
	pre: EffectGeom,
	filters: &[FilterOp],
	info: &FrameInfo,
	out_scale: f64,
) -> (Image, Placement) {
	let (mut img, mut p) = (src, p);
	for f in filters {
		match f {
			FilterOp::Matrix(m) => img = apply_matrix(&img, m),
			FilterOp::Blur { sigma } => {
				if let Some((b, g)) = blurred(&img, &p, mul(*sigma, out_scale)) {
					(img, p) = (b, g);
				}
			}
			FilterOp::Sharpen { amount, radius } => {
				if let Some(s) = sharpen(&img, &p, *amount, *radius, out_scale) {
					img = s;
				}
			}
			FilterOp::Vignette { amount, softness } => {
				let e = [Effect::Vignette {
					amount: *amount,
					softness: *softness,
				}];
				if let Some((b, g)) = apply_effects(&img, &p, pre, &e, None, info, out_scale) {
					(img, p) = (b, g);
				}
			}
			FilterOp::Grain { amount, size, seed } => {
				let e = [Effect::Grain {
					amount: *amount,
					size: *size,
					seed: *seed,
				}];
				if let Some((b, g)) = apply_effects(&img, &p, pre, &e, None, info, out_scale) {
					(img, p) = (b, g);
				}
			}
			FilterOp::Shadow(s) => {
				if let Some((b, g)) = shadowed(&img, &p, Some(s), None, out_scale) {
					(img, p) = (b, g);
				}
			}
			FilterOp::Stroke(o) => {
				if let Some((b, g)) = shadowed(&img, &p, None, Some(o), out_scale) {
					(img, p) = (b, g);
				}
			}
			FilterOp::OuterGlow {
				color,
				radius,
				amount,
			} => {
				if let Some((b, g)) = outer_glow(&img, &p, color, *radius, *amount, out_scale) {
					(img, p) = (b, g);
				}
			}
		}
	}
	(img, p)
}

/// The unsharp mask: each colour channel moves away from its blurred value by `amount`, held to
/// 0 and the pixel's alpha, which stays. `None` when the radius rounds to no blur.
fn sharpen(src: &Image, p: &Placement, amount: f64, radius: f64, out_scale: f64) -> Option<Image> {
	let sigma = mul(held(radius, 0.0, 100.0), out_scale);
	let rx = blur_radius(sigma / (p.w / src.w as f64));
	let ry = blur_radius(sigma / (p.h / src.h as f64));
	if rx == 0 && ry == 0 {
		return None;
	}
	let soft = blurred_replicate(src, rx, ry);
	let ap = round_half_away(mul(held(amount, 0.0, 3.0), 1000.0));
	let mut out = Image {
		pix: vec![0; src.pix.len()],
		w: src.w,
		h: src.h,
		opaque: src.opaque,
	};
	for ((o, s), u) in out
		.pix
		.as_chunks_mut::<4>()
		.0
		.iter_mut()
		.zip(src.pix.as_chunks::<4>().0)
		.zip(soft.pix.as_chunks::<4>().0)
	{
		let a = i64::from(s[3]);
		for k in 0..3 {
			let (c, b) = (i64::from(s[k]), i64::from(u[k]));
			let delta = (c - b) * ap;
			let q = if delta >= 0 {
				(delta + 500) / 1000
			} else {
				-((-delta + 500) / 1000)
			};
			o[k] = (c + q).clamp(0, a) as u8;
		}
		o[3] = s[3];
	}
	Some(out)
}

/// The outer glow: the blurred silhouette of the layer, painted in `color` with the strength
/// `amount`, under the layer. `None` when the radius rounds to no blur.
fn outer_glow(
	src: &Image,
	p: &Placement,
	color: &str,
	radius: f64,
	amount: f64,
	out_scale: f64,
) -> Option<(Image, Placement)> {
	let mut sil = Image::new(src.w, src.h);
	for (d, s) in sil
		.pix
		.as_chunks_mut::<4>()
		.0
		.iter_mut()
		.zip(src.pix.as_chunks::<4>().0)
	{
		d[3] = s[3];
	}
	let sigma = mul(held(radius, 0.0, 100.0), out_scale);
	let (soft, grown) = blurred(&sil, p, sigma)?;
	let (ox, oy) = ((soft.w - src.w) / 2, (soft.h - src.h) / 2);
	let col = parse_rgba(color);
	let ag = round_half_away(mul(held(amount, 0.0, 4.0), 1000.0)) as u32;

	let mut out = vec![0_u8; soft.pix.len()];
	for y in 0..soft.h {
		for x in 0..soft.w {
			let o = (y * soft.w + x) * 4;
			let g = u32::from(soft.pix[o + 3]);
			let ga = div255(((g * ag + 500) / 1000).min(255) * u32::from(col[3]));
			let inside = x >= ox && y >= oy && x - ox < src.w && y - oy < src.h;
			let pic = if inside {
				let so = ((y - oy) * src.w + (x - ox)) * 4;
				[
					u32::from(src.pix[so]),
					u32::from(src.pix[so + 1]),
					u32::from(src.pix[so + 2]),
					u32::from(src.pix[so + 3]),
				]
			} else {
				[0; 4]
			};
			let ia = 255 - pic[3];
			for k in 0..3 {
				let glow = div255(u32::from(col[k]) * ga);
				out[o + k] = u8_of(pic[k] + div255(glow * ia));
			}
			out[o + 3] = u8_of(pic[3] + div255(ga * ia));
		}
	}
	Some((
		Image {
			pix: out,
			w: soft.w,
			h: soft.h,
			opaque: false,
		},
		grown,
	))
}

/// A parameter held to its range; NaN is the low end.
fn held(v: f64, lo: f64, hi: f64) -> f64 {
	if v.is_nan() { lo } else { v.clamp(lo, hi) }
}
