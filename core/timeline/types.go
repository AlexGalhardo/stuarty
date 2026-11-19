package timeline

import "github.com/AlexGalhardo/stuarty/core/timeline/migrate"

// The Go types are the source of truth of the document (SDD-001 §7). Their tags
// drive the decoder, the canonical encoder and, later, the generated JSON Schema:
//
//   - `json:"name"`            the field is required and always written.
//   - `json:"name,omitempty"`  the field is optional; it is omitted when empty
//     (`omitzero` is the same, used on struct values).
//     A nil pointer, an empty map or slice, a zero scalar or an object that
//     encodes to {} is empty.
//   - `enum`, `min`, `max`, `gt`, `step`, `pattern`, `minlen`, `maxlen`, `minitems`,
//     `maxitems`, `keypattern`, `maxprops`, `desc`: constraints that only the
//     generated JSON Schema reads (schema.go).
//   - `default:"v"`            the field has a default: it is filled in on decode
//     and omitted on encode when equal to it. A rational default is "num/den".
//
// A pointer to a scalar is "set or not": the style objects and the x and y of an
// item store exactly the keys that were set (SDD-001 §2 item 8).

// CurrentSchemaVersion is the schema version this binary writes and supports.
const CurrentSchemaVersion = migrate.Current

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
	PrefixLink    = "lnk"
)

// Extra holds the fields the binary does not know, kept verbatim at every object
// level (SDD-001 §2 item 9). Values are what encoding/json produces with
// UseNumber: map[string]any, []any, json.Number, string, bool or nil.
type Extra map[string]any

// Color is "#RRGGBB" or "#RRGGBBAA" (stored upper case), or "$name" in a style
// where the name is a colour slot of the template.
type Color string

type (
	// AssetKind is video, audio, image or lut.
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
	// AssetLUT is a colour lookup table (a .cube file, SDD-001 §3.2); a clip refers to it with Lut, it is never placed.
	AssetLUT AssetKind = "lut"

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
	Num int `json:"num" min:"1"`
	Den int `json:"den" min:"1"`
}

// Document is one project (SDD-001 §3).
type Document struct {
	SchemaVersion int                    `json:"schema_version"     enum:"1" desc:"Version of this schema."`
	ID            string                 `json:"id"                                                         pattern:"^prj_[0-9a-hjkmnp-tv-z]{10}$"`
	Name          string                 `json:"name"                                                                                              minlen:"1" maxlen:"120"`
	Settings      Settings               `json:"settings"`
	Assets        map[string]Asset       `json:"assets"                                                                                                                    keypattern:"^ast_[0-9a-hjkmnp-tv-z]{10}$" maxprops:"200"`
	Tracks        map[string]Track       `json:"tracks"                                                                                                                    keypattern:"^trk_[0-9a-hjkmnp-tv-z]{10}$" maxprops:"50"`
	Items         map[string]Item        `json:"items"                                                                                                                     keypattern:"^itm_[0-9a-hjkmnp-tv-z]{10}$" maxprops:"500"`
	Fonts         map[string]ProjectFont `json:"fonts,omitempty"`
	Template      *TemplateInfo          `json:"template,omitempty"`
	Markers       []Marker               `json:"markers,omitempty"                                                                                                                                                                  maxitems:"200"`
	Extra         Extra                  `json:"-"`
}

// BackgroundTransparent is the value of settings.background that leaves the frame without a background
// colour: formats that carry alpha keep it, the others composite over black (SDD-001 §3.1).
const BackgroundTransparent = "transparent"

// Settings of the project (SDD-001 §3.1).
type Settings struct {
	Width          int      `json:"width"           default:"1080"    min:"16"  max:"4096" step:"2"`
	Height         int      `json:"height"          default:"1920"    min:"16"  max:"4096" step:"2"`
	FPS            Rational `json:"fps"             default:"30/1"`
	Background     Color    `json:"background"      default:"#000000"`
	LoudnessTarget float64  `json:"loudness_target" default:"-14"     min:"-30" max:"-5"`
	Extra          Extra    `json:"-"`
}

