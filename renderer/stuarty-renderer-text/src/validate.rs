//! The validation of a user font file: the port of `engine/raster/userfont.go` (SDD-007 section 7.7,
//! T-21): size, magic, table directory, then a parse.
//!
//! Go bounded the parse with a recover and a 5 second deadline whose goroutine it could only
//! abandon (SR1-02 of `docs/security-review-1.0.md`). Here the parse is bounded by construction: the
//! structure is checked before anything is read, `read-fonts` parses lazily and never panics on
//! malformed data (the whole crate forbids `unsafe`), and the glyphs touched are four. There is no
//! deadline to miss and no thread to leave behind.

use std::fmt;

use skrifa::raw::TableProvider;
use skrifa::{Font, FontRef};

/// The largest font file accepted (a CJK family can be large).
pub const MAX_FONT_BYTES: usize = 16 << 20;
/// The largest `numTables` of a font's table directory.
pub const MAX_FONT_TABLES: u64 = 64;
/// Bounds the fonts of a `.ttc` collection.
const MAX_COLLECTION_FONTS: u64 = 64;

/// The error code of a font file that fails validation.
pub const CODE_FONT_INVALID: &str = "E_VALIDATION";

/// Why a font file is refused (`details.reason`).
#[derive(Clone, Copy, Debug, PartialEq, Eq)]
pub enum Reason {
	TooLarge,
	BadMagic,
	Truncated,
	TooManyTables,
	TableOutOfRange,
	TablesOverlap,
	DuplicateTable,
	MissingTable,
	ParseFailed,
}

impl Reason {
	/// The identifier of the reason in an error's details.
	pub fn as_str(self) -> &'static str {
		match self {
			Self::TooLarge => "font_too_large",
			Self::BadMagic => "font_bad_magic",
			Self::Truncated => "font_truncated",
			Self::TooManyTables => "font_too_many_tables",
			Self::TableOutOfRange => "font_table_out_of_range",
			Self::TablesOverlap => "font_tables_overlap",
			Self::DuplicateTable => "font_duplicate_table",
			Self::MissingTable => "font_missing_table",
			Self::ParseFailed => "font_parse_failed",
		}
	}
}

/// A file that must not be stored or used (`raster.InvalidFontError`).
#[derive(Clone, Debug, PartialEq, Eq)]
pub struct InvalidFont {
	pub reason: Reason,
	pub message: String,
}

impl fmt::Display for InvalidFont {
	fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
		write!(
			f,
			"{CODE_FONT_INVALID}: font file refused ({}): {}",
			self.reason.as_str(),
			self.message
		)
	}
}

impl std::error::Error for InvalidFont {}

fn invalid(reason: Reason, message: String) -> InvalidFont {
	InvalidFont { reason, message }
}

/// Checks the bytes of a user font before it is stored or used.
pub fn check_font(data: &[u8]) -> Result<(), InvalidFont> {
	validate_structure(data)?;
	parse(data.to_vec()).map(|_| ())
}

/// [`check_font`] that keeps the parsed font.
pub(crate) fn validate_font(data: Vec<u8>) -> Result<Font, InvalidFont> {
	validate_structure(&data)?;
	parse(data)
}

const REQUIRED_TABLES: [&str; 5] = ["cmap", "head", "hhea", "hmtx", "maxp"];

struct TableRecord {
	tag: [u8; 4],
	off: u64,
	length: u64,
}

fn be16(data: &[u8], at: u64) -> u64 {
	let at = at as usize;
	u64::from(u16::from_be_bytes([data[at], data[at + 1]]))
}

fn be32(data: &[u8], at: u64) -> u64 {
	let at = at as usize;
	u64::from(u32::from_be_bytes([
		data[at],
		data[at + 1],
		data[at + 2],
		data[at + 3],
	]))
}

/// Reads only the header and the table directories. It never allocates in proportion to a declared
/// count: `numTables` is checked before the directory is read.
fn validate_structure(data: &[u8]) -> Result<(), InvalidFont> {
	if data.len() > MAX_FONT_BYTES {
		return Err(invalid(
			Reason::TooLarge,
			format!("{} bytes, at most {MAX_FONT_BYTES}", data.len()),
		));
	}
	if data.len() < 12 {
		return Err(invalid(
			Reason::Truncated,
			format!("{} bytes is not a font", data.len()),
		));
	}
	match &data[..4] {
		b"\x00\x01\x00\x00" | b"true" | b"OTTO" => validate_directory(data, 0),
		b"ttcf" => validate_collection(data),
		sig => Err(invalid(
			Reason::BadMagic,
			format!("unknown font signature {sig:02x?}"),
		)),
	}
}

