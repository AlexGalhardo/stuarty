//! The quad of one layer: where the GPU draws it and which texel each corner reads.

use stuarty_renderer::compose::{Placement, copy_origin};
use stuarty_renderer::math::{mul, sincos};

/// The four corners of a layer, in output pixels (x right, y down), with the texel position each
/// reads, in the order top-left, top-right, bottom-right, bottom-left. These are the numbers the
/// GPU is given.
#[derive(Clone, Copy, Debug, PartialEq)]
pub struct Quad {
	pub pos: [[f32; 2]; 4],
	pub uv: [[f32; 2]; 4],
}

/// The quad of a prepared layer whose texture is `src_w x src_h` texels. A placement that the CPU
/// path draws as a one-to-one copy is a rectangle at the rounded integer position, so the GPU
/// reads each texel at its own pixel; every other placement is the rectangle turned about its centre,
/// computed in `f64` with the sine and cosine of the CPU path.
pub fn quad_for(src_w: usize, src_h: usize, p: &Placement) -> Quad {
	let (sw, sh) = (src_w as f64, src_h as f64);
	let uv = [[0.0, 0.0], [sw, 0.0], [sw, sh], [0.0, sh]].map(|[u, v]| [u as f32, v as f32]);
	if let Some((x0, y0)) = copy_origin(src_w, src_h, p) {
		let (x0, y0) = (x0 as f64, y0 as f64);
		let pos = [[x0, y0], [x0 + sw, y0], [x0 + sw, y0 + sh], [x0, y0 + sh]]
			.map(|[x, y]| [x as f32, y as f32]);
		return Quad { pos, uv };
	}
	let (sin, cos) = sincos(mul(p.rot, core::f64::consts::PI / 180.0));
	let (hw, hh) = (p.w / 2.0, p.h / 2.0);
	let corner = |lu: f64, lv: f64| {
		// The inverse of the sampler's map: a point `(lu, lv)` of the rectangle, measured from its
		// centre, is drawn at the centre plus the same vector turned clockwise by `rot`.
		let x = p.cx + mul(cos, lu) - mul(sin, lv);
		let y = p.cy + mul(sin, lu) + mul(cos, lv);
		[x as f32, y as f32]
	};
	Quad {
		pos: [
			corner(-hw, -hh),
			corner(hw, -hh),
			corner(hw, hh),
			corner(-hw, hh),
		],
		uv,
	}
}
