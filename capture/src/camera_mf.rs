//! The webcam of Windows through Media Foundation (SDD-025 section 16.5), with the `windows` crate that D-142
//! approves for this module. A source reader with video processing on gives BGRA-like RGB32 frames whatever the
//! camera sends (YUY2, NV12, MJPEG); they go to the same mailbox as the screen. Listing a camera only
//! enumerates the devices: no camera is opened, so no camera light comes on, until a person starts a recording
//! with it. This is a reviewed boundary module (rule .agents/rules/rust.md 4): every `unsafe` block says why.
#![allow(unsafe_code)]

use std::marker::PhantomData;
use std::sync::Arc;
use std::sync::atomic::{AtomicBool, Ordering};
use std::sync::mpsc;
use std::time::Duration;

use windows::Win32::Media::MediaFoundation::{
	IMFActivate, IMFAttributes, IMFMediaSource, IMFSourceReader,
	MF_DEVSOURCE_ATTRIBUTE_FRIENDLY_NAME, MF_DEVSOURCE_ATTRIBUTE_SOURCE_TYPE,
	MF_DEVSOURCE_ATTRIBUTE_SOURCE_TYPE_VIDCAP_GUID,
	MF_DEVSOURCE_ATTRIBUTE_SOURCE_TYPE_VIDCAP_SYMBOLIC_LINK, MF_MT_DEFAULT_STRIDE,
	MF_MT_FRAME_SIZE, MF_MT_MAJOR_TYPE, MF_MT_SUBTYPE, MF_SOURCE_READER_ENABLE_VIDEO_PROCESSING,
	MF_SOURCE_READER_FIRST_VIDEO_STREAM, MF_SOURCE_READERF_ENDOFSTREAM, MF_VERSION,
	MFCreateAttributes, MFCreateMediaType, MFCreateSourceReaderFromMediaSource,
	MFEnumDeviceSources, MFMediaType_Video, MFShutdown, MFStartup, MFVideoFormat_RGB32,
};
use windows::Win32::System::Com::{
	COINIT_MULTITHREADED, CoInitializeEx, CoTaskMemFree, CoUninitialize,
};
use windows::core::GUID;

use crate::audio::device_id;
use crate::frame::{Frame, Mailbox};
use crate::protocol::{SourceInfo, printable};
use crate::source::{RunningSource, SourceError};

/// The largest camera picture asked for: more is no use in a recording of a face.
const MAX_AREA: u64 = 1280 * 720;
const READ_FLAGS: u32 = 0;
/// How long stopping waits for a camera that is stuck inside a read before the thread is left behind.
const STOP_WAIT: Duration = Duration::from_secs(3);
/// Frames of the stream are never fewer than this wide, as the screen (src/session.rs).
const MIN_SIDE: u32 = 16;

struct Entry {
	id: String,
	name: String,
	activate: IMFActivate,
}

/// Starts COM and Media Foundation on the calling thread for as long as the guard lives. Every Media Foundation
/// object made under it must be dropped before it is.
struct Runtime {
	/// Whether this guard's CoInitializeEx succeeded (S_OK or S_FALSE), so it owes one CoUninitialize.
	com: bool,
	/// COM and Media Foundation are counted per thread: the guard must not move to another one.
	_thread: PhantomData<*const ()>,
}

impl Runtime {
	fn start() -> Result<Self, String> {
		// SAFETY: a plain call with constants. A thread that already has COM in another mode gets
		// RPC_E_CHANGED_MODE, which is fine (Media Foundation works in either) and owes no CoUninitialize.
		let com = unsafe { CoInitializeEx(None, COINIT_MULTITHREADED) }.is_ok();
		// SAFETY: a plain call with constants; on failure the COM count taken above is given back here, since no
		// guard exists yet to do it.
		if let Err(e) = unsafe { MFStartup(MF_VERSION, 0) } {
			if com {
				// SAFETY: balances the successful CoInitializeEx above on this same thread.
				unsafe { CoUninitialize() };
			}
			return Err(e.to_string());
		}
		Ok(Self {
			com,
			_thread: PhantomData,
		})
	}
}

impl Drop for Runtime {
	fn drop(&mut self) {
		// SAFETY: balances the MFStartup of `start`, on the thread that called it (the guard is not Send).
		let _ = unsafe { MFShutdown() };
		if self.com {
			// SAFETY: balances the successful CoInitializeEx of `start` on the same thread.
			unsafe { CoUninitialize() };
		}
	}
}