// Asset is a reference to a file in the content store plus the probe facts (SDD-001 §3.2).
type Asset struct {
	ID          string    `json:"id"                     pattern:"^ast_[0-9a-hjkmnp-tv-z]{10}$"`
	Hash        string    `json:"hash"                   pattern:"^sha256:[0-9a-f]{64}$"`
	Kind        AssetKind `json:"kind"                                                          enum:"video,audio,image,lut"`
	Name        string    `json:"name"`
	FrameRate   Rational  `json:"frame_rate,omitzero"`
	FrameCount  int       `json:"frame_count,omitempty"                                                                      min:"1"`
	SampleCount int       `json:"sample_count,omitempty"                                                                     min:"1"`
	Width       int       `json:"width,omitempty"                                                                            min:"1"`
	Height      int       `json:"height,omitempty"                                                                           min:"1"`
	HasAudio    bool      `json:"has_audio,omitempty"`
	// HasAlpha says the video has an alpha channel (a lower third, a logo, an effect): the export
	// composes it over the layers below instead of drawing it opaque (SDD-011 4.3.5).
	HasAlpha bool `json:"has_alpha,omitempty"`
	// FrameDelays makes an image asset animated (BETA, SDD-001 §3.2): the display
	// time of each frame in milliseconds, one per FrameCount.
	FrameDelays []int `json:"frame_delays,omitempty" minitems:"2" maxitems:"1000"`
	Extra       Extra `json:"-"`
}

// Track is a layer or a lane (SDD-001 §3.3).
type Track struct {
	ID           string        `json:"id"                      pattern:"^trk_[0-9a-hjkmnp-tv-z]{10}$"`
	Kind         TrackKind     `json:"kind"                                                           enum:"visual,audio"`
	Name         string        `json:"name,omitempty"                                                                     maxlen:"60"`
	Order        string        `json:"order"                   pattern:"^[0-9a-z]+$"`
	Hidden       bool          `json:"hidden,omitempty"`
	Muted        bool          `json:"muted,omitempty"`
	Locked       bool          `json:"locked,omitempty"`
	Solo         bool          `json:"solo,omitempty"`
	Magnet       bool          `json:"magnet,omitempty"`
	Volume       *float64      `json:"volume,omitempty"                                                                               min:"0" max:"4"`
	CaptionStyle *CaptionStyle `json:"caption_style,omitempty"`
	Extra        Extra         `json:"-"`
}

