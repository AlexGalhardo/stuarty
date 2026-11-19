//! The GPU path draws what the CPU path draws for the plans the host protocol carries
//! (SDD-013 sections 2 and 3.8): the same decoder, the same stored textures, the same errors.

mod support;

use stuarty_renderer::host::{Limits, Renderer};
use stuarty_renderer_gpu::GpuError;

const INPUT: &str = r##"{"version":1,"output":{"width":8,"height":8,"scale":1},"plan":{"background":"#102030","layers":[
	{"item":"a","kind":"media","cx":4,"cy":4,"w":4,"h":4,"opacity":1}]}}"##;

fn host() -> Renderer {
	let mut r = Renderer::new(Limits::default());
	let pix: Vec<u8> = (0..16)
		.flat_map(|i| [i * 15, 255 - i * 15, 40 + i * 5, 255])
		.collect();
	r.put_texture("a", 4, 4, &pix, true).unwrap();
	r
}

fn cpu(host: &Renderer, input: &str) -> Vec<u8> {
	let mut out = vec![0xAA_u8; 8 * 8 * 4];
	host.render(input.as_bytes(), &mut out).unwrap();
	out
}

#[test]
fn a_one_to_one_copy_is_the_same_bytes_as_the_cpu_path() {
	let Some(gpu) = support::open() else { return };
	let host = host();
	let mut out = vec![0xAA_u8; 8 * 8 * 4];
	gpu.render(&host, INPUT.as_bytes(), &mut out).unwrap();
	assert_eq!(out, cpu(&host, INPUT));
}

#[test]
fn a_band_writes_only_its_rows() {
	let Some(gpu) = support::open() else { return };
	let host = host();
	let band = INPUT.replace("\"scale\":1", "\"scale\":1,\"y0\":3,\"y1\":5");
	let mut out = vec![7_u8; 8 * 8 * 4];
	gpu.render(&host, band.as_bytes(), &mut out).unwrap();
	assert!(out[..3 * 32].iter().all(|&b| b == 7));
	assert!(out[5 * 32..].iter().all(|&b| b == 7));
	let mut want = vec![7_u8; 8 * 8 * 4];
	host.render(band.as_bytes(), &mut want).unwrap();
	assert_eq!(out, want);
}

#[test]
fn a_stacked_opacity_layer_blends_within_one_level() {
	let Some(gpu) = support::open() else { return };
	let host = host();
	let input = INPUT.replace(
		"\"opacity\":1}",
		"\"opacity\":1},{\"item\":\"a\",\"kind\":\"media\",\"cx\":5,\"cy\":5,\"w\":4,\"h\":4,\"opacity\":0.5}",
	);
	let want = cpu(&host, &input);
	let mut out = vec![0xAA_u8; 8 * 8 * 4];
	gpu.render(&host, input.as_bytes(), &mut out).unwrap();
	let worst = out
		.iter()
		.zip(&want)
		.map(|(a, b)| a.abs_diff(*b))
		.max()
		.unwrap();
	assert!(worst <= 1, "worst channel difference {worst}");
}

#[test]
fn the_errors_are_the_ones_of_the_cpu_path() {
	let Some(gpu) = support::open() else { return };
	let mut out = vec![0_u8; 8 * 8 * 4];

	let empty = Renderer::new(Limits::default());
	let cpu_err = empty.render(INPUT.as_bytes(), &mut out).unwrap_err();
	let Err(GpuError::Render(e)) = gpu.render(&empty, INPUT.as_bytes(), &mut out) else {
		panic!("a missing texture is a render error");
	};
	assert_eq!(e, cpu_err);

	let Err(GpuError::Render(e)) = gpu.render(&host(), b"{", &mut out) else {
		panic!("a broken plan is a render error");
	};
	assert_eq!(e.reason.as_str(), "invalid_plan");

	let Err(GpuError::Render(e)) = gpu.render(&host(), INPUT.as_bytes(), &mut out[..63]) else {
		panic!("a short output is a render error");
	};
	assert_eq!(e.reason.as_str(), "invalid_argument");
}
