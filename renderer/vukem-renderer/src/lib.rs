//! The pixel renderer of Vukem Video (ADR-0029, rule `.agents/rules/rust.md`).
//!
//! The frame plan is decided in Go (`core/plan`); this crate only draws it. This is the
//! skeleton of the workspace: the composition arrives with the next stages.

use wasm_bindgen::prelude::wasm_bindgen;

/// The version of the boundary between the hosts and the renderer. A host refuses a module
/// whose number it does not know.
pub const API_VERSION: u32 = 1;

/// Returns [`API_VERSION`] to a host.
#[wasm_bindgen]
pub fn api_version() -> u32 {
	API_VERSION
}

#[cfg(test)]
mod tests {
	use super::*;

	#[test]
	fn api_version_is_the_constant() {
		assert_eq!(api_version(), API_VERSION);
	}
}
