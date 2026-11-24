//! A small JSON reader and writer over std only (ADR-0049 approves no JSON crate). It reads what the host sends
//! and writes what the helper answers: flat objects, arrays of objects, strings, numbers and booleans. It is
//! strict (no trailing text, no comments) and bounded (depth, string and element counts).

use std::fmt::Write as _;

const MAX_DEPTH: usize = 8;
const MAX_ITEMS: usize = 4096;

#[derive(Debug, Clone, PartialEq)]
pub enum Json {
	Null,
	Bool(bool),
	Num(f64),
	Str(String),
	Arr(Vec<Json>),
	Obj(Vec<(String, Json)>),
}

impl Json {
	pub fn get(&self, key: &str) -> Option<&Json> {
		match self {
			Json::Obj(items) => items.iter().find(|(k, _)| k == key).map(|(_, v)| v),
			_ => None,
		}
	}

	pub fn as_str(&self) -> Option<&str> {
		match self {
			Json::Str(s) => Some(s),
			_ => None,
		}
	}

	/// The number as an unsigned integer, when it is one.
	pub fn as_u64(&self) -> Option<u64> {
		match self {
			Json::Num(n) if n.is_finite() && *n >= 0.0 && n.fract() == 0.0 && *n <= 9.0e15 => {
				Some(*n as u64)
			}
			_ => None,
		}
	}
}

pub fn parse(text: &str) -> Result<Json, String> {
	let mut p = Parser {
		s: text.as_bytes(),
		i: 0,
	};
	let v = p.value(0)?;
	p.ws();
	if p.i != p.s.len() {
		return Err("text after the value".into());
	}
	Ok(v)
}

struct Parser<'a> {
	s: &'a [u8],
	i: usize,
}

impl Parser<'_> {
	fn ws(&mut self) {
		while matches!(self.s.get(self.i), Some(b' ' | b'\t' | b'\r' | b'\n')) {
			self.i += 1;
		}
	}

	fn eat(&mut self, c: u8) -> Result<(), String> {
		if self.s.get(self.i) == Some(&c) {
			self.i += 1;
			Ok(())
		} else {
			Err(format!("expected {:?} at {}", c as char, self.i))
		}
	}

	fn value(&mut self, depth: usize) -> Result<Json, String> {
		if depth > MAX_DEPTH {
			return Err("nested too deep".into());
		}
		self.ws();
		match self.s.get(self.i) {
			Some(b'{') => self.object(depth),
			Some(b'[') => self.array(depth),
			Some(b'"') => Ok(Json::Str(self.string()?)),
			Some(b't') => self.word("true", Json::Bool(true)),
			Some(b'f') => self.word("false", Json::Bool(false)),
			Some(b'n') => self.word("null", Json::Null),
			Some(b'-' | b'0'..=b'9') => self.number(),
			_ => Err(format!("unexpected input at {}", self.i)),
		}
	}

	fn word(&mut self, w: &str, v: Json) -> Result<Json, String> {
		if self.s[self.i..].starts_with(w.as_bytes()) {
			self.i += w.len();
			Ok(v)
		} else {
			Err(format!("unexpected input at {}", self.i))
		}
	}

	fn number(&mut self) -> Result<Json, String> {
		let start = self.i;
		while matches!(
			self.s.get(self.i),
			Some(b'-' | b'+' | b'.' | b'e' | b'E' | b'0'..=b'9')
		) {
			self.i += 1;
		}
		let t = std::str::from_utf8(&self.s[start..self.i]).map_err(|e| e.to_string())?;
		t.parse::<f64>()
			.map(Json::Num)
			.map_err(|_| format!("bad number at {start}"))
	}

	fn string(&mut self) -> Result<String, String> {
		self.eat(b'"')?;
		let mut out: Vec<u8> = Vec::new();
		loop {
			let c = *self.s.get(self.i).ok_or("unterminated string")?;
			self.i += 1;
			match c {
				b'"' => break,
				b'\\' => {
					let e = *self.s.get(self.i).ok_or("unterminated escape")?;
					self.i += 1;
					let ch = match e {
						b'"' => '"',
						b'\\' => '\\',
						b'/' => '/',
						b'b' => '\u{8}',
						b'f' => '\u{c}',
						b'n' => '\n',
						b'r' => '\r',
						b't' => '\t',
						b'u' => self.unicode()?,
						_ => return Err("bad escape".into()),
					};
					let mut buf = [0u8; 4];
					out.extend_from_slice(ch.encode_utf8(&mut buf).as_bytes());
				}
				0..=0x1f => return Err("control character in a string".into()),
				_ => out.push(c),
			}
		}
		String::from_utf8(out).map_err(|_| "string is not UTF-8".into())
	}

	fn hex4(&mut self) -> Result<u32, String> {
		let h = self
			.s
			.get(self.i..self.i + 4)
			.ok_or("short unicode escape")?;
		let t = std::str::from_utf8(h).map_err(|e| e.to_string())?;
		self.i += 4;
		u32::from_str_radix(t, 16).map_err(|_| "bad unicode escape".to_string())
	}

	fn unicode(&mut self) -> Result<char, String> {
		let hi = self.hex4()?;
		let cp = if (0xD800..0xDC00).contains(&hi) {
			if self.s.get(self.i..self.i + 2) != Some(b"\\u") {
				return Err("lone surrogate".into());
			}
			self.i += 2;
			let lo = self.hex4()?;
			if !(0xDC00..0xE000).contains(&lo) {
				return Err("lone surrogate".into());
			}
			0x10000 + ((hi - 0xD800) << 10) + (lo - 0xDC00)
		} else {
			hi
		};
		char::from_u32(cp).ok_or_else(|| "bad code point".to_string())
	}

	fn array(&mut self, depth: usize) -> Result<Json, String> {
		self.eat(b'[')?;
		let mut items = Vec::new();
		self.ws();
		if self.s.get(self.i) == Some(&b']') {
			self.i += 1;
			return Ok(Json::Arr(items));
		}
		loop {
			if items.len() >= MAX_ITEMS {
				return Err("too many elements".into());
			}
			items.push(self.value(depth + 1)?);
			self.ws();
			match self.s.get(self.i) {
				Some(b',') => self.i += 1,
				Some(b']') => {
					self.i += 1;
					return Ok(Json::Arr(items));
				}
				_ => return Err(format!("expected , or ] at {}", self.i)),
			}
		}
	}

	fn object(&mut self, depth: usize) -> Result<Json, String> {
		self.eat(b'{')?;
		let mut items = Vec::new();
		self.ws();
		if self.s.get(self.i) == Some(&b'}') {
			self.i += 1;
			return Ok(Json::Obj(items));
		}
		loop {
			if items.len() >= MAX_ITEMS {
				return Err("too many members".into());
			}
			self.ws();
			let k = self.string()?;
			self.ws();
			self.eat(b':')?;
			let v = self.value(depth + 1)?;
			items.push((k, v));
			self.ws();
			match self.s.get(self.i) {
				Some(b',') => self.i += 1,
				Some(b'}') => {
					self.i += 1;
					return Ok(Json::Obj(items));
				}
				_ => return Err(format!("expected , or }} at {}", self.i)),
			}
		}
	}
}

