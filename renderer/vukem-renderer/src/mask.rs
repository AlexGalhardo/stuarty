//! Masks (SDD-004 section 3.9, port of `engine/compositor/mask.go`).
//!
//! The coverage of a destination pixel inside the layer's own unrotated rectangle, in integer
//! arithmetic only, so that every executor draws the same bytes. A value outside the range of a
//! valid plan is clamped; only an invalid plan reaches the clamps.

use crate::compose::Placement;
use crate::effects::{Direction, Transition};
use crate::math::{div255, mul, round_half_away, round_half_up, sincos};

/// Fractional bits of a position inside the rectangle: 24.8 fixed point.
const MASK_FIX: u32 = 8;
/// Fractional bits of the sampler coordinates.
const FIX: u32 = 32;
/// The largest size, feather or position (24.8) the arithmetic is held to.
const LIMIT: f64 = 4_294_967_296.0;
/// A position is held to this many 24.8 units either side of the rectangle.
const POS_LIMIT: i64 = 1 << 40;

/// The shape of a mask.
#[derive(Clone, Copy, Debug, PartialEq, Eq)]
pub enum MaskShape {
	Rect,
	Ellipse,
}

/// The mask of a layer as the plan carries it: fractions of the layer's rectangle and a feather in
/// project pixels.
#[derive(Clone, Copy, Debug, PartialEq)]
pub struct MaskSpec {
	pub shape: MaskShape,
	pub x: f64,
	pub y: f64,
	pub width: f64,
	pub height: f64,
	pub feather: f64,
	pub invert: bool,
}

/// The mask in floating point for the GPU path (SDD-013 section 3.8): the shape in pixels of the
/// layer's own rectangle and the map from a destination pixel to a position in it. The GPU computes
/// the same coverage as [`MaskState::coverage`] from these, within the tolerance of that section.
#[derive(Clone, Copy, Debug, PartialEq)]
pub struct MaskGeometry {
	pub ellipse: bool,
	pub invert: bool,
	/// Left, top and the two half-extents of the shape, in pixels from the corner of the rectangle.
	pub left: f64,
	pub top: f64,
	pub half_w: f64,
	pub half_h: f64,
	/// The soft edge in pixels; 0 is a hard edge.
	pub feather: f64,
	/// `u = map[0] x + map[1] y + map[2]` and `v = map[3] x + map[4] y + map[5]` for the integer
	/// pixel index `(x, y)`: the position of the pixel centre in the rectangle, in pixels.
	pub map: [f64; 6],
	/// There is no shape, only a wipe: the coverage of the mask is 1 everywhere.
	pub shapeless: bool,
	/// The wipe of the layer's transition (SDD-004 section 3.2): its edge in pixels along `u`
	/// (`Left`, `Right`) or `v` (`Up`, `Down`), and the direction; `None` when there is no wipe.
	pub wipe: Option<(Direction, f64)>,
}

/// A mask ready to give a coverage for a destination pixel.
#[derive(Clone, Copy, Debug)]
pub struct MaskState {
	/// Only a wipe: the coverage of the mask is 255 everywhere.
	shapeless: bool,
	wipe: Option<WipeState>,
	ellipse: bool,
	invert: bool,
	l: i64,
	t: i64,
	r: i64,
	b: i64,
	ha: i64,
	hb: i64,
	fq: i64,
	left: i64,
	top: i64,
	ax: i64,
	bx: i64,
	ox: i64,
	ay: i64,
	by: i64,
	oy: i64,
}

/// `v` rounded half away from zero and held to `0..=hi`; NaN is 0.
fn clamped(v: f64, hi: f64) -> i64 {
	if v.is_nan() || v <= 0.0 {
		0
	} else if v > hi {
		hi as i64
	} else {
		round_half_away(v)
	}
}

/// The size of a layer's rectangle in 24.8 fixed point, held to the limit of the arithmetic.
pub fn clamped_q(v: f64) -> i64 {
	clamped(mul(v, 256.0), LIMIT)
}

/// The hard edge of a wipe on the layer's rectangle, in 24.8.
#[derive(Clone, Copy, Debug)]
struct WipeState {
	dir: Direction,
	r: i64,
	wq: i64,
	hq: i64,
}

