//! The pixel renderer of Stuarty (ADR-0029, rule `.agents/rules/rust.md`).
//!
//! The frame plan is decided in Go (`core/plan`); this crate only draws it. The CPU
//! path (`compose`) is the port of `engine/compositor`, bit-exact with it (SDD-013 section 3).
//!
//! The WebAssembly build is the core module of SDD-013 section 4.1 (decision DS-1): plain C-ABI
//! exports, no `wasm-bindgen`, no import. The exports are in the `ffi` module, the only place of
//! the crate that uses `unsafe`.

pub mod blend;
pub mod blur;
pub mod chroma;
pub mod colour;
pub mod compose;
pub mod effects;
pub mod error;
pub mod filters;
pub mod grade;
pub mod host;
pub mod image;
pub mod json;
pub mod lut;
pub mod mask;
pub mod math;
pub mod plan;
pub mod rasterhost;
pub mod resize;
pub mod tone;

#[cfg(target_arch = "wasm32")]
mod ffi;

/// The version of the boundary between the hosts and the renderer (`vr_api_version`). A host
/// refuses a module whose number it does not know.
pub const API_VERSION: u32 = 3;
