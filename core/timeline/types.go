package timeline

// The Go types are the source of truth of the document (SDD-001 §7). Their tags
// drive the decoder, the canonical encoder and, later, the generated JSON Schema:
//
//   - `json:"name"`            the field is required and always written.
//   - `json:"name,omitempty"`  the field is optional; it is omitted when empty
//     (`omitzero` is the same, used on struct values).
//     A nil pointer, an empty map or slice, a zero scalar or an object that
//     encodes to {} is empty.
//   - `default:"v"`            the field has a default: it is filled in on decode
//     and omitted on encode when equal to it. A rational default is "num/den".
//
// A pointer to a scalar is "set or not": the style objects and the x and y of an
// item store exactly the keys that were set (SDD-001 §2 item 8).

// CurrentSchemaVersion is the schema version this binary writes and supports.
const CurrentSchemaVersion = 1

// Limits of the document (V_LIMITS, V_ITEM_RANGE).
const (
	MaxTracks       = 50
	MaxItems        = 500
	MaxAssets       = 200
	MaxCaptionWords = 5000
	MaxFrames       = 108000
)

// PlaceholderAsset is the reserved asset id of an unfilled media slot in a template.
const PlaceholderAsset = "ast_placeholder"

// Identifier prefixes (SDD-000 §2).
const (
	PrefixProject = "prj"
	PrefixAsset   = "ast"
	PrefixTrack   = "trk"
	PrefixItem    = "itm"
)

// Extra holds the fields the binary does not know, kept verbatim at every object
// level (SDD-001 §2 item 9). Values are what encoding/json produces with
// UseNumber: map[string]any, []any, json.Number, string, bool or nil.
type Extra map[string]any

// Color is "#RRGGBB" or "#RRGGBBAA" (stored upper case), or "$name" in a style
// where the name is a colour slot of the template.
type Color string

type (
	// AssetKind is video, audio or image.
	AssetKind string
	// TrackKind is visual or audio.
	TrackKind string
	// ItemType is clip, audio, text, caption or shape.
	ItemType string
)

// Asset kinds, track kinds and item types.
const (
	AssetVideo AssetKind = "video"
	AssetAudio AssetKind = "audio"
	AssetImage AssetKind = "image"

	TrackVisual TrackKind = "visual"
	TrackAudio  TrackKind = "audio"

	ItemClip    ItemType = "clip"
	ItemAudio   ItemType = "audio"
	ItemText    ItemType = "text"
	ItemCaption ItemType = "caption"
	ItemShape   ItemType = "shape"
)

// Rational is {"num", "den"}, both positive.
type Rational struct {
	Num int `json:"num"`
	Den int `json:"den"`
}

// Document is one project (SDD-001 §3).
type Document struct {
	SchemaVersion int                    `json:"schema_version"`
	ID            string                 `json:"id"`
	Name          string                 `json:"name"`
	Settings      Settings               `json:"settings"`
	Assets        map[string]Asset       `json:"assets"`
	Tracks        map[string]Track       `json:"tracks"`
	Items         map[string]Item        `json:"items"`
	Fonts         map[string]ProjectFont `json:"fonts,omitempty"`
	Template      *TemplateInfo          `json:"template,omitempty"`
	Extra         Extra                  `json:"-"`
}

// Settings of the project (SDD-001 §3.1).
type Settings struct {
	Width          int      `json:"width"           default:"1080"`
	Height         int      `json:"height"          default:"1920"`
	FPS            Rational `json:"fps"             default:"30/1"`
	Background     Color    `json:"background"      default:"#000000"`
	LoudnessTarget float64  `json:"loudness_target" default:"-14"`
	Extra          Extra    `json:"-"`
}

// Asset is a reference to a file in the content store plus the probe facts (SDD-001 §3.2).
type Asset struct {
	ID          string    `json:"id"`
	Hash        string    `json:"hash"`
	Kind        AssetKind `json:"kind"`
	Name        string    `json:"name"`
	FrameRate   Rational  `json:"frame_rate,omitzero"`
	FrameCount  int       `json:"frame_count,omitempty"`
	SampleCount int       `json:"sample_count,omitempty"`
	Width       int       `json:"width,omitempty"`
	Height      int       `json:"height,omitempty"`
	HasAudio    bool      `json:"has_audio,omitempty"`
	Extra       Extra     `json:"-"`
}

