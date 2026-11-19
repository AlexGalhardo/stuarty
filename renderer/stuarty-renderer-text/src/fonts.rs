//! The fonts of the rasterizer: the port of `renderer/fonts.go`.
//!
//! The bundled fonts are embedded in the binary; system fonts are never read (ADR-0012 item 3,
//! D-016). The files and their licences are in `renderer/fonts`
//! (`FONTS.md` lists their sources).

use std::collections::HashMap;
use std::fmt;
use std::sync::{Arc, Mutex, PoisonError};

use skrifa::Font;

use crate::validate::{InvalidFont, validate_font};

/// The error code of a family or weight that is not in the font set (SDD-000 section 8).
pub const CODE_FONT_UNKNOWN: &str = "E_FONT_UNKNOWN";

/// A family or weight the set does not have, or a user font file that is not stored.
#[derive(Clone, Debug, PartialEq, Eq)]
pub struct FontError {
	pub family: String,
	pub weight: u32,
	/// The weights that exist for the family; `None` when the family is unknown.
	pub weights: Option<Vec<u32>>,
	/// Set when the family exists in the document's fonts map but the file of that weight is not in
	/// the workspace.
	pub hash: Option<String>,
}

impl fmt::Display for FontError {
	fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
		if let Some(hash) = &self.hash {
			return write!(
				f,
				"{CODE_FONT_UNKNOWN}: the file of family {:?} weight {} (sha256 {hash}) is not in the workspace",
				self.family, self.weight
			);
		}
		match &self.weights {
			None => write!(f, "{CODE_FONT_UNKNOWN}: no font family {:?}", self.family),
			Some(weights) => write!(
				f,
				"{CODE_FONT_UNKNOWN}: family {:?} has no weight {} (available: {weights:?})",
				self.family, self.weight
			),
		}
	}
}

impl std::error::Error for FontError {}

/// Why a face could not be had.
#[derive(Clone, Debug, PartialEq, Eq)]
pub enum LookupError {
	Font(FontError),
	Invalid(InvalidFont),
	/// The font source failed, or the bytes do not match their hash.
	Source(String),
}

impl fmt::Display for LookupError {
	fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
		match self {
			Self::Font(e) => e.fmt(f),
			Self::Invalid(e) => e.fmt(f),
			Self::Source(m) => write!(f, "raster: {m}"),
		}
	}
}

impl std::error::Error for LookupError {}

impl From<FontError> for LookupError {
	fn from(e: FontError) -> Self {
		Self::Font(e)
	}
}

impl From<InvalidFont> for LookupError {
	fn from(e: InvalidFont) -> Self {
		Self::Invalid(e)
	}
}

/// What a [`FontSource`] reports for a hash.
#[derive(Clone, Debug, PartialEq, Eq)]
pub enum SourceError {
	/// The file is not stored (`raster.ErrFontNotStored`).
	NotStored,
	Other(String),
}

/// Reads the bytes of a stored font by its lower-case hex SHA-256. It is never used for anything but
/// those hashes; system fonts are never read.
pub type FontSource = Arc<dyn Fn(&str) -> Result<Vec<u8>, SourceError> + Send + Sync>;

struct Family {
	name: &'static str,
	data: &'static [u8],
	min: u32,
	max: u32,
	variable: bool,
}

/// The bundled set (D-022). The weight ranges are the ranges of the `wght` axis of each file.
const FAMILIES: [Family; 6] = [
	Family {
		name: "Inter",
		data: include_bytes!("../../fonts/Inter-Variable.ttf"),
		min: 100,
		max: 900,
		variable: true,
	},
	Family {
		name: "Montserrat",
		data: include_bytes!("../../fonts/Montserrat-Variable.ttf"),
		min: 100,
		max: 900,
		variable: true,
	},
	Family {
		name: "Anton",
		data: include_bytes!("../../fonts/Anton-Regular.ttf"),
		min: 400,
		max: 400,
		variable: false,
	},
	Family {
		name: "Bebas Neue",
		data: include_bytes!("../../fonts/BebasNeue-Regular.ttf"),
		min: 400,
		max: 400,
		variable: false,
	},
	Family {
		name: "Playfair Display",
		data: include_bytes!("../../fonts/PlayfairDisplay-Variable.ttf"),
		min: 400,
		max: 900,
		variable: true,
	},
	Family {
		name: "JetBrains Mono",
		data: include_bytes!("../../fonts/JetBrainsMono-Variable.ttf"),
		min: 100,
		max: 800,
		variable: true,
	},
];

