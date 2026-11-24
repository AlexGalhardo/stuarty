//! Pointer events (SDD-025 section 16.6): the position of the pointer and the edges of the three mouse buttons,
//! stamped in active time, as JSON lines. **No key is ever read**: the only input this file knows is a `Sample`,
//! which has a position and three buttons, and nothing in it can carry a key. The Windows source of samples
//! (win32.rs) asks about the three mouse buttons and nothing else.

use std::io::Write;
use std::path::PathBuf;
use std::sync::Arc;
use std::sync::atomic::{AtomicBool, Ordering};
use std::thread::JoinHandle;
use std::time::Duration;

use crate::clock::Clock;
use crate::protocol::StartRequest;
use crate::source::SourceError;

/// How often the pointer is looked at.
const POLL: Duration = Duration::from_millis(4);
/// A move is written when the position changed and this long has passed since the last one.
const MOVE_EVERY_MS: u64 = 16;
/// The most lines a file holds, the header included (core/recording MaxEventLines).
const MAX_LINES: u64 = 2_000_000;
const BUTTONS: [&str; 3] = ["left", "right", "middle"];

/// What one look at the pointer found: a position in pixels of the captured source, whether it is inside it,
/// and the state of the left, right and middle buttons.
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub struct Sample {
	pub x: i32,
	pub y: i32,
	pub inside: bool,
	pub buttons: [bool; 3],
}

/// Where samples come from. `active_ms` is the time on the clock, which the test pointer follows and the real
/// one ignores.
pub trait Pointer: Send {
	fn sample(&mut self, active_ms: u64) -> Option<Sample>;
}

/// Turns samples into lines. Each call to `step` may write a move and the edges of the buttons.
pub struct Recorder<W: Write> {
	out: W,
	width: i32,
	height: i32,
	last_move_ms: Option<u64>,
	last_pos: Option<(i32, i32)>,
	pressed: [bool; 3],
	recorded: [bool; 3],
	events: u64,
	lines: u64,
}

impl<W: Write> Recorder<W> {
	pub fn new(mut out: W, width: u32, height: u32) -> std::io::Result<Self> {
		writeln!(
			out,
			r#"{{"k":"header","v":1,"width":{width},"height":{height},"clock":"active_ms"}}"#
		)?;
		Ok(Self {
			out,
			width: i32::try_from(width).unwrap_or(i32::MAX),
			height: i32::try_from(height).unwrap_or(i32::MAX),
			last_move_ms: None,
			last_pos: None,
			pressed: [false; 3],
			recorded: [false; 3],
			events: 0,
			lines: 1,
		})
	}

	/// The number of events written, the header not counted.
	pub fn events(&self) -> u64 {
		self.events
	}

	fn room(&self) -> bool {
		self.lines < MAX_LINES
	}

	pub fn step(&mut self, t_ms: u64, s: Sample) -> std::io::Result<()> {
		let pos = (s.x.clamp(0, self.width - 1), s.y.clamp(0, self.height - 1));
		let mut wrote = false;

		if s.inside
			&& self.last_pos != Some(pos)
			&& self
				.last_move_ms
				.is_none_or(|l| t_ms.saturating_sub(l) >= MOVE_EVERY_MS)
			&& self.room()
		{
			writeln!(
				self.out,
				r#"{{"k":"move","t":{t_ms},"x":{},"y":{}}}"#,
				pos.0, pos.1
			)?;
			(self.last_move_ms, self.last_pos) = (Some(t_ms), Some(pos));
			self.lines += 1;
			self.events += 1;
			wrote = true;
		}

		for (i, name) in BUTTONS.iter().enumerate() {
			let down = s.buttons[i];
			if down && !self.pressed[i] && s.inside && self.room() {
				// A press that starts outside the source is not written, and neither is its release.
				self.edge("down", name, t_ms, pos)?;
				self.recorded[i] = true;
				wrote = true;
			} else if !down && self.pressed[i] && self.recorded[i] {
				// A release is written for every press that was, wherever the pointer is by then.
				self.edge("up", name, t_ms, pos)?;
				self.recorded[i] = false;
				wrote = true;
			}
			self.pressed[i] = down;
		}

		if wrote {
			self.out.flush()?;
		}
		Ok(())
	}

	fn edge(
		&mut self,
		kind: &str,
		button: &str,
		t_ms: u64,
		pos: (i32, i32),
	) -> std::io::Result<()> {
		writeln!(
			self.out,
			r#"{{"k":"{kind}","t":{t_ms},"b":"{button}","x":{},"y":{}}}"#,
			pos.0, pos.1
		)?;
		self.lines += 1;
		self.events += 1;
		Ok(())
	}

	pub fn finish(mut self) -> std::io::Result<u64> {
		self.out.flush()?;
		Ok(self.events)
	}
}

