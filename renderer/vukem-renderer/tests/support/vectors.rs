//! The vectors of `renderer/testdata/compose-vectors.bin` (written by
//! `engine/compositor/vectors_test.go`), read for the CPU parity test and the GPU one.

use vukem_renderer::blend::BlendMode;
use vukem_renderer::chroma::ChromaKeySpec;
use vukem_renderer::compose::{Layer, Source, premultiply};
use vukem_renderer::effects::{Direction, Effect, FrameInfo, Transition, TransitionKind};
use vukem_renderer::grade::{CurvePoint, GradeSpec, HslBand, Wheel};
use vukem_renderer::image::Image;
use vukem_renderer::lut::Lut;
use vukem_renderer::mask::{MaskShape, MaskSpec};

const VECTORS: &[u8] = include_bytes!("../../../testdata/compose-vectors.bin");

struct Reader<'a>(&'a [u8]);

impl<'a> Reader<'a> {
	fn take(&mut self, n: usize) -> &'a [u8] {
		let (a, b) = self.0.split_at(n);
		self.0 = b;
		a
	}
	fn u8(&mut self) -> u8 {
		self.take(1)[0]
	}
	fn u32(&mut self) -> u32 {
		u32::from_le_bytes(self.take(4).try_into().unwrap())
	}
	fn u64(&mut self) -> u64 {
		u64::from_le_bytes(self.take(8).try_into().unwrap())
	}
	fn f64(&mut self) -> f64 {
		f64::from_bits(self.u64())
	}
	fn matrix(&mut self) -> Option<[f64; 12]> {
		(self.u8() == 1).then(|| core::array::from_fn(|_| self.f64()))
	}
}

pub enum Pixels {
	Media(Image),
	Bitmap {
		image: Image,
		natural_w: f64,
		center_dx: f64,
		center_dy: f64,
	},
}

pub struct VecLayer {
	pub pixels: Pixels,
	pub nums: [f64; 7],
	pub colour: Option<[f64; 12]>,
	pub filter: Option<[f64; 12]>,
	pub chroma_key: Option<ChromaKeySpec>,
	pub blend: BlendMode,
	pub mask: Option<MaskSpec>,
	pub flip_x: bool,
	pub flip_y: bool,
	pub effects: Vec<Effect>,
	pub transition: Option<Transition>,
	pub grade: Option<GradeSpec>,
	pub lut: Option<(Lut, f64)>,
}

pub struct Case {
	pub name: String,
	pub w: usize,
	pub h: usize,
	pub scale: f64,
	pub bg: [u8; 4],
	pub frame: FrameInfo,
	pub layers: Vec<VecLayer>,
	pub want: Vec<u8>,
}

