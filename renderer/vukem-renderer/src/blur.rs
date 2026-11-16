//! The blur of SDD-013 section 3.5: three box passes along each axis in integer arithmetic that
//! rounds half up at every pass, the port of `engine/compositor/blur.go`.

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

	let (px, py) = (3 * rx, 3 * ry);
	let (w, h) = (src.w + 2 * px, src.h + 2 * py);

	// Rows first: the source is placed in a transparent field and blurred three times along x.
	let mut rows = vec![0_u8; w * h * 4];
	let mut a = vec![0_u8; w * 4];
	let mut b = vec![0_u8; w * 4];
	for sy in 0..src.h {
		a.fill(0);
		a[px * 4..(px + src.w) * 4].copy_from_slice(&src.pix[sy * src.w * 4..(sy + 1) * src.w * 4]);
		box_pass(&a, &mut b, rx);
		box_pass(&b, &mut a, rx);
		let y = sy + py;
		box_pass(&a, &mut rows[y * w * 4..(y + 1) * w * 4], rx);
	}

	// Then columns, three passes along y on a gathered column.
	let mut out = vec![0_u8; w * h * 4];
	let mut ca = vec![0_u8; h * 4];
	let mut cb = vec![0_u8; h * 4];
	for x in 0..w {
		for y in 0..h {
			ca[y * 4..y * 4 + 4].copy_from_slice(&rows[(y * w + x) * 4..(y * w + x) * 4 + 4]);
		}
		box_pass(&ca, &mut cb, ry);
		box_pass(&cb, &mut ca, ry);
		box_pass(&ca, &mut cb, ry);
		for y in 0..h {
			out[(y * w + x) * 4..(y * w + x) * 4 + 4].copy_from_slice(&cb[y * 4..y * 4 + 4]);
		}
	}

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

/// One box pass over a line of RGBA pixels: each output pixel is the mean of the `2r + 1` pixels
/// centred on it, the outside counting as zero, rounded half up.
fn box_pass(line: &[u8], out: &mut [u8], r: usize) {
	let n = line.len() / 4;
	let div = (2 * r + 1) as u32;
	let half = div / 2;
	let mut sum = [0_u32; 4];

	for x in 0..=r.min(n.saturating_sub(1)) {
		add(&mut sum, &line[x * 4..x * 4 + 4]);
	}
	for x in 0..n {
		for k in 0..4 {
			out[x * 4 + k] = ((sum[k] + half) / div) as u8;
		}
		let incoming = x + r + 1;
		if incoming < n {
			add(&mut sum, &line[incoming * 4..incoming * 4 + 4]);
		}
		if x >= r {
			let outgoing = x - r;
			sub(&mut sum, &line[outgoing * 4..outgoing * 4 + 4]);
		}
	}
}

fn add(sum: &mut [u32; 4], px: &[u8]) {
	for k in 0..4 {
		sum[k] += u32::from(px[k]);
	}
}

fn sub(sum: &mut [u32; 4], px: &[u8]) {
	for k in 0..4 {
		sum[k] -= u32::from(px[k]);
	}
}