/// The names of the bundled families, Inter first.
pub fn families() -> Vec<&'static str> {
	FAMILIES.iter().map(|f| f.name).collect()
}

impl Family {
	fn weights(&self) -> Vec<u32> {
		(self.min..=self.max).step_by(100).collect()
	}
}

/// A font at one weight, ready to shape and draw.
#[derive(Clone)]
pub struct Face {
	pub font: Font,
	pub upem: u16,
}

impl Face {
	fn new(font: Font) -> Self {
		let upem = font.units_per_em();
		Self { font, upem }
	}
}

/// One weight of a user family.
#[derive(Clone, Debug)]
struct UserFace {
	weight: u32,
	/// 64 lower-case hex characters.
	hash: String,
}

/// One file of a document's fonts map (`timeline.ProjectFont` file): its weight and `sha256:<hex>`.
#[derive(Clone, Debug)]
pub struct DocFontFile {
	pub weight: u32,
	pub hash: String,
}

/// Gives the rasterizer its fonts: the bundled ones and the user fonts of a document. It is safe for
/// concurrent use.
pub struct FontSet {
	/// Parsed fonts, keyed by bundled family name or by the hex digest of a user font.
	parsed: Mutex<HashMap<String, Font>>,
	user: HashMap<String, Vec<UserFace>>,
	src: Option<FontSource>,
}

impl FontSet {
	/// The font set that has the bundled families only.
	pub fn bundled() -> Self {
		Self {
			parsed: Mutex::new(HashMap::new()),
			user: HashMap::new(),
			src: None,
		}
	}

	/// The bundled set plus the families of a document's fonts map (SDD-001 section 3.9), whose files
	/// `src` reads by hash. Bundled families win: the document validation forbids a key that names one.
	pub fn new(doc_fonts: &[(String, Vec<DocFontFile>)], src: Option<FontSource>) -> Self {
		let mut set = Self {
			parsed: Mutex::new(HashMap::new()),
			user: HashMap::new(),
			src,
		};
		set.set_doc_fonts(doc_fonts);
		set
	}

	/// Replaces the families of the document's fonts map, keeping the fonts already parsed (they are
	/// keyed by the digest of their bytes, so another document can reuse them).
	pub fn set_doc_fonts(&mut self, doc_fonts: &[(String, Vec<DocFontFile>)]) {
		let mut user: HashMap<String, Vec<UserFace>> = HashMap::new();
		for (name, files) in doc_fonts {
			let faces = user.entry(name.clone()).or_default();
			for f in files {
				faces.push(UserFace {
					weight: f.weight,
					hash: f.hash.strip_prefix("sha256:").unwrap_or(&f.hash).to_owned(),
				});
			}
		}
		self.user = user;
	}

	/// Returns a face of the family at the weight, or a [`FontError`] with the code `E_FONT_UNKNOWN`.
	pub fn lookup(&self, family: &str, weight: u32) -> Result<Face, LookupError> {
		if let Some(f) = FAMILIES.iter().find(|f| f.name == family) {
			if weight < f.min || weight > f.max || !weight.is_multiple_of(100) {
				return Err(FontError {
					family: family.to_owned(),
					weight,
					weights: Some(f.weights()),
					hash: None,
				}
				.into());
			}
			let parsed = self.load_bundled(f)?;
			if f.variable {
				return Ok(Face::new(
					parsed
						.instance_builder()
						.variations([("wght", weight as f32)])
						.build(),
				));
			}
			return Ok(Face::new(parsed));
		}
		if let Some(faces) = self.user.get(family) {
			return self.lookup_user(family, weight, faces);
		}
		Err(FontError {
			family: family.to_owned(),
			weight,
			weights: None,
			hash: None,
		}
		.into())
	}