// Track is a layer or a lane (SDD-001 §3.3).
type Track struct {
	ID           string        `json:"id"`
	Kind         TrackKind     `json:"kind"`
	Name         string        `json:"name,omitempty"`
	Order        string        `json:"order"`
	Hidden       bool          `json:"hidden,omitempty"`
	Muted        bool          `json:"muted,omitempty"`
	CaptionStyle *CaptionStyle `json:"caption_style,omitempty"`
	Extra        Extra         `json:"-"`
}

// Item is any of the five item types (SDD-001 §3.4 to §3.7). One flat struct holds
// the fields of every type; the fields of the other types keep their zero or default
// value and are omitted on encode. Use NewItem to get the defaults.
type Item struct {
	ID            string      `json:"id"`
	Type          ItemType    `json:"type"`
	Track         string      `json:"track"`
	Start         int         `json:"start"`
	Duration      int         `json:"duration"`
	Name          string      `json:"name,omitempty"`
	TransitionIn  *Transition `json:"transition_in,omitempty"`
	TransitionOut *Transition `json:"transition_out,omitempty"`
	Keyframes     Keyframes   `json:"keyframes,omitempty"`
	Slot          *Slot       `json:"slot,omitempty"`

	// Visual items.
	X        *float64 `json:"x,omitempty"`
	Y        *float64 `json:"y,omitempty"`
	Scale    float64  `json:"scale"              default:"1"`
	Rotation float64  `json:"rotation,omitempty"`
	Opacity  float64  `json:"opacity"            default:"1"`

	// Clip and audio.
	Asset      string   `json:"asset,omitempty"`
	SourceIn   int      `json:"source_in,omitempty"`
	Speed      Rational `json:"speed"                 default:"1/1"`
	Volume     float64  `json:"volume"                default:"1"`
	AudioMuted bool     `json:"audio_muted,omitempty"`
	FadeIn     int      `json:"fade_in,omitempty"`
	FadeOut    int      `json:"fade_out,omitempty"`

	// Clip only.
	Fit        string  `json:"fit"                  default:"cover"`
	Crop       *Crop   `json:"crop,omitempty"`
	Brightness float64 `json:"brightness,omitempty"`
	Contrast   float64 `json:"contrast"             default:"1"`
	Saturation float64 `json:"saturation"           default:"1"`

	// Text and caption. Style of a text item uses the TextStyle fields only.
	Text  string        `json:"text,omitempty"`
	Style *CaptionStyle `json:"style,omitempty"`

	// Caption only.
	Words      []Word `json:"words,omitempty"`
	Breaks     []int  `json:"breaks,omitempty"`
	LinkedItem string `json:"linked_item,omitempty"`

	// Shape only.
	Shape       string  `json:"shape,omitempty"`
	Width       float64 `json:"width,omitempty"`
	Height      float64 `json:"height,omitempty"`
	Radius      float64 `json:"radius,omitempty"`
	Fill        Color   `json:"fill"                   default:"#FFFFFF"`
	StrokeColor Color   `json:"stroke_color"           default:"#000000"`
	StrokeWidth float64 `json:"stroke_width,omitempty"`

	Extra Extra `json:"-"`
}

// Crop is the fraction removed from each side of the asset.
type Crop struct {
	Left   float64 `json:"left,omitempty"`
	Top    float64 `json:"top,omitempty"`
	Right  float64 `json:"right,omitempty"`
	Bottom float64 `json:"bottom,omitempty"`
	Extra  Extra   `json:"-"`
}

// Word is one timed word of a caption; Start is relative to the item.
type Word struct {
	Text     string `json:"text"`
	Start    int    `json:"start"`
	Duration int    `json:"duration"`
	Extra    Extra  `json:"-"`
}

// Transition of an item entry or exit (SDD-001 §3.8).
type Transition struct {
	Kind     string `json:"kind"`
	Duration int    `json:"duration"`
	Extra    Extra  `json:"-"`
}

// Keyframes maps a property (x, y, scale, rotation, opacity, volume) to its keyframes.
type Keyframes map[string][]Keyframe

