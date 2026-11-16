//! Shaping and line breaking: the port of `engine/raster/layout.go` (SDD-004 section 5.1).
//!
//! `engine/raster` shapes with go-text (a port of HarfBuzz) and wraps lines with its line wrapper.
//! The shaper here is `harfrust`, which follows HarfBuzz as well; the wrapper below keeps the
//! decisions of the go-text one for the configuration the rasterizer uses (the break policy
//! `Never`, no truncation): which UAX #14 opportunities are valid, what a candidate line measures,
//! when a line ends, how the trailing space is trimmed and how the runs of a line are ordered.

use crate::math::mul;
use harfrust::{Buffer, Direction, Language, ShapeOptions, ShaperFont, Tag};
use skrifa::GlyphId;
use unicode_bidi::BidiInfo;
use unicode_linebreak::{BreakOpportunity, linebreaks};
use unicode_script::Script as UScript;

use crate::Rect;
use crate::fonts::Face;
use crate::text::{Align, Style};

/// A glyph with its origin on the baseline, in block pixels.
#[derive(Clone, Copy, Debug, PartialEq)]
pub struct PlacedGlyph {
	pub gid: u32,
	pub x: f64,
	pub y: f64,
	/// The index of the glyph's cluster in the whole text, in characters.
	pub text: usize,
}

/// The result of shaping and line breaking: glyphs and the metrics of the block they fill. The block
/// starts at (0, 0).
#[derive(Clone, Debug)]
pub struct Layout {
	pub glyphs: Vec<PlacedGlyph>,
	pub width: f64,
	pub height: f64,
	/// The box of each word, in block pixels; a word with no glyph has an empty box. Empty for a text.
	pub words: Vec<Rect>,
	/// Pixels per font unit: size over units per em.
	pub scale: f64,
}

/// `EmptyGlyph` of go-text: the glyph that draws nothing.
const EMPTY_GLYPH: u32 = 0xFFFF;

/// A shaped glyph. The advance and the offsets are in 26.6 fixed point pixels at the size HarfBuzz
/// shaped at (`ceil(size)`), as go-text hands them over.
#[derive(Clone, Copy, Debug)]
struct Glyph {
	gid: u32,
	/// The cluster, an index in the paragraph's characters.
	cluster: usize,
	advance: i32,
	x_offset: i32,
	y_offset: i32,
	/// The width of the glyph's ink box at the shaping size, 26.6.
	width: i32,
}

/// A shaped run (`shaping.Output`): the glyphs of one direction and script, in the visual order of
/// the run.
#[derive(Clone, Debug)]
struct Run {
	rtl: bool,
	/// The characters of the paragraph the run covers.
	offset: usize,
	count: usize,
	glyphs: Vec<Glyph>,
	visual: i32,
}

impl Run {
	fn advance(&self) -> i32 {
		self.glyphs.iter().map(|g| g.advance).sum()
	}
}

/// One wrapped line: its runs in logical order and its width in pixels.
struct TextLine {
	runs: Vec<Run>,
	width: f64,
	/// The offset of its paragraph in the whole text.
	base: usize,
}

/// Shapes the characters of the text, breaks them into lines no wider than the maximum width and
/// places the glyphs. `spans` are the character ranges of the words of a caption; their boxes come
/// back in `Layout::words`.
pub fn layout_text(
	runes: &[char],
	spans: &[(usize, usize)],
	face: &Face,
	st: &Style,
	scale: f64,
) -> Layout {
	let size = mul(st.size, scale);
	let line_h = mul(size, st.line_height);
	let (asc, desc) = vertical_extents(face, size);
	let baseline = (line_h - (asc - desc)) / 2.0 + asc;

	let mut lines: Vec<TextLine> = Vec::new();
	for (start, end) in split_paragraphs(runes) {
		lines.extend(wrap_paragraph(
			&runes[start..end],
			start,
			face,
			size,
			mul(st.max_width, scale),
		));
	}

	let mut lay = Layout {
		glyphs: Vec::new(),
		width: 0.0,
		height: 0.0,
		words: vec![Rect::default(); spans.len()],
		scale: size / f64::from(face.upem),
	};
	for l in &lines {
		lay.width = lay.width.max(l.width);
	}
	lay.height = mul(line_h, lines.len() as f64);

	let mut boxes = vec![WordBox::default(); spans.len()];
	for (i, l) in lines.iter().enumerate() {
		let x = align_offset(st.align, lay.width - l.width);
		let top = mul(i as f64, line_h);
		lay.place(
			l,
			x,
			LineBox {
				top,
				baseline: top + baseline,
				height: line_h,
			},
			spans,
			&mut boxes,
		);
	}
	for (w, b) in lay.words.iter_mut().zip(&boxes) {
		*w = b.rect();
	}
	lay
}

