//! The pipe protocol of the native GPU host `vukem-render` (SDD-013 section 4.9, SDD-007 section 6.10).
//!
//! The Go export starts `vukem-render` as a child under the process policy of FFmpeg and talks to it
//! through standard input and output, the way it feeds frames to FFmpeg. Standard output carries
//! nothing but replies; the child reads no file, opens no connection and keeps no state outside its
//! memory.
//!
//! Everything is little endian. A request is `u8` kind, `u32` payload length, the payload. A reply is
//! `u8` status, `u32` payload length, the payload. Status 0 is success, 1 to 7 are the `reason`s of
//! SDD-013 section 7.1 (the payload is the detail), 8 is a failure of the device and 9 is a failure of
//! the protocol; after 8 or 9 the child ends.
//!
//! | kind | request | success payload |
//! | --- | --- | --- |
//! | 0 init | `u32` texture budget, `u32` bitmap budget (0: the default); first message, once | adapter text |
//! | 1 texture | `u8` id length, id, `u32` w, `u32` h, `u8` opaque, premultiplied RGBA8 | empty |
//! | 2 bitmap | `u8` key length, key, `u32` w, `u32` h, four `f64` (natural w and h, centre dx and dy), straight RGBA8 | empty |
//! | 3 release | `u8` id length, id | empty |
//! | 4 render | the render input JSON of SDD-013 section 2 | `width x height x 4` bytes |
//! | 5 lut | `u8` id length, id (the lut asset), `u32` size, `size^3 x 3` little-endian `u16` entries | empty |
//!
//! End of input at a message boundary ends the child cleanly.

use std::io::{self, Read, Write};

use vukem_renderer::error::{Error, Reason};
use vukem_renderer::host::{
	BitmapMetrics, DEFAULT_MAX_BITMAP_BYTES, DEFAULT_MAX_TEXTURE_BYTES, Limits, Renderer,
};
use vukem_renderer::plan::decode;

use crate::GpuError;

/// The largest payload of a request: an 8192 x 8192 texture and its header. Bigger is a failure of the
/// protocol, and nothing is allocated for it.
pub const MAX_PAYLOAD: usize = (8192 * 8192 * 4) + 4096;

const INIT: u8 = 0;
const TEXTURE: u8 = 1;
const BITMAP: u8 = 2;
const RELEASE: u8 = 3;
const RENDER: u8 = 4;
const LUT: u8 = 5;

/// Status of a failed device.
pub const STATUS_DEVICE: u8 = 8;
/// Status of a request the protocol does not allow.
pub const STATUS_PROTOCOL: u8 = 9;

/// What draws a frame: the GPU in the binary, a stand-in in the tests of the protocol.
pub trait Draw {
	/// Draws the render input with the stored pixels of `host` into `out` (`GpuRenderer::render`).
	fn draw(&self, host: &Renderer, input: &[u8], out: &mut [u8]) -> Result<(), GpuError>;
}

impl Draw for crate::GpuRenderer {
	fn draw(&self, host: &Renderer, input: &[u8], out: &mut [u8]) -> Result<(), GpuError> {
		self.render(host, input, out)
	}
}

/// Serves requests from `input` until it ends, answering on `output`. `adapter` is the text of the
/// answer to the init. A request the renderer refuses is answered and the child goes on; a failure
/// of the device or of the protocol is answered and returned.
pub fn serve(
	draw: &impl Draw,
	adapter: &str,
	input: &mut impl Read,
	output: &mut impl Write,
) -> io::Result<()> {
	let mut host: Option<Renderer> = None;
	loop {
		let (kind, payload) = match read_request(input) {
			Ok(Some(request)) => request,
			Ok(None) => return Ok(()),
			Err(e) => return fail(output, STATUS_PROTOCOL, &e.to_string()),
		};
		let result = match (kind, host.as_mut()) {
			(INIT, None) => init(&payload).map(|limits| {
				host = Some(Renderer::new(limits));
				adapter.as_bytes().to_vec()
			}),
			(INIT, Some(_)) => return fail(output, STATUS_PROTOCOL, "init twice"),
			(_, None) => return fail(output, STATUS_PROTOCOL, "the first message is not init"),
			(TEXTURE, Some(h)) => texture(h, &payload).map(|()| Vec::new()),
			(BITMAP, Some(h)) => bitmap(h, &payload).map(|()| Vec::new()),
			(RELEASE, Some(h)) => release(h, &payload).map(|()| Vec::new()),
			(LUT, Some(h)) => lut(h, &payload).map(|()| Vec::new()),
			(RENDER, Some(h)) => match render(draw, h, &payload) {
				Ok(frame) => Ok(frame),
				Err(GpuError::Render(e)) => Err(e),
				Err(GpuError::Device(m)) => return fail(output, STATUS_DEVICE, &m),
				Err(GpuError::NoAdapter) => {
					return fail(output, STATUS_DEVICE, "no graphics adapter");
				}
			},
			_ => return fail(output, STATUS_PROTOCOL, "unknown kind"),
		};
		match result {
			Ok(body) => reply(output, 0, &body)?,
			Err(e) => reply(output, e.reason as u8, e.detail.as_bytes())?,
		}
	}
}

