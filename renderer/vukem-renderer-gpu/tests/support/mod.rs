//! Helpers of the GPU tests: the adapter and the luma SSIM of SDD-013 section 3.8.

use vukem_renderer_gpu::{Adapter, GpuError, GpuRenderer};

/// A renderer on the adapter the machine offers, the software one when there is no other; `None`
/// (with a message, so the skip is visible in the log) when the machine has no adapter at all.
/// `VUKEM_VIDEO_TEST_GPU_ADAPTER=software` asks for the software adapter only.
pub fn open() -> Option<GpuRenderer> {
	let kinds: &[Adapter] =
		if std::env::var("VUKEM_VIDEO_TEST_GPU_ADAPTER").as_deref() == Ok("software") {
			&[Adapter::Software]
		} else {
			&[Adapter::Any, Adapter::Software]
		};
	for &kind in kinds {
		match GpuRenderer::new(kind) {
			Ok(g) => {
				let i = g.adapter();
				eprintln!(
					"gpu adapter: {} (vendor {:#06x}, {:?}, backend {:?})",
					i.name, i.vendor, i.device_type, i.backend
				);
				return Some(g);
			}
			Err(GpuError::NoAdapter) => {}
			Err(e) => panic!("the adapter exists but the device failed: {e}"),
		}
	}
	eprintln!("SKIP: no graphics adapter on this machine");
	None
}