// Item is any of the five item types (SDD-001 §3.4 to §3.7). One flat struct holds
// the fields of every type; the fields of the other types keep their zero or default
// value and are omitted on encode. Use NewItem to get the defaults.
type Item struct {
	ID            string      `json:"id"                       pattern:"^itm_[0-9a-hjkmnp-tv-z]{10}$"`
	Type          ItemType    `json:"type"                                                            enum:"clip,audio,text,caption,shape"`
	Track         string      `json:"track"                    pattern:"^trk_[0-9a-hjkmnp-tv-z]{10}$"`
	Start         int         `json:"start"                                                                                                min:"0"`
	Duration      int         `json:"duration"                                                                                             min:"1"`
	Name          string      `json:"name,omitempty"                                                                                               maxlen:"120"`
	TransitionIn  *Transition `json:"transition_in,omitempty"`
	TransitionOut *Transition `json:"transition_out,omitempty"`
	Keyframes     Keyframes   `json:"keyframes,omitempty"`
	Slot          *Slot       `json:"slot,omitempty"`
	Markers       []Marker    `json:"markers,omitempty"                                                                                                         maxitems:"200"`

	// Visual items.
	X        *float64 `json:"x,omitempty"`
	Y        *float64 `json:"y,omitempty"`
	Scale    float64  `json:"scale"              default:"1" gt:"0" max:"20"`
	Rotation float64  `json:"rotation,omitempty"`
	Opacity  float64  `json:"opacity"            default:"1"        max:"1"  min:"0"`

	// Visual items, BETA (SDD-001 §3.4): blur radius and one fixed filter.
	Blur         float64 `json:"blur,omitempty" min:"0" max:"100"`
	Filter       string  `json:"filter"                           default:"none" enum:"none,grayscale,sepia,invert,warm,cool"`
	FilterAmount float64 `json:"filter_amount"  min:"0" max:"1"   default:"1"`

	// Visual items, optional and additive (SDD-001 §3.4): the colour the compositor keys out.
	ChromaKey *ChromaKey `json:"chroma_key,omitempty"`

	// Clip only, optional and additive (SDD-001 §3.5): a colour lookup table applied after the colour adjustments.
	Lut *Lut `json:"lut,omitempty"`

	// Visual items, optional and additive (SDD-001 §3.4): how the item is mixed with what is below it.
	// Absent means "normal".
	Blend string `json:"blend,omitempty" enum:"normal,multiply,screen,overlay,add"`

	// Every item type, optional and additive (SDD-001 §3.4): the item stays on the timeline and
	// the plan, the preview and the export ignore it. Absent means enabled.
	Disabled bool `json:"disabled,omitempty"`

	// Visual items, optional and additive (SDD-001 §3.4): mirror the item across its own vertical
	// (FlipX) or horizontal (FlipY) axis. Absent means not mirrored.
	FlipX bool `json:"flip_x,omitempty"`
	FlipY bool `json:"flip_y,omitempty"`

	// Visual items, optional and additive (SDD-001 §3.4): the shape that limits where the item is drawn.
	Mask *Mask `json:"mask,omitempty"`

	// Clip only, optional and additive (SDD-001 §3.5, VV-122): a drop shadow and an outline drawn around
	// the visible (alpha) shape of the clip. Absent means none.
	Shadow  *Shadow  `json:"shadow,omitempty"`
	Outline *Outline `json:"outline,omitempty"`

	// Clip and audio.
	Asset      string   `json:"asset,omitempty"       pattern:"^ast_([0-9a-hjkmnp-tv-z]{10}|placeholder)$"`
	SourceIn   int      `json:"source_in,omitempty"                                                        min:"0"`
	Speed      Rational `json:"speed"                                                                              default:"1/1"`
	Volume     float64  `json:"volume"                                                                     min:"0" default:"1"   max:"4"`
	AudioMuted bool     `json:"audio_muted,omitempty"`
	// Clip only, optional and additive (SDD-001 §3.5): a freeze frame. The item shows the one source frame
	// SourceIn for its whole duration, with no sound; Speed is 1/1 and there is no curve or reverse.
	Freeze bool `json:"freeze,omitempty"`
	// Clip and audio, optional and additive (SDD-001 §3.5): the speed curve, 2 to 8 points with linear
	// ramps between them; speed is then the mean of the curve.
	SpeedCurve []SpeedPoint `json:"speed_curve,omitempty" minitems:"2" maxitems:"8"`
	// Visual items, optional and additive (SDD-001 §3.4): the effects, each kind once, applied in order.
	Effects []Effect `json:"effects,omitempty" minitems:"1" maxitems:"6"`

	// Clip, audio, text and shape, optional and additive (SDD-001 §3.5): the link group. Items with the
	// same link move, trim and split together.
	Link string `json:"link,omitempty" pattern:"^lnk_[0-9a-hjkmnp-tv-z]{10}$"`

	// Clip and audio, optional and additive (SDD-001 §3.5): play the source range backwards.
	Reverse bool `json:"reverse,omitempty"`
	// Clip and audio, optional and additive (SDD-001 §3.5): noise reduction strength, 0 (off) to 1.
	Denoise float64 `json:"denoise,omitempty" min:"0" max:"1"`
	// Clip and audio, optional and additive (SDD-001 §3.5): the voice preset of the item's audio.
	Voice   string `json:"voice,omitempty"    enum:"clear,warm,deep,bright,radio"`
	FadeIn  int    `json:"fade_in,omitempty"                                      min:"0"`
	FadeOut int    `json:"fade_out,omitempty"                                     min:"0"`

	// Clip only.
	Fit        string  `json:"fit"                  default:"cover" enum:"cover,contain,none"`
	Crop       *Crop   `json:"crop,omitempty"`
	Brightness float64 `json:"brightness,omitempty"                                           min:"-1" max:"1"`
	Contrast   float64 `json:"contrast"             default:"1"                               min:"0"  max:"2"`
	Saturation float64 `json:"saturation"           default:"1"                               min:"0"  max:"2"`
	// Clip only, optional and additive (SDD-001 §3.5): white balance and hue rotation.
	Temperature float64 `json:"temperature,omitempty" min:"-1"   max:"1"`
	Tint        float64 `json:"tint,omitempty"        min:"-1"   max:"1"`
	Hue         float64 `json:"hue,omitempty"         min:"-180" max:"180"`
	// Clip only, optional and additive (SDD-001 §3.5): advanced colour (SDD-004 §3.14), the bands of the
	// colour ranges, the wheels of the tonal ranges and the tone curves.
	Hsl    []HslBand `json:"hsl,omitempty"    minitems:"1" maxitems:"8"`
	Wheels *Wheels   `json:"wheels,omitempty"`
	Curves *Curves   `json:"curves,omitempty"`

	// Text and caption. Style of a text item uses the TextStyle fields only.
	Text  string        `json:"text,omitempty"  minlen:"1" maxlen:"2000"`
	Style *CaptionStyle `json:"style,omitempty"`

	// Caption only.
	Words      []Word `json:"words,omitempty"       minitems:"1" maxitems:"60"`
	Breaks     []int  `json:"breaks,omitempty"`
	LinkedItem string `json:"linked_item,omitempty"                            pattern:"^itm_[0-9a-hjkmnp-tv-z]{10}$"`

	// Shape only.
	Shape       string  `json:"shape,omitempty"        enum:"rect,rounded_rect,ellipse"`
	Width       float64 `json:"width,omitempty"                                         gt:"0"`
	Height      float64 `json:"height,omitempty"                                        gt:"0"`
	Radius      float64 `json:"radius,omitempty"                                               min:"0"`
	Fill        Color   `json:"fill"                                                                   default:"#FFFFFF"`
	StrokeColor Color   `json:"stroke_color"                                                           default:"#000000"`
	StrokeWidth float64 `json:"stroke_width,omitempty"                                         min:"0"`

	Extra Extra `json:"-"`
}

