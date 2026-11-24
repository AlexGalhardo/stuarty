//! What the test sources of the helper share (admitted only with `--allow-test-source`): one epoch, so the flash
//! of a fake screen and the tone burst of a fake microphone happen at the same instant of real time, and the
//! tags of a fake id (SDD-025 section 16.7).

use std::sync::OnceLock;
use std::time::Instant;

static EPOCH: OnceLock<Instant> = OnceLock::new();

/// The instant the first test source was made.
pub fn epoch() -> Instant {
	*EPOCH.get_or_init(Instant::now)
}

/// Nanoseconds since the epoch of an instant (0 before it).
pub fn ns_since_epoch(t: Instant) -> u64 {
	u64::try_from(t.saturating_duration_since(epoch()).as_nanos()).unwrap_or(u64::MAX)
}

/// The tags of a fake id: `fake` then `-p<ms>` (the pulse period), `-w<ms>` (the pulse width), `-d<ppm>` (the
/// device clock runs this much faster, a negative number slower, written `-dm<ppm>`), and `-s` (silence).
#[derive(Debug, Clone, Copy, PartialEq)]
pub struct Tags {
	pub period_ms: u64,
	pub width_ms: u64,
	pub drift_ppm: i64,
	pub silent: bool,
}

impl Default for Tags {
	fn default() -> Self {
		Self {
			period_ms: 1000,
			width_ms: 100,
			drift_ppm: 0,
			silent: false,
		}
	}
}

pub fn parse_tags(id: &str) -> Option<Tags> {
	let mut parts = id.split('-');
	if parts.next()? != "fake" {
		return None;
	}
	let mut t = Tags::default();
	for p in parts {
		if p == "s" {
			t.silent = true;
		} else if let Some(v) = p.strip_prefix("dm") {
			t.drift_ppm = -i64::from(v.parse::<u32>().ok()?);
		} else if let Some(v) = p.strip_prefix('d') {
			t.drift_ppm = i64::from(v.parse::<u32>().ok()?);
		} else if let Some(v) = p.strip_prefix('p') {
			t.period_ms = v.parse().ok().filter(|n| (100..=60_000).contains(n))?;
		} else {
			t.width_ms = p
				.strip_prefix('w')?
				.parse()
				.ok()
				.filter(|n| (10..=10_000).contains(n))?;
		}
	}
	(t.width_ms < t.period_ms && t.drift_ppm.abs() <= 100_000).then_some(t)
}

/// Whether the pulse is on at a moment: the first `width_ms` of every `period_ms` since the epoch.
pub fn pulse_on(tags: &Tags, ns: u64) -> bool {
	(ns / 1_000_000) % tags.period_ms < tags.width_ms
}

#[cfg(test)]
mod tests {
	use super::*;

	#[test]
	fn tags_are_parsed_and_bounded() {
		assert_eq!(parse_tags("fake"), Some(Tags::default()));
		let t = parse_tags("fake-p500-w50-d2000-s").unwrap();
		assert_eq!(
			(t.period_ms, t.width_ms, t.drift_ppm, t.silent),
			(500, 50, 2000, true)
		);
		assert_eq!(parse_tags("fake-dm300").unwrap().drift_ppm, -300);
		for bad in [
			"",
			"faker",
			"mic",
			"fake-p10",
			"fake-w5",
			"fake-p200-w300",
			"fake-d200000",
			"fake-x",
		] {
			assert_eq!(parse_tags(bad), None, "{bad}");
		}
	}

	#[test]
	fn the_pulse_is_on_for_the_width_of_every_period() {
		let t = Tags::default();
		assert!(pulse_on(&t, 0) && pulse_on(&t, 99_999_999));
		assert!(!pulse_on(&t, 100_000_000) && !pulse_on(&t, 999_999_999));
		assert!(pulse_on(&t, 1_000_000_000));
	}
}
