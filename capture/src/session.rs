//! One recording: the screen source and its grid, the sound tracks, the clock they share, and the messages about
//! them (SDD-025 sections 6, 8 and 16).

use std::sync::Arc;
use std::sync::atomic::{AtomicBool, AtomicU32, Ordering};
use std::time::{Duration, Instant};

use crate::audio::{self, Kind, RATE};
use crate::camera;
use crate::clock::Clock;
use crate::events;
use crate::frame::Mailbox;
use crate::protocol::{self, Finish, StartRequest, TrackEnd, TrackStart};
use crate::sink::{FfmpegSink, FileSink, Sink};
use crate::source;
use crate::video::{Grid, Step};

const FIRST_FRAME_WAIT: Duration = Duration::from_secs(5);
const LEVEL_EVERY: Duration = Duration::from_millis(250);
const PROGRESS_EVERY: Duration = Duration::from_secs(1);

/// What the main loop of the helper holds to steer a recording that runs on another thread: the stop flag and
/// the clock, which also holds the pauses.
pub struct Control {
	pub stop: Arc<AtomicBool>,
	pub clock: Arc<Clock>,
}

impl Control {
	pub fn new() -> Arc<Self> {
		Arc::new(Self {
			stop: Arc::new(AtomicBool::new(false)),
			clock: Arc::new(Clock::new()),
		})
	}
}

#[cfg(windows)]
fn qpc() -> u64 {
	crate::win32::qpc_100ns()
}

#[cfg(not(windows))]
fn qpc() -> u64 {
	0
}

/// A sound track that is running.
struct Sound {
	kind: &'static str,
	track: audio::Track,
}

/// The tracks beside the screen that are running.
#[derive(Default)]
struct Extra {
	sounds: Vec<Sound>,
	camera: Option<camera::Track>,
	pointer: Option<events::Track>,
}

impl Extra {
	/// Ends every track at once, which makes each finish its file, and reports how each ended. The stop flag must
	/// be set by the caller. It returns the number of pointer events written.
	fn join(self, ends: &mut Vec<TrackEnd>) -> u64 {
		let mut pointer_events = 0;
		if let Some(p) = self.pointer {
			let o = p.join();
			if let Some(e) = &o.error {
				eprintln!("the pointer track: {e}");
			}
			pointer_events = o.events;
		}
		for s in self.sounds {
			let o = s.track.join();
			if let Some(e) = &o.error {
				eprintln!("the {} track: {e}", s.kind);
			}
			ends.push(TrackEnd {
				kind: s.kind,
				start_ms: 0,
				duration_ms: o.frames * 1000 / u64::from(RATE),
				bytes: o.bytes,
				lost: o.lost || o.error.is_some(),
			});
		}
		if let Some(c) = self.camera {
			let o = c.join();
			if let Some(e) = &o.error {
				eprintln!("the camera track: {e}");
			}
			ends.push(TrackEnd {
				kind: "camera",
				start_ms: 0,
				duration_ms: o.duration_ms,
				bytes: o.bytes,
				lost: o.lost,
			});
		}
		pointer_events
	}
}

/// Ends the tracks and the screen source of a start that did not get to run.
fn abandon(src: Box<dyn source::RunningSource>, extra: Extra, stop: &AtomicBool) {
	stop.store(true, Ordering::Relaxed);
	let _ = extra.join(&mut Vec::new());
	src.stop();
}

