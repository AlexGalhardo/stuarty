//! Sound tracks: the microphone and the system mix (SDD-025 sections 16.1 to 16.3). Each is a WAV file on the
//! clock of the recording. The device is behind `AudioInput`; the real one is WASAPI (audio_wasapi.rs), the test
//! one makes tone bursts.

use std::sync::Arc;
use std::sync::atomic::{AtomicBool, AtomicU32, Ordering};
use std::time::{Duration, Instant};

use crate::clock::Clock;
use crate::protocol::SourceInfo;
use crate::source::SourceError;
use crate::testsrc::{self, Tags};
use crate::wav::WavWriter;

pub const RATE: u32 = 48_000;
/// A packet that starts further than this from the written length is corrected (SDD-025 section 16.2).
const CORRECT_AFTER_FRAMES: i64 = (RATE / 100) as i64;
const PACKET_FRAMES: usize = (RATE / 100) as usize;
const SILENCE_CHUNK_FRAMES: u64 = 4800;

/// One burst of interleaved 16-bit samples and the clock time of its first frame.
pub struct Packet {
	pub wall_ns: i64,
	pub pcm: Vec<u8>,
}

/// A sound device. It is made and used on one thread (a WASAPI client belongs to the thread that opened it), so
/// it is not `Send`.
pub trait AudioInput {
	fn channels(&self) -> u16;
	/// Waits at most `wait` for the next packet; None when there is none yet.
	fn read(&mut self, wait: Duration) -> Result<Option<Packet>, String>;
}

#[derive(Debug, Clone, PartialEq, Eq)]
pub enum Kind {
	/// A capture endpoint by its id, or `default`.
	Microphone(String),
	/// The loopback of the default render endpoint.
	System,
}

/// What of a packet to write: silence first, the packet without its first `skip` frames after it.
#[derive(Debug, PartialEq, Eq)]
pub struct Plan {
	pub silence: u64,
	pub skip: u64,
	pub take: u64,
}

/// Keeps a track as long as the clock says (SDD-025 section 16.2). A packet that starts later than what was
/// written gets silence in front of it (a loopback endpoint that delivers nothing while the computer is silent,
/// or a gap), one that starts earlier loses its overlap. The first packet is placed exactly; later ones are left
/// alone when they are within 10 ms.
pub struct Timeline {
	rate: u32,
	written: u64,
}

impl Timeline {
	pub fn new(rate: u32) -> Self {
		Self { rate, written: 0 }
	}

	pub fn written(&self) -> u64 {
		self.written
	}

	pub fn plan(&mut self, active_start_ns: i64, frames: u64) -> Plan {
		let expected =
			i64::try_from(i128::from(active_start_ns) * i128::from(self.rate) / 1_000_000_000)
				.unwrap_or(0);
		let written = i64::try_from(self.written).unwrap_or(i64::MAX);
		let diff = expected - written;
		let limit = if self.written == 0 {
			0
		} else {
			CORRECT_AFTER_FRAMES
		};
		let plan = if diff > limit {
			Plan {
				silence: diff as u64,
				skip: 0,
				take: frames,
			}
		} else if diff < -limit {
			let skip = (-diff as u64).min(frames);
			Plan {
				silence: 0,
				skip,
				take: frames - skip,
			}
		} else {
			Plan {
				silence: 0,
				skip: 0,
				take: frames,
			}
		};
		self.written += plan.silence + plan.take;
		plan
	}

	/// The silence that brings a track that ended early up to `active_ns`, when it is more than 10 ms short.
	pub fn pad_to(&mut self, active_ns: i64) -> u64 {
		let expected = i64::try_from(i128::from(active_ns) * i128::from(self.rate) / 1_000_000_000)
			.unwrap_or(0);
		let diff = expected - i64::try_from(self.written).unwrap_or(i64::MAX);
		if diff > CORRECT_AFTER_FRAMES {
			self.written += diff as u64;
			diff as u64
		} else {
			0
		}
	}
}

/// The result of a sound track.
#[derive(Debug, Default)]
pub struct Outcome {
	pub bytes: u64,
	pub frames: u64,
	/// The device went away during the recording.
	pub lost: bool,
	pub error: Option<String>,
}

