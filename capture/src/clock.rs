//! The one clock of a recording (SDD-025 section 16.2). Its origin is the instant the screen grid starts; every
//! track and every pointer event is stamped in **active** time, the wall time since the origin with the pauses
//! taken out, so one position means one moment in every file.
//!
//! `std::time::Instant` is the performance counter of Windows (QPC), and a WASAPI packet is stamped with a QPC
//! position in units of 100 ns, which `qpc_to_wall_ns` puts on the same axis.

use std::sync::Mutex;
use std::time::Instant;

const NS_PER_MS: i64 = 1_000_000;

/// Called first by every thread that stamps data on the clock (the screen grid, the camera, the sound, the
/// pointer): on Windows it raises the thread above normal so a busy machine does not delay it.
pub fn raise_this_thread() {
	#[cfg(windows)]
	crate::win32::raise_this_thread();
}

#[derive(Default)]
struct Inner {
	origin: Option<Instant>,
	/// The QPC reading, in units of 100 ns, taken at the origin.
	origin_qpc: u64,
	/// Closed pauses as (from, to) in wall nanoseconds since the origin, in order.
	pauses: Vec<(i64, i64)>,
	open_pause: Option<i64>,
}

#[derive(Default)]
pub struct Clock {
	inner: Mutex<Inner>,
}

/// A run of frames of a packet that lies outside every pause, with where it starts in active time.
#[derive(Debug, PartialEq, Eq)]
pub struct Segment {
	pub first_frame: u64,
	pub frames: u64,
	pub active_start_ns: i64,
}

impl Clock {
	pub fn new() -> Self {
		Self::default()
	}