fn fail(output: &mut impl Write, status: u8, detail: &str) -> io::Result<()> {
	reply(output, status, detail.as_bytes())?;
	Err(io::Error::other(format!("status {status}: {detail}")))
}

fn reply(output: &mut impl Write, status: u8, body: &[u8]) -> io::Result<()> {
	output.write_all(&[status])?;
	output.write_all(&u32::try_from(body.len()).unwrap_or(u32::MAX).to_le_bytes())?;
	output.write_all(body)?;
	output.flush()
}

/// Reads one request; `None` at a clean end of input. A length over [`MAX_PAYLOAD`] is an error
/// before anything is allocated, and a short payload is an error.
fn read_request(input: &mut impl Read) -> io::Result<Option<(u8, Vec<u8>)>> {
	let mut kind = [0_u8; 1];
	if input.read(&mut kind)? == 0 {
		return Ok(None);
	}
	let mut len = [0_u8; 4];
	input.read_exact(&mut len)?;
	let len = u32::from_le_bytes(len) as usize;
	if len > MAX_PAYLOAD {
		return Err(io::Error::new(
			io::ErrorKind::InvalidData,
			"a request is over the largest payload",
		));
	}
	let mut payload = Vec::new();
	input.take(len as u64).read_to_end(&mut payload)?;
	if payload.len() != len {
		return Err(io::ErrorKind::UnexpectedEof.into());
	}
	Ok(Some((kind[0], payload)))
}

/// A cursor over a payload; every read is checked.
struct Reader<'a>(&'a [u8]);

impl<'a> Reader<'a> {
	fn take(&mut self, n: usize) -> Result<&'a [u8], Error> {
		if self.0.len() < n {
			return Err(Error::new(Reason::InvalidArgument, "short_message"));
		}
		let (head, rest) = self.0.split_at(n);
		self.0 = rest;
		Ok(head)
	}

	fn u8(&mut self) -> Result<u8, Error> {
		Ok(self.take(1)?[0])
	}

	fn u32(&mut self) -> Result<usize, Error> {
		let b = self.take(4)?;
		Ok(u32::from_le_bytes([b[0], b[1], b[2], b[3]]) as usize)
	}

	fn f64(&mut self) -> Result<f64, Error> {
		let b = self.take(8)?;
		let mut a = [0_u8; 8];
		a.copy_from_slice(b);
		Ok(f64::from_le_bytes(a))
	}

	fn name(&mut self) -> Result<&'a str, Error> {
		let n = usize::from(self.u8()?);
		str::from_utf8(self.take(n)?).map_err(|_| Error::new(Reason::InvalidArgument, "name"))
	}

	fn rest(&mut self) -> &'a [u8] {
		std::mem::take(&mut self.0)
	}
}

fn init(payload: &[u8]) -> Result<Limits, Error> {
	let mut r = Reader(payload);
	let (tex, bm) = (r.u32()?, r.u32()?);
	Ok(Limits {
		max_texture_bytes: if tex == 0 {
			DEFAULT_MAX_TEXTURE_BYTES
		} else {
			tex
		},
		max_bitmap_bytes: if bm == 0 {
			DEFAULT_MAX_BITMAP_BYTES
		} else {
			bm
		},
	})
}

fn texture(host: &mut Renderer, payload: &[u8]) -> Result<(), Error> {
	let mut r = Reader(payload);
	let id = r.name()?;
	let (w, h) = (r.u32()?, r.u32()?);
	let opaque = r.u8()? == 1;
	host.put_texture(id, w, h, r.rest(), opaque)
}

fn bitmap(host: &mut Renderer, payload: &[u8]) -> Result<(), Error> {
	let mut r = Reader(payload);
	let key = r.name()?;
	let (w, h) = (r.u32()?, r.u32()?);
	let metrics = BitmapMetrics {
		natural_w: r.f64()?,
		natural_h: r.f64()?,
		center_dx: r.f64()?,
		center_dy: r.f64()?,
	};
	host.put_bitmap(key, w, h, r.rest(), metrics)
}

fn lut(host: &mut Renderer, payload: &[u8]) -> Result<(), Error> {
	let mut r = Reader(payload);
	let id = r.name()?;
	let size = r.u32()?;
	host.put_lut(id, size, r.rest())
}

fn release(host: &mut Renderer, payload: &[u8]) -> Result<(), Error> {
	host.release(Reader(payload).name()?);
	Ok(())
}

fn render(draw: &impl Draw, host: &Renderer, input: &[u8]) -> Result<Vec<u8>, GpuError> {
	let size = decode(input)?.output;
	let mut frame = vec![0_u8; size.width * size.height * 4];
	draw.draw(host, input, &mut frame)?;
	Ok(frame)
}
