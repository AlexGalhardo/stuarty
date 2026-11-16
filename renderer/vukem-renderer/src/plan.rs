//! The render input of SDD-013 section 2, decoded and validated.
//!
//! The renderer reads the JSON of `core/plan` (`FramePlan`) inside the envelope of section 2.1. It
//! refuses what it does not know: an unknown field is a pixel it would silently not draw.

use crate::blend::BlendMode;
use crate::chroma::ChromaKeySpec;
use crate::error::{Error, Reason};
use crate::json::{JsonError, Value, parse};

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
	pub blur: f64,
	pub chroma_key: Option<ChromaKeySpec>,
	pub blend: BlendMode,
}

/// A decoded render input.
#[derive(Clone, Debug, PartialEq)]
pub struct RenderInput {
	pub output: Output,
	pub background: String,
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

fn blend(v: Option<Value>) -> Result<BlendMode, Error> {
	match v {
		None | Some(Value::Null) => Ok(BlendMode::Normal),
		Some(Value::Str(s)) => BlendMode::from_name(&s)
			.ok_or_else(|| invalid(format!("layer: blend {s:?} is not a blend mode"))),
		Some(_) => Err(invalid("layer: blend is not a string")),
	}
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

	// Informational: nothing a pixel depends on.
	for name in ["frame", "width", "height"] {
		plan.opt_int(name)?;
	}
	let background = match plan.take("background") {
		None | Some(Value::Null) => "#000000".to_string(),
		Some(Value::Str(s)) => s,
		Some(_) => return Err(invalid("plan: background is not a string")),
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
		blur: f.num_or("blur", 0.0)?,
		chroma_key: chroma_key(f.take("chroma_key"))?,
		blend: blend(f.take("blend"))?,
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
		assert_eq!(r.layers.len(), 1);
		assert_eq!(r.layers[0].pixels, Pixels::Media("itm_a".into()));
		assert_eq!(r.layers[0].opacity, 1.0);
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
		] {
			let r = decode(with(&format!("\"{name}\"")).as_bytes()).unwrap();
			assert_eq!(r.layers[0].blend, mode, "{name}");
		}
		assert_eq!(
			decode(with("null").as_bytes()).unwrap().layers[0].blend,
			BlendMode::Normal
		);
		assert_eq!(reason(&with("\"darken\"")), Reason::InvalidPlan);
		assert_eq!(reason(&with("3")), Reason::InvalidPlan);
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
}
