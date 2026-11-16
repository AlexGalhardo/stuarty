//! The GPU path against the CPU path on every golden vector (SDD-013 section 3.8, skill
//! `renderer-parity` gate 2): SSIM of at least 0.99 on the luma of the whole frame, and a drawn
//! rectangle within 0.5 pixel of the CPU path's, with no displacement of the frame as a whole.
//!
//! The vectors are the 24 compositions of `renderer/testdata/compose-vectors.bin`, each with the
//! frame the Go compositor draws, which the CPU path equals byte for byte. The adapter is printed
//! next to every result; run with `--nocapture` to read the table.

#[path = "support/ssim.rs"]
mod ssim;
mod support;
#[path = "../../vukem-renderer/tests/support/vectors.rs"]
mod vectors;

use vukem_renderer::compose::{Placement, Target, compose, copy_origin, prepare_layer};
use vukem_renderer::math::{mul, sincos};
use vukem_renderer_gpu::quad_for;

use ssim::{luma, ssim};
use vectors::{Case, layers_of, read_cases};

const MIN_SSIM: f64 = 0.99;
const MAX_CORNER_PIXELS: f64 = 0.5;
/// The search for a displacement of the whole frame, in whole pixels.
const SHIFT: i64 = 2;

fn cpu_frame(c: &Case) -> Vec<u8> {
	let mut pix = vec![0xAA_u8; c.w * c.h * 4];
	compose(
		&mut Target {
			pix: &mut pix,
			width: c.w,
			height: c.h,
			rows: 0..c.h,
		},
		c.bg,
		&layers_of(c),
		c.scale,
	);
	pix
}

/// The SSIM of two luma planes where `b` is read `(dx, dy)` pixels away, over the part both cover
/// once the search margin is cut off.
fn shifted_ssim(a: &[u8], b: &[u8], w: usize, h: usize, dx: i64, dy: i64) -> f64 {
	let m = SHIFT as usize;
	let (iw, ih) = (w - 2 * m, h - 2 * m);
	let mut ca = Vec::with_capacity(iw * ih);
	let mut cb = Vec::with_capacity(iw * ih);
	for y in 0..ih {
		for x in 0..iw {
			ca.push(a[(y + m) * w + x + m]);
			let (sx, sy) = ((x + m) as i64 + dx, (y + m) as i64 + dy);
			cb.push(b[sy as usize * w + sx as usize]);
		}
	}
	ssim(&ca, &cb, iw, ih)
}

/// The whole-pixel displacement of `gpu` against `cpu` that makes them most alike; ties go to
/// no displacement.
fn best_shift(cpu: &[u8], gpu: &[u8], w: usize, h: usize) -> (i64, i64) {
	let mut best = ((0, 0), shifted_ssim(cpu, gpu, w, h, 0, 0));
	for dy in -SHIFT..=SHIFT {
		for dx in -SHIFT..=SHIFT {
			let s = shifted_ssim(cpu, gpu, w, h, dx, dy);
			if s > best.1 + 1e-9 {
				best = ((dx, dy), s);
			}
		}
	}
	best.0
}

/// The corners of the rectangle the CPU sampler draws, found by running its inverse map, from the
/// constants of `compose::sample` (SDD-013 section 3.6), the other way round: for each corner of
/// the source, the output position whose centre it comes from. A one-to-one copy is the integer
/// rectangle.
fn cpu_corners(src_w: usize, src_h: usize, p: &Placement) -> [[f64; 2]; 4] {
	let (sw, sh) = (src_w as f64, src_h as f64);
	if let Some((x0, y0)) = copy_origin(src_w, src_h, p) {
		let (x0, y0) = (x0 as f64, y0 as f64);
		return [[x0, y0], [x0 + sw, y0], [x0 + sw, y0 + sh], [x0, y0 + sh]];
	}
	let (sin, cos) = sincos(mul(p.rot, core::f64::consts::PI / 180.0));
	let (hw, hh) = (p.w / 2.0, p.h / 2.0);
	let (kx, ky) = (sw / p.w, sh / p.h);
	let (dx0, dy0) = (0.5 - p.cx, 0.5 - p.cy);
	let (ax, bx) = (cos * kx, sin * kx);
	let (ay, by) = (-sin * ky, cos * ky);
	let ox = (dx0 * cos + dy0 * sin + hw) * kx;
	let oy = (-dx0 * sin + dy0 * cos + hh) * ky;
	let det = ax * by - bx * ay;
	[[0.0, 0.0], [sw, 0.0], [sw, sh], [0.0, sh]].map(|[u, v]| {
		let (rx, ry) = (u - ox, v - oy);
		// (x, y) with ax x + bx y = rx and ay x + by y = ry, x and y being pixel indices: the
		// centre of pixel (x, y) is (x + 0.5, y + 0.5).
		let x = (rx * by - bx * ry) / det;
		let y = (ax * ry - ay * rx) / det;
		[x + 0.5, y + 0.5]
	})
}

