//! Fuzzing of the plan decoder and of the composition (rule `.agents/rules/rust.md` 8, SDD-013
//! section 7.4), with cargo's own test runner and a generator written here.
//!
//! `cargo fuzz` and `proptest` are not approved crates (AGENTS.md rule 9), so the inputs come from a
//! fixed-seed xorshift generator: random bytes, valid inputs mutated byte by byte and token by
//! token, and compositions whose numbers are the extremes of a double. The tests run on every
//! `cargo test`, are the same on every system, and fail on any panic (the overflow checks of the
//! test profile are on, so an arithmetic overflow is a panic too). `VR_FUZZ_ITERATIONS` scales
//! them for a long run.

use vukem_renderer::blend::BlendMode;
use vukem_renderer::chroma::ChromaKeySpec;
use vukem_renderer::compose::{Layer, Placement, Source, Target, compose, sample};
use vukem_renderer::effects::{Direction, Effect, FrameInfo, Transition, TransitionKind};
use vukem_renderer::error::Reason;
use vukem_renderer::grade::{CurvePoint, GradeSpec, HslBand, Wheel};
use vukem_renderer::host::{BitmapMetrics, Limits, Renderer};
use vukem_renderer::image::Image;
use vukem_renderer::lut::Lut;
use vukem_renderer::mask::{MaskShape, MaskSpec, MaskState};
use vukem_renderer::plan::{MAX_LAYERS, MAX_OUTPUT_SIDE, Pixels, decode};

struct Rng(u64);

impl Rng {
	fn next(&mut self) -> u64 {
		let mut x = self.0;
		x ^= x >> 12;
		x ^= x << 25;
		x ^= x >> 27;
		self.0 = x;
		x.wrapping_mul(0x2545_F491_4F6C_DD1D)
	}
	fn below(&mut self, n: usize) -> usize {
		(self.next() % n as u64) as usize
	}
	fn chance(&mut self, one_in: usize) -> bool {
		self.below(one_in) == 0
	}
	fn pick<'a, T>(&mut self, items: &'a [T]) -> &'a T {
		&items[self.below(items.len())]
	}
	fn unit(&mut self) -> f64 {
		(self.next() >> 11) as f64 / (1_u64 << 53) as f64
	}
}

fn iterations(default: usize) -> usize {
	std::env::var("VR_FUZZ_ITERATIONS")
		.ok()
		.and_then(|v| v.parse().ok())
		.unwrap_or(default)
}

