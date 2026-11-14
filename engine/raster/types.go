package raster

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
)

// Version is bumped on any change that alters output bytes (SDD-004 section 5).
const Version = 1

// Kinds of BitmapSpec.
const (
	KindText    = "text"
	KindCaption = "caption"
	KindShape   = "shape"
)

// Shapes of ShapeSpec (SDD-001 section 3.6).
const (
	ShapeRect        = "rect"
	ShapeRoundedRect = "rounded_rect"
	ShapeEllipse     = "ellipse"
)

// Alignments of ResolvedStyle.
const (
	AlignLeft   = "left"
	AlignCenter = "center"
	AlignRight  = "right"
)

// Colour is a straight (non-premultiplied) RGBA colour.
type Colour struct {
	R, G, B, A uint8
}

// MarshalJSON writes the colour as "#RRGGBBAA", so the cache key is readable.
func (c Colour) MarshalJSON() ([]byte, error) {
	return json.Marshal(fmt.Sprintf("#%02X%02X%02X%02X", c.R, c.G, c.B, c.A))
}

// Rect is an axis-aligned rectangle in frame pixels.
type Rect struct {
	X, Y, W, H float64
}

// ResolvedStyle is the effective style of a text or caption item (SDD-001
// section 3.7): every field is explicit and every colour is resolved, so the
// rasterizer needs no defaults.
type ResolvedStyle struct {
	FontFamily string  `json:"font_family"`
	FontWeight int     `json:"font_weight"`
	Size       float64 `json:"size"`
	Color      Colour  `json:"color"`

	StrokeColor Colour  `json:"stroke_color"`
	StrokeWidth float64 `json:"stroke_width"`

	ShadowColor Colour  `json:"shadow_color"`
	ShadowDX    float64 `json:"shadow_dx"`
	ShadowDY    float64 `json:"shadow_dy"`
	ShadowBlur  float64 `json:"shadow_blur"`

	// BackgroundColor is nil when there is no box behind the text.
	BackgroundColor   *Colour `json:"background_color"`
	BackgroundPadding float64 `json:"background_padding"`
	BackgroundRadius  float64 `json:"background_radius"`

	Align      string  `json:"align"`
	LineHeight float64 `json:"line_height"`
	// MaxWidth is in pixels: lines wrap at this width.
	MaxWidth  float64 `json:"max_width"`
	Uppercase bool    `json:"uppercase"`

	// HighlightColor and HighlightBackground apply to the active word of a
	// caption. HighlightBackground is nil when there is no box.
	HighlightColor      Colour  `json:"highlight_color"`
	HighlightBackground *Colour `json:"highlight_background"`
}

// ShapeSpec describes a shape item (SDD-001 section 3.6).
type ShapeSpec struct {
	Shape       string  `json:"shape"`
	Width       float64 `json:"width"`
	Height      float64 `json:"height"`
	Radius      float64 `json:"radius"`
	Fill        Colour  `json:"fill"`
	StrokeColor Colour  `json:"stroke_color"`
	StrokeWidth float64 `json:"stroke_width"`
}

// BitmapSpec is everything the rasterizer needs to draw one bitmap.
type BitmapSpec struct {
	Kind string `json:"kind"`
	// Text is the content of a text item. For a caption it is ignored: the
	// text is the Words joined by spaces, with a line break before every word
	// index in Breaks.
	Text   string   `json:"text"`
	Words  []string `json:"words"`
	Breaks []int    `json:"breaks"`
	// ActiveWord is the index of the highlighted word of a caption, or -1.
	ActiveWord int           `json:"active_word"`
	Style      ResolvedStyle `json:"style"`
	Shape      *ShapeSpec    `json:"shape"`
	// PixelScale is the oversampling factor of SDD-004 section 5.3.
	PixelScale float64 `json:"pixel_scale"`
}

// Key is the SHA-256 of the canonical JSON of the spec plus Version, in hex.
func (s BitmapSpec) Key() string {
	b, err := json.Marshal(s)
	if err != nil {
		// Every field is a plain value; NaN and infinities are the only way
		// to get here and are rejected by Render before a key is needed.
		return ""
	}

	h := sha256.New()
	h.Write(b)
	fmt.Fprintf(h, "|v%d", Version)

	return hex.EncodeToString(h.Sum(nil))
}

// Bitmap is a rendered bitmap and its metrics.
type Bitmap struct {
	// Pix is RGBA, 8 bits per channel, straight (non-premultiplied) alpha,
	// Width*Height*4 bytes. It is shared by the cache: do not modify it.
	Pix           []byte
	Width, Height int
	// NaturalW and NaturalH are the size of the bitmap at PixelScale 1, in
	// frame pixels.
	NaturalW, NaturalH float64
	// CenterDX and CenterDY are the offset, at PixelScale 1, from the centre
	// of the bitmap to the centre of the text block (or shape). The item's
	// position names the centre of the block, so a shadow that reaches further
	// on one side moves the bitmap, not the block.
	CenterDX, CenterDY float64
	// WordBoxes is the box of each word of a caption at PixelScale 1, in the
	// coordinates of the bitmap, for the lint and the UI.
	WordBoxes []Rect
}
