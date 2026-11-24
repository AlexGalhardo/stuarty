//! Where frames go: an FFmpeg child that writes the fragmented MP4, or (tests only) a raw file.

use std::fs::File;
use std::io::{Read, Write};
use std::process::{Child, ChildStdin, Command, Stdio};
use std::sync::{Arc, Mutex};
use std::thread::JoinHandle;
use std::time::{Duration, Instant};

use crate::protocol::StartRequest;

pub trait Sink: Send {
	fn write_frame(&mut self, bytes: &[u8]) -> std::io::Result<()>;
	/// Closes the input and waits for the output to be complete. The error is what the encoder said.
	fn finish(self: Box<Self>) -> Result<(), String>;
}

/// The bit rate for a size and a rate: about a tenth of a bit per pixel, between 2 and 80 Mbit/s.
pub fn bitrate(width: u32, height: u32, fps: u32) -> u64 {
	(u64::from(width) * u64::from(height) * u64::from(fps) / 10).clamp(2_000_000, 80_000_000)
}

/// The FFmpeg arguments for a request. The encoder is one of the closed list; nothing the caller sent reaches
/// an argument but the numbers and the output path, which the protocol already checked.
pub fn ffmpeg_args(req: &StartRequest, width: u32, height: u32) -> Vec<String> {
	let fps = req.fps.to_string();
	let rate = format!("{}", bitrate(width, height, req.fps));
	let mut a: Vec<String> = [
		"-hide_banner",
		"-loglevel",
		"error",
		"-y",
		"-f",
		"rawvideo",
		"-pix_fmt",
		"bgra",
		"-video_size",
	]
	.map(String::from)
	.to_vec();
	a.push(format!("{width}x{height}"));
	a.extend(
		[
			"-framerate",
			&fps,
			"-i",
			"pipe:0",
			"-an",
			"-c:v",
			&req.encoder,
			"-b:v",
			&rate,
			"-g",
			&fps,
		]
		.map(String::from),
	);
	// Measured in spikes/capture-windows/README.md: these keep the encoder from holding back the last seconds.
	match req.encoder.as_str() {
		"h264_nvenc" => {
			a.extend(["-delay", "0", "-rc-lookahead", "0", "-bf", "0"].map(String::from))
		}
		"h264_qsv" => a.extend(["-bf", "0"].map(String::from)),
		"h264_amf" => a.extend(["-usage", "lowlatency", "-bf", "0"].map(String::from)),
		_ => {}
	}
	a.extend(
		[
			"-pix_fmt",
			"yuv420p",
			"-movflags",
			"+frag_keyframe+empty_moov+default_base_moof",
			"-f",
			"mp4",
			"-flush_packets",
			"1",
		]
		.map(String::from),
	);
	a.push(req.output.to_string_lossy().into_owned());
	a
}

/// The only environment variables FFmpeg gets.
const ENV_ALLOW: [&str; 7] = [
	"SystemRoot",
	"windir",
	"SystemDrive",
	"TEMP",
	"TMP",
	"PATH",
	"ProgramData",
];

pub struct FfmpegSink {
	child: Child,
	stdin: Option<ChildStdin>,
	tail: Arc<Mutex<Vec<u8>>>,
	reader: Option<JoinHandle<()>>,
}

impl FfmpegSink {
	pub fn spawn(req: &StartRequest, width: u32, height: u32) -> Result<Self, String> {
		let mut cmd = Command::new(&req.ffmpeg);
		cmd.args(ffmpeg_args(req, width, height))
			.env_clear()
			.stdin(Stdio::piped())
			.stdout(Stdio::null())
			.stderr(Stdio::piped());
		for k in ENV_ALLOW {
			if let Some(v) = std::env::var_os(k) {
				cmd.env(k, v);
			}
		}
		#[cfg(windows)]
		{
			use std::os::windows::process::CommandExt;
			cmd.creation_flags(0x0800_0000); // CREATE_NO_WINDOW
		}
		let mut child = cmd
			.spawn()
			.map_err(|e| format!("could not start ffmpeg: {}", e.kind()))?;
		let stdin = child.stdin.take();
		let tail = Arc::new(Mutex::new(Vec::new()));
		let reader = child.stderr.take().map(|mut err| {
			let tail = Arc::clone(&tail);
			std::thread::spawn(move || {
				let mut buf = [0u8; 1024];
				while let Ok(n) = err.read(&mut buf) {
					if n == 0 {
						break;
					}
					let mut t = tail
						.lock()
						.unwrap_or_else(std::sync::PoisonError::into_inner);
					t.extend_from_slice(&buf[..n]);
					let over = t.len().saturating_sub(2048);
					t.drain(..over);
				}
			})
		});
		Ok(Self {
			child,
			stdin,
			tail,
			reader,
		})
	}

