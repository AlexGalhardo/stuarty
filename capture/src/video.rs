//! The grid of a video track (SDD-025 sections 8 and 16.5): one frame per slot of an absolute grid of the shared
//! clock, the last frame repeated when the source is slow. The screen and the camera each have one.

use std::sync::Arc;
use std::time::Duration;

use crate::clock::Clock;
use crate::frame::Mailbox;
use crate::sink::Sink;

/// How long one wait for a slot may be: short, so a slot is taken within a few milliseconds of its time.
const MAX_WAIT: Duration = Duration::from_millis(2);

/// What a call of `Grid::step` found.
pub enum Step {
	/// Nothing is due; sleep this long.
	Wait(Duration),
	/// A slot was written.
	Wrote,
	/// The source ended: because it went away (no error) or because it failed.
	Closed(Option<String>),
	/// The sink could not take the frame.
	Failed(std::io::Error),
}

pub struct Grid {
	mailbox: Arc<Mailbox>,
	pub sink: Box<dyn Sink>,
	width: u32,
	height: u32,
	fps: u64,
	next_slot: u64,
	pub frames: u64,
	pub dropped: u64,
}

impl Grid {
	/// A grid whose slot 0 was already written (the first frame warms the encoder before the origin).
	pub fn new(
		mailbox: Arc<Mailbox>,
		sink: Box<dyn Sink>,
		width: u32,
		height: u32,
		fps: u32,
	) -> Self {
		Self {
			mailbox,
			sink,
			width,
			height,
			fps: u64::from(fps),
			next_slot: 1,
			frames: 1,
			dropped: 0,
		}
	}

	pub fn step(&mut self, clock: &Clock) -> Step {
		if clock.paused() {
			return Step::Wait(MAX_WAIT);
		}
		let due = i64::try_from(self.next_slot * 1_000_000_000 / self.fps).unwrap_or(i64::MAX);
		let left = due - clock.active_ns();
		if left > 0 {
			return Step::Wait(Duration::from_nanos(left.unsigned_abs()).min(MAX_WAIT));
		}
		self.write_slot()
	}

	/// Writes the slots that were due and not written, for a grid that was held up by a busy machine, so the file
	/// ends at the length of the clock (frozen by a pause) and not where the thread happened to be. Called once,
	/// at the end.
	pub fn catch_up(&mut self, clock: &Clock) -> std::io::Result<()> {
		let now = clock.active_ns();
		loop {
			let due = i64::try_from(self.next_slot * 1_000_000_000 / self.fps).unwrap_or(i64::MAX);
			if due > now {
				return Ok(());
			}
			match self.write_slot() {
				Step::Failed(e) => return Err(e),
				Step::Closed(_) => return Ok(()),
				Step::Wrote | Step::Wait(_) => {}
			}
		}
	}

	fn write_slot(&mut self) -> Step {
		let slot = self.mailbox.take();
		if slot.closed && !slot.fresh {
			return Step::Closed(slot.error);
		}
		let Some(frame) = slot.frame else {
			return Step::Closed(None);
		};
		if !slot.fresh {
			self.dropped += 1;
		}
		if let Err(e) = self
			.sink
			.write_frame(&frame.fit(self.width, self.height).data)
		{
			return Step::Failed(e);
		}
		self.frames += 1;
		self.next_slot += 1;
		Step::Wrote
	}

	/// The length of the file the grid has made.
	pub fn duration_ms(&self) -> u64 {
		self.frames * 1000 / self.fps
	}
}

#[cfg(test)]
mod tests {
	use super::*;
	use crate::frame::Frame;
	use std::sync::atomic::{AtomicU64, Ordering};

	struct Count(Arc<AtomicU64>);

	impl Sink for Count {
		fn write_frame(&mut self, _bytes: &[u8]) -> std::io::Result<()> {
			self.0.fetch_add(1, Ordering::Relaxed);
			Ok(())
		}

		fn finish(self: Box<Self>) -> Result<(), String> {
			Ok(())
		}
	}

	// A thread that was held up owes the slots it missed: at the end the file has the length of the clock.
	#[test]
	fn a_grid_that_was_held_up_writes_what_it_owes_at_the_end() {
		let mailbox = Mailbox::new();
		mailbox.push(Frame {
			width: 2,
			height: 2,
			data: vec![7; 16],
		});
		let written = Arc::new(AtomicU64::new(1));
		let mut grid = Grid::new(mailbox, Box::new(Count(Arc::clone(&written))), 2, 2, 60);
		let clock = Clock::new();
		clock.start(0);
		std::thread::sleep(Duration::from_millis(500));
		// nothing ran during that half second: 30 slots are owed, and the first one is slot 1
		grid.catch_up(&clock).unwrap();
		let slots = written.load(Ordering::Relaxed);
		assert!(
			(30..=33).contains(&slots),
			"{slots} slots for 0.5 s at 60 fps"
		);
		assert!(
			grid.duration_ms().abs_diff(500) <= 40,
			"{} ms",
			grid.duration_ms()
		);
		// a second call owes nothing
		let before = written.load(Ordering::Relaxed);
		grid.catch_up(&clock).unwrap();
		assert!(written.load(Ordering::Relaxed) - before <= 1);
	}

	#[test]
	fn a_paused_grid_owes_nothing() {
		let mailbox = Mailbox::new();
		mailbox.push(Frame {
			width: 2,
			height: 2,
			data: vec![7; 16],
		});
		let written = Arc::new(AtomicU64::new(0));
		let mut grid = Grid::new(mailbox, Box::new(Count(Arc::clone(&written))), 2, 2, 60);
		let clock = Clock::new();
		clock.start(0);
		clock.pause().unwrap();
		std::thread::sleep(Duration::from_millis(300));
		grid.catch_up(&clock).unwrap();
		assert!(written.load(Ordering::Relaxed) <= 1);
	}
}
