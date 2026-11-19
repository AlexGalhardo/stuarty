//! The render input of SDD-013 section 2, decoded and validated.
//!
//! The renderer reads the JSON of `core/plan` (`FramePlan`) inside the envelope of section 2.1. It
//! refuses what it does not know: an unknown field is a pixel it would silently not draw.

use crate::blend::BlendMode;
use crate::chroma::ChromaKeySpec;
use crate::compose::{OutlineSpec, ShadowSpec, parse_colour};
use crate::effects::{Direction, Effect, FrameInfo, Transition, TransitionKind};
use crate::error::{Error, Reason};
use crate::filters::FilterOp;
use crate::grade::{CurvePoint, GradeSpec, HSL_RANGES, HslBand, Wheel};
use crate::json::{JsonError, Value, parse};
use crate::mask::{MaskShape, MaskSpec};

/// The version of the render input this module reads (`RenderInputVersion`, SDD-000 section 12).
pub const RENDER_INPUT_VERSION: i64 = 1;
/// The largest side of an output frame.
pub const MAX_OUTPUT_SIDE: usize = 4096;
/// The most layers in one plan.
pub const MAX_LAYERS: usize = 64;
/// The largest render input.
pub const MAX_INPUT_BYTES: usize = 4 << 20;
/// The longest item id or bitmap key.
pub const MAX_ID_BYTES: usize = 64;
/// The most effects on one layer: each kind at most once (SDD-004 section 3.13).
pub const MAX_EFFECTS: usize = 6;
/// The most points of one tone curve (SDD-001 section 3.5).
pub const MAX_CURVE_POINTS: usize = 8;
/// The most filters on one layer (SDD-013 section 3.11.3).
pub const MAX_FILTERS: usize = 8;
/// The entries of the tone table of a layer (SDD-013 section 3.11.1).
pub const TONE_LEN: usize = 256;

/// The frame to write.
#[derive(Clone, Debug, PartialEq)]
pub struct Output {
	pub width: usize,
	pub height: usize,
	pub scale: f64,
	/// The rows `[y0, y1)` to draw, the whole frame when absent.
	pub rows: core::ops::Range<usize>,
}

/// Where the pixels of a layer are held.
#[derive(Clone, Debug, PartialEq)]
pub enum Pixels {
	/// The texture of this item id.
	Media(String),
	/// The bitmap of this key.
	Bitmap(String),
}

/// The lookup table of a layer: the lut asset and how much of it is mixed in.
#[derive(Clone, Debug, PartialEq)]
pub struct LutRef {
	pub asset: String,
	pub intensity: f64,
}

/// The mask of a layer from the alpha of another layer of the plan (SDD-013 section 3.11.4).
#[derive(Clone, Debug, PartialEq)]
pub struct ClipSpec {
	/// The item id of the layer that gives the alpha.
	pub source: String,
	pub invert: bool,
}

/// One layer of the plan, as the renderer reads it.
#[derive(Clone, Debug, PartialEq)]
pub struct LayerSpec {
	pub item: String,
	pub pixels: Pixels,
	pub cx: f64,
	pub cy: f64,
	pub w: f64,
	pub h: f64,
	pub rotation: f64,
	pub opacity: f64,
	pub color_matrix: Option<[f64; 12]>,
	pub filter: Option<[f64; 12]>,
	pub grade: Option<GradeSpec>,
	pub lut: Option<LutRef>,
	pub blur: f64,
	pub chroma_key: Option<ChromaKeySpec>,
	pub blend: BlendMode,
	pub mask: Option<MaskSpec>,
	pub flip_x: bool,
	pub flip_y: bool,
	pub effects: Vec<Effect>,
	pub transition: Option<Transition>,
	pub shadow: Option<ShadowSpec>,
	pub outline: Option<OutlineSpec>,
	/// The tone table, before the colour matrix (SDD-013 section 3.11.1).
	pub tone: Option<Box<[u8; TONE_LEN]>>,
	/// The ordered filters, after the blur (SDD-013 section 3.11.3).
	pub filters: Vec<FilterOp>,
	pub clip: Option<ClipSpec>,
	/// The layer is read by a clip and drawn by nobody.
	pub stencil: bool,
}

/// A decoded render input.
#[derive(Clone, Debug, PartialEq)]
pub struct RenderInput {
	pub output: Output,
	pub background: String,
	/// True when uncovered pixels stay clear (SDD-004 section 6, VV-172).
	pub transparent: bool,
	/// The plan's frame number and the project's size: the grain and the whip depend on them.
	pub frame: FrameInfo,
	pub layers: Vec<LayerSpec>,
}

fn invalid(detail: impl Into<String>) -> Error {
	Error::new(Reason::InvalidPlan, detail)
}

fn limit(name: &str) -> Error {
	Error::new(Reason::LimitExceeded, name)
}

/// The members of an object, taken one by one; what is left at the end is unknown.
struct Fields {
	what: &'static str,
	rest: Vec<(String, Value)>,
}

impl Fields {
	fn of(v: Value, what: &'static str) -> Result<Self, Error> {
		match v {
			Value::Obj(rest) => Ok(Self { what, rest }),
			_ => Err(invalid(format!("{what} is not an object"))),
		}
	}

	fn take(&mut self, name: &str) -> Option<Value> {
		let i = self.rest.iter().position(|(k, _)| k == name)?;
		Some(self.rest.remove(i).1)
	}

	fn required(&mut self, name: &str) -> Result<Value, Error> {
		self.take(name)
			.ok_or_else(|| invalid(format!("{}: {name} is missing", self.what)))
	}

	fn num(&mut self, name: &str) -> Result<f64, Error> {
		let what = self.what;
		as_num(self.required(name)?, what, name)
	}

	fn num_or(&mut self, name: &str, default: f64) -> Result<f64, Error> {
		let what = self.what;
		match self.take(name) {
			None | Some(Value::Null) => Ok(default),
			Some(v) => as_num(v, what, name),
		}
	}