// Crop is the fraction removed from each side of the asset.
type Crop struct {
	Left   float64 `json:"left,omitempty"   min:"0" max:"1"`
	Top    float64 `json:"top,omitempty"    min:"0" max:"1"`
	Right  float64 `json:"right,omitempty"  min:"0" max:"1"`
	Bottom float64 `json:"bottom,omitempty" min:"0" max:"1"`
	Extra  Extra   `json:"-"`
}

// ChromaKey makes the pixels near Color transparent. Similarity is how far from Color a
// pixel is still removed, Smoothness the soft edge beyond it, Spill how much of Color is
// taken out of the pixels that stay; all three are 0 to 1.
type ChromaKey struct {
	Color      Color   `json:"color"`
	Similarity float64 `json:"similarity" min:"0" max:"1"`
	Smoothness float64 `json:"smoothness" min:"0" max:"1"`
	Spill      float64 `json:"spill"      min:"0" max:"1"`
	Extra      Extra   `json:"-"`
}

// Lut points a clip at a lookup table asset (a .cube file) and says how much of the grade is mixed in:
// 0 is the original picture, 1 the full table. The core stores and checks the reference; reading the file
// and applying it is the engine's work.
type Lut struct {
	Asset     string  `json:"asset"     pattern:"^ast_[0-9a-hjkmnp-tv-z]{10}$"`
	Intensity float64 `json:"intensity"                                        min:"0" max:"1"`
	Extra     Extra   `json:"-"`
}

// Mask limits the drawn pixels of an item to a rectangle or an ellipse in the item's own frame
// (SDD-004 §3.9). X, Y, Width and Height are fractions of the drawn item (0,0 is its top left
// corner), Feather is the width in pixels of the soft edge and Invert keeps the outside.
type Mask struct {
	Shape   string  `json:"shape"            enum:"rect,ellipse"`
	X       float64 `json:"x"                                    min:"0"     max:"1"`
	Y       float64 `json:"y"                                    min:"0"     max:"1"`
	Width   float64 `json:"width"                                min:"0.001" max:"1"`
	Height  float64 `json:"height"                               min:"0.001" max:"1"`
	Feather int     `json:"feather"                              min:"0"     max:"200"`
	Invert  bool    `json:"invert,omitempty"`
	Extra   Extra   `json:"-"`
}

