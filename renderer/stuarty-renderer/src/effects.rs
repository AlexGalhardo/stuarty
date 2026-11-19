//! The pixel effects of SDD-004 section 3.13 and the whip blur and dip mix of a transition (section
//! 3.2), the port of `engine/compositor/effects.go`.
//!
//! Each works on the layer's own pixels, after the filter, the blur and the flip and before the
//! mask. All arithmetic is integer, so the bytes are those of the Go compositor. The wipe, which
//! needs the destination pixel, is in `mask.rs`.

use crate::blur::blurred;
use crate::compose::Placement;
use crate::image::Image;
use crate::math::{div255, mul, round_half_away, u8_of};

/// The largest length, in pixels of the layer's own bitmap, of a motion blur, an RGB split or a
/// whip blur: it bounds the memory a source drawn much smaller than it is stored can ask for.
const MAX_EFFECT_LEN: usize = 256;

/// `T` of section 3.13: `round_half_up(1024 * sin(d degrees))` for `d` from 0 to 90.
const SIN_TABLE: [i64; 91] = [
	0, 18, 36, 54, 71, 89, 107, 125, 143, 160, 178, 195, 213, 230, 248, 265, 282, 299, 316, 333,
	350, 367, 384, 400, 416, 433, 449, 465, 481, 496, 512, 527, 543, 558, 573, 587, 602, 616, 630,
	644, 658, 672, 685, 698, 711, 724, 737, 749, 761, 773, 784, 796, 807, 818, 828, 839, 849, 859,
	868, 878, 887, 896, 904, 912, 920, 928, 935, 943, 949, 956, 962, 968, 974, 979, 984, 989, 994,
	998, 1002, 1005, 1008, 1011, 1014, 1016, 1018, 1020, 1022, 1023, 1023, 1024, 1024,
];

/// One pixel effect with every parameter filled in (the plan carries them all).
#[derive(Clone, Copy, Debug, PartialEq)]
pub enum Effect {
	MotionBlur {
		amount: f64,
		angle: f64,
	},
	Glow {
		amount: f64,
		radius: f64,
		threshold: f64,
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
	RgbSplit {
		amount: f64,
		angle: f64,
	},
}

/// The kind of a transition that changes pixels.
#[derive(Clone, Copy, Debug, PartialEq, Eq)]
pub enum TransitionKind {
	Whip,
	Dip,
	Wipe,
}

/// The direction of a whip or a wipe.
#[derive(Clone, Copy, Debug, PartialEq, Eq)]
pub enum Direction {
	Left,
	Right,
	Up,
	Down,
}

/// The whip, dip or wipe in progress on a layer (`core/plan.TransitionSpec`).
#[derive(Clone, Copy, Debug, PartialEq, Eq)]
pub struct Transition {
	pub kind: TransitionKind,
	/// The progress, 16.16 fixed point.
	pub p: i64,
	pub direction: Direction,
	/// The colour of a dip, `[r, g, b, 255]`.
	pub colour: [u8; 4],
}

/// What a layer needs to know of its frame: the plan's frame number and the project's size.
#[derive(Clone, Copy, Debug, Default, PartialEq, Eq)]
pub struct FrameInfo {
	pub frame: i64,
	pub width: i64,
	pub height: i64,
}

/// The cosine and the sine, times 1024, of an angle in degrees (clockwise on the screen).
fn angle_cs(deg: f64) -> (i64, i64) {
	let a = round_half_away(deg).clamp(-180, 180);
	let q = a.abs() % 180;
	let u = if q > 90 { 180 - q } else { q };
	let (mut s, mut c) = (SIN_TABLE[u as usize], SIN_TABLE[(90 - u) as usize]);
	if a < 0 {
		s = -s;
	}
	if a.abs() > 90 {
		c = -c;
	}
	(c, s)
}

/// `v / 1024` rounded half away from zero.
fn round_div_1024(v: i64) -> i64 {
	if v < 0 {
		-((-v + 512) >> 10)
	} else {
		(v + 512) >> 10
	}
}

/// The shift by `k` along the angle with cosine `c` and sine `s` (times 1024).
fn shift_along(k: i64, c: i64, s: i64) -> (i64, i64) {
	(round_div_1024(k * c), round_div_1024(k * s))
}

/// A parameter held to its range; NaN is the low end. A plan that core accepts is always inside it.
fn held(v: f64, lo: f64, hi: f64) -> f64 {
	if v.is_nan() { lo } else { v.clamp(lo, hi) }
}

/// A parameter as the whole number of thousandths, rounded half away from zero.
fn permille(x: f64) -> i64 {
	round_half_away(mul(x, 1000.0))
}

/// The frame size a whip reads is held to what its arithmetic takes.
const MAX_FRAME_SIDE: i64 = 1 << 20;

/// `a / b` rounded down, for `b > 0`.
fn floor_div(a: i64, b: i64) -> i64 {
	a.div_euclid(b)
}

/// Places the pixels of the layer's current image in the layer's own rectangle: the image started
/// as `sw` by `sh` pixels filling a rectangle of `wq` by `hq` (24.8), and grown by padding it has
/// its first pixel at `(offx, offy)`.
#[derive(Clone, Copy, Debug)]
pub struct EffectGeom {
	pub sw: i64,
	pub sh: i64,
	pub offx: i64,
	pub offy: i64,
	pub wq: i64,
	pub hq: i64,
}

impl EffectGeom {
	/// The position of the centre of column `x` from the left of the rectangle, in 24.8.
	fn lx(&self, x: i64) -> i64 {
		(2 * (x - self.offx) + 1) * self.wq / (2 * self.sw)
	}