struct Writer {
	wav: WavWriter,
	timeline: Timeline,
	channels: u16,
	peak: Arc<AtomicU32>,
}

impl Writer {
	fn frame_bytes(&self) -> usize {
		usize::from(self.channels) * 2
	}

	fn silence(&mut self, mut frames: u64) -> std::io::Result<()> {
		let zeros = vec![0u8; SILENCE_CHUNK_FRAMES as usize * self.frame_bytes()];
		while frames > 0 {
			let n = frames.min(SILENCE_CHUNK_FRAMES);
			self.wav.write(&zeros[..n as usize * self.frame_bytes()])?;
			frames -= n;
		}
		Ok(())
	}

	fn feed(&mut self, p: &Packet, clock: &Clock) -> std::io::Result<()> {
		let fb = self.frame_bytes();
		let frames = (p.pcm.len() / fb) as u64;
		for seg in clock.segments(p.wall_ns, frames, RATE) {
			let plan = self.timeline.plan(seg.active_start_ns, seg.frames);
			self.silence(plan.silence)?;
			let from = (seg.first_frame + plan.skip) as usize * fb;
			let to = (seg.first_frame + seg.frames) as usize * fb;
			let pcm = &p.pcm[from..to];
			self.wav.write(pcm)?;
			let top = pcm
				.as_chunks::<2>()
				.0
				.iter()
				.map(|b| i32::from(i16::from_le_bytes(*b)).unsigned_abs())
				.max()
				.unwrap_or(0);
			self.peak.fetch_max(top * 1000 / 32768, Ordering::Relaxed);
		}
		Ok(())
	}
}

/// Runs a sound track until `stop` is set: it waits for the clock, then writes what the input gives, and at the
/// end pads the file to the length of the clock. The peak (0 to 1000) of what is written is left in `peak` for
/// the meter, which resets it.
pub fn run(
	mut input: Box<dyn AudioInput>,
	wav: WavWriter,
	clock: &Clock,
	stop: &AtomicBool,
	peak: Arc<AtomicU32>,
) -> Outcome {
	let mut w = Writer {
		wav,
		timeline: Timeline::new(RATE),
		channels: input.channels(),
		peak,
	};
	let mut out = Outcome::default();

	while !clock.started() {
		if stop.load(Ordering::Relaxed) {
			return finish(w, out);
		}
		std::thread::sleep(Duration::from_millis(2));
	}

	while !stop.load(Ordering::Relaxed) {
		match input.read(Duration::from_millis(10)) {
			Ok(Some(p)) => {
				if let Err(e) = w.feed(&p, clock) {
					out.error = Some(format!("the sound file could not be written: {}", e.kind()));
					break;
				}
			}
			Ok(None) => {}
			Err(why) => {
				// The device went away: the track ends here and the recording goes on.
				eprintln!("a sound device ended: {why}");
				out.lost = true;
				break;
			}
		}
	}

	if !out.lost && out.error.is_none() && !clock.paused() {
		let pad = w.timeline.pad_to(clock.active_ns());
		if let Err(e) = w.silence(pad) {
			out.error = Some(format!("the sound file could not be written: {}", e.kind()));
		}
	}
	finish(w, out)
}

fn finish(w: Writer, mut out: Outcome) -> Outcome {
	out.frames = w.timeline.written();
	match w.wav.finish() {
		Ok(b) => out.bytes = b,
		Err(e) => {
			out.error.get_or_insert(format!(
				"the sound file could not be finished: {}",
				e.kind()
			));
		}
	}
	out
}

/// A running sound track: its thread and the peak the meter reads.
pub struct Track {
	handle: std::thread::JoinHandle<Outcome>,
	pub peak: Arc<AtomicU32>,
}

impl Track {
	pub fn join(self) -> Outcome {
		self.handle.join().unwrap_or_else(|_| Outcome {
			error: Some("the sound thread panicked".into()),
			..Outcome::default()
		})
	}
}

