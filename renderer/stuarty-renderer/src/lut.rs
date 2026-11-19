//! The 3D lookup table of SDD-004 section 3.6 (SDD-013 section 3.4): trilinear interpolation of the eight
//! entries around a pixel in integer arithmetic, mixed in by the intensity, the operations of
//! `engine/compositor/lut.go`. The renderer parses no text: the host reads the `.cube` file (`engine/lut`) and
//! uploads the entries as 16-bit integers.

use crate::error::{Error, Reason};
use crate::image::Image;
use crate::math::{div255_i64, mul};

/// The smallest and the largest number of points on a side of a table.
pub const MIN_SIZE: usize = 2;
pub const MAX_SIZE: usize = 65;

/// The denominator of the trilinear sum: 255 per axis for the weights, and 257 turning the 16-bit entries
/// (65535 = 255 x 257) into the 8 bits of a channel.
const DEN: i64 = 255 * 255 * 255 * 257;

/// A table: `size` points on a side and `size^3` entries of red, green and blue (0 is 0.0, 65535 is 1.0),
/// the red index varying fastest.
#[derive(Clone, Debug, PartialEq, Eq)]
pub struct Lut {
	size: usize,
	data: Vec<u16>,
}

impl Lut {
	/// A table from its entries; a size outside `MIN_SIZE..=MAX_SIZE` or a count other than `size^3 x 3` is an
	/// `invalid_argument`.
	pub fn new(size: usize, data: Vec<u16>) -> Result<Self, Error> {
		if !(MIN_SIZE..=MAX_SIZE).contains(&size) {
			return Err(Error::new(
				Reason::InvalidArgument,
				"a lookup table is 2 to 65 points on a side",
			));
		}
		if data.len() != 3 * size * size * size {
			return Err(Error::new(
				Reason::InvalidArgument,
				"the entries are not size x size x size x 3",
			));
		}
		Ok(Self { size, data })
	}

	/// A table from little-endian bytes, as the host uploads it.
	pub fn from_bytes(size: usize, bytes: &[u8]) -> Result<Self, Error> {
		let data = bytes
			.as_chunks::<2>()
			.0
			.iter()
			.map(|b| u16::from_le_bytes(*b))
			.collect();
		if !bytes.len().is_multiple_of(2) {
			return Err(Error::new(
				Reason::InvalidArgument,
				"the entries are not whole 16-bit values",
			));
		}
		Self::new(size, data)
	}

	/// The bytes of the entries, which is what the table costs against the budget.
	pub fn byte_len(&self) -> usize {
		self.data.len() * 2
	}

	pub fn size(&self) -> usize {
		self.size
	}
}

/// The intensity in thousandths, 0 to 1000; NaN is 0.
pub fn lut_mix(intensity: f64) -> i64 {
	if intensity.is_nan() {
		return 0;
	}
	mul(intensity, 1000.0).clamp(0.0, 1000.0).round() as i64
}

/// The colour of one straight pixel through the table, mixed with the pixel by `im` thousandths.
fn pixel(t: &Lut, im: i64, r: i64, g: i64, b: i64) -> (i64, i64, i64) {
	let n = t.size as i64;
	let n1 = n - 1;
	// A channel v sits at v x (n - 1) / 255 on the axis: the index below it and the weight of the one above,
	// out of 255.
	let (pr, pg, pb) = (r * n1, g * n1, b * n1);
	let (r0, g0, b0) = (pr / 255, pg / 255, pb / 255);
	let (fr, fg, fb) = (pr % 255, pg % 255, pb % 255);
	let (r1, g1, b1) = ((r0 + 1).min(n1), (g0 + 1).min(n1), (b0 + 1).min(n1));
	let at = |ri: i64, gi: i64, bi: i64| (3 * (ri + n * (gi + n * bi))) as usize;
	let corners = [
		(at(r0, g0, b0), (255 - fr) * (255 - fg) * (255 - fb)),
		(at(r1, g0, b0), fr * (255 - fg) * (255 - fb)),
		(at(r0, g1, b0), (255 - fr) * fg * (255 - fb)),
		(at(r1, g1, b0), fr * fg * (255 - fb)),
		(at(r0, g0, b1), (255 - fr) * (255 - fg) * fb),
		(at(r1, g0, b1), fr * (255 - fg) * fb),
		(at(r0, g1, b1), (255 - fr) * fg * fb),
		(at(r1, g1, b1), fr * fg * fb),
	];
	let mut sum = [0_i64; 3];
	for (i, w) in corners {
		for (k, s) in sum.iter_mut().enumerate() {
			*s += w * i64::from(t.data[i + k]);
		}
	}
	let input = [r, g, b];
	let mut out = [0_i64; 3];
	for k in 0..3 {
		let v = (sum[k] + DEN / 2) / DEN;
		out[k] = (input[k] * (1000 - im) + v * im + 500) / 1000;
	}
	(out[0], out[1], out[2])
}