const VALID: &[&str] = &[
	r##"{"version":1,"output":{"width":16,"height":12,"scale":1},"plan":{"frame":3,"width":16,"height":12,"background":"#102030","layers":[{"item":"a","kind":"media","asset":"x","asset_kind":"video","source_frame":2,"crop":{"x":0,"y":0,"w":8,"h":8},"color_matrix":null,"filter":null,"blur":0,"cx":8,"cy":6,"w":8,"h":8,"rotation":15,"scale":1,"opacity":0.5}]}}"##,
	r##"{"version":1,"output":{"width":16,"height":12,"scale":0.5,"y0":2,"y1":9},"plan":{"layers":[{"item":"b","kind":"media","color_matrix":[1,0,0,0,0,1,0,0,0,0,1,0],"filter":[0.3,0.6,0.1,0,0.3,0.6,0.1,0,0.3,0.6,0.1,0],"blur":2.5,"chroma_key":{"color":"#00ff00","similarity":0.3,"smoothness":0.1,"spill":0.5},"cx":8,"cy":6,"w":10,"h":10,"rotation":-30,"opacity":1}]}}"##,
	r##"{"version":1,"output":{"width":24,"height":16,"scale":1},"plan":{"frame":5,"width":24,"height":16,"layers":[{"item":"a","kind":"media","cx":12,"cy":8,"w":12,"h":10,"rotation":10,"opacity":1,"effects":[{"kind":"motion_blur","amount":0.6,"angle":30},{"kind":"glow","amount":0.5,"radius":4,"threshold":0.3},{"kind":"vignette","amount":0.7,"softness":0.5},{"kind":"grain","amount":0.4,"size":2,"seed":9},{"kind":"rgb_split","amount":0.5,"angle":-40}],"transition":{"kind":"whip","edge":"in","p":9000,"direction":"up"}},{"item":"b","kind":"media","cx":6,"cy":6,"w":8,"h":8,"opacity":0.8,"transition":{"kind":"wipe","edge":"out","p":30000,"direction":"right"},"mask":{"shape":"rect","x":0,"y":0,"width":1,"height":1}},{"item":"b","kind":"media","cx":18,"cy":6,"w":8,"h":8,"opacity":1,"transition":{"kind":"dip","edge":"in","p":20000,"color":"#ff8000"}}]}}"##,
	r##"{"version":1,"output":{"width":16,"height":12,"scale":1},"plan":{"background":"#ffffff","layers":[{"item":"t","kind":"bitmap","bitmap":{"kind":"text","text":"hi","words":["hi"],"breaks":[],"active_word":-1,"pixel_scale":1},"bitmap_key":"k1","cx":8,"cy":6,"w":8,"h":4,"rotation":0,"opacity":1},{"item":"a","kind":"media","cx":4,"cy":4,"w":8,"h":8,"rotation":90,"opacity":0.75}]}}"##,
];

const TOKENS: &[&str] = &[
	"\"effects\"",
	"\"transition\"",
	"\"whip\"",
	"\"glow\"",
	"\"grain\"",
	"65536",
	"65537",
	"null",
	"true",
	"{}",
	"[]",
	"\"\"",
	"-0",
	"1e308",
	"-1e308",
	"1e-320",
	"1e999",
	"NaN",
	"0.1",
	"4096",
	"4097",
	"65",
	"12",
	"-1",
	"9007199254740993",
	"\\u0000",
	"\\ud800",
	"\"version\"",
	"\"opacity\"",
	"\"layers\"",
	"\"kind\"",
	"\"bitmap_key\"",
	"\"chroma_key\"",
	"\"blend\"",
	"\"multiply\"",
	"\"mask\"",
	"\"ellipse\"",
	"\"feather\"",
	"\"similarity\"",
	",",
	":",
	"{\"a\":1}",
	"[1,2,3,4,5,6,7,8,9,10,11,12]",
	"\"media\"",
	"\"bitmap\"",
	"\u{e9}",
	"\u{1f600}",
];

fn mutate(rng: &mut Rng, src: &str) -> Vec<u8> {
	let mut b = src.as_bytes().to_vec();
	let rounds = if rng.chance(4) { 2 + rng.below(3) } else { 1 };
	for _ in 0..rounds {
		if b.is_empty() {
			b.push(b'{');
			continue;
		}
		match rng.below(7) {
			0 => {
				let i = rng.below(b.len());
				b[i] = rng.next() as u8;
			}
			1 => {
				let i = rng.below(b.len());
				b.truncate(i);
			}
			2 => {
				let i = rng.below(b.len());
				let n = rng.below(b.len() - i).min(40);
				b.drain(i..i + n);
			}
			3 => {
				let i = rng.below(b.len() + 1);
				let t = rng.pick(TOKENS).as_bytes().to_vec();
				b.splice(i..i, t);
			}
			4 => {
				let i = rng.below(b.len());
				let n = rng.below(b.len() - i).min(60);
				let copy = b[i..i + n].to_vec();
				let at = rng.below(b.len() + 1);
				b.splice(at..at, copy);
			}
			5 => {
				// Replace a digit run with a token.
				if let Some(i) = b
					.iter()
					.position(|c| c.is_ascii_digit())
					.map(|s| (s + rng.below(b.len() - s)) % b.len())
				{
					let t = rng.pick(TOKENS).as_bytes().to_vec();
					b.splice(i..=i, t);
				}
			}
			_ => {
				let i = rng.below(b.len());
				b[i] ^= 1 << rng.below(8);
			}
		}
	}
	b
}

