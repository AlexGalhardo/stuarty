//! The closed set of ways the renderer fails (SDD-013 section 7.1).

use core::fmt;

/// A `reason` of SDD-013 section 7.1 that has a status. The numbers are part of the API version.
#[derive(Clone, Copy, Debug, PartialEq, Eq)]
pub enum Reason {
	InvalidPlan = 1,
	UnsupportedVersion = 2,
	UnsupportedLayer = 3,
	MissingTexture = 4,
	LimitExceeded = 5,
	OutOfMemory = 6,
	InvalidArgument = 7,
	InvalidSpec = 8,
	FontUnknown = 9,
	FontInvalid = 10,
	FontNeeded = 11,
	TextFailed = 12,
}

impl Reason {
	/// The negative status a host receives.
	pub fn status(self) -> i32 {
		-(self as i32)
	}

	/// The `reason` string of the section.
	pub fn as_str(self) -> &'static str {
		match self {
			Self::InvalidPlan => "invalid_plan",
			Self::UnsupportedVersion => "unsupported_version",
			Self::UnsupportedLayer => "unsupported_layer",
			Self::MissingTexture => "missing_texture",
			Self::LimitExceeded => "limit_exceeded",
			Self::OutOfMemory => "out_of_memory",
			Self::InvalidArgument => "invalid_argument",
			Self::InvalidSpec => "invalid_spec",
			Self::FontUnknown => "font_unknown",
			Self::FontInvalid => "font_invalid",
			Self::FontNeeded => "font_needed",
			Self::TextFailed => "text_failed",
		}
	}
}

/// A failure: the reason and a detail that names an item, a key or a limit, never a pixel or a path.
#[derive(Clone, Debug, PartialEq, Eq)]
pub struct Error {
	pub reason: Reason,
	pub detail: String,
}

impl Error {
	pub fn new(reason: Reason, detail: impl Into<String>) -> Self {
		Self {
			reason,
			detail: detail.into(),
		}
	}
}

impl fmt::Display for Error {
	fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
		write!(f, "{}: {}", self.reason.as_str(), self.detail)
	}
}

impl std::error::Error for Error {}