	fn int(&mut self, name: &str) -> Result<i64, Error> {
		let what = self.what;
		as_int(self.required(name)?, what, name)
	}

	fn opt_int(&mut self, name: &str) -> Result<Option<i64>, Error> {
		let what = self.what;
		match self.take(name) {
			None | Some(Value::Null) => Ok(None),
			Some(v) => as_int(v, what, name).map(Some),
		}
	}

	fn string(&mut self, name: &str) -> Result<String, Error> {
		let what = self.what;
		match self.required(name)? {
			Value::Str(s) => Ok(s),
			_ => Err(invalid(format!("{what}: {name} is not a string"))),
		}
	}

	/// Fails on any member nobody took.
	fn finish(self) -> Result<(), Error> {
		match self.rest.first() {
			None => Ok(()),
			Some((k, _)) => Err(invalid(format!(
				"{}: unknown field {}",
				self.what,
				truncate(k)
			))),
		}
	}
}

fn truncate(s: &str) -> String {
	s.chars().take(32).collect()
}

fn as_num(v: Value, what: &str, name: &str) -> Result<f64, Error> {
	match v {
		Value::Num(n) => Ok(n),
		_ => Err(invalid(format!("{what}: {name} is not a number"))),
	}
}

fn as_int(v: Value, what: &str, name: &str) -> Result<i64, Error> {
	let n = as_num(v, what, name)?;
	if n.fract() == 0.0 && n.abs() <= 9_007_199_254_740_992.0 {
		Ok(n as i64)
	} else {
		Err(invalid(format!("{what}: {name} is not a whole number")))
	}
}

fn matrix(v: Option<Value>, name: &str) -> Result<Option<[f64; 12]>, Error> {
	match v {
		None | Some(Value::Null) => Ok(None),
		Some(Value::Arr(items)) if items.len() == 12 => {
			let mut m = [0.0; 12];
			for (o, it) in m.iter_mut().zip(items) {
				*o = as_num(it, "layer", name)?;
			}
			Ok(Some(m))
		}
		Some(_) => Err(invalid(format!("layer: {name} is not 12 numbers"))),
	}
}

fn chroma_key(v: Option<Value>) -> Result<Option<ChromaKeySpec>, Error> {
	let Some(v) = v.filter(|v| !matches!(v, Value::Null)) else {
		return Ok(None);
	};
	let mut f = Fields::of(v, "chroma_key")?;
	let spec = ChromaKeySpec {
		color: f.string("color")?,
		similarity: f.num("similarity")?,
		smoothness: f.num("smoothness")?,
		spill: f.num("spill")?,
	};
	f.finish()?;
	Ok(Some(spec))
}

fn grade(v: Option<Value>) -> Result<Option<GradeSpec>, Error> {
	let Some(v) = v.filter(|v| !matches!(v, Value::Null)) else {
		return Ok(None);
	};
	let mut f = Fields::of(v, "grade")?;
	let mut spec = GradeSpec::default();
	match f.take("hsl") {
		None | Some(Value::Null) => {}
		Some(Value::Arr(items)) => {
			if items.len() > HSL_RANGES.len() {
				return Err(limit("render_grade_bands"));
			}
			for it in items {
				let mut b = Fields::of(it, "hsl")?;
				let range = b.string("range")?;
				let Some(i) = HSL_RANGES.iter().position(|r| *r == range) else {
					return Err(invalid(format!(
						"hsl: range {:?} is not a colour range",
						truncate(&range)
					)));
				};
				spec.hsl[i] = HslBand {
					hue: b.num_or("hue", 0.0)?,
					saturation: b.num_or("saturation", 0.0)?,
					lightness: b.num_or("lightness", 0.0)?,
				};
				b.finish()?;
			}
		}
		Some(_) => return Err(invalid("grade: hsl is not an array")),
	}
	match f.take("wheels") {
		None | Some(Value::Null) => {}
		Some(v) => {
			let mut w = Fields::of(v, "wheels")?;
			for (i, name) in ["shadows", "midtones", "highlights"]
				.into_iter()
				.enumerate()
			{
				spec.wheels[i] = match w.take(name) {
					None | Some(Value::Null) => None,
					Some(v) => {
						let mut o = Fields::of(v, "wheel")?;
						let wheel = Wheel {
							hue: o.num_or("hue", 0.0)?,
							amount: o.num_or("amount", 0.0)?,
							luma: o.num_or("luma", 0.0)?,
						};
						o.finish()?;
						Some(wheel)
					}
				};
			}
			w.finish()?;
		}
	}
	match f.take("curves") {
		None | Some(Value::Null) => {}
		Some(v) => {
			let mut c = Fields::of(v, "curves")?;
			spec.master = curve(c.take("master"))?;
			spec.red = curve(c.take("red"))?;
			spec.green = curve(c.take("green"))?;
			spec.blue = curve(c.take("blue"))?;
			c.finish()?;
		}
	}
	f.finish()?;
	Ok(Some(spec))
}

fn lut_ref(v: Option<Value>) -> Result<Option<LutRef>, Error> {
	let Some(v) = v.filter(|v| !matches!(v, Value::Null)) else {
		return Ok(None);
	};
	let mut f = Fields::of(v, "lut")?;
	let asset = id(f.string("asset")?, "lut: asset")?;
	let lut = LutRef {
		asset,
		intensity: f.num("intensity")?,
	};
	f.finish()?;
	Ok(Some(lut))
}

fn curve(v: Option<Value>) -> Result<Vec<CurvePoint>, Error> {
	let items = match v {
		None | Some(Value::Null) => return Ok(Vec::new()),
		Some(Value::Arr(items)) => items,
		Some(_) => return Err(invalid("curves: a curve is not an array")),
	};
	if items.len() > MAX_CURVE_POINTS {
		return Err(limit("render_curve_points"));
	}
	items
		.into_iter()
		.map(|it| {
			let mut p = Fields::of(it, "curve point")?;
			let point = CurvePoint {
				x: p.num("x")?,
				y: p.num("y")?,
			};
			p.finish()?;
			Ok(point)
		})
		.collect()
}

