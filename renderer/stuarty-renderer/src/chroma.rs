//! The chroma key of SDD-004 section 3.7, the port of `engine/compositor/chroma.go`.

use crate::compose::parse_colour;
use crate::image::Image;
use crate::math::{div255_i64, mul, round_half_away};

/// The chroma distance that a similarity or a smoothness of 1 stands for, in 1/256 of an 8-bit
/// chroma unit: 128 units.
const CHROMA_SCALE: f64 = (128 * 256) as f64;

/// A key as the plan carries it: the colour as `#RRGGBB` or `#RRGGBBAA`, three settings from 0 to 1.
#[derive(Clone, Debug, PartialEq)]
pub struct ChromaKeySpec {
	pub color: String,
	pub similarity: f64,
	pub smoothness: f64,
	pub spill: f64,
}

/// A key reduced to integers once per layer.
struct Key {
	cb: i64,
	cr: i64,
	t0: i64,
	t1: i64,
	spill: i64,
	/// The channel of the key colour strictly above both others (0 red, 1 green, 2 blue).
	dominant: Option<usize>,
}

impl Key {
	fn new(spec: &ChromaKeySpec) -> Self {
		let c = parse_colour(&spec.color);
		let unit = |v: f64| v.clamp(0.0, 1.0);
		let (cb, cr) = chroma(i64::from(c[0]), i64::from(c[1]), i64::from(c[2]));
		let mut dominant = None;
		for ch in 0..3 {
			if c[ch] > c[(ch + 1) % 3] && c[ch] > c[(ch + 2) % 3] {
				dominant = Some(ch);
			}
		}
		Self {
			cb,
			cr,
			t0: round_half_away(mul(unit(spec.similarity), CHROMA_SCALE)),
			t1: round_half_away(mul(unit(spec.smoothness), CHROMA_SCALE)),
			spill: round_half_away(mul(unit(spec.spill), 65536.0)),
			dominant,
		}
	}

	/// The opacity, 0 to 255, that a pixel of straight colour keeps.
	fn keep(&self, r: i64, g: i64, b: i64) -> i64 {
		let (cb, cr) = chroma(r, g, b);
		let (dcb, dcr) = (cb - self.cb, cr - self.cr);
		let d = isqrt((dcb * dcb + dcr * dcr) as u64) as i64;
		if d <= self.t0 {
			0
		} else if d >= self.t0 + self.t1 {
			255
		} else {
			((d - self.t0) * 255 + self.t1 / 2) / self.t1
		}
	}

	/// Pulls the dominant channel of the key colour towards the mean of the two others.
	fn despill(&self, c: &mut [i64; 3]) {
		let Some(d) = self.dominant else { return };
		if self.spill == 0 {
			return;
		}
		let limit = (c[(d + 1) % 3] + c[(d + 2) % 3]) / 2;
		let excess = c[d] - limit;
		if excess > 0 {
			c[d] -= (excess * self.spill + 32768) >> 16;
		}
	}
}

/// The BT.601 Cb and Cr of a colour, in 1/256 of a unit.
fn chroma(r: i64, g: i64, b: i64) -> (i64, i64) {
	(-43 * r - 85 * g + 128 * b, 128 * r - 107 * g - 21 * b)
}

/// The largest `x` with `x * x <= n`.
fn isqrt(n: u64) -> u64 {
	if n < 2 {
		return n;
	}
	let mut x = 1_u64 << ((64 - n.leading_zeros()).div_ceil(2));
	loop {
		let y = (x + n / x) / 2;
		if y >= x {
			return x;
		}
		x = y;
	}
}

/// `src` with the key applied to its own premultiplied pixels: alpha goes down for the pixels near
/// the key colour and the key colour is taken out of the others. The result is never opaque.
pub fn apply_chroma_key(src: &Image, spec: &ChromaKeySpec) -> Image {
	let key = Key::new(spec);
	let mut out = Image {
		pix: vec![0; src.w * src.h * 4],
		w: src.w,
		h: src.h,
		opaque: false,
	};

	for (o, s) in out
		.pix
		.as_chunks_mut::<4>()
		.0
		.iter_mut()
		.zip(src.pix.as_chunks::<4>().0)
	{
		let a = i64::from(s[3]);
		if a == 0 {
			o.copy_from_slice(s);
			continue;
		}
		let mut px = [i64::from(s[0]), i64::from(s[1]), i64::from(s[2])];
		if a != 255 {
			for c in &mut px {
				*c = ((*c * 255 + a / 2) / a).min(255);
			}
		}
		let f = key.keep(px[0], px[1], px[2]);
		if f == 0 {
			*o = [0; 4];
			continue;
		}
		key.despill(&mut px);
		let na = div255_i64(a * f);
		o[0] = div255_i64(px[0] * na).min(255) as u8;
		o[1] = div255_i64(px[1] * na).min(255) as u8;
		o[2] = div255_i64(px[2] * na).min(255) as u8;
		o[3] = na.min(255) as u8;
	}
	out
}

#[cfg(test)]
mod tests {
	use super::*;

	#[test]
	fn isqrt_is_the_floor_of_the_square_root() {
		for n in 0..70_000_u64 {
			let r = isqrt(n);
			assert!(r * r <= n && (r + 1) * (r + 1) > n, "isqrt({n}) = {r}");
		}
		for n in [1_u64 << 40, (1 << 62) - 1, (1 << 63) + 12345, u64::MAX] {
			let r = isqrt(n);
			assert!(u128::from(r) * u128::from(r) <= u128::from(n));
			assert!(
				(u128::from(r) + 1) * (u128::from(r) + 1) > u128::from(n),
				"isqrt({n}) = {r}"
			);
		}
	}

	#[test]
	fn the_key_colour_goes_and_red_stays() {
		let spec = ChromaKeySpec {
			color: "#00FF00".into(),
			similarity: 0.3,
			smoothness: 0.0,
			spill: 0.0,
		};
		let img = Image {
			pix: vec![0, 255, 0, 255, 255, 0, 0, 255],
			w: 2,
			h: 1,
			opaque: true,
		};
		let out = apply_chroma_key(&img, &spec);
		assert_eq!(out.pix, [0, 0, 0, 0, 255, 0, 0, 255]);
		assert!(!out.opaque);
	}
}
