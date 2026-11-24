package command

import (
	"maps"
	"math"
	"slices"
	"strings"

	"github.com/AlexGalhardo/stuarty/core/timeline"
)

// The named caption styles of caption.apply_style and caption.list_styles (SDD-002 C.88, C.89). They are
// original; each uses a bundled font family only. A length is in pixels of a frame whose shorter side is
// captionStyleRefSide, and is scaled to the project when the style is applied.

func init() {
	Register(Define(Spec{
		Name:    "caption.apply_style",
		Summary: "Applies a named style to a caption track.",
		Description: "Replaces the caption style of a visual track with a named style: clean_white, bold_pop, " +
			"marker_yellow, soft_box, neon_night, editorial, mono_code, karaoke_pill, thick_outline, warm_glow, " +
			"lower_bar, big_center, top_headline or ink_paper. Lengths scale with the shorter side of the project. " +
			"Captions with their own style override keep it unless reset_overrides is true; effective.overridden_items " +
			"lists them. caption.list_styles describes the styles.",
		Example: `{"track_id": "trk_a1b2c3d4e5", "style": "bold_pop"}`,
		Errors:  []string{CodeValidation, CodeNotFound, CodeTypeMismatch, CodeFontUnknown},
		Scope:   ScopeEdit,
	}, applyCaptionApplyStyle))

	Register(Define(Spec{
		Name:    "caption.list_styles",
		Summary: "Describes the named caption styles.",
		Description: "Changes nothing. The result lists each style of caption.apply_style with its name, a summary, " +
			"its font family and the stored fields for a project whose shorter side is 1080 pixels.",
		Example: `{}`,
		Errors:  []string{CodeValidation},
		Scope:   ScopeEdit,
	}, applyCaptionListStyles))
}

// captionStyleRefSide is the shorter side of the frame the lengths of the table are written for.
const captionStyleRefSide = 1080

// CaptionStyleSpec is one row of the table of SDD-002 C.88.
type CaptionStyleSpec struct {
	ID      string
	Name    string
	Summary string
	Style   timeline.CaptionStyle
	// LineWidth is the max_width of the style as a fraction of the frame width; 0 keeps the default.
	LineWidth float64
}

func ptr[T any](v T) *T { return &v }

func colorPtr(s string) *timeline.Color { return ptr(timeline.Color(s)) }

// captionStyleBase is what every style sets: a font, its colours and the word highlight.
func captionStyleBase(family string, weight int, size float64, color, highlight string, position string) timeline.CaptionStyle {
	return timeline.CaptionStyle{
		TextStyle: timeline.TextStyle{
			FontFamily: ptr(family), FontWeight: ptr(weight), Size: ptr(size), Color: colorPtr(color),
			Align: ptr("center"),
		},
		Position:       ptr(position),
		Highlight:      ptr("word"),
		HighlightColor: colorPtr(highlight),
	}
}

func (s *CaptionStyleSpec) stroke(color string, width float64) *CaptionStyleSpec {
	s.Style.StrokeColor, s.Style.StrokeWidth = colorPtr(color), ptr(width)
	return s
}

func (s *CaptionStyleSpec) shadow(color string, dx, dy, blur float64) *CaptionStyleSpec {
	s.Style.ShadowColor, s.Style.ShadowDX, s.Style.ShadowDY, s.Style.ShadowBlur = colorPtr(color), ptr(dx), ptr(dy), ptr(blur)
	return s
}

func (s *CaptionStyleSpec) box(color string, padding, radius float64) *CaptionStyleSpec {
	s.Style.BackgroundColor, s.Style.BackgroundPadding, s.Style.BackgroundRadius = colorPtr(color), ptr(padding), ptr(radius)
	return s
}

func (s *CaptionStyleSpec) upper() *CaptionStyleSpec {
	s.Style.Uppercase = ptr(true)
	return s
}

func (s *CaptionStyleSpec) lineHeight(v float64) *CaptionStyleSpec {
	s.Style.LineHeight = ptr(v)
	return s
}

func (s *CaptionStyleSpec) highlightBox(color string) *CaptionStyleSpec {
	s.Style.HighlightBackground = colorPtr(color)
	return s
}

func newCaptionStyle(id, name, summary string, base timeline.CaptionStyle) *CaptionStyleSpec {
	return &CaptionStyleSpec{ID: id, Name: name, Summary: summary, Style: base}
}

