//! The exports of the core module (SDD-013 section 4.2): plain C-ABI functions over
//! [`Renderer`], with no import, so the Go export (`wazero`) and the browser load the same bytes.
//!
//! This is the reviewed `unsafe` boundary of the crate (rule `.agents/rules/rust.md` 4): every
//! block turns an offset into the module's own memory, which the host wrote, into a slice. All
//! numbers are `i32` or `u32`; a pointer is an offset into `memory`.

#![allow(unsafe_code)]

use std::alloc::{Layout, alloc, dealloc};
use std::cell::RefCell;

use crate::API_VERSION;
use crate::error::{Error, Reason};
use crate::host::{BitmapMetrics, Limits, Renderer};
use crate::rasterhost::TextHost;

thread_local! {
	static RENDERER: RefCell<Renderer> = RefCell::new(Renderer::new(Limits::default()));
	static TEXT: RefCell<TextHost> = RefCell::new(TextHost::new());
	static PREPARED: RefCell<Vec<u8>> = const { RefCell::new(Vec::new()) };
	static LAST_ERROR: RefCell<String> = const { RefCell::new(String::new()) };
}

/// Records a failure for `vr_error` and returns its status.
fn fail(e: &Error) -> i32 {
	LAST_ERROR.with(|m| m.borrow_mut().clone_from(&e.detail));
	e.reason.status()
}

fn status(r: Result<(), Error>) -> i32 {
	match r {
		Ok(()) => {
			LAST_ERROR.with(|m| m.borrow_mut().clear());
			0
		}
		Err(e) => fail(&e),
	}
}

/// The block a host passed: `len` bytes at `ptr`.
///
/// # Safety
///
/// The host owns the block for the length of the call (SDD-013 section 4.4): `ptr` and `len` name
/// bytes of this module's memory that no other code touches during the call. An empty block is
/// never dereferenced.
unsafe fn block<'a>(ptr: u32, len: u32) -> &'a [u8] {
	if len == 0 {
		return &[];
	}
	// SAFETY: see above; the memory is the module's own, so a lying host can only read bytes of
	// its own instance, and a pointer outside the memory traps (an out-of-bounds access).
	unsafe { core::slice::from_raw_parts(ptr as usize as *const u8, len as usize) }
}

/// The block a host passed for writing.
///
/// # Safety
///
/// As [`block`], and the block does not overlap any other block of the call.
unsafe fn block_mut<'a>(ptr: u32, len: u32) -> &'a mut [u8] {
	if len == 0 {
		return &mut [];
	}
	// SAFETY: see above.
	unsafe { core::slice::from_raw_parts_mut(ptr as usize as *mut u8, len as usize) }
}

fn text<'a>(ptr: u32, len: u32) -> Result<&'a str, Error> {
	// SAFETY: a block of the host, valid for the call.
	let b = unsafe { block(ptr, len) };
	core::str::from_utf8(b).map_err(|_| Error::new(Reason::InvalidArgument, "an id is not UTF-8"))
}

fn layout(len: u32) -> Option<Layout> {
	Layout::from_size_align((len as usize).max(1), 8).ok()
}

/// The byte length of a `width x height` RGBA8 block, or `limit_exceeded` when it cannot be one.
fn rgba_len(width: u32, height: u32) -> Result<u32, Error> {
	let n = u64::from(width) * u64::from(height) * 4;
	u32::try_from(n).map_err(|_| Error::new(Reason::LimitExceeded, "render_texture_side"))
}

/// `API_VERSION` of this module.
#[unsafe(no_mangle)]
pub extern "C" fn vr_api_version() -> u32 {
	API_VERSION
}

/// Reserves `len` bytes, 8-aligned; 0 when the memory is exhausted.
#[unsafe(no_mangle)]
pub extern "C" fn vr_alloc(len: u32) -> u32 {
	let Some(l) = layout(len) else { return 0 };
	// SAFETY: the layout has a non-zero size.
	let p = unsafe { alloc(l) };
	p as usize as u32
}