	fn said(&self) -> String {
		let t = self
			.tail
			.lock()
			.unwrap_or_else(std::sync::PoisonError::into_inner);
		let text = String::from_utf8_lossy(&t).trim().to_string();
		text.lines().last().unwrap_or("").to_string()
	}
}

impl Sink for FfmpegSink {
	fn write_frame(&mut self, bytes: &[u8]) -> std::io::Result<()> {
		match self.stdin.as_mut() {
			Some(s) => s.write_all(bytes),
			None => Err(std::io::ErrorKind::BrokenPipe.into()),
		}
	}

	fn finish(mut self: Box<Self>) -> Result<(), String> {
		drop(self.stdin.take());
		let deadline = Instant::now() + Duration::from_secs(30);
		let status = loop {
			match self.child.try_wait() {
				Ok(Some(s)) => break s,
				Ok(None) if Instant::now() < deadline => {
					std::thread::sleep(Duration::from_millis(20))
				}
				_ => {
					let _ = self.child.kill();
					let _ = self.child.wait();
					return Err("ffmpeg did not finish in time".into());
				}
			}
		};
		if let Some(r) = self.reader.take() {
			let _ = r.join();
		}
		if status.success() {
			Ok(())
		} else {
			Err(format!("ffmpeg failed: {}", self.said()))
		}
	}
}

impl Drop for FfmpegSink {
	fn drop(&mut self) {
		// A sink dropped without finish (an error path) must not leave the child behind.
		if self.child.try_wait().ok().flatten().is_none() {
			let _ = self.child.kill();
			let _ = self.child.wait();
		}
	}
}

/// Writes the bytes it gets to a file. Used only with the test encoder `raw`.
pub struct FileSink(File);

impl FileSink {
	pub fn create(output: &std::path::Path) -> Result<Self, String> {
		File::create(output)
			.map(Self)
			.map_err(|e| format!("could not create the output: {}", e.kind()))
	}
}

impl Sink for FileSink {
	fn write_frame(&mut self, bytes: &[u8]) -> std::io::Result<()> {
		self.0.write_all(bytes)
	}

	fn finish(mut self: Box<Self>) -> Result<(), String> {
		self.0.flush().map_err(|e| e.to_string())
	}
}

#[cfg(test)]
mod tests {
	use super::*;
	use std::path::PathBuf;

	fn req(encoder: &str) -> StartRequest {
		StartRequest {
			recording: "rec_a".into(),
			kind: "display".into(),
			source_id: "d1".into(),
			fps: 60,
			encoder: encoder.into(),
			ffmpeg: PathBuf::from("ffmpeg"),
			output: PathBuf::from("out.mp4"),
			extras: Default::default(),
		}
	}

	#[test]
	fn nvenc_gets_the_low_latency_options_the_spike_measured() {
		let a = ffmpeg_args(&req("h264_nvenc"), 1920, 1080).join(" ");
		for want in [
			"-delay 0",
			"-rc-lookahead 0",
			"-bf 0",
			"-flush_packets 1",
			"frag_keyframe+empty_moov+default_base_moof",
		] {
			assert!(a.contains(want), "{want} missing from {a}");
		}
		assert!(
			a.contains("-video_size 1920x1080")
				&& a.contains("-framerate 60")
				&& a.ends_with("out.mp4")
		);
	}

	#[test]
	fn other_encoders_get_only_options_they_know() {
		let a = ffmpeg_args(&req("libopenh264"), 1920, 1080).join(" ");
		assert!(
			!a.contains("-delay") && !a.contains("-rc-lookahead") && a.contains("-flush_packets 1")
		);
		assert!(
			ffmpeg_args(&req("h264_amf"), 1280, 720)
				.join(" ")
				.contains("-usage lowlatency")
		);
	}

	#[test]
	fn bitrate_is_bounded() {
		assert_eq!(bitrate(1920, 1080, 60), 12_441_600);
		assert_eq!(bitrate(16, 16, 24), 2_000_000);
		assert_eq!(bitrate(8192, 4096, 60), 80_000_000);
	}
}
