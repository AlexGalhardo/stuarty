//! The `wgpu` path of the Vukem Video renderer (ADR-0029 stage D, SDD-013 section 3.8).
//!
//! It draws the same render input as the CPU path of `vukem-renderer` and shares with it the plan
//! decoder, the stored textures and bitmaps, the preparation of every layer (chroma key, colour
//! matrix, filter, blur: `prepare_layer`), the opacity and the choice between the one-to-one copy
//! and the sampled draw. What the GPU decides is the sampling and the blend: each layer is one
//! quad, transformed on the CPU in `f64` and handed over as final positions, drawn bottom first with
//! premultiplied source-over, or by a shader that mixes with a copy of the frame for a blend mode. The result is not bit-exact; it is held to the CPU path by SSIM and
//! geometry (SDD-013 section 3.8).
//!
//! This crate is native (Vulkan, Metal, Direct3D 12). It is separate from the core module of the CPU
//! path so that module keeps no import (SDD-013 decision DS-1).

mod block;
mod gpu;
pub mod proto;
mod quad;

pub use gpu::{Adapter, GpuError, GpuRenderer};
pub use quad::{Quad, quad_for};