impl WipeState {
	/// 255 where the edge has passed and 0 where it has not.
	fn coverage(&self, lx: i64, ly: i64) -> u32 {
		let covered = match self.dir {
			Direction::Left => lx < self.r,
			Direction::Right => lx >= self.wq - self.r,
			Direction::Up => ly < self.r,
			Direction::Down => ly >= self.hq - self.r,
		};
		if covered { 255 } else { 0 }
	}
}

impl MaskState {
	/// The mask of `spec` (none for a wipe alone) and the wipe, for a layer placed as `p`: the
	/// placement before any blur grows it, so a blur halo never moves the mask.
	pub fn new(
		spec: Option<&MaskSpec>,
		wipe: Option<&Transition>,
		p: &Placement,
		out_scale: f64,
	) -> Self {
		let wq = clamped_q(p.w);
		let hq = clamped_q(p.h);
		let default = MaskSpec {
			shape: MaskShape::Rect,
			x: 0.0,
			y: 0.0,
			width: 1.0,
			height: 1.0,
			feather: 0.0,
			invert: false,
		};
		let shapeless = spec.is_none();
		let spec = spec.unwrap_or(&default);
		let xm = clamped(mul(spec.x, 1000.0), 1000.0);
		let ym = clamped(mul(spec.y, 1000.0), 1000.0);
		let wm = clamped(mul(spec.width, 1000.0), 1000.0);
		let hm = clamped(mul(spec.height, 1000.0), 1000.0);

		let l = (xm * wq + 500) / 1000;
		let t = (ym * hq + 500) / 1000;
		let ha = ((wm * wq + 1000) / 2000).max(1);
		let hb = ((hm * hq + 1000) / 2000).max(1);
		let fq = clamped(mul(mul(spec.feather, out_scale), 256.0), LIMIT);
		let wipe = wipe.map(|w| {
			let size = match w.direction {
				Direction::Up | Direction::Down => hq,
				_ => wq,
			};
			WipeState {
				dir: w.direction,
				r: (w.p.clamp(0, 65536) * size + 32768) >> 16,
				wq,
				hq,
			}
		});

		let (sin, cos) = sincos(mul(p.rot, core::f64::consts::PI / 180.0));
		let (hw, hh) = (p.w / 2.0, p.h / 2.0);
		let (dx0, dy0) = (0.5 - p.cx, 0.5 - p.cy);
		let one = (1_u64 << FIX) as f64;
		let q = |v: f64| round_half_away(mul(v, one));

		Self {
			shapeless,
			wipe,
			ellipse: spec.shape == MaskShape::Ellipse,
			invert: spec.invert,
			l,
			t,
			r: l + 2 * ha,
			b: t + 2 * hb,
			ha,
			hb,
			fq,
			left: round_half_up(p.cx - p.w / 2.0),
			top: round_half_up(p.cy - p.h / 2.0),
			ax: q(cos),
			bx: q(sin),
			ox: q(mul(dx0, cos) + mul(dy0, sin) + hw),
			ay: q(-sin),
			by: q(cos),
			oy: q(-mul(dx0, sin) + mul(dy0, cos) + hh),
		}
	}

	/// The mask for the GPU; `copy` says the layer is drawn as a one-to-one copy.
	pub fn geometry(&self, copy: bool) -> MaskGeometry {
		let px = |v: i64| v as f64 / f64::from(1 << MASK_FIX);
		let fix = (1_u64 << FIX) as f64;
		let map = if copy {
			[
				1.0,
				0.0,
				0.5 - self.left as f64,
				0.0,
				1.0,
				0.5 - self.top as f64,
			]
		} else {
			[
				self.ax as f64 / fix,
				self.bx as f64 / fix,
				self.ox as f64 / fix,
				self.ay as f64 / fix,
				self.by as f64 / fix,
				self.oy as f64 / fix,
			]
		};
		MaskGeometry {
			ellipse: self.ellipse,
			invert: self.invert,
			left: px(self.l),
			top: px(self.t),
			half_w: px(self.ha),
			half_h: px(self.hb),
			feather: px(self.fq),
			map,
			shapeless: self.shapeless,
			wipe: self.wipe.map(|w| {
				let edge = match w.dir {
					Direction::Left | Direction::Up => w.r,
					Direction::Right => w.wq - w.r,
					Direction::Down => w.hq - w.r,
				};
				(w.dir, px(edge))
			}),
		}
	}