fn string_attr(a: &IMFAttributes, key: &GUID) -> Option<String> {
	// SAFETY: `key` points to a live GUID; the buffer is one longer than the length the object reported and is
	// only read up to that length.
	unsafe {
		let n = a.GetStringLength(key).ok()? as usize;
		let mut buf = vec![0u16; n + 1];
		a.GetString(key, &mut buf, None).ok()?;
		Some(String::from_utf16_lossy(&buf[..n]))
	}
}

/// Every video capture device, as Media Foundation lists them. Nothing is opened.
fn enumerate() -> Result<Vec<Entry>, String> {
	// SAFETY: every pointer is to a live local; the array that MFEnumDeviceSources allocates is read element by
	// element, each element is moved out once (so its reference is ours and is released when the entry is
	// dropped), and the array itself is freed once with CoTaskMemFree.
	unsafe {
		let mut attrs: Option<IMFAttributes> = None;
		MFCreateAttributes(&mut attrs, 1).map_err(|e| e.to_string())?;
		let attrs = attrs.ok_or("no attributes")?;
		attrs
			.SetGUID(
				&MF_DEVSOURCE_ATTRIBUTE_SOURCE_TYPE,
				&MF_DEVSOURCE_ATTRIBUTE_SOURCE_TYPE_VIDCAP_GUID,
			)
			.map_err(|e| e.to_string())?;
		let mut list: *mut Option<IMFActivate> = std::ptr::null_mut();
		let mut count = 0u32;
		MFEnumDeviceSources(&attrs, &mut list, &mut count).map_err(|e| e.to_string())?;
		let mut out = Vec::new();
		for i in 0..count as usize {
			let Some(activate) = list.add(i).read() else {
				continue;
			};
			let attrs: &IMFAttributes = &activate;
			let (Some(link), Some(name)) = (
				string_attr(
					attrs,
					&MF_DEVSOURCE_ATTRIBUTE_SOURCE_TYPE_VIDCAP_SYMBOLIC_LINK,
				),
				string_attr(attrs, &MF_DEVSOURCE_ATTRIBUTE_FRIENDLY_NAME),
			) else {
				continue;
			};
			out.push(Entry {
				id: device_id('c', &link),
				name,
				activate,
			});
		}
		CoTaskMemFree(Some(list.cast_const().cast()));
		Ok(out)
	}
}

/// The cameras of this machine for a sources message. Empty when Media Foundation is not there or no camera is.
pub fn list() -> Vec<SourceInfo> {
	let Ok(_rt) = Runtime::start() else {
		return Vec::new();
	};
	let found = enumerate().unwrap_or_default();
	found
		.into_iter()
		.enumerate()
		.map(|(i, e)| SourceInfo {
			id: e.id,
			name: e.name.chars().take(200).collect(),
			width: 0,
			height: 0,
			default: i == 0,
		})
		.collect()
}

/// What a source reader answers about its picture.
struct Shape {
	width: u32,
	height: u32,
	/// Rows run bottom to top (a negative stride), as RGB32 does unless told otherwise.
	bottom_up: bool,
}

/// Turns the bytes of a sample into a top-down picture of `width` by `height` BGRA pixels. A sample shorter than
/// the picture is refused rather than read past its end.
pub fn top_down(data: &[u8], width: u32, height: u32, bottom_up: bool) -> Option<Vec<u8>> {
	let row = (width as usize).checked_mul(4)?;
	let need = row.checked_mul(height as usize)?;
	if row == 0 || data.len() < need {
		return None;
	}
	if !bottom_up {
		return Some(data[..need].to_vec());
	}
	let mut out = Vec::with_capacity(need);
	for y in (0..height as usize).rev() {
		out.extend_from_slice(&data[y * row..(y + 1) * row]);
	}
	Some(out)
}

