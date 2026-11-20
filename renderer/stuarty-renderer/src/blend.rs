//! Blend modes (SDD-004 section 3.8 and SDD-013 section 3.11.2, port of `engine/compositor/blend.go`
//! for the first five).
//!
//! The mix of a premultiplied source pixel with the destination, per channel, in integer
//! arithmetic only. On an opaque destination the first four modes keep the formulas of SDD-004;
//! the others, and every mode on a destination with alpha below 255, use the general form of
//! SDD-013 section 3.11.2.

use crate::math::{div255, u8_of};

/// The blend mode of a layer; `Normal` is the source-over of SDD-013 section 3.7.
#[derive(Clone, Copy, Debug, Default, PartialEq, Eq)]
pub enum BlendMode {
	#[default]
	Normal,
	Multiply,
	Screen,
	Overlay,
	Add,
	SoftLight,
	HardLight,
	Darken,
	Lighten,
	ColorDodge,
	ColorBurn,
	Difference,
}

impl BlendMode {
	/// The mode named by `blend` in the plan; `None` for a name the renderer does not know.
	pub fn from_name(name: &str) -> Option<Self> {
		Some(match name {
			"normal" => Self::Normal,
			"multiply" => Self::Multiply,
			"screen" => Self::Screen,
			"overlay" => Self::Overlay,
			"add" => Self::Add,
			"soft_light" => Self::SoftLight,
			"hard_light" => Self::HardLight,
			"darken" => Self::Darken,
			"lighten" => Self::Lighten,
			"color_dodge" => Self::ColorDodge,
			"color_burn" => Self::ColorBurn,
			"difference" => Self::Difference,
			_ => return None,
		})
	}

	/// Whether the mode has the multiplied-out formula of SDD-004 section 3.8 on an opaque
	/// destination.
	fn has_opaque_form(self) -> bool {
		matches!(
			self,
			Self::Multiply | Self::Screen | Self::Overlay | Self::Add
		)
	}
}

/// One destination channel `dv` mixed with the premultiplied source channel `c` of alpha `a`, on
/// an opaque destination, for the first four modes (and `Normal`). The overlay term is signed
/// (`a - c` is negative only through rounding), so it is computed on `i64` and clamped.
pub fn blend_channel(mode: BlendMode, c: u32, dv: u32, a: u32) -> u8 {
	let ia = 255 - a;
	match mode {
		BlendMode::Multiply => u8_of(div255(dv * (c + ia))),
		BlendMode::Screen => u8_of(c + dv - div255(c * dv)),
		BlendMode::Overlay => {
			if dv <= 127 {
				u8_of(div255(dv * (2 * c + ia)))
			} else {
				let v =
					i64::from(dv * ia) + (2 * i64::from(dv) - 255) * (i64::from(a) - i64::from(c));
				u8_of(c + div255(v.max(0) as u32))
			}
		}
		BlendMode::Add => u8_of(c + dv),
		_ => u8_of(c + div255(dv * ia)),
	}
}

/// `x * y / 255` rounded.
fn mul255(x: u32, y: u32) -> u32 {
	div255(x * y)
}

/// The screen of two 8-bit values.
fn screen(x: u32, y: u32) -> u32 {
	x + y - mul255(x, y)
}

/// `n / d` rounded to nearest, halves away from zero, for `d > 0`.
fn round_div_away(n: i64, d: i64) -> i64 {
	if n < 0 {
		-((-n * 2 + d) / (2 * d))
	} else {
		(n * 2 + d) / (2 * d)
	}
}

/// `sqrt(n)` rounded to nearest, in integers.
const fn isqrt_round(n: u32) -> u32 {
	let mut r = 0_u32;
	while (r + 1) * (r + 1) <= n {
		r += 1;
	}
	if n - r * r > r { r + 1 } else { r }
}

/// The `D` function of the soft light for one 8-bit value.
const fn soft_light_d_of(b: u32) -> i64 {
	if b <= 64 {
		let b = b as i64;
		(((16 * b - 3060) * b + 260_100) * b + 32_512) / 65_025
	} else {
		isqrt_round(255 * b) as i64
	}
}

/// `D` for every 8-bit value, made once at compile time: the soft light reads it per pixel.
static SOFT_LIGHT_D: [i64; 256] = {
	let mut t = [0_i64; 256];
	let mut b = 0;
	while b < 256 {
		t[b] = soft_light_d_of(b as u32);
		b += 1;
	}
	t
};

/// The `D` function of the soft light, in 8 bits.
fn soft_light_d(b: u32) -> i64 {
	SOFT_LIGHT_D[b.min(255) as usize]
}