fn check_decoded(input: &[u8]) {
	match decode(input) {
		Ok(r) => {
			assert!(r.layers.len() <= MAX_LAYERS);
			assert!((1..=MAX_OUTPUT_SIDE).contains(&r.output.width));
			assert!((1..=MAX_OUTPUT_SIDE).contains(&r.output.height));
			assert!(
				r.output.rows.start < r.output.rows.end && r.output.rows.end <= r.output.height
			);
			assert!(r.output.scale > 0.0 && r.output.scale.is_finite());
			for l in &r.layers {
				assert!(!l.item.is_empty() && l.item.len() <= 64 && l.item.is_ascii());
				for v in [l.cx, l.cy, l.w, l.h, l.rotation, l.opacity, l.blur] {
					assert!(v.is_finite());
				}
				let (Pixels::Bitmap(k) | Pixels::Media(k)) = &l.pixels;
				assert!(!k.is_empty() && k.len() <= 64);
			}
		}
		Err(e) => assert!(
			e.reason.status() < 0 && e.reason.status() >= -7 && e.reason != Reason::OutOfMemory
		),
	}
}

#[test]
fn the_decoder_survives_random_bytes() {
	let mut rng = Rng(0x9E37_79B9_7F4A_7C15);
	let alphabet = br#"{}[]",:0123456789.eE+-tfnrulas \"#;
	for i in 0..iterations(100_000) {
		let len = rng.below(200);
		let input: Vec<u8> = (0..len)
			.map(|_| {
				if i % 2 == 0 {
					rng.next() as u8
				} else {
					alphabet[rng.below(alphabet.len())]
				}
			})
			.collect();
		check_decoded(&input);
	}
}

#[test]
fn the_decoder_survives_mutated_inputs() {
	let mut rng = Rng(0xD1B5_4A32_D192_ED03);
	for src in VALID {
		check_decoded(src.as_bytes());
	}
	let mut accepted = 0;
	let n = iterations(200_000);
	for _ in 0..n {
		let src = *rng.pick(VALID);
		let input = mutate(&mut rng, src);
		check_decoded(&input);
		accepted += usize::from(decode(&input).is_ok());
	}
	// The mutations reach both the accepting and the refusing paths of the decoder.
	assert!(
		accepted > n / 50 && accepted < n,
		"{accepted} of {n} accepted"
	);
}

#[test]
fn the_decoder_refuses_deep_and_large_inputs() {
	let deep = "[".repeat(100_000);
	assert_eq!(
		decode(deep.as_bytes()).unwrap_err().reason,
		Reason::InvalidPlan
	);
	let wide = format!("[{}]", "1,".repeat(3_000_000));
	assert_eq!(
		decode(wide.as_bytes()).unwrap_err().reason,
		Reason::LimitExceeded
	);
	let long_key = format!("{{\"{}\":1}}", "k".repeat(3_000_000));
	assert_eq!(
		decode(long_key.as_bytes()).unwrap_err().reason,
		Reason::InvalidPlan
	);
}

fn extreme(rng: &mut Rng) -> f64 {
	const SPECIAL: &[f64] = &[
		0.0,
		-0.0,
		1.0,
		-1.0,
		0.5,
		255.0,
		1e-300,
		-1e-300,
		1e300,
		-1e300,
		f64::MAX,
		f64::MIN,
		f64::MIN_POSITIVE,
		f64::NAN,
		f64::INFINITY,
		f64::NEG_INFINITY,
		4096.0,
		1e9,
		-1e9,
		9_007_199_254_740_992.0,
	];
	match rng.below(4) {
		0 => *rng.pick(SPECIAL),
		1 => (rng.unit() - 0.5) * 200.0,
		2 => (rng.unit() - 0.5) * 20.0,
		_ => rng.unit(),
	}
}

