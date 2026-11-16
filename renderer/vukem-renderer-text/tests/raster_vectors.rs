//! The Rust rasterizer against `engine/raster`, byte for byte.
//!
//! `renderer/testdata/raster-vectors.bin` is written by `engine/raster/vectors_test.go`: each case
//! holds a spec and the bitmap the Go rasterizer draws for it. A single differing byte is a failure
//! (rule `.agents/skills/renderer-parity`).

use vukem_renderer_text::Colour;
use vukem_renderer_text::shape::{Shape, ShapeKind, render_shape};

const VECTORS: &[u8] = include_bytes!("../../testdata/raster-vectors.bin");

struct Reader<'a>(&'a [u8]);

impl<'a> Reader<'a> {
	fn take(&mut self, n: usize) -> &'a [u8] {
		let (head, tail) = self.0.split_at(n);
		self.0 = tail;
		head
	}
	fn u32(&mut self) -> usize {
		u32::from_le_bytes(self.take(4).try_into().unwrap()) as usize
	}
	fn f64(&mut self) -> f64 {
		f64::from_bits(u64::from_le_bytes(self.take(8).try_into().unwrap()))
	}
	fn str(&mut self) -> String {
		let n = self.u32();
		String::from_utf8(self.take(n).to_vec()).unwrap()
	}
	fn colour(&mut self) -> Colour {
		let c = self.take(4);
		Colour {
			r: c[0],
			g: c[1],
			b: c[2],
			a: c[3],
		}
	}
}

#[test]
fn every_shape_vector_is_the_bytes_of_the_go_rasterizer() {
	let mut r = Reader(VECTORS);
	assert_eq!(r.take(4), b"VRAS");
	assert_eq!(r.u32(), 1, "vector file version");
	let n = r.u32();
	assert!(n >= 10, "{n} vectors");
	let mut shapes = 0;
	for _ in 0..n {
		let name = r.str();
		let kind = r.u32();
		assert_eq!(kind, 0, "{name}: only shapes so far");
		let shape_kind = match r.u32() {
			0 => ShapeKind::Rect,
			1 => ShapeKind::RoundedRect,
			_ => ShapeKind::Ellipse,
		};
		let (width, height, radius, stroke_width) = (r.f64(), r.f64(), r.f64(), r.f64());
		let (fill, stroke_color) = (r.colour(), r.colour());
		let scale = r.f64();
		let sh = Shape {
			kind: shape_kind,
			width,
			height,
			radius,
			fill,
			stroke_color,
			stroke_width,
		};
		let (w, h) = (r.u32(), r.u32());
		let (nw, nh, cdx, cdy) = (r.f64(), r.f64(), r.f64(), r.f64());
		let pix = r.take(w * h * 4);

		let got = render_shape(&sh, scale).unwrap();
		assert_eq!((got.width, got.height), (w, h), "{name}: size");
		assert_eq!(
			(got.natural_w, got.natural_h, got.center_dx, got.center_dy),
			(nw, nh, cdx, cdy),
			"{name}: metrics"
		);
		let first = got.pix.iter().zip(pix).position(|(a, b)| a != b);
		assert_eq!(first, None, "{name}: first differing byte");
		shapes += 1;
	}
	assert_eq!(shapes, n);
	assert!(r.0.is_empty(), "trailing bytes");
}