/// The separable blend function `B(s, b)` of the straight source value `s` and the straight
/// destination value `b`, both 0 to 255 (SDD-013 section 3.11.2).
pub fn blend_fn(mode: BlendMode, s: u32, b: u32) -> u32 {
	match mode {
		BlendMode::Normal => s,
		BlendMode::Multiply => mul255(s, b),
		BlendMode::Screen => screen(s, b),
		BlendMode::Overlay => {
			if b <= 127 {
				mul255(s, 2 * b)
			} else {
				screen(s, 2 * b - 255)
			}
		}
		BlendMode::Add => (s + b).min(255),
		BlendMode::Darken => s.min(b),
		BlendMode::Lighten => s.max(b),
		BlendMode::Difference => s.abs_diff(b),
		BlendMode::HardLight => {
			if s <= 127 {
				mul255(b, 2 * s)
			} else {
				screen(b, 2 * s - 255)
			}
		}
		BlendMode::SoftLight => {
			if s <= 127 {
				b - mul255(255 - 2 * s, mul255(b, 255 - b))
			} else {
				let delta = (2 * i64::from(s) - 255) * (soft_light_d(b) - i64::from(b));
				(i64::from(b) + round_div_away(delta, 255)).clamp(0, 255) as u32
			}
		}
		BlendMode::ColorDodge => {
			if b == 0 {
				0
			} else if s == 255 {
				255
			} else {
				((b * 255 + (255 - s) / 2) / (255 - s)).min(255)
			}
		}
		BlendMode::ColorBurn => {
			if b == 255 {
				255
			} else {
				// `s == 0` has no quotient: the burn is 0 there.
				match ((255 - b) * 255 + s / 2).checked_div(s) {
					None => 0,
					Some(q) => 255 - q.min(255),
				}
			}
		}
	}
}

/// The straight value of a premultiplied channel `c` of alpha `a > 0`.
fn straight(c: u32, a: u32) -> u32 {
	((c * 255 + a / 2) / a).min(255)
}

/// One colour channel of the general form: the premultiplied source channel `c` of alpha `a` over
/// the destination channel `dc` of alpha `da > 0`.
fn general_channel(mode: BlendMode, c: u32, a: u32, dc: u32, da: u32) -> u8 {
	let t = div255(a * da);
	let bf = blend_fn(mode, straight(c, a), straight(dc, da));
	u8_of(div255(c * (255 - da)) + div255(dc * (255 - a)) + div255(t * bf))
}

/// Draws the premultiplied pixel `(r, g, b, a)`, opacity already applied, over `d`. `Normal`
/// is the source-over of every channel; the first four modes on an opaque destination keep the
/// formulas of SDD-004 section 3.8 and its alpha; everything else is the general form of SDD-013
/// section 3.11.2.
pub fn mix_pixel(d: &mut [u8], r: u32, g: u32, b: u32, a: u32, mode: BlendMode) {
	if mode == BlendMode::Normal {
		let ia = 255 - a;
		d[0] = u8_of(r + div255(u32::from(d[0]) * ia));
		d[1] = u8_of(g + div255(u32::from(d[1]) * ia));
		d[2] = u8_of(b + div255(u32::from(d[2]) * ia));
		d[3] = u8_of(a + div255(u32::from(d[3]) * ia));
		return;
	}
	let da = u32::from(d[3]);
	if da == 255 && mode.has_opaque_form() {
		d[0] = blend_channel(mode, r, u32::from(d[0]), a);
		d[1] = blend_channel(mode, g, u32::from(d[1]), a);
		d[2] = blend_channel(mode, b, u32::from(d[2]), a);
		return;
	}
	if a == 0 {
		return;
	}
	if da == 0 {
		d.copy_from_slice(&[u8_of(r), u8_of(g), u8_of(b), u8_of(a)]);
		return;
	}
	for (k, c) in [r, g, b].into_iter().enumerate() {
		d[k] = general_channel(mode, c, a, u32::from(d[k]), da);
	}
	d[3] = u8_of(a + da - div255(a * da));
}

#[cfg(test)]
mod tests {
	use super::*;

	#[test]
	fn opaque_source_values_are_the_ones_of_the_sdd() {
		let cases = [
			(BlendMode::Multiply, 255, 200, 200),
			(BlendMode::Multiply, 0, 200, 0),
			(BlendMode::Multiply, 128, 100, 50),
			(BlendMode::Screen, 0, 200, 200),
			(BlendMode::Screen, 255, 10, 255),
			(BlendMode::Overlay, 128, 100, 100),
			(BlendMode::Overlay, 255, 127, 254),
			(BlendMode::Overlay, 0, 128, 1),
			(BlendMode::Add, 200, 100, 255),
			(BlendMode::Add, 20, 30, 50),
		];
		for (mode, c, d, want) in cases {
			assert_eq!(blend_channel(mode, c, d, 255), want, "{mode:?} {c} {d}");
		}
	}