fn open_reader(id: &str) -> Result<(IMFSourceReader, Shape), SourceError> {
	let unavailable = |why: String| SourceError::new("device_unavailable", printable(&why));
	let found = enumerate().map_err(unavailable)?;
	// Only a camera the helper itself lists can be opened, never an arbitrary id.
	let entry = found
		.into_iter()
		.find(|e| e.id == id)
		.ok_or_else(|| SourceError::new("device_unavailable", "the camera is not there"))?;
	// SAFETY: every pointer is to a live local; the reader and the media types are reference counted objects
	// that this function owns; the stream index constant is the documented -4 as an unsigned number.
	unsafe {
		// The attributes come first, so nothing can fail between opening the camera and handing it to the reader
		// but the reader itself.
		let mut attrs: Option<IMFAttributes> = None;
		MFCreateAttributes(&mut attrs, 1).map_err(|e| unavailable(e.to_string()))?;
		let attrs = attrs.ok_or_else(|| unavailable("no attributes".into()))?;
		attrs
			.SetUINT32(&MF_SOURCE_READER_ENABLE_VIDEO_PROCESSING, 1)
			.map_err(|e| unavailable(e.to_string()))?;
		let source: IMFMediaSource = entry
			.activate
			.ActivateObject()
			.map_err(|e| unavailable(format!("the camera could not be opened: {e}")))?;
		// From here the reader owns the source and shuts it down when it is released (the default of
		// MF_SOURCE_READER_DISCONNECT_MEDIASOURCE_ON_SHUTDOWN), so every later error path releases the camera.
		let reader = match MFCreateSourceReaderFromMediaSource(&source, &attrs) {
			Ok(r) => r,
			Err(e) => {
				// No reader took the source: shut it down here, or the camera stays in use (and its light on).
				let _ = source.Shutdown();
				return Err(unavailable(format!(
					"the camera reader could not be made: {e}"
				)));
			}
		};
		let stream = MF_SOURCE_READER_FIRST_VIDEO_STREAM.0 as u32;

		// The largest native picture that fits in 1280 by 720; a camera with only larger ones gets the smallest.
		let (mut best, mut best_area, mut smallest, mut smallest_area) =
			(None, 0u64, None, u64::MAX);
		let mut i = 0;
		while let Ok(t) = reader.GetNativeMediaType(stream, i) {
			i += 1;
			let Ok(packed) = t.GetUINT64(&MF_MT_FRAME_SIZE) else {
				continue;
			};
			let area = (packed >> 32) * (packed & 0xffff_ffff);
			if area <= MAX_AREA && area > best_area {
				(best, best_area) = (Some(t.clone()), area);
			}
			if area < smallest_area {
				(smallest, smallest_area) = (Some(t), area);
			}
		}
		if let Some(native) = best.or(smallest) {
			reader
				.SetCurrentMediaType(stream, None, &native)
				.map_err(|e| {
					unavailable(format!("the camera refused its own picture size: {e}"))
				})?;
		}

		let want = MFCreateMediaType().map_err(|e| unavailable(e.to_string()))?;
		want.SetGUID(&MF_MT_MAJOR_TYPE, &MFMediaType_Video)
			.map_err(|e| unavailable(e.to_string()))?;
		want.SetGUID(&MF_MT_SUBTYPE, &MFVideoFormat_RGB32)
			.map_err(|e| unavailable(e.to_string()))?;
		reader
			.SetCurrentMediaType(stream, None, &want)
			.map_err(|e| unavailable(format!("the camera cannot give RGB frames: {e}")))?;

		let now = reader
			.GetCurrentMediaType(stream)
			.map_err(|e| unavailable(e.to_string()))?;
		let packed = now
			.GetUINT64(&MF_MT_FRAME_SIZE)
			.map_err(|e| unavailable(e.to_string()))?;
		let (width, height) = ((packed >> 32) as u32, (packed & 0xffff_ffff) as u32);
		if width < MIN_SIDE || height < MIN_SIDE || width > 8192 || height > 8192 {
			return Err(SourceError::new(
				"unsupported",
				"the size of the camera is outside the supported range",
			));
		}
		// Without the attribute the stride of RGB32 is the default one, which is negative: bottom-up.
		let bottom_up = now
			.GetUINT32(&MF_MT_DEFAULT_STRIDE)
			.map_or(true, |s| (s as i32) < 0);
		Ok((
			reader,
			Shape {
				width,
				height,
				bottom_up,
			},
		))
	}
}

/// Reads one sample and puts its picture in the mailbox. Err is why the camera ended; Ok(false) is a normal end.
fn read_one(reader: &IMFSourceReader, shape: &Shape, mailbox: &Mailbox) -> Result<bool, String> {
	// SAFETY: every out pointer is to a live local; the sample and its buffer are owned objects; the buffer is
	// locked once and unlocked on every path before it is dropped, and the bytes are copied while it is locked
	// and never kept.
	unsafe {
		let mut flags = 0u32;
		let mut sample = None;
		reader
			.ReadSample(
				MF_SOURCE_READER_FIRST_VIDEO_STREAM.0 as u32,
				READ_FLAGS,
				None,
				Some(&mut flags),
				None,
				Some(&mut sample),
			)
			.map_err(|e| format!("the camera stopped answering: {e}"))?;
		if flags & (MF_SOURCE_READERF_ENDOFSTREAM.0 as u32) != 0 {
			return Ok(false);
		}
		let Some(sample) = sample else {
			// A gap in the stream: no picture this time.
			return Ok(true);
		};
		let buffer = sample
			.ConvertToContiguousBuffer()
			.map_err(|e| e.to_string())?;
		let (mut ptr, mut len) = (std::ptr::null_mut::<u8>(), 0u32);
		buffer
			.Lock(&mut ptr, None, Some(&mut len))
			.map_err(|e| e.to_string())?;
		let copied = if ptr.is_null() {
			None
		} else {
			top_down(
				std::slice::from_raw_parts(ptr, len as usize),
				shape.width,
				shape.height,
				shape.bottom_up,
			)
		};
		let _ = buffer.Unlock();
		if let Some(data) = copied {
			mailbox.push(Frame {
				width: shape.width,
				height: shape.height,
				data,
			});
		}
		Ok(true)
	}
}

