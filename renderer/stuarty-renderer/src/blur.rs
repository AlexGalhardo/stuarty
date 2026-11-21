//! The blur of SDD-013 section 3.5: three box passes along each axis in integer arithmetic that
//! rounds half up at every pass, the port of `engine/compositor/blur.go`.
//!
//! The arithmetic is the one of the specification; how it is computed is not part of it, so three
//! things keep the cost down without changing a bit: the mean is a multiplication by the reciprocal
//! (exact for every sum a pass can reach, see `the_reciprocal_is_the_division`), only the channels
//! that can be non-zero are computed (a silhouette has an alpha and nothing else), and a pass along
//! the columns walks the rows with one running sum per column instead of gathering each column.

use crate::compose::Placement;
use crate::image::Image;
use crate::math::{mul, round_half_away};

/// The largest box radius of one pass, in source pixels.
pub const MAX_BLUR_RADIUS: usize = 128;

/// The radius of the box that three passes turn into a Gaussian of standard deviation `sigma`:
/// the positive root of `r (r + 1) = sigma^2`, rounded. A sigma under about 0.8 gives 0.
pub fn blur_radius(sigma: f64) -> usize {
	if sigma.is_nan() || sigma <= 0.0 {
		return 0;
	}
	let r = round_half_away(((mul(4.0, mul(sigma, sigma)) + 1.0).sqrt() - 1.0) / 2.0);
	usize::try_from(r).unwrap_or(0).min(MAX_BLUR_RADIUS)
}

/// The mean of a window of `2r + 1` values, rounded half up: `(sum + half) / div` as a multiplication.
#[derive(Clone, Copy)]
struct Mean {
	half: u32,
	recip: u64,
}

impl Mean {
	fn new(r: usize) -> Self {
		let div = (2 * r + 1) as u64;
		Self {
			half: (div / 2) as u32,
			recip: (1_u64 << 32) / div + 1,
		}
	}

	#[inline(always)]
	fn of(self, sum: u32) -> u8 {
		(((u64::from(sum) + u64::from(self.half)) * self.recip) >> 32) as u8
	}
}

/// The channels worth computing: the alpha alone when the colour channels are all zero (the
/// silhouette of a glow or a shadow), and every channel otherwise.
fn alpha_only(src: &Image) -> bool {
	src.pix
		.as_chunks::<4>()
		.0
		.iter()
		.all(|p| p[0] == 0 && p[1] == 0 && p[2] == 0)
}

/// `src` blurred by `sigma`, a standard deviation in output pixels, and the placement that draws
/// it. The blur is of the layer's own premultiplied pixels with the outside counted as
/// transparent, so the result is larger than `src` by three radii on every side and the
/// placement grows by the same amount around the same centre. `None` when both radii are 0 and the
/// layer is drawn as it is.
pub fn blurred(src: &Image, p: &Placement, sigma: f64) -> Option<(Image, Placement)> {
	let rx = blur_radius(sigma / (p.w / src.w as f64));
	let ry = blur_radius(sigma / (p.h / src.h as f64));
	if rx == 0 && ry == 0 {
		return None;
	}

	let (w, h) = (src.w + 6 * rx, src.h + 6 * ry);
	let out = if alpha_only(src) {
		blur_zero::<3, 1>(src, rx, ry)
	} else {
		blur_zero::<0, 4>(src, rx, ry)
	};

	let grown = Placement {
		cx: p.cx,
		cy: p.cy,
		rot: p.rot,
		w: mul(p.w, w as f64) / src.w as f64,
		h: mul(p.h, h as f64) / src.h as f64,
	};
	Some((
		Image {
			pix: out,
			w,
			h,
			opaque: false,
		},
		grown,
	))
}

/// The channels `F..F + C` of `src` blurred with the outside counted as zero; the others are 0.
fn blur_zero<const F: usize, const C: usize>(src: &Image, rx: usize, ry: usize) -> Vec<u8> {
	let (px, py) = (3 * rx, 3 * ry);
	let (w, h) = (src.w + 2 * px, src.h + 2 * py);
	let (mx, my) = (Mean::new(rx), Mean::new(ry));

	// Rows first: the source is placed in a transparent field and blurred three times along x.
	let mut rows = vec![0_u8; w * h * 4];
	let mut a = vec![0_u8; w * 4];
	let mut b = vec![0_u8; w * 4];
	for sy in 0..src.h {
		a.fill(0);
		a[px * 4..(px + src.w) * 4].copy_from_slice(&src.pix[sy * src.w * 4..(sy + 1) * src.w * 4]);
		box_pass::<F, C>(&a, &mut b, rx, mx);
		box_pass::<F, C>(&b, &mut a, rx, mx);
		let y = sy + py;
		box_pass::<F, C>(&a, &mut rows[y * w * 4..(y + 1) * w * 4], rx, mx);
	}

	// Then columns, three passes along y.
	let mut out = vec![0_u8; w * h * 4];
	let mut sums = Vec::new();
	column_pass::<F, C>(&rows, &mut out, w, h, ry, my, &mut sums);
	column_pass::<F, C>(&out, &mut rows, w, h, ry, my, &mut sums);
	column_pass::<F, C>(&rows, &mut out, w, h, ry, my, &mut sums);
	out
}

