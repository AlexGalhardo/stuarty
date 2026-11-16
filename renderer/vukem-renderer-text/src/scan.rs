//! The scanline rasterizer of polygons: the port of `engine/raster/internal/scan`, itself the
//! floating-point path of `golang.org/x/image/vector` (BSD-3, Copyright 2016 The Go Authors).
//!
//! The area is accumulated in `f32`, every product is rounded on its own (Rust never fuses), and the
//! accumulation into 8-bit coverage is a plain loop, so the bits are the same on every target.

const ALMOST_256_BITS: u32 = 0x437f_ffff;

/// Accumulates the signed area of line segments, then converts it into coverage.
#[derive(Default)]
pub struct Rasterizer {
	buf: Vec<f32>,
	w: i32,
	h: i32,
	first_x: f32,
	first_y: f32,
	pen_x: f32,
	pen_y: f32,
}

impl Rasterizer {
	pub fn reset(&mut self, w: usize, h: usize) {
		self.w = w as i32;
		self.h = h as i32;
		self.first_x = 0.0;
		self.first_y = 0.0;
		self.pen_x = 0.0;
		self.pen_y = 0.0;
		self.buf.clear();
		self.buf.resize(w * h, 0.0);
	}

	pub fn move_to(&mut self, ax: f32, ay: f32) {
		self.first_x = ax;
		self.first_y = ay;
		self.pen_x = ax;
		self.pen_y = ay;
	}

	pub fn close_path(&mut self) {
		self.line_to(self.first_x, self.first_y);
	}

	/// Writes the coverage of every pixel, 0 to 255, into `dst` (`w * h` values). It reports false
	/// when `dst` is shorter.
	pub fn coverage(&self, dst: &mut [u8]) -> bool {
		if dst.len() < self.buf.len() {
			return false;
		}
		let almost256 = f32::from_bits(ALMOST_256_BITS);
		let mut acc = 0.0_f32;
		for (d, v) in dst.iter_mut().zip(&self.buf) {
			acc += *v;
			let a = acc.abs().min(1.0);
			*d = (almost256 * a) as u8;
		}
		true
	}

	/// Adds a line segment from the pen to `(bx, by)` and moves the pen there.
	pub fn line_to(&mut self, bx: f32, by: f32) {
		let (mut ax, mut ay) = (self.pen_x, self.pen_y);
		let (mut bx, mut by) = (bx, by);
		self.pen_x = bx;
		self.pen_y = by;
		let mut dir = 1.0_f32;
		if ay > by {
			dir = -1.0;
			std::mem::swap(&mut ax, &mut bx);
			std::mem::swap(&mut ay, &mut by);
		}
		// A segment this close to horizontal changes no coverage and 1 / (by - ay) is unstable.
		if by - ay <= 0.000_001 {
			return;
		}
		let dxdy = (bx - ax) / (by - ay);

		let mut x = ax;
		let mut y = ay.floor() as i32;
		let y_max = (by.ceil() as i32).min(self.h);
		let width = self.w;

		while y < y_max {
			let dy = ((y + 1) as f32).min(by) - (y as f32).max(ay);
			let x_next = x + dy * dxdy;
			if y < 0 {
				x = x_next;
				y += 1;
				continue;
			}
			let base = (y * width) as usize;
			let d = dy * dir;
			let (mut x0, mut x1) = (x, x_next);
			if x > x_next {
				std::mem::swap(&mut x0, &mut x1);
			}
			let x0i = x0.floor() as i32;
			let x0_floor = x0i as f32;
			let x1i = x1.ceil() as i32;
			let x1_ceil = x1i as f32;

			if x1i <= x0i + 1 {
				let xmf = 0.5 * (x + x_next) - x0_floor;
				self.add(base, x0i, d - d * xmf);
				self.add(base, x0i + 1, d * xmf);
			} else {
				let s = 1.0 / (x1 - x0);
				let x0f = x0 - x0_floor;
				let one_minus_x0f = 1.0 - x0f;
				let a0 = 0.5 * s * one_minus_x0f * one_minus_x0f;
				let x1f = x1 - x1_ceil + 1.0;
				let am = 0.5 * s * x1f * x1f;

				self.add(base, x0i, d * a0);

				if x1i == x0i + 2 {
					self.add(base, x0i + 1, d * (1.0 - a0 - am));
				} else {
					let a1 = s * (1.5 - x0f);
					self.add(base, x0i + 1, d * (a1 - a0));
					let d_times_s = d * s;
					let mut xi = x0i + 2;
					while xi < x1i - 1 {
						self.add(base, xi, d_times_s);
						xi += 1;
					}
					let a2 = a1 + s * ((x1i - x0i - 3) as f32);
					self.add(base, x1i - 1, d * (1.0 - a2 - am));
				}

				self.add(base, x1i, d * am);
			}

			x = x_next;
			y += 1;
		}
	}

	/// Adds `v` at column `i` of the row starting at `base`. The column is clamped to the width, so
	/// one past the last column lands on the first pixel of the next row, as in the Go code, and is
	/// dropped on the last row.
	fn add(&mut self, base: usize, i: i32, v: f32) {
		let c = i.clamp(0, self.w) as usize;
		if let Some(p) = self.buf.get_mut(base + c) {
			*p += v;
		}
	}
}

#[cfg(test)]
mod tests {
	use super::*;

	fn fill(w: usize, h: usize, poly: &[(f32, f32)]) -> Vec<u8> {
		let mut z = Rasterizer::default();
		z.reset(w, h);
		z.move_to(poly[0].0, poly[0].1);
		for p in &poly[1..] {
			z.line_to(p.0, p.1);
		}
		z.close_path();
		let mut out = vec![0; w * h];
		assert!(z.coverage(&mut out));
		out
	}

	#[test]
	fn a_pixel_aligned_square_is_fully_covered_inside_and_empty_outside() {
		let c = fill(4, 4, &[(1.0, 1.0), (3.0, 1.0), (3.0, 3.0), (1.0, 3.0)]);
		for y in 0..4 {
			for x in 0..4 {
				let want = if (1..3).contains(&x) && (1..3).contains(&y) {
					255
				} else {
					0
				};
				assert_eq!(c[y * 4 + x], want, "({x}, {y})");
			}
		}
	}

	#[test]
	fn a_half_covered_column_is_half_coverage() {
		let c = fill(3, 2, &[(0.5, 0.0), (1.0, 0.0), (1.0, 2.0), (0.5, 2.0)]);
		assert_eq!(&c[0..3], &[127, 0, 0]);
	}

	#[test]
	fn a_short_destination_is_refused() {
		let mut z = Rasterizer::default();
		z.reset(2, 2);
		assert!(!z.coverage(&mut [0; 3]));
	}
}
