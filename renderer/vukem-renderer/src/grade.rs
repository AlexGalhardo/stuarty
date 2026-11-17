//! The advanced colour of SDD-004 section 3.14 (SDD-013 section 3.4): HSL bands, then the three colour
//! wheels, then the tone curves, on straight RGB values 0..255, in the integer arithmetic that
//! `engine/compositor/grade.go` uses, so a frame has the same bytes in both.

use crate::image::Image;
use crate::math::{div255_i64, mul};

/// A full turn of the hue, 65536 per 60 degrees.
const HUE_TURN: i64 = 6 * 65536;
const BANDS: usize = 8;
/// The largest slope of a curve, 1024 in 16.16.
const MAX_SLOPE: i64 = 1 << 26;

/// The hues of the eight HSL ranges (red, orange, yellow, green, cyan, blue, purple, magenta) in 65536ths of
/// 60 degrees, then red again a turn later.
const HUE_CENTRES: [i64; BANDS + 1] = [
	0, 32768, 65536, 131_072, 196_608, 262_144, 300_373, 349_525, HUE_TURN,
];

/// The ranges in the order of `HUE_CENTRES`.
pub const HSL_RANGES: [&str; BANDS] = [
	"red", "orange", "yellow", "green", "cyan", "blue", "purple", "magenta",
];

/// One HSL band as the plan states it: a shift of the hue, of the saturation and of the lightness, each -1 to 1.
#[derive(Clone, Copy, Debug, Default, PartialEq)]
pub struct HslBand {
	pub hue: f64,
	pub saturation: f64,
	pub lightness: f64,
}

/// One colour wheel: the direction of the tint in degrees, how strong it is and the lift of its tonal range.
#[derive(Clone, Copy, Debug, PartialEq)]
pub struct Wheel {
	pub hue: f64,
	pub amount: f64,
	pub luma: f64,
}

/// A point of a tone curve, both coordinates 0 to 1.
#[derive(Clone, Copy, Debug, PartialEq)]
pub struct CurvePoint {
	pub x: f64,
	pub y: f64,
}

/// The advanced colour of a layer, as the plan states it.
#[derive(Clone, Debug, Default, PartialEq)]
pub struct GradeSpec {
	/// One entry per range of `HSL_RANGES`; a range the plan does not list is the default band.
	pub hsl: [HslBand; BANDS],
	pub wheels: [Option<Wheel>; 3],
	pub master: Vec<CurvePoint>,
	pub red: Vec<CurvePoint>,
	pub green: Vec<CurvePoint>,
	pub blue: Vec<CurvePoint>,
}

#[derive(Clone, Copy, Default, PartialEq, Eq)]
struct Band {
	hue: i64,
	sat: i64,
	light: i64,
}

/// A layer's grade with every parameter already in integers.
pub struct Grade {
	bands: [Band; BANDS],
	has_hsl: bool,
	wheels: [Option<[i64; 3]>; 3],
	has_wheel: bool,
	curves: Option<Box<[[u8; 256]; 3]>>,
}

/// A parameter in 1/1024 of its range -1..1; NaN is 0.
fn q1024(v: f64) -> i64 {
	if v.is_nan() {
		return 0;
	}
	mul(v, 1024.0).clamp(-1024.0, 1024.0).round() as i64
}

fn clamp8(v: i64) -> i64 {
	v.clamp(0, 255)
}

fn sign(v: i64) -> i32 {
	match v {
		0 => 0,
		v if v > 0 => 1,
		_ => -1,
	}
}