/// `src` blurred like [`blurred`] but with the edge pixels repeated beyond the layer instead of
/// transparent ones, and kept at its size: the blurred copy an unsharp mask subtracts (SDD-013
/// section 3.11.3), so an opaque picture has no bright fringe. `rx` and `ry` are the box radii. Only
/// the colour channels are computed (the unsharp mask never reads the alpha of the copy); its alpha
/// is 0.
pub fn blurred_replicate(src: &Image, rx: usize, ry: usize) -> Image {
	let (px, py) = (3 * rx, 3 * ry);
	let (w, h) = (src.w, src.h);
	let (mx, my) = (Mean::new(rx), Mean::new(ry));

	// The padded field, rows first: each row is padded by its edge pixels and blurred three times.
	let ph = h + 2 * py;
	let mut field = vec![0_u8; w * ph * 4];
	let mut a = vec![0_u8; (w + 2 * px) * 4];
	let mut b = vec![0_u8; (w + 2 * px) * 4];
	for sy in 0..h {
		let line = &src.pix[sy * w * 4..(sy + 1) * w * 4];
		for x in 0..w + 2 * px {
			let sx = x.saturating_sub(px).min(w - 1);
			a[x * 4..x * 4 + 4].copy_from_slice(&line[sx * 4..sx * 4 + 4]);
		}
		box_pass::<0, 3>(&a, &mut b, rx, mx);
		box_pass::<0, 3>(&b, &mut a, rx, mx);
		box_pass::<0, 3>(&a, &mut b, rx, mx);
		let y = sy + py;
		field[y * w * 4..(y + 1) * w * 4].copy_from_slice(&b[px * 4..(px + w) * 4]);
	}
	// The rows above and below repeat the first and the last.
	for y in 0..py {
		let (first, last) = (py * w * 4, (py + h - 1) * w * 4);
		field.copy_within(first..first + w * 4, y * w * 4);
		field.copy_within(last..last + w * 4, (py + h + y) * w * 4);
	}

	let mut other = vec![0_u8; w * ph * 4];
	let mut sums = Vec::new();
	column_pass::<0, 3>(&field, &mut other, w, ph, ry, my, &mut sums);
	column_pass::<0, 3>(&other, &mut field, w, ph, ry, my, &mut sums);
	column_pass::<0, 3>(&field, &mut other, w, ph, ry, my, &mut sums);

	Image {
		pix: other[py * w * 4..(py + h) * w * 4].to_vec(),
		w,
		h,
		opaque: false,
	}
}

/// One box pass over a line of RGBA pixels: each output pixel is the mean of the `2r + 1` pixels
/// centred on it, the outside counting as zero, rounded half up. Only the channels `F..F + C` are
/// read and written.
fn box_pass<const F: usize, const C: usize>(line: &[u8], out: &mut [u8], r: usize, m: Mean) {
	let n = line.len() / 4;
	let mut sum = [0_u32; C];

	for x in 0..=r.min(n.saturating_sub(1)) {
		for k in 0..C {
			sum[k] += u32::from(line[x * 4 + F + k]);
		}
	}
	for x in 0..n {
		for k in 0..C {
			out[x * 4 + F + k] = m.of(sum[k]);
		}
		let incoming = x + r + 1;
		if incoming < n {
			for k in 0..C {
				sum[k] += u32::from(line[incoming * 4 + F + k]);
			}
		}
		if x >= r {
			let outgoing = x - r;
			for k in 0..C {
				sum[k] -= u32::from(line[outgoing * 4 + F + k]);
			}
		}
	}
}

