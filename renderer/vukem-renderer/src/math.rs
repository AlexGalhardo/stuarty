//! The integer and floating-point helpers of SDD-013 section 3.2 and 3.9.
//!
//! Only operations that IEEE 754 rounds exactly are used, so every host gets the same bits.

use core::f64::consts::PI;

/// `x / 255` rounded to nearest, exact for `x` from 0 to 65535.
#[inline]
pub fn div255(x: u32) -> u32 {
	let x = x + 128;
	(x + (x >> 8)) >> 8
}

/// [`div255`] on `i64`.
#[inline]
pub fn div255_i64(x: i64) -> i64 {
	let x = x + 128;
	(x + (x >> 8)) >> 8
}

/// The byte of a value that is at most 255 by construction; the `min` keeps it so.
#[inline]
pub fn u8_of(v: u32) -> u8 {
	v.min(255) as u8
}

/// A product that is never fused with a following addition. Rust does not contract `a * b + c`
/// unless the build asks for it, and the build never does (SDD-013 section 3.9 rule 2); the
/// function names the places where that matters.
#[inline]
pub fn mul(a: f64, b: f64) -> f64 {
	a * b
}

/// Round half up, for placements and sizes: `floor(v + 0.5)` as an integer. Out of range values
/// saturate, which only an absurd plan reaches.
#[inline]
pub fn round_half_up(v: f64) -> i64 {
	(v + 0.5).floor() as i64
}

/// Round half away from zero, for the opacity and for fixed-point coefficients.
#[inline]
pub fn round_half_away(v: f64) -> i64 {
	v.round() as i64
}

const TWO_PI: f64 = 2.0 * PI;

/// The sine and cosine of `x` radians from a Taylor series with exactly rounded operations only,
/// the same series as `engine/compositor.sincos`.
pub fn sincos(x: f64) -> (f64, f64) {
	let x = x - mul(TWO_PI, (x / TWO_PI + 0.5).floor());
	let x2 = mul(x, x);
	let (mut sin, mut cos) = (x, 1.0_f64);
	let (mut ts, mut tc) = (x, 1.0_f64);
	for k in 1..=24_i32 {
		tc = -mul(tc, x2) / f64::from((2 * k - 1) * (2 * k));
		ts = -mul(ts, x2) / f64::from((2 * k) * (2 * k + 1));
		cos += tc;
		sin += ts;
	}
	(sin, cos)
}

#[cfg(test)]
mod tests {
	use super::*;

	#[test]
	fn div255_is_division_rounded_to_nearest_for_every_input() {
		for x in 0..=65535_u32 {
			let want = (x + 127) / 255;
			assert_eq!(div255(x), want, "x = {x}");
			assert_eq!(div255_i64(i64::from(x)), i64::from(want), "x = {x}");
		}
	}

	#[test]
	fn rounding_modes_differ_on_halves() {
		assert_eq!(round_half_up(2.5), 3);
		assert_eq!(round_half_up(-2.5), -2);
		assert_eq!(round_half_away(2.5), 3);
		assert_eq!(round_half_away(-2.5), -3);
	}

	#[test]
	fn sincos_is_exact_at_the_axes() {
		let (s, c) = sincos(0.0);
		assert_eq!((s, c), (0.0, 1.0));
		let (s, c) = sincos(PI / 2.0);
		assert!((s - 1.0).abs() < 1e-15 && c.abs() < 1e-15);
	}
}