/// The ascent (positive) and descent (negative) of the face at the given size in pixels.
fn vertical_extents(face: &Face, size: f64) -> (f64, f64) {
	let k = size / f64::from(face.upem);
	match face.font.metrics().h_line() {
		Some(line) => (
			mul(f64::from(line.ascender.to_f32()), k),
			mul(f64::from(line.descender.to_f32()), k),
		),
		None => (mul(size, 0.8), -mul(size, 0.2)),
	}
}

fn align_offset(align: Align, free: f64) -> f64 {
	match align {
		Align::Left => 0.0,
		Align::Right => free,
		Align::Center => free / 2.0,
	}
}

/// The `[start, end)` ranges between forced breaks. A text that ends with a break has a last, empty
/// paragraph, so the break counts as a line.
fn split_paragraphs(runes: &[char]) -> Vec<(usize, usize)> {
	let mut out = Vec::new();
	let mut start = 0;
	for (i, &r) in runes.iter().enumerate() {
		if r == '\n' {
			out.push((start, i));
			start = i + 1;
		}
	}
	out.push((start, runes.len()));
	out
}

fn fix(v: i32) -> f64 {
	f64::from(v) / 64.0
}

/// The break option of UAX #14: after the character at `at`.
#[derive(Clone, Copy)]
struct BreakOption {
	at: usize,
	required: bool,
}

fn break_options(par: &[char]) -> Vec<BreakOption> {
	let text: String = par.iter().collect();
	let mut char_of_byte = vec![0_usize; text.len() + 1];
	for (ci, (bi, c)) in text.char_indices().enumerate() {
		for k in 0..c.len_utf8() {
			char_of_byte[bi + k] = ci;
		}
	}
	char_of_byte[text.len()] = par.len();
	linebreaks(&text)
		.filter_map(|(byte, opp)| {
			let next = char_of_byte[byte];
			let at = next.checked_sub(1)?;
			Some(BreakOption {
				at,
				required: opp == BreakOpportunity::Mandatory && at != par.len() - 1,
			})
		})
		.collect()
}

/// Shapes one paragraph and breaks it into lines. `base` is the index of the paragraph's first
/// character in the whole text.
fn wrap_paragraph(
	par: &[char],
	base: usize,
	face: &Face,
	size: f64,
	max_width: f64,
) -> Vec<TextLine> {
	if par.is_empty() {
		return vec![TextLine {
			runs: Vec::new(),
			width: 0.0,
			base,
		}];
	}

	let text: String = par.iter().collect();
	let info = BidiInfo::new(&text, None);
	let mut levels = Vec::with_capacity(par.len());
	let mut byte_levels = info.levels.iter();
	for c in text.chars() {
		levels.push(byte_levels.next().copied());
		for _ in 1..c.len_utf8() {
			byte_levels.next();
		}
	}
	let rtl_of = |i: usize| levels[i].is_some_and(|l| l.is_rtl());

	// go-text's Size: the size in 26.6, rounded; the shaper uses its ceiling in whole pixels.
	let size64 = (size * 64.0).round() as i64;
	let ppem = ((size64 + 63) >> 6) as i32;

	let mut dir_rtl = false;
	let mut runs: Vec<Run> = Vec::new();
	let mut start = 0;
	while start < par.len() {
		let rtl = rtl_of(start);
		let mut end = start + 1;
		while end < par.len() && rtl_of(end) == rtl {
			end += 1;
		}
		if start == 0 {
			dir_rtl = rtl;
		}
		for (s, e, script) in split_by_script(par, start, end) {
			runs.push(Run {
				rtl,
				offset: s,
				count: e - s,
				glyphs: shape_piece(face, par, s, e, rtl, script, ppem),
				visual: 0,
			});
		}
		start = end;
	}

	let max64 = (max_width * 64.0).round() as i64;
	let options = break_options(par);
	let mut lines = wrap_lines(&runs, &options, par.len(), dir_rtl, max64);
	for l in &mut lines {
		l.base = base;
	}
	lines
}