	/// The position of the centre of row `y` from the top of the rectangle, in 24.8.
	fn ly(&self, y: i64) -> i64 {
		(2 * (y - self.offy) + 1) * self.hq / (2 * self.sh)
	}
}

struct Ctx {
	out_scale: f64,
	/// The scale at which the layer's bitmap is drawn, in output pixels per bitmap pixel.
	scale: f64,
	frame: i64,
	geom: EffectGeom,
}

impl Ctx {
	/// A length in frame pixels at project resolution as a length in pixels of the layer's bitmap:
	/// times the output scale, divided by the scale of the bitmap, rounded half up.
	fn px(&self, length: f64) -> usize {
		let v = (mul(length, self.out_scale) / self.scale + 0.5).floor();
		if v.is_nan() || v <= 0.0 {
			0
		} else {
			v.min(MAX_EFFECT_LEN as f64) as usize
		}
	}

	fn pad(&mut self, px: usize, py: usize) {
		self.geom.offx += px as i64;
		self.geom.offy += py as i64;
	}
}

/// The placement of an image that padding made `w` by `h` from one of `sw` by `sh`.
fn grown(p: &Placement, sw: usize, sh: usize, w: usize, h: usize) -> Placement {
	Placement {
		w: mul(p.w, w as f64) / sw as f64,
		h: mul(p.h, h as f64) / sh as f64,
		..*p
	}
}

/// Draws the pixel effects of a layer, then the whip blur and the dip of its transition, on `src`.
/// Returns the image and the placement of the image after the effects that pad it.
pub fn apply_effects(
	src: &Image,
	p: &Placement,
	pre: EffectGeom,
	effects: &[Effect],
	transition: Option<&Transition>,
	info: &FrameInfo,
	out_scale: f64,
) -> Option<(Image, Placement)> {
	let pixel_transition = transition.filter(|t| t.kind != TransitionKind::Wipe);
	if effects.is_empty() && pixel_transition.is_none() {
		return None;
	}

	let mut e = Ctx {
		out_scale,
		scale: p.w / src.w as f64,
		frame: info.frame,
		geom: pre,
	};
	e.geom.offx = (src.w as i64 - pre.sw) / 2;
	e.geom.offy = (src.h as i64 - pre.sh) / 2;

	let mut img = src.clone();
	let mut p = *p;
	for ef in effects {
		match *ef {
			Effect::MotionBlur { amount, angle } => {
				let (c, s) = angle_cs(held(angle, -180.0, 180.0));
				let n = e.px((permille(held(amount, 0.0, 1.0)) * 60) as f64 / 1000.0);
				(img, p) = motion_blur(&mut e, img, p, n, c, s);
			}
			Effect::Glow {
				amount,
				radius,
				threshold,
			} => {
				(img, p) = glow(
					&mut e,
					img,
					p,
					held(amount, 0.0, 1.0),
					held(radius, 1.0, 100.0),
					held(threshold, 0.0, 1.0),
				);
			}
			Effect::Vignette { amount, softness } => {
				img = vignette(&e, &img, held(amount, 0.0, 1.0), held(softness, 0.0, 1.0));
			}
			Effect::Grain { amount, size, seed } => {
				img = grain(
					&e,
					&img,
					held(amount, 0.0, 1.0),
					held(size, 1.0, 8.0),
					seed.clamp(0, 65535),
				);
			}
			Effect::RgbSplit { amount, angle } => {
				let (c, s) = angle_cs(held(angle, -180.0, 180.0));
				let d = e.px((permille(held(amount, 0.0, 1.0)) * 24) as f64 / 1000.0);
				(img, p) = rgb_split(&mut e, img, p, d, c, s);
			}
		}
	}

	match pixel_transition {
		Some(t) if t.kind == TransitionKind::Whip => {
			let (size, c, s) = match t.direction {
				Direction::Up | Direction::Down => (info.height, 0, 1024),
				_ => (info.width, 1024, 0),
			};
			let size = size.clamp(0, MAX_FRAME_SIDE);
			let n = ((((65536 - t.p.clamp(0, 65536)) * size) >> 4) + 32768) >> 16;
			let n = e.px(n as f64);
			(img, p) = motion_blur(&mut e, img, p, n, c, s);
		}
		Some(t) => img = dip_mix(&img, t.p.clamp(0, 65536), t.colour),
		None => {}
	}
	Some((img, p))
}

/// The directional box blur: the mean of the `2h + 1` shifts of the pixel along the angle, `h = n / 2`,
/// over a layer padded by `h` on every side. `n` under 2 changes nothing.
fn motion_blur(
	e: &mut Ctx,
	src: Image,
	p: Placement,
	n: usize,
	c: i64,
	s: i64,
) -> (Image, Placement) {
	if n < 2 {
		return (src, p);
	}
	let h = n / 2;
	let taps = 2 * h + 1;
	let shifts: Vec<(i64, i64)> = (0..taps)
		.map(|i| shift_along(i as i64 - h as i64, c, s))
		.collect();

	let (w, ht) = (src.w + 2 * h, src.h + 2 * h);
	let mut out = vec![0_u8; w * ht * 4];
	let half = (taps / 2) as u32;
	let div = taps as u32;
	for y in 0..ht {
		for x in 0..w {
			let mut sum = [0_u32; 4];
			for &(dx, dy) in &shifts {
				let (sx, sy) = (x as i64 - h as i64 + dx, y as i64 - h as i64 + dy);
				if sx < 0 || sy < 0 || sx >= src.w as i64 || sy >= src.h as i64 {
					continue;
				}
				let o = (sy as usize * src.w + sx as usize) * 4;
				for k in 0..4 {
					sum[k] += u32::from(src.pix[o + k]);
				}
			}
			let o = (y * w + x) * 4;
			for k in 0..4 {
				out[o + k] = ((sum[k] + half) / div) as u8;
			}
		}
	}
	e.pad(h, h);
	let placement = grown(&p, src.w, src.h, w, ht);
	(
		Image {
			pix: out,
			w,
			h: ht,
			opaque: false,
		},
		placement,
	)
}

/// Moves the red channel by `d` along the angle one way and the blue the other, over a layer padded by
/// the shift; the alpha is the largest of the three.
fn rgb_split(
	e: &mut Ctx,
	src: Image,
	p: Placement,
	d: usize,
	c: i64,
	s: i64,
) -> (Image, Placement) {
	let (dx, dy) = shift_along(d as i64, c, s);
	if dx == 0 && dy == 0 {
		return (src, p);
	}
	let (ax, ay) = (dx.unsigned_abs() as usize, dy.unsigned_abs() as usize);
	let (w, h) = (src.w + 2 * ax, src.h + 2 * ay);
	let mut out = vec![0_u8; w * h * 4];

	let at = |x: i64, y: i64| -> [u8; 4] {
		if x < 0 || y < 0 || x >= src.w as i64 || y >= src.h as i64 {
			return [0; 4];
		}
		let o = (y as usize * src.w + x as usize) * 4;
		[src.pix[o], src.pix[o + 1], src.pix[o + 2], src.pix[o + 3]]
	};
	for y in 0..h {
		for x in 0..w {
			let (sx, sy) = (x as i64 - ax as i64, y as i64 - ay as i64);
			let (r, g, b) = (at(sx - dx, sy - dy), at(sx, sy), at(sx + dx, sy + dy));
			let o = (y * w + x) * 4;
			out[o] = r[0];
			out[o + 1] = g[1];
			out[o + 2] = b[2];
			out[o + 3] = r[3].max(g[3]).max(b[3]);
		}
	}
	e.pad(ax, ay);
	let placement = grown(&p, src.w, src.h, w, h);
	(
		Image {
			pix: out,
			w,
			h,
			opaque: false,
		},
		placement,
	)
}

/// Adds the blurred bright part of the layer back to it.
fn glow(
	e: &mut Ctx,
	src: Image,
	p: Placement,
	amount: f64,
	radius: f64,
	threshold: f64,
) -> (Image, Placement) {
	let aq = round_half_away(mul(amount, 255.0)) as u32;
	let t = round_half_away(mul(threshold, 255.0));
	let sigma = mul(radius, e.out_scale);

	let mut bright = Image::new(src.w, src.h);
	for (b, s) in bright
		.pix
		.as_chunks_mut::<4>()
		.0
		.iter_mut()
		.zip(src.pix.as_chunks::<4>().0)
	{
		let (r, g, bl, a) = (
			i64::from(s[0]),
			i64::from(s[1]),
			i64::from(s[2]),
			i64::from(s[3]),
		);
		let y = (54 * r + 183 * g + 19 * bl + 128) >> 8;
		if t >= 255 || y <= t {
			continue;
		}
		let k = (((y - t) * 255 + (255 - t) / 2) / (255 - t)).min(255) as u32;
		b[0] = u8_of(div255(r as u32 * k));
		b[1] = u8_of(div255(g as u32 * k));
		b[2] = u8_of(div255(bl as u32 * k));
		b[3] = u8_of(div255(a as u32 * k));
	}

	let (blurred_img, gp) = match blurred(&bright, &p, sigma) {
		Some(v) => v,
		None => (bright, p),
	};
	let (ox, oy) = ((blurred_img.w - src.w) / 2, (blurred_img.h - src.h) / 2);
	let mut out = vec![0_u8; blurred_img.pix.len()];
	for y in 0..blurred_img.h {
		for x in 0..blurred_img.w {
			let o = (y * blurred_img.w + x) * 4;
			let inside = x >= ox && y >= oy && x - ox < src.w && y - oy < src.h;
			for k in 0..4 {
				let v = if inside {
					u32::from(src.pix[((y - oy) * src.w + (x - ox)) * 4 + k])
				} else {
					0
				};
				out[o + k] = u8_of(v + div255(u32::from(blurred_img.pix[o + k]) * aq));
			}
		}
	}
	e.pad(ox, oy);
	(
		Image {
			pix: out,
			w: blurred_img.w,
			h: blurred_img.h,
			opaque: false,
		},
		gp,
	)
}

/// Darkens the colour channels towards the edges and corners of the layer's rectangle.
fn vignette(e: &Ctx, src: &Image, amount: f64, softness: f64) -> Image {
	const R1: i64 = 92682;
	let amq = round_half_away(mul(amount, 255.0));
	let r0 = R1 - (R1 * permille(softness) + 500) / 1000;
	let g = &e.geom;
	let (hw, hh) = ((g.wq / 2).max(1), (g.hq / 2).max(1));

	let nx: Vec<i64> = (0..src.w as i64)
		.map(|x| (((g.lx(x) - g.wq / 2).abs() << 16) / hw).min(1 << 22))
		.collect();
	let mut out = Image {
		pix: vec![0; src.pix.len()],
		w: src.w,
		h: src.h,
		opaque: src.opaque,
	};
	for y in 0..src.h {
		let ny = (((g.ly(y as i64) - g.hq / 2).abs() << 16) / hh).min(1 << 22);
		for (x, &nxx) in nx.iter().enumerate() {
			let r = crate::mask::isqrt((nxx * nxx + ny * ny) as u64) as i64;
			let f = if r >= R1 {
				65536
			} else if r <= r0 {
				0
			} else {
				(r - r0) * 65536 / (R1 - r0)
			};
			let f2 = (f * f + 32768) >> 16;
			let m = (255 - ((f2 * amq + 32768) >> 16)) as u32;
			let o = (y * src.w + x) * 4;
			out.pix[o] = u8_of(div255(u32::from(src.pix[o]) * m));
			out.pix[o + 1] = u8_of(div255(u32::from(src.pix[o + 1]) * m));
			out.pix[o + 2] = u8_of(div255(u32::from(src.pix[o + 2]) * m));
			out.pix[o + 3] = src.pix[o + 3];
		}
	}
	out
}

/// `hash3` of section 3.13 on `u32` with wrapping arithmetic.
pub fn hash3(a: u32, b: u32, c: u32) -> u32 {
	fn mix(mut x: u32) -> u32 {
		x ^= x >> 16;
		x = x.wrapping_mul(0x7feb_352d);
		x ^= x >> 15;
		x = x.wrapping_mul(0x846c_a68b);
		x ^ (x >> 16)
	}
	mix(a.wrapping_add(mix(b.wrapping_add(mix(c.wrapping_add(0x9e37_79b9))))))
}

/// Adds the same hashed noise to the three colour channels, a new pattern every frame.
fn grain(e: &Ctx, src: &Image, amount: f64, size: f64, seed: i64) -> Image {
	let am = permille(amount);
	let sz = ((mul(size, e.out_scale) + 0.5).floor() as i64).max(1);
	let amq = (am * 64 + 500) / 1000;
	let frame = e.frame as u32;

	let cxs: Vec<i64> = (0..src.w as i64)
		.map(|x| floor_div(e.geom.lx(x) >> 8, sz))
		.collect();
	let mut out = Image {
		pix: vec![0; src.pix.len()],
		w: src.w,
		h: src.h,
		opaque: src.opaque,
	};
	for y in 0..src.h {
		let cy = floor_div(e.geom.ly(y as i64) >> 8, sz) as u32;
		for (x, &cx) in cxs.iter().enumerate() {
			let o = (y * src.w + x) * 4;
			let a = i64::from(src.pix[o + 3]);
			let v = hash3((cx + (seed << 16)) as u32, cy, frame) >> 24;
			let delta = ((i64::from(v) - 128) * amq + 64) >> 7;
			let dl = match delta {
				d if d > 0 => (d * a + 127) / 255,
				d if d < 0 => -((-d * a + 127) / 255),
				_ => 0,
			};
			for k in 0..3 {
				out.pix[o + k] = (i64::from(src.pix[o + k]) + dl).clamp(0, a) as u8;
			}
			out.pix[o + 3] = src.pix[o + 3];
		}
	}
	out
}

/// Mixes the layer with the solid colour `col`: its own pixels at the progress `progress` (16.16),
/// the colour, inside the layer's own shape, at the rest.
pub fn dip_mix(src: &Image, progress: i64, col: [u8; 4]) -> Image {
	let pq = ((progress * 255 + 32768) >> 16) as u32;
	let mut out = Image {
		pix: vec![0; src.pix.len()],
		w: src.w,
		h: src.h,
		opaque: src.opaque,
	};
	for (o, s) in out
		.pix
		.as_chunks_mut::<4>()
		.0
		.iter_mut()
		.zip(src.pix.as_chunks::<4>().0)
	{
		let a = u32::from(s[3]);
		for k in 0..3 {
			o[k] = u8_of(
				div255(u32::from(s[k]) * pq) + div255(div255(u32::from(col[k]) * a) * (255 - pq)),
			);
		}
		o[3] = s[3];
	}
	out
}

#[cfg(test)]
mod tests {
	use super::*;

