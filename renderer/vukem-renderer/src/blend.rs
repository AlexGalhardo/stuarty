//! Blend modes (SDD-004 section 3.8, port of `engine/compositor/blend.go`).
//!
//! The mix of a premultiplied source pixel with the opaque destination, per channel, in integer
//! arithmetic only. The destination's alpha is never changed by a blend mode.

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
			_ => return None,
		})
	}
}

/// One destination channel `dv` mixed with the premultiplied source channel `c` of alpha `a`.
/// The overlay term is signed (`a - c` is negative only through rounding), so it is computed on
/// `i64` and clamped.
pub fn blend_channel(mode: BlendMode, c: u32, dv: u32, a: u32) -> u8 {
	let ia = 255 - a;
	match mode {
		BlendMode::Normal => u8_of(c + div255(dv * ia)),
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
	}
}

/// Draws the premultiplied pixel `(r, g, b, a)`, opacity already applied, over `d`. A blend mode
/// leaves the destination's alpha alone; `Normal` blends it like the colour channels.
pub fn mix_pixel(d: &mut [u8], r: u32, g: u32, b: u32, a: u32, mode: BlendMode) {
	if mode == BlendMode::Normal {
		let ia = 255 - a;
		d[0] = u8_of(r + div255(u32::from(d[0]) * ia));
		d[1] = u8_of(g + div255(u32::from(d[1]) * ia));
		d[2] = u8_of(b + div255(u32::from(d[2]) * ia));
		d[3] = u8_of(a + div255(u32::from(d[3]) * ia));
		return;
	}
	d[0] = blend_channel(mode, r, u32::from(d[0]), a);
	d[1] = blend_channel(mode, g, u32::from(d[1]), a);
	d[2] = blend_channel(mode, b, u32::from(d[2]), a);
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
	}
}
