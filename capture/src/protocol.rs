//! The control protocol, version 1 (SDD-023 section 6): one JSON object per line, at most 65,536 bytes.

use std::io::BufRead;
use std::path::PathBuf;

use crate::json::{self, Json};

pub const VERSION: u64 = 1;
pub const MAX_LINE: usize = 65_536;
pub const FRAME_RATES: [u32; 3] = [24, 30, 60];
pub const ENCODERS: [&str; 5] = [
	"h264_nvenc",
	"h264_qsv",
	"h264_amf",
	"h264_mf",
	"libopenh264",
];

/// What the host asks for. Every field is validated before anything starts.
#[derive(Debug, Clone, PartialEq)]
pub struct StartRequest {
	pub recording: String,
	pub kind: String,
	pub source_id: String,
	pub fps: u32,
	pub encoder: String,
	pub ffmpeg: PathBuf,
	pub output: PathBuf,
}

#[derive(Debug, PartialEq)]
pub enum Command {
	Sources,
	Start(Box<StartRequest>),
	Stop,
}

/// The extra values only a test build of the host may use (the flag is never passed by the server).
#[derive(Debug, Clone, Copy, Default)]
pub struct Policy {
	pub allow_test_source: bool,
}

pub fn parse_command(line: &str, policy: Policy) -> Result<Command, String> {
	let v = json::parse(line)?;
	match v.get("type").and_then(Json::as_str) {
		Some("sources") => Ok(Command::Sources),
		Some("stop") => Ok(Command::Stop),
		Some("start") => parse_start(&v, policy).map(|s| Command::Start(Box::new(s))),
		Some(other) => Err(format!("unknown message type {}", printable(other))),
		None => Err("the message has no type".into()),
	}
}

fn parse_start(v: &Json, policy: Policy) -> Result<StartRequest, String> {
	let text = |key: &str| -> Result<&str, String> {
		v.get(key)
			.and_then(Json::as_str)
			.ok_or_else(|| format!("{key} is required"))
	};
	let recording = text("recording")?;
	if recording.is_empty()
		|| recording.len() > 32
		|| !recording
			.bytes()
			.all(|b| b.is_ascii_alphanumeric() || b == b'_')
	{
		return Err("recording is not an identifier".into());
	}
	let source = v.get("source").ok_or("source is required")?;
	let kind = source
		.get("kind")
		.and_then(Json::as_str)
		.ok_or("source.kind is required")?;
	let allowed_kind =
		matches!(kind, "display" | "window") || (policy.allow_test_source && kind == "fake");
	if !allowed_kind {
		return Err("source.kind is not display or window".into());
	}
	let source_id = source
		.get("id")
		.and_then(Json::as_str)
		.ok_or("source.id is required")?;
	if source_id.is_empty()
		|| source_id.len() > 64
		|| !source_id
			.bytes()
			.all(|b| b.is_ascii_alphanumeric() || b == b'-' || b == b'_')
	{
		return Err("source.id is not an identifier".into());
	}
	let fps = v
		.get("fps")
		.and_then(Json::as_u64)
		.ok_or("fps is required")?;
	let fps = u32::try_from(fps)
		.ok()
		.filter(|f| FRAME_RATES.contains(f))
		.ok_or("fps is not 24, 30 or 60")?;
	let encoder = text("encoder")?;
	let encoder_ok = ENCODERS.contains(&encoder) || (policy.allow_test_source && encoder == "raw");
	if !encoder_ok {
		return Err("encoder is not in the closed list".into());
	}
	let ffmpeg = PathBuf::from(text("ffmpeg")?);
	let output = PathBuf::from(text("output")?);
	if !ffmpeg.is_absolute() {
		return Err("ffmpeg must be an absolute path".into());
	}
	let ext = if encoder == "raw" { "raw" } else { "mp4" };
	if !output.is_absolute() || output.extension().and_then(|e| e.to_str()) != Some(ext) {
		return Err(format!("output must be an absolute path ending in .{ext}"));
	}
	if !output.parent().is_some_and(|p| p.is_dir()) {
		return Err("the folder of output does not exist".into());
	}
	Ok(StartRequest {
		recording: recording.to_string(),
		kind: kind.to_string(),
		source_id: source_id.to_string(),
		fps,
		encoder: encoder.to_string(),
		ffmpeg,
		output,
	})
}

/// Text that is safe to put in a message: printable ASCII and at most 200 characters.
pub fn printable(s: &str) -> String {
	s.chars()
		.map(|c| {
			if c.is_ascii_graphic() || c == ' ' {
				c
			} else {
				'?'
			}
		})
		.take(200)
		.collect()
}

/// The outcome of reading one line.
#[derive(Debug, PartialEq)]
pub enum Line {
	Text(String),
	TooLong,
	Eof,
}