	#[test]
	fn the_angle_table_gives_the_documented_cosines_and_sines() {
		for (deg, c, s) in [
			(0.0, 1024, 0),
			(90.0, 0, 1024),
			(-90.0, 0, -1024),
			(180.0, -1024, 0),
			(-180.0, -1024, 0),
			(45.0, 724, 724),
			(-45.0, 724, -724),
			(135.0, -724, 724),
			(-135.0, -724, -724),
			(30.0, 887, 512),
			(30.4, 887, 512),
			(120.0, -512, 887),
		] {
			assert_eq!(angle_cs(deg), (c, s), "{deg}");
		}
	}

	#[test]
	fn hash3_has_the_pinned_values() {
		assert_eq!(hash3(0, 0, 0), 2_488_251_732);
		assert_eq!(hash3(1, 2, 3), 3_114_175_480);
		assert_eq!(hash3(65535, 100, 2), 3_848_593_025);
	}

	#[test]
	fn a_dip_ends_at_the_layer_and_at_the_colour() {
		let img = Image {
			pix: vec![200, 100, 50, 255, 100, 50, 25, 128, 0, 0, 0, 0],
			w: 3,
			h: 1,
			opaque: false,
		};
		let col = [10, 20, 30, 255];
		assert_eq!(dip_mix(&img, 65535, col).pix, img.pix);
		let got = dip_mix(&img, 1, col);
		assert_eq!(got.pix, [10, 20, 30, 255, 5, 10, 15, 128, 0, 0, 0, 0]);
	}
}
