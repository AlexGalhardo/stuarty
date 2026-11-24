//! The webcam: a video track of its own, on its own file, written on the grid of the shared clock (SDD-025
//! section 16.5). The device sits behind the same mailbox as the screen. On Windows the device is Media
//! Foundation (camera_mf.rs, D-142); the test camera of `--allow-test-source` makes the same track without a
//! device, which is how the file and the clock are tested.

use std::path::Path;
use std::sync::Arc;
use std::sync::atomic::{AtomicBool, Ordering};
use std::thread::JoinHandle;
use std::time::Duration;

use crate::clock::Clock;
use crate::frame::Mailbox;
use crate::protocol::{SourceInfo, StartRequest};
use crate::sink::{FfmpegSink, FileSink, Sink};
use crate::source::{self, RunningSource, SourceError};
use crate::testsrc;
use crate::video::{Grid, Step};

const FIRST_FRAME_WAIT: Duration = Duration::from_secs(5);

/// What a camera track did.
#[derive(Debug, Default)]
pub struct Outcome {
	pub frames: u64,
	pub duration_ms: u64,
	pub bytes: u64,
	/// The camera went away during the recording or its file could not be written.
	pub lost: bool,
	pub error: Option<String>,
}

pub struct Track {
	handle: JoinHandle<Outcome>,
}

impl Track {
	pub fn join(self) -> Outcome {
		self.handle.join().unwrap_or_else(|_| Outcome {
			lost: true,
			error: Some("the camera thread panicked".into()),
			..Outcome::default()
		})
	}
}

/// The cameras that can be recorded. Only the test camera exists, with the test flag.
pub fn list(allow_test: bool) -> Vec<SourceInfo> {
	#[cfg(windows)]
	let mut out = crate::camera_mf::list();
	#[cfg(not(windows))]
	let mut out = Vec::new();
	if allow_test {
		out.push(SourceInfo {
			id: "fake".into(),
			name: "Test camera".into(),
			width: 0,
			height: 0,
			default: out.is_empty(),
		});
	}
	out
}

/// Whether this system can record a camera at all.
pub fn can_capture() -> bool {
	cfg!(windows)
}

fn open(id: &str, mailbox: &Arc<Mailbox>) -> Result<Box<dyn RunningSource>, SourceError> {
	match testsrc::parse_tags(id) {
		Some(t) => {
			// A test camera is a small source (160 by 90) at 120 frames a second with the pulse of the test epoch:
			// fast, so the latency of the source stays well under the 40 ms the sync test measures.
			let spec = format!("160x90r120p{}w{}", t.period_ms, t.width_ms);
			source::open_fake(&spec, mailbox)
		}
		#[cfg(windows)]
		None => crate::camera_mf::open(id, mailbox),
		#[cfg(not(windows))]
		None => Err(SourceError::new(
			"unsupported",
			"there is no camera backend on this system yet",
		)),
	}
}

/// Opens the camera and its file, writes the first frame to warm the encoder, and starts the track on a thread
/// that waits for the clock. The error is why the camera could not be used; nothing is left running then.
pub fn spawn(
	req: &StartRequest,
	id: &str,
	output: &Path,
	clock: Arc<Clock>,
	stop: Arc<AtomicBool>,
) -> Result<Track, SourceError> {
	let mailbox = Mailbox::new();
	let src = open(id, &mailbox)?;
	let first = mailbox.wait_first(FIRST_FRAME_WAIT);
	let Some(frame) = first.frame else {
		src.stop();
		return Err(SourceError::new(
			"device_unavailable",
			first.error.as_deref().unwrap_or("the camera gave no frame"),
		));
	};
	let (w, h) = (frame.width & !1, frame.height & !1);
	if w < 16 || h < 16 || w > 8192 || h > 8192 {
		src.stop();
		return Err(SourceError::new(
			"unsupported",
			"the size of the camera is outside the supported range",
		));
	}
	let sink: Result<Box<dyn Sink>, String> = if req.encoder == "raw" {
		FileSink::create(output).map(|s| Box::new(s) as Box<dyn Sink>)
	} else {
		FfmpegSink::spawn_to(req, output, w, h).map(|s| Box::new(s) as Box<dyn Sink>)
	};
	let mut sink = match sink {
		Ok(s) => s,
		Err(m) => {
			src.stop();
			return Err(SourceError::new("encoder_failed", m));
		}
	};
	if let Err(e) = sink.write_frame(&frame.fit(w, h).data) {
		src.stop();
		return Err(SourceError::new("encoder_failed", e.to_string()));
	}
	mailbox.take();

	let fps = req.fps;
	let path = output.to_path_buf();
	let handle = std::thread::spawn(move || {
		crate::clock::raise_this_thread();
		let mut out = Outcome::default();
		let mut grid = Grid::new(mailbox, sink, w, h, fps);
		while !clock.started() {
			if stop.load(Ordering::Relaxed) {
				break;
			}
			std::thread::sleep(Duration::from_millis(2));
		}
		while !stop.load(Ordering::Relaxed) {
			match grid.step(&clock) {
				Step::Wait(d) => std::thread::sleep(d),
				Step::Wrote => {}
				Step::Closed(why) => {
					out.lost = true;
					out.error = why;
					break;
				}
				Step::Failed(e) => {
					out.lost = true;
					out.error = Some(format!(
						"the camera file could not be written: {}",
						e.kind()
					));
					break;
				}
			}
		}
		if !out.lost && clock.started() {
			let _ = grid.catch_up(&clock);
		}
		src.stop();
		out.frames = grid.frames;
		out.duration_ms = grid.duration_ms();
		if let Err(m) = grid.sink.finish() {
			out.lost = true;
			out.error.get_or_insert(m);
		}
		out.bytes = std::fs::metadata(&path).map_or(0, |m| m.len());
		out
	});
	Ok(Track { handle })
}