/// Opens the device and the file on a thread of their own and returns once they are open, so a device that
/// cannot be opened ends the start before anything is recorded. The thread then waits for the clock.
pub fn spawn(
	kind: Kind,
	path: std::path::PathBuf,
	clock: Arc<Clock>,
	stop: Arc<AtomicBool>,
) -> Result<Track, SourceError> {
	let (tx, rx) = std::sync::mpsc::channel::<Result<(), SourceError>>();
	let peak = Arc::new(AtomicU32::new(0));
	let p2 = Arc::clone(&peak);
	let handle = std::thread::spawn(move || {
		crate::clock::raise_this_thread();
		let input = match open_input(&kind, &clock) {
			Ok(i) => i,
			Err(e) => {
				let _ = tx.send(Err(e));
				return Outcome::default();
			}
		};
		let wav = match WavWriter::create(&path, RATE, input.channels()) {
			Ok(w) => w,
			Err(e) => {
				let _ = tx.send(Err(SourceError::new(
					"io",
					format!("the sound file could not be made: {}", e.kind()),
				)));
				return Outcome::default();
			}
		};
		let _ = tx.send(Ok(()));
		run(input, wav, &clock, &stop, p2)
	});
	match rx.recv_timeout(Duration::from_secs(10)) {
		Ok(Ok(())) => Ok(Track { handle, peak }),
		Ok(Err(e)) => {
			let _ = handle.join();
			Err(e)
		}
		Err(_) => Err(SourceError::new(
			"device_unavailable",
			"the sound device did not open in time",
		)),
	}
}

/// A stable, opaque id for a device: the letter of its kind and the FNV-1a hash of the endpoint id of the system.
pub fn device_id(prefix: char, endpoint: &str) -> String {
	let mut h: u64 = 0xcbf2_9ce4_8422_2325;
	for b in endpoint.bytes() {
		h ^= u64::from(b);
		h = h.wrapping_mul(0x0100_0000_01b3);
	}
	format!("{prefix}{h:016x}")
}

/// Opens the device a request names. A `fake` id is a test source, admitted by the protocol only with the test
/// flag.
pub fn open_input(kind: &Kind, clock: &Arc<Clock>) -> Result<Box<dyn AudioInput>, SourceError> {
	if let Kind::Microphone(id) = kind
		&& let Some(tags) = testsrc::parse_tags(id)
	{
		return Ok(Box::new(FakeInput::new(tags, 1, Arc::clone(clock))));
	}
	if let Kind::Microphone(id) = kind
		&& id.starts_with("fake")
	{
		return Err(SourceError::new(
			"unsupported",
			"the test microphone id is malformed",
		));
	}
	#[cfg(windows)]
	{
		crate::audio_wasapi::open(kind, clock)
	}
	#[cfg(not(windows))]
	{
		let _ = (kind, clock);
		Err(SourceError::new(
			"unsupported",
			"sound capture exists only on Windows in this version",
		))
	}
}

/// What can be recorded: the microphones and whether the system mix exists.
pub fn list(allow_test: bool) -> (Vec<SourceInfo>, bool) {
	let (mut mics, system) = list_real();
	if allow_test {
		mics.push(SourceInfo {
			id: "fake".into(),
			name: "Test microphone".into(),
			width: 0,
			height: 0,
			default: mics.is_empty(),
		});
		return (mics, true);
	}
	(mics, system)
}

fn list_real() -> (Vec<SourceInfo>, bool) {
	#[cfg(windows)]
	{
		crate::audio_wasapi::list()
	}
	#[cfg(not(windows))]
	{
		(Vec::new(), false)
	}
}

pub fn can_capture() -> bool {
	cfg!(windows)
}

/// The test microphone: a tone burst at the pulse of the shared epoch, packets of 10 ms stamped with the
/// instant of their first frame, and a device clock that can run fast or slow.
struct FakeInput {
	tags: Tags,
	channels: u16,
	clock: Arc<Clock>,
	next: Instant,
	step: Duration,
}

impl FakeInput {
	fn new(tags: Tags, channels: u16, clock: Arc<Clock>) -> Self {
		let _ = testsrc::epoch();
		// A device whose clock runs `ppm` faster delivers its 10 ms packets that much sooner.
		let ns = 10_000_000f64 * 1e6 / (1e6 + tags.drift_ppm as f64);
		Self {
			tags,
			channels,
			clock,
			next: Instant::now(),
			step: Duration::from_nanos(ns as u64),
		}
	}
}