fn random_image(rng: &mut Rng, premultiplied: bool) -> Image {
	let (w, h) = (1 + rng.below(24), 1 + rng.below(24));
	let mut pix: Vec<u8> = (0..w * h * 4).map(|_| rng.next() as u8).collect();
	if premultiplied && rng.chance(2) {
		for p in pix.as_chunks_mut::<4>().0.iter_mut() {
			let a = u32::from(p[3]);
			for c in &mut p[..3] {
				*c = (u32::from(*c) * a / 255) as u8;
			}
		}
	}
	let opaque = rng.chance(4);
	if opaque {
		for p in pix.as_chunks_mut::<4>().0.iter_mut() {
			p[3] = 255;
		}
	}
	Image { pix, w, h, opaque }
}

const BLENDS: [BlendMode; 5] = [
	BlendMode::Normal,
	BlendMode::Multiply,
	BlendMode::Screen,
	BlendMode::Overlay,
	BlendMode::Add,
];

fn mask(rng: &mut Rng) -> MaskSpec {
	let fraction = |rng: &mut Rng| {
		if rng.chance(2) {
			extreme(rng)
		} else {
			rng.below(1001) as f64 / 1000.0
		}
	};
	MaskSpec {
		shape: if rng.chance(2) {
			MaskShape::Ellipse
		} else {
			MaskShape::Rect
		},
		x: fraction(rng),
		y: fraction(rng),
		width: fraction(rng),
		height: fraction(rng),
		feather: if rng.chance(2) {
			extreme(rng)
		} else {
			rng.below(60) as f64
		},
		invert: rng.chance(2),
	}
}

fn chroma_key(rng: &mut Rng) -> Option<ChromaKeySpec> {
	const COLOURS: &[&str] = &[
		"#00ff00",
		"#808080",
		"#ff0000ff",
		"#zzzzzz",
		"",
		"#12",
		"#0000FF",
		"00ff00",
	];
	rng.chance(3).then(|| ChromaKeySpec {
		color: (*rng.pick(COLOURS)).to_string(),
		similarity: extreme(rng),
		smoothness: extreme(rng),
		spill: extreme(rng),
	})
}

fn effect(rng: &mut Rng) -> Effect {
	match rng.below(5) {
		0 => Effect::MotionBlur {
			amount: extreme(rng),
			angle: extreme(rng),
		},
		1 => Effect::Glow {
			amount: extreme(rng),
			radius: extreme(rng),
			threshold: extreme(rng),
		},
		2 => Effect::Vignette {
			amount: extreme(rng),
			softness: extreme(rng),
		},
		3 => Effect::Grain {
			amount: extreme(rng),
			size: extreme(rng),
			seed: rng.next() as i64,
		},
		_ => Effect::RgbSplit {
			amount: extreme(rng),
			angle: extreme(rng),
		},
	}
}

fn transition(rng: &mut Rng) -> Transition {
	Transition {
		kind: *rng.pick(&[
			TransitionKind::Whip,
			TransitionKind::Dip,
			TransitionKind::Wipe,
		]),
		p: rng.next() as i64 >> rng.below(64),
		direction: *rng.pick(&[
			Direction::Left,
			Direction::Right,
			Direction::Up,
			Direction::Down,
		]),
		colour: [rng.next() as u8, rng.next() as u8, rng.next() as u8, 255],
	}
}

fn curve(rng: &mut Rng) -> Vec<CurvePoint> {
	(0..rng.below(9))
		.map(|_| CurvePoint {
			x: extreme(rng),
			y: extreme(rng),
		})
		.collect()
}

