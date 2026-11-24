//! The sound devices of Windows through WASAPI in shared mode, with the `wasapi` crate ADR-0049 approves: the
//! microphone is a capture endpoint, the system mix is the loopback of the default render endpoint.

use std::sync::Arc;
use std::time::{Duration, Instant};

use wasapi::{
	AudioCaptureClient, AudioClient, Device, DeviceEnumerator, Direction, SampleType, StreamMode,
	WaveFormat, initialize_mta,
};

use crate::audio::{AudioInput, Kind, Packet, RATE, device_id};
use crate::clock::Clock;
use crate::protocol::{SourceInfo, printable};
use crate::source::SourceError;

/// How much sound the engine may hold for us between two reads.
const BUFFER_HNS: i64 = 1_000_000;

struct Input {
	// The client is kept alive for the stream; the capture client reads from it.
	client: AudioClient,
	capture: AudioCaptureClient,
	channels: u16,
	block: usize,
	clock: Arc<Clock>,
	buf: Vec<u8>,
}

impl Drop for Input {
	fn drop(&mut self) {
		let _ = self.client.stop_stream();
	}
}

fn unavailable(what: &str, e: impl std::fmt::Display) -> SourceError {
	SourceError::new(
		"device_unavailable",
		format!("{what}: {}", printable(&e.to_string())),
	)
}

fn find(enumerator: &DeviceEnumerator, id: &str) -> Result<Device, SourceError> {
	if id == "default" {
		return enumerator
			.get_default_device(Direction::Capture)
			.map_err(|e| unavailable("no default microphone", e));
	}
	let all = enumerator
		.get_device_collection(Direction::Capture)
		.map_err(|e| unavailable("microphones", e))?;
	for d in &all {
		let d = d.map_err(|e| unavailable("microphones", e))?;
		if d.get_id().is_ok_and(|e| device_id('m', &e) == id) {
			return Ok(d);
		}
	}
	// Only an endpoint the helper itself lists can be opened, never an arbitrary id.
	Err(SourceError::new(
		"device_unavailable",
		"the microphone is not there",
	))
}

fn start(
	device: &Device,
	loopback: bool,
	channels: usize,
	clock: &Arc<Clock>,
) -> Result<Input, String> {
	let mut client = device.get_iaudioclient().map_err(|e| e.to_string())?;
	let format = WaveFormat::new(16, 16, SampleType::Int, RATE as usize, channels, None);
	let mode = StreamMode::PollingShared {
		autoconvert: true,
		buffer_duration_hns: BUFFER_HNS,
	};
	// Asking a render endpoint for a capture stream is the loopback of the system mix.
	let direction = if loopback {
		Direction::Capture
	} else {
		device.get_direction()
	};
	client
		.initialize_client(&format, direction, mode, None)
		.map_err(|e| e.to_string())?;
	let capture = client.get_audiocaptureclient().map_err(|e| e.to_string())?;
	client.start_stream().map_err(|e| e.to_string())?;
	Ok(Input {
		client,
		capture,
		channels: channels as u16,
		block: channels * 2,
		clock: Arc::clone(clock),
		buf: Vec::new(),
	})
}

/// Opens the device on the calling thread, which must keep it: the thread is made a COM multithreaded apartment
/// thread here.
pub fn open(kind: &Kind, clock: &Arc<Clock>) -> Result<Box<dyn AudioInput>, SourceError> {
	let _ = initialize_mta();
	let enumerator = DeviceEnumerator::new().map_err(|e| unavailable("audio", e))?;
	let (device, loopback, wanted) = match kind {
		Kind::Microphone(id) => (find(&enumerator, id)?, false, [1usize, 2]),
		Kind::System => (
			enumerator
				.get_default_device(Direction::Render)
				.map_err(|e| unavailable("no sound output", e))?,
			true,
			[2usize, 1],
		),
	};
	let mut last = String::new();
	for channels in wanted {
		match start(&device, loopback, channels, clock) {
			Ok(i) => return Ok(Box::new(i)),
			Err(e) => last = e,
		}
	}
	Err(unavailable("the device could not be opened", last))
}

impl AudioInput for Input {
	fn channels(&self) -> u16 {
		self.channels
	}