impl Grade {
	/// The grade of a spec, or `None` when it changes no pixel.
	pub fn new(spec: &GradeSpec) -> Option<Self> {
		let mut g = Self {
			bands: [Band::default(); BANDS],
			has_hsl: false,
			wheels: [None; 3],
			has_wheel: false,
			curves: None,
		};
		for (i, b) in spec.hsl.iter().enumerate() {
			g.bands[i] = Band {
				hue: q1024(b.hue),
				sat: q1024(b.saturation),
				light: q1024(b.lightness),
			};
			g.has_hsl |= g.bands[i] != Band::default();
		}
		for (i, w) in spec.wheels.iter().enumerate() {
			g.wheels[i] = w.as_ref().and_then(new_wheel);
			g.has_wheel |= g.wheels[i].is_some();
		}
		g.curves = build_curves(spec);
		if !g.has_hsl && !g.has_wheel && g.curves.is_none() {
			return None;
		}
		Some(g)
	}

	/// The colour of one pixel moved by the bands.
	fn hsl(&self, r: i64, g: i64, b: i64) -> (i64, i64, i64) {
		let mx = r.max(g).max(b);
		let mn = r.min(g).min(b);
		let d = mx - mn;
		if d == 0 {
			return (r, g, b);
		}
		let sum = mx + mn;
		let den = if sum > 255 { 510 - sum } else { sum };
		let sat = d * 65536 / den;
		let mut h = if mx == r {
			let h = (g - b) * 65536 / d;
			if h < 0 { h + HUE_TURN } else { h }
		} else if mx == g {
			(b - r) * 65536 / d + 131_072
		} else {
			(r - g) * 65536 / d + 262_144
		};
		let mut i = 0;
		while h >= HUE_CENTRES[i + 1] {
			i += 1;
		}
		let t = (h - HUE_CENTRES[i]) * 65536 / (HUE_CENTRES[i + 1] - HUE_CENTRES[i]);
		let wa = ((65536 - t) * sat) >> 16;
		let wb = (t * sat) >> 16;
		let (ba, bb) = (self.bands[i], self.bands[(i + 1) % BANDS]);
		let hs = (wa * ba.hue + wb * bb.hue) / 2048;
		let ds = (wa * ba.sat + wb * bb.sat) / 1024;
		let dl = (wa * ba.light + wb * bb.light) / 1024;
		if hs == 0 && ds == 0 && dl == 0 {
			return (r, g, b);
		}
		h = ((h + hs) % HUE_TURN + HUE_TURN) % HUE_TURN;
		let s2 = ((sat * (65536 + ds)) >> 16).clamp(0, 65536);
		let mut l = (sum * 65536 + 255) / 510;
		if dl >= 0 {
			l += ((65536 - l) * dl) >> 16;
		} else {
			l -= (l * -dl) >> 16;
		}
		let c = ((65536 - (2 * l - 65536).abs()) * s2) >> 16;
		let (sector, f) = (h / 65536, h % 65536);
		let x = if sector % 2 == 1 {
			(c * (65536 - f)) >> 16
		} else {
			(c * f) >> 16
		};
		let m = l - (c >> 1);
		let v = match sector {
			0 => [c, x, 0],
			1 => [x, c, 0],
			2 => [0, c, x],
			3 => [0, x, c],
			4 => [x, 0, c],
			_ => [c, 0, x],
		};
		let out = |c: i64| clamp8(((c + m) * 255 + 32768) >> 16);
		(out(v[0]), out(v[1]), out(v[2]))
	}

	/// The three wheels added to one pixel.
	fn wheel(&self, r: i64, g: i64, b: i64) -> (i64, i64, i64) {
		let y = (54 * r + 183 * g + 19 * b + 128) >> 8;
		let s = (255 - y) * (255 - y);
		let hi = y * y;
		let ws = [s, 65025 - s - hi, hi];
		let mut n = [0_i64; 3];
		for (k, w) in self.wheels.iter().enumerate() {
			let Some(w) = w else { continue };
			let w16 = (ws[k] * 65536 + 32512) / 65025;
			for (acc, b) in n.iter_mut().zip(w) {
				*acc += w16 * b;
			}
		}
		const HALF: i64 = 1 << 43;
		(
			clamp8(r + ((n[0] * 255 + HALF) >> 44)),
			clamp8(g + ((n[1] * 255 + HALF) >> 44)),
			clamp8(b + ((n[2] * 255 + HALF) >> 44)),
		)
	}