/// Releases a block of `vr_alloc` with the same `len`.
#[unsafe(no_mangle)]
pub extern "C" fn vr_free(ptr: u32, len: u32) {
	let Some(l) = layout(len) else { return };
	if ptr == 0 {
		return;
	}
	// SAFETY: the host passes a block it got from `vr_alloc` with the same length.
	unsafe { dealloc(ptr as usize as *mut u8, l) }
}

/// Prepares an empty renderer with its two budgets; a second call resets it.
#[unsafe(no_mangle)]
pub extern "C" fn vr_init(max_texture_bytes: u32, max_bitmap_bytes: u32) -> i32 {
	let limits = Limits {
		max_texture_bytes: max_texture_bytes as usize,
		max_bitmap_bytes: max_bitmap_bytes as usize,
	};
	RENDERER.with(|r| *r.borrow_mut() = Renderer::new(limits));
	status(Ok(()))
}

/// Stores the premultiplied pixels of a media layer under an item id.
#[unsafe(no_mangle)]
pub extern "C" fn vr_put_texture(
	id_ptr: u32,
	id_len: u32,
	width: u32,
	height: u32,
	rgba_ptr: u32,
	opaque: u32,
) -> i32 {
	status((|| {
		let id = text(id_ptr, id_len)?;
		let n = rgba_len(width, height)?;
		// SAFETY: a block of the host, valid for the call.
		let rgba = unsafe { block(rgba_ptr, n) };
		RENDERER.with(|r| {
			r.borrow_mut()
				.put_texture(id, width as usize, height as usize, rgba, opaque == 1)
		})
	})())
}

/// Stores a straight-alpha bitmap of the rasterizer under its key.
#[unsafe(no_mangle)]
pub extern "C" fn vr_put_bitmap(
	key_ptr: u32,
	key_len: u32,
	width: u32,
	height: u32,
	natural_w: f64,
	natural_h: f64,
	center_dx: f64,
	center_dy: f64,
	rgba_ptr: u32,
) -> i32 {
	status((|| {
		let key = text(key_ptr, key_len)?;
		let n = rgba_len(width, height)?;
		// SAFETY: a block of the host, valid for the call.
		let rgba = unsafe { block(rgba_ptr, n) };
		let metrics = BitmapMetrics {
			natural_w,
			natural_h,
			center_dx,
			center_dy,
		};
		RENDERER.with(|r| {
			r.borrow_mut()
				.put_bitmap(key, width as usize, height as usize, rgba, metrics)
		})
	})())
}

/// Stores a lookup table under the id of its asset: `size` points on a side and the entries, little endian.
#[unsafe(no_mangle)]
pub extern "C" fn vr_put_lut(
	id_ptr: u32,
	id_len: u32,
	size: u32,
	data_ptr: u32,
	data_len: u32,
) -> i32 {
	status((|| {
		let id = text(id_ptr, id_len)?;
		// SAFETY: a block of the host, valid for the call.
		let data = unsafe { block(data_ptr, data_len) };
		RENDERER.with(|r| r.borrow_mut().put_lut(id, size as usize, data))
	})())
}

/// Forgets the texture, bitmap or lookup table with that id or key.
#[unsafe(no_mangle)]
pub extern "C" fn vr_release(id_ptr: u32, id_len: u32) -> i32 {
	status((|| {
		let id = text(id_ptr, id_len)?;
		RENDERER.with(|r| r.borrow_mut().release(id));
		Ok(())
	})())
}

/// Draws the render input into the output block, which must be exactly the frame size.
#[unsafe(no_mangle)]
pub extern "C" fn vr_render(in_ptr: u32, in_len: u32, out_ptr: u32, out_len: u32) -> i32 {
	// SAFETY: two blocks of the host, valid for the call; the host keeps them apart.
	let (input, out) = unsafe { (block(in_ptr, in_len), block_mut(out_ptr, out_len)) };
	status(RENDERER.with(|r| r.borrow().render(input, out)))
}

/// Copies the detail of the last failed call and returns its full length.
#[unsafe(no_mangle)]
pub extern "C" fn vr_error(buf_ptr: u32, buf_len: u32) -> u32 {
	LAST_ERROR.with(|m| {
		let m = m.borrow();
		let n = m.len().min(buf_len as usize);
		// SAFETY: a block of the host, valid for the call.
		let dst = unsafe { block_mut(buf_ptr, n as u32) };
		dst.copy_from_slice(&m.as_bytes()[..n]);
		m.len() as u32
	})
}

