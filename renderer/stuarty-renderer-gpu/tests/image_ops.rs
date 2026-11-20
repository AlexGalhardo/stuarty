//! The GPU path against the CPU path on every image-operation case of SDD-013 section 3.11 (epic
//! 29): the 23 render inputs of `renderer/testdata/image-ops`, each drawn by both paths; the luma SSIM
//! must be at least 0.99 (SDD-013 section 3.8). The adapter is printed with the worst result; run with
//! `--nocapture` to read the table.

#[path = "../../stuarty-renderer/tests/support/image_ops.rs"]
mod cases;
#[path = "support/ssim.rs"]
mod ssim;
mod support;

use stuarty_renderer::compose::Target;
use stuarty_renderer::plan::decode;

use ssim::{luma, ssim};

const MIN_SSIM: f64 = 0.99;

#[test]
fn every_image_operation_is_within_the_gpu_tolerance_of_the_cpu_path() {
	let Some(gpu) = support::open() else { return };
	let info = gpu.adapter();
	let adapter = format!(
		"{} / {:?} / {:?}",
		info.name, info.device_type, info.backend
	);
	let all = cases::cases();
	assert!(all.len() >= 20, "the cases are missing");

	let mut worst = (1.0_f64, String::new());
	let mut failures = Vec::new();
	for case in &all {
		let name = case.file_stem().unwrap().to_string_lossy().to_string();
		let (r, input, w, h) = cases::load(case);

		let mut want = vec![0xAA_u8; w * h * 4];
		r.render(&input, &mut want).unwrap();

		let decoded = decode(&input).unwrap();
		let layers = r.layers(&decoded).unwrap();
		let mut got = vec![0xAA_u8; w * h * 4];
		gpu.compose(
			&mut Target {
				pix: &mut got,
				width: w,
				height: h,
				rows: 0..h,
			},
			stuarty_renderer::compose::frame_fill(&decoded.background, decoded.transparent),
			&layers,
			decoded.output.scale,
		)
		.unwrap_or_else(|e| panic!("{name}: {e}"));

		let score = ssim(&luma(&want), &luma(&got), w, h);
		eprintln!("{name:<40} SSIM {score:.5}  [{adapter}]");
		if score < worst.0 {
			worst = (score, name.clone());
		}
		if score < MIN_SSIM {
			failures.push(format!("{name}: SSIM {score:.5}"));
		}
	}
	eprintln!("worst: {} SSIM {:.5} on {adapter}", worst.1, worst.0);
	assert!(failures.is_empty(), "{failures:#?}");
}
