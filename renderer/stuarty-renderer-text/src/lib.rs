//! The text and shape rasterizer of the Stuarty renderer (ADR-0029 stage E, SDD-013
//! section 3.11): the port of `engine/raster`, byte-equal with it.
//!
//! Every value that reaches a pixel is computed with the same operations, in the same order, as the
//! Go code: the geometry is `f64` with exactly rounded operations only, the coverage accumulation is
//! the `f32` scanline of `scan`, and the blend is integer arithmetic (rule 5 of
//! `.agents/rules/rust.md`).

pub mod canvas;
pub mod fonts;
pub mod geom;
pub mod layout;
pub mod math;
pub mod scan;
pub mod shape;
pub mod text;
pub mod validate;

/// A straight (non-premultiplied) RGBA colour.
#[derive(Clone, Copy, Debug, Default, PartialEq, Eq)]
pub struct Colour {
	pub r: u8,
	pub g: u8,
	pub b: u8,
	pub a: u8,
}

/// A rectangle by its position and size, in pixels.
#[derive(Clone, Copy, Debug, Default, PartialEq)]
pub struct Rect {
	pub x: f64,
	pub y: f64,
	pub w: f64,
	pub h: f64,
}

/// A rendered bitmap and its metrics (`raster.Bitmap`).
#[derive(Clone, Debug, PartialEq)]
pub struct Bitmap {
	/// RGBA, 8 bits per channel, straight alpha, `width * height * 4` bytes.
	pub pix: Vec<u8>,
	pub width: usize,
	pub height: usize,
	pub natural_w: f64,
	pub natural_h: f64,
	pub center_dx: f64,
	pub center_dy: f64,
	pub word_boxes: Vec<Rect>,
}

/// Why a spec cannot be drawn (`raster.ErrInvalidSpec`).
#[derive(Clone, Debug, PartialEq, Eq)]
pub struct InvalidSpec(pub String);

impl std::fmt::Display for InvalidSpec {
	fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
		write!(f, "raster: invalid spec: {}", self.0)
	}
}

impl std::error::Error for InvalidSpec {}

/// The largest bitmap, in pixels (`raster.MaxPixels`).
pub const MAX_PIXELS: usize = 4096 * 8192;