	/// The whole grade of one straight pixel.
	fn pixel(&self, r: i64, g: i64, b: i64) -> (i64, i64, i64) {
		let (mut r, mut g, mut b) = (r, g, b);
		if self.has_hsl {
			(r, g, b) = self.hsl(r, g, b);
		}
		if self.has_wheel {
			(r, g, b) = self.wheel(r, g, b);
		}
		if let Some(t) = &self.curves {
			(r, g, b) = (
				i64::from(t[0][r as usize]),
				i64::from(t[1][g as usize]),
				i64::from(t[2][b as usize]),
			);
		}
		(r, g, b)
	}
}

/// `B[c] = amount x tint[c] x 4 + luma x 65536`, with the amount and the luma in 1/1024 and the tint of the
/// hue (its RGB at full saturation and value, minus their mean) in 16.16; `None` when the wheel does nothing.
fn new_wheel(w: &Wheel) -> Option<[i64; 3]> {
	let (amount, luma) = (q1024(w.amount), q1024(w.luma));
	if amount == 0 && luma == 0 {
		return None;
	}
	let hue = if w.hue.is_nan() {
		0
	} else {
		mul(w.hue, 1000.0).clamp(0.0, 360_000.0).round() as i64 % 360_000
	};
	let h6 = hue * 6;
	let (sector, f) = (h6 / 360_000, (h6 % 360_000) * 65536 / 360_000);
	let one = 65536;
	let rgb = match sector {
		0 => [one, f, 0],
		1 => [one - f, one, 0],
		2 => [0, one, f],
		3 => [0, one - f, one],
		4 => [f, 0, one],
		_ => [one, 0, one - f],
	};
	let mean = (rgb[0] + rgb[1] + rgb[2]) / 3;
	Some(core::array::from_fn(|c| {
		amount * (rgb[c] - mean) * 4 + luma * 65536
	}))
}

fn curve_points(pts: &[CurvePoint]) -> Vec<(i64, i64)> {
	let q = |v: f64| {
		if v.is_nan() {
			0
		} else {
			mul(v, 65536.0).clamp(0.0, 65536.0).round() as i64
		}
	};
	let mut out: Vec<(i64, i64)> = Vec::with_capacity(pts.len());
	for p in pts {
		let (x, y) = (q(p.x), q(p.y));
		// Two points that share a 16.16 position keep the first.
		if out.last().is_some_and(|l| x <= l.0) {
			continue;
		}
		out.push((x, y));
	}
	out
}

