package plan

import (
	"math"
	"slices"
	"sort"

	"github.com/AlexGalhardo/stuarty/core/timeline"
)

// planner holds what Frame needs and Window reuses for every frame: the
// visible tracks bottom first with their items in time order.
type planner struct {
	doc    *timeline.Document
	opts   Options
	tracks []trackItems
}

type trackItems struct {
	id    string
	order string
	items []*timeline.Item // by start
}

func newPlanner(doc *timeline.Document, opts Options) *planner {
	p := &planner{doc: doc, opts: opts}
	byTrack := map[string]*trackItems{}

	for id := range doc.Tracks {
		t := doc.Tracks[id]
		if t.Kind == timeline.TrackAudio || !doc.TrackVisible(id) {
			continue
		}

		byTrack[id] = &trackItems{id: id, order: t.Order}
	}

	for id := range doc.Items {
		it := doc.Items[id]
		if ti, ok := byTrack[it.Track]; ok && it.Type != timeline.ItemAudio && !it.Disabled {
			ti.items = append(ti.items, &it)
		}
	}

	for _, ti := range byTrack {
		slices.SortFunc(ti.items, func(a, b *timeline.Item) int { return a.Start - b.Start })
		p.tracks = append(p.tracks, *ti)
	}

	// Descending order: the first track in order is the top layer, so it is last here.
	slices.SortFunc(p.tracks, func(a, b trackItems) int {
		switch {
		case a.order > b.order:
			return -1
		case a.order < b.order:
			return 1
		}

		return 0
	})

	return p
}

// activeItem is the item of the track active at frame t, or nil. A track
// holds at most one item at a frame.
func (ti *trackItems) activeItem(t int) *timeline.Item {
	// The first item that starts after t; the one before it is the candidate.
	i := sort.Search(len(ti.items), func(i int) bool { return ti.items[i].Start > t })
	if i == 0 {
		return nil
	}

	if it := ti.items[i-1]; t < it.Start+it.Duration {
		return it
	}

	return nil
}

// Frame returns what frame t of the document contains. It is pure. Bitmap
// layers have size 0 unless a Measurer is given; see FrameWith.
func Frame(doc *timeline.Document, t int) FramePlan { return FrameWith(doc, t, Options{}) }

// FrameWith is Frame with a Measurer for the natural size of bitmaps.
func FrameWith(doc *timeline.Document, t int, opts Options) FramePlan {
	return newPlanner(doc, opts).frame(t)
}

// Planner plans frames of one document; building it once and asking many
// frames avoids copying and sorting the items per frame. It is not safe for
// concurrent use.
type Planner struct{ p *planner }

// NewPlanner prepares the frames of doc.
func NewPlanner(doc *timeline.Document, opts Options) *Planner {
	return &Planner{p: newPlanner(doc, opts)}
}

// Frame is FrameWith for this planner's document and options.
func (pl *Planner) Frame(t int) FramePlan { return pl.p.frame(t) }

func (p *planner) frame(t int) FramePlan {
	st := p.doc.Settings
	fp := FramePlan{
		Frame:       t,
		Width:       st.Width,
		Height:      st.Height,
		Background:  backgroundOf(st),
		Transparent: st.Background == timeline.BackgroundTransparent,
		Layers:      make([]Layer, 0, len(p.tracks)),
	}

	for i := range p.tracks {
		it := p.tracks[i].activeItem(t)
		if it == nil {
			continue
		}

		if l, ok := p.layer(it, t-it.Start); ok {
			fp.Layers = append(fp.Layers, l)
		}
	}

	return fp
}

// backgroundOf is the background colour of the plan: black when the project has none (settings.background
// "transparent"), so that every executor has a colour to start from.
func backgroundOf(st timeline.Settings) string {
	if st.Background == timeline.BackgroundTransparent {
		return "#000000"
	}

	return string(st.Background)
}

func (p *planner) layer(it *timeline.Item, r int) (Layer, bool) {
	l, ok := p.layerBase(it, r)
	if !ok {
		return l, false
	}

	l.Effects, l.Transition = pixelEffects(it), p.transitionAt(it, r)
	p.applyShake(it, r, &l)

	return l, true
}