/// `src` with the table applied to its straight RGB values, mixed in by `im` thousandths (SDD-004 section 3.6).
pub fn apply_lut(src: &Image, t: &Lut, im: i64) -> Image {
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
		let (mut nr, mut ng, mut nb) = pixel(t, im, r, g, b);
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

	/// A table that maps every colour to itself, on a grid of `n` points.
	fn identity(n: usize) -> Lut {
		let mut data = Vec::new();
		for b in 0..n {
			for g in 0..n {
				for r in 0..n {
					for v in [r, g, b] {
						data.push(((v * 65535 + (n - 1) / 2) / (n - 1)) as u16);
					}
				}
			}
		}
		Lut::new(n, data).unwrap()
	}

	#[test]
	fn the_identity_table_keeps_a_pixel_within_one_level() {
		for n in [2, 3, 17, 33] {
			let t = identity(n);
			for v in (0..256).step_by(7) {
				let (r, g, b) = pixel(&t, 1000, v, 255 - v, (v * 3) % 256);
				assert!(
					(r - v).abs() <= 1
						&& (g - (255 - v)).abs() <= 1
						&& (b - (v * 3) % 256).abs() <= 1,
					"size {n}, v {v}: {r} {g} {b}"
				);
			}
		}
		let t = identity(2);
		for v in 0..256 {
			assert_eq!(pixel(&t, 1000, v, v, v), (v, v, v));
		}
	}

	#[test]
	fn a_table_that_inverts_and_the_mix_with_the_pixel() {
		let mut data = Vec::new();
		for b in 0..2_u16 {
			for g in 0..2_u16 {
				for r in 0..2_u16 {
					data.extend([65535 - r * 65535, 65535 - g * 65535, 65535 - b * 65535]);
				}
			}
		}
		let t = Lut::new(2, data).unwrap();
		assert_eq!(pixel(&t, 1000, 10, 200, 255), (245, 55, 0));
		assert_eq!(pixel(&t, 500, 0, 200, 255), (128, 128, 128));
		assert_eq!(pixel(&t, 0, 10, 200, 255), (10, 200, 255));
	}

	#[test]
	fn refuses_a_table_that_is_not_one() {
		assert_eq!(
			Lut::new(1, vec![0; 3]).unwrap_err().reason,
			Reason::InvalidArgument
		);
		assert_eq!(
			Lut::new(66, vec![]).unwrap_err().reason,
			Reason::InvalidArgument
		);
		assert_eq!(
			Lut::new(2, vec![0; 23]).unwrap_err().reason,
			Reason::InvalidArgument
		);
		assert_eq!(
			Lut::from_bytes(2, &[0; 47]).unwrap_err().reason,
			Reason::InvalidArgument
		);
		assert_eq!(Lut::from_bytes(2, &[1, 0].repeat(24)).unwrap().size(), 2);
	}

	#[test]
	fn the_intensity_is_held_to_its_range() {
		assert_eq!(lut_mix(f64::NAN), 0);
		assert_eq!(lut_mix(-3.0), 0);
		assert_eq!(lut_mix(7.0), 1000);
		assert_eq!(lut_mix(0.6), 600);
	}
}