// captionStyles is the table of SDD-002 C.88, in its order.
var captionStyles = func() []CaptionStyleSpec {
	left := captionStyleBase("Inter", 500, 52, "#FFFFFF", "#7DD3FC", "bottom")
	left.Align = ptr("left")
	lower := newCaptionStyle("lower_bar", "Lower bar", "Left-aligned text on a dark bar, calm and easy to read.", left).
		box("#000000A6", 18, 0)
	lower.LineWidth = 0.86

	return []CaptionStyleSpec{
		*newCaptionStyle("clean_white", "Clean white", "White bold text with a thin edge and a soft shadow.",
			captionStyleBase("Inter", 700, 72, "#FFFFFF", "#FFD23F", "bottom")).
			stroke("#000000", 3).shadow("#000000B3", 0, 3, 8),
		*newCaptionStyle("bold_pop", "Bold pop", "Tall condensed capitals with a heavy edge and a green spoken word.",
			captionStyleBase("Anton", 400, 92, "#FFFFFF", "#3DFF7A", "bottom")).
			upper().stroke("#000000", 8).shadow("#00000099", 0, 4, 0),
		*newCaptionStyle("marker_yellow", "Marker yellow", "Yellow text with a black edge, the spoken word turns white.",
			captionStyleBase("Montserrat", 800, 76, "#FFE600", "#FFFFFF", "bottom")).
			stroke("#000000", 6).lineHeight(1.1),
		*newCaptionStyle("soft_box", "Soft box", "White text on a rounded dark box, the spoken word on blue.",
			captionStyleBase("Inter", 600, 64, "#FFFFFF", "#FFFFFF", "bottom")).
			box("#111827CC", 22, 18).highlightBox("#2563EB"),
		*newCaptionStyle("neon_night", "Neon night", "Light capitals with a cyan glow, in the middle of the frame.",
			captionStyleBase("Bebas Neue", 400, 96, "#E8FFFF", "#FF4FD8", "middle")).
			upper().stroke("#00131A", 3).shadow("#00E5FF", 0, 0, 24),
		*newCaptionStyle("editorial", "Editorial", "A serif face in warm white, like a magazine pull quote.",
			captionStyleBase("Playfair Display", 700, 70, "#FFF8E7", "#F4B860", "bottom")).
			stroke("#1A1206", 2).shadow("#000000AA", 0, 2, 10).lineHeight(1.2),
		*newCaptionStyle("mono_code", "Mono code", "Green monospaced text on a near-black box.",
			captionStyleBase("JetBrains Mono", 700, 56, "#7CFFB2", "#FFFFFF", "bottom")).
			box("#0B0F14E6", 20, 8).highlightBox("#1F6F4A"),
		*newCaptionStyle("karaoke_pill", "Karaoke pill", "White text; the spoken word sits on a white pill.",
			captionStyleBase("Montserrat", 700, 66, "#FFFFFF", "#101010", "bottom")).
			stroke("#000000", 4).highlightBox("#FFFFFF"),
		*newCaptionStyle("thick_outline", "Thick outline", "Very heavy capitals with a thick black outline.",
			captionStyleBase("Montserrat", 900, 80, "#FFFFFF", "#FF5A36", "bottom")).
			upper().stroke("#000000", 14).lineHeight(1.05),
		*newCaptionStyle("warm_glow", "Warm glow", "Amber text with an orange glow.",
			captionStyleBase("Montserrat", 800, 74, "#FFB347", "#FFFFFF", "bottom")).
			stroke("#3A1500", 3).shadow("#FF6A00B3", 0, 0, 22),
		*lower,
		*newCaptionStyle("big_center", "Big centre", "One huge capital line in the middle, for short punchy words.",
			captionStyleBase("Anton", 400, 140, "#FFFFFF", "#FFE600", "middle")).
			upper().stroke("#000000", 10).lineHeight(0.95),
		*newCaptionStyle("top_headline", "Top headline", "Capitals at the top of the frame, like a headline.",
			captionStyleBase("Bebas Neue", 400, 88, "#FFFFFF", "#FF3B30", "top")).
			upper().stroke("#000000", 3).shadow("#000000CC", 0, 4, 6),
		*newCaptionStyle("ink_paper", "Ink on paper", "Dark serif text on a cream box.",
			captionStyleBase("Playfair Display", 700, 64, "#1B1B1B", "#B3261E", "bottom")).
			box("#F6EFE0F2", 24, 12),
	}
}()

// CaptionStyleIDs lists the style ids in the order of SDD-002 C.88.
func CaptionStyleIDs() []string {
	ids := make([]string, len(captionStyles))
	for i, s := range captionStyles {
		ids[i] = s.ID
	}
	return ids
}

// CaptionStyleByID returns the row of the table for an id.
func CaptionStyleByID(id string) (CaptionStyleSpec, bool) {
	for _, s := range captionStyles {
		if s.ID == id {
			return s, true
		}
	}
	return CaptionStyleSpec{}, false
}