func (p *planner) layerBase(it *timeline.Item, r int) (Layer, bool) {
	st := p.doc.Settings
	w, h := float64(st.Width), float64(st.Height)
	a := evaluate(it, r, w, h, w/2, h/2)
	l := Layer{ItemID: it.ID, Rotation: round3(a.rotation), Scale: round3(a.scale), Opacity: round3(a.opacity)}
	l.Filter, l.Blur = FilterMatrix(it.Filter, it.FilterAmount), round3(it.Blur)

	if it.Blend != "" && it.Blend != "normal" {
		l.Blend = it.Blend
	}

	l.FlipX, l.FlipY = it.FlipX, it.FlipY

	l.Mask = maskAt(it, r)

	if s := it.Shadow; s != nil {
		l.Shadow = &ShadowSpec{
			Color: resolveColor(p.doc, string(s.Color)), Opacity: round3(s.Opacity),
			DX: round3(s.DX), DY: round3(s.DY), Blur: round3(s.Blur),
		}
	}

	if o := it.Outline; o != nil {
		l.Outline = &OutlineSpec{Color: resolveColor(p.doc, string(o.Color)), Width: round3(o.Width)}
	}

	if k := it.ChromaKey; k != nil {
		l.ChromaKey = &ChromaKeySpec{
			Color: resolveColor(p.doc, string(k.Color)), Similarity: round3(k.Similarity),
			Smoothness: round3(k.Smoothness), Spill: round3(k.Spill),
		}
	}

	switch it.Type {
	case timeline.ItemClip:
		return p.mediaLayer(l, it, r, a)
	case timeline.ItemShape:
		sp := &BitmapSpec{
			Kind: "shape", ActiveWord: -1, PixelScale: pixelScale(it, p.opts.OutputScale),
			Shape: &ShapeSpec{
				Shape: it.Shape, Width: it.Width, Height: it.Height, Radius: it.Radius,
				Fill: resolveColor(p.doc, string(it.Fill)), StrokeColor: resolveColor(p.doc, string(it.StrokeColor)),
				StrokeWidth: it.StrokeWidth,
			},
		}

		l.Kind, l.Bitmap = Bitmap, sp
		l.CX, l.CY, l.W, l.H = round3(a.x), round3(a.y), round3(it.Width*a.scale), round3(it.Height*a.scale)

		return l, true
	case timeline.ItemText, timeline.ItemCaption:
		return p.textLayer(l, it, r, a), true
	case timeline.ItemAudio:
		// Audio items make no layer (SDD-004 §2).
	}

	return Layer{}, false
}

// minMask is the smallest width or height of a mask, as in the document (SDD-001 §3.4).
const minMask = 0.001

// maskAt is the mask of the item at frame r inside it, or nil: a mask property with keyframes takes
// the interpolated value, the others their static one (SDD-004 §3.9). The rectangle is kept inside the item.
func maskAt(it *timeline.Item, r int) *MaskSpec {
	m := it.Mask
	if m == nil {
		return nil
	}

	fr := float64(r)
	x, y := property(it, "mask_x", m.X, fr), property(it, "mask_y", m.Y, fr)
	w, h := property(it, "mask_width", m.Width, fr), property(it, "mask_height", m.Height, fr)
	feather := property(it, "mask_feather", float64(m.Feather), fr)

	x, y = min(max(x, 0), 1-minMask), min(max(y, 0), 1-minMask)
	w, h = min(max(w, minMask), 1-x), min(max(h, minMask), 1-y)

	return &MaskSpec{
		Shape: m.Shape, X: round3(x), Y: round3(y), Width: round3(w), Height: round3(h),
		Feather: int(min(max(math.Round(feather), 0), timeline.MaxMaskFeather)), Invert: m.Invert,
	}
}

// cropAt is the crop fractions of a clip at frame r inside it, and whether it has any: a side with
// keyframes takes the interpolated value (SDD-004 §3.4). Opposite sides are kept below 1 together.
func cropAt(it *timeline.Item, r int) (left, top, right, bottom float64, ok bool) {
	animated := false

	for _, name := range timeline.CropProps {
		animated = animated || len(it.Keyframes[name]) > 0
	}

	if it.Crop == nil && !animated {
		return 0, 0, 0, 0, false
	}

	fr := float64(r)
	left, top = property(
		it,
		"crop_left",
		it.CropValue("crop_left"),
		fr,
	), property(
		it,
		"crop_top",
		it.CropValue("crop_top"),
		fr,
	)
	right, bottom = property(it, "crop_right", it.CropValue("crop_right"), fr),
		property(it, "crop_bottom", it.CropValue("crop_bottom"), fr)

	left, top = min(max(left, 0), 1-minMask), min(max(top, 0), 1-minMask)
	right, bottom = min(max(right, 0), 1-minMask-left), min(max(bottom, 0), 1-minMask-top)

	return left, top, right, bottom, true
}