/// Reads one line of at most MAX_LINE bytes. A longer line is discarded to its end and reported as TooLong, so
/// the next read starts on a new message.
pub fn read_line(r: &mut impl BufRead) -> std::io::Result<Line> {
	let mut buf: Vec<u8> = Vec::new();
	let mut too_long = false;
	loop {
		let chunk = r.fill_buf()?;
		if chunk.is_empty() {
			return Ok(if buf.is_empty() && !too_long {
				Line::Eof
			} else {
				finish(buf, too_long)
			});
		}
		let (take, done) = match chunk.iter().position(|&b| b == b'\n') {
			Some(i) => (i + 1, true),
			None => (chunk.len(), false),
		};
		if !too_long {
			if buf.len() + take > MAX_LINE + 1 {
				too_long = true;
				buf.clear();
			} else {
				buf.extend_from_slice(&chunk[..take]);
			}
		}
		r.consume(take);
		if done {
			return Ok(finish(buf, too_long));
		}
	}
}

fn finish(buf: Vec<u8>, too_long: bool) -> Line {
	if too_long {
		return Line::TooLong;
	}
	let text = String::from_utf8_lossy(&buf);
	Line::Text(text.trim_end_matches(['\n', '\r']).to_string())
}

pub fn ready(os: &str, display: bool, window: bool) -> String {
	json::object(&[
		("type", json::string("ready")),
		("v", VERSION.to_string()),
		("helper", json::string(env!("CARGO_PKG_VERSION"))),
		("os", json::string(os)),
		("display", display.to_string()),
		("window", window.to_string()),
	])
}

pub fn error(code: &str, message: &str) -> String {
	json::object(&[
		("type", json::string("error")),
		("code", json::string(code)),
		("message", json::string(&printable(message))),
	])
}

pub fn started(width: u32, height: u32, fps: u32) -> String {
	json::object(&[
		("type", json::string("started")),
		("width", width.to_string()),
		("height", height.to_string()),
		("fps", fps.to_string()),
	])
}

pub fn progress(elapsed_ms: u64, frames: u64, dropped: u64, bytes: u64) -> String {
	json::object(&[
		("type", json::string("progress")),
		("elapsed_ms", elapsed_ms.to_string()),
		("frames", frames.to_string()),
		("dropped", dropped.to_string()),
		("bytes", bytes.to_string()),
	])
}

pub fn finished(reason: &str, frames: u64, dropped: u64, duration_ms: u64, bytes: u64) -> String {
	json::object(&[
		("type", json::string("finished")),
		("reason", json::string(reason)),
		("frames", frames.to_string()),
		("dropped", dropped.to_string()),
		("duration_ms", duration_ms.to_string()),
		("bytes", bytes.to_string()),
	])
}

/// One entry of a sources message.
pub struct SourceInfo {
	pub id: String,
	pub name: String,
	pub width: u32,
	pub height: u32,
}

pub fn sources(displays: &[SourceInfo], windows: &[SourceInfo]) -> String {
	let list = |items: &[SourceInfo]| {
		let parts: Vec<String> = items
			.iter()
			.map(|s| {
				json::object(&[
					("id", json::string(&s.id)),
					("name", json::string(&s.name)),
					("width", s.width.to_string()),
					("height", s.height.to_string()),
				])
			})
			.collect();
		format!("[{}]", parts.join(","))
	};
	json::object(&[
		("type", json::string("sources")),
		("displays", list(displays)),
		("windows", list(windows)),
	])
}

#[cfg(test)]
mod tests {
	use super::*;
	use std::io::Cursor;

	fn start_line(dir: &std::path::Path, extra: &str) -> String {
		let out = dir.join("c.mp4");
		let ff = std::env::current_exe().unwrap();
		format!(
			r#"{{"type":"start","recording":"rec_abc","source":{{"kind":"display","id":"d1"}},"fps":30,"encoder":"h264_nvenc","ffmpeg":{},"output":{}{extra}}}"#,
			json::string(&ff.to_string_lossy()),
			json::string(&out.to_string_lossy()),
		)
	}