// Keyframe animates a property; T is relative to the item's start.
type Keyframe struct {
	T      int     `json:"t"`
	Value  float64 `json:"value"`
	Easing string  `json:"easing" default:"linear"`
	Extra  Extra   `json:"-"`
}

// TextStyle is the style of a text item; every field is set or not (cascade, SDD-001 §3.7).
type TextStyle struct {
	FontFamily        *string  `json:"font_family,omitempty"`
	FontWeight        *int     `json:"font_weight,omitempty"`
	Size              *float64 `json:"size,omitempty"`
	Color             *Color   `json:"color,omitempty"`
	StrokeColor       *Color   `json:"stroke_color,omitempty"`
	StrokeWidth       *float64 `json:"stroke_width,omitempty"`
	ShadowColor       *Color   `json:"shadow_color,omitempty"`
	ShadowDX          *float64 `json:"shadow_dx,omitempty"`
	ShadowDY          *float64 `json:"shadow_dy,omitempty"`
	ShadowBlur        *float64 `json:"shadow_blur,omitempty"`
	BackgroundColor   *Color   `json:"background_color,omitempty"`
	BackgroundPadding *float64 `json:"background_padding,omitempty"`
	BackgroundRadius  *float64 `json:"background_radius,omitempty"`
	Align             *string  `json:"align,omitempty"`
	LineHeight        *float64 `json:"line_height,omitempty"`
	MaxWidth          *float64 `json:"max_width,omitempty"`
	Uppercase         *bool    `json:"uppercase,omitempty"`
}

// CaptionStyle is TextStyle plus the caption fields.
type CaptionStyle struct {
	TextStyle
	Position            *string `json:"position,omitempty"`
	Highlight           *string `json:"highlight,omitempty"`
	HighlightColor      *Color  `json:"highlight_color,omitempty"`
	HighlightBackground *Color  `json:"highlight_background,omitempty"`
	Extra               Extra   `json:"-"`
}

// ProjectFont is a user font of the project (BETA, SDD-001 §3.9).
type ProjectFont struct {
	Files []FontFile `json:"files"`
	Extra Extra      `json:"-"`
}

// FontFile is one weight of a user font, stored by hash.
type FontFile struct {
	Weight int    `json:"weight"`
	Hash   string `json:"hash"`
	Extra  Extra  `json:"-"`
}

// Slot marks an item as a template slot (SDD-001 §3.10).
type Slot struct {
	Name           string `json:"name"`
	Kind           string `json:"kind"`
	Description    string `json:"description,omitempty"`
	Required       bool   `json:"required"              default:"true"`
	DurationPolicy string `json:"duration_policy"       default:"fit"`
	Filled         bool   `json:"filled,omitempty"`
	Extra          Extra  `json:"-"`
}

// TemplateInfo is present only in a template document.
type TemplateInfo struct {
	Description string           `json:"description"`
	Colors      map[string]Color `json:"colors,omitempty"`
	Extra       Extra            `json:"-"`
}

// BundledFamilies are the font families shipped in the binary (ADR-0012). The
// weights shipped per family are listed in engine/raster/fonts/FONTS.md; until that
// file exists Validate checks the weight only for user fonts.
var BundledFamilies = []string{"Inter", "Montserrat", "Anton", "Bebas Neue", "Playfair Display", "JetBrains Mono"}

// DefaultSettings returns Settings with every default filled in.
func DefaultSettings() Settings { return withDefaults[Settings]() }

// NewItem returns an item of the given type with every default filled in.
func NewItem(id string, typ ItemType, track string, start, duration int) Item {
	it := withDefaults[Item]()
	it.ID, it.Type, it.Track, it.Start, it.Duration = id, typ, track, start, duration
	return it
}

// NewSlot returns a slot with the defaults filled in.
func NewSlot(name, kind string) Slot {
	s := withDefaults[Slot]()
	s.Name, s.Kind = name, kind
	return s
}

// NewKeyframe returns a linear keyframe.
func NewKeyframe(t int, value float64) Keyframe {
	k := withDefaults[Keyframe]()
	k.T, k.Value = t, value
	return k
}

// IsVisual reports whether the item type is drawn (clip, text, caption, shape).
func (t ItemType) IsVisual() bool { return t != ItemAudio }
