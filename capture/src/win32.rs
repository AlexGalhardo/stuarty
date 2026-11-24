//! The one place of the helper that declares Windows functions by hand (rule .agents/rules/rust.md 4: a reviewed
//! boundary module). Each function is a few lines, safe to call, and every `unsafe` carries its reason. The
//! module holds nothing that reads a key: the only input function is `mouse_buttons`, which asks about the three
//! mouse buttons and nothing else (SDD-025 section 16.6, and the test of this file that scans for it).
#![allow(unsafe_code)]

use std::ffi::c_void;

unsafe extern "system" {
	fn QueryPerformanceCounter(count: *mut i64) -> i32;
	fn QueryPerformanceFrequency(freq: *mut i64) -> i32;
}

#[link(name = "winmm")]
unsafe extern "system" {
	fn timeBeginPeriod(period_ms: u32) -> u32;
}

#[link(name = "user32")]
unsafe extern "system" {
	fn GetCursorPos(point: *mut Point) -> i32;
	fn GetAsyncKeyState(virtual_key: i32) -> i16;
	fn GetMonitorInfoW(monitor: *mut c_void, info: *mut MonitorInfo) -> i32;
	fn GetWindowRect(window: *mut c_void, rect: *mut Rect) -> i32;
	fn IsWindow(window: *mut c_void) -> i32;
	fn SetProcessDpiAwarenessContext(context: isize) -> i32;
}

#[link(name = "dwmapi")]
unsafe extern "system" {
	fn DwmGetWindowAttribute(
		window: *mut c_void,
		attribute: u32,
		value: *mut c_void,
		size: u32,
	) -> i32;
}

#[repr(C)]
struct Point {
	x: i32,
	y: i32,
}

/// A rectangle in pixels of the desktop.
#[repr(C)]
#[derive(Debug, Clone, Copy, Default, PartialEq, Eq)]
pub struct Rect {
	pub left: i32,
	pub top: i32,
	pub right: i32,
	pub bottom: i32,
}

#[repr(C)]
struct MonitorInfo {
	size: u32,
	monitor: Rect,
	work: Rect,
	flags: u32,
}

/// The virtual keys of the three mouse buttons: left, right, middle. They are the only keys this helper asks
/// about (SDD-025 section 16.6), and a test pins the list.
pub const MOUSE_BUTTONS: [i32; 3] = [0x01, 0x02, 0x04];

/// The position of the pointer on the desktop, in pixels of the desktop.
pub fn cursor_pos() -> Option<(i32, i32)> {
	let mut p = Point { x: 0, y: 0 };
	// SAFETY: the pointer is to a live local the function writes through.
	let ok = unsafe { GetCursorPos(&mut p) } != 0;
	ok.then_some((p.x, p.y))
}

/// Whether the left, right and middle buttons are down now. It asks about those three and no other key.
pub fn mouse_buttons() -> [bool; 3] {
	MOUSE_BUTTONS.map(|vk| {
		// SAFETY: a plain call with a number; the high bit of the answer is "down now".
		let state = unsafe { GetAsyncKeyState(vk) };
		(state as u16) & 0x8000 != 0
	})
}

/// The rectangle of a monitor, from its handle.
pub fn monitor_rect(monitor: *mut c_void) -> Option<Rect> {
	let mut info = MonitorInfo {
		size: size_of::<MonitorInfo>() as u32,
		monitor: Rect::default(),
		work: Rect::default(),
		flags: 0,
	};
	// SAFETY: the pointer is to a live local with its size field set, as the function requires; a handle that is
	// not a monitor makes it return 0.
	let ok = unsafe { GetMonitorInfoW(monitor, &mut info) } != 0;
	ok.then_some(info.monitor)
}

/// The rectangle of a window as the capture draws it: the visible frame, without the invisible border that
/// Windows 10 and 11 add around a window. None when the handle is not a window any more.
pub fn window_frame(window: *mut c_void) -> Option<Rect> {
	// SAFETY: IsWindow takes any handle and answers whether it is a window.
	if unsafe { IsWindow(window) } == 0 {
		return None;
	}
	const DWMWA_EXTENDED_FRAME_BOUNDS: u32 = 9;
	let mut r = Rect::default();
	// SAFETY: the pointer is to a live Rect and the size is its size, as the function requires.
	let hr = unsafe {
		DwmGetWindowAttribute(
			window,
			DWMWA_EXTENDED_FRAME_BOUNDS,
			(&mut r as *mut Rect).cast(),
			size_of::<Rect>() as u32,
		)
	};
	if hr >= 0 {
		return Some(r);
	}
	// SAFETY: as above, with a live Rect.
	let ok = unsafe { GetWindowRect(window, &mut r) } != 0;
	ok.then_some(r)
}

/// Makes the process aware of each monitor's scaling, so the position of the pointer and the size of a window
/// are in the physical pixels the capture delivers. It must run before the first window is made.
pub fn per_monitor_dpi() {
	// DPI_AWARENESS_CONTEXT_PER_MONITOR_AWARE_V2 is the handle -4.
	// SAFETY: a plain call with a number; a failure (it was already set) leaves what is there.
	let _ = unsafe { SetProcessDpiAwarenessContext(-4) };
}

/// Asks Windows for a 1 ms timer, so a sleep of a few milliseconds is not stretched to the default 15.6 ms and
/// the frame grid and the pointer sampler keep their time. It lasts for the life of the process.
pub fn timer_1ms() {
	// SAFETY: a plain call with a number; a failure only leaves the default timer.
	let _ = unsafe { timeBeginPeriod(1) };
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