/// The 256 entries of the monotone cubic through the points (Fritsch-Carlson with the box limit of three times
/// the secant on each side), flat before the first point and after the last one; `None` for no points.
fn curve_table(pts: &[(i64, i64)]) -> Option<[u8; 256]> {
	let n = pts.len();
	if n == 0 {
		return None;
	}
	let delta: Vec<i64> = (0..n.saturating_sub(1))
		.map(|k| ((pts[k + 1].1 - pts[k].1) << 16) / (pts[k + 1].0 - pts[k].0))
		.collect();
	let mut m = vec![0_i64; n];
	if n > 1 {
		m[0] = delta[0];
		m[n - 1] = delta[n - 2];
		for k in 1..n - 1 {
			if sign(delta[k - 1]) == sign(delta[k]) && sign(delta[k]) != 0 {
				m[k] = (delta[k - 1] + delta[k]) / 2;
			}
		}
		for (k, &d) in delta.iter().enumerate() {
			for e in [k, k + 1] {
				if d == 0 || sign(m[e]) != sign(d) {
					m[e] = 0;
				} else if m[e].abs() > 3 * d.abs() {
					m[e] = i64::from(sign(d)) * 3 * d.abs();
				}
				m[e] = m[e].clamp(-MAX_SLOPE, MAX_SLOPE);
			}
		}
	}
	let mut t = [0_u8; 256];
	for (v, out) in t.iter_mut().enumerate() {
		let x = (v as i64 * 65536 + 127) / 255;
		let y = if x <= pts[0].0 {
			pts[0].1
		} else if x >= pts[n - 1].0 {
			pts[n - 1].1
		} else {
			let mut k = 0;
			while x >= pts[k + 1].0 {
				k += 1;
			}
			let h = pts[k + 1].0 - pts[k].0;
			let u = (x - pts[k].0) * 65536 / h;
			let u2 = (u * u) >> 16;
			let u3 = (u2 * u) >> 16;
			let (h00, h10, h01, h11) = (
				2 * u3 - 3 * u2 + 65536,
				u3 - 2 * u2 + u,
				-2 * u3 + 3 * u2,
				u3 - u2,
			);
			(h00 * pts[k].1
				+ h01 * pts[k + 1].1
				+ h10 * ((h * m[k]) >> 16)
				+ h11 * ((h * m[k + 1]) >> 16)
				+ 32768)
				>> 16
		};
		*out = (((y.clamp(0, 65536)) * 255 + 32768) >> 16) as u8;
	}
	Some(t)
}

/// The curves of the spec as one table per channel (the master curve first, then the channel's own), or `None`.
fn build_curves(spec: &GradeSpec) -> Option<Box<[[u8; 256]; 3]>> {
	let master = curve_table(&curve_points(&spec.master));
	let chans = [
		curve_table(&curve_points(&spec.red)),
		curve_table(&curve_points(&spec.green)),
		curve_table(&curve_points(&spec.blue)),
	];
	if master.is_none() && chans.iter().all(Option::is_none) {
		return None;
	}
	let mut out = Box::new([[0_u8; 256]; 3]);
	for (table, own) in out.iter_mut().zip(&chans) {
		for (v, slot) in table.iter_mut().enumerate() {
			let mut o = v as u8;
			if let Some(m) = &master {
				o = m[usize::from(o)];
			}
			if let Some(c) = own {
				o = c[usize::from(o)];
			}
			*slot = o;
		}
	}
	Some(out)
}

/// `src` with the grade applied to its straight RGB values (SDD-004 section 3.14).
pub fn apply_grade(src: &Image, g: &Grade) -> Image {
	let mut out = Image {
		pix: vec![0; src.w * src.h * 4],
		w: src.w,
		h: src.h,
		opaque: src.opaque,
	};
	for (o, s) in out
		.pix
		.as_chunks_mut::<4>()
		.0
		.iter_mut()
		.zip(src.pix.as_chunks::<4>().0)
	{
		let a = i64::from(s[3]);
		let (mut r, mut gr, mut b) = (i64::from(s[0]), i64::from(s[1]), i64::from(s[2]));
		match a {
			0 => {
				o.copy_from_slice(s);
				continue;
			}
			255 => {}
			_ => {
				r = ((r * 255 + a / 2) / a).min(255);
				gr = ((gr * 255 + a / 2) / a).min(255);
				b = ((b * 255 + a / 2) / a).min(255);
			}
		}
		let (mut nr, mut ng, mut nb) = g.pixel(r, gr, b);
		if a != 255 {
			nr = div255_i64(nr * a);
			ng = div255_i64(ng * a);
			nb = div255_i64(nb * a);
		}
		o[0] = nr.min(255) as u8;
		o[1] = ng.min(255) as u8;
		o[2] = nb.min(255) as u8;
		o[3] = s[3];
	}
	out
}

#[cfg(test)]
mod tests {
	use super::*;

	fn pt(x: f64, y: f64) -> CurvePoint {
		CurvePoint { x, y }
	}

