package plan

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"math"
)

// BitmapVersion is part of every bitmap key; bump it with any change that
// alters the bytes the rasterizer produces (SDD-004 §5).
const BitmapVersion = 1

// Rect is a rectangle in pixels.
type Rect struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
	W float64 `json:"w"`
	H float64 `json:"h"`
}

// LayerKind says how a layer is drawn.
type LayerKind string

// The two kinds of layer: pixels of a media asset, or a bitmap the rasterizer draws.
const (
	Media  LayerKind = "media"
	Bitmap LayerKind = "bitmap"
)

// FramePlan is what one frame contains (SDD-004 §2).
type FramePlan struct {
	Frame      int     `json:"frame"`
	Width      int     `json:"width"`
	Height     int     `json:"height"`
	Background string  `json:"background"`
	Layers     []Layer `json:"layers"` // bottom first
}

// Layer is one drawn thing. Placement is valid for both kinds.
type Layer struct {
	ItemID string    `json:"item"`
	Kind   LayerKind `json:"kind"`

	// Media layers (clip items).
	AssetID     string       `json:"asset,omitempty"`
	AssetKind   string       `json:"asset_kind,omitempty"` // "video" or "image"
	SourceFrame int          `json:"source_frame,omitempty"`
	Crop        Rect         `json:"crop,omitzero"`          // in source pixels after the item's crop fractions
	ColorMatrix *[12]float64 `json:"color_matrix,omitempty"` // nil when brightness, contrast and saturation are neutral

	// Bitmap layers (text, caption, shape items).
	Bitmap *BitmapSpec `json:"bitmap,omitempty"`

	// Both kinds (SDD-004 §3.6): the filter matrix, applied after ColorMatrix and
	// before the blur, and the blur radius in frame pixels. Nil and 0 mean none;
	// filters and blur are drawn in Go by the compositor, never by FFmpeg.
	Filter *[12]float64 `json:"filter,omitempty"`
	Blur   float64      `json:"blur,omitempty"`

	CX       float64 `json:"cx"`
	CY       float64 `json:"cy"`
	W        float64 `json:"w"` // drawn size before rotation
	H        float64 `json:"h"`
	Rotation float64 `json:"rotation"` // degrees clockwise
	Opacity  float64 `json:"opacity"`
}

// BitmapSpec says what the rasterizer draws (SDD-004 §5). It lives here so
// that the plan, the rasterizer and the browser name the same thing; its Key
// identifies a bitmap in the cache.
type BitmapSpec struct {
	Kind       string        `json:"kind"` // "text", "caption" or "shape"
	Text       string        `json:"text,omitempty"`
	Words      []string      `json:"words,omitempty"` // captions only
	Breaks     []int         `json:"breaks,omitempty"`
	ActiveWord int           `json:"active_word"` // -1 for none
	Style      ResolvedStyle `json:"style"`
	Shape      *ShapeSpec    `json:"shape,omitempty"`
	PixelScale float64       `json:"pixel_scale"`
}

// ResolvedStyle has every field explicit and every colour resolved (the
// cascade of SDD-001 §3.7 and the template colour slots applied). An empty
// BackgroundColor means no box. The highlight fields are for captions.
type ResolvedStyle struct {
	FontFamily          string  `json:"font_family"`
	FontWeight          int     `json:"font_weight"`
	Size                float64 `json:"size"`
	Color               string  `json:"color"`
	StrokeColor         string  `json:"stroke_color"`
	StrokeWidth         float64 `json:"stroke_width"`
	ShadowColor         string  `json:"shadow_color"`
	ShadowDX            float64 `json:"shadow_dx"`
	ShadowDY            float64 `json:"shadow_dy"`
	ShadowBlur          float64 `json:"shadow_blur"`
	BackgroundColor     string  `json:"background_color,omitempty"`
	BackgroundPadding   float64 `json:"background_padding"`
	BackgroundRadius    float64 `json:"background_radius"`
	Align               string  `json:"align"`
	LineHeight          float64 `json:"line_height"`
	MaxWidth            float64 `json:"max_width"`
	Uppercase           bool    `json:"uppercase"`
	Highlight           string  `json:"highlight,omitempty"`
	HighlightColor      string  `json:"highlight_color,omitempty"`
	HighlightBackground string  `json:"highlight_background,omitempty"`
}

// ShapeSpec is a shape item before scale.
type ShapeSpec struct {
	Shape       string  `json:"shape"`
	Width       float64 `json:"width"`
	Height      float64 `json:"height"`
	Radius      float64 `json:"radius"`
	Fill        string  `json:"fill"`
	StrokeColor string  `json:"stroke_color"`
	StrokeWidth float64 `json:"stroke_width"`
}

// Key is the SHA-256 of the canonical JSON of the spec plus BitmapVersion, as
// lower-case hex. Struct fields have a fixed order, so the JSON is canonical.
func (s BitmapSpec) Key() string {
	data, err := json.Marshal(struct {
		Version int        `json:"version"`
		Spec    BitmapSpec `json:"spec"`
	}{BitmapVersion, s})
	if err != nil {
		// Unreachable: a BitmapSpec holds only strings, numbers and slices of
		// them. An empty key never matches a cached bitmap.
		return ""
	}

	sum := sha256.Sum256(data)

	return hex.EncodeToString(sum[:])
}

// Measurer returns the natural size, in frame pixels at PixelScale 1, that the
// rasterizer gives a bitmap (SDD-004 §4.2). The plan is pure and cannot lay
// text out, so the caller that owns the rasterizer supplies it. With a nil
// Measurer a bitmap layer has size 0, and a caption at the top or bottom is
// anchored by its centre on the edge.
type Measurer func(BitmapSpec) (w, h float64)

// Options configure FrameWith and WindowWith.
type Options struct {
	Measure Measurer
	// OutputScale is width / project width of the frame asked for; 0 means 1 (export).
	// It enters the oversampling of bitmap layers (SDD-004 §5.3).
	OutputScale float64
}

// round3 rounds to 3 decimal places and never yields -0 (SDD-004 §2, stability).
func round3(x float64) float64 {
	return math.Round(x*1000)/1000 + 0
}
