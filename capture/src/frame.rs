//! Frames and the mailbox between a source and the writer.

use std::sync::{Arc, Condvar, Mutex};
use std::time::Duration;

/// One BGRA image, tightly packed.
#[derive(Debug)]
pub struct Frame {
	pub width: u32,
	pub height: u32,
	pub data: Vec<u8>,
}

impl Frame {
	/// The frame as a canvas of w by h: the overlap is copied at the top left, the rest is black. A frame of the
	/// size of the canvas is returned as it is.
	pub fn fit(self: &Arc<Self>, w: u32, h: u32) -> Arc<Frame> {
		if self.width == w && self.height == h {
			return Arc::clone(self);
		}
		let mut data = vec![0u8; w as usize * h as usize * 4];
		let copy_w = self.width.min(w) as usize * 4;
		for y in 0..self.height.min(h) as usize {
			let src = y * self.width as usize * 4;
			let dst = y * w as usize * 4;
			data[dst..dst + copy_w].copy_from_slice(&self.data[src..src + copy_w]);
		}
		Arc::new(Frame {
			width: w,
			height: h,
			data,
		})
	}
}

#[derive(Default)]
struct State {
	frame: Option<Arc<Frame>>,
	fresh: bool,
	closed: bool,
	error: Option<String>,
	arrived: u64,
}

/// Holds the latest frame a source produced. The writer takes it once per slot of its grid.
#[derive(Default)]
pub struct Mailbox {
	state: Mutex<State>,
	cond: Condvar,
}

/// What the writer finds in the mailbox at a slot.
pub struct Slot {
	pub frame: Option<Arc<Frame>>,
	/// True when the frame arrived after the last slot.
	pub fresh: bool,
	pub closed: bool,
	pub error: Option<String>,
}

impl Mailbox {
	pub fn new() -> Arc<Self> {
		Arc::new(Self::default())
	}

	fn lock(&self) -> std::sync::MutexGuard<'_, State> {
		self.state
			.lock()
			.unwrap_or_else(std::sync::PoisonError::into_inner)
	}

	pub fn push(&self, f: Frame) {
		let mut s = self.lock();
		s.frame = Some(Arc::new(f));
		s.fresh = true;
		s.arrived += 1;
		self.cond.notify_all();
	}

	/// The source ended: because the window or display went away, or because it failed.
	pub fn close(&self, error: Option<String>) {
		let mut s = self.lock();
		s.closed = true;
		if error.is_some() {
			s.error = error;
		}
		self.cond.notify_all();
	}

	/// Waits for the first frame, a close, or the timeout.
	pub fn wait_first(&self, timeout: Duration) -> Slot {
		let s = self.lock();
		let (s, _) = self
			.cond
			.wait_timeout_while(s, timeout, |s| s.frame.is_none() && !s.closed)
			.unwrap_or_else(std::sync::PoisonError::into_inner);
		Slot {
			frame: s.frame.clone(),
			fresh: s.fresh,
			closed: s.closed,
			error: s.error.clone(),
		}
	}

	/// Takes what is there for this slot and marks it as seen.
	pub fn take(&self) -> Slot {
		let mut s = self.lock();
		let slot = Slot {
			frame: s.frame.clone(),
			fresh: s.fresh,
			closed: s.closed,
			error: s.error.clone(),
		};
		s.fresh = false;
		slot
	}

	#[cfg(test)]
	pub fn arrived(&self) -> u64 {
		self.lock().arrived
	}
}

#[cfg(test)]
mod tests {
	use super::*;

	fn solid(w: u32, h: u32, v: u8) -> Frame {
		Frame {
			width: w,
			height: h,
			data: vec![v; w as usize * h as usize * 4],
		}
	}

	#[test]
	fn fit_of_the_same_size_shares_the_frame() {
		let f = Arc::new(solid(4, 2, 9));
		assert!(Arc::ptr_eq(&f, &f.fit(4, 2)));
	}

	#[test]
	fn fit_pads_with_black_and_cuts() {
		let f = Arc::new(solid(2, 2, 9));
		let big = f.fit(4, 3);
		assert_eq!((big.width, big.height, big.data.len()), (4, 3, 48));
		assert_eq!(&big.data[0..8], &[9; 8]);
		assert_eq!(&big.data[8..16], &[0; 8]);
		assert!(big.data[32..].iter().all(|&b| b == 0));
		let small = Arc::new(solid(4, 4, 7)).fit(2, 2);
		assert_eq!(
			(small.data.len(), small.data[0], small.data[15]),
			(16, 7, 7)
		);
	}

	#[test]
	fn a_slot_sees_a_fresh_frame_once() {
		let m = Mailbox::new();
		assert!(m.take().frame.is_none());
		m.push(solid(1, 1, 1));
		let a = m.take();
		assert!(a.frame.is_some() && a.fresh);
		let b = m.take();
		assert!(b.frame.is_some() && !b.fresh);
		assert_eq!(m.arrived(), 1);
	}

	#[test]
	fn close_wakes_the_first_wait() {
		let m = Mailbox::new();
		let m2 = Arc::clone(&m);
		let t = std::thread::spawn(move || m2.close(Some("gone".into())));
		let s = m.wait_first(Duration::from_secs(5));
		t.join().unwrap();
		assert!(s.closed && s.frame.is_none() && s.error.as_deref() == Some("gone"));
	}
}
