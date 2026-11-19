//! The golden frames of the image operations of SDD-013 section 3.11 (epic 29).
//!
//! `renderer/testdata/image-ops/<case>.json` is a render input; each of its layers is a media layer
//! whose item id names the texture it draws (`pattern`, the part before a `#`); `<case>.rgba` is the
//! frame the CPU path draws, as raw RGBA8. `engine/renderhost/image_ops_test.go` reads the same files
//! and draws them through the WebAssembly module under `wazero`: the two must give the same bytes.
//! `STUARTY_WRITE_IMAGE_OPS=1 cargo test --test image_ops` rewrites the frames, which is a reviewed
//! change of output.

use std::fs;

#[path = "support/image_ops.rs"]
mod support;

fn render(case: &std::path::Path) -> Vec<u8> {
	let (r, input, w, h) = support::load(case);
	let mut out = vec![0_u8; w * h * 4];
	r.render(&input, &mut out)
		.unwrap_or_else(|e| panic!("{}: {e:?}", case.display()));
	// Two renders and six bands give the same bytes.
	let mut again = vec![0_u8; w * h * 4];
	r.render(&input, &mut again).unwrap();
	assert_eq!(out, again, "{}: not repeatable", case.display());
	let text = String::from_utf8(input).unwrap();
	let mut banded = vec![0_u8; w * h * 4];
	let rows = h / 6;
	for b in 0..6 {
		let t = text.replacen(
			"\"scale\":1}",
			&format!(
				"\"scale\":1,\"y0\":{},\"y1\":{}}}",
				b * rows,
				(b + 1) * rows
			),
			1,
		);
		r.render(t.as_bytes(), &mut banded).unwrap();
	}
	assert_eq!(
		out,
		banded,
		"{}: six bands differ from the whole",
		case.display()
	);
	out
}

#[test]
fn every_case_equals_its_golden_frame() {
	let write = std::env::var("STUARTY_WRITE_IMAGE_OPS").is_ok();
	let cases = support::cases();
	assert!(cases.len() >= 20, "the cases are missing: {}", cases.len());
	for case in cases {
		let got = render(&case);
		let golden = case.with_extension("rgba");
		if write {
			fs::write(&golden, &got).unwrap();
			continue;
		}
		let want = fs::read(&golden).unwrap_or_else(|_| panic!("{} has no frame", case.display()));
		assert!(
			got == want,
			"{}: {} bytes differ",
			case.display(),
			got.iter().zip(&want).filter(|(a, b)| a != b).count()
		);
	}
}
