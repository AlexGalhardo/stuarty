package plan

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"math"

	"github.com/AlexGalhardo/stuarty/core/timeline"
)

// BitmapVersion is part of every bitmap key; bump it with any change that
// alters the bytes the rasterizer produces (SDD-004 §5).
const BitmapVersion = 2

// Rect is a rectangle in pixels.
type Rect struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
	W float64 `json:"w"`
	H float64 `json:"h"`
}

// LayerKind says how a layer is drawn.
type LayerKind string

// The kinds of layer: pixels of a media asset, a bitmap the rasterizer draws, or a group, the layers of a
// nested timeline that an executor draws on a surface of their own before it places that surface as one layer
// (SDD-004 §3.16, SDD-013 §3.12).
const (
	Media  LayerKind = "media"
	Bitmap LayerKind = "bitmap"
	Group  LayerKind = "group"
)

// GroupSpec is the nested frame of a group layer: its own size in pixels and its layers, bottom first, placed
// in that frame. The surface the executor makes is transparent where the layers draw nothing.
type GroupSpec struct {
	Width  int     `json:"width"`
	Height int     `json:"height"`
	Layers []Layer `json:"layers"`
}

// FramePlan is what one frame contains (SDD-004 §2).
type FramePlan struct {
	Frame      int    `json:"frame"`
	Width      int    `json:"width"`
	Height     int    `json:"height"`
	Background string `json:"background"`
	// Transparent is true when the project has no background colour (settings.background is "transparent");
	// Background is then #000000, so an executor that ignores the flag composites over black.
	Transparent bool    `json:"transparent,omitempty"`
	Layers      []Layer `json:"layers"` // bottom first
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
	ColorMatrix *[12]float64 `json:"color_matrix,omitempty"` // nil when every colour adjustment is neutral

	// Bitmap layers (text, caption, shape items).
	Bitmap *BitmapSpec `json:"bitmap,omitempty"`

	// Group layers (composition items that draw as one unit): the nested frame. Crop is the whole nested frame.
	Group *GroupSpec `json:"group,omitempty"`

	// Both kinds (SDD-004 §3.6): the filter matrix, applied after ColorMatrix and
	// before the blur, and the blur radius in frame pixels. Nil and 0 mean none;
	// filters and blur are drawn in Go by the compositor, never by FFmpeg.
	Filter *[12]float64 `json:"filter,omitempty"`
	Blur   float64      `json:"blur,omitempty"`

	// ChromaKey is the colour the compositor makes transparent (SDD-004 §3.7); nil means none.
	ChromaKey *ChromaKeySpec `json:"chroma_key,omitempty"`

	// Grade is the advanced colour of a media layer, drawn after ColorMatrix (SDD-004 §3.14); nil means none.
	Grade *GradeSpec `json:"grade,omitempty"`

	// Lut is the colour lookup table of a media layer, drawn after Grade and before Filter (SDD-004 §3.6); nil means none.
	Lut *LutSpec `json:"lut,omitempty"`

	// Blend is the blend mode of SDD-004 §3.8: multiply, screen, overlay or add; empty means normal.
	Blend string `json:"blend,omitempty"`

	// Mask limits where the layer is drawn (SDD-004 §3.9); nil means no mask.
	Mask *MaskSpec `json:"mask,omitempty"`

	// Tone is the 256-entry table of the exposure, levels and curve of an image layer, applied to every colour
	// channel before the colour matrix (SDD-013 §3.11.1); nil means none.
	Tone *[256]uint8 `json:"tone,omitempty"`

	// Filters are the ordered pixel filters of an image layer, drawn after the blur (SDD-013 §3.11.3).
	Filters []FilterSpec `json:"filters,omitempty"`

	// Clip limits the layer to the alpha of another layer of the plan; Stencil marks a layer that only gives
	// that alpha and is not drawn (SDD-013 §3.11.4).
	Clip    *ClipSpec `json:"clip,omitempty"`
	Stencil bool      `json:"stencil,omitempty"`

	// Shadow and Outline are the drop shadow and the contour of a media layer (SDD-004 §3.15); nil means none.
	Shadow  *ShadowSpec  `json:"shadow,omitempty"`
	Outline *OutlineSpec `json:"outline,omitempty"`

	// Effects change the layer's pixels, in order, after the filter and the blur (SDD-004 §3.13); shake is
	// not here, it has already moved the layer. Transition is the whip, dip or wipe in progress (§3.2).
	Effects    []timeline.Effect `json:"effects,omitempty"`
	Transition *TransitionSpec   `json:"transition,omitempty"`

	// FlipX and FlipY mirror the drawn source across the layer's own vertical or horizontal axis
	// (SDD-004 §3.10); the quadrilateral and its rotation are the same.
	FlipX bool `json:"flip_x,omitempty"`
	FlipY bool `json:"flip_y,omitempty"`

	CX       float64 `json:"cx"`
	CY       float64 `json:"cy"`
	W        float64 `json:"w"` // drawn size before rotation
	H        float64 `json:"h"`
	Rotation float64 `json:"rotation"` // degrees clockwise
	Scale    float64 `json:"scale"`    // effective scale: keyframes and static value, before W and H are derived
	Opacity  float64 `json:"opacity"`
}