	fn read(&mut self, wait: Duration) -> Result<Option<Packet>, String> {
		let deadline = Instant::now() + wait;
		loop {
			let frames = self
				.capture
				.get_next_packet_size()
				.map_err(|e| e.to_string())?
				.unwrap_or(0) as usize;
			if frames > 0 {
				self.buf.resize(frames * self.block, 0);
				let (n, info) = self
					.capture
					.read_from_device(&mut self.buf)
					.map_err(|e| e.to_string())?;
				let n = n as usize;
				if n == 0 {
					continue;
				}
				let wall_ns = if info.flags.timestamp_error || info.timestamp == 0 {
					// The device gave no usable stamp: the packet ended now.
					self.clock.wall_ns() - (n as i64) * 1_000_000_000 / i64::from(RATE)
				} else {
					self.clock.qpc_to_wall_ns(info.timestamp)
				};
				return Ok(Some(Packet {
					wall_ns,
					pcm: self.buf[..n * self.block].to_vec(),
				}));
			}
			let left = deadline.saturating_duration_since(Instant::now());
			if left.is_zero() {
				return Ok(None);
			}
			std::thread::sleep(left.min(Duration::from_millis(4)));
		}
	}
}

/// The microphones and whether there is a sound output to take the mix of.
pub fn list() -> (Vec<SourceInfo>, bool) {
	let _ = initialize_mta();
	let Ok(enumerator) = DeviceEnumerator::new() else {
		return (Vec::new(), false);
	};
	let default = enumerator
		.get_default_device(Direction::Capture)
		.ok()
		.and_then(|d| d.get_id().ok());
	let mut mics = Vec::new();
	if let Ok(all) = enumerator.get_device_collection(Direction::Capture) {
		for d in &all {
			let Ok(d) = d else { continue };
			let (Ok(id), Ok(name)) = (d.get_id(), d.get_friendlyname()) else {
				continue;
			};
			mics.push(SourceInfo {
				id: device_id('m', &id),
				name: name.chars().take(200).collect(),
				width: 0,
				height: 0,
				default: default.as_deref() == Some(id.as_str()),
			});
		}
	}
	let system = enumerator.get_default_device(Direction::Render).is_ok();
	(mics, system)
}

#[cfg(test)]
mod tests {
	use super::*;
	use std::sync::atomic::{AtomicBool, Ordering};

	#[test]
	fn the_devices_have_opaque_ids() {
		// Reads the list of devices only: nothing is opened and nothing is recorded.
		let (mics, _system) = list();
		for m in &mics {
			assert!(
				m.id.len() == 17
					&& m.id.starts_with('m')
					&& m.id[1..].bytes().all(|b| b.is_ascii_hexdigit()),
				"{}",
				m.id
			);
		}
		assert!(mics.iter().filter(|m| m.default).count() <= 1);
	}

	/// verify:env-skip needs a sound output and STUARTY_TEST_REAL_AUDIO=1. It opens the loopback of the system
	/// mix for a second and a half and checks the file has the length of the clock, whether the computer plays
	/// anything or not (an idle endpoint delivers no packet, and the timeline fills it with silence). It reads
	/// no sample: the file is removed. The microphone is never opened by a test.
	#[test]
	fn the_system_mix_is_recorded_with_the_length_of_the_clock() {
		if std::env::var_os("STUARTY_TEST_REAL_AUDIO").is_none() {
			eprintln!("skipped: STUARTY_TEST_REAL_AUDIO is not set");
			return;
		}
		let clock = Arc::new(Clock::new());
		let stop = Arc::new(AtomicBool::new(false));
		let path =
			std::env::temp_dir().join(format!("stuarty-loopback-{}.wav", std::process::id()));
		let track = crate::audio::spawn(
			Kind::System,
			path.clone(),
			Arc::clone(&clock),
			Arc::clone(&stop),
		)
		.expect("the loopback of the default output opens");
		clock.start(crate::win32::qpc_100ns());
		std::thread::sleep(Duration::from_millis(1500));
		stop.store(true, Ordering::Relaxed);
		let out = track.join();
		let ms = out.frames * 1000 / u64::from(RATE);
		let _ = std::fs::remove_file(&path);
		assert!(!out.lost && out.error.is_none(), "{out:?}");
		assert!((1400..=1700).contains(&ms), "{ms} ms for a 1.5 s recording");
	}
}
