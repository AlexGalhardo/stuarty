package image

import (
	"bytes"
	"encoding/json"
	"maps"

	"github.com/AlexGalhardo/stuarty/core/timeline"
)

// SchemaVersion is the version of the stored document (SDD-021 section 2).
const SchemaVersion = 1

// Identifier prefixes of SDD-000 section 2.
const (
	PrefixImage = "img"
	PrefixLayer = "lyr"
)

// The kinds of layer.
const (
	KindImage = "image"
	KindText  = "text"
	KindShape = "shape"
	KindFill  = "fill"
	KindGroup = "group"
)

// The shapes of a shape layer, those of a shape item of a video.
const (
	ShapeRect        = "rect"
	ShapeRoundedRect = "rounded_rect"
	ShapeEllipse     = "ellipse"
)

// Transparent is the background of a document that has none.
const Transparent = "transparent"

// Limits (SDD-021 section 12).
const (
	MaxCanvas      = 4096
	MaxLayers      = 500
	MaxDepth       = 8
	MaxGroupLayers = 100
	MaxAssets      = 64
	MaxText        = 2000
	MaxName        = 120
	MaxSlots       = 25
	MaxCoord       = 16384
	MaxBoxSide     = 16384
)

// Document is an image: a canvas and the layers on it, bottom first.
type Document struct {
	SchemaVersion int                 `json:"schema_version"`
	ID            string              `json:"id"`
	Name          string              `json:"name"`
	Width         int                 `json:"width"`
	Height        int                 `json:"height"`
	Background    string              `json:"background"`
	Preset        string              `json:"preset,omitempty"`
	Template      bool                `json:"template"`
	Assets        map[string]AssetRef `json:"assets,omitempty"`
	Layers        []Layer             `json:"layers"`
}

// AssetRef is an image the document uses, by content hash: what the core needs of it without reading a file.
type AssetRef struct {
	Hash   string `json:"hash"`
	Name   string `json:"name"`
	Width  int    `json:"width"`
	Height int    `json:"height"`
}

// Crop is a rectangle of the source of an image layer, in source pixels.
type Crop struct {
	X int `json:"x"`
	Y int `json:"y"`
	W int `json:"w"`
	H int `json:"h"`
}

// Layer is one layer. Position and size are whole pixels of the parent; Order is the sort key among the
// siblings (SDD-001 section 3.3.1) and always agrees with the position in the slice.
type Layer struct {
	ID       string `json:"id"`
	Kind     string `json:"kind"`
	Name     string `json:"name"`
	Order    string `json:"order"`
	Visible  bool   `json:"visible"`
	Locked   bool   `json:"locked,omitempty"`
	Opacity  int    `json:"opacity"`
	X        int    `json:"x"`
	Y        int    `json:"y"`
	W        int    `json:"w"`
	H        int    `json:"h"`
	Rotation int    `json:"rotation,omitempty"`
	Slot     string `json:"slot,omitempty"`

	Asset string `json:"asset,omitempty"` // image
	Crop  *Crop  `json:"crop,omitempty"`  // image

	Text  string                 `json:"text,omitempty"`  // text
	Style *timeline.CaptionStyle `json:"style,omitempty"` // text

	Shape       string `json:"shape,omitempty"` // shape
	Fill        string `json:"fill,omitempty"`  // shape and fill
	StrokeColor string `json:"stroke_color,omitempty"`
	StrokeWidth int    `json:"stroke_width,omitempty"`
	Radius      int    `json:"radius,omitempty"`

	Children []Layer `json:"children,omitempty"` // group
}

// Clone returns a deep copy of the document.
func Clone(d *Document) *Document {
	out := *d
	out.Layers = cloneLayers(d.Layers)

	if d.Assets != nil {
		out.Assets = maps.Clone(d.Assets)
	}

	return &out
}

func cloneLayers(in []Layer) []Layer {
	if in == nil {
		return nil
	}

	out := make([]Layer, len(in))
	for i := range in {
		out[i] = cloneLayer(&in[i])
	}

	return out
}

func cloneLayer(l *Layer) Layer {
	out := *l
	if l.Crop != nil {
		c := *l.Crop
		out.Crop = &c
	}

	if l.Style != nil {
		out.Style = cloneStyle(l.Style)
	}

	out.Children = cloneLayers(l.Children)

	return out
}

func cloneStyle(s *timeline.CaptionStyle) *timeline.CaptionStyle {
	out, err := timeline.StyleFromMap(timeline.StyleToMap(s))
	if err != nil {
		panic("image: a stored style does not round trip: " + err.Error())
	}

	return out
}

// Canonical is the canonical JSON of a document: keys sorted, no whitespace, so that the same document always
// has the same bytes and the same digest.
func Canonical(d *Document) []byte {
	raw, err := json.Marshal(d)
	if err != nil {
		panic("image: a document does not encode: " + err.Error())
	}

	var tree any

	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()

	if err := dec.Decode(&tree); err != nil {
		panic("image: a document does not decode: " + err.Error())
	}

	out, err := json.Marshal(tree)
	if err != nil {
		panic("image: a document does not encode again: " + err.Error())
	}

	return out
}

// Decode reads a stored document and checks it.
func Decode(raw []byte) (*Document, error) {
	var d Document

	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()

	if err := dec.Decode(&d); err != nil {
		return nil, err
	}

	return &d, nil
}

// New returns the empty document that image.create fills in: it has an id and nothing else.
func New(id string) *Document {
	return &Document{SchemaVersion: SchemaVersion, ID: id, Layers: []Layer{}}
}

// Count is the number of layers of the document, groups included.
func Count(d *Document) int {
	n := 0
	Walk(d, func(*Layer, *Layer, int) { n++ })

	return n
}

// Walk calls fn for every layer, depth first, bottom first, with its parent (nil at the top level) and its
// depth (0 at the top level).
func Walk(d *Document, fn func(l, parent *Layer, depth int)) {
	walk(d.Layers, nil, 0, fn)
}

func walk(ls []Layer, parent *Layer, depth int, fn func(l, parent *Layer, depth int)) {
	for i := range ls {
		fn(&ls[i], parent, depth)
		walk(ls[i].Children, &ls[i], depth+1, fn)
	}
}

// Slots lists the slot layers of the document in plan order.
func Slots(d *Document) []*Layer {
	var out []*Layer

	Walk(d, func(l, _ *Layer, _ int) {
		if l.Slot != "" {
			out = append(out, l)
		}
	})

	return out
}
