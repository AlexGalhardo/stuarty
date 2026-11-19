//! The CPU path against the Go compositor, byte for byte (SDD-013 section 3.10).
//!
//! `renderer/testdata/compose-vectors.bin` is written by `engine/compositor/vectors_test.go`: small
//! compositions with their inputs and the frame the Go compositor draws for each.

use stuarty_renderer::compose::{Target, compose, parse_colour};
use stuarty_renderer::math::sincos;

#[path = "support/vectors.rs"]
mod vectors;

use vectors::{Case, layers_of, read_cases};

fn render(c: &Case, bands: usize) -> Vec<u8> {
	let layers = layers_of(c);
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