// Shadow is a drop shadow of a clip (SDD-004 §3.15): the silhouette of the picture in Color, moved by
// DX and DY pixels and blurred by Blur pixels, drawn below it at Opacity. Lengths are project pixels.
type Shadow struct {
	Color   Color   `json:"color"`
	Opacity float64 `json:"opacity" min:"0"    max:"1"`
	DX      float64 `json:"dx"      min:"-500" max:"500"`
	DY      float64 `json:"dy"      min:"-500" max:"500"`
	Blur    float64 `json:"blur"    min:"0"    max:"100"`
	Extra   Extra   `json:"-"`
}

// Outline is a contour of a clip (SDD-004 §3.15): Width project pixels of Color around the silhouette.
type Outline struct {
	Color Color   `json:"color"`
	Width float64 `json:"width" min:"0.5" max:"100"`
	Extra Extra   `json:"-"`
}

// Word is one timed word of a caption; Start is relative to the item.
type Word struct {
	Text     string `json:"text"     minlen:"1" maxlen:"80"`
	Start    int    `json:"start"                           min:"0"`
	Duration int    `json:"duration"                        min:"1"`
	Extra    Extra  `json:"-"`
}

// Transition of an item entry or exit (SDD-001 §3.8).
type Transition struct {
	Kind     string `json:"kind"     enum:"fade,slide_left,slide_right,slide_up,slide_down,zoom,whip,dip,wipe,spin,shrink,grow"`
	Duration int    `json:"duration"                                                                                            min:"1"`
	// Added after BETA (TASK-0278, SDD-004 §3.2): the colour of a dip, #RRGGBB or #RRGGBBAA, default
	// black when absent, and the direction of a whip or a wipe, default left when absent.
	Color     Color  `json:"color,omitempty"`
	Direction string `json:"direction,omitempty" enum:"left,right,up,down"`
	Extra     Extra  `json:"-"`
}

// Effect kinds (SDD-004 §3.13).
const (
	EffectMotionBlur = "motion_blur"
	EffectGlow       = "glow"
	EffectVignette   = "vignette"
	EffectGrain      = "grain"
	EffectRGBSplit   = "rgb_split"
	EffectShake      = "shake"
)

// MaxEffects is the number of effects an item can carry: each kind at most once.
const MaxEffects = 6

// Effect is one visual effect of an item (SDD-004 §3.13). Only the parameters of its kind are
// present, all of them: the commands fill the defaults in, so a stored effect says everything.
type Effect struct {
	Kind      string   `json:"kind"                enum:"motion_blur,glow,vignette,grain,rgb_split,shake"`
	Amount    *float64 `json:"amount,omitempty"                                                           min:"0"    max:"1"`
	Angle     *float64 `json:"angle,omitempty"                                                            min:"-180" max:"180"`
	Radius    *float64 `json:"radius,omitempty"                                                           min:"1"    max:"100"`
	Threshold *float64 `json:"threshold,omitempty"                                                        min:"0"    max:"1"`
	Softness  *float64 `json:"softness,omitempty"                                                         min:"0"    max:"1"`
	Size      *int     `json:"size,omitempty"                                                             min:"1"    max:"8"`
	Speed     *int     `json:"speed,omitempty"                                                            min:"1"    max:"30"`
	Seed      *int     `json:"seed,omitempty"                                                             min:"0"    max:"65535"`
	Extra     Extra    `json:"-"`
}

// EffectParams is, per kind, the parameters it takes in the order of SDD-004 §3.13.
var EffectParams = map[string][]string{
	EffectMotionBlur: {"amount", "angle"},
	EffectGlow:       {"amount", "radius", "threshold"},
	EffectVignette:   {"amount", "softness"},
	EffectGrain:      {"amount", "size", "seed"},
	EffectRGBSplit:   {"amount", "angle"},
	EffectShake:      {"amount", "speed", "seed"},
}

// Keyframes maps a property (x, y, scale, rotation, opacity, volume) to its keyframes.
type Keyframes map[string][]Keyframe