fn validate_collection(data: &[u8]) -> Result<(), InvalidFont> {
	let n = be32(data, 8);
	if n == 0 || n > MAX_COLLECTION_FONTS {
		return Err(invalid(
			Reason::TooManyTables,
			format!("a collection of {n} fonts, at most {MAX_COLLECTION_FONTS}"),
		));
	}
	if (data.len() as u64) < 12 + 4 * n {
		return Err(invalid(
			Reason::Truncated,
			"the collection header is cut".to_owned(),
		));
	}
	for i in 0..n {
		let off = be32(data, 12 + 4 * i);
		if off + 12 > data.len() as u64 {
			return Err(invalid(
				Reason::TableOutOfRange,
				format!("font {i} starts at {off}, past the end of the file"),
			));
		}
		match &data[off as usize..off as usize + 4] {
			b"\x00\x01\x00\x00" | b"true" | b"OTTO" => {}
			_ => {
				return Err(invalid(
					Reason::BadMagic,
					format!("font {i} of the collection has an unknown signature"),
				));
			}
		}
		validate_directory(data, off)?;
	}
	Ok(())
}

fn validate_directory(data: &[u8], base: u64) -> Result<(), InvalidFont> {
	let size = data.len() as u64;
	let num = be16(data, base + 4);
	if num > MAX_FONT_TABLES {
		return Err(invalid(
			Reason::TooManyTables,
			format!("numTables is {num}, at most {MAX_FONT_TABLES}"),
		));
	}
	let dir_end = base + 12 + 16 * num;
	if dir_end > size {
		return Err(invalid(
			Reason::Truncated,
			format!("the table directory ends at {dir_end}, the file has {size} bytes"),
		));
	}
	let mut recs: Vec<TableRecord> = Vec::with_capacity(num as usize);
	for i in 0..num {
		let at = base + 12 + 16 * i;
		let r = TableRecord {
			tag: [
				data[at as usize],
				data[at as usize + 1],
				data[at as usize + 2],
				data[at as usize + 3],
			],
			off: be32(data, at + 8),
			length: be32(data, at + 12),
		};
		if r.off < dir_end || r.off + r.length > size {
			return Err(invalid(
				Reason::TableOutOfRange,
				format!(
					"table {:?} at {}, length {}, is outside the file",
					tag_str(r.tag),
					r.off,
					r.length
				),
			));
		}
		if recs.iter().any(|o| o.tag == r.tag) {
			return Err(invalid(
				Reason::DuplicateTable,
				format!("table {:?} appears twice", tag_str(r.tag)),
			));
		}
		recs.push(r);
	}
	for name in REQUIRED_TABLES {
		if !has_table(&recs, name.as_bytes()) {
			return Err(invalid(
				Reason::MissingTable,
				format!("required table {name:?} is missing"),
			));
		}
	}
	if !has_table(&recs, b"glyf") && !has_table(&recs, b"CFF ") {
		return Err(invalid(
			Reason::MissingTable,
			"neither glyf nor CFF is present".to_owned(),
		));
	}
	recs.sort_by_key(|r| r.off);
	for w in recs.windows(2) {
		if w[1].off < w[0].off + w[0].length {
			return Err(invalid(
				Reason::TablesOverlap,
				format!(
					"tables {:?} and {:?} overlap",
					tag_str(w[0].tag),
					tag_str(w[1].tag)
				),
			));
		}
	}
	Ok(())
}

fn tag_str(tag: [u8; 4]) -> String {
	String::from_utf8_lossy(&tag).into_owned()
}

fn has_table(recs: &[TableRecord], name: &[u8]) -> bool {
	recs.iter().any(|r| r.tag == name && r.length > 0)
}

/// Parses the font and touches a few glyphs, so lazy tables that are broken fail here and not while
/// a frame renders.
fn parse(data: Vec<u8>) -> Result<Font, InvalidFont> {
	// The tables every font needs are read lazily and a junk one reads as empty, so they are read
	// here, once, from the bytes before the font takes them.
	let tables_ok = FontRef::from_index(&data, 0).is_ok_and(|f| {
		f.head().is_ok() && f.hhea().is_ok() && f.maxp().is_ok() && f.cmap().is_ok()
	});
	if !tables_ok {
		return Err(invalid(
			Reason::ParseFailed,
			"a table every font needs does not parse".to_owned(),
		));
	}
	let font = Font::new(data, 0).ok_or_else(|| {
		invalid(
			Reason::ParseFailed,
			"the data holds no font in a supported format".to_owned(),
		)
	})?;
	let charmap = font.charmap();
	let outlines = font.outline_glyphs();
	let glyph_metrics = font.glyph_metrics();
	for c in "Aa0 ".chars() {
		if let Some(gid) = charmap.map_unicode(c) {
			let _ = glyph_metrics.h_advance(gid);
			if let Some(g) = outlines.get(gid) {
				let _ = g.extents();
			}
		}
	}
	Ok(font)
}