/// The same pass along every column of a `w` by `h` field at once: one running sum per column, the
/// rows walked in order, so the memory is read and written row by row.
fn column_pass<const F: usize, const C: usize>(
	src: &[u8],
	dst: &mut [u8],
	w: usize,
	h: usize,
	r: usize,
	m: Mean,
	sums: &mut Vec<u32>,
) {
	let stride = w * 4;
	sums.clear();
	sums.resize(w * C, 0);

	for y in 0..=r.min(h.saturating_sub(1)) {
		add_row::<F, C>(sums, &src[y * stride..(y + 1) * stride]);
	}
	for y in 0..h {
		let out = &mut dst[y * stride..(y + 1) * stride];
		for (px, s) in out
			.as_chunks_mut::<4>()
			.0
			.iter_mut()
			.zip(sums.as_chunks::<C>().0)
		{
			for k in 0..C {
				px[F + k] = m.of(s[k]);
			}
		}
		let incoming = y + r + 1;
		if incoming < h {
			add_row::<F, C>(sums, &src[incoming * stride..(incoming + 1) * stride]);
		}
		if y >= r {
			let outgoing = y - r;
			for (px, s) in src[outgoing * stride..(outgoing + 1) * stride]
				.as_chunks::<4>()
				.0
				.iter()
				.zip(sums.as_chunks_mut::<C>().0.iter_mut())
			{
				for k in 0..C {
					s[k] -= u32::from(px[F + k]);
				}
			}
		}
	}
}

fn add_row<const F: usize, const C: usize>(sums: &mut [u32], row: &[u8]) {
	for (px, s) in row
		.as_chunks::<4>()
		.0
		.iter()
		.zip(sums.as_chunks_mut::<C>().0.iter_mut())
	{
		for k in 0..C {
			s[k] += u32::from(px[F + k]);
		}
	}
}

#[cfg(test)]
mod tests {
	use super::*;

	/// The blur as the specification wrote it: a division, every channel, a gathered column.
	mod reference {
		use super::*;

		pub fn box_pass(line: &[u8], out: &mut [u8], r: usize) {
			let n = line.len() / 4;
			let div = (2 * r + 1) as u32;
			let half = div / 2;
			let mut sum = [0_u32; 4];
			for x in 0..=r.min(n.saturating_sub(1)) {
				for k in 0..4 {
					sum[k] += u32::from(line[x * 4 + k]);
				}
			}
			for x in 0..n {
				for k in 0..4 {
					out[x * 4 + k] = ((sum[k] + half) / div) as u8;
				}
				let incoming = x + r + 1;
				if incoming < n {
					for k in 0..4 {
						sum[k] += u32::from(line[incoming * 4 + k]);
					}
				}
				if x >= r {
					for k in 0..4 {
						sum[k] -= u32::from(line[(x - r) * 4 + k]);
					}
				}
			}
		}

		pub fn blurred(src: &Image, rx: usize, ry: usize) -> Vec<u8> {
			let (px, py) = (3 * rx, 3 * ry);
			let (w, h) = (src.w + 2 * px, src.h + 2 * py);
			let mut rows = vec![0_u8; w * h * 4];
			let mut a = vec![0_u8; w * 4];
			let mut b = vec![0_u8; w * 4];
			for sy in 0..src.h {
				a.fill(0);
				a[px * 4..(px + src.w) * 4]
					.copy_from_slice(&src.pix[sy * src.w * 4..(sy + 1) * src.w * 4]);
				box_pass(&a, &mut b, rx);
				box_pass(&b, &mut a, rx);
				let y = sy + py;
				box_pass(&a, &mut rows[y * w * 4..(y + 1) * w * 4], rx);
			}
			let mut out = vec![0_u8; w * h * 4];
			let mut ca = vec![0_u8; h * 4];
			let mut cb = vec![0_u8; h * 4];
			for x in 0..w {
				for y in 0..h {
					ca[y * 4..y * 4 + 4]
						.copy_from_slice(&rows[(y * w + x) * 4..(y * w + x) * 4 + 4]);
				}
				box_pass(&ca, &mut cb, ry);
				box_pass(&cb, &mut ca, ry);
				box_pass(&ca, &mut cb, ry);
				for y in 0..h {
					out[(y * w + x) * 4..(y * w + x) * 4 + 4]
						.copy_from_slice(&cb[y * 4..y * 4 + 4]);
				}
			}
			out
		}

