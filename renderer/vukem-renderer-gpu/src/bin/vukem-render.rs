//! `vukem-render`: the native GPU host of the export (ADR-0029 item 4, SDD-013 section 4.9).
//!
//! `vukem-render --version` prints the version, `vukem-render --probe` prints the adapter the machine
//! offers and exits 0, or exits 3 when there is none. With no argument it serves the pipe protocol of
//! `vukem_renderer_gpu::proto` on standard input and output. It reads no file and opens no connection.

use std::io::{self, BufReader, BufWriter};
use std::process::ExitCode;

use vukem_renderer_gpu::proto::serve;
use vukem_renderer_gpu::{Adapter, GpuError, GpuRenderer};

/// No graphics adapter: the caller falls back to the CPU path.
const EXIT_NO_ADAPTER: u8 = 3;

fn main() -> ExitCode {
	let args: Vec<String> = std::env::args().skip(1).collect();
	match args
		.iter()
		.map(String::as_str)
		.collect::<Vec<_>>()
		.as_slice()
	{
		["--version"] => {
			println!("vukem-render {}", env!("CARGO_PKG_VERSION"));
			ExitCode::SUCCESS
		}
		["--probe"] => match open() {
			Ok(gpu) => {
				println!("{}", describe(&gpu));
				ExitCode::SUCCESS
			}
			Err(code) => code,
		},
		[] => match open() {
			Ok(gpu) => run(&gpu),
			Err(code) => code,
		},
		_ => {
			eprintln!("usage: vukem-render [--version | --probe]");
			ExitCode::from(2)
		}
	}
}

fn open() -> Result<GpuRenderer, ExitCode> {
	GpuRenderer::new(Adapter::Any).map_err(|e| {
		eprintln!("vukem-render: {e}");
		match e {
			GpuError::NoAdapter => ExitCode::from(EXIT_NO_ADAPTER),
			_ => ExitCode::FAILURE,
		}
	})
}

/// The adapter on one line: name, backend and device type, never a path.
fn describe(gpu: &GpuRenderer) -> String {
	let i = gpu.adapter();
	format!("{} ({:?}, {:?})", i.name, i.backend, i.device_type)
}

fn run(gpu: &GpuRenderer) -> ExitCode {
	let mut input = BufReader::new(io::stdin().lock());
	let mut output = BufWriter::new(io::stdout().lock());
	match serve(gpu, &describe(gpu), &mut input, &mut output) {
		Ok(()) => ExitCode::SUCCESS,
		Err(e) => {
			eprintln!("vukem-render: {e}");
			ExitCode::FAILURE
		}
	}
}
