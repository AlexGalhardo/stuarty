//! Talks to the real binary over its pipes, with the fake source (the helper is started with
//! `--allow-test-source`, which the server never passes).

use std::io::{BufRead, BufReader, Write};
use std::path::PathBuf;
use std::process::{Child, ChildStdin, ChildStdout, Command, Stdio};
use std::time::Duration;

struct Helper {
	child: Child,
	stdin: Option<ChildStdin>,
	out: BufReader<ChildStdout>,
}

impl Helper {
	fn start(args: &[&str]) -> Self {
		let mut child = Command::new(env!("CARGO_BIN_EXE_stuarty-capture"))
			.args(args)
			.stdin(Stdio::piped())
			.stdout(Stdio::piped())
			.stderr(Stdio::null())
			.spawn()
			.unwrap();
		let stdin = child.stdin.take();
		let out = BufReader::new(child.stdout.take().unwrap());
		Self { child, stdin, out }
	}

	fn send(&mut self, line: &str) {
		let s = self.stdin.as_mut().unwrap();
		writeln!(s, "{line}").unwrap();
		s.flush().unwrap();
	}

	fn read(&mut self) -> Option<String> {
		let mut l = String::new();
		match self.out.read_line(&mut l).unwrap() {
			0 => None,
			_ => Some(l.trim_end().to_string()),
		}
	}

	fn read_until(&mut self, kind: &str) -> Vec<String> {
		let mut all = Vec::new();
		while let Some(l) = self.read() {
			let done = l.contains(&format!("\"type\":\"{kind}\""));
			all.push(l);
			if done {
				break;
			}
		}
		all
	}

	fn close_input(&mut self) {
		self.stdin.take();
	}

	fn wait(mut self) -> bool {
		self.child.wait().unwrap().success()
	}
}

fn dir(name: &str) -> PathBuf {
	let d = std::env::temp_dir().join(format!("stuarty-capture-it-{name}-{}", std::process::id()));
	std::fs::create_dir_all(&d).unwrap();
	d
}

fn start_line(out: &std::path::Path, source: &str) -> String {
	let esc = |p: &std::path::Path| p.to_string_lossy().replace('\\', "\\\\");
	format!(
		r#"{{"type":"start","recording":"rec_it","source":{{"kind":"fake","id":"{source}"}},"fps":30,"encoder":"raw","ffmpeg":"{}","output":"{}"}}"#,
		esc(&std::env::current_exe().unwrap()),
		esc(out)
	)
}

#[test]
fn announces_itself_and_lists_sources() {
	let mut h = Helper::start(&[]);
	let ready = h.read().unwrap();
	assert!(
		ready.contains(r#""type":"ready""#) && ready.contains(r#""v":1"#),
		"{ready}"
	);
	h.send(r#"{"type":"sources"}"#);
	let s = h.read().unwrap();
	assert!(
		s.contains(r#""type":"sources""#) && s.contains(r#""displays":["#),
		"{s}"
	);
	h.close_input();
	assert!(h.wait(), "end of input is a clean exit");
}

#[test]
fn bad_input_gets_an_error_and_the_helper_keeps_running() {
	let mut h = Helper::start(&[]);
	h.read().unwrap();
	for bad in [
		"not json",
		r#"{"type":"jump"}"#,
		r#"{"type":"start"}"#,
		&"x".repeat(70_000),
	] {
		h.send(bad);
		let e = h.read().unwrap();
		assert!(
			e.contains(r#""type":"error""#) && e.contains("bad_request"),
			"{bad:.20}: {e}"
		);
	}
	h.send(r#"{"type":"sources"}"#);
	assert!(h.read().unwrap().contains(r#""type":"sources""#));
	h.close_input();
	assert!(h.wait());
}

#[test]
fn the_test_source_is_refused_without_the_flag() {
	let d = dir("noflag");
	let mut h = Helper::start(&[]);
	h.read().unwrap();
	h.send(&start_line(&d.join("o.raw"), "16x16"));
	assert!(h.read().unwrap().contains("bad_request"));
	h.close_input();
	assert!(h.wait());
	let _ = std::fs::remove_dir_all(d);
}

#[test]
fn an_argument_is_refused() {
	let st = Command::new(env!("CARGO_BIN_EXE_stuarty-capture"))
		.arg("--ffmpeg=evil")
		.stderr(Stdio::null())
		.stdout(Stdio::null())
		.status()
		.unwrap();
	assert_eq!(st.code(), Some(2));
}

#[test]
fn records_until_stopped_and_exits() {
	let d = dir("stop");
	let out = d.join("o.raw");
	let mut h = Helper::start(&["--allow-test-source"]);
	h.read().unwrap();
	h.send(&start_line(&out, "16x16"));
	let started = h.read().unwrap();
	assert!(
		started.contains(r#""type":"started""#) && started.contains(r#""width":16"#),
		"{started}"
	);
	std::thread::sleep(Duration::from_millis(1300));
	h.send(r#"{"type":"stop"}"#);
	let rest = h.read_until("finished");
	let fin = rest.last().unwrap();
	assert!(fin.contains(r#""reason":"stopped""#), "{rest:?}");
	assert!(rest.iter().any(|l| l.contains(r#""type":"progress""#)));
	assert!(h.wait(), "the helper exits after its recording");
	assert!(std::fs::metadata(&out).unwrap().len() >= 30 * 16 * 16 * 4);
	let _ = std::fs::remove_dir_all(d);
}

#[test]
fn closing_the_input_ends_a_recording_with_its_file_complete() {
	let d = dir("eof");
	let out = d.join("o.raw");
	let mut h = Helper::start(&["--allow-test-source"]);
	h.read().unwrap();
	h.send(&start_line(&out, "16x16"));
	h.read().unwrap();
	std::thread::sleep(Duration::from_millis(800));
	h.close_input();
	let rest = h.read_until("finished");
	assert!(
		rest.last().unwrap().contains(r#""type":"finished""#),
		"{rest:?}"
	);
	assert!(h.wait());
	let len = std::fs::metadata(&out).unwrap().len();
	assert!(
		len > 0 && len.is_multiple_of(16 * 16 * 4),
		"whole frames only: {len}"
	);
	let _ = std::fs::remove_dir_all(d);
}

#[test]
fn a_second_start_is_refused_while_one_runs() {
	let d = dir("twice");
	let mut h = Helper::start(&["--allow-test-source"]);
	h.read().unwrap();
	h.send(&start_line(&d.join("a.raw"), "16x16"));
	h.read().unwrap();
	h.send(&start_line(&d.join("b.raw"), "16x16"));
	let e = h.read().unwrap();
	assert!(e.contains("bad_request") && e.contains("already"), "{e}");
	h.send(r#"{"type":"stop"}"#);
	h.read_until("finished");
	assert!(h.wait());
	let _ = std::fs::remove_dir_all(d);
}

#[test]
fn version_is_printed_and_exits() {
	let out = Command::new(env!("CARGO_BIN_EXE_stuarty-capture"))
		.arg("--version")
		.output()
		.unwrap();
	assert!(out.status.success());
	assert!(String::from_utf8_lossy(&out.stdout).starts_with("stuarty-capture "));
}
