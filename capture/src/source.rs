//! Frame sources. The real one is Windows Graphics Capture (windows.rs); the fake one makes a moving pattern and
//! exists only for tests, behind the policy flag that the server never passes.

use std::sync::Arc;
use std::sync::atomic::{AtomicBool, Ordering};
use std::thread::JoinHandle;
use std::time::{Duration, Instant};

use crate::frame::{Frame, Mailbox};
use crate::protocol::{SourceInfo, StartRequest};
use crate::testsrc;

#[derive(Debug)]
pub struct SourceError {
	pub code: &'static str,
	pub message: String,
}

impl SourceError {
	pub fn new(code: &'static str, message: impl Into<String>) -> Self {
		Self {
			code,
			message: message.into(),
		}
	}
}

/// A source that is producing frames into a mailbox.
pub trait RunningSource: Send {
	/// Stops the source and waits for it to end.
	fn stop(self: Box<Self>);
}

/// Starts a test source from its id (`WxH` and tags); the test camera uses it too.
pub fn open_fake(id: &str, mailbox: &Arc<Mailbox>) -> Result<Box<dyn RunningSource>, SourceError> {
	Fake::start(id, mailbox)
}

/// Opens the source a request names and starts it. Frames go to the mailbox until it is stopped or closed.
pub fn open(
	req: &StartRequest,
	mailbox: &Arc<Mailbox>,
) -> Result<Box<dyn RunningSource>, SourceError> {
	if req.kind == "fake" {
		return Fake::start(&req.source_id, mailbox);
	}
	#[cfg(windows)]
	{
		crate::windows::open(req, mailbox)
	}
	#[cfg(not(windows))]
	{
		let _ = mailbox;
		Err(SourceError::new(
			"unsupported",
			"capture exists only on Windows in this version",
		))
	}
}

/// The displays and windows that can be captured. Empty where there is no capture.
pub fn list() -> (Vec<SourceInfo>, Vec<SourceInfo>) {
	#[cfg(windows)]
	{
		crate::windows::list()
	}
	#[cfg(not(windows))]
	{
		(Vec::new(), Vec::new())
	}
}

pub fn can_capture() -> (bool, bool) {
	let on = cfg!(windows);
	(on, on)
}

/// The fake source: `WxH`, then optionally `rN` (frames per second) and `nN` (end after N frames).
struct Fake {
	stop: Arc<AtomicBool>,
	thread: Option<JoinHandle<()>>,
}

struct FakeSpec {
	width: u32,
	height: u32,
	rate: u32,
	limit: Option<u64>,
	/// `p<ms>` and `w<ms>`: a flash (every byte 255, otherwise 10) for the first `w` ms of every `p` ms of the
	/// shared test epoch, which is when the fake microphone makes its tone burst.
	pulse: Option<testsrc::Tags>,
}

fn parse_fake(id: &str) -> Option<FakeSpec> {
	let (w, rest) = id.split_once('x')?;
	let mut spec = FakeSpec {
		width: w.parse().ok()?,
		height: 0,
		rate: 60,
		limit: None,
		pulse: None,
	};
	let digits = |s: &str| s.chars().take_while(char::is_ascii_digit).count();
	let n = digits(rest);
	spec.height = rest[..n].parse().ok()?;
	let mut tail = &rest[n..];
	let mut pulse = testsrc::Tags::default();
	while !tail.is_empty() {
		let (tag, after) = tail.split_at(1);
		let n = digits(after);
		let v: u64 = after[..n].parse().ok()?;
		match tag {
			"r" => spec.rate = u32::try_from(v).ok().filter(|r| (1..=240).contains(r))?,
			"n" => spec.limit = Some(v),
			"p" => {
				pulse.period_ms = v;
				spec.pulse = Some(pulse);
			}
			"w" => {
				pulse.width_ms = v;
				spec.pulse = Some(pulse);
			}
			_ => return None,
		}
		tail = &after[n..];
	}
	if let Some(p) = spec.pulse {
		let ok = (100..=60_000).contains(&p.period_ms)
			&& (10..=10_000).contains(&p.width_ms)
			&& p.width_ms < p.period_ms;
		if !ok {
			return None;
		}
		// The epoch is the instant the first test source is made.
		let _ = testsrc::epoch();
	}
	(spec.width >= 2 && spec.width <= 4096 && spec.height >= 2 && spec.height <= 4096)
		.then_some(spec)
}

impl Fake {
	fn start(id: &str, mailbox: &Arc<Mailbox>) -> Result<Box<dyn RunningSource>, SourceError> {
		let spec = parse_fake(id)
			.ok_or_else(|| SourceError::new("source_gone", "the test source id is malformed"))?;
		let stop = Arc::new(AtomicBool::new(false));
		let (s, m) = (Arc::clone(&stop), Arc::clone(mailbox));
		let thread = std::thread::spawn(move || {
			let t0 = Instant::now();
			let mut n: u64 = 0;
			while !s.load(Ordering::Relaxed) {
				if spec.limit.is_some_and(|l| n >= l) {
					m.close(None);
					return;
				}
				let value = match &spec.pulse {
					Some(p) if testsrc::pulse_on(p, testsrc::ns_since_epoch(Instant::now())) => 255,
					Some(_) => 10,
					None => (n % 251) as u8,
				};
				m.push(Frame {
					width: spec.width,
					height: spec.height,
					data: vec![value; spec.width as usize * spec.height as usize * 4],
				});
				n += 1;
				let due = t0 + Duration::from_secs_f64(n as f64 / f64::from(spec.rate));
				std::thread::sleep(due.saturating_duration_since(Instant::now()));
			}
		});
		Ok(Box::new(Self {
			stop,
			thread: Some(thread),
		}))
	}
}

impl RunningSource for Fake {
	fn stop(mut self: Box<Self>) {
		self.stop.store(true, Ordering::Relaxed);
		if let Some(t) = self.thread.take() {
			let _ = t.join();
		}
	}
}

#[cfg(test)]
mod tests {
	use super::*;

	#[test]
	fn the_fake_id_is_parsed() {
		let s = parse_fake("320x240").unwrap();
		assert_eq!((s.width, s.height, s.rate, s.limit), (320, 240, 60, None));
		let s = parse_fake("64x48r20n30").unwrap();
		assert_eq!((s.width, s.height, s.rate, s.limit), (64, 48, 20, Some(30)));
		for bad in [
			"",
			"320",
			"x240",
			"320x",
			"320x240q1",
			"320x240r0",
			"1x1",
			"9000x100",
		] {
			assert!(parse_fake(bad).is_none(), "{bad}");
		}
	}

	#[test]
	fn the_fake_source_ends_after_its_limit() {
		let m = Mailbox::new();
		let src = Fake::start("16x16r240n5", &m).unwrap();
		let deadline = Instant::now() + Duration::from_secs(5);
		while !m.take().closed && Instant::now() < deadline {
			std::thread::sleep(Duration::from_millis(5));
		}
		assert_eq!(m.arrived(), 5);
		src.stop();
	}
}