#[test]
fn every_golden_vector_is_within_the_gpu_tolerance_of_the_cpu_path() {
	let Some(gpu) = support::open() else { return };
	let info = gpu.adapter();
	let adapter = format!(
		"{} / {:?} / {:?}",
		info.name, info.device_type, info.backend
	);
	let (_, cases) = read_cases();
	assert!(cases.len() >= 20);

	let (mut worst_ssim, mut worst_corner) = (1.0_f64, 0.0_f64);
	let mut failures = Vec::new();
	for c in &cases {
		let want = cpu_frame(c);
		assert!(
			want == c.want,
			"{}: the CPU path is not the Go compositor",
			c.name
		);
		let mut got = vec![0xAA_u8; c.w * c.h * 4];
		let layers = layers_of(c);
		gpu.compose(
			&mut Target {
				pix: &mut got,
				width: c.w,
				height: c.h,
				rows: 0..c.h,
			},
			c.bg,
			&layers,
			c.scale,
		)
		.unwrap_or_else(|e| panic!("{}: {e}", c.name));

		let (lw, lg) = (luma(&want), luma(&got));
		let score = ssim(&lw, &lg, c.w, c.h);
		let shift = best_shift(&lw, &lg, c.w, c.h);

		let mut corner = 0.0_f64;
		for l in &layers {
			let Some(p) = prepare_layer(l, c.scale) else {
				continue;
			};
			let q = quad_for(p.image.w, p.image.h, &p.placement);
			let truth = cpu_corners(p.image.w, p.image.h, &p.placement);
			for (g, t) in q.pos.iter().zip(&truth) {
				let d = (f64::from(g[0]) - t[0]).hypot(f64::from(g[1]) - t[1]);
				corner = corner.max(d);
			}
		}
		eprintln!(
			"{adapter} | {:<34} ssim {score:.5} shift {shift:?} corner {corner:.4} px",
			c.name
		);

		worst_ssim = worst_ssim.min(score);
		worst_corner = worst_corner.max(corner);
		if score < MIN_SSIM {
			failures.push(format!("{}: SSIM {score:.5} below {MIN_SSIM}", c.name));
		}
		if shift != (0, 0) {
			failures.push(format!("{}: the frame is displaced by {shift:?}", c.name));
		}
		if corner > MAX_CORNER_PIXELS {
			failures.push(format!(
				"{}: a corner is {corner:.3} px from the CPU path's",
				c.name
			));
		}
	}
	eprintln!(
		"{adapter} | {} cases, worst SSIM {worst_ssim:.5}, worst corner {worst_corner:.4} px",
		cases.len()
	);
	assert!(failures.is_empty(), "{}", failures.join("\n"));
}

#[test]
fn the_tolerance_checks_are_not_vacuous() {
	// A frame drawn one pixel to the side must fail the displacement search, and a mismatched
	// frame must fall under the SSIM floor: the same routines judge the GPU frames above.
	let (_, cases) = read_cases();
	let c = cases.iter().find(|c| c.name == "scale-down").unwrap();
	let want = luma(&cpu_frame(c));
	let (w, h) = (c.w, c.h);

	let mut moved = want.clone();
	for y in 0..h {
		for x in 0..w {
			moved[y * w + x] = want[y * w + x.saturating_sub(1)];
		}
	}
	assert_eq!(best_shift(&want, &moved, w, h), (1, 0));

	let inverted: Vec<u8> = want.iter().map(|v| 255 - v).collect();
	assert!(ssim(&want, &inverted, w, h) < MIN_SSIM);
	assert!(ssim(&want, &want, w, h) > 0.999_999);
}
