//! The control protocol, version 2 (SDD-025 sections 6 and 16.7): one JSON object per line, at most 65,536 bytes.

use std::io::BufRead;
use std::path::{Path, PathBuf};

use crate::json::{self, Json};
use crate::testsrc;

pub const VERSION: u64 = 2;
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
	pub extras: Extras,
}

/// The tracks beyond the screen a request asks for, each with its file (SDD-025 section 16.7).
#[derive(Debug, Clone, PartialEq, Default)]
pub struct Extras {
	/// A device id or `default`.
	pub microphone: Option<String>,
	pub system_audio: bool,
	pub camera: Option<String>,
	pub events: bool,
	pub out_microphone: Option<PathBuf>,
	pub out_system: Option<PathBuf>,
	pub out_camera: Option<PathBuf>,
	pub out_events: Option<PathBuf>,
}

#[derive(Debug, PartialEq)]
pub enum Command {
	Sources,
	Start(Box<StartRequest>),
	Stop,
	Pause,
	Resume,
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
		Some("pause") => Ok(Command::Pause),
		Some("resume") => Ok(Command::Resume),
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
	let extras = parse_extras(v, policy, &output, ext)?;
	Ok(StartRequest {
		recording: recording.to_string(),
		kind: kind.to_string(),
		source_id: source_id.to_string(),
		fps,
		encoder: encoder.to_string(),
		ffmpeg,
		output,
		extras,
	})
}

fn device_id(
	v: &Json,
	key: &str,
	policy: Policy,
	allow_default: bool,
) -> Result<Option<String>, String> {
	let Some(raw) = v.get(key) else {
		return Ok(None);
	};
	let id = raw.as_str().ok_or_else(|| format!("{key} must be text"))?;
	if id.is_empty() {
		return Ok(None);
	}
	let shaped = id.len() <= 64
		&& id
			.bytes()
			.all(|b| b.is_ascii_alphanumeric() || b == b'-' || b == b'_');
	if !shaped || (id == "default" && !allow_default) {
		return Err(format!("{key} is not a device id"));
	}
	if id.starts_with("fake") && (!policy.allow_test_source || testsrc::parse_tags(id).is_none()) {
		return Err(format!("{key} is not a device id"));
	}
	Ok(Some(id.to_string()))
}