/// Runs a recording to its end and reports every step through emit. It returns when the files are complete or
/// the recording failed; `ctl.stop` asks for a graceful end.
pub fn run(req: &StartRequest, emit: &dyn Fn(String), ctl: &Control) {
	#[cfg(windows)]
	crate::win32::timer_1ms();
	let (stop, clock) = (&ctl.stop, &ctl.clock);
	let mailbox = Mailbox::new();
	let src = match source::open(req, &mailbox) {
		Ok(s) => s,
		Err(e) => {
			emit(protocol::error(e.code, &e.message));
			return;
		}
	};

	// Every device is opened before the origin of the clock, so every track can start at it.
	let mut extra = Extra::default();
	let asked = [
		(
			"microphone",
			req.extras.microphone.clone().map(Kind::Microphone),
			req.extras.out_microphone.clone(),
		),
		(
			"system",
			req.extras.system_audio.then_some(Kind::System),
			req.extras.out_system.clone(),
		),
	];
	for (name, kind, path) in asked {
		let (Some(kind), Some(path)) = (kind, path) else {
			continue;
		};
		match audio::spawn(kind, path, Arc::clone(clock), Arc::clone(stop)) {
			Ok(track) => extra.sounds.push(Sound { kind: name, track }),
			Err(e) => {
				abandon(src, extra, stop);
				emit(protocol::error(e.code, &e.message));
				return;
			}
		}
	}
	if let (Some(id), Some(path)) = (&req.extras.camera, &req.extras.out_camera) {
		match camera::spawn(req, id, path, Arc::clone(clock), Arc::clone(stop)) {
			Ok(track) => extra.camera = Some(track),
			Err(e) => {
				abandon(src, extra, stop);
				emit(protocol::error(e.code, &e.message));
				return;
			}
		}
	}

	let first = mailbox.wait_first(FIRST_FRAME_WAIT);
	let Some(first_frame) = first.frame else {
		abandon(src, extra, stop);
		let why = first
			.error
			.unwrap_or_else(|| "no frame arrived from the source".into());
		emit(protocol::error("source_gone", &why));
		return;
	};

	let (w, h) = (first_frame.width & !1, first_frame.height & !1);
	if w < 16 || h < 16 || w > 8192 || h > 8192 || u64::from(w) * u64::from(h) > 33_177_600 {
		abandon(src, extra, stop);
		emit(protocol::error(
			"unsupported",
			"the size of the source is outside the supported range",
		));
		return;
	}

	// The pointer file has the size of the screen file in its header, so it starts here, still before the origin.
	if let Some(path) = &req.extras.out_events {
		let track = events::open_pointer(req, w, h).and_then(|p| {
			events::spawn(p, path.clone(), w, h, Arc::clone(clock), Arc::clone(stop))
		});
		match track {
			Ok(t) => extra.pointer = Some(t),
			Err(e) => {
				abandon(src, extra, stop);
				emit(protocol::error(e.code, &e.message));
				return;
			}
		}
	}

	let sink: Result<Box<dyn Sink>, String> = if req.encoder == "raw" {
		FileSink::create(&req.output).map(|s| Box::new(s) as Box<dyn Sink>)
	} else {
		FfmpegSink::spawn(req, w, h).map(|s| Box::new(s) as Box<dyn Sink>)
	};
	let mut sink = match sink {
		Ok(s) => s,
		Err(m) => {
			abandon(src, extra, stop);
			emit(protocol::error("encoder_failed", &m));
			return;
		}
	};

	// Warm the encoder: the first write blocks while it starts (150 to 600 ms measured on NVENC), so the grid
	// starts after it, and that frame is slot 0.
	if let Err(e) = sink.write_frame(&first_frame.fit(w, h).data) {
		abandon(src, extra, stop);
		emit(protocol::error(
			"encoder_failed",
			&finish_message(sink, &e.to_string()),
		));
		return;
	}
	mailbox.take();

	// The origin: from here every track is written on the one clock.
	clock.start(qpc());
	let mut tracks = vec![TrackStart {
		kind: "screen",
		start_ms: 0,
	}];
	tracks.extend(extra.sounds.iter().map(|s| TrackStart {
		kind: s.kind,
		start_ms: 0,
	}));
	if extra.camera.is_some() {
		tracks.push(TrackStart {
			kind: "camera",
			start_ms: 0,
		});
	}
	emit(protocol::started(w, h, req.fps, &tracks));

	let mut grid = Grid::new(Arc::clone(&mailbox), sink, w, h, req.fps);
	let mut reason = "stopped";
	let mut source_error: Option<String> = None;
	let (mut last_level, mut last_progress) = (Instant::now(), Instant::now());
	let peaks: Vec<(&'static str, Arc<AtomicU32>)> = extra
		.sounds
		.iter()
		.map(|s| (s.kind, Arc::clone(&s.track.peak)))
		.collect();

	// Signs of life: a level four times a second, a progress once a second, paused or not.
	let mut maintain = |frames: u64, dropped: u64| {
		if last_level.elapsed() >= LEVEL_EVERY && !peaks.is_empty() {
			last_level = Instant::now();
			let peak = |name: &str| {
				peaks
					.iter()
					.find(|(k, _)| *k == name)
					.map(|(_, p)| p.swap(0, Ordering::Relaxed))
			};
			emit(protocol::level(peak("microphone"), peak("system")));
		}
		if last_progress.elapsed() >= PROGRESS_EVERY {
			last_progress = Instant::now();
			emit(protocol::progress(
				u64::try_from(clock.active_ns() / 1_000_000).unwrap_or(0),
				frames,
				dropped,
				total_bytes(req),
			));
		}
	};

	while !stop.load(Ordering::Relaxed) {
		maintain(grid.frames, grid.dropped);
		match grid.step(clock) {
			Step::Wait(d) => std::thread::sleep(d),
			Step::Wrote => {}
			Step::Closed(why) => {
				reason = "source_closed";
				source_error = why;
				break;
			}
			Step::Failed(e) => {
				src.stop();
				stop.store(true, Ordering::Relaxed);
				let _ = extra.join(&mut Vec::new());
				emit(protocol::error(
					"encoder_failed",
					&finish_message(grid.sink, &e.to_string()),
				));
				return;
			}
		}
	}

	src.stop();
	// The other tracks end at the same moment as the screen.
	stop.store(true, Ordering::Relaxed);
	let (frames, dropped, duration_ms) = (grid.frames, grid.dropped, grid.duration_ms());
	let mut ends = vec![TrackEnd {
		kind: "screen",
		start_ms: 0,
		duration_ms,
		bytes: 0,
		lost: false,
	}];
	let pointer_events = extra.join(&mut ends);

	if let Err(m) = grid.sink.finish() {
		emit(protocol::error("encoder_failed", &m));
		return;
	}
	if let Some(why) = source_error {
		emit(protocol::error("source_gone", &why));
		return;
	}
	ends[0].bytes = std::fs::metadata(&req.output).map_or(0, |m| m.len());
	let pauses = clock.pauses_ms();
	emit(protocol::finished(&Finish {
		reason,
		frames,
		dropped,
		duration_ms,
		bytes: ends.iter().map(|t| t.bytes).sum(),
		tracks: &ends,
		pauses: &pauses,
		pointer_events,
	}));
}

/// Finishes a sink after a write failed, to learn what the encoder said; the write error is the fallback.
fn finish_message(sink: Box<dyn Sink>, fallback: &str) -> String {
	match sink.finish() {
		Err(m) => m,
		Ok(()) => fallback.to_string(),
	}
}

/// The size of every file of the recording so far.
fn total_bytes(req: &StartRequest) -> u64 {
	let x = &req.extras;
	[
		Some(&req.output),
		x.out_microphone.as_ref(),
		x.out_system.as_ref(),
		x.out_camera.as_ref(),
		x.out_events.as_ref(),
	]
	.into_iter()
	.flatten()
	.map(|p| std::fs::metadata(p).map_or(0, |m| m.len()))
	.sum()
}

#[cfg(test)]
mod tests {
	use super::*;
	use crate::json;
	use std::path::PathBuf;
	use std::sync::Mutex;

	fn request(dir: &std::path::Path, id: &str, fps: u32) -> StartRequest {
		StartRequest {
			recording: "rec_test".into(),
			kind: "fake".into(),
			source_id: id.into(),
			fps,
			encoder: "raw".into(),
			ffmpeg: PathBuf::from("unused"),
			output: dir.join("out.raw"),
			extras: Default::default(),
		}
	}

	fn collect(req: &StartRequest, ctl: &Control) -> Vec<json::Json> {
		let events = Mutex::new(Vec::new());
		run(
			req,
			&|line| events.lock().unwrap().push(json::parse(&line).unwrap()),
			ctl,
		);
		events.into_inner().unwrap()
	}

	fn kinds(events: &[json::Json]) -> Vec<String> {
		events
			.iter()
			.map(|e| e.get("type").unwrap().as_str().unwrap().to_string())
			.collect()
	}

	fn tmp(name: &str) -> PathBuf {
		let d = std::env::temp_dir().join(format!("stuarty-capture-{name}-{}", std::process::id()));
		std::fs::create_dir_all(&d).unwrap();
		d
	}

	#[test]
	fn a_source_that_ends_gives_a_finished_file_with_every_slot() {
		let dir = tmp("ends");
		let req = request(&dir, "32x32r30n45", 30);
		let events = collect(&req, &Control::new());
		assert_eq!(kinds(&events)[0], "started");
		let last = events.last().unwrap();
		assert_eq!(
			last.get("type").unwrap().as_str(),
			Some("finished"),
			"{events:?}"
		);
		assert_eq!(last.get("reason").unwrap().as_str(), Some("source_closed"));
		let frames = last.get("frames").unwrap().as_u64().unwrap();
		let bytes = std::fs::metadata(&req.output).unwrap().len();
		assert_eq!(
			bytes,
			frames * 32 * 32 * 4,
			"the file has exactly the frames reported"
		);
		// A grid that is starved on a loaded machine sees the close of the source before it has taken every
		// slot, so the count has a lower bound that is loose on purpose; the exact one is the file size above.
		assert!((30..=50).contains(&frames), "about 45 frames, got {frames}");
		assert_eq!(
			last.get("duration_ms").unwrap().as_u64().unwrap(),
			frames * 1000 / 30
		);
		std::fs::remove_dir_all(dir).unwrap();
	}

	#[test]
	fn a_slow_source_is_repeated_and_counted_as_dropped() {
		let dir = tmp("slow");
		// the source makes 10 frames a second and the grid wants 30: two of three slots repeat
		let req = request(&dir, "16x16r10n20", 30);
		let events = collect(&req, &Control::new());
		let last = events.last().unwrap();
		let (frames, dropped) = (
			last.get("frames").unwrap().as_u64().unwrap(),
			last.get("dropped").unwrap().as_u64().unwrap(),
		);
		assert!(
			frames >= 50,
			"about 60 slots over two seconds, got {frames}"
		);
		assert!(
			dropped * 100 / frames >= 50,
			"dropped {dropped} of {frames}"
		);
		std::fs::remove_dir_all(dir).unwrap();
	}

	fn stop_after(ctl: &Arc<Control>, ms: u64) -> std::thread::JoinHandle<()> {
		let c = Arc::clone(ctl);
		std::thread::spawn(move || {
			std::thread::sleep(Duration::from_millis(ms));
			c.stop.store(true, Ordering::Relaxed);
		})
	}

	#[test]
	fn stop_ends_a_running_recording_gracefully_with_progress() {
		let dir = tmp("stop");
		let req = request(&dir, "16x16", 30);
		let ctl = Control::new();
		let t = stop_after(&ctl, 1500);
		let events = collect(&req, &ctl);
		t.join().unwrap();
		let k = kinds(&events);
		assert_eq!(k[0], "started");
		assert!(k.contains(&"progress".to_string()), "{k:?}");
		let last = events.last().unwrap();
		assert_eq!(last.get("reason").unwrap().as_str(), Some("stopped"));
		assert!(last.get("frames").unwrap().as_u64().unwrap() >= 30);
		std::fs::remove_dir_all(dir).unwrap();
	}

	#[test]
	fn a_malformed_source_is_an_error_not_a_panic() {
		let dir = tmp("bad");
		let events = collect(&request(&dir, "nonsense", 30), &Control::new());
		assert_eq!(kinds(&events), ["error"]);
		assert_eq!(events[0].get("code").unwrap().as_str(), Some("source_gone"));
		std::fs::remove_dir_all(dir).unwrap();
	}

	#[test]
	fn an_unwritable_output_is_an_encoder_error() {
		let dir = tmp("nowrite");
		let mut req = request(&dir, "16x16", 30);
		req.output = dir.join("missing-folder").join("out.raw");
		let events = collect(&req, &Control::new());
		assert_eq!(
			events.last().unwrap().get("code").unwrap().as_str(),
			Some("encoder_failed")
		);
		std::fs::remove_dir_all(dir).unwrap();
	}

	#[test]
	fn a_frame_of_another_size_is_fitted_not_rejected() {
		let dir = tmp("fit");
		let events = collect(&request(&dir, "33x33r60n10", 30), &Control::new());
		let started = &events[0];
		assert_eq!(
			started.get("width").unwrap().as_u64(),
			Some(32),
			"odd sizes are cut to even"
		);
		let _ = std::fs::remove_dir_all(dir);
	}

	#[test]
	fn a_device_that_cannot_be_opened_ends_the_start_with_its_code() {
		let dir = tmp("nodevice");
		let mut req = request(&dir, "32x32r60", 30);
		req.extras.microphone = Some("fake-zzz".into());
		req.extras.out_microphone = Some(dir.join("mic.wav"));
		let events = collect(&req, &Control::new());
		assert_eq!(kinds(&events), ["error"], "{events:?}");
		assert_eq!(events[0].get("code").unwrap().as_str(), Some("unsupported"));
		std::fs::remove_dir_all(dir).unwrap();
	}

	#[test]
	fn the_microphone_track_has_the_length_of_the_screen_and_the_tone() {
		let dir = tmp("mic");
		let mut req = request(&dir, "32x32r60", 30);
		req.extras.microphone = Some("fake".into());
		req.extras.out_microphone = Some(dir.join("mic.wav"));
		let ctl = Control::new();
		let t = stop_after(&ctl, 2300);
		let events = collect(&req, &ctl);
		t.join().unwrap();
		let k = kinds(&events);
		assert_eq!(k[0], "started");
		assert!(k.contains(&"level".to_string()), "a level is sent: {k:?}");
		let tracks = match events[0].get("tracks") {
			Some(json::Json::Arr(a)) => a.len(),
			other => panic!("{other:?}"),
		};
		assert_eq!(tracks, 2);
		let last = events.last().unwrap();
		let Some(json::Json::Arr(ends)) = last.get("tracks") else {
			panic!("{last:?}");
		};
		let dur = |kind: &str| {
			ends.iter()
				.find(|t| t.get("kind").and_then(json::Json::as_str) == Some(kind))
				.and_then(|t| t.get("duration_ms"))
				.and_then(json::Json::as_u64)
				.unwrap()
		};
		let (screen, mic) = (dur("screen"), dur("microphone"));
		assert!(
			screen.abs_diff(mic) <= 40,
			"screen {screen} ms, microphone {mic} ms"
		);
		let wav = std::fs::read(dir.join("mic.wav")).unwrap();
		let data = wav.len() as u64 - 44;
		assert!(
			(mic * 96..mic * 96 + 96).contains(&data),
			"{data} bytes for {mic} ms"
		);
		std::fs::remove_dir_all(dir).unwrap();
	}

	#[test]
	fn the_camera_is_a_track_of_its_own_with_the_length_of_the_screen() {
		let dir = tmp("camera");
		let mut req = request(&dir, "32x32r60", 30);
		req.extras.camera = Some("fake".into());
		req.extras.out_camera = Some(dir.join("camera.raw"));
		let ctl = Control::new();
		let t = stop_after(&ctl, 2000);
		let events = collect(&req, &ctl);
		t.join().unwrap();
		assert_eq!(kinds(&events)[0], "started");
		let Some(json::Json::Arr(started)) = events[0].get("tracks") else {
			panic!("{events:?}");
		};
		let names: Vec<&str> = started
			.iter()
			.filter_map(|t| t.get("kind").and_then(json::Json::as_str))
			.collect();
		assert_eq!(names, ["screen", "camera"]);
		let last = events.last().unwrap();
		let Some(json::Json::Arr(ends)) = last.get("tracks") else {
			panic!("{last:?}");
		};
		let dur = |kind: &str| {
			ends.iter()
				.find(|t| t.get("kind").and_then(json::Json::as_str) == Some(kind))
				.and_then(|t| t.get("duration_ms"))
				.and_then(json::Json::as_u64)
				.unwrap()
		};
		let (screen, cam) = (dur("screen"), dur("camera"));
		assert!(
			screen.abs_diff(cam) <= 40,
			"screen {screen} ms, camera {cam} ms"
		);
		let bytes = std::fs::metadata(dir.join("camera.raw")).unwrap().len();
		assert_eq!(bytes % (160 * 90 * 4), 0, "whole frames of 160 by 90");
		assert_eq!(bytes / (160 * 90 * 4), cam * 30 / 1000);
		std::fs::remove_dir_all(dir).unwrap();
	}

	#[test]
	fn a_camera_with_no_backend_ends_the_start_before_anything_runs() {
		let dir = tmp("nocamera");
		let mut req = request(&dir, "32x32r60", 30);
		req.extras.camera = Some("c0123456789abcdef".into());
		req.extras.out_camera = Some(dir.join("camera.raw"));
		let events = collect(&req, &Control::new());
		assert_eq!(kinds(&events), ["error"], "{events:?}");
		assert_eq!(events[0].get("code").unwrap().as_str(), Some("unsupported"));
		assert!(!dir.join("camera.raw").exists());
		std::fs::remove_dir_all(dir).unwrap();
	}
}