/// What the pointer track did.
#[derive(Debug, Default)]
pub struct Outcome {
	pub events: u64,
	pub error: Option<String>,
}

pub struct Track {
	handle: JoinHandle<Outcome>,
}

impl Track {
	pub fn join(self) -> Outcome {
		self.handle.join().unwrap_or_else(|_| Outcome {
			error: Some("the pointer thread panicked".into()),
			..Outcome::default()
		})
	}
}

/// Starts the sampler on a thread of its own: it waits for the clock, looks at the pointer every 4 ms except
/// while paused, and writes the file. The file and its header are made before it returns.
pub fn spawn(
	mut pointer: Box<dyn Pointer>,
	path: PathBuf,
	width: u32,
	height: u32,
	clock: Arc<Clock>,
	stop: Arc<AtomicBool>,
) -> Result<Track, SourceError> {
	let io = |e: std::io::Error| {
		SourceError::new(
			"io",
			format!("the pointer file could not be made: {}", e.kind()),
		)
	};
	let file = std::fs::File::create(&path).map_err(io)?;
	let mut rec = Recorder::new(std::io::BufWriter::new(file), width, height).map_err(io)?;
	let handle = std::thread::spawn(move || {
		let mut out = Outcome::default();
		while !clock.started() && !stop.load(Ordering::Relaxed) {
			std::thread::sleep(Duration::from_millis(2));
		}
		while !stop.load(Ordering::Relaxed) {
			if clock.paused() {
				std::thread::sleep(POLL);
				continue;
			}
			let t_ms = u64::try_from(clock.active_ns() / 1_000_000).unwrap_or(0);
			if let Some(s) = pointer.sample(t_ms)
				&& let Err(e) = rec.step(t_ms, s)
			{
				out.error = Some(format!(
					"the pointer file could not be written: {}",
					e.kind()
				));
				break;
			}
			std::thread::sleep(POLL);
		}
		out.events = rec.events();
		if let Err(e) = rec.finish() {
			out.error.get_or_insert(format!(
				"the pointer file could not be finished: {}",
				e.kind()
			));
		}
		out
	});
	Ok(Track { handle })
}

/// A scripted pointer for the tests of the helper (admitted only with `--allow-test-source`): it moves along a
/// line, and presses the left button for 60 ms at every 500 ms of active time, at a position that depends on
/// which press it is, so a test knows where and when each click must be found.
pub struct Scripted {
	width: i32,
	height: i32,
}

impl Scripted {
	pub fn new(width: u32, height: u32) -> Self {
		Self {
			width: i32::try_from(width).unwrap_or(i32::MAX),
			height: i32::try_from(height).unwrap_or(i32::MAX),
		}
	}

	/// The position of the k-th click (also where the pointer rests during it).
	pub fn click_at(k: u64, width: i32, height: i32) -> (i32, i32) {
		let k = i32::try_from(k % 1000).unwrap_or(0);
		((k * 7) % width, (k * 3) % height)
	}
}

impl Pointer for Scripted {
	fn sample(&mut self, active_ms: u64) -> Option<Sample> {
		let (k, into) = (active_ms / 500, active_ms % 500);
		if into < 60 {
			let (x, y) = Self::click_at(k, self.width, self.height);
			return Some(Sample {
				x,
				y,
				inside: true,
				buttons: [true, false, false],
			});
		}
		let t = i32::try_from(active_ms % 100_000).unwrap_or(0);
		Some(Sample {
			x: (t / 10) % self.width,
			y: (t / 20) % self.height,
			inside: true,
			buttons: [false; 3],
		})
	}
}

/// The pointer a request names: the scripted one for a test source, the real one on Windows.
pub fn open_pointer(
	req: &StartRequest,
	width: u32,
	height: u32,
) -> Result<Box<dyn Pointer>, SourceError> {
	if req.kind == "fake" {
		return Ok(Box::new(Scripted::new(width, height)));
	}
	#[cfg(windows)]
	{
		crate::windows::pointer(req)
	}
	#[cfg(not(windows))]
	{
		let _ = (width, height);
		Err(SourceError::new(
			"unsupported",
			"pointer events exist only on Windows in this version",
		))
	}
}

#[cfg(test)]
mod tests {
	use super::*;

	fn rec(w: u32, h: u32) -> Recorder<Vec<u8>> {
		Recorder::new(Vec::new(), w, h).unwrap()
	}

	fn text(r: Recorder<Vec<u8>>) -> Vec<String> {
		String::from_utf8(r.out)
			.unwrap()
			.lines()
			.map(String::from)
			.collect()
	}

	fn at(x: i32, y: i32, buttons: [bool; 3]) -> Sample {
		Sample {
			x,
			y,
			inside: true,
			buttons,
		}
	}