impl AudioInput for FakeInput {
	fn channels(&self) -> u16 {
		self.channels
	}

	fn read(&mut self, wait: Duration) -> Result<Option<Packet>, String> {
		let ready = self.next + Duration::from_millis(10);
		let now = Instant::now();
		if now < ready {
			std::thread::sleep((ready - now).min(wait));
			if Instant::now() < ready {
				return Ok(None);
			}
		}
		let stamp = self.next;
		self.next += self.step;
		let t0 = testsrc::ns_since_epoch(stamp);
		let mut pcm = Vec::with_capacity(PACKET_FRAMES * usize::from(self.channels) * 2);
		for f in 0..PACKET_FRAMES as u64 {
			let t = t0 + f * 1_000_000_000 / u64::from(RATE);
			let v: i16 = if !self.tags.silent && testsrc::pulse_on(&self.tags, t) {
				// 1 kHz: a period is 1,000,000 ns.
				let phase = (t % 1_000_000) as f64 / 1_000_000.0;
				((phase * std::f64::consts::TAU).sin() * 16_000.0) as i16
			} else {
				0
			};
			for _ in 0..self.channels {
				pcm.extend_from_slice(&v.to_le_bytes());
			}
		}
		let wall_ns = self.clock.wall_ns_of(stamp).unwrap_or(0);
		Ok(Some(Packet { wall_ns, pcm }))
	}
}

#[cfg(test)]
mod tests {
	use super::*;

	const SEC: i64 = 1_000_000_000;

	#[test]
	fn the_first_packet_is_placed_exactly() {
		let mut t = Timeline::new(48_000);
		// starts 5 ms into the recording: 240 frames of silence first
		assert_eq!(
			t.plan(5_000_000, 480),
			Plan {
				silence: 240,
				skip: 0,
				take: 480
			}
		);
		assert_eq!(t.written(), 720);
		let mut t = Timeline::new(48_000);
		// the packet began 3 ms before the origin: its first 144 frames are not part of the recording
		assert_eq!(
			t.plan(-3_000_000, 480),
			Plan {
				silence: 0,
				skip: 144,
				take: 336
			}
		);
	}

	#[test]
	fn jitter_under_ten_milliseconds_is_left_alone() {
		let mut t = Timeline::new(48_000);
		t.plan(0, 480);
		// the next packet should start at 480 frames (10 ms); it is stamped 6 ms late and 6 ms early
		assert_eq!(t.plan(16_000_000, 480).silence, 0);
		assert_eq!(t.plan(14_000_000, 480).skip, 0);
	}

	#[test]
	fn a_gap_gets_silence_and_an_overlap_loses_frames() {
		let mut t = Timeline::new(48_000);
		t.plan(0, 480);
		// nothing for 90 ms, then a packet stamped at 100 ms: 4,320 frames of silence
		let p = t.plan(100_000_000, 480);
		assert_eq!((p.silence, p.skip, p.take), (4320, 0, 480));
		assert_eq!(t.written(), 5280);
		// a packet stamped 20 ms before the end of what was written loses 960 frames... but only 480 are there
		let p = t.plan(90_000_000, 480);
		assert_eq!((p.silence, p.skip, p.take), (0, 480, 0));
	}