/// Cuts `[start, end)` where the script changes. Common and Inherited characters (spaces, digits,
/// punctuation, combining marks) join the script before them, or the one after them at the start of
/// the run.
fn split_by_script(text: &[char], start: usize, end: usize) -> Vec<(usize, usize, UScript)> {
	let mut out = Vec::new();
	let (mut cur_start, mut cur_script) = (start, UScript::Common);
	for (i, &c) in text.iter().enumerate().take(end).skip(start) {
		let s = UScript::from(c);
		if s == UScript::Common || s == UScript::Inherited {
			continue;
		}
		if cur_script == UScript::Common || cur_script == UScript::Inherited {
			cur_script = s;
			continue;
		}
		if s != cur_script {
			out.push((cur_start, i, cur_script));
			cur_start = i;
			cur_script = s;
		}
	}
	out.push((cur_start, end, cur_script));
	out
}

fn harf_script(s: UScript) -> Option<harfrust::Script> {
	let name = s.short_name().as_bytes();
	let tag: [u8; 4] = name.try_into().ok()?;
	harfrust::Script::from_iso15924_tag(Tag::new(&tag))
}

/// The Unicode category Zs.
fn is_space_separator(c: char) -> bool {
	matches!(
		c as u32,
		0x20 | 0xA0 | 0x1680 | 0x2000..=0x200A | 0x202F | 0x205F | 0x3000
	)
}

fn shape_piece(
	face: &Face,
	text: &[char],
	start: usize,
	end: usize,
	rtl: bool,
	script: UScript,
	ppem: i32,
) -> Vec<Glyph> {
	let mut buf = Buffer::new();
	for (i, &c) in text.iter().enumerate().take(end).skip(start) {
		buf.push(c as u32, i as u32);
	}
	let pre: Vec<u32> = text[..start]
		.iter()
		.rev()
		.take(5)
		.map(|&c| c as u32)
		.collect();
	let post: Vec<u32> = text[end..].iter().take(5).map(|&c| c as u32).collect();
	buf.set_pre_context_codepoints(&pre);
	buf.set_post_context_codepoints(&post);
	buf.set_direction(if rtl {
		Direction::RightToLeft
	} else {
		Direction::LeftToRight
	});
	buf.set_script(harf_script(script));
	buf.set_language("en".parse::<Language>().ok());

	let font = ShaperFont::new(&face.font).with_scale(ppem << 6);
	if harfrust::shape(&font, &mut buf, ShapeOptions::new()).is_err() {
		return Vec::new();
	}

	let infos = buf.glyph_infos();
	let pos = buf.glyph_positions();
	let mut glyphs: Vec<Glyph> = infos
		.iter()
		.zip(pos)
		.map(|(i, p)| {
			let width = font
				.glyph_extents(GlyphId::new(i.glyph_id))
				.map_or(0, |e| e.width);
			Glyph {
				gid: i.glyph_id,
				cluster: i.cluster as usize,
				advance: p.x_advance,
				x_offset: p.x_offset,
				y_offset: p.y_offset,
				width,
			}
		})
		.collect();

	let counts = count_clusters(&glyphs, end, rtl);
	for (g, (glyph_count, rune_count)) in glyphs.iter_mut().zip(counts) {
		if g.gid == 0
			&& glyph_count == 1
			&& rune_count == 1
			&& text.get(g.cluster).is_some_and(|&c| is_space_separator(c))
		{
			g.gid = EMPTY_GLYPH;
			g.width = 0;
		}
	}
	glyphs
}

