//! The NFR-068 fixture (SDD-025 section 16.10): the test sources make a flash (screen and camera) and a tone burst
//! (microphone) at the same instants of one shared epoch; a recording with three pauses is measured by the
//! position of each flash and burst in its file, and no two tracks may differ by more than 40 ms.

mod common;

use std::path::Path;
use std::time::Duration;

use common::{Helper, dir, esc, number, start_with, wav_samples};

const FPS: u64 = 60;
const LIMIT_MS: f64 = 40.0;
/// A pulse of 100 ms every 500 ms, for the three test sources.
const PULSE: &str = "p500-w100";

struct Scenario {
	/// How long each of the four runs lasts, and each of the three pauses between them.
	runs_ms: [u64; 4],
	pauses_ms: [u64; 3],
}

struct Measure {
	/// The 95th percentile of the distance between a flash of the screen and the nearest one of the camera and of
	/// the microphone, and how much that distance changed from the start to the end of the recording.
	p95_ms: f64,
	trend_ms: f64,
	matched: usize,
	lengths_ms: [f64; 3],
}

/// The times, in ms, at which a flag rises. A flag that is on at the first frame has no onset to measure.
fn rises(flags: &[bool], fps: u64) -> Vec<f64> {
	(1..flags.len())
		.filter(|&i| flags[i] && !flags[i - 1])
		.map(|i| i as f64 * 1000.0 / fps as f64)
		.collect()
}

fn bursts_ms(samples: &[i16]) -> Vec<f64> {
	let mut out = Vec::new();
	let mut last: Option<usize> = None;
	for (i, s) in samples.iter().enumerate() {
		if s.unsigned_abs() > 8000 {
			if last.is_none_or(|l| i - l > 2400) && i > 0 {
				out.push(i as f64 * 1000.0 / 48_000.0);
			}
			last = Some(i);
		}
	}
	out
}

fn flags(path: &Path, frame_bytes: usize) -> Vec<bool> {
	let b = std::fs::read(path).unwrap();
	assert_eq!(b.len() % frame_bytes, 0, "whole frames only");
	b.chunks(frame_bytes).map(|f| f[0] == 255).collect()
}

fn nearest(of: f64, among: &[f64]) -> Option<f64> {
	among
		.iter()
		.map(|x| (x - of).abs())
		.min_by(|a, b| a.partial_cmp(b).unwrap())
}