// ShadowSpec is the drop shadow of a layer with the colour resolved to #RRGGBB or #RRGGBBAA; the lengths
// are project pixels.
type ShadowSpec struct {
	Color   string  `json:"color"`
	Opacity float64 `json:"opacity"`
	DX      float64 `json:"dx"`
	DY      float64 `json:"dy"`
	Blur    float64 `json:"blur"`
}

// OutlineSpec is the contour of a layer, with the colour resolved; Width is in project pixels.
type OutlineSpec struct {
	Color string  `json:"color"`
	Width float64 `json:"width"`
}

// LutSpec names the lookup table asset of a layer and how much of it is mixed in (0 to 1, never 0 in a plan).
type LutSpec struct {
	Asset     string  `json:"asset"`
	Intensity float64 `json:"intensity"`
}

// GradeSpec is the advanced colour of a layer: the bands, wheels and curves of the item, as stored.
type GradeSpec struct {
	Hsl    []timeline.HslBand `json:"hsl,omitempty"`
	Wheels *timeline.Wheels   `json:"wheels,omitempty"`
	Curves *timeline.Curves   `json:"curves,omitempty"`
}

// ChromaKeySpec is the key of a layer with the colour already resolved to #RRGGBB or #RRGGBBAA.
type ChromaKeySpec struct {
	Color      string  `json:"color"`
	Similarity float64 `json:"similarity"`
	Smoothness float64 `json:"smoothness"`
	Spill      float64 `json:"spill"`
}

// MaskSpec is the mask of a layer: a rectangle or an ellipse in the layer's own frame, as fractions
// of its drawn size (SDD-004 §3.9). Feather is in project pixels.
type MaskSpec struct {
	Shape   string  `json:"shape"`
	X       float64 `json:"x"`
	Y       float64 `json:"y"`
	Width   float64 `json:"width"`
	Height  float64 `json:"height"`
	Feather int     `json:"feather,omitempty"`
	Invert  bool    `json:"invert,omitempty"`
	Radius  int     `json:"radius,omitempty"` // the corner of shape rounded_rect, in project pixels
}

// ClipSpec is the mask of a layer from the alpha of another layer, named by its item id (SDD-013 §3.11.4).
type ClipSpec struct {
	Source string `json:"source"`
	Invert bool   `json:"invert,omitempty"`
}

// FilterSpec is one pixel filter of an image layer (SDD-013 §3.11.3): the kind and the members it uses, the
// others absent. Kinds: matrix, blur, sharpen, vignette, grain, shadow, stroke and outer_glow.
type FilterSpec struct {
	Kind     string       `json:"kind"`
	Matrix   *[12]float64 `json:"matrix,omitempty"`
	Sigma    float64      `json:"sigma,omitempty"`
	Amount   float64      `json:"amount,omitempty"`
	Radius   float64      `json:"radius,omitempty"`
	Softness float64      `json:"softness,omitempty"`
	Size     int          `json:"size,omitempty"`
	Seed     int          `json:"seed,omitempty"`
	Color    string       `json:"color,omitempty"`
	Opacity  float64      `json:"opacity,omitempty"`
	DX       float64      `json:"dx,omitempty"`
	DY       float64      `json:"dy,omitempty"`
	Blur     float64      `json:"blur,omitempty"`
	Width    float64      `json:"width,omitempty"`
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

// Nested gives the document of a timeline of the same project by its id, for the composition items of a
// document (SDD-004 §3.16). The plan is pure and reads no store, so the caller that has the project answers it;
// a nil Nested, or an id it does not know, makes the composition draw and sound nothing.
type Nested func(id string) (*timeline.Document, bool)

// Options configure FrameWith and WindowWith.
type Options struct {
	Measure Measurer
	// Nested resolves the timelines that composition items use; see Nested.
	Nested Nested
	// OutputScale is width / project width of the frame asked for; 0 means 1 (export).
	// It enters the oversampling of bitmap layers (SDD-004 §5.3).
	OutputScale float64
}

// round3 rounds to 3 decimal places and never yields -0 (SDD-004 §2, stability).
func round3(x float64) float64 {
	return math.Round(x*1000)/1000 + 0
}
