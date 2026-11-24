//! The sound, camera, pause and pointer tracks through the real binary with the test sources (SDD-025 section 16).
//! The test microphone makes a 1 kHz burst for the first `w` ms of every `p` ms of the shared test epoch and
//! the test screen flashes at the same instants, so the position of a burst in a WAV file can be compared with
//! the position of a flash in the video file.

mod common;

use std::path::Path;
use std::time::Duration;

use common::{Helper, dir, esc, number, start_with, wav_samples};

const RATE: i64 = 48_000;

/// The start sample of every run of loud samples at least `gap` samples after the previous one.
fn bursts(samples: &[i16], gap: usize) -> Vec<usize> {
	let mut out = Vec::new();
	let mut last: Option<usize> = None;
	for (i, s) in samples.iter().enumerate() {
		if s.unsigned_abs() > 8000 {
			if last.is_none_or(|l| i - l > gap) {
				out.push(i);
			}
			last = Some(i);
		}
	}
	out
}

fn mic_args(d: &Path) -> String {
	format!(
		r#","microphone":"fake","outputs":{{"microphone":"{}"}}"#,
		esc(&d.join("mic.wav"))
	)
}

#[test]
fn a_known_tone_is_found_in_the_microphone_file() {
	let d = dir("tone");
	let mut h = Helper::start(&["--allow-test-source"]);
	let ready = h.read().unwrap();
	assert!(ready.contains(r#""audio":true"#), "{ready}");
	h.send(&start_with(&d.join("o.raw"), "16x16r60", 30, &mic_args(&d)));
	let started = h.read().unwrap();
	assert!(
		started.contains(r#""kind":"screen""#) && started.contains(r#""kind":"microphone""#),
		"{started}"
	);
	std::thread::sleep(Duration::from_millis(3300));
	h.send(r#"{"type":"stop"}"#);
	let rest = h.read_until("finished");
	assert!(h.wait());
	let fin = rest.last().unwrap();
	assert!(fin.contains(r#""type":"finished""#), "{rest:?}");
	assert!(
		rest.iter().any(|l| l.contains(r#""type":"level""#)),
		"the meter spoke: {rest:?}"
	);

	let s = wav_samples(&d.join("mic.wav"));
	let ms = s.len() as i64 * 1000 / RATE;
	let mut tone = bursts(&s, 4_800);
	assert!(
		(3200..=3700).contains(&ms),
		"{ms} ms of sound for a recording of about 3.3 s"
	);
	// A burst is 100 ms of a 1 kHz tone at every second of the epoch: three or four in 3.3 s. The recording
	// starts a few milliseconds after the epoch, so it can begin in the middle of the first burst; that one
	// has no onset to measure.
	assert!((3..=4).contains(&tone.len()), "{tone:?}");
	if tone[0] < 100 {
		tone.remove(0);
	}
	for pair in tone.windows(2) {
		let gap_ms = (pair[1] - pair[0]) as i64 * 1000 / RATE;
		assert!((990..=1010).contains(&gap_ms), "{gap_ms} ms between bursts");
	}
	let _ = std::fs::remove_dir_all(d);
}

#[test]
fn a_microphone_that_does_not_exist_ends_the_start_and_leaves_no_file_behind() {
	let d = dir("nomic");
	let mut h = Helper::start(&["--allow-test-source"]);
	h.read().unwrap();
	let extra = format!(
		r#","microphone":"fake","outputs":{{"microphone":"{}"}}"#,
		esc(&d.join("no-such-folder").join("mic.wav"))
	);
	h.send(&start_with(&d.join("o.raw"), "16x16", 30, &extra));
	// the folder of the file is not the folder of the output: the request is refused before anything starts
	let e = h.read().unwrap();
	assert!(
		e.contains("bad_request") && e.contains("outputs.microphone"),
		"{e}"
	);
	h.close_input();
	assert!(h.wait());
	assert!(!d.join("mic.wav").exists());
	let _ = std::fs::remove_dir_all(d);
}

#[test]
fn the_file_of_a_track_that_was_not_asked_for_is_refused() {
	let d = dir("extra");
	let mut h = Helper::start(&["--allow-test-source"]);
	h.read().unwrap();
	let extra = format!(
		r#","outputs":{{"microphone":"{}"}}"#,
		esc(&d.join("mic.wav"))
	);
	h.send(&start_with(&d.join("o.raw"), "16x16", 30, &extra));
	assert!(h.read().unwrap().contains("not asked for"));
	h.close_input();
	assert!(h.wait());
	assert_eq!(number(r#"{"a":12}"#, "a"), Some(12));
	let _ = std::fs::remove_dir_all(d);
}