fn blend(v: Option<Value>) -> Result<BlendMode, Error> {
	match v {
		None | Some(Value::Null) => Ok(BlendMode::Normal),
		Some(Value::Str(s)) => BlendMode::from_name(&s)
			.ok_or_else(|| invalid(format!("layer: blend {s:?} is not a blend mode"))),
		Some(_) => Err(invalid("layer: blend is not a string")),
	}
}

fn flag(v: Option<Value>, name: &str) -> Result<bool, Error> {
	match v {
		None | Some(Value::Null) => Ok(false),
		Some(Value::Bool(b)) => Ok(b),
		Some(_) => Err(invalid(format!("layer: {name} is not a boolean"))),
	}
}

fn shadow(v: Option<Value>) -> Result<Option<ShadowSpec>, Error> {
	let Some(v) = v.filter(|v| !matches!(v, Value::Null)) else {
		return Ok(None);
	};
	let mut f = Fields::of(v, "shadow")?;
	let spec = ShadowSpec {
		color: f.string("color")?,
		opacity: f.num("opacity")?,
		dx: f.num("dx")?,
		dy: f.num("dy")?,
		blur: f.num("blur")?,
	};
	f.finish()?;
	Ok(Some(spec))
}

fn outline(v: Option<Value>) -> Result<Option<OutlineSpec>, Error> {
	let Some(v) = v.filter(|v| !matches!(v, Value::Null)) else {
		return Ok(None);
	};
	let mut f = Fields::of(v, "outline")?;
	let spec = OutlineSpec {
		color: f.string("color")?,
		width: f.num("width")?,
	};
	f.finish()?;
	Ok(Some(spec))
}

fn mask(v: Option<Value>) -> Result<Option<MaskSpec>, Error> {
	let Some(v) = v.filter(|v| !matches!(v, Value::Null)) else {
		return Ok(None);
	};
	let mut f = Fields::of(v, "mask")?;
	let shape = match f.string("shape")?.as_str() {
		"rect" => MaskShape::Rect,
		"ellipse" => MaskShape::Ellipse,
		"rounded_rect" => MaskShape::RoundedRect,
		other => {
			return Err(invalid(format!(
				"mask: shape {:?} is not rect, rounded_rect or ellipse",
				truncate(other)
			)));
		}
	};
	let invert = match f.take("invert") {
		None | Some(Value::Null) => false,
		Some(Value::Bool(b)) => b,
		Some(_) => return Err(invalid("mask: invert is not a boolean")),
	};
	let spec = MaskSpec {
		shape,
		x: f.num("x")?,
		y: f.num("y")?,
		width: f.num("width")?,
		height: f.num("height")?,
		feather: f.num_or("feather", 0.0)?,
		invert,
		radius: f.num_or("radius", 0.0)?,
	};
	f.finish()?;
	Ok(Some(spec))
}

fn tone(v: Option<Value>) -> Result<Option<Box<[u8; TONE_LEN]>>, Error> {
	let items = match v {
		None | Some(Value::Null) => return Ok(None),
		Some(Value::Arr(items)) if items.len() == TONE_LEN => items,
		Some(_) => return Err(invalid("layer: tone is not 256 numbers")),
	};
	let mut out = Vec::with_capacity(TONE_LEN);
	for it in items {
		let n = as_int(it, "layer", "tone")?;
		if !(0..=255).contains(&n) {
			return Err(invalid("layer: a tone entry is not 0 to 255"));
		}
		out.push(n as u8);
	}
	let table: Box<[u8; TONE_LEN]> = out
		.into_boxed_slice()
		.try_into()
		.map_err(|_| invalid("layer: tone is not 256 numbers"))?;
	Ok(Some(table))
}

fn clip(v: Option<Value>) -> Result<Option<ClipSpec>, Error> {
	let Some(v) = v.filter(|v| !matches!(v, Value::Null)) else {
		return Ok(None);
	};
	let mut f = Fields::of(v, "clip")?;
	let source = id(f.string("source")?, "clip: source")?;
	let invert = match f.take("invert") {
		None | Some(Value::Null) => false,
		Some(Value::Bool(b)) => b,
		Some(_) => return Err(invalid("clip: invert is not a boolean")),
	};
	f.finish()?;
	Ok(Some(ClipSpec { source, invert }))
}

fn filters(v: Option<Value>) -> Result<Vec<FilterOp>, Error> {
	let items = match v {
		None | Some(Value::Null) => return Ok(Vec::new()),
		Some(Value::Arr(items)) => items,
		Some(_) => return Err(invalid("layer: filters is not an array")),
	};
	if items.len() > MAX_FILTERS {
		return Err(limit("render_filters"));
	}
	items.into_iter().map(filter).collect()
}

fn filter(v: Value) -> Result<FilterOp, Error> {
	let mut f = Fields::of(v, "filter")?;
	let kind = f.string("kind")?;
	let op = match kind.as_str() {
		"matrix" => match matrix(f.take("matrix"), "matrix")? {
			Some(m) => FilterOp::Matrix(m),
			None => return Err(invalid("filter: matrix is missing")),
		},
		"blur" => FilterOp::Blur {
			sigma: f.num_or("sigma", 0.0)?,
		},
		"sharpen" => FilterOp::Sharpen {
			amount: f.num_or("amount", 0.0)?,
			radius: f.num_or("radius", 0.0)?,
		},
		"vignette" => FilterOp::Vignette {
			amount: f.num_or("amount", 0.0)?,
			softness: f.num_or("softness", 0.0)?,
		},
		"grain" => FilterOp::Grain {
			amount: f.num_or("amount", 0.0)?,
			size: f.num_or("size", 0.0)?,
			seed: f.opt_int("seed")?.unwrap_or(0),
		},
		"shadow" => FilterOp::Shadow(ShadowSpec {
			color: f.string("color")?,
			opacity: f.num_or("opacity", 0.0)?,
			dx: f.num_or("dx", 0.0)?,
			dy: f.num_or("dy", 0.0)?,
			blur: f.num_or("blur", 0.0)?,
		}),
		"stroke" => FilterOp::Stroke(OutlineSpec {
			color: f.string("color")?,
			width: f.num_or("width", 0.0)?,
		}),
		"outer_glow" => FilterOp::OuterGlow {
			color: f.string("color")?,
			radius: f.num_or("radius", 0.0)?,
			amount: f.num_or("amount", 0.0)?,
		},
		other => {
			return Err(invalid(format!(
				"filter: kind {:?} is not a filter",
				truncate(other)
			)));
		}
	};
	f.finish()?;
	Ok(op)
}

