//! The grid of a video track (SDD-025 sections 8 and 16.5): one frame per slot of an absolute grid of the shared
//! clock, the last frame repeated when the source is slow. The screen and the camera each have one.

use std::sync::Arc;
use std::time::Duration;

use crate::clock::Clock;
use crate::frame::Mailbox;
use crate::sink::Sink;

const MAX_WAIT: Duration = Duration::from_millis(10);

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
