//! A strict JSON reader for the render input (SDD-013 section 2.1).
//!
//! RFC 8259, UTF-8 only, no extension: no comments, no trailing comma, no `NaN`, no duplicate key
//! (an ambiguous object is refused instead of resolved), a nesting depth of at most [`MAX_DEPTH`]
//! and a number that must fit a double. It never panics and never allocates more than the input
//! length times a small constant.

/// The deepest nesting the reader accepts; a render input is four levels deep.
pub const MAX_DEPTH: usize = 32;

/// A JSON value. Objects keep the order of the text.
#[derive(Clone, Debug, PartialEq)]
pub enum Value {
	Null,
	Bool(bool),
	Num(f64),
	Str(String),
	Arr(Vec<Value>),
	Obj(Vec<(String, Value)>),
}

/// Why a text is not JSON of the form the renderer reads.
#[derive(Clone, Copy, Debug, PartialEq, Eq)]
pub enum JsonError {
	/// The text is not UTF-8.
	Utf8,
	/// The text is not valid JSON at this byte offset.
	Syntax(usize),
	/// An object repeats a key, at this byte offset.
	DuplicateKey(usize),
	/// A number is outside the range of a double, at this byte offset.
	Range(usize),
	/// The nesting is deeper than [`MAX_DEPTH`].
	Depth,
}

/// Reads one JSON document; anything after it but white space is an error.
pub fn parse(input: &[u8]) -> Result<Value, JsonError> {
	let text = core::str::from_utf8(input).map_err(|_| JsonError::Utf8)?;
	let mut p = Parser {
		b: text.as_bytes(),
		text,
		i: 0,
	};
	p.ws();
	let v = p.value(0)?;
	p.ws();
	if p.i != p.b.len() {
		return Err(JsonError::Syntax(p.i));
	}
	Ok(v)
}

struct Parser<'a> {
	b: &'a [u8],
	text: &'a str,
	i: usize,
}

impl Parser<'_> {
	fn ws(&mut self) {
		while matches!(self.b.get(self.i), Some(b' ' | b'\t' | b'\n' | b'\r')) {
			self.i += 1;
		}
	}

	fn err<T>(&self) -> Result<T, JsonError> {
		Err(JsonError::Syntax(self.i))
	}

	fn eat(&mut self, c: u8) -> Result<(), JsonError> {
		if self.b.get(self.i) == Some(&c) {
			self.i += 1;
			Ok(())
		} else {
			self.err()
		}
	}

	fn literal(&mut self, word: &str, v: Value) -> Result<Value, JsonError> {
		if self.b[self.i..].starts_with(word.as_bytes()) {
			self.i += word.len();
			Ok(v)
		} else {
			self.err()
		}
	}

	fn value(&mut self, depth: usize) -> Result<Value, JsonError> {
		if depth > MAX_DEPTH {
			return Err(JsonError::Depth);
		}
		match self.b.get(self.i) {
			Some(b'{') => self.object(depth),
			Some(b'[') => self.array(depth),
			Some(b'"') => Ok(Value::Str(self.string()?)),
			Some(b't') => self.literal("true", Value::Bool(true)),
			Some(b'f') => self.literal("false", Value::Bool(false)),
			Some(b'n') => self.literal("null", Value::Null),
			Some(b'-' | b'0'..=b'9') => self.number(),
			_ => self.err(),
		}
	}

	fn object(&mut self, depth: usize) -> Result<Value, JsonError> {
		self.eat(b'{')?;
		let mut out: Vec<(String, Value)> = Vec::new();
		self.ws();
		if self.b.get(self.i) == Some(&b'}') {
			self.i += 1;
			return Ok(Value::Obj(out));
		}
		loop {
			self.ws();
			let at = self.i;
			let key = self.string()?;
			if out.iter().any(|(k, _)| *k == key) {
				return Err(JsonError::DuplicateKey(at));
			}
			self.ws();
			self.eat(b':')?;
			self.ws();
			let v = self.value(depth + 1)?;
			out.push((key, v));
			self.ws();
			match self.b.get(self.i) {
				Some(b',') => self.i += 1,
				Some(b'}') => {
					self.i += 1;
					return Ok(Value::Obj(out));
				}
				_ => return self.err(),
			}
		}
	}

	fn array(&mut self, depth: usize) -> Result<Value, JsonError> {
		self.eat(b'[')?;
		let mut out = Vec::new();
		self.ws();
		if self.b.get(self.i) == Some(&b']') {
			self.i += 1;
			return Ok(Value::Arr(out));
		}
		loop {
			self.ws();
			out.push(self.value(depth + 1)?);
			self.ws();
			match self.b.get(self.i) {
				Some(b',') => self.i += 1,
				Some(b']') => {
					self.i += 1;
					return Ok(Value::Arr(out));
				}
				_ => return self.err(),
			}
		}
	}

	fn number(&mut self) -> Result<Value, JsonError> {
		let start = self.i;
		if self.b.get(self.i) == Some(&b'-') {
			self.i += 1;
		}
		match self.b.get(self.i) {
			Some(b'0') => self.i += 1,
			Some(b'1'..=b'9') => self.digits(),
			_ => return self.err(),
		}
		if self.b.get(self.i) == Some(&b'.') {
			self.i += 1;
			if !matches!(self.b.get(self.i), Some(b'0'..=b'9')) {
				return self.err();
			}
			self.digits();
		}
		if matches!(self.b.get(self.i), Some(b'e' | b'E')) {
			self.i += 1;
			if matches!(self.b.get(self.i), Some(b'+' | b'-')) {
				self.i += 1;
			}
			if !matches!(self.b.get(self.i), Some(b'0'..=b'9')) {
				return self.err();
			}
			self.digits();
		}
		// The token is ASCII by the grammar above, so the slice is on character boundaries.
		match self.text[start..self.i].parse::<f64>() {
			Ok(v) if v.is_finite() => Ok(Value::Num(v)),
			_ => Err(JsonError::Range(start)),
		}
	}

	fn digits(&mut self) {
		while matches!(self.b.get(self.i), Some(b'0'..=b'9')) {
			self.i += 1;
		}
	}

	fn hex4(&mut self) -> Result<u32, JsonError> {
		let mut v = 0_u32;
		for _ in 0..4 {
			let d = match self.b.get(self.i) {
				Some(c @ b'0'..=b'9') => c - b'0',
				Some(c @ b'a'..=b'f') => c - b'a' + 10,
				Some(c @ b'A'..=b'F') => c - b'A' + 10,
				_ => return self.err(),
			};
			v = v << 4 | u32::from(d);
			self.i += 1;
		}
		Ok(v)
	}

	fn string(&mut self) -> Result<String, JsonError> {
		self.eat(b'"')?;
		let mut out = String::new();
		loop {
			let Some(&c) = self.b.get(self.i) else {
				return self.err();
			};
			match c {
				b'"' => {
					self.i += 1;
					return Ok(out);
				}
				b'\\' => {
					self.i += 1;
					let Some(&e) = self.b.get(self.i) else {
						return self.err();
					};
					self.i += 1;
					match e {
						b'"' => out.push('"'),
						b'\\' => out.push('\\'),
						b'/' => out.push('/'),
						b'b' => out.push('\u{8}'),
						b'f' => out.push('\u{c}'),
						b'n' => out.push('\n'),
						b'r' => out.push('\r'),
						b't' => out.push('\t'),
						b'u' => {
							let hi = self.hex4()?;
							let cp = if (0xD800..0xDC00).contains(&hi) {
								if self.b.get(self.i) != Some(&b'\\')
									|| self.b.get(self.i + 1) != Some(&b'u')
								{
									return self.err();
								}
								self.i += 2;
								let lo = self.hex4()?;
								if !(0xDC00..0xE000).contains(&lo) {
									return self.err();
								}
								0x10000 + ((hi - 0xD800) << 10) + (lo - 0xDC00)
							} else {
								hi
							};
							match char::from_u32(cp) {
								Some(ch) => out.push(ch),
								None => return self.err(),
							}
						}
						_ => return self.err(),
					}
				}
				0..=0x1F => return self.err(),
				_ => {
					// A whole UTF-8 character: the input was validated as UTF-8, and the offset is
					// always on a boundary here because every other branch consumed ASCII.
					let ch = self.text[self.i..].chars().next();
					match ch {
						Some(ch) => {
							out.push(ch);
							self.i += ch.len_utf8();
						}
						None => return self.err(),
					}
				}
			}
		}
	}
}