fn effects(v: Option<Value>) -> Result<Vec<Effect>, Error> {
	let items = match v {
		None | Some(Value::Null) => return Ok(Vec::new()),
		Some(Value::Arr(items)) => items,
		Some(_) => return Err(invalid("layer: effects is not an array")),
	};
	if items.len() > MAX_EFFECTS {
		return Err(limit("render_effects"));
	}
	items.into_iter().map(effect).collect()
}

fn effect(v: Value) -> Result<Effect, Error> {
	let mut f = Fields::of(v, "effect")?;
	let kind = f.string("kind")?;
	let e = match kind.as_str() {
		"motion_blur" => Effect::MotionBlur {
			amount: f.num_or("amount", 0.5)?,
			angle: f.num_or("angle", 0.0)?,
		},
		"glow" => Effect::Glow {
			amount: f.num_or("amount", 0.5)?,
			radius: f.num_or("radius", 12.0)?,
			threshold: f.num_or("threshold", 0.6)?,
		},
		"vignette" => Effect::Vignette {
			amount: f.num_or("amount", 0.5)?,
			softness: f.num_or("softness", 0.5)?,
		},
		"grain" => Effect::Grain {
			amount: f.num_or("amount", 0.3)?,
			size: f.num_or("size", 1.0)?,
			seed: f.opt_int("seed")?.unwrap_or(0),
		},
		"rgb_split" => Effect::RgbSplit {
			amount: f.num_or("amount", 0.3)?,
			angle: f.num_or("angle", 0.0)?,
		},
		other => {
			return Err(invalid(format!(
				"effect: kind {:?} is not a pixel effect",
				truncate(other)
			)));
		}
	};
	f.finish()?;
	Ok(e)
}

fn transition(v: Option<Value>) -> Result<Option<Transition>, Error> {
	let Some(v) = v.filter(|v| !matches!(v, Value::Null)) else {
		return Ok(None);
	};
	let mut f = Fields::of(v, "transition")?;
	let kind = match f.string("kind")?.as_str() {
		"whip" => TransitionKind::Whip,
		"dip" => TransitionKind::Dip,
		"wipe" => TransitionKind::Wipe,
		other => {
			return Err(invalid(format!(
				"transition: kind {:?} is not whip, dip or wipe",
				truncate(other)
			)));
		}
	};
	match f.string("edge")?.as_str() {
		"in" | "out" => {}
		other => {
			return Err(invalid(format!(
				"transition: edge {:?} is not in or out",
				truncate(other)
			)));
		}
	}
	let p = f.int("p")?;
	if !(0..=65536).contains(&p) {
		return Err(invalid("transition: p is not a 16.16 progress"));
	}
	let direction = match f.take("direction") {
		None | Some(Value::Null) => Direction::Left,
		Some(Value::Str(s)) => match s.as_str() {
			"left" => Direction::Left,
			"right" => Direction::Right,
			"up" => Direction::Up,
			"down" => Direction::Down,
			other => {
				return Err(invalid(format!(
					"transition: direction {:?} is not left, right, up or down",
					truncate(other)
				)));
			}
		},
		Some(_) => return Err(invalid("transition: direction is not a string")),
	};
	let colour = match f.take("color") {
		None | Some(Value::Null) => [0, 0, 0, 255],
		Some(Value::Str(s)) => parse_colour(&s),
		Some(_) => return Err(invalid("transition: color is not a string")),
	};
	f.finish()?;
	Ok(Some(Transition {
		kind,
		p,
		direction,
		colour,
	}))
}

fn id(s: String, what: &str) -> Result<String, Error> {
	if s.is_empty() || s.len() > MAX_ID_BYTES || !s.is_ascii() {
		return Err(invalid(format!(
			"{what} is not 1 to {MAX_ID_BYTES} ASCII bytes"
		)));
	}
	Ok(s)
}

/// Decodes and validates a render input. The version is read first, so an input of a newer
/// version is `unsupported_version` and never a complaint about its new fields.
pub fn decode(input: &[u8]) -> Result<RenderInput, Error> {
	if input.len() > MAX_INPUT_BYTES {
		return Err(limit("render_input_bytes"));
	}
	let root = parse(input).map_err(|e| match e {
		JsonError::Utf8 => invalid("the input is not UTF-8"),
		JsonError::Syntax(at) => invalid(format!("the input is not JSON (byte {at})")),
		JsonError::DuplicateKey(at) => invalid(format!("a key is repeated (byte {at})")),
		JsonError::Range(at) => invalid(format!("a number is out of range (byte {at})")),
		JsonError::Depth => invalid("the input is nested too deeply"),
	})?;
	let mut top = Fields::of(root, "input")?;

	let version = top.int("version")?;
	if version != RENDER_INPUT_VERSION {
		return Err(Error::new(
			Reason::UnsupportedVersion,
			format!("render input version {version}"),
		));
	}

	let output = decode_output(top.required("output")?)?;
	let mut plan = Fields::of(top.required("plan")?, "plan")?;
	top.finish()?;

	let frame = FrameInfo {
		frame: plan.opt_int("frame")?.unwrap_or(0),
		width: plan.opt_int("width")?.unwrap_or(0),
		height: plan.opt_int("height")?.unwrap_or(0),
	};
	if !(0..=i64::from(u32::MAX)).contains(&frame.frame) || frame.width < 0 || frame.height < 0 {
		return Err(invalid("plan: frame, width or height is out of range"));
	}
	let background = match plan.take("background") {
		None | Some(Value::Null) => "#000000".to_string(),
		Some(Value::Str(s)) => s,
		Some(_) => return Err(invalid("plan: background is not a string")),
	};
	let transparent = match plan.take("transparent") {
		None | Some(Value::Null) => false,
		Some(Value::Bool(b)) => b,
		Some(_) => return Err(invalid("plan: transparent is not a boolean")),
	};
	let layers = match plan.take("layers") {
		None | Some(Value::Null) => Vec::new(),
		Some(Value::Arr(items)) => {
			if items.len() > MAX_LAYERS {
				return Err(limit("render_layers"));
			}
			items
				.into_iter()
				.map(decode_layer)
				.collect::<Result<_, _>>()?
		}
		Some(_) => return Err(invalid("plan: layers is not an array")),
	};
	plan.finish()?;

	Ok(RenderInput {
		output,
		background,
		transparent,
		frame,
		layers,
	})
}

