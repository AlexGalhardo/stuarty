//! A helper process driven over its pipes, shared by the integration tests. The helper is started with
//! `--allow-test-source` where a test needs the fake sources, a flag the server never passes.
#![allow(dead_code)]

use std::io::{BufRead, BufReader, Write};
use std::path::{Path, PathBuf};
use std::process::{Child, ChildStdin, ChildStdout, Command, Stdio};

pub struct Helper {
	child: Child,
	stdin: Option<ChildStdin>,
	out: Option<BufReader<ChildStdout>>,
}

/// The lines of a helper read on a thread of their own, for a test that sleeps for minutes and must not let the
/// pipe fill.
pub struct Lines(std::sync::mpsc::Receiver<String>);

impl Lines {
	/// Every line up to and including the first one of `kind`.
	pub fn until(&self, kind: &str) -> Vec<String> {
		let mut all = Vec::new();
		while let Ok(l) = self.0.recv_timeout(std::time::Duration::from_secs(60)) {
			let done = l.contains(&format!("\"type\":\"{kind}\""));
			all.push(l);
			if done {
				return all;
			}
		}
		panic!("no {kind} message; got {all:?}");
	}
}

impl Helper {
	pub fn start(args: &[&str]) -> Self {
		let mut child = Command::new(env!("CARGO_BIN_EXE_stuarty-capture"))
			.args(args)
			.stdin(Stdio::piped())
			.stdout(Stdio::piped())
			.stderr(Stdio::null())
			.spawn()
			.unwrap();
		let stdin = child.stdin.take();
		let out = Some(BufReader::new(child.stdout.take().unwrap()));
		Self { child, stdin, out }
	}

	pub fn send(&mut self, line: &str) {
		let s = self.stdin.as_mut().unwrap();
		writeln!(s, "{line}").unwrap();
		s.flush().unwrap();
	}

	pub fn read(&mut self) -> Option<String> {
		let mut l = String::new();
		let out = self.out.as_mut().expect("the output moved to a thread");
		match out.read_line(&mut l).unwrap() {
			0 => None,
			_ => Some(l.trim_end().to_string()),
		}
	}

	pub fn read_until(&mut self, kind: &str) -> Vec<String> {
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

	/// Moves the reading of the helper output to a thread; `read` must not be used after it.
	pub fn lines(&mut self) -> Lines {
		let (tx, rx) = std::sync::mpsc::channel();
		let out = self.out.take().expect("the output moved to a thread");
		std::thread::spawn(move || {
			let mut out = out;
			let mut l = String::new();
			loop {
				l.clear();
				match out.read_line(&mut l) {
					Ok(0) | Err(_) => return,
					Ok(_) => {
						if tx.send(l.trim_end().to_string()).is_err() {
							return;
						}
					}
				}
			}
		});
		Lines(rx)
	}

	pub fn close_input(&mut self) {
		self.stdin.take();
	}

	pub fn wait(mut self) -> bool {
		self.child.wait().unwrap().success()
	}
}

pub fn dir(name: &str) -> PathBuf {
	let d = std::env::temp_dir().join(format!("stuarty-capture-it-{name}-{}", std::process::id()));
	std::fs::create_dir_all(&d).unwrap();
	d
}

pub fn esc(p: &Path) -> String {
	p.to_string_lossy().replace('\\', "\\\\")
}

/// A start line for the fake screen `source` (a test encoder, raw frames in `out`), with `extra` members added
/// to the object, for example `"microphone":"fake"`.
pub fn start_with(out: &Path, source: &str, fps: u32, extra: &str) -> String {
	format!(
		r#"{{"type":"start","recording":"rec_it","source":{{"kind":"fake","id":"{source}"}},"fps":{fps},"encoder":"raw","ffmpeg":"{}","output":"{}"{extra}}}"#,
		esc(&std::env::current_exe().unwrap()),
		esc(out)
	)
}

pub fn start_line(out: &Path, source: &str) -> String {
	start_with(out, source, 30, "")
}

/// The value of a number member of a one-line JSON object, by a plain search: the lines are the helper's own.
pub fn number(line: &str, key: &str) -> Option<u64> {
	let at = line.find(&format!("\"{key}\":"))? + key.len() + 3;
	let digits: String = line[at..]
		.chars()
		.take_while(char::is_ascii_digit)
		.collect();
	digits.parse().ok()
}

/// The samples of a 16-bit WAV file written by the helper.
pub fn wav_samples(path: &Path) -> Vec<i16> {
	let b = std::fs::read(path).unwrap();
	assert_eq!(&b[0..4], b"RIFF");
	assert_eq!(&b[36..40], b"data");
	b[44..]
		.as_chunks::<2>()
		.0
		.iter()
		.map(|c| i16::from_le_bytes(*c))
		.collect()
}