	fn cache(&self) -> std::sync::MutexGuard<'_, HashMap<String, Font>> {
		self.parsed.lock().unwrap_or_else(PoisonError::into_inner)
	}

	fn load_bundled(&self, f: &Family) -> Result<Font, LookupError> {
		let mut cache = self.cache();
		if let Some(font) = cache.get(f.name) {
			return Ok(font.clone());
		}
		let font = Font::new(f.data, 0).ok_or_else(|| {
			LookupError::Source(format!("bundled font {} does not parse", f.name))
		})?;
		cache.insert(f.name.to_owned(), font.clone());
		Ok(font)
	}

	/// Resolves a family of the document's fonts map to a face. The family and weight come from the
	/// document, never from the font's name table.
	fn lookup_user(
		&self,
		family: &str,
		weight: u32,
		faces: &[UserFace],
	) -> Result<Face, LookupError> {
		let Some(face) = faces.iter().find(|f| f.weight == weight) else {
			let mut weights: Vec<u32> = faces.iter().map(|f| f.weight).collect();
			weights.sort_unstable();
			return Err(FontError {
				family: family.to_owned(),
				weight,
				weights: Some(weights),
				hash: None,
			}
			.into());
		};
		Ok(Face::new(self.load_user(family, weight, &face.hash)?))
	}

	fn load_user(&self, family: &str, weight: u32, hash: &str) -> Result<Font, LookupError> {
		let mut cache = self.cache();
		if let Some(font) = cache.get(hash) {
			return Ok(font.clone());
		}
		let not_stored = || FontError {
			family: family.to_owned(),
			weight,
			weights: None,
			hash: Some(hash.to_owned()),
		};
		let Some(src) = &self.src else {
			return Err(not_stored().into());
		};
		let data = match src(hash) {
			Ok(d) => d,
			Err(SourceError::NotStored) => return Err(not_stored().into()),
			Err(SourceError::Other(m)) => {
				return Err(LookupError::Source(format!("read font {hash}: {m}")));
			}
		};
		if crate::validate::sha256_hex(&data) != hash {
			return Err(LookupError::Source(format!(
				"font {hash} does not match its hash"
			)));
		}
		let font = validate_font(data)?;
		cache.insert(hash.to_owned(), font.clone());
		Ok(font)
	}

	/// The characters of `text` that the family has no glyph for, in the order they first appear, each
	/// once. Those are drawn as `.notdef`: there is no fallback to another font (SDD-004 section 5.1
	/// item 9). Control and format characters (line breaks, joiners) are not glyphs and are skipped.
	pub fn missing(&self, family: &str, weight: u32, text: &str) -> Result<Vec<char>, LookupError> {
		let face = self.lookup(family, weight)?;
		let charmap = face.font.charmap();
		let mut out: Vec<char> = Vec::new();
		for r in text.chars() {
			if r.is_control() || is_format(r) || out.contains(&r) {
				continue;
			}
			if charmap.map_unicode(r).is_none() {
				out.push(r);
			}
		}
		Ok(out)
	}
}

/// Unicode category Cf (format), the characters `unicode.Is(unicode.Cf, r)` reports in Go.
fn is_format(c: char) -> bool {
	matches!(
		c as u32,
		0xAD | 0x600..=0x605
			| 0x61C
			| 0x6DD
			| 0x70F
			| 0x890..=0x891
			| 0x8E2
			| 0x180E
			| 0x200B..=0x200F
			| 0x202A..=0x202E
			| 0x2060..=0x2064
			| 0x2066..=0x206F
			| 0xFEFF
			| 0xFFF9..=0xFFFB
			| 0x110BD
			| 0x110CD
			| 0x13430..=0x1343F
			| 0x1BCA0..=0x1BCA3
			| 0x1D173..=0x1D17A
			| 0xE0001
			| 0xE0020..=0xE007F
	)
}