pub fn read_cases() -> (u64, Vec<Case>) {
	let mut r = Reader(VECTORS);
	assert_eq!(r.take(4), b"VVCV");
	assert_eq!(r.u32(), 8);
	let hash = r.u64();
	let n = r.u32();
	let mut cases = Vec::new();
	for _ in 0..n {
		let len = usize::from(r.u8());
		let name = String::from_utf8(r.take(len).to_vec()).unwrap();
		let (w, h) = (r.u32() as usize, r.u32() as usize);
		let scale = r.f64();
		let bg: [u8; 4] = r.take(4).try_into().unwrap();
		let frame = FrameInfo {
			frame: i64::from(r.u32()),
			width: i64::from(r.u32()),
			height: i64::from(r.u32()),
		};
		let nl = r.u32();
		let mut layers = Vec::new();
		for _ in 0..nl {
			let kind = r.u8();
			let (sw, sh) = (r.u32() as usize, r.u32() as usize);
			let opaque = r.u8() == 1;
			let pix = r.take(sw * sh * 4).to_vec();
			let pixels = if kind == 1 {
				let natural_w = r.f64();
				let (center_dx, center_dy) = (r.f64(), r.f64());
				Pixels::Bitmap {
					image: premultiply(&pix, sw, sh),
					natural_w,
					center_dx,
					center_dy,
				}
			} else {
				Pixels::Media(Image {
					pix,
					w: sw,
					h: sh,
					opaque,
				})
			};
			let nums = core::array::from_fn(|_| r.f64());
			let (colour, filter) = (r.matrix(), r.matrix());
			let chroma_key = (r.u8() == 1).then(|| {
				let n = usize::from(r.u8());
				let color = String::from_utf8(r.take(n).to_vec()).unwrap();
				ChromaKeySpec {
					color,
					similarity: r.f64(),
					smoothness: r.f64(),
					spill: r.f64(),
				}
			});
			let blend = match r.u8() {
				0 => BlendMode::Normal,
				1 => BlendMode::Multiply,
				2 => BlendMode::Screen,
				3 => BlendMode::Overlay,
				4 => BlendMode::Add,
				n => panic!("unknown blend mode {n}"),
			};
			let mask = (r.u8() == 1).then(|| {
				let shape = if r.u8() == 1 {
					MaskShape::Ellipse
				} else {
					MaskShape::Rect
				};
				MaskSpec {
					shape,
					x: r.f64(),
					y: r.f64(),
					width: r.f64(),
					height: r.f64(),
					feather: f64::from(r.u32()),
					invert: r.u8() == 1,
				}
			});
			let flips = r.u8();
			let effects = (0..r.u8())
				.map(|_| {
					let kind = r.u8();
					let n = usize::from(r.u8());
					let v: Vec<f64> = (0..n).map(|_| r.f64()).collect();
					match kind {
						0 => Effect::MotionBlur {
							amount: v[0],
							angle: v[1],
						},
						1 => Effect::Glow {
							amount: v[0],
							radius: v[1],
							threshold: v[2],
						},
						2 => Effect::Vignette {
							amount: v[0],
							softness: v[1],
						},
						3 => Effect::Grain {
							amount: v[0],
							size: v[1],
							seed: v[2] as i64,
						},
						4 => Effect::RgbSplit {
							amount: v[0],
							angle: v[1],
						},
						n => panic!("unknown effect {n}"),
					}
				})
				.collect();
			let transition = match r.u8() {
				0 => None,
				k => {
					let kind = match k {
						1 => TransitionKind::Whip,
						2 => TransitionKind::Dip,
						_ => TransitionKind::Wipe,
					};
					let _edge = r.u8();
					let p = i64::from(r.u32());
					let direction = match r.u8() {
						0 => Direction::Left,
						1 => Direction::Right,
						2 => Direction::Up,
						_ => Direction::Down,
					};
					let colour: [u8; 4] = r.take(4).try_into().unwrap();
					Some(Transition {
						kind,
						p,
						direction,
						colour,
					})
				}
			};
			let grade = (r.u8() == 1).then(|| {
				let mut g = GradeSpec::default();
				for b in &mut g.hsl {
					*b = HslBand {
						hue: r.f64(),
						saturation: r.f64(),
						lightness: r.f64(),
					};
				}
				for w in &mut g.wheels {
					*w = (r.u8() == 1).then(|| Wheel {
						hue: r.f64(),
						amount: r.f64(),
						luma: r.f64(),
					});
				}
				let mut curve = || -> Vec<CurvePoint> {
					(0..r.u8())
						.map(|_| CurvePoint {
							x: r.f64(),
							y: r.f64(),
						})
						.collect()
				};
				g.master = curve();
				g.red = curve();
				g.green = curve();
				g.blue = curve();
				g
			});
			let lut = (r.u8() == 1).then(|| {
				let size = r.u32() as usize;
				let intensity = r.f64();
				let data = (0..size * size * size * 3)
					.map(|_| u16::from_le_bytes(r.take(2).try_into().unwrap()))
					.collect();
				(Lut::new(size, data).unwrap(), intensity)
			});
			layers.push(VecLayer {
				pixels,
				nums,
				colour,
				filter,
				chroma_key,
				blend,
				mask,
				flip_x: flips & 1 != 0,
				flip_y: flips & 2 != 0,
				effects,
				transition,
				grade,
				lut,
			});
		}
		let want = r.take(w * h * 4).to_vec();
		cases.push(Case {
			name,
			w,
			h,
			scale,
			bg,
			frame,
			layers,
			want,
		});
	}
	assert!(r.0.is_empty());
	(hash, cases)
}

/// The layers of a vector, borrowing its pixels.
pub fn layers_of(c: &Case) -> Vec<Layer<'_>> {
	c.layers
		.iter()
		.map(|l| {
			let [cx, cy, w, h, rotation, opacity, blur] = l.nums;
			let source = match &l.pixels {
				Pixels::Media(img) => Source::Media(img),
				Pixels::Bitmap {
					image,
					natural_w,
					center_dx,
					center_dy,
				} => Source::Bitmap {
					image,
					natural_w: *natural_w,
					center_dx: *center_dx,
					center_dy: *center_dy,
				},
			};
			Layer {
				source,
				cx,
				cy,
				w,
				h,
				rotation,
				opacity,
				color_matrix: l.colour,
				filter: l.filter,
				grade: l.grade.clone(),
				lut: l.lut.as_ref().map(|(t, i)| (t, *i)),
				chroma_key: l.chroma_key.clone(),
				blend: l.blend,
				mask: l.mask,
				flip_x: l.flip_x,
				flip_y: l.flip_y,
				effects: l.effects.clone(),
				transition: l.transition,
				frame: c.frame,
				blur,
				shadow: None,
				outline: None,
			}
		})
		.collect()
}
