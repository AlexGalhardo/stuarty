//! A real capture on Windows: the helper records the first display for a few seconds through the LGPL FFmpeg
//! the environment names, and `ffprobe` reads the file. The frames only go to a temporary folder that the test
//! removes, and nothing reads them.
//!
//! verify:env-skip needs a Windows desktop session and STUARTY_FFMPEG and STUARTY_FFPROBE (the pinned LGPL
//! build); with either missing the test prints why and passes without capturing.
#![cfg(windows)]

use std::io::{BufRead, BufReader, Write};
use std::path::PathBuf;
use std::process::{Command, Stdio};
use std::time::Duration;

fn tools() -> Option<(PathBuf, PathBuf)> {
	let ffmpeg = PathBuf::from(std::env::var_os("STUARTY_FFMPEG")?);
	let ffprobe = PathBuf::from(std::env::var_os("STUARTY_FFPROBE")?);
	(ffmpeg.is_absolute() && ffmpeg.is_file() && ffprobe.is_file()).then_some((ffmpeg, ffprobe))
}

fn esc(p: &std::path::Path) -> String {
	p.to_string_lossy().replace('\\', "\\\\")
}

struct Run {
	lines: Vec<String>,
}

fn record(ffmpeg: &std::path::Path, out: &std::path::Path, encoder: &str, seconds: u64) -> Run {
	let mut child = Command::new(env!("CARGO_BIN_EXE_stuarty-capture"))
		.stdin(Stdio::piped())
		.stdout(Stdio::piped())
		.stderr(Stdio::null())
		.spawn()
		.unwrap();
	let mut stdin = child.stdin.take().unwrap();
	let mut stdout = BufReader::new(child.stdout.take().unwrap());
	let mut read = move || {
		let mut l = String::new();
		stdout.read_line(&mut l).unwrap();
		l.trim_end().to_string()
	};
	let ready = read();
	assert!(ready.contains(r#""display":true"#), "{ready}");
	writeln!(stdin, r#"{{"type":"sources"}}"#).unwrap();
	let sources = read();
	assert!(
		sources.contains(r#""id":"d1""#),
		"no first display in {sources}"
	);
	writeln!(
		stdin,
		r#"{{"type":"start","recording":"rec_it","source":{{"kind":"display","id":"d1"}},"fps":30,"encoder":"{encoder}","ffmpeg":"{}","output":"{}"}}"#,
		esc(ffmpeg),
		esc(out)
	)
	.unwrap();
	let mut lines = vec![read()];
	if lines[0].contains(r#""type":"started""#) {
		std::thread::sleep(Duration::from_secs(seconds));
		writeln!(stdin, r#"{{"type":"stop"}}"#).unwrap();
		loop {
			let l = read();
			let done = l.is_empty()
				|| l.contains(r#""type":"finished""#)
				|| l.contains(r#""type":"error""#);
			lines.push(l);
			if done {
				break;
			}
		}
	}
	drop(stdin);
	let _ = child.wait();
	Run { lines }
}

#[test]
fn records_the_first_display_into_a_playable_fragmented_mp4() {
	let Some((ffmpeg, ffprobe)) = tools() else {
		eprintln!("skipped: STUARTY_FFMPEG and STUARTY_FFPROBE name no tools");
		return;
	};
	let dir = std::env::temp_dir().join(format!("stuarty-capture-real-{}", std::process::id()));
	std::fs::create_dir_all(&dir).unwrap();
	let out = dir.join("capture.mp4");

	let mut used = None;
	for encoder in ["h264_nvenc", "libopenh264", "h264_mf"] {
		let run = record(&ffmpeg, &out, encoder, 3);
		if run
			.lines
			.last()
			.is_some_and(|l| l.contains(r#""type":"finished""#))
		{
			used = Some((encoder, run));
			break;
		}
		eprintln!("{encoder} did not finish: {:?}", run.lines);
	}
	let (encoder, run) = used.expect("no encoder produced a finished recording");
	let fin = run.lines.last().unwrap();
	assert!(fin.contains(r#""reason":"stopped""#), "{fin}");

	let probe = Command::new(&ffprobe)
		.args(["-v", "error", "-count_frames", "-select_streams", "v:0"])
		.args([
			"-show_entries",
			"stream=codec_name,nb_read_frames:format=duration",
			"-of",
			"default=nw=1",
		])
		.arg(&out)
		.output()
		.unwrap();
	let text = String::from_utf8_lossy(&probe.stdout).to_string();
	assert!(
		probe.status.success(),
		"ffprobe failed on the {encoder} file: {}",
		String::from_utf8_lossy(&probe.stderr)
	);
	assert!(text.contains("codec_name=h264"), "{text}");
	let duration: f64 = text
		.lines()
		.find_map(|l| l.strip_prefix("duration="))
		.expect("a duration")
		.parse()
		.expect("a number");
	assert!(
		(2.5..=4.5).contains(&duration),
		"the file lasts {duration} s for a 3 s recording ({encoder}): {text}"
	);
	let frames: u64 = text
		.lines()
		.find_map(|l| l.strip_prefix("nb_read_frames="))
		.expect("frames")
		.parse()
		.expect("a number");
	assert!(frames >= 60, "{frames} frames for about 3 s at 30 fps");
	let _ = std::fs::remove_dir_all(dir);
}

#[test]
fn a_window_id_the_helper_did_not_list_is_refused() {
	let Some((ffmpeg, _)) = tools() else {
		eprintln!("skipped: STUARTY_FFMPEG and STUARTY_FFPROBE name no tools");
		return;
	};
	let dir = std::env::temp_dir().join(format!("stuarty-capture-refuse-{}", std::process::id()));
	std::fs::create_dir_all(&dir).unwrap();
	let mut child = Command::new(env!("CARGO_BIN_EXE_stuarty-capture"))
		.stdin(Stdio::piped())
		.stdout(Stdio::piped())
		.stderr(Stdio::null())
		.spawn()
		.unwrap();
	let mut stdin = child.stdin.take().unwrap();
	let mut stdout = BufReader::new(child.stdout.take().unwrap());
	let mut l = String::new();
	stdout.read_line(&mut l).unwrap();
	writeln!(
		stdin,
		r#"{{"type":"start","recording":"rec_it","source":{{"kind":"window","id":"w1"}},"fps":30,"encoder":"libopenh264","ffmpeg":"{}","output":"{}"}}"#,
		esc(&ffmpeg),
		esc(&dir.join("o.mp4"))
	)
	.unwrap();
	l.clear();
	stdout.read_line(&mut l).unwrap();
	assert!(
		l.contains(r#""type":"error""#) && l.contains("source_gone"),
		"{l}"
	);
	drop(stdin);
	let _ = child.wait();
	assert!(
		!dir.join("o.mp4").exists(),
		"no file may be written for a refused source"
	);
	let _ = std::fs::remove_dir_all(dir);
}