/// The number of glyphs and of characters of the cluster of each glyph (`countClusters`).
fn count_clusters(glyphs: &[Glyph], text_len: usize, rtl: bool) -> Vec<(usize, usize)> {
	let mut out = vec![(0, 0); glyphs.len()];
	let mut current: Option<usize> = None;
	let (mut runes_in, mut glyphs_in) = (0, 0);
	let mut previous = text_len;
	for i in 0..glyphs.len() {
		let g = glyphs[i].cluster;
		if current != Some(g) {
			glyphs_in = 1;
			current = Some(g);
			let mut next = None;
			for later in &glyphs[i + 1..] {
				if later.cluster == g {
					glyphs_in += 1;
				} else {
					next = Some(later.cluster);
					break;
				}
			}
			let next = next.unwrap_or(text_len);
			runes_in = if rtl {
				previous.saturating_sub(g)
			} else {
				next.saturating_sub(g)
			};
			previous = g;
		}
		out[i] = (glyphs_in, runes_in);
	}
	out
}

/// The cut of a run to the characters `[lo, hi]`: the glyphs of the clusters inside the range.
fn cut(run: &Run, lo: usize, hi: usize) -> Run {
	Run {
		rtl: run.rtl,
		offset: run.offset.max(lo),
		count: (run.offset + run.count)
			.min(hi + 1)
			.saturating_sub(run.offset.max(lo)),
		glyphs: run
			.glyphs
			.iter()
			.copied()
			.filter(|g| g.cluster >= lo && g.cluster <= hi)
			.collect(),
		visual: run.visual,
	}
}

/// The advance of the cut run, less the trailing space or end letter spacing
/// (`advanceSpaceAware`): only when the run runs in the direction of the paragraph.
fn advance_space_aware(run: &Run, dir_rtl: bool) -> i32 {
	let adv = run.advance();
	if run.glyphs.is_empty() || run.rtl != dir_rtl {
		return adv;
	}
	let last = if run.rtl {
		run.glyphs[0]
	} else {
		run.glyphs[run.glyphs.len() - 1]
	};
	if last.width == 0 {
		adv - last.advance
	} else {
		adv
	}
}

/// The wrapper of go-text for the break policy `Never`, without truncation.
fn wrap_lines(
	runs: &[Run],
	options: &[BreakOption],
	total: usize,
	dir_rtl: bool,
	max64: i64,
) -> Vec<TextLine> {
	// The single run that fits and has no forced break is the line, untouched.
	if runs.len() == 1
		&& !options.iter().any(|o| o.required)
		&& i64::from(runs[0].advance()) <= max64
	{
		let run = runs[0].clone();
		let width = fix(run.advance());
		return vec![TextLine {
			runs: vec![run],
			width,
			base: 0,
		}];
	}

	let mut lines: Vec<TextLine> = Vec::new();
	let mut line_start = 0_usize;
	let mut oi = 0_usize;

	while line_start < total {
		// The best line so far and where the next one starts if it is the one.
		let mut best: Option<(Vec<Run>, usize)> = None;

		while oi < options.len() {
			let option = options[oi];
			let Some(k) = runs
				.iter()
				.position(|r| option.at >= r.offset && option.at < r.offset + r.count)
			else {
				oi += 1;
				continue;
			};
			// A break before the line, or inside a cluster, is not an option.
			if option.at < line_start {
				oi += 1;
				continue;
			}
			let next = option.at + 1;
			if next < runs[k].offset + runs[k].count
				&& !runs[k].glyphs.iter().any(|g| g.cluster == next)
			{
				oi += 1;
				continue;
			}

			let mut candidate: Vec<Run> = Vec::new();
			let mut prior = 0_i64;
			for run in &runs[..k] {
				if line_start >= run.offset + run.count {
					continue;
				}
				let c = cut(run, line_start, usize::MAX - 1);
				prior += i64::from(c.advance());
				candidate.push(c);
			}
			let last = cut(&runs[k], line_start, option.at);
			let width = i64::from(advance_space_aware(&last, dir_rtl)) + prior;
			candidate.push(last);

			if width > max64 {
				// The first option does not fit: the line is that option anyway; otherwise the line
				// is the best one and this option starts the next.
				if best.is_none() {
					best = Some((candidate, next));
					oi += 1;
				}
				break;
			}
			best = Some((candidate, next));
			oi += 1;
			if option.required {
				break;
			}
		}

		let Some((mut line, next_start)) = best else {
			break;
		};
		line_start = next_start;
		order_and_trim(&mut line, dir_rtl);
		let width = line.iter().map(|r| fix(r.advance())).sum();
		lines.push(TextLine {
			runs: line,
			width,
			base: 0,
		});
	}
	lines
}