/// The SHA-256 of `data` as lower-case hex.
pub fn sha256_hex(data: &[u8]) -> String {
	const K: [u32; 64] = [
		0x428a2f98, 0x71374491, 0xb5c0fbcf, 0xe9b5dba5, 0x3956c25b, 0x59f111f1, 0x923f82a4,
		0xab1c5ed5, 0xd807aa98, 0x12835b01, 0x243185be, 0x550c7dc3, 0x72be5d74, 0x80deb1fe,
		0x9bdc06a7, 0xc19bf174, 0xe49b69c1, 0xefbe4786, 0x0fc19dc6, 0x240ca1cc, 0x2de92c6f,
		0x4a7484aa, 0x5cb0a9dc, 0x76f988da, 0x983e5152, 0xa831c66d, 0xb00327c8, 0xbf597fc7,
		0xc6e00bf3, 0xd5a79147, 0x06ca6351, 0x14292967, 0x27b70a85, 0x2e1b2138, 0x4d2c6dfc,
		0x53380d13, 0x650a7354, 0x766a0abb, 0x81c2c92e, 0x92722c85, 0xa2bfe8a1, 0xa81a664b,
		0xc24b8b70, 0xc76c51a3, 0xd192e819, 0xd6990624, 0xf40e3585, 0x106aa070, 0x19a4c116,
		0x1e376c08, 0x2748774c, 0x34b0bcb5, 0x391c0cb3, 0x4ed8aa4a, 0x5b9cca4f, 0x682e6ff3,
		0x748f82ee, 0x78a5636f, 0x84c87814, 0x8cc70208, 0x90befffa, 0xa4506ceb, 0xbef9a3f7,
		0xc67178f2,
	];
	let mut h: [u32; 8] = [
		0x6a09e667, 0xbb67ae85, 0x3c6ef372, 0xa54ff53a, 0x510e527f, 0x9b05688c, 0x1f83d9ab,
		0x5be0cd19,
	];
	let mut msg = data.to_vec();
	msg.push(0x80);
	while msg.len() % 64 != 56 {
		msg.push(0);
	}
	msg.extend_from_slice(&((data.len() as u64) * 8).to_be_bytes());
	for chunk in msg.chunks(64) {
		let mut w = [0_u32; 64];
		for (i, b) in chunk.chunks(4).enumerate() {
			w[i] = u32::from_be_bytes([b[0], b[1], b[2], b[3]]);
		}
		for i in 16..64 {
			let s0 = w[i - 15].rotate_right(7) ^ w[i - 15].rotate_right(18) ^ (w[i - 15] >> 3);
			let s1 = w[i - 2].rotate_right(17) ^ w[i - 2].rotate_right(19) ^ (w[i - 2] >> 10);
			w[i] = w[i - 16]
				.wrapping_add(s0)
				.wrapping_add(w[i - 7])
				.wrapping_add(s1);
		}
		let mut v = h;
		for i in 0..64 {
			let s1 = v[4].rotate_right(6) ^ v[4].rotate_right(11) ^ v[4].rotate_right(25);
			let ch = (v[4] & v[5]) ^ (!v[4] & v[6]);
			let t1 = v[7]
				.wrapping_add(s1)
				.wrapping_add(ch)
				.wrapping_add(K[i])
				.wrapping_add(w[i]);
			let s0 = v[0].rotate_right(2) ^ v[0].rotate_right(13) ^ v[0].rotate_right(22);
			let maj = (v[0] & v[1]) ^ (v[0] & v[2]) ^ (v[1] & v[2]);
			let t2 = s0.wrapping_add(maj);
			v = [
				t1.wrapping_add(t2),
				v[0],
				v[1],
				v[2],
				v[3].wrapping_add(t1),
				v[4],
				v[5],
				v[6],
			];
		}
		for (x, y) in h.iter_mut().zip(v) {
			*x = x.wrapping_add(y);
		}
	}
	h.iter().map(|x| format!("{x:08x}")).collect()
}

#[cfg(test)]
mod tests {
	use super::*;

