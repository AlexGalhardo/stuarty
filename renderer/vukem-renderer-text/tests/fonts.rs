//! The font set: bundled families, user fonts by hash and the errors of both (port of
//! `renderer/fonts_test.go` and `userfont_test.go`).

use std::sync::Arc;

use vukem_renderer_text::Colour;
use vukem_renderer_text::fonts::{DocFontFile, FontSet, LookupError, SourceError, families};
use vukem_renderer_text::text::{Align, Content, Style, TextSpec, measure_text, render_text};
use vukem_renderer_text::validate::Reason;

const ANTON: &[u8] = include_bytes!("../../fonts/Anton-Regular.ttf");

fn hex_sha256(data: &[u8]) -> String {
	vukem_renderer_text::validate::sha256_hex(data)
}

fn style(family: &str, weight: u32) -> Style {
	let white = Colour {
		r: 255,
		g: 255,
		b: 255,
		a: 255,
	};
	Style {
		font_family: family.to_owned(),
		font_weight: weight,
		size: 48.0,
		color: white,
		stroke_color: Colour {
			r: 0,
			g: 0,
			b: 0,
			a: 255,
		},
		stroke_width: 2.0,
		shadow_color: Colour::default(),
		shadow_dx: 0.0,
		shadow_dy: 0.0,
		shadow_blur: 0.0,
		background: None,
		background_padding: 8.0,
		background_radius: 0.0,
		align: Align::Center,
		line_height: 1.2,
		max_width: 600.0,
		uppercase: false,
		highlight: true,
		highlight_color: white,
		highlight_background: None,
	}
}

fn spec(family: &str, weight: u32, text: &str) -> TextSpec {
	TextSpec {
		content: Content::Text(text.to_owned()),
		style: style(family, weight),
		pixel_scale: 1.0,
	}
}

fn user_set(hash: &str, bytes: Vec<u8>) -> FontSet {
	let h = hash.to_owned();
	FontSet::new(
		&[(
			"Mine".to_owned(),
			vec![DocFontFile {
				weight: 400,
				hash: format!("sha256:{hash}"),
			}],
		)],
		Some(Arc::new(move |want: &str| {
			if want == h {
				Ok(bytes.clone())
			} else {
				Err(SourceError::NotStored)
			}
		})),
	)
}

#[test]
fn the_bundled_families_start_with_inter() {
	let names = families();
	assert_eq!(names[0], "Inter");
	assert_eq!(names.len(), 6);
}

#[test]
fn an_unknown_family_and_an_unknown_weight_are_font_errors() {
	let fonts = FontSet::bundled();
	let Err(LookupError::Font(e)) = fonts.lookup("Comic Sans", 400) else {
		panic!("an unknown family must be refused");
	};
	assert_eq!(e.weights, None);
	assert!(e.to_string().starts_with("E_FONT_UNKNOWN: no font family"));

	let Err(LookupError::Font(e)) = fonts.lookup("Inter", 650) else {
		panic!("a weight between two steps must be refused");
	};
	assert_eq!(
		e.weights,
		Some(vec![100, 200, 300, 400, 500, 600, 700, 800, 900])
	);

	let Err(LookupError::Font(e)) = fonts.lookup("Anton", 700) else {
		panic!("Anton has one weight");
	};
	assert_eq!(e.weights, Some(vec![400]));
	assert!(e.to_string().contains("available: [400]"));
}

#[test]
fn missing_lists_the_characters_the_family_has_no_glyph_for() {
	let fonts = FontSet::bundled();
	let got = fonts
		.missing("Inter", 400, "Aa \u{4f60}\u{597d}\n\u{4f60}\u{200d}")
		.unwrap();
	assert_eq!(got, vec!['\u{4f60}', '\u{597d}']);
}

#[test]
fn a_user_font_draws_the_same_bytes_as_the_same_file_bundled() {
	let hash = hex_sha256(ANTON);
	let fonts = user_set(&hash, ANTON.to_vec());
	let user = render_text(&spec("Mine", 400, "User font"), &fonts).unwrap();
	let bundled = render_text(&spec("Anton", 400, "User font"), &FontSet::bundled()).unwrap();
	assert_eq!(user, bundled);
}

