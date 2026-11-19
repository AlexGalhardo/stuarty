//! The pixel buffer of the CPU path.

/// RGBA, 8 bits per channel, premultiplied alpha, `w * h * 4` bytes, no padding.
#[derive(Clone, Debug, Default, PartialEq, Eq)]
pub struct Image {
	pub pix: Vec<u8>,
	pub w: usize,
	pub h: usize,
	/// Every alpha is 255, which lets a copy skip blending (a decoded video frame).
	pub opaque: bool,
}

impl Image {
	/// An image with a zeroed (transparent) buffer.
	pub fn new(w: usize, h: usize) -> Self {
		Self {
			pix: vec![0; w.saturating_mul(h).saturating_mul(4)],
			w,
			h,
			opaque: false,
		}
	}

	/// Whether the buffer holds at least `w * h` pixels and the size is not empty.
	pub fn is_drawable(&self) -> bool {
		self.w > 0
			&& self.h > 0
			&& self.pix.len() >= self.w.saturating_mul(self.h).saturating_mul(4)
	}
}