	fn lock(&self) -> std::sync::MutexGuard<'_, Inner> {
		self.inner
			.lock()
			.unwrap_or_else(std::sync::PoisonError::into_inner)
	}

	/// Starts the clock now. `qpc_100ns` is the performance counter read at the same moment (0 where there is no
	/// device that stamps with it).
	pub fn start(&self, qpc_100ns: u64) {
		let mut i = self.lock();
		if i.origin.is_none() {
			i.origin = Some(Instant::now());
			i.origin_qpc = qpc_100ns;
		}
	}

	pub fn started(&self) -> bool {
		self.lock().origin.is_some()
	}

	/// Wall nanoseconds since the origin of a given instant; negative before it. None until the clock starts.
	pub fn wall_ns_of(&self, t: Instant) -> Option<i64> {
		let o = self.lock().origin?;
		Some(if t >= o {
			i64::try_from((t - o).as_nanos()).unwrap_or(i64::MAX)
		} else {
			-i64::try_from((o - t).as_nanos()).unwrap_or(i64::MAX)
		})
	}

	pub fn wall_ns(&self) -> i64 {
		self.wall_ns_of(Instant::now()).unwrap_or(0)
	}

	/// A QPC position in units of 100 ns as wall nanoseconds since the origin.
	pub fn qpc_to_wall_ns(&self, qpc_100ns: u64) -> i64 {
		let origin = self.lock().origin_qpc;
		let ns = (i128::from(qpc_100ns) - i128::from(origin)) * 100;
		i64::try_from(ns).unwrap_or(if ns < 0 { i64::MIN } else { i64::MAX })
	}

	pub fn paused(&self) -> bool {
		self.lock().open_pause.is_some()
	}

	/// Active nanoseconds now. While paused it stays at the value of the moment the pause began.
	pub fn active_ns(&self) -> i64 {
		let wall = self.wall_ns();
		let i = self.lock();
		active_of(&i, wall).unwrap_or_else(|| open_pause_active(&i))
	}

	/// Active time of a wall moment; None inside a pause. A moment before the origin is returned as it is
	/// (negative): no pause can precede the origin.
	#[cfg(test)]
	pub fn active_of_wall(&self, wall_ns: i64) -> Option<i64> {
		active_of(&self.lock(), wall_ns)
	}

	/// Begins a pause now. The error is why it cannot.
	pub fn pause(&self) -> Result<i64, &'static str> {
		self.pause_at(self.wall_ns())
	}

	pub fn resume(&self) -> Result<i64, &'static str> {
		self.resume_at(self.wall_ns())
	}

	pub fn pause_at(&self, wall_ns: i64) -> Result<i64, &'static str> {
		let mut i = self.lock();
		if i.origin.is_none() {
			return Err("the recording has not started");
		}
		if i.open_pause.is_some() {
			return Err("the recording is already paused");
		}
		let last_end = i.pauses.last().map_or(0, |p| p.1);
		let at = wall_ns.max(last_end);
		i.open_pause = Some(at);
		Ok(open_pause_active(&i))
	}

	pub fn resume_at(&self, wall_ns: i64) -> Result<i64, &'static str> {
		let mut i = self.lock();
		let Some(from) = i.open_pause else {
			return Err("the recording is not paused");
		};
		// A pause has a length, so a resume in the same nanosecond still counts one.
		let to = wall_ns.max(from + 1);
		i.pauses.push((from, to));
		i.open_pause = None;
		active_of(&i, to).ok_or("the recording is not paused")
	}

	/// The pauses in milliseconds on the wall clock, an open one closed at now.
	pub fn pauses_ms(&self) -> Vec<(u64, u64)> {
		let wall = self.wall_ns();
		let i = self.lock();
		let mut out: Vec<(i64, i64)> = i.pauses.clone();
		if let Some(from) = i.open_pause {
			out.push((from, wall.max(from + 1)));
		}
		out.into_iter()
			.map(|(a, b)| {
				(
					u64::try_from(a / NS_PER_MS).unwrap_or(0),
					u64::try_from(b / NS_PER_MS).unwrap_or(0),
				)
			})
			.collect()
	}

	/// Cuts the frames of a packet at the pauses. `wall_start_ns` is the stamp of its first frame, `rate` the
	/// frames per second. Only the runs outside a pause are returned; a packet inside a pause gives none.
	pub fn segments(&self, wall_start_ns: i64, frames: u64, rate: u32) -> Vec<Segment> {
		let i = self.lock();
		let rate = i128::from(rate);
		let ns_of = |f: u64| i128::from(wall_start_ns) + i128::from(f) * 1_000_000_000 / rate;
		let frame_at = |ns: i64| -> u64 {
			// The first frame whose time is not before ns.
			let d = (i128::from(ns) - i128::from(wall_start_ns)) * rate;
			if d <= 0 {
				0
			} else {
				u64::try_from((d + 999_999_999) / 1_000_000_000).unwrap_or(u64::MAX)
			}
		};

		let mut cuts: Vec<(i64, i64)> = i.pauses.clone();
		if let Some(from) = i.open_pause {
			cuts.push((from, i64::MAX));
		}

		let mut out = Vec::new();
		let mut cursor = 0u64;
		for (a, b) in cuts {
			let end_ns = ns_of(frames);
			if i128::from(b) <= i128::from(wall_start_ns) || i128::from(a) >= end_ns {
				continue;
			}
			let f_a = frame_at(a).clamp(cursor, frames);
			if f_a > cursor {
				out.push((cursor, f_a - cursor));
			}
			let f_b = if b == i64::MAX { frames } else { frame_at(b) };
			cursor = cursor.max(f_b.min(frames));
		}
		if cursor < frames {
			out.push((cursor, frames - cursor));
		}

		out.into_iter()
			.filter_map(|(first, n)| {
				let wall = i64::try_from(ns_of(first)).ok()?;
				Some(Segment {
					first_frame: first,
					frames: n,
					active_start_ns: active_of(&i, wall)?,
				})
			})
			.collect()
	}
}

fn removed_before(i: &Inner, wall: i64) -> i64 {
	let mut removed = 0;
	for &(a, b) in &i.pauses {
		if wall >= b {
			removed += b - a;
		} else if wall > a {
			removed += wall - a;
		}
	}
	removed
}

fn active_of(i: &Inner, wall: i64) -> Option<i64> {
	if wall < 0 {
		return Some(wall);
	}
	if let Some(from) = i.open_pause
		&& wall >= from
	{
		return None;
	}
	if i.pauses.iter().any(|&(a, b)| wall >= a && wall < b) {
		return None;
	}
	Some(wall - removed_before(i, wall))
}