// mediaLayer places a clip: the source frame, the crop and the cover, contain
// or none size of SDD-004 §3.4.
func (p *planner) mediaLayer(l Layer, it *timeline.Item, r int, a animated) (Layer, bool) {
	asset, ok := p.doc.Assets[it.Asset]
	if !ok {
		return Layer{}, false
	}

	sw, sh := float64(asset.Width), float64(asset.Height)
	crop := Rect{W: sw, H: sh}

	if left, top, right, bottom, ok := cropAt(it, r); ok {
		crop = Rect{X: left * sw, Y: top * sh, W: sw * (1 - left - right), H: sh * (1 - top - bottom)}
	}

	fw, fh := float64(p.doc.Settings.Width), float64(p.doc.Settings.Height)

	b := 1.0

	if crop.W > 0 && crop.H > 0 {
		switch it.Fit {
		case "cover":
			b = max(fw/crop.W, fh/crop.H)
		case "contain":
			b = min(fw/crop.W, fh/crop.H)
		}
	}

	l.Kind, l.AssetID, l.AssetKind = Media, it.Asset, string(asset.Kind)
	l.SourceFrame = sourceFrame(it, &asset, p.doc.Settings.FPS, r)
	l.Crop = Rect{X: round3(crop.X), Y: round3(crop.Y), W: round3(crop.W), H: round3(crop.H)}
	l.ColorMatrix = colorMatrixAt(it, r)
	if len(it.Hsl) > 0 || it.Wheels != nil || it.Curves != nil {
		l.Grade = &GradeSpec{Hsl: it.Hsl, Wheels: it.Wheels, Curves: it.Curves}
	}
	if it.Lut != nil && it.Lut.Intensity > 0 {
		l.Lut = &LutSpec{Asset: it.Lut.Asset, Intensity: it.Lut.Intensity}
	}
	l.CX, l.CY = round3(a.x), round3(a.y)
	l.W, l.H = round3(crop.W*b*a.scale), round3(crop.H*b*a.scale)

	return l, true
}

// fitToMaxWidth is the factor that brings a bitmap of natural width nw back to the style's max_width
// when one word is wider than it: the layout never cuts a word, so the bitmap would leave the frame
// (SDD-004 section 5.1 item 2). The padding, stroke and shadow the bitmap is grown by do not count
// against max_width.
func fitToMaxWidth(st ResolvedStyle, nw float64) float64 {
	grown := 2 * (max(st.BackgroundPadding, st.StrokeWidth/2, math.Abs(st.ShadowDX)+st.ShadowBlur) + 1)
	limit := st.MaxWidth + grown

	if st.MaxWidth <= 0 || nw <= limit {
		return 1
	}

	return limit / nw
}

// textLayer places a text or caption item. Captions anchor to the safe area
// by their position (SDD-004 §4.3); text and custom captions use x and y.
func (p *planner) textLayer(l Layer, it *timeline.Item, r int, a animated) Layer {
	var spec BitmapSpec

	position := "custom"

	if it.Type == timeline.ItemCaption {
		var st ResolvedStyle

		st, position = captionStyle(p.doc, it)
		words := make([]string, len(it.Words))

		for i := range it.Words {
			words[i] = it.Words[i].Text
		}

		active := -1
		if st.Highlight == "word" {
			active = activeWord(it.Words, r)
		}

		spec = BitmapSpec{
			Kind: "caption", Text: captionText(words, it.Breaks), Words: words, Breaks: it.Breaks,
			ActiveWord: active, Style: st, PixelScale: pixelScale(it, p.opts.OutputScale),
		}
	} else {
		spec = BitmapSpec{
			Kind:       "text",
			Text:       it.Text,
			ActiveWord: -1,
			Style:      textStyle(p.doc, it),
			PixelScale: pixelScale(it, p.opts.OutputScale),
		}
	}

	var nw, nh float64
	if p.opts.Measure != nil {
		nw, nh = p.opts.Measure(spec)
	}

	fit := fitToMaxWidth(spec.Style, nw)
	w, h := nw*fit*a.scale, nh*fit*a.scale
	cx, cy := a.x, a.y
	fw, fh := float64(p.doc.Settings.Width), float64(p.doc.Settings.Height)
	safe := SafeArea(p.doc.Settings.Width, p.doc.Settings.Height)

	switch position {
	case "bottom":
		cx, cy = fw/2, safe.Y+safe.H-h/2
	case "top":
		cx, cy = fw/2, safe.Y+h/2
	case "middle":
		cx, cy = fw/2, fh/2
	}

	l.Kind, l.Bitmap = Bitmap, &spec
	l.CX, l.CY, l.W, l.H = round3(cx), round3(cy), round3(w), round3(h)

	return l
}
