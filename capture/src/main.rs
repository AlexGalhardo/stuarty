//! stuarty-capture: the native capture helper (ADR-0049, SDD-025). It takes JSON commands on standard input and
//! answers with JSON events on standard output. It serves one recording and exits when it ends, or when its input
//! closes. It has no option but `--allow-test-source`, which only a test passes.

mod audio;
#[cfg(windows)]
mod audio_wasapi;
mod camera;
#[cfg(windows)]
mod camera_mf;
mod clock;
mod events;
mod frame;
mod json;
mod protocol;
mod session;
mod sink;
mod source;
mod testsrc;
mod video;
mod wav;
#[cfg(windows)]
mod win32;
#[cfg(windows)]
mod windows;

use std::io::{BufReader, Write};
use std::sync::mpsc;
use std::sync::{Arc, Mutex};

use protocol::{Capabilities, Command, Line, Policy, SourceLists};

enum Msg {
	Line(String),
	TooLong,
	Eof,
	SessionEnded,
}

fn main() {
	let mut policy = Policy::default();
	for a in std::env::args().skip(1) {
		if a == "--version" {
			// The server asks once, when it resolves the helper (SDD-007 6.10).
			println!("stuarty-capture {}", env!("CARGO_PKG_VERSION"));
			return;
		}
		if a == "--allow-test-source" {
			policy.allow_test_source = true;
		} else {
			eprintln!("stuarty-capture takes no argument");
			std::process::exit(2);
		}
	}
	// The pointer and the size of a window are read in the physical pixels the capture delivers.
	#[cfg(windows)]
	win32::per_monitor_dpi();
	serve(policy);
}

/// Active nanoseconds as the milliseconds of a message.
fn ms(ns: i64) -> u64 {
	u64::try_from(ns / 1_000_000).unwrap_or(0)
}

fn serve(policy: Policy) {
	let out = Arc::new(Mutex::new(std::io::stdout()));
	let emit = {
		let out = Arc::clone(&out);
		move |line: String| {
			let mut o = out
				.lock()
				.unwrap_or_else(std::sync::PoisonError::into_inner);
			// A closed output means the host is gone; its closed input stops the recording too.
			let _ = writeln!(o, "{line}");
			let _ = o.flush();
		}
	};
	let (display, window) = source::can_capture();
	let test = policy.allow_test_source;
	emit(protocol::ready(
		if cfg!(windows) {
			"windows"
		} else {
			std::env::consts::OS
		},
		Capabilities {
			display,
			window,
			audio: audio::can_capture() || test,
			camera: camera::can_capture() || test,
			events: cfg!(windows) || test,
		},
	));

	let (tx, rx) = mpsc::channel::<Msg>();
	let reader_tx = tx.clone();
	std::thread::spawn(move || {
		let mut input = BufReader::new(std::io::stdin());
		loop {
			let msg = match protocol::read_line(&mut input) {
				Ok(Line::Text(t)) => Msg::Line(t),
				Ok(Line::TooLong) => Msg::TooLong,
				Ok(Line::Eof) | Err(_) => Msg::Eof,
			};
			let end = matches!(msg, Msg::Eof);
			if reader_tx.send(msg).is_err() || end {
				return;
			}
		}
	});

	let mut control: Option<Arc<session::Control>> = None;
	let mut running: Option<std::thread::JoinHandle<()>> = None;

	for msg in rx {
		match msg {
			Msg::TooLong => emit(protocol::error("bad_request", "the line is too long")),
			Msg::Line(text) => match protocol::parse_command(&text, policy) {
				Err(why) => emit(protocol::error("bad_request", &why)),
				Ok(Command::Sources) => {
					let (displays, windows) = source::list();
					let (microphones, system_audio) = audio::list(test);
					emit(protocol::sources(&SourceLists {
						displays,
						windows,
						microphones,
						cameras: camera::list(test),
						system_audio,
					}));
				}
				Ok(Command::Stop) => {
					if let Some(c) = &control {
						c.request_stop();
					}
				}
				Ok(Command::Pause) => match control.as_ref().map(|c| c.clock.pause()) {
					Some(Ok(at)) => emit(protocol::paused(ms(at))),
					Some(Err(why)) => emit(protocol::error("bad_request", why)),
					None => emit(protocol::error("bad_request", "no recording is running")),
				},
				Ok(Command::Resume) => match control.as_ref().map(|c| c.clock.resume()) {
					Some(Ok(at)) => emit(protocol::resumed(ms(at))),
					Some(Err(why)) => emit(protocol::error("bad_request", why)),
					None => emit(protocol::error("bad_request", "no recording is running")),
				},
				Ok(Command::Start(req)) => {
					if running.is_some() {
						emit(protocol::error(
							"bad_request",
							"a recording is already running",
						));
						continue;
					}
					let ctl = session::Control::new();
					control = Some(Arc::clone(&ctl));
					let (tx, emit) = (tx.clone(), emit.clone());
					running = Some(std::thread::spawn(move || {
						session::run(&req, &emit, &ctl);
						let _ = tx.send(Msg::SessionEnded);
					}));
				}
			},
			Msg::Eof => {
				if let Some(c) = &control {
					c.request_stop();
				}
				if let Some(t) = running.take() {
					let _ = t.join();
				}
				return;
			}
			Msg::SessionEnded => {
				if let Some(t) = running.take() {
					let _ = t.join();
				}
				return;
			}
		}
	}
}
