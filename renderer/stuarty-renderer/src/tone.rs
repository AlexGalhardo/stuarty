//! The tone table of SDD-013 section 3.11.1: the exposure, the levels and the curve of an image
//! layer, built in Go and applied here to every colour channel.

use crate::image::Image;
use crate::math::div255_i64;

/// The entries of a tone table.
pub const TONE_ENTRIES: usize = 256;

/// `src` with every colour channel mapped through `table` on the straight colour, alpha untouched.
pub fn apply_tone(src: &Image, table: &[u8; TONE_ENTRIES]) -> Image {
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
		let (mut nr, mut ng, mut nb) = (
			i64::from(table[r as usize]),
			i64::from(table[g as usize]),
			i64::from(table[b as usize]),
		);
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

#[cfg(test)]
mod tests {
	use super::*;

	fn table(f: impl Fn(usize) -> u8) -> [u8; TONE_ENTRIES] {
		core::array::from_fn(f)
	}

	#[test]
	fn the_identity_table_changes_nothing() {
		let img = Image {
			pix: vec![10, 20, 30, 255, 5, 10, 15, 100, 7, 7, 7, 0],
			w: 3,
			h: 1,
			opaque: false,
		};
		let out = apply_tone(&img, &table(|v| v as u8));
		// A translucent pixel goes through straight colour and back, which may move a channel by one.
		assert_eq!(out.pix[..4], img.pix[..4]);
		assert_eq!(out.pix[8..], img.pix[8..]);
		for k in 0..4 {
			assert!(out.pix[4 + k].abs_diff(img.pix[4 + k]) <= 1);
		}
	}

	#[test]
	fn a_translucent_pixel_is_mapped_on_its_straight_colour() {
		// Straight (200, 100, 50) at alpha 128 is premultiplied (100, 50, 25).
		let img = Image {
			pix: vec![100, 50, 25, 128],
			w: 1,
			h: 1,
			opaque: false,
		};
		let out = apply_tone(&img, &table(|v| 255 - v as u8));
		// The inverse is (55, 155, 205) straight, premultiplied (28, 78, 103).
		assert_eq!(out.pix, [28, 78, 103, 128]);
	}
}