#[test]
fn a_user_font_that_is_not_stored_is_a_font_error_with_its_hash() {
	let hash = hex_sha256(ANTON);
	let fonts = user_set(&"0".repeat(64), ANTON.to_vec());
	let fonts_missing = FontSet::new(
		&[(
			"Mine".to_owned(),
			vec![DocFontFile {
				weight: 400,
				hash: format!("sha256:{hash}"),
			}],
		)],
		Some(Arc::new(|_: &str| Err(SourceError::NotStored))),
	);
	let Err(LookupError::Font(e)) = fonts_missing.lookup("Mine", 400) else {
		panic!("a font that is not stored must be refused");
	};
	assert_eq!(e.hash.as_deref(), Some(hash.as_str()));
	assert!(e.to_string().contains("is not in the workspace"));

	// A weight the family does not have lists the ones it has.
	let Err(LookupError::Font(e)) = fonts.lookup("Mine", 700) else {
		panic!("an unknown weight must be refused");
	};
	assert_eq!(e.weights, Some(vec![400]));
}

#[test]
fn a_user_font_with_the_wrong_bytes_or_a_broken_file_is_refused() {
	// The bytes do not match their hash.
	let hash = hex_sha256(b"something else");
	let fonts = user_set(&hash, ANTON.to_vec());
	assert!(matches!(
		fonts.lookup("Mine", 400),
		Err(LookupError::Source(_))
	));

	// The bytes match their hash but are not a font.
	let junk = b"this is not a font file at all".to_vec();
	let fonts = user_set(&hex_sha256(&junk), junk);
	let Err(LookupError::Invalid(e)) = fonts.lookup("Mine", 400) else {
		panic!("junk must be refused");
	};
	assert_eq!(e.reason, Reason::BadMagic);
}

#[test]
fn measure_is_the_natural_size_of_the_render() {
	let fonts = FontSet::bundled();
	for text in ["Hello", "two\nlines of text", ""] {
		let s = spec("Anton", 400, text);
		let bm = render_text(&s, &fonts).unwrap();
		assert_eq!(
			measure_text(&s, &fonts),
			(bm.natural_w, bm.natural_h),
			"{text:?}"
		);
	}
}

#[test]
fn a_spec_that_cannot_be_drawn_is_refused_and_measures_nothing() {
	let fonts = FontSet::bundled();
	let mut s = spec("Anton", 400, "x");
	s.style.size = 0.0;
	assert!(render_text(&s, &fonts).is_err());
	assert_eq!(measure_text(&s, &fonts), (0.0, 0.0));

	let mut s = spec("Nope", 400, "x");
	assert!(render_text(&s, &fonts).is_err());
	assert_eq!(measure_text(&s, &fonts), (0.0, 0.0));

	s = spec("Anton", 400, "x");
	s.pixel_scale = 9.0;
	assert!(render_text(&s, &fonts).is_err());
}

/// An sfnt whose six tables are well placed and four-byte aligned but hold only junk.
fn junk_table_font() -> Vec<u8> {
	let tags = ["cmap", "head", "hhea", "hmtx", "maxp", "glyf"];
	let mut out = vec![0_u8; 12 + 16 * tags.len()];
	out[1] = 1;
	out[5] = tags.len() as u8;
	for (i, tag) in tags.iter().enumerate() {
		let off = out.len() as u32;
		let rec = 12 + 16 * i;
		out[rec..rec + 4].copy_from_slice(tag.as_bytes());
		out[rec + 8..rec + 12].copy_from_slice(&off.to_be_bytes());
		out[rec + 12..rec + 16].copy_from_slice(&40_u32.to_be_bytes());
		out.extend_from_slice(&[0xAB; 40]);
	}
	out
}

#[test]
fn a_font_whose_tables_are_junk_is_refused_even_when_its_structure_is_right() {
	let Err(e) = vukem_renderer_text::validate::check_font(&junk_table_font()) else {
		panic!("junk tables must be refused");
	};
	assert_eq!(e.reason, Reason::ParseFailed);
}