/// Stores the bytes of a user font under its lower-case hex SHA-256; no bytes record that the host
/// does not have the file.
#[unsafe(no_mangle)]
pub extern "C" fn vr_text_font_put(
	hash_ptr: u32,
	hash_len: u32,
	data_ptr: u32,
	data_len: u32,
) -> i32 {
	status((|| {
		let hash = text(hash_ptr, hash_len)?;
		// SAFETY: a block of the host, valid for the call.
		let data = unsafe { block(data_ptr, data_len) };
		TEXT.with(|t| t.borrow_mut().put_font(hash, data))
	})())
}

/// Draws or measures a shape, text or caption (the request of `rasterhost`); the result is read
/// with `vr_text_result_ptr` and `vr_text_result_len`.
#[unsafe(no_mangle)]
pub extern "C" fn vr_text_run(req_ptr: u32, req_len: u32) -> i32 {
	// SAFETY: a block of the host, valid for the call.
	let request = unsafe { block(req_ptr, req_len) };
	status(TEXT.with(|t| t.borrow_mut().run(request)))
}

/// Validates a user font file; a refusal leaves its reason and message as the result.
#[unsafe(no_mangle)]
pub extern "C" fn vr_text_validate_font(data_ptr: u32, data_len: u32) -> i32 {
	// SAFETY: a block of the host, valid for the call.
	let data = unsafe { block(data_ptr, data_len) };
	status(TEXT.with(|t| t.borrow_mut().validate_font(data)))
}

/// The offset of the result of the last text call; valid until the next call.
#[unsafe(no_mangle)]
pub extern "C" fn vr_text_result_ptr() -> u32 {
	TEXT.with(|t| t.borrow().result().as_ptr() as usize as u32)
}

/// The byte length of the result of the last text call.
#[unsafe(no_mangle)]
pub extern "C" fn vr_text_result_len() -> u32 {
	TEXT.with(|t| t.borrow().result().len() as u32)
}

/// Releases the result of the last text call.
#[unsafe(no_mangle)]
pub extern "C" fn vr_text_result_clear() {
	TEXT.with(|t| t.borrow_mut().clear_result());
}

/// Prepares the one layer of a render input (SDD-013 section 4.2.2); the block is read with
/// `vr_prepared_ptr` and `vr_prepared_len`.
#[unsafe(no_mangle)]
pub extern "C" fn vr_prepare(in_ptr: u32, in_len: u32) -> i32 {
	// SAFETY: a block of the host, valid for the call.
	let input = unsafe { block(in_ptr, in_len) };
	status(RENDERER.with(|r| {
		let block = r.borrow().prepare(input)?;
		PREPARED.with(|p| *p.borrow_mut() = block);
		Ok(())
	}))
}

/// The offset of the block of the last `vr_prepare`; valid until the next call.
#[unsafe(no_mangle)]
pub extern "C" fn vr_prepared_ptr() -> u32 {
	PREPARED.with(|p| p.borrow().as_ptr() as usize as u32)
}

/// The byte length of the block of the last `vr_prepare`, 0 when the layer draws nothing.
#[unsafe(no_mangle)]
pub extern "C" fn vr_prepared_len() -> u32 {
	PREPARED.with(|p| p.borrow().len() as u32)
}

/// Releases the block of the last `vr_prepare`.
#[unsafe(no_mangle)]
pub extern "C" fn vr_prepared_clear() {
	PREPARED.with(|p| *p.borrow_mut() = Vec::new());
}

/// Stores a block of `vr_prepare` as the texture of an item id.
#[unsafe(no_mangle)]
pub extern "C" fn vr_put_prepared(id_ptr: u32, id_len: u32, blk_ptr: u32, blk_len: u32) -> i32 {
	status((|| {
		let id = text(id_ptr, id_len)?;
		// SAFETY: a block of the host, valid for the call.
		let blk = unsafe { block(blk_ptr, blk_len) };
		RENDERER.with(|r| r.borrow_mut().put_prepared(id, blk))
	})())
}
