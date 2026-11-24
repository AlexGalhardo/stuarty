//! One recording: a source, the pacing grid, a sink, and the messages about them (SDD-025 sections 6 and 8).

use std::sync::Arc;
use std::sync::atomic::{AtomicBool, Ordering};
use std::time::{Duration, Instant};

use crate::frame::Mailbox;
use crate::protocol::{self, StartRequest};
use crate::sink::{FfmpegSink, FileSink, Sink};
use crate::source;

const FIRST_FRAME_WAIT: Duration = Duration::from_secs(5);

/// Runs a recording to its end and reports every step through emit. It returns when the file is complete or
/// the recording failed; stop asks for a graceful end.
pub fn run(req: &StartRequest, emit: &dyn Fn(String), stop: &AtomicBool) {
	let mailbox = Mailbox::new();
	let src = match source::open(req, &mailbox) {
		Ok(s) => s,
		Err(e) => {
			emit(protocol::error(e.code, &e.message));
			return;
		}
	};

	let first = mailbox.wait_first(FIRST_FRAME_WAIT);
	let Some(first_frame) = first.frame else {
		src.stop();
		let why = first
			.error
			.unwrap_or_else(|| "no frame arrived from the source".into());
		emit(protocol::error("source_gone", &why));
		return;
	};

	let (w, h) = (first_frame.width & !1, first_frame.height & !1);
	if w < 16 || h < 16 || w > 8192 || h > 8192 || u64::from(w) * u64::from(h) > 33_177_600 {
		src.stop();
		emit(protocol::error(
			"unsupported",
			"the size of the source is outside the supported range",
		));
		return;
	}

	let sink: Result<Box<dyn Sink>, String> = if req.encoder == "raw" {
		FileSink::create(req).map(|s| Box::new(s) as Box<dyn Sink>)
	} else {
		FfmpegSink::spawn(req, w, h).map(|s| Box::new(s) as Box<dyn Sink>)
	};
	let mut sink = match sink {
		Ok(s) => s,
		Err(m) => {
			src.stop();
			emit(protocol::error("encoder_failed", &m));
			return;
		}
	};

	// Warm the encoder: the first write blocks while it starts (150 to 600 ms measured on NVENC), so the grid
	// starts after it, and that frame is slot 0.
	if let Err(e) = sink.write_frame(&first_frame.fit(w, h).data) {
		src.stop();
		emit(protocol::error(
			"encoder_failed",
			&finish_message(sink, &e.to_string()),
		));
		return;
	}
	mailbox.take();

	emit(protocol::started(w, h, req.fps));

	let t0 = Instant::now();
	let fps = u64::from(req.fps);
	let (mut frames, mut dropped, mut n): (u64, u64, u64) = (1, 0, 1);
	let mut reason = "stopped";
	let mut source_error: Option<String> = None;

	'grid: loop {
		let due = t0 + Duration::from_nanos(n * 1_000_000_000 / fps);
		loop {
			if stop.load(Ordering::Relaxed) {
				break 'grid;
			}
			let left = due.saturating_duration_since(Instant::now());
			if left.is_zero() {
				break;
			}
			std::thread::sleep(left.min(Duration::from_millis(10)));
		}

		let slot = mailbox.take();
		if slot.closed && !slot.fresh {
			reason = "source_closed";
			source_error = slot.error;
			break;
		}
		let Some(frame) = slot.frame else { break };
		if !slot.fresh {
			dropped += 1;
		}
		if let Err(e) = sink.write_frame(&frame.fit(w, h).data) {
			src.stop();
			emit(protocol::error(
				"encoder_failed",
				&finish_message(sink, &e.to_string()),
			));
			return;
		}
		frames += 1;
		n += 1;

		if n % fps == 0 {
			emit(protocol::progress(
				n * 1000 / fps,
				frames,
				dropped,
				file_size(req),
			));
		}
	}

	src.stop();
	if let Err(m) = sink.finish() {
		emit(protocol::error("encoder_failed", &m));
		return;
	}
	if let Some(why) = source_error {
		emit(protocol::error("source_gone", &why));
		return;
	}
	emit(protocol::finished(
		reason,
		frames,
		dropped,
		frames * 1000 / fps,
		file_size(req),
	));
}

/// Finishes a sink after a write failed, to learn what the encoder said; the write error is the fallback.
fn finish_message(sink: Box<dyn Sink>, fallback: &str) -> String {
	match sink.finish() {
		Err(m) => m,
		Ok(()) => fallback.to_string(),
	}
}

fn file_size(req: &StartRequest) -> u64 {
	std::fs::metadata(&req.output).map(|m| m.len()).unwrap_or(0)
}

/// Helper for the main loop: a stop flag shared with the thread that runs the session.
pub fn new_stop() -> Arc<AtomicBool> {
	Arc::new(AtomicBool::new(false))
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
		}
	}

	fn collect(req: &StartRequest, stop: &AtomicBool) -> Vec<json::Json> {
		let events = Mutex::new(Vec::new());
		run(
			req,
			&|line| events.lock().unwrap().push(json::parse(&line).unwrap()),
			stop,
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
		let events = collect(&req, &AtomicBool::new(false));
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
		assert!((40..=50).contains(&frames), "about 45 frames, got {frames}");
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
		let events = collect(&req, &AtomicBool::new(false));
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

	#[test]
	fn stop_ends_a_running_recording_gracefully_with_progress() {
		let dir = tmp("stop");
		let req = request(&dir, "16x16", 30);
		let stop = Arc::new(AtomicBool::new(false));
		let s2 = Arc::clone(&stop);
		let t = std::thread::spawn(move || {
			std::thread::sleep(Duration::from_millis(1500));
			s2.store(true, Ordering::Relaxed);
		});
		let events = collect(&req, &stop);
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
		let events = collect(&request(&dir, "nonsense", 30), &AtomicBool::new(false));
		assert_eq!(kinds(&events), ["error"]);
		assert_eq!(events[0].get("code").unwrap().as_str(), Some("source_gone"));
		std::fs::remove_dir_all(dir).unwrap();
	}

	#[test]
	fn an_unwritable_output_is_an_encoder_error() {
		let dir = tmp("nowrite");
		let mut req = request(&dir, "16x16", 30);
		req.output = dir.join("missing-folder").join("out.raw");
		let events = collect(&req, &AtomicBool::new(false));
		assert_eq!(
			events.last().unwrap().get("code").unwrap().as_str(),
			Some("encoder_failed")
		);
		std::fs::remove_dir_all(dir).unwrap();
	}

	#[test]
	fn a_frame_of_another_size_is_fitted_not_rejected() {
		let dir = tmp("fit");
		let events = collect(&request(&dir, "33x33r60n10", 30), &AtomicBool::new(false));
		let started = &events[0];
		assert_eq!(
			started.get("width").unwrap().as_u64(),
			Some(32),
			"odd sizes are cut to even"
		);
		let _ = std::fs::remove_dir_all(dir);
	}
}