struct Running {
	stop: Arc<AtomicBool>,
	done: mpsc::Receiver<()>,
}

impl RunningSource for Running {
	fn stop(self: Box<Self>) {
		self.stop.store(true, Ordering::Relaxed);
		// A camera stuck inside a read leaves its thread behind rather than hanging the helper.
		let _ = self.done.recv_timeout(STOP_WAIT);
	}
}

/// Opens the camera with that id and starts reading it into the mailbox on a thread of its own.
pub fn open(id: &str, mailbox: &Arc<Mailbox>) -> Result<Box<dyn RunningSource>, SourceError> {
	let (id, mailbox) = (id.to_string(), Arc::clone(mailbox));
	let end = Arc::clone(&mailbox);
	let running = run_reader(
		move || {
			Runtime::start()
				.map_err(|e| SourceError::new("device_unavailable", printable(&e)))
				.and_then(|rt| open_reader(&id).map(|r| (r, rt)))
		},
		// The runtime is the last member of the tuple, and a tuple drops its members in order: the reader is
		// released before Media Foundation shuts down.
		move |((reader, shape), _rt)| read_one(reader, shape, &mailbox),
		move |error| end.close(error),
		OPEN_WAIT,
	)?;
	Ok(Box::new(running))
}

/// How long a start waits for the camera to open.
const OPEN_WAIT: Duration = Duration::from_secs(10);

/// The thread of a camera: `open` runs on it (COM and Media Foundation belong to the thread that starts them),
/// then `read` until it says the stream ended, fails, or a stop is asked; `end` gets the reason. A camera that
/// opens only after `wait` has run out is closed at once without one read: the start was already refused.
fn run_reader<T: 'static>(
	open: impl FnOnce() -> Result<T, SourceError> + Send + 'static,
	mut read: impl FnMut(&T) -> Result<bool, String> + Send + 'static,
	end: impl FnOnce(Option<String>) + Send + 'static,
	wait: Duration,
) -> Result<Running, SourceError> {
	let (ready_tx, ready_rx) = mpsc::channel::<Result<(), SourceError>>();
	let (done_tx, done_rx) = mpsc::channel::<()>();
	let stop = Arc::new(AtomicBool::new(false));
	let flag = Arc::clone(&stop);
	std::thread::spawn(move || {
		crate::clock::raise_this_thread();
		let opened = match open() {
			Ok(x) => {
				let _ = ready_tx.send(Ok(()));
				x
			}
			Err(e) => {
				let _ = ready_tx.send(Err(e));
				let _ = done_tx.send(());
				return;
			}
		};
		let mut error = None;
		while !flag.load(Ordering::Relaxed) {
			match read(&opened) {
				Ok(true) => {}
				Ok(false) => break,
				Err(e) => {
					error = Some(printable(&e));
					break;
				}
			}
		}
		drop(opened);
		end(error);
		let _ = done_tx.send(());
	});
	match ready_rx.recv_timeout(wait) {
		Ok(Ok(())) => Ok(Running {
			stop,
			done: done_rx,
		}),
		Ok(Err(e)) => Err(e),
		Err(_) => {
			// The thread may still open the camera after this answer: the flag makes it close the camera at once
			// instead of reading it, with its light on, for a recording that was refused.
			stop.store(true, Ordering::Relaxed);
			Err(SourceError::new(
				"device_unavailable",
				"the camera did not open in time",
			))
		}
	}
}

#[cfg(test)]
mod tests {
	use super::*;

	// 2 by 2 pixels, rows as bytes: the first row is 1s, the second is 2s.
	fn picture() -> Vec<u8> {
		[vec![1u8; 8], vec![2u8; 8]].concat()
	}

	#[test]
	fn a_top_down_picture_is_copied_as_it_is() {
		assert_eq!(top_down(&picture(), 2, 2, false).unwrap(), picture());
	}