/// Appends s as a JSON string, escaping what JSON requires and every non-printable character.
pub fn write_str(out: &mut String, s: &str) {
	out.push('"');
	for c in s.chars() {
		match c {
			'"' => out.push_str("\\\""),
			'\\' => out.push_str("\\\\"),
			'\n' => out.push_str("\\n"),
			'\r' => out.push_str("\\r"),
			'\t' => out.push_str("\\t"),
			c if (c as u32) < 0x20 || c == '\u{7f}' => {
				let _ = write!(out, "\\u{:04x}", c as u32);
			}
			c => out.push(c),
		}
	}
	out.push('"');
}

/// Writes a JSON object from members whose values are already JSON text.
pub fn object(members: &[(&str, String)]) -> String {
	let mut out = String::from("{");
	for (i, (k, v)) in members.iter().enumerate() {
		if i > 0 {
			out.push(',');
		}
		write_str(&mut out, k);
		out.push(':');
		out.push_str(v);
	}
	out.push('}');
	out
}

pub fn string(s: &str) -> String {
	let mut out = String::new();
	write_str(&mut out, s);
	out
}

#[cfg(test)]
mod tests {
	use super::*;

	#[test]
	fn reads_a_message() {
		let v = parse(
			r#" {"type":"start","fps":30,"source":{"kind":"display","id":"d1"},"x":[1,true,null]} "#,
		)
		.unwrap();
		assert_eq!(v.get("type").unwrap().as_str(), Some("start"));
		assert_eq!(v.get("fps").unwrap().as_u64(), Some(30));
		assert_eq!(
			v.get("source").unwrap().get("id").unwrap().as_str(),
			Some("d1")
		);
		assert!(v.get("missing").is_none());
	}

	#[test]
	fn reads_escapes_and_surrogates() {
		let v = parse(r#""a\n\u00e9\ud83d\ude00\"""#).unwrap();
		assert_eq!(v.as_str(), Some("a\n\u{e9}\u{1f600}\""));
	}

	#[test]
	fn refuses_what_is_not_json() {
		for bad in [
			"",
			"{",
			"{\"a\":}",
			"{\"a\":1,}",
			"[1,]",
			"{\"a\":1} x",
			"\"\u{1}\"",
			"\"\\ud800\"",
			"tru",
			"{'a':1}",
			"01x",
		] {
			assert!(parse(bad).is_err(), "{bad:?} must be refused");
		}
	}

	#[test]
	fn refuses_deep_nesting_without_overflowing() {
		let deep = "[".repeat(10_000);
		assert!(parse(&deep).is_err());
	}

	#[test]
	fn only_whole_non_negative_numbers_are_unsigned() {
		assert_eq!(parse("1.5").unwrap().as_u64(), None);
		assert_eq!(parse("-1").unwrap().as_u64(), None);
		assert_eq!(parse("1e3").unwrap().as_u64(), Some(1000));
	}

	#[test]
	fn writes_valid_escaped_json() {
		let text = object(&[("a", string("x\"y\\\n\u{1}")), ("n", "3".into())]);
		assert_eq!(text, r#"{"a":"x\"y\\\n\u0001","n":3}"#);
		assert!(parse(&text).is_ok());
	}
}
