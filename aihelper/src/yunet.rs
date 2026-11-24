//! YuNet face detection: the pre-processing of a frame and the decoding of the twelve output
//! tensors into boxes. It needs no runtime, so it is tested with synthetic tensors.

/// The strides of the three detection heads of YuNet.
pub const STRIDES: [usize; 3] = [8, 16, 32];

/// A detected face, in pixels of the frame the helper was given.
#[derive(Debug, Clone, Copy, PartialEq)]
pub struct Face {
	pub x: f32,
	pub y: f32,
	pub w: f32,
	pub h: f32,
	pub score: f32,
}

impl Face {
	fn iou(&self, other: &Face) -> f32 {
		let x1 = self.x.max(other.x);
		let y1 = self.y.max(other.y);
		let x2 = (self.x + self.w).min(other.x + other.w);
		let y2 = (self.y + self.h).min(other.y + other.h);
		let inter = (x2 - x1).max(0.0) * (y2 - y1).max(0.0);
		let union = self.w * self.h + other.w * other.h - inter;
		if union <= 0.0 { 0.0 } else { inter / union }
	}
}

/// Converts an RGB24 frame to the planar BGR float tensor YuNet reads (values 0 to 255).
pub fn to_nchw_bgr(rgb: &[u8], width: usize, height: usize) -> Vec<f32> {
	let plane = width * height;
	let mut out = vec![0.0_f32; plane * 3];
	for (i, px) in rgb.as_chunks::<3>().0.iter().take(plane).enumerate() {
		out[i] = f32::from(px[2]);
		out[plane + i] = f32::from(px[1]);
		out[2 * plane + i] = f32::from(px[0]);
	}
	out
}

/// The tensors of one head: classification, objectness and box regression per cell.
pub struct Head<'a> {
	pub stride: usize,
	pub cls: &'a [f32],
	pub obj: &'a [f32],
	pub bbox: &'a [f32],
}

