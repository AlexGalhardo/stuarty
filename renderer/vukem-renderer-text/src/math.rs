//! The two floating-point helpers the geometry of the rasterizer shares with the renderer
//! (SDD-013 section 3.9). They live in this crate so that the renderer can link the rasterizer
//! without a dependency cycle; `vukem_renderer::math` re-exports them.

use core::f64::consts::PI;

/// A product that is never fused with a following addition. Rust does not contract `a * b + c`
/// unless the build asks for it, and the build never does (SDD-013 section 3.9 rule 2); the
/// function names the places where that matters.
#[inline]
pub fn mul(a: f64, b: f64) -> f64 {
	a * b
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
	fn sincos_is_exact_at_the_axes() {
		let (s, c) = sincos(0.0);
		assert_eq!((s, c), (0.0, 1.0));
		let (s, c) = sincos(PI / 2.0);
		assert!((s - 1.0).abs() < 1e-15 && c.abs() < 1e-15);
	}
}