fn grade(rng: &mut Rng) -> Option<GradeSpec> {
	rng.chance(3).then(|| {
		let mut g = GradeSpec::default();
		for b in &mut g.hsl {
			*b = HslBand {
				hue: extreme(rng),
				saturation: extreme(rng),
				lightness: extreme(rng),
			};
		}
		for w in &mut g.wheels {
			*w = rng.chance(2).then(|| Wheel {
				hue: extreme(rng),
				amount: extreme(rng),
				luma: extreme(rng),
			});
		}
		g.master = curve(rng);
		g.red = curve(rng);
		g.green = curve(rng);
		g.blue = curve(rng);
		g
	})
}

fn random_lut(rng: &mut Rng) -> Lut {
	let size = 2 + rng.below(4);
	let data = (0..size * size * size * 3)
		.map(|_| rng.next() as u16)
		.collect();
	Lut::new(size, data).unwrap()
}

fn matrix(rng: &mut Rng) -> Option<[f64; 12]> {
	rng.chance(3)
		.then(|| core::array::from_fn(|_| extreme(rng)))
}

#[test]
fn the_composition_survives_extreme_numbers() {
	let mut rng = Rng(0xA076_1D64_78BD_642F);
	for i in 0..iterations(6_000) {
		let images: Vec<Image> = (0..1 + rng.below(3))
			.map(|_| random_image(&mut rng, true))
			.collect();
		let tables: Vec<Lut> = (0..2).map(|_| random_lut(&mut rng)).collect();
		let layers: Vec<Layer<'_>> = images
			.iter()
			.map(|img| {
				let source = if rng.chance(3) {
					Source::Bitmap {
						image: img,
						natural_w: extreme(&mut rng),
						center_dx: extreme(&mut rng),
						center_dy: extreme(&mut rng),
					}
				} else {
					Source::Media(img)
				};
				Layer {
					source,
					cx: extreme(&mut rng),
					cy: extreme(&mut rng),
					w: extreme(&mut rng),
					h: extreme(&mut rng),
					rotation: extreme(&mut rng),
					opacity: extreme(&mut rng),
					color_matrix: matrix(&mut rng),
					filter: matrix(&mut rng),
					grade: grade(&mut rng),
					lut: rng
						.chance(3)
						.then(|| (&tables[rng.below(2)], extreme(&mut rng))),
					chroma_key: chroma_key(&mut rng),
					blend: *rng.pick(&BLENDS),
					mask: rng.chance(2).then(|| mask(&mut rng)),
					flip_x: rng.chance(2),
					flip_y: rng.chance(2),
					effects: if rng.chance(8) {
						(0..1 + rng.below(3)).map(|_| effect(&mut rng)).collect()
					} else {
						Vec::new()
					},
					transition: rng.chance(8).then(|| transition(&mut rng)),
					shadow: None,
					outline: None,
					frame: FrameInfo {
						frame: rng.next() as i64,
						width: rng.next() as i64 >> rng.below(64),
						height: rng.next() as i64 >> rng.below(64),
					},
					// A blur of a huge radius is capped at 128 source pixels; keep most small.
					blur: if rng.chance(3) {
						extreme(&mut rng)
							.abs()
							.min(if i % 100 == 0 { 1e9 } else { 12.0 })
					} else {
						0.0
					},
				}
			})
			.collect();
		let (w, h) = (1 + rng.below(40), 1 + rng.below(40));
		let start = rng.below(h + 3);
		let rows = start..start + rng.below(h + 3);
		let mut pix = vec![0x5A_u8; w * h * 4];
		let before = pix.clone();
		compose(
			&mut Target {
				pix: &mut pix,
				width: w,
				height: h,
				rows: rows.clone(),
			},
			[1, 2, 3, 255],
			&layers,
			extreme(&mut rng),
		);

		// Rows outside the band are never touched.
		let (a, b) = (rows.start.min(h), rows.end.min(h));
		assert!(
			pix[..a * w * 4] == before[..a * w * 4]
				&& pix[b.max(a) * w * 4..] == before[b.max(a) * w * 4..]
		);
	}
}