	#[test]
	fn a_bottom_up_picture_is_turned_over() {
		let out = top_down(&picture(), 2, 2, true).unwrap();
		assert_eq!(&out[..8], &[2u8; 8]);
		assert_eq!(&out[8..], &[1u8; 8]);
	}

	#[test]
	fn a_short_sample_is_refused_not_read_past_its_end() {
		assert!(top_down(&picture()[..15], 2, 2, false).is_none());
		assert!(top_down(&picture()[..15], 2, 2, true).is_none());
		assert!(top_down(&[], 0, 0, false).is_none());
	}

	#[test]
	fn a_longer_sample_is_cut_to_the_picture() {
		let mut data = picture();
		data.extend_from_slice(&[9; 5]);
		assert_eq!(top_down(&data, 2, 2, false).unwrap(), picture());
	}

	/// Listing only enumerates the devices: no camera is opened, so no light comes on and nothing is recorded.
	#[test]
	fn the_cameras_are_listed_with_opaque_ids_without_opening_one() {
		for c in list() {
			assert!(
				c.id.len() == 17
					&& c.id.starts_with('c')
					&& c.id[1..].bytes().all(|b| b.is_ascii_hexdigit()),
				"{}",
				c.id
			);
		}
	}

	/// A camera that opens after the start gave up on it is closed without one read, so it does not stay in use
	/// (with its light on) behind a refused recording.
	#[test]
	fn a_camera_that_opens_too_late_is_closed_without_a_read() {
		let reads = Arc::new(std::sync::atomic::AtomicU64::new(0));
		let (ended_tx, ended_rx) = mpsc::channel::<Option<String>>();
		let counted = Arc::clone(&reads);
		let got = run_reader(
			|| {
				std::thread::sleep(Duration::from_millis(300));
				Ok(())
			},
			move |_| {
				counted.fetch_add(1, Ordering::Relaxed);
				std::thread::sleep(Duration::from_millis(5));
				Ok(true)
			},
			move |e| {
				let _ = ended_tx.send(e);
			},
			Duration::from_millis(20),
		);
		assert!(got.is_err(), "the start must be refused");
		let ended = ended_rx.recv_timeout(Duration::from_secs(5));
		assert_eq!(ended, Ok(None), "the late camera must be closed");
		assert_eq!(
			reads.load(Ordering::Relaxed),
			0,
			"a refused camera was read"
		);
	}

	/// A camera that opens in time is read until a stop, and the stop returns.
	#[test]
	fn a_camera_is_read_until_it_is_stopped() {
		let reads = Arc::new(std::sync::atomic::AtomicU64::new(0));
		let counted = Arc::clone(&reads);
		let running = run_reader(
			|| Ok(()),
			move |_| {
				counted.fetch_add(1, Ordering::Relaxed);
				std::thread::sleep(Duration::from_millis(2));
				Ok(true)
			},
			|_| {},
			Duration::from_secs(5),
		)
		.expect("the camera opens");
		std::thread::sleep(Duration::from_millis(50));
		Box::new(running).stop();
		let after = reads.load(Ordering::Relaxed);
		assert!(after > 0, "the camera was never read");
		std::thread::sleep(Duration::from_millis(30));
		assert_eq!(
			reads.load(Ordering::Relaxed),
			after,
			"reads went on after the stop"
		);
	}

	/// verify:env-skip needs a camera and STUARTY_TEST_REAL_CAMERA=1: it OPENS the first camera of the machine
	/// for two seconds and checks that frames of a sensible size arrive. It keeps no frame.
	#[test]
	fn the_first_camera_gives_frames_when_asked_to() {
		if std::env::var_os("STUARTY_TEST_REAL_CAMERA").is_none() {
			eprintln!(
				"skipped: STUARTY_TEST_REAL_CAMERA is not set (this test opens a real camera)"
			);
			return;
		}
		let Some(first) = list().into_iter().next() else {
			eprintln!("skipped: no camera on this machine");
			return;
		};
		let mailbox = Mailbox::new();
		let src = open(&first.id, &mailbox).expect("the camera opens");
		std::thread::sleep(Duration::from_secs(2));
		let slot = mailbox.take();
		src.stop();
		let frame = slot.frame.expect("a frame arrived");
		assert!(frame.width >= MIN_SIDE && frame.height >= MIN_SIDE);
		assert_eq!(
			frame.data.len(),
			frame.width as usize * frame.height as usize * 4
		);
		assert!(
			mailbox.arrived() >= 5,
			"{} frames in 2 s",
			mailbox.arrived()
		);
	}
}