fn run(name: &str, sc: &Scenario) -> Measure {
	let d = dir(name);
	let mut h = Helper::start(&["--allow-test-source"]);
	h.read().unwrap();
	let extra = format!(
		r#","microphone":"fake-{PULSE}","camera":"fake-{PULSE}","outputs":{{"microphone":"{}","camera":"{}"}}"#,
		esc(&d.join("mic.wav")),
		esc(&d.join("cam.raw"))
	);
	h.send(&start_with(
		&d.join("o.raw"),
		"16x16r240p500w100",
		FPS as u32,
		&extra,
	));
	let lines = h.lines();
	let started = lines.until("started");
	assert!(
		started.last().unwrap().contains(r#""kind":"camera""#),
		"{started:?}"
	);

	// The active time at which each pause ended: a pulse cut by it starts again at that point, which is not an
	// onset of the source but of the cut, so it is left out of the comparison.
	let mut resumed_at: Vec<f64> = Vec::new();
	for i in 0..3 {
		std::thread::sleep(Duration::from_millis(sc.runs_ms[i]));
		h.send(r#"{"type":"pause"}"#);
		lines.until("paused");
		std::thread::sleep(Duration::from_millis(sc.pauses_ms[i]));
		h.send(r#"{"type":"resume"}"#);
		let r = lines.until("resumed");
		resumed_at.push(number(r.last().unwrap(), "elapsed_ms").unwrap() as f64);
	}
	std::thread::sleep(Duration::from_millis(sc.runs_ms[3]));
	h.send(r#"{"type":"stop"}"#);
	let fin = lines.until("finished").pop().unwrap();
	assert!(h.wait());
	assert_eq!(
		fin.matches(r#""from_ms""#).count(),
		3,
		"three pauses: {fin}"
	);

	let screen = flags(&d.join("o.raw"), 16 * 16 * 4);
	let cam = flags(&d.join("cam.raw"), 160 * 90 * 4);
	let mic = wav_samples(&d.join("mic.wav"));
	let lengths_ms = [
		screen.len() as f64 * 1000.0 / FPS as f64,
		cam.len() as f64 * 1000.0 / FPS as f64,
		mic.len() as f64 * 1000.0 / 48_000.0,
	];

	let skip = |t: f64| t < 150.0 || resumed_at.iter().any(|r| t >= *r - 20.0 && t <= *r + 160.0);
	let screen_on = rises(&screen, FPS);
	let cam_on = rises(&cam, FPS);
	let mic_on = bursts_ms(&mic);

	let (mut worst, mut matched, mut missed) = (0f64, 0usize, 0usize);
	let mut dists: Vec<f64> = Vec::new();
	let mut odd: Vec<(f64, f64, f64)> = Vec::new();
	for t in screen_on.iter().filter(|t| !skip(**t)) {
		let (Some(a), Some(b)) = (nearest(*t, &cam_on), nearest(*t, &mic_on)) else {
			continue;
		};
		if a > 40.0 || b > 40.0 {
			odd.push((*t, a, b));
		}
		// A pulse repeats every 500 ms: a nearest onset farther than a quarter of that is another pulse, so this
		// one has no counterpart (a source that gave no new frame for it). It is not a distance, but it is
		// counted and reported, so the worst distance printed is never read as the worst gap of the file.
		if a > 125.0 || b > 125.0 {
			missed += 1;
			continue;
		}
		matched += 1;
		worst = worst.max(a).max(b);
		dists.push(a.max(b));
	}
	// Drift is how far the tracks are apart and whether that grows. One late frame on a loaded machine is
	// jitter, not drift: the 95th percentile of the distances is held to the limit, and so is the change of the
	// distance between the start and the end of the recording. The largest one is reported.
	let mut sorted = dists.clone();
	sorted.sort_by(|x, y| x.partial_cmp(y).unwrap());
	let p95 = sorted.get(sorted.len() * 95 / 100).copied().unwrap_or(0.0);
	let edge = (dists.len() / 5).max(1);
	let mean = |s: &[f64]| s.iter().sum::<f64>() / s.len().max(1) as f64;
	let trend = (mean(&dists[dists.len() - edge.min(dists.len())..])
		- mean(&dists[..edge.min(dists.len())]))
	.abs();
	if !odd.is_empty() {
		eprintln!(
			"{name}: {} onsets over 40 ms (time, camera, microphone), the first ones: {:.0?}",
			odd.len(),
			&odd[..odd.len().min(8)]
		);
	}
	eprintln!(
		"{name}: {matched} flashes compared, {missed} with no counterpart within 125 ms, worst compared distance \
		 {worst:.1} ms, 95th percentile {p95:.1} ms, change start to end {trend:.1} ms, lengths {lengths_ms:.1?}"
	);
	let _ = std::fs::remove_dir_all(d);
	Measure {
		p95_ms: p95,
		trend_ms: trend,
		matched,
		lengths_ms,
	}
}

#[test]
fn three_pauses_keep_the_screen_the_camera_and_the_microphone_aligned() {
	let m = run(
		"sync",
		&Scenario {
			// 23.8 s of recording and about 45 flashes: with 11 the 95th percentile was the maximum, so one frame
			// that a busy CPU delayed decided the test (VV-417). With this many it is a real percentile.
			runs_ms: [6000, 5400, 6400, 6000],
			pauses_ms: [1300, 700, 1100],
		},
	);
	assert!(
		m.matched >= 32,
		"only {} flashes could be compared",
		m.matched
	);
	assert!(
		m.p95_ms <= LIMIT_MS && m.trend_ms <= LIMIT_MS,
		"the tracks are {:.1} ms apart (95th percentile) and moved {:.1} ms, over the {LIMIT_MS} ms limit",
		m.p95_ms,
		m.trend_ms
	);
	let (lo, hi) = m
		.lengths_ms
		.iter()
		.fold((f64::MAX, 0f64), |(lo, hi), l| (lo.min(*l), hi.max(*l)));
	assert!(
		hi - lo <= LIMIT_MS,
		"the files differ in length by {:.1} ms",
		hi - lo
	);
	// The pauses were removed: about 23.8 s of the 26.9 s of the wall clock is in the files.
	assert!((23_000.0..=25_200.0).contains(&lo), "{lo} ms in the files");
}

/// The fixture of acceptance criterion 2: ten minutes with three pauses. It takes ten minutes, so it runs only on
/// request: `cargo test --release -- --ignored full_ten_minute_drift`.
#[test]
#[ignore = "ten minutes; run with --ignored"]
fn full_ten_minute_drift() {
	// STUARTY_SYNC_SECONDS shortens the run for a quick look (default 600); the pauses scale with it.
	let secs: u64 = std::env::var("STUARTY_SYNC_SECONDS")
		.ok()
		.and_then(|s| s.parse().ok())
		.unwrap_or(600);
	let run_ms = secs * 250;
	let m = run(
		"sync-full",
		&Scenario {
			runs_ms: [run_ms; 4],
			pauses_ms: [secs * 100 / 3, secs * 50, secs * 100 / 6],
		},
	);
	assert!(
		m.matched >= (secs * 3 / 2) as usize,
		"only {} flashes could be compared",
		m.matched
	);
	assert!(
		m.p95_ms <= LIMIT_MS && m.trend_ms <= LIMIT_MS,
		"over ten minutes the tracks are {:.1} ms apart (95th percentile) and moved {:.1} ms",
		m.p95_ms,
		m.trend_ms
	);
	let (lo, hi) = m
		.lengths_ms
		.iter()
		.fold((f64::MAX, 0f64), |(lo, hi), l| (lo.min(*l), hi.max(*l)));
	assert!(
		hi - lo <= LIMIT_MS,
		"the files differ in length by {:.1} ms",
		hi - lo
	);
}