	#[test]
	fn the_file_starts_with_the_header() {
		let r = rec(1920, 1080);
		assert_eq!(
			text(r),
			[r#"{"k":"header","v":1,"width":1920,"height":1080,"clock":"active_ms"}"#]
		);
	}

	#[test]
	fn a_click_is_a_down_and_an_up_at_the_pointer() {
		let mut r = rec(100, 100);
		r.step(10, at(5, 6, [false; 3])).unwrap();
		r.step(500, at(40, 50, [true, false, false])).unwrap();
		r.step(560, at(41, 50, [false; 3])).unwrap();
		let n = r.events();
		let lines = text(r);
		assert_eq!(n, 5);
		assert_eq!(lines[1], r#"{"k":"move","t":10,"x":5,"y":6}"#);
		assert_eq!(lines[2], r#"{"k":"move","t":500,"x":40,"y":50}"#);
		assert_eq!(lines[3], r#"{"k":"down","t":500,"b":"left","x":40,"y":50}"#);
		assert_eq!(lines[4], r#"{"k":"move","t":560,"x":41,"y":50}"#);
		assert_eq!(lines[5], r#"{"k":"up","t":560,"b":"left","x":41,"y":50}"#);
	}

	#[test]
	fn moves_are_throttled_and_a_still_pointer_writes_nothing() {
		let mut r = rec(100, 100);
		for (t, x) in [(0, 1), (4, 2), (8, 3), (15, 4), (16, 5), (20, 5), (40, 5)] {
			r.step(t, at(x, 0, [false; 3])).unwrap();
		}
		let lines = text(r);
		// t 0 (x 1) and t 16 (x 5); the others are inside 16 ms of a move, or did not move
		assert_eq!(lines.len(), 3, "{lines:?}");
		assert!(lines[2].contains(r#""t":16"#) && lines[2].contains(r#""x":5"#));
	}

	#[test]
	fn a_press_outside_the_source_and_its_release_are_not_written() {
		let mut r = rec(100, 100);
		let out = Sample {
			x: 500,
			y: 5,
			inside: false,
			buttons: [true, false, false],
		};
		r.step(10, out).unwrap();
		// the pointer comes in while the button is still down: that is not a press inside
		r.step(30, at(5, 5, [true, false, false])).unwrap();
		r.step(60, at(5, 5, [false; 3])).unwrap();
		let lines = text(r);
		assert!(
			!lines
				.iter()
				.any(|l| l.contains(r#""k":"down""#) || l.contains(r#""k":"up""#)),
			"{lines:?}"
		);
	}

	#[test]
	fn a_release_is_written_for_every_press_that_was_even_outside() {
		let mut r = rec(100, 100);
		r.step(10, at(99, 5, [false, true, false])).unwrap();
		r.step(20, at(99, 5, [false, true, false])).unwrap();
		r.step(
			30,
			Sample {
				x: 400,
				y: -20,
				inside: false,
				buttons: [false; 3],
			},
		)
		.unwrap();
		let lines = text(r);
		let up = lines.last().unwrap();
		// the position of the release is kept inside the source, which the file format requires
		assert_eq!(
			up, r#"{"k":"up","t":30,"b":"right","x":99,"y":0}"#,
			"{lines:?}"
		);
	}

	#[test]
	fn every_line_is_valid_for_the_reader_of_the_server() {
		// The same bounds as core/recording: at most 200 bytes a line, coordinates inside the header size.
		let mut r = rec(8192, 8192);
		r.step(14_399_999, at(8191, 8191, [true, true, true]))
			.unwrap();
		r.step(14_400_000, at(0, 0, [false; 3])).unwrap();
		for l in text(r) {
			assert!(l.len() <= 200, "{l}");
			assert!(crate::json::parse(&l).is_ok(), "{l}");
			assert!(!l.contains("key"), "{l}");
		}
	}

	#[test]
	fn the_scripted_pointer_clicks_where_and_when_it_says() {
		let mut p = Scripted::new(16, 16);
		assert!(p.sample(0).unwrap().buttons[0]);
		assert!(p.sample(59).unwrap().buttons[0]);
		assert!(!p.sample(60).unwrap().buttons[0]);
		assert!(p.sample(500).unwrap().buttons[0]);
		let s = p.sample(1000).unwrap();
		assert_eq!((s.x, s.y), Scripted::click_at(2, 16, 16));
	}

	/// A pointer that is real on this machine: it reads the position and the three mouse buttons, nothing else.
	/// It moves and presses nothing, so it is safe to run anywhere there is a desktop.
	#[cfg(windows)]
	#[test]
	fn the_real_pointer_reads_a_position_on_the_desktop() {
		let (x, y) = crate::win32::cursor_pos().expect("the cursor has a position");
		assert!(
			(-40_000..40_000).contains(&x) && (-40_000..40_000).contains(&y),
			"{x},{y}"
		);
		let _ = crate::win32::mouse_buttons();
		assert_eq!(crate::win32::MOUSE_BUTTONS, [0x01, 0x02, 0x04]);
	}
}