fn decode_output(v: Value) -> Result<Output, Error> {
	let mut f = Fields::of(v, "output")?;
	let (w, h) = (f.int("width")?, f.int("height")?);
	let scale = f.num("scale")?;
	let (y0, y1) = (f.opt_int("y0")?, f.opt_int("y1")?);
	f.finish()?;

	if w < 1 || h < 1 {
		return Err(invalid("output: the size is not positive"));
	}
	if w > MAX_OUTPUT_SIDE as i64 || h > MAX_OUTPUT_SIDE as i64 {
		return Err(limit("render_frame_side"));
	}
	if scale.is_nan() || scale <= 0.0 {
		return Err(invalid("output: scale is not positive"));
	}
	let (width, height) = (w as usize, h as usize);
	let rows = match (y0, y1) {
		(None, None) => 0..height,
		(Some(a), Some(b)) if 0 <= a && a < b && b <= h => a as usize..b as usize,
		_ => {
			return Err(Error::new(
				Reason::InvalidArgument,
				"output: y0 and y1 do not name rows of the frame",
			));
		}
	};
	Ok(Output {
		width,
		height,
		scale,
		rows,
	})
}

fn decode_layer(v: Value) -> Result<LayerSpec, Error> {
	let mut f = Fields::of(v, "layer")?;
	let item = id(f.string("item")?, "layer: item")?;
	let kind = f.string("kind")?;

	// Informational fields: accepted and checked for their type only.
	for name in ["asset", "asset_kind"] {
		if let Some(v) = f.take(name)
			&& !matches!(v, Value::Str(_) | Value::Null)
		{
			return Err(invalid(format!("layer: {name} is not a string")));
		}
	}
	f.opt_int("source_frame")?;
	f.num_or("scale", 1.0)?;
	if let Some(c) = f.take("crop")
		&& !matches!(c, Value::Null)
	{
		let mut c = Fields::of(c, "crop")?;
		for name in ["x", "y", "w", "h"] {
			c.num(name)?;
		}
		c.finish()?;
	}

	let spec = f.take("bitmap").filter(|v| !matches!(v, Value::Null));
	let key = f.take("bitmap_key").filter(|v| !matches!(v, Value::Null));
	let pixels = match kind.as_str() {
		"media" => {
			if spec.is_some() || key.is_some() {
				return Err(invalid("layer: a media layer has a bitmap"));
			}
			Pixels::Media(item.clone())
		}
		"bitmap" => {
			let (Some(spec), Some(Value::Str(key))) = (spec, key) else {
				return Err(invalid("layer: a bitmap layer needs bitmap and bitmap_key"));
			};
			let mut spec = Fields::of(spec, "bitmap")?;
			match spec.take("kind") {
				Some(Value::Str(k)) if matches!(k.as_str(), "text" | "caption" | "shape") => {}
				_ => {
					return Err(Error::new(
						Reason::UnsupportedLayer,
						format!("layer {item}: bitmap kind"),
					));
				}
			}
			// The rest of the spec is the rasterizer's until stage E: the host has drawn it.
			Pixels::Bitmap(id(key, "layer: bitmap_key")?)
		}
		_ => {
			return Err(Error::new(
				Reason::UnsupportedLayer,
				format!("layer {item}: kind {}", truncate(&kind)),
			));
		}
	};

	let layer = LayerSpec {
		pixels,
		cx: f.num_or("cx", 0.0)?,
		cy: f.num_or("cy", 0.0)?,
		w: f.num_or("w", 0.0)?,
		h: f.num_or("h", 0.0)?,
		rotation: f.num_or("rotation", 0.0)?,
		opacity: f.num("opacity")?,
		color_matrix: matrix(f.take("color_matrix"), "color_matrix")?,
		filter: matrix(f.take("filter"), "filter")?,
		grade: grade(f.take("grade"))?,
		lut: lut_ref(f.take("lut"))?,
		blur: f.num_or("blur", 0.0)?,
		chroma_key: chroma_key(f.take("chroma_key"))?,
		blend: blend(f.take("blend"))?,
		mask: mask(f.take("mask"))?,
		flip_x: flag(f.take("flip_x"), "flip_x")?,
		flip_y: flag(f.take("flip_y"), "flip_y")?,
		effects: effects(f.take("effects"))?,
		transition: transition(f.take("transition"))?,
		shadow: shadow(f.take("shadow"))?,
		outline: outline(f.take("outline"))?,
		tone: tone(f.take("tone"))?,
		filters: filters(f.take("filters"))?,
		clip: clip(f.take("clip"))?,
		stencil: flag(f.take("stencil"), "stencil")?,
		item,
	};
	f.finish()?;
	Ok(layer)
}

#[cfg(test)]
mod tests {
	use super::*;