	#[test]
	fn a_blend_keeps_the_destination_alpha_and_normal_blends_it() {
		let mut d = [10, 20, 30, 255];
		mix_pixel(&mut d, 100, 100, 100, 100, BlendMode::Multiply);
		assert_eq!(d[3], 255);
		let mut d = [10, 20, 30, 100];
		mix_pixel(&mut d, 100, 100, 100, 100, BlendMode::Normal);
		assert_eq!(d[3], 100 + div255(100 * 155) as u8);
	}

	#[test]
	fn a_source_brighter_than_its_alpha_stays_in_range() {
		// c larger than a only happens through rounding; the overlay term must clamp, not wrap.
		assert_eq!(blend_channel(BlendMode::Overlay, 255, 255, 10), 255);
		assert_eq!(blend_channel(BlendMode::Overlay, 255, 200, 10), 255);
		assert_eq!(blend_channel(BlendMode::Multiply, 255, 255, 10), 255);
	}

	#[test]
	fn names_are_the_ones_of_the_plan() {
		assert_eq!(BlendMode::from_name("overlay"), Some(BlendMode::Overlay));
		assert_eq!(BlendMode::from_name("Overlay"), None);
		assert_eq!(BlendMode::from_name(""), None);
		assert_eq!(
			BlendMode::from_name("soft_light"),
			Some(BlendMode::SoftLight)
		);
		assert_eq!(
			BlendMode::from_name("color_dodge"),
			Some(BlendMode::ColorDodge)
		);
		assert_eq!(BlendMode::from_name("softlight"), None);
	}

	/// The values of the integer formulas, checked against the floating-point separable blend
	/// functions of the W3C compositing specification to within one level.
	#[test]
	fn the_blend_functions_have_their_known_values() {
		use BlendMode::*;
		let cases = [
			(Darken, 100, 200, 100),
			(Lighten, 100, 200, 200),
			(Difference, 100, 200, 100),
			(Difference, 200, 100, 100),
			(HardLight, 64, 200, 100),
			(HardLight, 192, 100, 178),
			(Overlay, 100, 64, 50),
			(ColorDodge, 0, 100, 100),
			(ColorDodge, 255, 100, 255),
			(ColorDodge, 128, 0, 0),
			(ColorDodge, 128, 100, 201),
			(ColorBurn, 255, 100, 100),
			(ColorBurn, 128, 255, 255),
			(ColorBurn, 128, 100, 0),
			(ColorBurn, 200, 200, 185),
			(SoftLight, 128, 0, 0),
			(SoftLight, 128, 255, 255),
			(SoftLight, 0, 128, 64),
			(SoftLight, 255, 128, 181),
			(SoftLight, 255, 32, 88),
			(SoftLight, 200, 200, 215),
			(SoftLight, 50, 50, 26),
		];
		for (mode, s, b, want) in cases {
			assert_eq!(blend_fn(mode, s, b), want, "{mode:?} s={s} b={b}");
		}
	}

	#[test]
	fn the_general_form_agrees_with_the_opaque_form_within_two_levels() {
		// `Add` is left out: SDD-004 section 3.8 adds the premultiplied source without the clamp of
		// `B` before the alpha, so it differs from the general form by design.
		for mode in [BlendMode::Multiply, BlendMode::Screen, BlendMode::Overlay] {
			for a in [1_u32, 60, 128, 200, 255] {
				for c in (0..=a).step_by(7) {
					for dv in (0..=255_u32).step_by(5) {
						let opaque = blend_channel(mode, c, dv, a);
						let general = general_channel(mode, c, a, dv, 255);
						assert!(
							opaque.abs_diff(general) <= 2,
							"{mode:?} c={c} a={a} d={dv}: {opaque} against {general}"
						);
					}
				}
			}
		}
	}

	#[test]
	fn a_new_mode_over_a_transparent_destination_draws_the_source() {
		let mut d = [0, 0, 0, 0];
		mix_pixel(&mut d, 50, 60, 70, 100, BlendMode::Difference);
		assert_eq!(d, [50, 60, 70, 100]);
		// Over a translucent destination the alphas combine like a source-over.
		let mut d = [40, 40, 40, 128];
		mix_pixel(&mut d, 50, 60, 70, 100, BlendMode::Lighten);
		assert_eq!(d[3], (100 + 128 - div255(100 * 128)) as u8);
	}

	#[test]
	fn a_new_mode_on_an_opaque_destination_keeps_it_opaque() {
		for mode in [
			BlendMode::SoftLight,
			BlendMode::HardLight,
			BlendMode::Darken,
			BlendMode::Lighten,
			BlendMode::ColorDodge,
			BlendMode::ColorBurn,
			BlendMode::Difference,
		] {
			let mut d = [10, 120, 250, 255];
			mix_pixel(&mut d, 90, 20, 200, 200, mode);
			assert_eq!(d[3], 255, "{mode:?}");
		}
	}
}
