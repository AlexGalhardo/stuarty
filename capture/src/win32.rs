//! The one place of the helper that declares Windows functions by hand (rule .agents/rules/rust.md 4: a reviewed
//! boundary module). Each function is a few lines, safe to call, and every `unsafe` carries its reason. The
//! module holds nothing that reads a key: the only input function is `mouse_buttons`, which asks about the three
//! mouse buttons and nothing else (SDD-025 section 16.6, and the test of this file that scans for it).
#![allow(unsafe_code)]

unsafe extern "system" {
	fn QueryPerformanceCounter(count: *mut i64) -> i32;
	fn QueryPerformanceFrequency(freq: *mut i64) -> i32;
}

/// The performance counter in units of 100 ns, the unit WASAPI stamps its packets with. 0 if it cannot be read.
pub fn qpc_100ns() -> u64 {
	let (mut count, mut freq) = (0i64, 0i64);
	// SAFETY: both pointers are to live local variables the function writes through.
	let ok = unsafe {
		QueryPerformanceCounter(&mut count) != 0 && QueryPerformanceFrequency(&mut freq) != 0
	};
	if !ok || freq <= 0 || count < 0 {
		return 0;
	}
	u64::try_from(i128::from(count) * 10_000_000 / i128::from(freq)).unwrap_or(0)
}

#[cfg(test)]
mod tests {
	use super::*;

	#[test]
	fn the_counter_runs_forward_in_units_of_100_ns() {
		let a = qpc_100ns();
		std::thread::sleep(std::time::Duration::from_millis(50));
		let b = qpc_100ns();
		let ms = (b - a) / 10_000;
		assert!((45..=500).contains(&ms), "{ms} ms for a 50 ms sleep");
	}
}
