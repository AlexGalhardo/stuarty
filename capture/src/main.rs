//! stuarty-capture: the native capture helper (ADR-0049, SDD-025). It takes JSON commands on standard input and
//! answers with JSON events on standard output. It serves one recording and exits when it ends, or when its input
//! closes. It has no option but `--allow-test-source`, which only a test passes.

mod frame;
mod json;
mod protocol;
mod session;
mod sink;
mod source;
#[cfg(windows)]
mod windows;

use std::io::{BufReader, Write};
use std::sync::atomic::Ordering;
use std::sync::mpsc;
use std::sync::{Arc, Mutex};

use protocol::{Command, Line, Policy};

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
	serve(policy);
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
	emit(protocol::ready(
		if cfg!(windows) {
			"windows"
		} else {
			std::env::consts::OS
		},
		display,
		window,
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

	let stop = session::new_stop();
	let mut running: Option<std::thread::JoinHandle<()>> = None;

	for msg in rx {
		match msg {
			Msg::TooLong => emit(protocol::error("bad_request", "the line is too long")),
			Msg::Line(text) => match protocol::parse_command(&text, policy) {
				Err(why) => emit(protocol::error("bad_request", &why)),
				Ok(Command::Sources) => {
					let (displays, windows) = source::list();
					emit(protocol::sources(&displays, &windows));
				}
				Ok(Command::Stop) => stop.store(true, Ordering::Relaxed),
				Ok(Command::Start(req)) => {
					if running.is_some() {
						emit(protocol::error(
							"bad_request",
							"a recording is already running",
						));
						continue;
					}
					let (stop, tx, emit) = (Arc::clone(&stop), tx.clone(), emit.clone());
					running = Some(std::thread::spawn(move || {
						session::run(&req, &emit, &stop);
						let _ = tx.send(Msg::SessionEnded);
					}));
				}
			},
			Msg::Eof => {
				stop.store(true, Ordering::Relaxed);
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
