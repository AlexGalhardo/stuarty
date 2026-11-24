//! `stuarty-aihelper`: the local AI helper of Stuarty (ADR-0050, SDD-024).
//!
//! It reads raw frames from standard input and writes one JSON object per line on standard output.
//! It decides nothing about a project: the server owns every rule. The ONNX Runtime library and the
//! model are files the server verified by their SHA-256 before it started this process.

mod protocol;
mod runtime;
mod yunet;

use std::io::{self, BufWriter, Write};
use std::process::ExitCode;

use protocol::{Args, Command, ErrorLine};

const VERSION: &str = env!("CARGO_PKG_VERSION");

fn main() -> ExitCode {
	let argv: Vec<String> = std::env::args().skip(1).collect();
	if argv.first().map(String::as_str) == Some("--version") {
		println!("stuarty-aihelper {VERSION}");
		return ExitCode::SUCCESS;
	}

	let stdout = io::stdout();
	let mut out = BufWriter::new(stdout.lock());
	let result = Args::parse(&argv).and_then(|args| match args.command {
		Command::Probe => runtime::probe(&args, &mut out),
		Command::Faces => runtime::faces(&args, &mut io::stdin().lock(), &mut out),
	});
	match result {
		Ok(()) => {
			let _ = out.flush();
			ExitCode::SUCCESS
		}
		Err(e) => {
			let _ = writeln!(out, "{}", ErrorLine::from(&e).to_json());
			let _ = out.flush();
			ExitCode::from(1)
		}
	}
}