/// Checks the tracks of a request and the file of each: a file is required for exactly each track asked for, in
/// the folder of `output`, with the extension of its kind (video: the one of `output`).
fn parse_extras(
	v: &Json,
	policy: Policy,
	output: &Path,
	video_ext: &str,
) -> Result<Extras, String> {
	let mut x = Extras {
		microphone: device_id(v, "microphone", policy, true)?,
		camera: device_id(v, "camera", policy, false)?,
		..Extras::default()
	};
	let flag = |key: &str| -> Result<bool, String> {
		match v.get(key) {
			None => Ok(false),
			Some(j) => j
				.as_bool()
				.ok_or_else(|| format!("{key} must be true or false")),
		}
	};
	x.system_audio = flag("system_audio")?;
	x.events = flag("events")?;

	let outputs = v.get("outputs");
	if let Some(o) = outputs
		&& !matches!(o, Json::Obj(_))
	{
		return Err("outputs must be an object".into());
	}
	let folder = output.parent();
	let path = |key: &str, asked: bool, ext: &str| -> Result<Option<PathBuf>, String> {
		let given = outputs.and_then(|o| o.get(key));
		match (asked, given) {
			(false, None) => Ok(None),
			(false, Some(_)) => Err(format!(
				"outputs.{key} was given for a track that was not asked for"
			)),
			(true, None) => Err(format!("outputs.{key} is required")),
			(true, Some(p)) => {
				let p = PathBuf::from(
					p.as_str()
						.ok_or_else(|| format!("outputs.{key} must be text"))?,
				);
				let ok = p.is_absolute()
					&& p.extension().and_then(|e| e.to_str()) == Some(ext)
					&& p.parent() == folder;
				if ok {
					Ok(Some(p))
				} else {
					Err(format!(
						"outputs.{key} must be an absolute path ending in .{ext}, in the folder of output"
					))
				}
			}
		}
	};
	x.out_microphone = path("microphone", x.microphone.is_some(), "wav")?;
	x.out_system = path("system", x.system_audio, "wav")?;
	x.out_camera = path("camera", x.camera.is_some(), video_ext)?;
	x.out_events = path("events", x.events, "jsonl")?;
	Ok(x)
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

/// What the helper can capture on this system.
#[derive(Debug, Clone, Copy, Default)]
pub struct Capabilities {
	pub display: bool,
	pub window: bool,
	pub audio: bool,
	pub camera: bool,
	pub events: bool,
}

pub fn ready(os: &str, c: Capabilities) -> String {
	json::object(&[
		("type", json::string("ready")),
		("v", VERSION.to_string()),
		("helper", json::string(env!("CARGO_PKG_VERSION"))),
		("os", json::string(os)),
		("display", c.display.to_string()),
		("window", c.window.to_string()),
		("audio", c.audio.to_string()),
		("camera", c.camera.to_string()),
		("events", c.events.to_string()),
	])
}

/// A track that is recording: its kind and where it starts on the clock.
pub struct TrackStart {
	pub kind: &'static str,
	pub start_ms: u64,
}

/// A track at the end of the recording.
pub struct TrackEnd {
	pub kind: &'static str,
	pub start_ms: u64,
	pub duration_ms: u64,
	pub bytes: u64,
	pub lost: bool,
}

fn array(items: Vec<String>) -> String {
	format!("[{}]", items.join(","))
}

pub fn level(microphone: Option<u32>, system: Option<u32>) -> String {
	let mut m = vec![("type", json::string("level"))];
	if let Some(v) = microphone {
		m.push(("microphone", v.min(1000).to_string()));
	}
	if let Some(v) = system {
		m.push(("system", v.min(1000).to_string()));
	}
	json::object(&m)
}

pub fn paused(elapsed_ms: u64) -> String {
	json::object(&[
		("type", json::string("paused")),
		("elapsed_ms", elapsed_ms.to_string()),
	])
}

pub fn resumed(elapsed_ms: u64) -> String {
	json::object(&[
		("type", json::string("resumed")),
		("elapsed_ms", elapsed_ms.to_string()),
	])
}

pub fn error(code: &str, message: &str) -> String {
	json::object(&[
		("type", json::string("error")),
		("code", json::string(code)),
		("message", json::string(&printable(message))),
	])
}

pub fn started(width: u32, height: u32, fps: u32, tracks: &[TrackStart]) -> String {
	let tracks = tracks
		.iter()
		.map(|t| {
			json::object(&[
				("kind", json::string(t.kind)),
				("start_ms", t.start_ms.to_string()),
			])
		})
		.collect();
	json::object(&[
		("type", json::string("started")),
		("width", width.to_string()),
		("height", height.to_string()),
		("fps", fps.to_string()),
		("tracks", array(tracks)),
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

/// The totals of a finished recording.
pub struct Finish<'a> {
	pub reason: &'a str,
	pub frames: u64,
	pub dropped: u64,
	pub duration_ms: u64,
	pub bytes: u64,
	pub tracks: &'a [TrackEnd],
	pub pauses: &'a [(u64, u64)],
	pub pointer_events: u64,
}

pub fn finished(f: &Finish) -> String {
	let tracks = f
		.tracks
		.iter()
		.map(|t| {
			json::object(&[
				("kind", json::string(t.kind)),
				("start_ms", t.start_ms.to_string()),
				("duration_ms", t.duration_ms.to_string()),
				("bytes", t.bytes.to_string()),
				("lost", t.lost.to_string()),
			])
		})
		.collect();
	let pauses = f
		.pauses
		.iter()
		.map(|(a, b)| json::object(&[("from_ms", a.to_string()), ("to_ms", b.to_string())]))
		.collect();
	json::object(&[
		("type", json::string("finished")),
		("reason", json::string(f.reason)),
		("frames", f.frames.to_string()),
		("dropped", f.dropped.to_string()),
		("duration_ms", f.duration_ms.to_string()),
		("bytes", f.bytes.to_string()),
		("tracks", array(tracks)),
		("pauses", array(pauses)),
		("pointer_events", f.pointer_events.to_string()),
	])
}

/// One entry of a sources message: a display, a window, a microphone or a camera.
pub struct SourceInfo {
	pub id: String,
	pub name: String,
	pub width: u32,
	pub height: u32,
	/// A microphone that is the default one of the system.
	pub default: bool,
}

/// What a sources message lists.
#[derive(Default)]
pub struct SourceLists {
	pub displays: Vec<SourceInfo>,
	pub windows: Vec<SourceInfo>,
	pub microphones: Vec<SourceInfo>,
	pub cameras: Vec<SourceInfo>,
	pub system_audio: bool,
}

pub fn sources(l: &SourceLists) -> String {
	let list = |items: &[SourceInfo], device: bool| {
		let parts: Vec<String> = items
			.iter()
			.map(|s| {
				let mut m = vec![("id", json::string(&s.id)), ("name", json::string(&s.name))];
				if device {
					m.push(("default", s.default.to_string()));
				} else {
					m.push(("width", s.width.to_string()));
					m.push(("height", s.height.to_string()));
				}
				json::object(&m)
			})
			.collect();
		array(parts)
	};
	json::object(&[
		("type", json::string("sources")),
		("displays", list(&l.displays, false)),
		("windows", list(&l.windows, false)),
		("microphones", list(&l.microphones, true)),
		("cameras", list(&l.cameras, true)),
		("system_audio", l.system_audio.to_string()),
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
		let tracks = [TrackEnd {
			kind: "microphone",
			start_ms: 0,
			duration_ms: 990,
			bytes: 5,
			lost: false,
		}];
		for line in [
			ready("windows", Capabilities::default()),
			error("io", "caf\u{e9}\n\u{1b}[31m"),
			started(
				1920,
				1080,
				60,
				&[TrackStart {
					kind: "screen",
					start_ms: 0,
				}],
			),
			progress(1000, 60, 1, 5),
			level(Some(400), None),
			paused(10),
			resumed(10),
			finished(&Finish {
				reason: "stopped",
				frames: 60,
				dropped: 1,
				duration_ms: 1000,
				bytes: 5,
				tracks: &tracks,
				pauses: &[(10, 20)],
				pointer_events: 3,
			}),
			sources(&SourceLists {
				displays: vec![SourceInfo {
					id: "d1".into(),
					name: "Display 1".into(),
					width: 1,
					height: 2,
					default: false,
				}],
				microphones: vec![SourceInfo {
					id: "m0".into(),
					name: "Mic".into(),
					width: 0,
					height: 0,
					default: true,
				}],
				system_audio: true,
				..SourceLists::default()
			}),
		] {
			assert!(json::parse(&line).is_ok(), "{line}");
			assert!(!line.contains('\n'));
		}
		assert_eq!(printable("a\u{e9}b"), "a?b");
	}

	fn with_extras(extra: &str, fake: bool) -> Result<Command, String> {
		let dir = std::env::temp_dir();
		let esc = |n: &str| json::string(&dir.join(n).to_string_lossy());
		let line = start_line(&dir, extra)
			.replace("MIC", &esc("m.wav"))
			.replace("SYS", &esc("s.wav"))
			.replace("CAM", &esc("cam.mp4"))
			.replace("EVT_BAD", &esc("e.txt"))
			.replace("EVT", &esc("e.jsonl"));
		parse_command(
			&line,
			Policy {
				allow_test_source: fake,
			},
		)
	}

	#[test]
	fn the_tracks_of_a_start_need_exactly_their_files() {
		let all = r#","microphone":"default","system_audio":true,"camera":"c0123","events":true,"outputs":{"microphone":MIC,"system":SYS,"camera":CAM,"events":EVT}"#;
		match with_extras(all, false) {
			Ok(Command::Start(s)) => {
				let x = s.extras;
				assert_eq!(x.microphone.as_deref(), Some("default"));
				assert!(x.system_audio && x.events);
				assert_eq!(x.camera.as_deref(), Some("c0123"));
				assert!(x.out_microphone.is_some() && x.out_system.is_some());
				assert!(x.out_camera.is_some() && x.out_events.is_some());
			}
			other => panic!("{other:?}"),
		}
		let only_screen = with_extras("", false).unwrap();
		match only_screen {
			Command::Start(s) => assert_eq!(s.extras, Extras::default()),
			other => panic!("{other:?}"),
		}
		let cases = [
			(
				r#","microphone":"default""#,
				"outputs.microphone is required",
			),
			(
				r#","system_audio":true,"outputs":{}"#,
				"outputs.system is required",
			),
			(r#","outputs":{"events":EVT}"#, "was not asked for"),
			(
				r#","events":true,"outputs":{"events":"rel.jsonl"}"#,
				"absolute",
			),
			(
				r#","events":true,"outputs":{"events":EVT_BAD}"#,
				"outputs.events",
			),
			(
				r#","microphone":"a b","outputs":{"microphone":MIC}"#,
				"microphone is not a device id",
			),
			(
				r#","camera":"default","outputs":{"camera":CAM}"#,
				"camera is not a device id",
			),
			(r#","events":"yes""#, "events must be true or false"),
			(
				r#","microphone":"fake","outputs":{"microphone":MIC}"#,
				"microphone is not a device id",
			),
		];
		for (extra, want) in cases {
			let err = with_extras(extra, false).unwrap_err();
			assert!(err.contains(want), "{extra}: {err}");
		}
		let elsewhere = r#","events":true,"outputs":{"events":"C:\\elsewhere\\e.jsonl"}"#;
		assert!(with_extras(elsewhere, false).is_err());
	}

	#[test]
	fn a_fake_device_needs_the_policy_and_a_valid_tag() {
		let ok = r#","microphone":"fake-p500","outputs":{"microphone":MIC}"#;
		assert!(with_extras(ok, true).is_ok());
		assert!(with_extras(ok, false).is_err());
		let bad = r#","microphone":"fake-q9","outputs":{"microphone":MIC}"#;
		assert!(with_extras(bad, true).is_err());
	}

	#[test]
	fn pause_and_resume_are_commands() {
		assert_eq!(
			parse_command(r#"{"type":"pause"}"#, Policy::default()),
			Ok(Command::Pause)
		);
		assert_eq!(
			parse_command(r#"{"type":"resume","x":1}"#, Policy::default()),
			Ok(Command::Resume)
		);
	}
}