fn order_and_trim(line: &mut [Run], dir_rtl: bool) {
	if line.is_empty() {
		return;
	}
	compute_bidi_ordering(dir_rtl, line);
	let goal = if dir_rtl { 0 } else { line.len() as i32 - 1 };
	let Some(idx) = line.iter().position(|r| r.visual == goal) else {
		return;
	};
	let run = &mut line[idx];
	let n = run.glyphs.len();
	if n == 0 {
		return;
	}
	let g = if dir_rtl { 0 } else { n - 1 };
	if run.glyphs[g].width == 0 {
		run.glyphs[g].advance = 0;
	}
}

fn swap_visual_order(sub: &mut [Run]) {
	let l = sub.len();
	for i in 0..l / 2 {
		let j = l - i - 1;
		let (a, b) = (sub[i].visual, sub[j].visual);
		sub[i].visual = b;
		sub[j].visual = a;
	}
}

/// Resolves the visual index of each run (`computeBidiOrdering`).
fn compute_bidi_ordering(dir_rtl: bool, line: &mut [Run]) {
	let mut bidi_start: Option<usize> = None;
	let n = line.len();
	for idx in 0..n {
		let base = if dir_rtl { n - 1 - idx } else { idx };
		line[idx].visual = base as i32;
		if line[idx].rtl == dir_rtl {
			if let Some(s) = bidi_start.take() {
				swap_visual_order(&mut line[s..idx]);
			}
		} else if bidi_start.is_none() {
			bidi_start = Some(idx);
		}
	}
	if let Some(s) = bidi_start {
		swap_visual_order(&mut line[s..]);
	}
}

/// The extent of the glyphs of one word.
#[derive(Clone, Copy, Default)]
struct WordBox {
	x0: f64,
	x1: f64,
	y0: f64,
	y1: f64,
	set: bool,
}

impl WordBox {
	fn add(&mut self, x0: f64, x1: f64, y0: f64, y1: f64) {
		if !self.set {
			*self = Self {
				x0,
				x1,
				y0,
				y1,
				set: true,
			};
			return;
		}
		self.x0 = self.x0.min(x0);
		self.x1 = self.x1.max(x1);
		self.y0 = self.y0.min(y0);
		self.y1 = self.y1.max(y1);
	}

	fn rect(&self) -> Rect {
		if !self.set {
			return Rect::default();
		}
		Rect {
			x: self.x0,
			y: self.y0,
			w: self.x1 - self.x0,
			h: self.y1 - self.y0,
		}
	}
}

impl Layout {
	/// Puts the glyphs of a line on the block. `x` is where the line starts; the line box has the
	/// top and baseline y coordinates and the height.
	fn place(
		&mut self,
		l: &TextLine,
		x: f64,
		at: LineBox,
		spans: &[(usize, usize)],
		boxes: &mut [WordBox],
	) {
		let mut runs: Vec<&Run> = l.runs.iter().collect();
		runs.sort_by_key(|r| r.visual);
		let mut pen = x;
		for run in runs {
			for g in &run.glyphs {
				let idx = l.base + g.cluster;
				let adv = fix(g.advance);
				self.glyphs.push(PlacedGlyph {
					gid: g.gid,
					x: pen + fix(g.x_offset),
					y: at.baseline - fix(g.y_offset),
					text: idx,
				});
				if let Some(k) = word_at(spans, idx) {
					boxes[k].add(pen, pen + adv, at.top, at.top + at.height);
				}
				pen += adv;
			}
		}
	}
}

/// The y coordinates of a line box: its top, its baseline and its height.
#[derive(Clone, Copy)]
struct LineBox {
	top: f64,
	baseline: f64,
	height: f64,
}

fn word_at(spans: &[(usize, usize)], idx: usize) -> Option<usize> {
	spans.iter().position(|&(a, b)| idx >= a && idx < b)
}
