//! The box-filtered shrink of a bitmap that is much larger than it is drawn (SDD-013 section 3.3
//! step 4), the port of `compositor.Resize`.

use crate::blend::BlendMode;
use crate::compose::{Placement, Target, sample};
use crate::image::Image;

/// `src` scaled to `w` by `h` pixels with the sampler of the compositor. A source at least twice
/// as large as the target is first halved by averaging 2x2 blocks.
pub fn resize(src: &Image, w: usize, h: usize) -> Image {
	if w == 0 || h == 0 || src.w == 0 || src.h == 0 {
		return Image::default();
	}
	let mut halved: Option<Image> = None;
	loop {
		let cur = halved.as_ref().unwrap_or(src);
		if cur.w < 2 * w || cur.h < 2 * h {
			break;
		}
		halved = Some(halve(cur));
	}
	let cur = halved.as_ref().unwrap_or(src);
	if cur.w == w && cur.h == h {
		return cur.clone();
	}

	let mut dst = Image::new(w, h);
	let p = Placement {
		cx: w as f64 / 2.0,
		cy: h as f64 / 2.0,
		w: w as f64,
		h: h as f64,
		rot: 0.0,
	};
	sample(
		&mut Target {
			pix: &mut dst.pix,
			width: w,
			height: h,
			rows: 0..h,
		},
		cur,
		&p,
		255,
		BlendMode::Normal,
		None,
	);
	dst.opaque = cur.opaque;
	dst
}

/// Averages each 2x2 block, rounding to nearest; an odd last row or column is dropped.
fn halve(src: &Image) -> Image {
	let (w, h) = (src.w / 2, src.h / 2);
	let mut out = vec![0_u8; w * h * 4];
	let stride = src.w * 4;
	for y in 0..h {
		for x in 0..w {
			let i = 2 * y * stride + 2 * x * 4;
			let o = (y * w + x) * 4;
			for k in 0..4 {
				let sum = u32::from(src.pix[i + k])
					+ u32::from(src.pix[i + 4 + k])
					+ u32::from(src.pix[i + stride + k])
					+ u32::from(src.pix[i + stride + 4 + k]);
				out[o + k] = ((sum + 2) >> 2).min(255) as u8;
			}
		}
	}
	Image {
		pix: out,
		w,
		h,
		opaque: src.opaque,
	}
}