	const GOOD: &str = r##"{"version":1,"output":{"width":540,"height":960,"scale":0.5},
		"plan":{"frame":120,"width":1080,"height":1920,"background":"#112233","layers":[
		{"item":"itm_a","kind":"media","asset":"ast_1","asset_kind":"video","source_frame":62,
		 "crop":{"x":0,"y":0,"w":1080,"h":1920},"color_matrix":null,"filter":null,"blur":0,
		 "cx":540,"cy":960,"w":1080,"h":1920,"rotation":0,"scale":1,"opacity":1}]}}"##;

	fn reason(s: &str) -> Reason {
		decode(s.as_bytes()).expect_err(s).reason
	}

	#[test]
	fn decodes_a_plan() {
		let r = decode(GOOD.as_bytes()).unwrap();
		assert_eq!(
			(r.output.width, r.output.height, r.output.rows.clone()),
			(540, 960, 0..960)
		);
		assert_eq!(r.background, "#112233");
		assert!(!r.transparent);
		assert_eq!(r.layers.len(), 1);
		assert_eq!(r.layers[0].pixels, Pixels::Media("itm_a".into()));
		assert_eq!(r.layers[0].opacity, 1.0);
	}

	#[test]
	fn decodes_a_transparent_plan() {
		let s = GOOD.replace(
			"\"background\":\"#112233\"",
			"\"background\":\"#000000\",\"transparent\":true",
		);
		let r = decode(s.as_bytes()).unwrap();
		assert!(r.transparent);
		let bad = GOOD.replace(
			"\"background\":\"#112233\"",
			"\"background\":\"#000000\",\"transparent\":1",
		);
		assert_eq!(reason(&bad), Reason::InvalidPlan);
	}

	#[test]
	fn decodes_a_grade_and_refuses_a_bad_one() {
		let with = |g: &str| GOOD.replace("\"blur\":0", &format!("\"blur\":0,\"grade\":{g}"));
		let r = decode(
			with(
				r#"{"hsl":[{"range":"green","saturation":-0.5},{"range":"red","hue":0.25}],
				"wheels":{"shadows":{"hue":220,"amount":0.5,"luma":-0.1}},
				"curves":{"master":[{"x":0,"y":0.1},{"x":1,"y":1}]}}"#,
			)
			.as_bytes(),
		)
		.unwrap();
		let g = r.layers[0].grade.as_ref().unwrap();
		assert_eq!(g.hsl[3].saturation, -0.5);
		assert_eq!(g.hsl[0].hue, 0.25);
		assert_eq!(g.wheels[0].unwrap().amount, 0.5);
		assert!(g.wheels[1].is_none());
		assert_eq!(g.master.len(), 2);
		assert!(
			decode(with("null").as_bytes()).unwrap().layers[0]
				.grade
				.is_none()
		);

		assert_eq!(
			reason(&with(r#"{"hsl":[{"range":"pink","hue":1}]}"#)),
			Reason::InvalidPlan
		);
		assert_eq!(reason(&with(r#"{"extra":1}"#)), Reason::InvalidPlan);
		assert_eq!(reason(&with("[]")), Reason::InvalidPlan);
		let points = [r#"{"x":0,"y":0}"#; MAX_CURVE_POINTS + 1].join(",");
		assert_eq!(
			reason(&with(&format!(r#"{{"curves":{{"red":[{points}]}}}}"#))),
			Reason::LimitExceeded
		);
		let bands = [r#"{"range":"red"}"#; 9].join(",");
		assert_eq!(
			reason(&with(&format!(r#"{{"hsl":[{bands}]}}"#))),
			Reason::LimitExceeded
		);
	}

	#[test]
	fn refuses_what_section_7_names() {
		assert_eq!(
			reason(&GOOD.replace("\"version\":1", "\"version\":2")),
			Reason::UnsupportedVersion
		);
		assert_eq!(
			reason(&GOOD.replace("\"blur\":0", "\"blurry\":0")),
			Reason::InvalidPlan
		);
		assert_eq!(
			reason(&GOOD.replace(",\"opacity\":1", "")),
			Reason::InvalidPlan
		);
		assert_eq!(
			reason(&GOOD.replace("\"media\"", "\"video\"")),
			Reason::UnsupportedLayer
		);
		assert_eq!(
			reason(&GOOD.replace("\"filter\":null", "\"filter\":[1,2]")),
			Reason::InvalidPlan
		);
		assert_eq!(
			reason(&GOOD.replace("\"width\":540", "\"width\":4097")),
			Reason::LimitExceeded
		);
		assert_eq!(
			reason(&GOOD.replace("\"scale\":0.5", "\"scale\":0.5,\"y0\":9,\"y1\":9")),
			Reason::InvalidArgument
		);
		assert_eq!(
			reason(&GOOD.replace("\"cx\":540", "\"cx\":NaN")),
			Reason::InvalidPlan
		);
		assert_eq!(
			reason(&GOOD.replace("\"cx\":540", "\"cx\":1e999")),
			Reason::InvalidPlan
		);
		assert_eq!(
			reason(&GOOD.replace(
				"\"item\":\"itm_a\"",
				"\"item\":\"itm_a\",\"bitmap\":{\"kind\":\"text\"}"
			)),
			Reason::InvalidPlan
		);
		assert_eq!(reason("[]"), Reason::InvalidPlan);
		assert_eq!(reason(""), Reason::InvalidPlan);
	}

	#[test]
	fn reads_the_blend_mode_of_a_layer() {
		let with = |b: &str| GOOD.replace("\"blur\":0", &format!("\"blur\":0,\"blend\":{b}"));
		assert_eq!(
			decode(GOOD.as_bytes()).unwrap().layers[0].blend,
			BlendMode::Normal
		);
		for (name, mode) in [
			("normal", BlendMode::Normal),
			("multiply", BlendMode::Multiply),
			("screen", BlendMode::Screen),
			("overlay", BlendMode::Overlay),
			("add", BlendMode::Add),
			("soft_light", BlendMode::SoftLight),
			("hard_light", BlendMode::HardLight),
			("darken", BlendMode::Darken),
			("lighten", BlendMode::Lighten),
			("color_dodge", BlendMode::ColorDodge),
			("color_burn", BlendMode::ColorBurn),
			("difference", BlendMode::Difference),
		] {
			let r = decode(with(&format!("\"{name}\"")).as_bytes()).unwrap();
			assert_eq!(r.layers[0].blend, mode, "{name}");
		}
		assert_eq!(
			decode(with("null").as_bytes()).unwrap().layers[0].blend,
			BlendMode::Normal
		);
		assert_eq!(reason(&with("\"dissolve\"")), Reason::InvalidPlan);
		assert_eq!(reason(&with("3")), Reason::InvalidPlan);
	}

	#[test]
	fn reads_the_flips_of_a_layer() {
		let with = |m: &str| GOOD.replace("\"blur\":0", &format!("\"blur\":0,{m}"));
		let l = &decode(GOOD.as_bytes()).unwrap().layers[0];
		assert!(!l.flip_x && !l.flip_y);
		let r = decode(with(r#""flip_x":true,"flip_y":null"#).as_bytes()).unwrap();
		assert!(r.layers[0].flip_x && !r.layers[0].flip_y);
		let r = decode(with(r#""flip_y":true"#).as_bytes()).unwrap();
		assert!(!r.layers[0].flip_x && r.layers[0].flip_y);
		assert_eq!(reason(&with(r#""flip_x":1"#)), Reason::InvalidPlan);
	}

	#[test]
	fn reads_the_effects_and_the_transition_of_a_layer() {
		let with = |m: &str| GOOD.replace("\"blur\":0", &format!("\"blur\":0,{m}"));
		let l = &decode(GOOD.as_bytes()).unwrap().layers[0];
		assert!(l.effects.is_empty() && l.transition.is_none());
		let r = decode(
			with(
				r##""effects":[{"kind":"motion_blur","amount":0.2,"angle":-30},{"kind":"grain","amount":0.5,"size":2,"seed":7},{"kind":"vignette"}],"transition":{"kind":"dip","edge":"out","p":40000,"color":"#ff8000"}"##,
			)
			.as_bytes(),
		)
		.unwrap();
		assert_eq!(
			r.layers[0].effects,
			vec![
				Effect::MotionBlur {
					amount: 0.2,
					angle: -30.0
				},
				Effect::Grain {
					amount: 0.5,
					size: 2.0,
					seed: 7
				},
				Effect::Vignette {
					amount: 0.5,
					softness: 0.5
				},
			]
		);
		let t = r.layers[0].transition.unwrap();
		assert_eq!(
			(t.kind, t.p, t.colour),
			(TransitionKind::Dip, 40000, [255, 128, 0, 255])
		);
		assert_eq!(t.direction, Direction::Left);
		let r = decode(
			with(r#""transition":{"kind":"wipe","edge":"in","p":1,"direction":"down"}"#).as_bytes(),
		)
		.unwrap();
		assert_eq!(r.layers[0].transition.unwrap().direction, Direction::Down);
		for bad in [
			r#""effects":[{"kind":"shake"}]"#,
			r#""effects":[{"kind":"glow","spin":1}]"#,
			r#""effects":3"#,
			r#""transition":{"kind":"fade","edge":"in","p":1}"#,
			r#""transition":{"kind":"wipe","edge":"sideways","p":1}"#,
			r#""transition":{"kind":"wipe","edge":"in","p":65537}"#,
			r#""transition":{"kind":"wipe","edge":"in","p":-1}"#,
			r#""transition":{"kind":"wipe","edge":"in","p":1,"direction":"north"}"#,
			r#""transition":{"kind":"wipe","edge":"in"}"#,
		] {
			assert_eq!(reason(&with(bad)), Reason::InvalidPlan, "{bad}");
		}
		let seven = format!(
			"\"effects\":[{}]",
			[r#"{"kind":"vignette"}"#; MAX_EFFECTS + 1].join(",")
		);
		assert_eq!(reason(&with(&seven)), Reason::LimitExceeded);
	}

	#[test]
	fn reads_the_frame_the_grain_and_the_whip_depend_on() {
		let r = decode(GOOD.as_bytes()).unwrap();
		assert_eq!(
			r.frame,
			FrameInfo {
				frame: 120,
				width: 1080,
				height: 1920
			}
		);
		assert_eq!(
			reason(&GOOD.replace("\"frame\":120", "\"frame\":-1")),
			Reason::InvalidPlan
		);
		assert_eq!(
			reason(&GOOD.replace("\"width\":1080", "\"width\":-1")),
			Reason::InvalidPlan
		);
	}

	#[test]
	fn reads_the_mask_of_a_layer() {
		let with = |m: &str| GOOD.replace("\"blur\":0", &format!("\"blur\":0,\"mask\":{m}"));
		assert_eq!(decode(GOOD.as_bytes()).unwrap().layers[0].mask, None);
		let r = decode(
			with(
				r#"{"shape":"ellipse","x":0.1,"y":0.2,"width":0.5,"height":0.6,"feather":4,"invert":true}"#,
			)
			.as_bytes(),
		)
		.unwrap();
		assert_eq!(
			r.layers[0].mask,
			Some(MaskSpec {
				shape: MaskShape::Ellipse,
				x: 0.1,
				y: 0.2,
				width: 0.5,
				height: 0.6,
				feather: 4.0,
				invert: true,
				radius: 0.0,
			})
		);
		let r = decode(with(r#"{"shape":"rect","x":0,"y":0,"width":1,"height":1}"#).as_bytes())
			.unwrap();
		assert_eq!(r.layers[0].mask.unwrap().feather, 0.0);
		assert_eq!(
			decode(with("null").as_bytes()).unwrap().layers[0].mask,
			None
		);
		for bad in [
			r#"{"shape":"star","x":0,"y":0,"width":1,"height":1}"#,
			r#"{"shape":"rect","x":0,"y":0,"width":1}"#,
			r#"{"shape":"rect","x":0,"y":0,"width":1,"height":1,"spin":1}"#,
			r#"{"shape":"rect","x":0,"y":0,"width":1,"height":1,"invert":1}"#,
			"3",
		] {
			assert_eq!(reason(&with(bad)), Reason::InvalidPlan, "{bad}");
		}
	}

	#[test]
	fn a_newer_version_is_not_a_complaint_about_new_fields() {
		assert_eq!(
			reason(r#"{"version":2,"future":true}"#),
			Reason::UnsupportedVersion
		);
	}

	#[test]
	fn limits_are_enforced() {
		let layers =
			vec!["{\"item\":\"a\",\"kind\":\"media\",\"opacity\":1}"; MAX_LAYERS + 1].join(",");
		let input = format!(
			"{{\"version\":1,\"output\":{{\"width\":8,\"height\":8,\"scale\":1}},\"plan\":{{\"layers\":[{layers}]}}}}"
		);
		assert_eq!(reason(&input), Reason::LimitExceeded);
		let big = vec![b' '; MAX_INPUT_BYTES + 1];
		assert_eq!(decode(&big).unwrap_err().reason, Reason::LimitExceeded);
	}

	#[test]
	fn a_bitmap_layer_needs_its_spec_and_key() {
		let bm = r##"{"version":1,"output":{"width":8,"height":8,"scale":1},"plan":{"layers":[
			{"item":"t","kind":"bitmap","bitmap":{"kind":"text","text":"hi"},"bitmap_key":"abc","opacity":1}]}}"##;
		let r = decode(bm.as_bytes()).unwrap();
		assert_eq!(r.layers[0].pixels, Pixels::Bitmap("abc".into()));
		assert_eq!(
			reason(&bm.replace(",\"bitmap_key\":\"abc\"", "")),
			Reason::InvalidPlan
		);
		assert_eq!(
			reason(&bm.replace("\"kind\":\"text\"", "\"kind\":\"gif\"")),
			Reason::UnsupportedLayer
		);
	}

	#[test]
	fn reads_the_look_of_an_image_layer() {
		let tone: Vec<String> = (0..256).map(|v| (255 - v).to_string()).collect();
		let layer = format!(
			r##","tone":[{}],"stencil":true,"clip":{{"source":"itm_b","invert":true}},
			"filters":[{{"kind":"matrix","matrix":[1,0,0,0,0,1,0,0,0,0,1,0]}},{{"kind":"blur","sigma":4}},
			{{"kind":"sharpen","amount":1.5,"radius":2}},{{"kind":"vignette","amount":0.5,"softness":0.5}},
			{{"kind":"grain","amount":0.3,"size":2,"seed":7}},
			{{"kind":"shadow","color":"#000000","opacity":0.5,"dx":4,"dy":4,"blur":8}},
			{{"kind":"stroke","color":"#FFFFFF","width":3}},
			{{"kind":"outer_glow","color":"#FFAA00","radius":12,"amount":1}}]"##,
			tone.join(",")
		);
		let input = GOOD.replace("\"blur\":0", &format!("\"blur\":0{layer}"));
		let r = decode(input.as_bytes()).unwrap();
		let l = &r.layers[0];
		assert_eq!(l.tone.as_ref().map(|t| (t[0], t[255])), Some((255, 0)));
		assert!(l.stencil);
		assert_eq!(
			l.clip,
			Some(ClipSpec {
				source: "itm_b".into(),
				invert: true
			})
		);
		assert_eq!(l.filters.len(), 8);
		assert_eq!(l.filters[1], FilterOp::Blur { sigma: 4.0 });
		assert_eq!(
			l.filters[7],
			FilterOp::OuterGlow {
				color: "#FFAA00".into(),
				radius: 12.0,
				amount: 1.0
			}
		);
	}

	#[test]
	fn refuses_a_bad_look() {
		let with = |m: &str| GOOD.replace("\"blur\":0", &format!("\"blur\":0,{m}"));
		assert_eq!(reason(&with("\"tone\":[1,2,3]")), Reason::InvalidPlan);
		let long: Vec<String> = (0..256).map(|_| "256".to_string()).collect();
		assert_eq!(
			reason(&with(&format!("\"tone\":[{}]", long.join(",")))),
			Reason::InvalidPlan
		);
		assert_eq!(
			reason(&with("\"filters\":[{\"kind\":\"sparkle\"}]")),
			Reason::InvalidPlan
		);
		assert_eq!(
			reason(&with("\"filters\":[{\"kind\":\"blur\",\"radius\":3}]")),
			Reason::InvalidPlan
		);
		assert_eq!(
			reason(&with("\"filters\":[{\"kind\":\"matrix\"}]")),
			Reason::InvalidPlan
		);
		let nine = ["{\"kind\":\"blur\",\"sigma\":1}"; MAX_FILTERS + 1].join(",");
		assert_eq!(
			reason(&with(&format!("\"filters\":[{nine}]"))),
			Reason::LimitExceeded
		);
		assert_eq!(
			reason(&with("\"clip\":{\"source\":\"\"}")),
			Reason::InvalidPlan
		);
		assert_eq!(
			reason(&with("\"clip\":{\"source\":\"a\",\"x\":1}")),
			Reason::InvalidPlan
		);
		assert_eq!(reason(&with("\"stencil\":1")), Reason::InvalidPlan);
	}

	#[test]
	fn reads_a_rounded_rectangle_mask() {
		let m = r#""mask":{"shape":"rounded_rect","x":0,"y":0,"width":1,"height":1,"radius":12}"#;
		let r = decode(
			GOOD.replace("\"blur\":0", &format!("\"blur\":0,{m}"))
				.as_bytes(),
		)
		.unwrap();
		let mask = r.layers[0].mask.unwrap();
		assert_eq!(mask.shape, MaskShape::RoundedRect);
		assert_eq!(mask.radius, 12.0);
	}
}