	#[test]
	fn parses_every_command() {
		let dir = std::env::temp_dir();
		assert_eq!(
			parse_command(r#"{"type":"sources"}"#, Policy::default()),
			Ok(Command::Sources)
		);
		assert_eq!(
			parse_command(r#"{"type":"stop","x":1}"#, Policy::default()),
			Ok(Command::Stop)
		);
		match parse_command(&start_line(&dir, ""), Policy::default()) {
			Ok(Command::Start(s)) => {
				assert_eq!(
					(s.fps, s.encoder.as_str(), s.kind.as_str()),
					(30, "h264_nvenc", "display")
				);
			}
			other => panic!("{other:?}"),
		}
	}

	#[test]
	fn refuses_bad_messages_with_a_reason() {
		let p = Policy::default();
		for bad in [
			"",
			"nope",
			"[]",
			r#"{"type":"jump"}"#,
			r#"{"type":"start"}"#,
			r#"{"type":7}"#,
		] {
			assert!(parse_command(bad, p).is_err(), "{bad:?}");
		}
	}

	#[test]
	fn start_is_validated_field_by_field() {
		let dir = std::env::temp_dir();
		let good = start_line(&dir, "");
		let p = Policy::default();
		let cases = [
			(good.replace("\"fps\":30", "\"fps\":59"), "fps"),
			(good.replace("h264_nvenc", "libx264"), "encoder"),
			(good.replace("\"display\"", "\"camera\""), "source.kind"),
			(good.replace("\"d1\"", "\"a b\""), "source.id"),
			(good.replace("rec_abc", "rec abc"), "recording"),
			(good.replace(".mp4", ".txt"), "output"),
			(
				good.replace("\"kind\":\"display\"", "\"kind\":\"fake\""),
				"source.kind",
			),
			(good.replace("h264_nvenc", "raw"), "encoder"),
		];
		for (line, what) in cases {
			let err = parse_command(&line, p).unwrap_err();
			assert!(err.contains(what), "{what}: {err}");
		}
	}

	#[test]
	fn paths_must_be_absolute_and_existing() {
		let dir = std::env::temp_dir();
		let good = start_line(&dir, "");
		let relative = good.replace(
			&json::string(&dir.join("c.mp4").to_string_lossy()),
			"\"c.mp4\"",
		);
		assert!(
			parse_command(&relative, Policy::default())
				.unwrap_err()
				.contains("output")
		);
		let missing = good.replace(
			&json::string(&dir.join("c.mp4").to_string_lossy()),
			&json::string(
				&dir.join("no-such-folder-xyz")
					.join("c.mp4")
					.to_string_lossy(),
			),
		);
		assert!(
			parse_command(&missing, Policy::default())
				.unwrap_err()
				.contains("folder")
		);
	}

	#[test]
	fn the_test_source_needs_the_policy() {
		let dir = std::env::temp_dir();
		let line = start_line(&dir, "")
			.replace("\"kind\":\"display\"", "\"kind\":\"fake\"")
			.replace("h264_nvenc", "raw")
			.replace(".mp4", ".raw");
		assert!(parse_command(&line, Policy::default()).is_err());
		assert!(
			parse_command(
				&line,
				Policy {
					allow_test_source: true
				}
			)
			.is_ok()
		);
	}

	#[test]
	fn reads_lines_and_survives_a_long_one() {
		let mut data = b"{\"a\":1}\r\n".to_vec();
		data.extend(std::iter::repeat_n(b'x', MAX_LINE * 3));
		data.extend_from_slice(b"\n{\"b\":2}\n{\"c\":3}");
		let mut r = Cursor::new(data);
		assert_eq!(read_line(&mut r).unwrap(), Line::Text("{\"a\":1}".into()));
		assert_eq!(read_line(&mut r).unwrap(), Line::TooLong);
		assert_eq!(read_line(&mut r).unwrap(), Line::Text("{\"b\":2}".into()));
		assert_eq!(read_line(&mut r).unwrap(), Line::Text("{\"c\":3}".into()));
		assert_eq!(read_line(&mut r).unwrap(), Line::Eof);
	}

	#[test]
	fn a_line_of_exactly_the_limit_is_accepted() {
		let mut data = vec![b'x'; MAX_LINE];
		data.push(b'\n');
		assert!(
			matches!(read_line(&mut Cursor::new(data)).unwrap(), Line::Text(t) if t.len() == MAX_LINE)
		);
		let mut data = vec![b'x'; MAX_LINE + 1];
		data.push(b'\n');
		assert_eq!(read_line(&mut Cursor::new(data)).unwrap(), Line::TooLong);
	}

	#[test]
	fn events_are_valid_json_and_messages_are_printable() {
		for line in [
			ready("windows", true, true),
			error("io", "caf\u{e9}\n\u{1b}[31m"),
			started(1920, 1080, 60),
			progress(1000, 60, 1, 5),
			finished("stopped", 60, 1, 1000, 5),
			sources(
				&[SourceInfo {
					id: "d1".into(),
					name: "Display 1".into(),
					width: 1,
					height: 2,
				}],
				&[],
			),
		] {
			assert!(json::parse(&line).is_ok(), "{line}");
			assert!(!line.contains('\n'));
		}
		assert_eq!(printable("a\u{e9}b"), "a?b");
	}
}
