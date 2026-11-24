//! The command line and the lines the helper writes. Version 1 of the protocol of SDD-024 section 5.

use std::fmt::Write as _;

pub const PROTOCOL: u32 = 1;

/// The largest frame the helper accepts, in pixels per side, and the most it holds in memory.
pub const MAX_SIDE: usize = 4096;

#[derive(Debug, PartialEq, Eq)]
pub enum Command {
	/// Loads the runtime and says which execution providers it offers.
	Probe,
	/// Detects faces in raw RGB24 frames read from standard input.
	Faces,
}

#[derive(Debug, PartialEq, Eq, Clone, Copy)]
pub enum Provider {
	Auto,
	Cpu,
	DirectMl,
}

#[derive(Debug)]
pub struct Args {
	pub command: Command,
	pub runtime: String,
	pub model: String,
	pub width: usize,
	pub height: usize,
	pub provider: Provider,
	/// Minimum score in thousandths.
	pub min_score_milli: u32,
	pub threads: usize,
}

/// A failure with a stable code, a message with no path and no secret, and an exit code of 1.
#[derive(Debug)]
pub struct HelperError {
	pub code: &'static str,
	pub message: String,
}

impl HelperError {
	pub fn new(code: &'static str, message: impl Into<String>) -> Self {
		Self {
			code,
			message: message.into(),
		}
	}
}

impl Args {
	pub fn parse(argv: &[String]) -> Result<Args, HelperError> {
		let mut it = argv.iter();
		let command = match it.next().map(String::as_str) {
			Some("probe") => Command::Probe,
			Some("faces") => Command::Faces,
			_ => {
				return Err(HelperError::new(
					"usage",
					"the first argument is probe or faces",
				));
			}
		};
		let mut args = Args {
			command,
			runtime: String::new(),
			model: String::new(),
			width: 0,
			height: 0,
			provider: Provider::Auto,
			min_score_milli: 700,
			threads: 0,
		};
		while let Some(flag) = it.next() {
			let value = it
				.next()
				.ok_or_else(|| HelperError::new("usage", format!("{flag} needs a value")))?;
			match flag.as_str() {
				"--runtime" => args.runtime = value.clone(),
				"--model" => args.model = value.clone(),
				"--width" => args.width = number(flag, value)?,
				"--height" => args.height = number(flag, value)?,
				"--min-score-milli" => args.min_score_milli = number::<u32>(flag, value)?,
				"--threads" => args.threads = number(flag, value)?,
				"--provider" => {
					args.provider = match value.as_str() {
						"auto" => Provider::Auto,
						"cpu" => Provider::Cpu,
						"directml" => Provider::DirectMl,
						_ => {
							return Err(HelperError::new(
								"usage",
								"--provider is auto, cpu or directml",
							));
						}
					}
				}
				_ => return Err(HelperError::new("usage", format!("unknown option {flag}"))),
			}
		}
		if args.runtime.is_empty() {
			return Err(HelperError::new("usage", "--runtime is required"));
		}
		if args.command == Command::Faces {
			if args.model.is_empty() {
				return Err(HelperError::new("usage", "--model is required"));
			}
			let ok = |v: usize| (32..=MAX_SIDE).contains(&v) && v.is_multiple_of(32);
			if !ok(args.width) || !ok(args.height) {
				return Err(HelperError::new(
					"frame_size",
					"--width and --height are multiples of 32 from 32 to 4096",
				));
			}
			if args.min_score_milli > 1000 {
				return Err(HelperError::new(
					"usage",
					"--min-score-milli is at most 1000",
				));
			}
		}
		Ok(args)
	}
}

fn number<T: std::str::FromStr>(flag: &str, value: &str) -> Result<T, HelperError> {
	value
		.parse()
		.map_err(|_| HelperError::new("usage", format!("{flag} is not a number")))
}

/// Escapes a string for a JSON string literal.
pub fn json_string(s: &str) -> String {
	let mut out = String::with_capacity(s.len() + 2);
	out.push('"');
	for c in s.chars() {
		match c {
			'"' => out.push_str("\\\""),
			'\\' => out.push_str("\\\\"),
			'\n' => out.push_str("\\n"),
			'\r' => out.push_str("\\r"),
			'\t' => out.push_str("\\t"),
			c if (c as u32) < 0x20 => {
				let _ = write!(out, "\\u{:04x}", c as u32);
			}
			c => out.push(c),
		}
	}
	out.push('"');
	out
}

pub struct ErrorLine<'a> {
	code: &'a str,
	message: &'a str,
}

impl<'a> From<&'a HelperError> for ErrorLine<'a> {
	fn from(e: &'a HelperError) -> Self {
		Self {
			code: e.code,
			message: &e.message,
		}
	}
}

impl ErrorLine<'_> {
	pub fn to_json(&self) -> String {
		format!(
			r#"{{"type":"error","code":{},"message":{}}}"#,
			json_string(self.code),
			json_string(self.message)
		)
	}
}

#[cfg(test)]
mod tests {
	use super::*;

	fn v(parts: &[&str]) -> Vec<String> {
		parts.iter().map(|s| (*s).to_string()).collect()
	}

	#[test]
	fn parses_a_faces_command() {
		let a = Args::parse(&v(&[
			"faces",
			"--runtime",
			"a.dll",
			"--model",
			"m.onnx",
			"--width",
			"640",
			"--height",
			"384",
			"--provider",
			"cpu",
		]))
		.unwrap();
		assert_eq!(a.command, Command::Faces);
		assert_eq!((a.width, a.height, a.provider), (640, 384, Provider::Cpu));
		assert_eq!(a.min_score_milli, 700);
	}

	#[test]
	fn refuses_sizes_that_are_not_multiples_of_32() {
		let e = Args::parse(&v(&[
			"faces",
			"--runtime",
			"a",
			"--model",
			"m",
			"--width",
			"641",
			"--height",
			"384",
		]))
		.unwrap_err();
		assert_eq!(e.code, "frame_size");
	}

	#[test]
	fn refuses_huge_frames_and_unknown_options() {
		let e = Args::parse(&v(&[
			"faces",
			"--runtime",
			"a",
			"--model",
			"m",
			"--width",
			"8192",
			"--height",
			"384",
		]))
		.unwrap_err();
		assert_eq!(e.code, "frame_size");
		let e = Args::parse(&v(&["probe", "--runtime", "a", "--bogus", "1"])).unwrap_err();
		assert_eq!(e.code, "usage");
		assert_eq!(Args::parse(&v(&[])).unwrap_err().code, "usage");
	}

	#[test]
	fn probe_needs_only_the_runtime() {
		assert!(Args::parse(&v(&["probe", "--runtime", "a"])).is_ok());
		assert!(Args::parse(&v(&["probe"])).is_err());
	}

	#[test]
	fn json_strings_are_escaped() {
		assert_eq!(json_string("a\"b\\c\n\u{1}"), r#""a\"b\\c\n\u0001""#);
	}
}