#[test]
fn the_sampler_survives_every_rotation_and_scale() {
	let mut rng = Rng(0xE703_7ED1_A0B4_28DB);
	for _ in 0..iterations(30_000) {
		let img = random_image(&mut rng, true);
		let (w, h) = (1 + rng.below(30), 1 + rng.below(30));
		let mut pix = vec![0_u8; w * h * 4];
		let p = Placement {
			cx: extreme(&mut rng),
			cy: extreme(&mut rng),
			w: extreme(&mut rng),
			h: extreme(&mut rng),
			rot: extreme(&mut rng),
		};
		let state = rng
			.chance(2)
			.then(|| MaskState::new(Some(&mask(&mut rng)), None, &p, extreme(&mut rng)));
		sample(
			&mut Target {
				pix: &mut pix,
				width: w,
				height: h,
				rows: 0..h,
			},
			&img,
			&p,
			rng.below(256) as u32,
			*rng.pick(&BLENDS),
			state.as_ref(),
		);
	}
}

#[test]
fn the_renderer_survives_mutated_inputs_and_is_repeatable() {
	let mut rng = Rng(0x8EBC_6AF0_9C88_C6E3);
	let mut host = Renderer::new(Limits::default());
	let a = random_image(&mut rng, true);
	host.put_texture("a", a.w, a.h, &a.pix, a.opaque).unwrap();
	host.put_texture("b", a.w, a.h, &a.pix, false).unwrap();
	let bm = random_image(&mut rng, false);
	let metrics = BitmapMetrics {
		natural_w: 8.0,
		natural_h: 4.0,
		center_dx: 1.0,
		center_dy: -1.0,
	};
	host.put_bitmap("k1", bm.w, bm.h, &bm.pix, metrics).unwrap();

	let (mut drawn, n) = (0, iterations(50_000));
	for _ in 0..n {
		let src = rng_pick(&mut rng);
		let input = mutate(&mut rng, src);
		let Ok(r) = decode(&input) else { continue };
		let mut out = vec![0x77_u8; r.output.width * r.output.height * 4];
		if r.output.width * r.output.height > 64 * 64 {
			continue;
		}
		match host.render(&input, &mut out) {
			Ok(()) => {
				drawn += 1;
				let mut again = vec![0x77_u8; out.len()];
				host.render(&input, &mut again).unwrap();
				assert!(out == again, "a render is not repeatable");
			}
			Err(e) => assert!(
				matches!(e.reason, Reason::MissingTexture | Reason::InvalidArgument),
				"{e}"
			),
		}
	}
	assert!(drawn > 0, "no mutated input ever drew a frame");
}

fn rng_pick(rng: &mut Rng) -> &'static str {
	VALID[rng.below(VALID.len())]
}

#[test]
fn the_store_survives_every_size_and_length() {
	let mut rng = Rng(0x5851_F42D_4C95_7F2D);
	let mut host = Renderer::new(Limits {
		max_texture_bytes: 1 << 16,
		max_bitmap_bytes: 1 << 14,
	});
	let metrics = BitmapMetrics {
		natural_w: 3.0,
		natural_h: 3.0,
		center_dx: 0.0,
		center_dy: 0.0,
	};
	for _ in 0..iterations(6_000) {
		let (w, h) = (rng.below(300), rng.below(300));
		let len = rng.below(w * h * 4 + 8);
		let pix = vec![rng.next() as u8; len];
		let id = "x".repeat(rng.below(70));
		let r = if rng.chance(2) {
			host.put_texture(&id, w, h, &pix, rng.chance(2))
		} else {
			host.put_bitmap(&id, w, h, &pix, metrics)
		};
		if let Err(e) = r {
			assert!(matches!(
				e.reason,
				Reason::InvalidArgument | Reason::LimitExceeded
			));
		}
		if rng.chance(3) {
			host.release(&id);
		}
	}
}