	// A device clock that runs 0.1 % fast delivers one more second of sound in 1,000 seconds. The stamps are
	// on the clock of the recording, so the timeline cuts what is too much: the file keeps the length of the
	// clock to within the 10 ms of the rule, which is under the 40 ms NFR-068 allows.
	#[test]
	fn a_fast_device_clock_stays_within_the_limit_over_ten_minutes() {
		let mut t = Timeline::new(48_000);
		let ppm = 1000i64;
		let mut worst = 0i64;
		// the device makes 480 frames per 10 ms of its own time; its time runs ppm faster than the clock
		let n_packets = 60_000u64;
		for k in 0..n_packets {
			let stamp = (k as i64) * 10_000_000 * 1_000_000 / (1_000_000 + ppm);
			t.plan(stamp, 480);
			let clock_frames = i128::from(stamp) * 48_000 / i128::from(SEC);
			worst = worst.max((t.written() as i128 - clock_frames).abs() as i64);
		}
		let worst_ms = worst * 1000 / 48_000;
		assert!(worst_ms <= 21, "the track drifted {worst_ms} ms");
		let final_ms = (t.written() as i64) * 1000 / 48_000;
		let clock_ms =
			((n_packets as i64 - 1) * 10_000_000 * 1_000_000 / (1_000_000 + ppm)) / 1_000_000;
		assert!(
			(final_ms - clock_ms).abs() <= 40,
			"{final_ms} against {clock_ms}"
		);
	}

	#[test]
	fn a_track_that_ended_early_is_padded_to_the_clock() {
		let mut t = Timeline::new(48_000);
		t.plan(0, 480);
		assert_eq!(t.pad_to(5_000_000), 0, "5 ms short is left alone");
		assert_eq!(t.pad_to(1_000_000_000), 48_000 - 480);
		assert_eq!(t.written(), 48_000);
	}

	#[test]
	fn a_device_id_is_opaque_stable_and_a_valid_source_id() {
		let id = device_id('m', "{0.0.1.00000000}.{abc-123}");
		assert_eq!(id, device_id('m', "{0.0.1.00000000}.{abc-123}"));
		assert_ne!(id, device_id('m', "{0.0.1.00000000}.{abc-124}"));
		assert_eq!(id.len(), 17);
		assert!(id.starts_with('m') && id[1..].bytes().all(|b| b.is_ascii_hexdigit()));
		assert_ne!(device_id('c', "x"), device_id('m', "x"));
	}

	fn tmp(name: &str) -> std::path::PathBuf {
		std::env::temp_dir().join(format!("stuarty-audio-{name}-{}.wav", std::process::id()))
	}

	fn samples(path: &std::path::Path) -> Vec<i16> {
		std::fs::read(path).unwrap()[44..]
			.as_chunks::<2>()
			.0
			.iter()
			.map(|b| i16::from_le_bytes(*b))
			.collect()
	}

	// The test microphone makes a 1 kHz burst for the first 100 ms of every second of the shared epoch. The file
	// of a short recording must have the burst, and its length must be that of the recording.
	#[test]
	fn a_known_tone_is_found_in_the_file() {
		let clock = Arc::new(Clock::new());
		let path = tmp("tone");
		let stop = Arc::new(AtomicBool::new(false));
		let t = spawn(
			Kind::Microphone("fake".into()),
			path.clone(),
			Arc::clone(&clock),
			Arc::clone(&stop),
		)
		.unwrap();
		let peak = Arc::clone(&t.peak);
		clock.start(0);
		std::thread::sleep(Duration::from_millis(2300));
		stop.store(true, Ordering::Relaxed);
		let out = t.join();
		assert!(!out.lost && out.error.is_none(), "{out:?}");

		let s = samples(&path);
		let ms = s.len() as i64 * 1000 / i64::from(RATE);
		assert!(
			(2200..=2500).contains(&ms),
			"a file of {ms} ms for a 2.3 s recording"
		);
		assert_eq!(out.frames, s.len() as u64);
		// the tone: a run of loud samples, 100 ms long, followed by silence, at the pulse of the epoch
		let loud: Vec<usize> = s
			.iter()
			.enumerate()
			.filter(|(_, v)| v.unsigned_abs() > 8000)
			.map(|(i, _)| i)
			.collect();
		assert!(loud.len() > 1500, "{} loud samples", loud.len());
		assert!(
			peak.load(Ordering::Relaxed) >= 400,
			"the meter saw the tone"
		);
		// each burst is 100 ms wide at most
		let mut bursts = 0;
		let mut last = None::<usize>;
		for &i in &loud {
			if last.is_none_or(|l| i - l > 2400) {
				bursts += 1;
			}
			last = Some(i);
		}
		assert!((2..=3).contains(&bursts), "{bursts} bursts in 2.3 s");
		std::fs::remove_file(path).unwrap();
	}
}
