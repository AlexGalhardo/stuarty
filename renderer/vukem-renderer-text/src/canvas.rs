//! The canvas, the coverage mask and the box blur: the port of `engine/raster/draw.go` (canvas,
//! blend, `boxBlur3`) and the painter of `geom.go`. Integer arithmetic only.

use crate::geom::{Point, add_polygon};
use crate::scan::Rasterizer;
use crate::{Colour, InvalidSpec, MAX_PIXELS};

/// An 8-bit coverage image.
pub struct Mask {
	pub w: usize,
	pub h: usize,
	pub pix: Vec<u8>,
}

/// An RGBA bitmap with straight alpha.
pub struct Canvas {
	pub w: usize,
	pub h: usize,
	pub pix: Vec<u8>,
}

impl Canvas {
	pub fn new(w: usize, h: usize) -> Result<Self, InvalidSpec> {
		if w == 0 || h == 0 || w.checked_mul(h).is_none_or(|n| n > MAX_PIXELS) {
			return Err(InvalidSpec(format!("bitmap of {w} by {h} pixels")));
		}
		Ok(Self {
			w,
			h,
			pix: vec![0; w * h * 4],
		})
	}

	/// Composites the colour through the coverage mask onto the canvas.
	pub fn over(&mut self, m: &Mask, col: Colour) {
		if col.a == 0 {
			return;
		}
		for (i, &cov) in m.pix.iter().enumerate() {
			if cov == 0 {
				continue;
			}
			let sa = (u32::from(cov) * u32::from(col.a) + 127) / 255;
			self.blend(i * 4, col, sa);
		}
	}

	/// Composites the colour with alpha `sa` (0 to 255) onto the pixel at offset `o`, straight alpha.
	fn blend(&mut self, o: usize, col: Colour, sa: u32) {
		if sa == 0 {
			return;
		}
		let d = &mut self.pix[o..o + 4];
		let da = u32::from(d[3]);
		// Over an empty pixel, or with an opaque source, the result is the source.
		if da == 0 || sa == 255 {
			d[0] = col.r;
			d[1] = col.g;
			d[2] = col.b;
			d[3] = sa as u8;
			return;
		}
		let keep = (da * (255 - sa) + 127) / 255;
		let oa = sa + keep;
		d[0] = mix(u32::from(col.r), sa, u32::from(d[0]), keep, oa);
		d[1] = mix(u32::from(col.g), sa, u32::from(d[1]), keep, oa);
		d[2] = mix(u32::from(col.b), sa, u32::from(d[2]), keep, oa);
		d[3] = oa as u8;
	}
}

fn mix(s: u32, sa: u32, d: u32, keep: u32, oa: u32) -> u8 {
	((s * sa + d * keep + oa / 2) / oa) as u8
}

/// Rasterizes polygons into one reusable mask.
pub struct Painter {
	w: usize,
	h: usize,
	rz: Rasterizer,
	m: Mask,
}

impl Painter {
	pub fn new(w: usize, h: usize) -> Self {
		Self {
			w,
			h,
			rz: Rasterizer::default(),
			m: Mask {
				w,
				h,
				pix: vec![0; w * h],
			},
		}
	}

	/// Rasterizes the polygons, moved by `off`, into the mask. The polygons share one accumulation
	/// buffer, so overlapping polygons of the same orientation union and opposite ones cut holes.
	/// The mask is valid until the next call.
	pub fn fill(&mut self, off: Point, polys: &[&[Point]]) -> &mut Mask {
		self.m.pix.fill(0);
		if self.w == 0 || self.h == 0 {
			return &mut self.m;
		}
		self.rz.reset(self.w, self.h);
		for poly in polys {
			add_polygon(&mut self.rz, poly, off);
		}
		self.rz.coverage(&mut self.m.pix);
		&mut self.m
	}
}

/// Blurs the mask with a box blur of the given radius applied three times, in place.
pub fn box_blur3(m: &mut Mask, radius: usize) {
	if radius == 0 {
		return;
	}
	let mut tmp = vec![0_u8; m.pix.len()];
	for _ in 0..3 {
		blur_rows(&m.pix, &mut tmp, m.w, m.h, radius);
		blur_columns(&tmp, &mut m.pix, m.w, m.h, radius);
	}
}

fn blur_rows(src: &[u8], dst: &mut [u8], w: usize, h: usize, r: usize) {
	let div = 2 * r + 1;
	for y in 0..h {
		let row = &src[y * w..(y + 1) * w];
		let out = &mut dst[y * w..(y + 1) * w];
		let mut sum = 0_usize;
		for v in row.iter().take(r + 1) {
			sum += usize::from(*v);
		}
		for x in 0..w {
			out[x] = ((sum + div / 2) / div) as u8;
			if x >= r {
				sum -= usize::from(row[x - r]);
			}
			if x + r + 1 < w {
				sum += usize::from(row[x + r + 1]);
			}
		}
	}
}

fn blur_columns(src: &[u8], dst: &mut [u8], w: usize, h: usize, r: usize) {
	let div = 2 * r + 1;
	let mut sums = vec![0_usize; w];
	for y in 0..=r.min(h.saturating_sub(1)) {
		for (s, v) in sums.iter_mut().zip(&src[y * w..(y + 1) * w]) {
			*s += usize::from(*v);
		}
	}
	for y in 0..h {
		for (o, s) in dst[y * w..(y + 1) * w].iter_mut().zip(&sums) {
			*o = ((s + div / 2) / div) as u8;
		}
		if y >= r {
			let old = y - r;
			for (s, v) in sums.iter_mut().zip(&src[old * w..(old + 1) * w]) {
				*s -= usize::from(*v);
			}
		}
		if y + r + 1 < h {
			let new = y + r + 1;
			for (s, v) in sums.iter_mut().zip(&src[new * w..(new + 1) * w]) {
				*s += usize::from(*v);
			}
		}
	}
}

#[cfg(test)]
mod tests {
	use super::*;

	#[test]
	fn a_blur_spreads_a_dot() {
		let mut m = Mask {
			w: 9,
			h: 9,
			pix: vec![0; 81],
		};
		m.pix[4 * 9 + 4] = 255;
		box_blur3(&mut m, 1);
		assert!(m.pix[4 * 9 + 4] > 0 && m.pix[4 * 9 + 4] < 255);
		assert!(m.pix[3 * 9 + 3] > 0);
		assert_eq!(m.pix[0], 0);
	}

	#[test]
	fn over_an_empty_pixel_the_result_is_the_source() {
		let mut c = Canvas::new(1, 1).unwrap();
		let m = Mask {
			w: 1,
			h: 1,
			pix: vec![255],
		};
		c.over(
			&m,
			Colour {
				r: 10,
				g: 20,
				b: 30,
				a: 128,
			},
		);
		assert_eq!(c.pix, vec![10, 20, 30, 128]);
	}

	#[test]
	fn a_canvas_of_no_pixels_is_refused() {
		assert!(Canvas::new(0, 4).is_err());
	}
}
