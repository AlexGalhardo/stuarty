//! The CPU path against the Go compositor, byte for byte (SDD-013 section 3.10).
//!
//! `renderer/testdata/compose-vectors.bin` is written by `engine/compositor/vectors_test.go`: small
//! compositions with their inputs and the frame the Go compositor draws for each.

use vukem_renderer::compose::{Layer, Source, Target, compose, parse_colour, premultiply};
use vukem_renderer::image::Image;
use vukem_renderer::math::sincos;

const VECTORS: &[u8] = include_bytes!("../../testdata/compose-vectors.bin");

struct Reader<'a>(&'a [u8]);

impl<'a> Reader<'a> {
	fn take(&mut self, n: usize) -> &'a [u8] {
		let (a, b) = self.0.split_at(n);
		self.0 = b;
		a
	}
	fn u8(&mut self) -> u8 {
		self.take(1)[0]
	}
	fn u32(&mut self) -> u32 {
		u32::from_le_bytes(self.take(4).try_into().unwrap())
	}
	fn u64(&mut self) -> u64 {
		u64::from_le_bytes(self.take(8).try_into().unwrap())
	}
	fn f64(&mut self) -> f64 {
		f64::from_bits(self.u64())
	}
	fn matrix(&mut self) -> Option<[f64; 12]> {
		(self.u8() == 1).then(|| core::array::from_fn(|_| self.f64()))
	}
}

enum Pixels {
	Media(Image),
	Bitmap {
		image: Image,
		natural_w: f64,
		center_dx: f64,
		center_dy: f64,
	},
}

struct VecLayer {
	pixels: Pixels,
	nums: [f64; 7],
	colour: Option<[f64; 12]>,
	filter: Option<[f64; 12]>,
}

struct Case {
	name: String,
	w: usize,
	h: usize,
	scale: f64,
	bg: [u8; 4],
	layers: Vec<VecLayer>,
	want: Vec<u8>,
}

fn read_cases() -> (u64, Vec<Case>) {
	let mut r = Reader(VECTORS);
	assert_eq!(r.take(4), b"VVCV");
	assert_eq!(r.u32(), 1);
	let hash = r.u64();
	let n = r.u32();
	let mut cases = Vec::new();
	for _ in 0..n {
		let len = usize::from(r.u8());
		let name = String::from_utf8(r.take(len).to_vec()).unwrap();
		let (w, h) = (r.u32() as usize, r.u32() as usize);
		let scale = r.f64();
		let bg: [u8; 4] = r.take(4).try_into().unwrap();
		let nl = r.u32();
		let mut layers = Vec::new();
		for _ in 0..nl {
			let kind = r.u8();
			let (sw, sh) = (r.u32() as usize, r.u32() as usize);
			let opaque = r.u8() == 1;
			let pix = r.take(sw * sh * 4).to_vec();
			let pixels = if kind == 1 {
				let natural_w = r.f64();
				let (center_dx, center_dy) = (r.f64(), r.f64());
				Pixels::Bitmap {
					image: premultiply(&pix, sw, sh),
					natural_w,
					center_dx,
					center_dy,
				}
			} else {
				Pixels::Media(Image {
					pix,
					w: sw,
					h: sh,
					opaque,
				})
			};
			let nums = core::array::from_fn(|_| r.f64());
			let (colour, filter) = (r.matrix(), r.matrix());
			layers.push(VecLayer {
				pixels,
				nums,
				colour,
				filter,
			});
		}
		let want = r.take(w * h * 4).to_vec();
		cases.push(Case {
			name,
			w,
			h,
			scale,
			bg,
			layers,
			want,
		});
	}
	assert!(r.0.is_empty());
	(hash, cases)
}

fn render(c: &Case, bands: usize) -> Vec<u8> {
	let layers: Vec<Layer<'_>> = c
		.layers
		.iter()
		.map(|l| {
			let [cx, cy, w, h, rotation, opacity, blur] = l.nums;
			let source = match &l.pixels {
				Pixels::Media(img) => Source::Media(img),
				Pixels::Bitmap {
					image,
					natural_w,
					center_dx,
					center_dy,
				} => Source::Bitmap {
					image,
					natural_w: *natural_w,
					center_dx: *center_dx,
					center_dy: *center_dy,
				},
			};
			Layer {
				source,
				cx,
				cy,
				w,
				h,
				rotation,
				opacity,
				color_matrix: l.colour,
				filter: l.filter,
				blur,
			}
		})
		.collect();

	let mut pix = vec![0xAA_u8; c.w * c.h * 4];
	for i in 0..bands {
		let rows = c.h * i / bands..c.h * (i + 1) / bands;
		compose(
			&mut Target {
				pix: &mut pix,
				width: c.w,
				height: c.h,
				rows,
			},
			c.bg,
			&layers,
			c.scale,
		);
	}
	pix
}

fn first_difference(a: &[u8], b: &[u8], w: usize) -> String {
	match a.iter().zip(b).position(|(x, y)| x != y) {
		Some(i) => format!(
			"byte {i} (pixel {},{} channel {}): {} against {}",
			i / 4 % w,
			i / 4 / w,
			i % 4,
			a[i],
			b[i]
		),
		None => format!("lengths {} and {}", a.len(), b.len()),
	}
}

#[test]
fn every_vector_equals_the_go_compositor_at_one_band_and_at_eight() {
	let (_, cases) = read_cases();
	assert!(cases.len() >= 20);
	for c in &cases {
		for bands in [1, 8] {
			let got = render(c, bands);
			assert!(
				got == c.want,
				"{} at {bands} bands: {}",
				c.name,
				first_difference(&got, &c.want, c.w)
			);
		}
	}
}

#[test]
fn sincos_equals_the_go_series_bit_for_bit() {
	let (want, _) = read_cases();
	let mut h: u64 = 0xcbf2_9ce4_8422_2325;
	let mut put = |x: f64| {
		let (s, c) = sincos(x);
		for b in s
			.to_bits()
			.to_le_bytes()
			.into_iter()
			.chain(c.to_bits().to_le_bytes())
		{
			h = (h ^ u64::from(b)).wrapping_mul(0x0000_0100_0000_01b3);
		}
	};
	for i in 0..=10000_u32 {
		put(-1000.0 + 2000.0 * f64::from(i) / 10000.0);
	}
	for d in (-720..=720).step_by(15) {
		put(f64::from(d) * (core::f64::consts::PI / 180.0));
	}
	assert_eq!(h, want);
}

#[test]
fn colours_parse_as_in_go() {
	assert_eq!(parse_colour("#102030"), [0x10, 0x20, 0x30, 255]);
	assert_eq!(parse_colour("#AbCdEf80"), [0xab, 0xcd, 0xef, 255]);
	assert_eq!(parse_colour("102030"), [0, 0, 0, 255]);
	assert_eq!(parse_colour("#10203"), [0, 0, 0, 255]);
	assert_eq!(parse_colour("#10g030"), [0, 0, 0, 255]);
}