#[cfg(test)]
mod tests {
	use super::*;

	fn ok(s: &str) -> Value {
		parse(s.as_bytes()).unwrap_or_else(|e| panic!("{s}: {e:?}"))
	}

	#[test]
	fn reads_every_kind_of_value() {
		assert_eq!(ok(" null "), Value::Null);
		assert_eq!(ok("true"), Value::Bool(true));
		assert_eq!(ok("-0.5e1"), Value::Num(-5.0));
		assert_eq!(
			ok("\"a\\u00e9\\n\\ud83d\\ude00\""),
			Value::Str("a\u{e9}\n\u{1f600}".into())
		);
		assert_eq!(
			ok("{\"a\":[1,2],\"b\":{}}"),
			Value::Obj(vec![
				(
					"a".into(),
					Value::Arr(vec![Value::Num(1.0), Value::Num(2.0)])
				),
				("b".into(), Value::Obj(vec![]))
			])
		);
	}

	#[test]
	fn numbers_are_correctly_rounded() {
		assert_eq!(ok("0.1"), Value::Num(0.1));
		assert_eq!(ok("123456789.123"), Value::Num(123_456_789.123));
	}

	#[test]
	fn refuses_what_is_not_json() {
		for s in [
			"",
			"{",
			"[1,]",
			"{\"a\":1,}",
			"01",
			"1.",
			".5",
			"+1",
			"NaN",
			"Infinity",
			"-",
			"1e",
			"'a'",
			"\"a",
			"\"\\x\"",
			"\"\\ud800\"",
			"\"\\udc00\"",
			"\"a\nb\"",
			"[] []",
			"tru",
			"{\"a\" 1}",
			"{1:2}",
		] {
			assert!(parse(s.as_bytes()).is_err(), "{s:?} must be refused");
		}
	}

	#[test]
	fn refuses_ambiguity_and_excess() {
		assert!(matches!(
			parse(b"{\"a\":1,\"a\":2}"),
			Err(JsonError::DuplicateKey(_))
		));
		assert!(matches!(parse(b"1e999"), Err(JsonError::Range(_))));
		assert_eq!(parse(&[0xff, 0xfe]), Err(JsonError::Utf8));
		let deep = "[".repeat(MAX_DEPTH + 2) + &"]".repeat(MAX_DEPTH + 2);
		assert_eq!(parse(deep.as_bytes()), Err(JsonError::Depth));
		let fine = "[".repeat(MAX_DEPTH) + &"]".repeat(MAX_DEPTH);
		assert!(parse(fine.as_bytes()).is_ok());
	}
}
