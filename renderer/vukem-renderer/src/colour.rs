//! The 3x4 colour matrix of SDD-013 section 3.4.

use crate::image::Image;
use crate::math::{div255_i64, mul, round_half_away};

/// The largest fixed-point coefficient: 2^24 in 16.16.
const MAX_Q: i64 = 1 << 40;

/// `src` with the matrix applied to its non-linear RGB values, clamped. Coefficients are rounded
/// to 16.16 fixed point once, so the per-pixel work is integer arithmetic.
pub fn apply_matrix(src: &Image, m: &[f64; 12]) -> Image {
	// A coefficient beyond 2^24 only saturates the result, and the clamp keeps every sum far from
	// the limits of an i64.
	let q: [i64; 12] =
		core::array::from_fn(|i| round_half_away(mul(m[i], 65536.0)).clamp(-MAX_Q, MAX_Q));
	let mut out = Image {
		pix: vec![0; src.w * src.h * 4],
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
		let a = i64::from(s[3]);
		let (mut r, mut g, mut b) = (i64::from(s[0]), i64::from(s[1]), i64::from(s[2]));
		match a {
			0 => {
				o.copy_from_slice(s);
				continue;
			}
			255 => {}
			_ => {
				r = ((r * 255 + a / 2) / a).min(255);
				g = ((g * 255 + a / 2) / a).min(255);
				b = ((b * 255 + a / 2) / a).min(255);
			}
		}
		let mut nr = clamp8((q[0] * r + q[1] * g + q[2] * b + q[3] * 255 + 32768) >> 16);
		let mut ng = clamp8((q[4] * r + q[5] * g + q[6] * b + q[7] * 255 + 32768) >> 16);
		let mut nb = clamp8((q[8] * r + q[9] * g + q[10] * b + q[11] * 255 + 32768) >> 16);
		if a != 255 {
			nr = div255_i64(nr * a);
			ng = div255_i64(ng * a);
			nb = div255_i64(nb * a);
		}
		o[0] = nr.min(255) as u8;
		o[1] = ng.min(255) as u8;
		o[2] = nb.min(255) as u8;
		o[3] = s[3];
	}
	out
}

fn clamp8(v: i64) -> i64 {
	v.clamp(0, 255)
}

#[cfg(test)]
mod tests {
	use super::*;

	#[test]
	fn a_huge_coefficient_saturates_instead_of_overflowing() {
		let img = Image {
			pix: vec![200, 100, 50, 255],
			w: 1,
			h: 1,
			opaque: true,
		};
		let mut m = [0.0; 12];
		m[0] = f64::MAX;
		m[5] = f64::MIN;
		m[10] = 1.0;
		let out = apply_matrix(&img, &m);
		assert_eq!(out.pix, [255, 0, 50, 255]);
	}
}