	fn spec() -> GradeSpec {
		let mut g = GradeSpec::default();
		g.hsl[0] = HslBand {
			hue: 0.5,
			saturation: 0.4,
			lightness: -0.2,
		};
		g.hsl[3] = HslBand {
			hue: 0.0,
			saturation: -0.7,
			lightness: 0.0,
		};
		g.wheels[0] = Some(Wheel {
			hue: 220.0,
			amount: 0.6,
			luma: -0.2,
		});
		g.wheels[2] = Some(Wheel {
			hue: 359.9,
			amount: 0.8,
			luma: 0.3,
		});
		g.master = vec![pt(0.0, 0.1), pt(0.3, 0.2), pt(0.7, 0.9), pt(1.0, 1.0)];
		g.blue = vec![pt(0.0, 1.0), pt(1.0, 0.0)];
		g
	}

	/// The same pixels are pinned in `engine/compositor/grade_test.go`.
	#[test]
	fn pixels_are_pinned() {
		let g = Grade::new(&spec()).unwrap();
		for (input, want) in [
			((200, 60, 40), (230, 33, 221)),
			((30, 220, 80), (102, 206, 184)),
			((128, 128, 128), (181, 100, 101)),
			((0, 0, 0), (26, 26, 208)),
			((255, 255, 255), (255, 246, 9)),
			((250, 250, 20), (255, 247, 229)),
			((10, 20, 240), (26, 26, 0)),
		] {
			assert_eq!(g.pixel(input.0, input.1, input.2), want, "{input:?}");
		}
	}

	#[test]
	fn a_grade_that_changes_nothing_is_none() {
		assert!(Grade::new(&GradeSpec::default()).is_none());
		let mut g = GradeSpec::default();
		g.wheels[1] = Some(Wheel {
			hue: 120.0,
			amount: 0.0,
			luma: 0.0,
		});
		assert!(Grade::new(&g).is_none());
	}

	#[test]
	fn greys_and_the_identity_curve_are_untouched() {
		let mut s = GradeSpec::default();
		s.hsl[0] = HslBand {
			hue: 1.0,
			saturation: 1.0,
			lightness: 1.0,
		};
		let g = Grade::new(&s).unwrap();
		for v in (0..256).step_by(5) {
			assert_eq!(g.pixel(v, v, v), (v, v, v));
		}
		let id = GradeSpec {
			master: vec![pt(0.0, 0.0), pt(1.0, 1.0)],
			..GradeSpec::default()
		};
		let g = Grade::new(&id).unwrap();
		for v in 0..256 {
			assert_eq!(g.pixel(v, v, v), (v, v, v));
		}
	}

	#[test]
	fn a_curve_through_rising_points_never_falls() {
		let s = GradeSpec {
			master: vec![
				pt(0.0, 0.0),
				pt(0.1, 0.05),
				pt(0.2, 0.9),
				pt(0.5, 0.9),
				pt(0.6, 0.95),
				pt(1.0, 1.0),
			],
			..GradeSpec::default()
		};
		let g = Grade::new(&s).unwrap();
		let t = g.curves.as_ref().unwrap();
		assert!(t[0].windows(2).all(|w| w[0] <= w[1]));
	}

	#[test]
	fn nan_and_huge_parameters_do_not_panic() {
		let mut s = GradeSpec::default();
		s.hsl[2] = HslBand {
			hue: f64::NAN,
			saturation: f64::MAX,
			lightness: f64::MIN,
		};
		s.wheels[1] = Some(Wheel {
			hue: f64::INFINITY,
			amount: f64::NAN,
			luma: 1e300,
		});
		s.red = vec![
			pt(f64::NAN, 2.0),
			pt(0.5, f64::MAX),
			pt(0.5, 0.0),
			pt(-1.0, 3.0),
		];
		let g = Grade::new(&s).unwrap();
		for v in 0..256 {
			g.pixel(v, 255 - v, v / 2);
		}
	}
}
