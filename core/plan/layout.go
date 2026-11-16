package plan

import (
	"math"
	"strings"

	"github.com/AlexGalhardo/vukem-video/core/timeline"
)

// verticalAspect is the largest width/height treated as vertical video.
const verticalAspect = 0.6

// SafeArea is the rectangle of the frame that platform interfaces leave
// uncovered (SDD-004 §4.1). It is the only place the margins exist: the
// default position of captions and the lint rule for text outside it both
// use it, so the two cannot disagree. For a frame whose width divided by
// height is at most 0.6 the margins are 14% at the top, 35% at the bottom and
// 6% on each side (Meta's ads guide for Reels, D-018); otherwise 5% on every
// side. Edges are rounded to whole pixels: 1080×1920 gives {65, 269, 950, 979}.
func SafeArea(width, height int) Rect {
	l, r, t, b := 0.05, 0.05, 0.05, 0.05
	if height > 0 && float64(width)/float64(height) <= verticalAspect {
		l, r, t, b = 0.06, 0.06, 0.14, 0.35
	}

	w, h := float64(width), float64(height)
	x0, y0 := math.Round(l*w), math.Round(t*h)
	x1, y1 := w-math.Round(r*w), h-math.Round(b*h)

	return Rect{X: x0, Y: y0, W: x1 - x0, H: y1 - y0}
}

// Defaults of SDD-001 §3.6 and §3.7.
const (
	defaultTextSize       = 72
	defaultCaptionSize    = 64
	textMaxWidthFraction  = 0.86
	captionMaxWidthFactor = 0.8
)

func colorString(doc *timeline.Document, c *timeline.Color) string {
	if c == nil {
		return ""
	}

	return resolveColor(doc, string(*c))
}

// resolveColor replaces a "$name" slot with the template's colour. A slot
// without a value stays as written (Validate reports it as V_COLOR_REF).
func resolveColor(doc *timeline.Document, c string) string {
	if strings.HasPrefix(c, "$") && doc.Template != nil {
		if v, ok := doc.Template.Colors[c[1:]]; ok {
			return string(v)
		}
	}

	return c
}

// applyStyle lays the fields that are set in s over st.
func applyStyle(doc *timeline.Document, st *ResolvedStyle, s *timeline.TextStyle) {
	if s == nil {
		return
	}

	setStr(&st.FontFamily, s.FontFamily)
	setNum(&st.FontWeight, s.FontWeight)
	setNum(&st.Size, s.Size)
	setColor(doc, &st.Color, s.Color)
	setColor(doc, &st.StrokeColor, s.StrokeColor)
	setNum(&st.StrokeWidth, s.StrokeWidth)
	setColor(doc, &st.ShadowColor, s.ShadowColor)
	setNum(&st.ShadowDX, s.ShadowDX)
	setNum(&st.ShadowDY, s.ShadowDY)
	setNum(&st.ShadowBlur, s.ShadowBlur)
	setColor(doc, &st.BackgroundColor, s.BackgroundColor)
	setNum(&st.BackgroundPadding, s.BackgroundPadding)
	setNum(&st.BackgroundRadius, s.BackgroundRadius)
	setStr(&st.Align, s.Align)
	setNum(&st.LineHeight, s.LineHeight)
	setNum(&st.MaxWidth, s.MaxWidth)
	setNum(&st.Uppercase, s.Uppercase)
}

func setStr(dst *string, src *string) {
	if src != nil {
		*dst = *src
	}
}

func setNum[T int | float64 | bool](dst *T, src *T) {
	if src != nil {
		*dst = *src
	}
}

func setColor(doc *timeline.Document, dst *string, src *timeline.Color) {
	if src != nil {
		*dst = colorString(doc, src)
	}
}

// baseStyle is the defaults of a text item (SDD-001 §3.6) at the frame size.
func baseStyle(width int, size, maxWidthFraction float64) ResolvedStyle {
	return ResolvedStyle{
		FontFamily: "Inter", FontWeight: 700, Size: size,
		Color: "#FFFFFF", StrokeColor: "#000000", ShadowColor: "#00000080",
		BackgroundPadding: 16, Align: "center", LineHeight: 1.2,
		MaxWidth: round3(maxWidthFraction * float64(width)),
	}
}

// textStyle is the effective style of a text item: the defaults overridden by
// the item's style.
func textStyle(doc *timeline.Document, it *timeline.Item) ResolvedStyle {
	st := baseStyle(doc.Settings.Width, defaultTextSize, textMaxWidthFraction)
	if it.Style != nil {
		applyStyle(doc, &st, &it.Style.TextStyle)
	}

	if it.Style == nil || it.Style.FontWeight == nil {
		st.FontWeight = timeline.DefaultFontWeight(doc, st.FontFamily)
	}

	return st
}

// captionStyle is the effective style of a caption: the defaults, overridden
// by the track's caption_style, overridden by the item's style (SDD-001 §3.7).
// position is the effective vertical anchor.
func captionStyle(doc *timeline.Document, it *timeline.Item) (st ResolvedStyle, position string) {
	st = baseStyle(doc.Settings.Width, defaultCaptionSize, captionMaxWidthFactor)
	st.Highlight, st.HighlightColor = "none", "#FFE600"
	position = "bottom"

	layers := []*timeline.CaptionStyle{doc.Tracks[it.Track].CaptionStyle, it.Style}
	weightSet := false

	for _, cs := range layers {
		if cs == nil {
			continue
		}

		weightSet = weightSet || cs.FontWeight != nil

		applyStyle(doc, &st, &cs.TextStyle)
		setStr(&position, cs.Position)
		setStr(&st.Highlight, cs.Highlight)
		setColor(doc, &st.HighlightColor, cs.HighlightColor)
		setColor(doc, &st.HighlightBackground, cs.HighlightBackground)
	}

	if !weightSet {
		st.FontWeight = timeline.DefaultFontWeight(doc, st.FontFamily)
	}

	return st, position
}

// pixelScale is the oversampling factor of SDD-004 §5.3:
// the largest scale the item reaches times the output scale (0 means 1),
// rounded up to the next quarter and clamped to 0.25..4.
func pixelScale(it *timeline.Item, output float64) float64 {
	top := it.Scale
	for _, kf := range it.Keyframes["scale"] {
		top = max(top, kf.Value)
	}

	if output > 0 {
		top *= output
	}

	return min(max(math.Ceil(top*4)/4, 0.25), 4)
}

// activeWord is the word being spoken at frame r: the last word whose start is
// not after r, while r is before its end; -1 between words (SDD-004 §4.3).
func activeWord(words []timeline.Word, r int) int {
	idx := -1

	for i := range words {
		if words[i].Start <= r {
			idx = i
		}
	}

	if idx >= 0 && r < words[idx].Start+words[idx].Duration {
		return idx
	}

	return -1
}

// captionText joins the words with spaces, and with "\n" before each word
// named in breaks (SDD-004 §5).
func captionText(words []string, breaks []int) string {
	var b strings.Builder

	for i, w := range words {
		if i > 0 {
			sep := " "
			for _, br := range breaks {
				if br == i {
					sep = "\n"
				}
			}

			b.WriteString(sep)
		}

		b.WriteString(w)
	}

	return b.String()
}