	/// The coverage of the destination pixel `(x, y)` of a copied layer.
	pub fn copy_cov(&self, x: i64, y: i64) -> u32 {
		let lx = x
			.wrapping_sub(self.left)
			.wrapping_shl(MASK_FIX)
			.wrapping_add(128);
		let ly = y
			.wrapping_sub(self.top)
			.wrapping_shl(MASK_FIX)
			.wrapping_add(128);
		self.coverage(lx, ly)
	}

	/// The coverage of the destination pixel `(x, y)` of a sampled layer.
	pub fn sample_cov(&self, x: i64, y: i64) -> u32 {
		let half = 1_i64 << (FIX - MASK_FIX - 1);
		let u = self
			.bx
			.wrapping_mul(y)
			.wrapping_add(self.ox)
			.wrapping_add(self.ax.wrapping_mul(x));
		let v = self
			.by
			.wrapping_mul(y)
			.wrapping_add(self.oy)
			.wrapping_add(self.ay.wrapping_mul(x));
		self.coverage(
			u.wrapping_add(half) >> (FIX - MASK_FIX),
			v.wrapping_add(half) >> (FIX - MASK_FIX),
		)
	}

	/// The 0..255 coverage at `(lx, ly)` in 24.8 inside the rectangle.
	pub fn coverage(&self, lx: i64, ly: i64) -> u32 {
		let lx = lx.clamp(-POS_LIMIT, POS_LIMIT);
		let ly = ly.clamp(-POS_LIMIT, POS_LIMIT);
		let mut cov = if self.shapeless {
			255
		} else {
			self.shape_coverage(lx, ly)
		};
		if let Some(w) = &self.wipe {
			cov = div255(cov * w.coverage(lx, ly));
		}
		cov
	}

	/// The coverage of the mask's shape alone.
	fn shape_coverage(&self, lx: i64, ly: i64) -> u32 {
		let d = if self.ellipse {
			let qx = (((lx - (self.l + self.ha)).abs() << 16) / self.ha).min(1 << 22);
			let qy = (((ly - (self.t + self.hb)).abs() << 16) / self.hb).min(1 << 22);
			let r = isqrt((qx * qx + qy * qy) as u64) as i64;
			((65536 - r) * self.ha.min(self.hb)) >> 16
		} else {
			(lx - self.l)
				.min(self.r - lx)
				.min(ly - self.t)
				.min(self.b - ly)
		};

		let mut cov = if self.fq == 0 {
			if d >= 0 { 255 } else { 0 }
		} else {
			let t = (2 * d + self.fq).clamp(0, 2 * self.fq);
			(255 * t + self.fq) / (2 * self.fq)
		};
		if self.invert {
			cov = 255 - cov;
		}
		cov as u32
	}
}

/// The premultiplied pixel multiplied by a coverage, one `div255` per channel.
pub fn scale(px: &mut [u32; 4], cov: u32) {
	if cov == 255 {
		return;
	}
	for v in px.iter_mut() {
		*v = div255(*v * cov);
	}
}

/// The largest `x` with `x * x <= n`.
pub(crate) fn isqrt(n: u64) -> u64 {
	if n < 2 {
		return n;
	}
	let mut x = 1_u64 << ((64 - n.leading_zeros()).div_ceil(2));
	loop {
		let y = (x + n / x) / 2;
		if y >= x {
			return x;
		}
		x = y;
	}
}

#[cfg(test)]
mod tests {
	use super::*;

	fn state(spec: MaskSpec, p: Placement) -> MaskState {
		MaskState::new(Some(&spec), None, &p, 1.0)
	}

	fn spec(shape: MaskShape) -> MaskSpec {
		MaskSpec {
			shape,
			x: 0.0,
			y: 0.0,
			width: 1.0,
			height: 1.0,
			feather: 0.0,
			invert: false,
		}
	}

	fn place(cx: f64, cy: f64, w: f64, h: f64) -> Placement {
		Placement {
			cx,
			cy,
			w,
			h,
			rot: 0.0,
		}
	}