	#[test]
	fn sha256_of_known_inputs() {
		assert_eq!(
			sha256_hex(b""),
			"e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"
		);
		assert_eq!(
			sha256_hex(b"abc"),
			"ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad"
		);
		let long = vec![b'a'; 1000];
		assert_eq!(
			sha256_hex(&long),
			"41edece42d63e8d9bf515a9ba6932e1c20cbc9f5a5d134645adb5db1b9737ea3"
		);
	}

	/// A font file made of a directory and the given tables `(tag, offset, length)`, padded to `size`.
	fn directory(version: &[u8; 4], tables: &[(&[u8; 4], u32, u32)], size: usize) -> Vec<u8> {
		let mut d = version.to_vec();
		d.extend_from_slice(&(tables.len() as u16).to_be_bytes());
		d.extend_from_slice(&[0; 6]);
		for (tag, off, len) in tables {
			d.extend_from_slice(*tag);
			d.extend_from_slice(&[0; 4]);
			d.extend_from_slice(&off.to_be_bytes());
			d.extend_from_slice(&len.to_be_bytes());
		}
		d.resize(size, 0);
		d
	}

	fn required(first: u32) -> Vec<(&'static [u8; 4], u32, u32)> {
		let mut t = Vec::new();
		for (i, tag) in [b"cmap", b"head", b"hhea", b"hmtx", b"maxp", b"glyf"]
			.into_iter()
			.enumerate()
		{
			t.push((tag, first + 8 * i as u32, 8));
		}
		t
	}

	fn reason(data: &[u8]) -> Reason {
		check_font(data).unwrap_err().reason
	}

	#[test]
	fn the_structure_of_the_table_directory_is_checked() {
		let ok = directory(b"   ", &required(200), 400);
		// The directory is well formed, but the zeroed tables behind it are not a font.
		assert_eq!(reason(&ok), Reason::ParseFailed);

		let mut missing = required(200);
		missing.remove(3);
		assert_eq!(
			reason(&directory(b"   ", &missing, 400)),
			Reason::MissingTable
		);

		let mut no_outlines = required(200);
		no_outlines.pop();
		assert_eq!(
			reason(&directory(b"   ", &no_outlines, 400)),
			Reason::MissingTable
		);

		let mut dup = required(200);
		dup.push((b"cmap", 300, 8));
		assert_eq!(
			reason(&directory(b"   ", &dup, 400)),
			Reason::DuplicateTable
		);

		let mut over = required(200);
		over[1].1 = 204;
		assert_eq!(
			reason(&directory(b"   ", &over, 400)),
			Reason::TablesOverlap
		);

		let mut out = required(200);
		out[0].1 = 398;
		assert_eq!(
			reason(&directory(b"   ", &out, 400)),
			Reason::TableOutOfRange
		);

		assert_eq!(
			reason(&directory(b"   ", &required(200), 40)),
			Reason::Truncated
		);
	}

	#[test]
	fn too_many_tables_and_too_large_files_are_refused_before_reading_them() {
		let mut d = b"OTTO".to_vec();
		d.extend_from_slice(&65_u16.to_be_bytes());
		d.resize(64, 0);
		assert_eq!(reason(&d), Reason::TooManyTables);

		assert_eq!(reason(&vec![0; MAX_FONT_BYTES + 1]), Reason::TooLarge);
	}

	#[test]
	fn a_collection_is_checked_font_by_font() {
		let mut c = b"ttcf".to_vec();
		c.extend_from_slice(&[0, 1, 0, 0]);
		c.extend_from_slice(&1_u32.to_be_bytes());
		c.extend_from_slice(&100_u32.to_be_bytes());
		c.resize(200, 0);
		assert_eq!(reason(&c), Reason::BadMagic);

		let mut many = b"ttcf".to_vec();
		many.extend_from_slice(&[0, 1, 0, 0]);
		many.extend_from_slice(&65_u32.to_be_bytes());
		many.resize(400, 0);
		assert_eq!(reason(&many), Reason::TooManyTables);
	}

	#[test]
	fn a_bundled_font_is_valid() {
		let anton = include_bytes!("../../../renderer/fonts/Anton-Regular.ttf");
		assert!(check_font(anton).is_ok());
	}

	#[test]
	fn short_and_unknown_files_are_refused() {
		assert_eq!(check_font(&[0; 4]).unwrap_err().reason, Reason::Truncated);
		assert_eq!(
			check_font(b"NOTAFONTFILE").unwrap_err().reason,
			Reason::BadMagic
		);
	}
}
