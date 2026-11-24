//! Windows Graphics Capture through `windows-capture` (ADR-0049). One display or one window, BGRA, no cursor
//! (cursor events are epic 62), and the system capture border always on: it is the indicator that the screen is
//! being recorded, and the helper never turns it off (SDD-025 section 8).

use std::sync::Arc;
use std::time::Duration;

use windows_capture::capture::{CaptureControl, Context, GraphicsCaptureApiHandler};
use windows_capture::frame::Frame as WgcFrame;
use windows_capture::graphics_capture_api::InternalCaptureControl;
use windows_capture::monitor::Monitor;
use windows_capture::settings::{
	ColorFormat, CursorCaptureSettings, DirtyRegionSettings, DrawBorderSettings,
	MinimumUpdateIntervalSettings, SecondaryWindowSettings, Settings,
};
use windows_capture::window::Window;

use crate::frame::{Frame, Mailbox};
use crate::protocol::{SourceInfo, StartRequest, printable};
use crate::source::{RunningSource, SourceError};

type BoxErr = Box<dyn std::error::Error + Send + Sync>;

/// Windows throttles frame delivery by default (about 42 per second for a 74 Hz window, measured in the spike);
/// any explicit interval lifts it to the compositor rate. The 60 fps grid is made by the session, so the
/// interval only has to stay under one frame.
const MIN_UPDATE: Duration = Duration::from_millis(8);

/// A frame that cannot be read is skipped: the slot of the grid repeats the last one and counts as dropped. Only a
/// run of this many in a row ends the recording, so one hiccup of the graphics driver does not lose a take.
const MAX_UNREADABLE_IN_A_ROW: u32 = 120;

struct Handler {
	mailbox: Arc<Mailbox>,
	scratch: Vec<u8>,
	unreadable: u32,
}

impl GraphicsCaptureApiHandler for Handler {
	type Flags = Arc<Mailbox>;
	type Error = BoxErr;

	fn new(ctx: Context<Self::Flags>) -> Result<Self, Self::Error> {
		Ok(Self {
			mailbox: ctx.flags,
			scratch: Vec::new(),
			unreadable: 0,
		})
	}

	fn on_frame_arrived(
		&mut self,
		frame: &mut WgcFrame,
		_control: InternalCaptureControl,
	) -> Result<(), Self::Error> {
		let (width, height) = (frame.width(), frame.height());
		let result = frame
			.buffer()
			.map(|fb| fb.as_nopadding_buffer(&mut self.scratch).to_vec());
		match result {
			Ok(data) => {
				self.unreadable = 0;
				self.mailbox.push(Frame {
					width,
					height,
					data,
				});
				Ok(())
			}
			Err(e) => {
				self.unreadable += 1;
				if self.unreadable == 1 {
					eprintln!("a frame could not be read: {e}");
				}
				if self.unreadable >= MAX_UNREADABLE_IN_A_ROW {
					self.mailbox.close(Some(format!(
						"{} frames in a row could not be read: {e}",
						self.unreadable
					)));
					return Err(e.into());
				}
				Ok(())
			}
		}
	}

	fn on_closed(&mut self) -> Result<(), Self::Error> {
		// The window or the display went away: a normal end, the file is kept.
		self.mailbox.close(None);
		Ok(())
	}
}

struct Running(CaptureControl<Handler, BoxErr>);

impl RunningSource for Running {
	fn stop(self: Box<Self>) {
		let _ = self.0.stop();
	}
}

fn settings<T: TryInto<windows_capture::settings::GraphicsCaptureItemType> + Send + 'static>(
	item: T,
	mailbox: &Arc<Mailbox>,
) -> Settings<Arc<Mailbox>, T> {
	Settings::new(
		item,
		CursorCaptureSettings::WithoutCursor,
		DrawBorderSettings::WithBorder,
		SecondaryWindowSettings::Exclude,
		MinimumUpdateIntervalSettings::Custom(MIN_UPDATE),
		DirtyRegionSettings::Default,
		ColorFormat::Bgra8,
		Arc::clone(mailbox),
	)
}

fn failed(what: &str, e: impl std::fmt::Display) -> SourceError {
	SourceError::new(
		"unsupported",
		format!("{what}: {}", printable(&e.to_string())),
	)
}

pub fn open(
	req: &StartRequest,
	mailbox: &Arc<Mailbox>,
) -> Result<Box<dyn RunningSource>, SourceError> {
	match req.kind.as_str() {
		"display" => {
			let index: usize = req
				.source_id
				.strip_prefix('d')
				.and_then(|n| n.parse().ok())
				.ok_or_else(|| {
					SourceError::new("source_gone", "the display id is not one the helper made")
				})?;
			let monitor = Monitor::from_index(index)
				.map_err(|_| SourceError::new("source_gone", "the display is not there"))?;
			let control = Handler::start_free_threaded(settings(monitor, mailbox))
				.map_err(|e| failed("capture", e))?;
			Ok(Box::new(Running(control)))
		}
		"window" => {
			let raw: usize = req
				.source_id
				.strip_prefix('w')
				.and_then(|n| n.parse().ok())
				.ok_or_else(|| {
					SourceError::new("source_gone", "the window id is not one the helper made")
				})?;
			// Only a window the helper itself lists can be captured, never an arbitrary handle.
			let window = Window::enumerate()
				.map_err(|e| failed("windows", e))?
				.into_iter()
				.find(|w| w.as_raw_hwnd() as usize == raw)
				.ok_or_else(|| SourceError::new("source_gone", "the window is not there"))?;
			let control = Handler::start_free_threaded(settings(window, mailbox))
				.map_err(|e| failed("capture", e))?;
			Ok(Box::new(Running(control)))
		}
		_ => Err(SourceError::new(
			"unsupported",
			"the source kind is not display or window",
		)),
	}
}

pub fn list() -> (Vec<SourceInfo>, Vec<SourceInfo>) {
	let displays = Monitor::enumerate()
		.unwrap_or_default()
		.into_iter()
		.filter_map(|m| {
			let index = m.index().ok()?;
			Some(SourceInfo {
				id: format!("d{index}"),
				name: m.name().unwrap_or_else(|_| format!("Display {index}")),
				width: m.width().ok()?,
				height: m.height().ok()?,
			})
		})
		.collect();
	let windows = Window::enumerate()
		.unwrap_or_default()
		.into_iter()
		.filter_map(|w| {
			let title = w.title().ok().filter(|t| !t.trim().is_empty())?;
			let (width, height) = (
				u32::try_from(w.width().ok()?).ok()?,
				u32::try_from(w.height().ok()?).ok()?,
			);
			(width > 0 && height > 0).then(|| SourceInfo {
				id: format!("w{}", w.as_raw_hwnd() as usize),
				name: title.chars().take(200).collect(),
				width,
				height,
			})
		})
		.collect();
	(displays, windows)
}
