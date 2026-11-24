//! A WAV file of 16-bit PCM written as the sound arrives. The header is rewritten every second, so a recording
//! that is cut off keeps what was written (SDD-025 section 16.1).

use std::fs::File;
use std::io::{Seek, SeekFrom, Write};
use std::path::Path;
use std::time::{Duration, Instant};

const HEADER: u64 = 44;
const SYNC_EVERY: Duration = Duration::from_secs(1);

pub struct WavWriter {
	file: File,
	channels: u16,
	rate: u32,
	data_bytes: u64,
	synced: Instant,
}

impl WavWriter {
	pub fn create(path: &Path, rate: u32, channels: u16) -> std::io::Result<Self> {
		let mut w = Self {
			file: File::create(path)?,
			channels,
			rate,
			data_bytes: 0,
			synced: Instant::now(),
		};
		w.write_header()?;
		Ok(w)
	}

	fn write_header(&mut self) -> std::io::Result<()> {
		// A WAV size field holds 32 bits; past that the file is cut at 4 GiB, which the byte limit keeps far away.
		let data = u32::try_from(self.data_bytes).unwrap_or(u32::MAX);
		let block = u32::from(self.channels) * 2;
		let mut h = Vec::with_capacity(HEADER as usize);
		h.extend_from_slice(b"RIFF");
		h.extend_from_slice(&data.saturating_add(36).to_le_bytes());
		h.extend_from_slice(b"WAVEfmt ");
		h.extend_from_slice(&16u32.to_le_bytes());
		h.extend_from_slice(&1u16.to_le_bytes());
		h.extend_from_slice(&self.channels.to_le_bytes());
		h.extend_from_slice(&self.rate.to_le_bytes());
		h.extend_from_slice(&(self.rate * block).to_le_bytes());
		h.extend_from_slice(&(block as u16).to_le_bytes());
		h.extend_from_slice(&16u16.to_le_bytes());
		h.extend_from_slice(b"data");
		h.extend_from_slice(&data.to_le_bytes());
		self.file.seek(SeekFrom::Start(0))?;
		self.file.write_all(&h)?;
		self.file.seek(SeekFrom::Start(HEADER + self.data_bytes))?;
		Ok(())
	}

	/// Appends interleaved little-endian 16-bit samples.
	pub fn write(&mut self, pcm: &[u8]) -> std::io::Result<()> {
		self.file.write_all(pcm)?;
		self.data_bytes += pcm.len() as u64;
		if self.synced.elapsed() >= SYNC_EVERY {
			self.synced = Instant::now();
			self.write_header()?;
		}
		Ok(())
	}

	pub fn bytes(&self) -> u64 {
		HEADER + self.data_bytes
	}

	pub fn finish(mut self) -> std::io::Result<u64> {
		self.write_header()?;
		self.file.flush()?;
		Ok(self.bytes())
	}
}

#[cfg(test)]
mod tests {
	use super::*;

	#[test]
	fn the_header_matches_what_was_written() {
		let p = std::env::temp_dir().join(format!("stuarty-wav-{}.wav", std::process::id()));
		let mut w = WavWriter::create(&p, 48_000, 2).unwrap();
		w.write(&[1u8; 400]).unwrap();
		w.write(&[2u8; 400]).unwrap();
		assert_eq!(w.finish().unwrap(), 844);
		let b = std::fs::read(&p).unwrap();
		assert_eq!(b.len(), 844);
		assert_eq!(&b[0..4], b"RIFF");
		assert_eq!(u32::from_le_bytes(b[4..8].try_into().unwrap()), 836);
		assert_eq!(&b[8..16], b"WAVEfmt ");
		assert_eq!(u16::from_le_bytes(b[22..24].try_into().unwrap()), 2);
		assert_eq!(u32::from_le_bytes(b[24..28].try_into().unwrap()), 48_000);
		assert_eq!(u32::from_le_bytes(b[28..32].try_into().unwrap()), 192_000);
		assert_eq!(&b[36..40], b"data");
		assert_eq!(u32::from_le_bytes(b[40..44].try_into().unwrap()), 800);
		assert_eq!(b[44], 1);
		assert_eq!(b[843], 2);
		std::fs::remove_file(p).unwrap();
	}
}