// ResolveCaptionStyle is the stored style of a named style for a project of the given size: every length of the
// table is multiplied by the shorter side over 1080 and rounded (SDD-002 C.88).
func ResolveCaptionStyle(spec CaptionStyleSpec, width, height int) *timeline.CaptionStyle {
	k := float64(min(width, height)) / captionStyleRefSide
	s := spec.Style
	scale := func(p *float64, lo, hi float64, decimals float64) *float64 {
		if p == nil {
			return nil
		}
		f := math.Pow(10, decimals)
		return ptr(math.Min(math.Max(math.Round(*p*k*f)/f, lo), hi))
	}
	s.Size = scale(s.Size, 8, 1000, 0)
	s.StrokeWidth = scale(s.StrokeWidth, 0, 40, 1)
	s.ShadowDX = scale(s.ShadowDX, -math.MaxFloat64, math.MaxFloat64, 1)
	s.ShadowDY = scale(s.ShadowDY, -math.MaxFloat64, math.MaxFloat64, 1)
	s.ShadowBlur = scale(s.ShadowBlur, 0, 100, 1)
	s.BackgroundPadding = scale(s.BackgroundPadding, 0, math.MaxFloat64, 1)
	s.BackgroundRadius = scale(s.BackgroundRadius, 0, math.MaxFloat64, 1)
	if spec.LineWidth > 0 {
		s.MaxWidth = ptr(math.Round(spec.LineWidth * float64(width)))
	}
	// Copy the pointers the table shares so that a caller cannot change the table through the result.
	out, _ := timeline.StyleFromMap(timeline.StyleToMap(&s))
	return out
}

type captionApplyStyleArgs struct {
	TrackID        string `json:"track_id"                  desc:"A visual track."                                                  pattern:"^trk_[0123456789abcdefghjkmnpqrstvwxyz]{10}$"`
	Style          string `json:"style"                     desc:"The style id; caption.list_styles describes them."                                                                                                                      enum:"clean_white,bold_pop,marker_yellow,soft_box,neon_night,editorial,mono_code,karaoke_pill,thick_outline,warm_glow,lower_bar,big_center,top_headline,ink_paper"`
	ResetOverrides bool   `json:"reset_overrides,omitempty" desc:"true clears the style override of the captions of the track."`
}

func applyCaptionApplyStyle(c *Context, a *captionApplyStyleArgs) error {
	if err := idForm("args.track_id", timeline.PrefixTrack, a.TrackID); err != nil {
		return err
	}
	spec, ok := CaptionStyleByID(a.Style)
	if !ok {
		return invalidArg("args.style", "args.style %q is not a caption style.", SafeKey(a.Style)).
			WithHint("Use %s; caption.list_styles describes them.", strings.Join(CaptionStyleIDs(), ", "))
	}
	t, err := c.track(a.TrackID, "args.track_id")
	if err != nil {
		return err
	}
	if t.Kind != timeline.TrackVisual {
		return Errorf(CodeTypeMismatch, "Track %s is an audio track and cannot hold captions.", a.TrackID).
			WithHint("Use a visual track.").WithPath("args.track_id")
	}
	style := ResolveCaptionStyle(spec, c.Doc.Settings.Width, c.Doc.Settings.Height)
	if err := c.checkFont(style); err != nil {
		return err
	}
	t.CaptionStyle = style
	c.Doc.Tracks[a.TrackID] = t

	overridden := []string{}
	for _, id := range slices.Sorted(maps.Keys(c.Doc.Items)) {
		it := c.Doc.Items[id]
		if it.Type != timeline.ItemCaption || it.Track != a.TrackID || it.Style == nil {
			continue
		}
		if a.ResetOverrides {
			it.Style = nil
			c.Doc.Items[id] = it
			continue
		}
		overridden = append(overridden, id)
	}
	c.SetEffective("style", a.Style)
	c.SetEffective("caption_style", timeline.StyleToMap(style))
	c.SetEffective("overridden_items", overridden)
	return nil
}

type captionListStylesArgs struct{}

func applyCaptionListStyles(c *Context, _ *captionListStylesArgs) error {
	styles := make([]map[string]any, 0, len(captionStyles))
	for _, s := range captionStyles {
		styles = append(styles, map[string]any{
			"id": s.ID, "name": s.Name, "summary": s.Summary, "font_family": *s.Style.FontFamily,
			"caption_style": timeline.StyleToMap(ResolveCaptionStyle(s, captionStyleRefSide, captionStyleRefSide)),
		})
	}
	c.SetEffective("styles", styles)
	return nil
}