	#[test]
	fn a_hard_rectangle_covers_its_columns() {
		let m = state(
			MaskSpec {
				x: 0.25,
				width: 0.5,
				..spec(MaskShape::Rect)
			},
			place(6.0, 5.0, 8.0, 6.0),
		);
		for x in 0..8 {
			let want = if (2..=5).contains(&x) { 255 } else { 0 };
			assert_eq!(m.copy_cov(2 + x, 3), want, "column {x}");
		}
	}

	#[test]
	fn the_ellipse_is_full_at_the_centre_and_empty_at_the_corner_and_invert_complements() {
		let p = place(20.0, 20.0, 40.0, 40.0);
		let m = state(spec(MaskShape::Ellipse), p);
		let inv = state(
			MaskSpec {
				invert: true,
				..spec(MaskShape::Ellipse)
			},
			p,
		);
		assert_eq!(m.copy_cov(20, 20), 255);
		assert_eq!(m.copy_cov(0, 0), 0);
		for y in 0..40 {
			for x in 0..40 {
				assert_eq!(m.copy_cov(x, y) + inv.copy_cov(x, y), 255);
			}
		}
	}

	#[test]
	fn the_feather_is_half_on_the_edge() {
		let m = state(
			MaskSpec {
				feather: 8.0,
				..spec(MaskShape::Rect)
			},
			place(20.0, 20.0, 40.0, 40.0),
		);
		assert_eq!(m.coverage(m.l, 20 << 8), 128);
		assert_eq!(m.coverage(m.l - 4 * 256, 20 << 8), 0);
		assert_eq!(m.coverage(m.l + 4 * 256, 20 << 8), 255);
	}

	#[test]
	fn the_sampler_position_equals_the_copy_one_in_place() {
		let m = state(
			MaskSpec {
				x: 0.1,
				y: 0.2,
				width: 0.7,
				height: 0.6,
				feather: 3.0,
				..spec(MaskShape::Ellipse)
			},
			place(30.0, 25.0, 40.0, 20.0),
		);
		for y in 10..40 {
			for x in 5..60 {
				assert_eq!(m.copy_cov(x, y), m.sample_cov(x, y), "({x}, {y})");
			}
		}
	}

	#[test]
	fn the_gpu_map_gives_the_position_of_the_cpu_path() {
		let p = Placement {
			rot: 20.0,
			..place(30.0, 25.0, 40.0, 20.0)
		};
		let m = state(spec(MaskShape::Ellipse), p);
		let copy = state(spec(MaskShape::Rect), place(30.0, 25.0, 40.0, 20.0));
		for (mask, is_copy) in [(&m, false), (&copy, true)] {
			let g = mask.geometry(is_copy);
			for (x, y) in [(12_i64, 15_i64), (30, 25), (47, 33)] {
				let (u, v) = (
					g.map[0] * x as f64 + g.map[1] * y as f64 + g.map[2],
					g.map[3] * x as f64 + g.map[4] * y as f64 + g.map[5],
				);
				let (lx, ly) = if is_copy {
					((x - mask.left) * 256 + 128, (y - mask.top) * 256 + 128)
				} else {
					let half = 1_i64 << 23;
					(
						(mask.bx * y + mask.ox + mask.ax * x + half) >> 24,
						(mask.by * y + mask.oy + mask.ay * x + half) >> 24,
					)
				};
				assert!((u * 256.0 - lx as f64).abs() <= 1.0, "u at ({x}, {y})");
				assert!((v * 256.0 - ly as f64).abs() <= 1.0, "v at ({x}, {y})");
			}
		}
	}

	#[test]
	fn absurd_numbers_are_clamped_not_a_panic() {
		let huge = MaskSpec {
			x: f64::NAN,
			y: -3.0,
			width: 1e300,
			height: f64::INFINITY,
			feather: 1e300,
			..spec(MaskShape::Ellipse)
		};
		let m = MaskState::new(
			Some(&huge),
			None,
			&place(1e300, -1e300, 1e300, 1e-300),
			1e300,
		);
		for (x, y) in [(0, 0), (i64::MAX, i64::MIN), (-5, 1 << 40)] {
			assert!(m.copy_cov(x, y) <= 255);
			assert!(m.sample_cov(x, y) <= 255);
		}
	}
}