/// Decodes the heads of a `width` by `height` input into faces scoring at least `min_score`,
/// then removes overlaps (non-maximum suppression at `nms_iou`), best first, at most `max_faces`.
pub fn decode(
	heads: &[Head<'_>],
	width: usize,
	height: usize,
	min_score: f32,
	nms_iou: f32,
	max_faces: usize,
) -> Vec<Face> {
	let mut found: Vec<Face> = Vec::new();
	for head in heads {
		let cols = width / head.stride;
		let rows = height / head.stride;
		let cells = cols * rows;
		if head.cls.len() < cells || head.obj.len() < cells || head.bbox.len() < cells * 4 {
			continue;
		}
		let s = head.stride as f32;
		for idx in 0..cells {
			let cls = head.cls[idx].clamp(0.0, 1.0);
			let obj = head.obj[idx].clamp(0.0, 1.0);
			let score = (cls * obj).sqrt();
			if score < min_score {
				continue;
			}
			let (r, c) = (idx / cols, idx % cols);
			let b = &head.bbox[idx * 4..idx * 4 + 4];
			let cx = (c as f32 + b[0]) * s;
			let cy = (r as f32 + b[1]) * s;
			let w = b[2].exp() * s;
			let h = b[3].exp() * s;
			// A box the host cannot read (NaN or infinite) would end the whole run; it is not a face.
			if ![score, cx, cy, w, h].iter().all(|v| v.is_finite()) {
				continue;
			}
			found.push(Face {
				x: cx - w / 2.0,
				y: cy - h / 2.0,
				w,
				h,
				score,
			});
		}
	}
	found.sort_by(|a, b| b.score.total_cmp(&a.score));
	let mut kept: Vec<Face> = Vec::new();
	for f in found {
		if kept.len() >= max_faces {
			break;
		}
		if kept.iter().all(|k| k.iou(&f) <= nms_iou) {
			kept.push(f);
		}
	}
	kept
}

#[cfg(test)]
mod tests {
	use super::*;

	#[test]
	fn bgr_planes_swap_red_and_blue() {
		let t = to_nchw_bgr(&[10, 20, 30, 40, 50, 60], 2, 1);
		assert_eq!(t, vec![30.0, 60.0, 20.0, 50.0, 10.0, 40.0]);
	}

	fn head(stride: usize, width: usize, height: usize) -> (Vec<f32>, Vec<f32>, Vec<f32>) {
		let cells = (width / stride) * (height / stride);
		(vec![0.0; cells], vec![0.0; cells], vec![0.0; cells * 4])
	}

	#[test]
	fn decodes_one_cell_into_a_box() {
		let (mut cls, mut obj, mut bbox) = head(8, 64, 32);
		// cell column 3, row 1 of a 8-column grid
		let idx = 8 + 3;
		cls[idx] = 0.81;
		obj[idx] = 1.0;
		bbox[idx * 4..idx * 4 + 4].copy_from_slice(&[
			0.5,
			0.5,
			1.0_f32.ln() + 1.0,
			1.0_f32.ln() + 1.0,
		]);
		let heads = [Head {
			stride: 8,
			cls: &cls,
			obj: &obj,
			bbox: &bbox,
		}];
		let faces = decode(&heads, 64, 32, 0.5, 0.3, 10);
		assert_eq!(faces.len(), 1);
		let f = faces[0];
		assert!((f.score - 0.9).abs() < 1e-5);
		assert!(
			(f.x + f.w / 2.0 - 28.0).abs() < 1e-3,
			"centre x {}",
			f.x + f.w / 2.0
		);
		assert!((f.y + f.h / 2.0 - 12.0).abs() < 1e-3);
		assert!((f.w - 8.0 * std::f32::consts::E).abs() < 1e-3);
	}

	#[test]
	fn overlapping_boxes_collapse_to_the_best() {
		let (mut cls, mut obj, mut bbox) = head(8, 64, 32);
		for idx in [2usize, 3] {
			cls[idx] = 0.9;
			obj[idx] = 0.9 + idx as f32 / 100.0;
			bbox[idx * 4..idx * 4 + 4].copy_from_slice(&[0.5, 0.5, 1.0, 1.0]);
		}
		let heads = [Head {
			stride: 8,
			cls: &cls,
			obj: &obj,
			bbox: &bbox,
		}];
		let faces = decode(&heads, 64, 32, 0.5, 0.3, 10);
		// Two cells one stride apart give boxes that overlap by about 0.46, over the 0.3 limit.
		assert_eq!(faces.len(), 1, "{faces:?}");
		assert!(
			(faces[0].score - (0.9_f32 * 0.93).sqrt()).abs() < 1e-5,
			"the better score is kept"
		);
	}

	#[test]
	fn low_scores_and_short_tensors_yield_nothing() {
		let (cls, obj, bbox) = head(8, 64, 32);
		let heads = [
			Head {
				stride: 8,
				cls: &cls,
				obj: &obj,
				bbox: &bbox,
			},
			Head {
				stride: 16,
				cls: &[],
				obj: &[],
				bbox: &[],
			},
		];
		assert!(decode(&heads, 64, 32, 0.5, 0.3, 10).is_empty());
	}

	#[test]
	fn boxes_that_are_not_finite_are_dropped() {
		let (mut cls, mut obj, mut bbox) = head(8, 64, 32);
		for idx in [0usize, 4, 9] {
			cls[idx] = 1.0;
			obj[idx] = 1.0;
		}
		bbox[0..4].copy_from_slice(&[0.5, 0.5, 200.0, 0.0]);
		bbox[16..20].copy_from_slice(&[f32::NAN, 0.5, 0.0, 0.0]);
		bbox[36..40].copy_from_slice(&[0.5, 0.5, 0.0, 0.0]);
		cls[12] = f32::NAN;
		obj[12] = 1.0;
		let heads = [Head {
			stride: 8,
			cls: &cls,
			obj: &obj,
			bbox: &bbox,
		}];
		let faces = decode(&heads, 64, 32, 0.5, 0.3, 10);
		assert_eq!(faces.len(), 1, "{faces:?}");
		assert!(
			faces
				.iter()
				.all(|f| [f.x, f.y, f.w, f.h, f.score].iter().all(|v| v.is_finite()))
		);
	}

	#[test]
	fn max_faces_caps_the_answer() {
		let (mut cls, mut obj, mut bbox) = head(8, 256, 32);
		for idx in [0usize, 10, 20, 30] {
			cls[idx] = 1.0;
			obj[idx] = 1.0;
			bbox[idx * 4..idx * 4 + 4].copy_from_slice(&[0.5, 0.5, 0.0, 0.0]);
		}
		let heads = [Head {
			stride: 8,
			cls: &cls,
			obj: &obj,
			bbox: &bbox,
		}];
		assert_eq!(decode(&heads, 256, 32, 0.5, 0.3, 2).len(), 2);
	}
}