		pub fn replicate(src: &Image, rx: usize, ry: usize) -> Vec<u8> {
			let (px, py) = (3 * rx, 3 * ry);
			let (w, h) = (src.w, src.h);
			let mut rows = vec![0_u8; w * h * 4];
			let mut a = vec![0_u8; (w + 2 * px) * 4];
			let mut b = vec![0_u8; (w + 2 * px) * 4];
			for sy in 0..h {
				let line = &src.pix[sy * w * 4..(sy + 1) * w * 4];
				for x in 0..w + 2 * px {
					let sx = x.saturating_sub(px).min(w - 1);
					a[x * 4..x * 4 + 4].copy_from_slice(&line[sx * 4..sx * 4 + 4]);
				}
				box_pass(&a, &mut b, rx);
				box_pass(&b, &mut a, rx);
				box_pass(&a, &mut b, rx);
				rows[sy * w * 4..(sy + 1) * w * 4].copy_from_slice(&b[px * 4..(px + w) * 4]);
			}
			let mut out = vec![0_u8; w * h * 4];
			let mut ca = vec![0_u8; (h + 2 * py) * 4];
			let mut cb = vec![0_u8; (h + 2 * py) * 4];
			for x in 0..w {
				for y in 0..h + 2 * py {
					let sy = y.saturating_sub(py).min(h - 1);
					ca[y * 4..y * 4 + 4]
						.copy_from_slice(&rows[(sy * w + x) * 4..(sy * w + x) * 4 + 4]);
				}
				box_pass(&ca, &mut cb, ry);
				box_pass(&cb, &mut ca, ry);
				box_pass(&ca, &mut cb, ry);
				for y in 0..h {
					out[(y * w + x) * 4..(y * w + x) * 4 + 4]
						.copy_from_slice(&cb[(y + py) * 4..(y + py) * 4 + 4]);
				}
			}
			out
		}
	}

	fn noise(w: usize, h: usize, seed: u32) -> Image {
		let mut s = seed;
		let mut img = Image::new(w, h);
		for v in &mut img.pix {
			s = s.wrapping_mul(1_664_525).wrapping_add(1_013_904_223);
			*v = (s >> 24) as u8;
		}
		img
	}

	#[test]
	fn the_reciprocal_is_the_division() {
		// Every radius a pass can have, every sum a window of that radius can reach.
		for r in 0..=MAX_BLUR_RADIUS {
			let div = (2 * r + 1) as u32;
			let m = Mean::new(r);
			for sum in 0..=255 * div {
				assert_eq!(
					u32::from(m.of(sum)),
					(sum + div / 2) / div,
					"r {r}, sum {sum}"
				);
			}
		}
	}

	#[test]
	fn the_blur_is_the_specified_blur() {
		for (w, h, rx, ry, seed) in [
			(17, 11, 2, 3, 1),
			(5, 9, 0, 2, 2),
			(23, 7, 4, 0, 3),
			(1, 1, 3, 3, 4),
			(40, 30, 6, 5, 5),
		] {
			let img = noise(w, h, seed);
			assert_eq!(
				blur_zero::<0, 4>(&img, rx, ry),
				reference::blurred(&img, rx, ry),
				"{w}x{h} r{rx},{ry}"
			);
		}
	}

	#[test]
	fn a_silhouette_blurs_to_the_same_bytes_in_its_alpha_alone() {
		let mut img = noise(31, 19, 9);
		for p in img.pix.chunks_mut(4) {
			p[0] = 0;
			p[1] = 0;
			p[2] = 0;
		}
		assert!(alpha_only(&img));
		for (rx, ry) in [(1, 1), (3, 5), (0, 4), (7, 2)] {
			assert_eq!(
				blur_zero::<3, 1>(&img, rx, ry),
				reference::blurred(&img, rx, ry),
				"r{rx},{ry}"
			);
		}
	}

	#[test]
	fn the_replicating_blur_is_the_specified_one_in_its_colour_channels() {
		for (w, h, rx, ry, seed) in [
			(13, 9, 2, 3, 7),
			(6, 20, 0, 4, 8),
			(1, 5, 2, 2, 9),
			(30, 30, 5, 5, 10),
		] {
			let img = noise(w, h, seed);
			let got = blurred_replicate(&img, rx, ry);
			let want = reference::replicate(&img, rx, ry);
			for (g, w) in got.pix.chunks(4).zip(want.chunks(4)) {
				assert_eq!(g[..3], w[..3], "{w:?} r{rx},{ry}");
				assert_eq!(g[3], 0);
			}
		}
	}

	#[test]
	fn a_layer_blur_grows_the_placement_by_three_radii() {
		let img = noise(10, 8, 3);
		let p = Placement {
			cx: 5.0,
			cy: 4.0,
			w: 10.0,
			h: 8.0,
			rot: 0.0,
		};
		let (out, grown) = blurred(&img, &p, 2.0).unwrap();
		let r = blur_radius(2.0);
		assert_eq!((out.w, out.h), (10 + 6 * r, 8 + 6 * r));
		assert_eq!(grown.cx, 5.0);
		assert!(blurred(&img, &p, 0.1).is_none());
	}
}
