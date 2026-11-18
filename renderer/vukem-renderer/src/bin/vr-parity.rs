//! The native side of the parity harness (SDD-013 section 3.10, skill `renderer-parity`).
//!
//! `vr-parity <corpus> <output>` reads a corpus written by `engine/export`'s parity test: for each
//! case the render inputs, the textures and the bitmaps the Go compositor was given. It renders
//! every case through [`Renderer`] and writes the frames, one after the other, to the output file.
//! It reads and writes files named on the command line and nothing else.
//!
//! Corpus, little endian: `"VVPC"`, `u32` version 2, `u32` cases; a case is `u32` renders, that
//! many (`u32` length, JSON), `u32` textures, each `u8` id length, id, `u32` w, `u32` h, `u8`
//! opaque, pixels; `u32` bitmaps, each `u8` key length, key, `u32` w, `u32` h, four `f64` (natural
//! w and h, centre dx and dy), pixels; `u32` lookup tables, each `u8` id length, id, `u32` size, then
//! `size^3 x 3` little-endian `u16` entries. The renders of a case draw into one frame in order, which
//! is how a band split is expressed (`output.y0`, `output.y1`).

use std::fs::File;
use std::io::{self, BufReader, BufWriter, Read, Write};
use std::process::ExitCode;

use vukem_renderer::host::{BitmapMetrics, Limits, Renderer};
use vukem_renderer::plan::decode;

fn main() -> ExitCode {
	let args: Vec<String> = std::env::args().collect();
	let [_, corpus, output] = args.as_slice() else {
		eprintln!("usage: vr-parity <corpus> <output>");
		return ExitCode::from(2);
	};
	match run(corpus, output) {
		Ok(()) => ExitCode::SUCCESS,
		Err(e) => {
			eprintln!("vr-parity: {e}");
			ExitCode::FAILURE
		}
	}
}

fn run(corpus: &str, output: &str) -> Result<(), String> {
	let mut r = BufReader::new(File::open(corpus).map_err(|e| e.to_string())?);
	let mut w = BufWriter::new(File::create(output).map_err(|e| e.to_string())?);
	let io = |e: io::Error| e.to_string();

	let mut magic = [0_u8; 4];
	r.read_exact(&mut magic).map_err(io)?;
	if &magic != b"VVPC" || u32_of(&mut r).map_err(io)? != 2 {
		return Err("not a parity corpus of version 2".into());
	}
	let cases = u32_of(&mut r).map_err(io)?;
	for case in 0..cases {
		let frame = one_case(&mut r).map_err(|e| format!("case {case}: {e}"))?;
		w.write_all(&frame).map_err(io)?;
	}
	w.flush().map_err(io)
}

fn one_case(r: &mut impl Read) -> Result<Vec<u8>, String> {
	let io = |e: io::Error| e.to_string();
	let renders = u32_of(r).map_err(io)?;
	let mut inputs = Vec::new();
	for _ in 0..renders {
		let n = u32_of(r).map_err(io)? as usize;
		inputs.push(bytes(r, n).map_err(io)?);
	}

	let mut host = Renderer::new(Limits::default());
	for _ in 0..u32_of(r).map_err(io)? {
		let id = name(r).map_err(io)?;
		let (w, h) = (
			u32_of(r).map_err(io)? as usize,
			u32_of(r).map_err(io)? as usize,
		);
		let opaque = u8_of(r).map_err(io)? == 1;
		let pix = bytes(r, w * h * 4).map_err(io)?;
		host.put_texture(&id, w, h, &pix, opaque)
			.map_err(|e| e.to_string())?;
	}
	for _ in 0..u32_of(r).map_err(io)? {
		let key = name(r).map_err(io)?;
		let (w, h) = (
			u32_of(r).map_err(io)? as usize,
			u32_of(r).map_err(io)? as usize,
		);
		let m = BitmapMetrics {
			natural_w: f64_of(r).map_err(io)?,
			natural_h: f64_of(r).map_err(io)?,
			center_dx: f64_of(r).map_err(io)?,
			center_dy: f64_of(r).map_err(io)?,
		};
		let pix = bytes(r, w * h * 4).map_err(io)?;
		host.put_bitmap(&key, w, h, &pix, m)
			.map_err(|e| e.to_string())?;
	}

	for _ in 0..u32_of(r).map_err(io)? {
		let id = name(r).map_err(io)?;
		let size = u32_of(r).map_err(io)? as usize;
		let data = bytes(
			r,
			size.saturating_mul(size)
				.saturating_mul(size)
				.saturating_mul(6),
		)
		.map_err(io)?;
		host.put_lut(&id, size, &data).map_err(|e| e.to_string())?;
	}

	let first = inputs.first().ok_or("a case without a render")?;
	let size = decode(first).map_err(|e| e.to_string())?.output;
	let mut frame = vec![0_u8; size.width * size.height * 4];
	for input in &inputs {
		host.render(input, &mut frame).map_err(|e| e.to_string())?;
	}
	Ok(frame)
}

fn bytes(r: &mut impl Read, n: usize) -> io::Result<Vec<u8>> {
	let mut b = vec![0_u8; n];
	r.read_exact(&mut b)?;
	Ok(b)
}

fn u8_of(r: &mut impl Read) -> io::Result<u8> {
	Ok(bytes(r, 1)?[0])
}

fn u32_of(r: &mut impl Read) -> io::Result<u32> {
	let mut b = [0_u8; 4];
	r.read_exact(&mut b)?;
	Ok(u32::from_le_bytes(b))
}

fn f64_of(r: &mut impl Read) -> io::Result<f64> {
	let mut b = [0_u8; 8];
	r.read_exact(&mut b)?;
	Ok(f64::from_le_bytes(b))
}

fn name(r: &mut impl Read) -> io::Result<String> {
	let n = usize::from(u8_of(r)?);
	String::from_utf8(bytes(r, n)?).map_err(|e| io::Error::new(io::ErrorKind::InvalidData, e))
}