/// Where active time stood when the open pause began.
fn open_pause_active(i: &Inner) -> i64 {
	i.open_pause
		.map_or(0, |from| from - removed_before(i, from))
}

#[cfg(test)]
mod tests {
	use super::*;

	fn clock() -> Clock {
		let c = Clock::new();
		c.start(0);
		c
	}

	const MS: i64 = NS_PER_MS;

	#[test]
	fn active_time_removes_every_pause() {
		let c = clock();
		c.pause_at(1000 * MS).unwrap();
		c.resume_at(1500 * MS).unwrap();
		c.pause_at(3000 * MS).unwrap();
		c.resume_at(4000 * MS).unwrap();
		assert_eq!(c.active_of_wall(999 * MS), Some(999 * MS));
		assert_eq!(c.active_of_wall(1000 * MS), None);
		assert_eq!(c.active_of_wall(1499 * MS), None);
		assert_eq!(c.active_of_wall(1500 * MS), Some(1000 * MS));
		assert_eq!(c.active_of_wall(2999 * MS), Some(2499 * MS));
		assert_eq!(c.active_of_wall(4000 * MS), Some(2500 * MS));
		assert_eq!(c.active_of_wall(5000 * MS), Some(3500 * MS));
		assert_eq!(c.active_of_wall(-5 * MS), Some(-5 * MS));
		assert_eq!(c.pauses_ms(), vec![(1000, 1500), (3000, 4000)]);
	}

	#[test]
	fn pause_and_resume_refuse_what_they_cannot_do() {
		let c = Clock::new();
		assert!(c.pause().is_err(), "before the origin");
		c.start(0);
		assert!(c.resume().is_err(), "not paused");
		c.pause().unwrap();
		assert!(c.paused());
		assert!(c.pause().is_err(), "already paused");
		c.resume().unwrap();
		assert!(!c.paused());
	}

	#[test]
	fn active_ns_freezes_while_paused() {
		let c = clock();
		std::thread::sleep(std::time::Duration::from_millis(30));
		c.pause().unwrap();
		let a = c.active_ns();
		std::thread::sleep(std::time::Duration::from_millis(40));
		assert_eq!(c.active_ns(), a);
		assert!(a >= 25 * MS, "{a}");
		c.resume().unwrap();
		std::thread::sleep(std::time::Duration::from_millis(20));
		let b = c.active_ns();
		assert!(b > a && b < a + 200 * MS, "{a} {b}");
	}

	#[test]
	fn a_packet_is_cut_at_the_pauses() {
		let c = clock();
		// 1000 frames a second: one frame per millisecond.
		c.pause_at(100 * MS).unwrap();
		c.resume_at(150 * MS).unwrap();
		// a packet of 200 frames from wall 80 ms to 280 ms
		let s = c.segments(80 * MS, 200, 1000);
		assert_eq!(
			s,
			vec![
				Segment {
					first_frame: 0,
					frames: 20,
					active_start_ns: 80 * MS
				},
				Segment {
					first_frame: 70,
					frames: 130,
					active_start_ns: 100 * MS
				},
			]
		);
		// a packet entirely inside the pause has nothing
		assert!(c.segments(110 * MS, 30, 1000).is_empty());
		// a packet outside every pause is one run
		assert_eq!(c.segments(300 * MS, 10, 1000).len(), 1);
	}

	#[test]
	fn an_open_pause_swallows_what_follows() {
		let c = clock();
		c.pause_at(100 * MS).unwrap();
		let s = c.segments(90 * MS, 50, 1000);
		assert_eq!(s.len(), 1);
		assert_eq!((s[0].first_frame, s[0].frames), (0, 10));
		assert!(c.segments(200 * MS, 10, 1000).is_empty());
	}

	#[test]
	fn qpc_positions_land_on_the_wall_axis() {
		let c = Clock::new();
		c.start(5_000_000);
		assert_eq!(c.qpc_to_wall_ns(5_000_000), 0);
		assert_eq!(c.qpc_to_wall_ns(5_010_000), 1_000_000);
		assert_eq!(c.qpc_to_wall_ns(4_990_000), -1_000_000);
	}
}