// Keyframe animates a property; T is relative to the item's start.
type Keyframe struct {
	T      int     `json:"t"      min:"0"`
	Value  float64 `json:"value"`
	Easing string  `json:"easing"         default:"linear" enum:"linear,ease_in,ease_out,ease_in_out,hold"`
	Extra  Extra   `json:"-"`
}

// TextStyle is the style of a text item; every field is set or not (cascade, SDD-001 §3.7).
type TextStyle struct {
	FontFamily        *string  `json:"font_family,omitempty"`
	FontWeight        *int     `json:"font_weight,omitempty"        min:"100" max:"900"  step:"100"`
	Size              *float64 `json:"size,omitempty"               min:"8"   max:"1000"`
	Color             *Color   `json:"color,omitempty"`
	StrokeColor       *Color   `json:"stroke_color,omitempty"`
	StrokeWidth       *float64 `json:"stroke_width,omitempty"       min:"0"   max:"40"`
	ShadowColor       *Color   `json:"shadow_color,omitempty"`
	ShadowDX          *float64 `json:"shadow_dx,omitempty"`
	ShadowDY          *float64 `json:"shadow_dy,omitempty"`
	ShadowBlur        *float64 `json:"shadow_blur,omitempty"        min:"0"   max:"100"`
	BackgroundColor   *Color   `json:"background_color,omitempty"`
	BackgroundPadding *float64 `json:"background_padding,omitempty" min:"0"`
	BackgroundRadius  *float64 `json:"background_radius,omitempty"  min:"0"`
	Align             *string  `json:"align,omitempty"                                              enum:"left,center,right"`
	LineHeight        *float64 `json:"line_height,omitempty"        min:"0.8" max:"3"`
	MaxWidth          *float64 `json:"max_width,omitempty"                                                                   gt:"0"`
	Uppercase         *bool    `json:"uppercase,omitempty"`
}

// CaptionStyle is TextStyle plus the caption fields.
type CaptionStyle struct {
	TextStyle
	Position            *string `json:"position,omitempty"             enum:"bottom,middle,top,custom"`
	Highlight           *string `json:"highlight,omitempty"            enum:"none,word"`
	HighlightColor      *Color  `json:"highlight_color,omitempty"`
	HighlightBackground *Color  `json:"highlight_background,omitempty"`
	Extra               Extra   `json:"-"`
}

// ProjectFont is a user font of the project (BETA, SDD-001 §3.9).
type ProjectFont struct {
	Files []FontFile `json:"files" minitems:"1"`
	Extra Extra      `json:"-"`
}

// FontFile is one weight of a user font, stored by hash.
type FontFile struct {
	Weight int    `json:"weight" min:"100" max:"900" step:"100"`
	Hash   string `json:"hash"                                  pattern:"^sha256:[0-9a-f]{64}$"`
	Extra  Extra  `json:"-"`
}

// Slot marks an item as a template slot (SDD-001 §3.10).
type Slot struct {
	Name           string `json:"name"                  pattern:"^[a-z][a-z0-9_]{0,39}$"`
	Kind           string `json:"kind"                                                   enum:"media,text,caption"`
	Description    string `json:"description,omitempty"                                                              maxlen:"200"`
	Required       bool   `json:"required"                                                                                        default:"true"`
	DurationPolicy string `json:"duration_policy"                                        enum:"fit,stretch_timeline"              default:"fit"`
	Filled         bool   `json:"filled,omitempty"`
	Extra          Extra  `json:"-"`
}

// TemplateInfo is present in a template draft and in a project instantiated from a
// template. Instance tells them apart (SDD-001 §3.10): a draft has open slots by
// design, an instance must fill them before export.
type TemplateInfo struct {
	Description string           `json:"description"`
	Colors      map[string]Color `json:"colors,omitempty"`
	Instance    bool             `json:"instance,omitempty" desc:"True in a project made from a template: its open slots are lint errors."`
	Extra       Extra            `json:"-"`
}

// BundledFamilies are the font families shipped in the binary (ADR-0012). The
// weights shipped per family are listed in renderer/fonts/FONTS.md; until that
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
