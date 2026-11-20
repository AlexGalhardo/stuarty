package plan

import (
	"cmp"
	"encoding/json"
	"slices"
	"strings"

	"github.com/AlexGalhardo/stuarty/core/timeline"
)

// MaxWindow is the largest number of frames a window covers (SDD-004 §10).
const MaxWindow = 300

// Series is a value over the frames of a layer: one number when it is the
// same on every frame, an array with one value per frame otherwise.
type Series []float64

// MarshalJSON writes the constant form when it applies.
func (s Series) MarshalJSON() ([]byte, error) {
	if len(s) > 0 && !slices.ContainsFunc(s, func(v float64) bool { return v != s[0] }) {
		return json.Marshal(s[0])
	}

	return json.Marshal([]float64(s))
}

// WindowPlan is the compact form of SDD-004 §10. The caller sets Revision.
type WindowPlan struct {
	Revision    int64         `json:"revision"`
	From        int           `json:"from"`
	To          int           `json:"to"`
	EndFrame    int           `json:"end_frame"`
	Width       int           `json:"width"`
	Height      int           `json:"height"`
	Background  string        `json:"background"`
	Transparent bool          `json:"transparent,omitempty"`
	SafeArea    [4]float64    `json:"safe_area"`
	Layers      []WindowLayer `json:"layers"`
	Audio       []WindowAudio `json:"audio"`
}

// WindowLayer is one item over the frames of the window in which it is active.
type WindowLayer struct {
	Item      string    `json:"item"`
	Kind      LayerKind `json:"kind"`
	Asset     string    `json:"asset,omitempty"`
	AssetKind string    `json:"asset_kind,omitempty"`
	// Timeline is the timeline whose assets table names Asset (and Lut) when the layer comes from a composition;
	// empty for a layer of the timeline of the plan (SDD-004 section 3.16).
	Timeline string `json:"timeline,omitempty"`
	First    int    `json:"first"`
	Last     int    `json:"last"`

	// Media layers: the time in the proxy at which each frame starts, the
	// crop in source pixels [x, y, w, h], and the colour matrix (null when neutral).
	SourceSeconds []float64   `json:"source_seconds,omitempty"`
	Crop          *[4]float64 `json:"crop,omitempty"`
	// Crops is the crop of each frame of the window, present only when a crop side is animated; then it
	// replaces crop (SDD-004 §3.4).
	Crops       [][4]float64    `json:"crops,omitempty"`
	ColorMatrix json.RawMessage `json:"color_matrix,omitempty"`
	// ColorMatrices is the matrix of each frame of the window, present only when a colour property is
	// animated and the matrix changes (SDD-004 §3.5); then it replaces color_matrix.
	ColorMatrices []json.RawMessage `json:"color_matrices,omitempty"`

	// Animated image layers (BETA, SDD-004 section 3.3): the frame of the image shown at
	// each frame of the window. Absent for video and still images.
	SourceFrames []int `json:"source_frames,omitempty"`

	// Bitmap layers: the cache key of each frame's bitmap and its natural size.
	Bitmaps *WindowBitmaps `json:"bitmaps,omitempty"`
	Natural *[2]float64    `json:"natural,omitempty"`

	// Group layers (SDD-004 §3.16): the nested frame and its window layers, bottom first, each with the series
	// of the frames of the window in which it is present. Crop is the whole nested frame.
	Group *WindowGroup `json:"group,omitempty"`

	// Both kinds: constant over the item (filter and blur are not keyframed).
	Filter *[12]float64 `json:"filter,omitempty"`
	Blur   float64      `json:"blur,omitempty"`

	ChromaKey *ChromaKeySpec `json:"chroma_key,omitempty"`

	Grade *GradeSpec `json:"grade,omitempty"`

	Lut *LutSpec `json:"lut,omitempty"`

	Blend string `json:"blend,omitempty"`

	FlipX bool `json:"flip_x,omitempty"`
	FlipY bool `json:"flip_y,omitempty"`

	Mask *MaskSpec `json:"mask,omitempty"`

	// Constant over the item: the drop shadow and the outline (SDD-004 §3.15).
	Shadow  *ShadowSpec  `json:"shadow,omitempty"`
	Outline *OutlineSpec `json:"outline,omitempty"`
	// Masks is the mask of each frame of the window, present only when a mask property is animated; then it
	// replaces mask (SDD-004 §3.9).
	Masks []*MaskSpec `json:"masks,omitempty"`

	// Constant over the item: the pixel effects (SDD-004 §3.13). Transition is the whip, dip or wipe at
	// each frame of the window, null where there is none; absent when no frame has one.
	Effects    []timeline.Effect `json:"effects,omitempty"`
	Transition []*TransitionSpec `json:"transition,omitempty"`

	CX       Series `json:"cx"`
	CY       Series `json:"cy"`
	W        Series `json:"w"`
	H        Series `json:"h"`
	Rotation Series `json:"rotation"`
	Scale    Series `json:"scale"`
	Opacity  Series `json:"opacity"`
}

// WindowGroup is the nested frame of a group layer in the compact form: its size in its own pixels and the layers
// of every frame of the window, by their ids (paths, SDD-004 §3.16).
type WindowGroup struct {
	Width  int           `json:"width"`
	Height int           `json:"height"`
	Layers []WindowLayer `json:"layers"`
}

// WindowBitmaps selects, for each frame, one of the keys.
type WindowBitmaps struct {
	Keys  []string `json:"keys"`
	Index []int    `json:"index"`
}

// WindowAudio is one audio source over the frames of the window.
type WindowAudio struct {
	Item          string     `json:"item"`
	Asset         string     `json:"asset"`
	First         int        `json:"first"`
	Last          int        `json:"last"`
	SourceSeconds float64    `json:"source_seconds"` // at the first frame
	Speed         float64    `json:"speed"`
	Curve         *SpeedRamp `json:"speed_curve,omitempty"`
	Reverse       bool       `json:"reverse,omitempty"`
	Gain          Series     `json:"gain"`
	Denoise       float64    `json:"denoise,omitempty"`
	Voice         string     `json:"voice,omitempty"`
	// Processed names the file of the item's processed sound, when it has a chain or a denoise (SDD-004 section 7.1).
	Processed *ProcessedAudio `json:"processed,omitempty"`
}

// Window returns the plans of frames [from, to) in the compact form of
// SDD-004 §10. A window has at most MaxWindow frames: a longer one is cut.
func Window(doc *timeline.Document, from, to int) WindowPlan {
	return WindowWith(doc, from, to, Options{})
}

// WindowWith is Window with a Measurer for the natural size of bitmaps.
func WindowWith(doc *timeline.Document, from, to int, opts Options) WindowPlan {
	from = max(from, 0)
	to = min(to, from+MaxWindow)
	to = max(to, from)

	st := doc.Settings
	safe := SafeArea(st.Width, st.Height)
	w := WindowPlan{
		From:     from,
		To:       to,
		EndFrame: endFrame(doc),
		Width:    st.Width,
		Height:   st.Height,
		Background: backgroundOf(
			st,
		),
		Transparent: st.Background == timeline.BackgroundTransparent,
		SafeArea:    [4]float64{safe.X, safe.Y, safe.W, safe.H},
		Layers:      []WindowLayer{},
		Audio:       []WindowAudio{},
	}

	p := newPlanner(doc, opts)
	acc := map[string]*layerAcc{}

	for t := from; t < to; t++ {
		layers := p.frame(t).Layers
		for i := range layers {
			l := &layers[i]

			a := acc[l.ItemID]
			if a == nil {
				a = newLayerAcc(p, t, to-t, l)
				acc[l.ItemID] = a
			}

			a.add(t, l)
		}
	}

	for _, id := range sortedLayers(acc) {
		acc[id].finish()
		w.Layers = append(w.Layers, acc[id].wl)
	}

	w.Audio = append(windowAudio(doc, from, to), nestedWindowAudio(doc, from, to, opts)...)

	return w
}

// endFrame is the end of the project: the largest start+duration, 0 when empty.
func endFrame(doc *timeline.Document) int {
	end := 0
	for id := range doc.Items {
		it := doc.Items[id]
		end = max(end, it.Start+it.Duration)
	}

	return end
}

// layerAcc builds the window form of one item as its frames arrive.
type layerAcc struct {
	wl        WindowLayer
	asset     timeline.Asset // media layers
	keyByWord map[int]int    // bitmap layers: active word -> index in the keys
	colourKF  bool           // media layers: a colour property has keyframes
	maskKF    bool           // a mask property has keyframes
	cropKF    bool           // media layers: a crop side has keyframes

	// order is the track order of the item and of each composition item above it, which sorts the layers
	// bottom first among their siblings (SDD-004 §3.16).
	order []string
	// Group layers: the accumulators of the nested layers, by id, and what they need to be made.
	p    *planner
	to   int
	kids map[string]*layerAcc
}

// orderKey is the track order of the item a layer id names and, for a path, of each composition item on the
// way down: nested layers of one composition item sort together, in the place of the item.
func orderKey(p *planner, id string) []string {
	var key []string

	for cur := p; cur != nil; {
		head, rest, more := strings.Cut(id, "/")

		it, ok := cur.doc.Items[head]
		if !ok {
			break
		}

		key = append(key, cur.doc.Tracks[it.Track].Order)

		if !more || cur.kids[head] == nil {
			break
		}

		cur, id = cur.kids[head].p, rest
	}

	return key
}

func newLayerAcc(p *planner, t, room int, first *Layer) *layerAcc {
	it, owner, _ := p.lookup(first.ItemID)
	if owner == p {
		room = min(room, it.Start+it.Duration-t)
	}

	room = max(room, 0)

	a := &layerAcc{order: orderKey(p, first.ItemID), p: p, to: t + room}
	for _, name := range timeline.ColourProps {
		a.colourKF = a.colourKF || len(it.Keyframes[name]) > 0
	}
	for _, name := range timeline.MaskProps {
		a.maskKF = a.maskKF || len(it.Keyframes[name]) > 0
	}

	for _, name := range timeline.CropProps {
		a.cropKF = a.cropKF || len(it.Keyframes[name]) > 0
	}

	a.wl = WindowLayer{
		Timeline:  ownerTimeline(owner),
		Item:      first.ItemID,
		Kind:      first.Kind,
		Asset:     first.AssetID,
		AssetKind: first.AssetKind,
		First:     t,
		Last:      t,
		CX:        make(Series, 0, room),
		CY:        make(Series, 0, room),
		W:         make(Series, 0, room),
		H:         make(Series, 0, room),
		Rotation:  make(Series, 0, room),
		Scale:     make(Series, 0, room),
		Opacity:   make(Series, 0, room),
		Filter:    first.Filter,
		Blur:      first.Blur,
		ChromaKey: first.ChromaKey,
		Grade:     first.Grade,
		Lut:       first.Lut,
		Blend:     first.Blend,
		Mask:      first.Mask,
		Shadow:    first.Shadow,
		Outline:   first.Outline,
		FlipX:     first.FlipX,
		FlipY:     first.FlipY,
		Effects:   first.Effects,
	}

	if first.Kind == Group {
		a.wl.Group = &WindowGroup{Width: first.Group.Width, Height: first.Group.Height, Layers: []WindowLayer{}}
		a.wl.Crop = &[4]float64{first.Crop.X, first.Crop.Y, first.Crop.W, first.Crop.H}
		a.kids = map[string]*layerAcc{}

		return a
	}

	if first.Kind == Media {
		if owner == nil {
			owner = p
		}

		a.asset = owner.doc.Assets[first.AssetID]
		a.wl.SourceSeconds = make([]float64, 0, room)

		if a.asset.IsAnimated() {
			a.wl.SourceFrames = make([]int, 0, room)
		}
		a.wl.Crop = &[4]float64{first.Crop.X, first.Crop.Y, first.Crop.W, first.Crop.H}
		a.wl.ColorMatrix = json.RawMessage("null")

		if first.ColorMatrix != nil {
			if data, err := json.Marshal(first.ColorMatrix); err == nil {
				a.wl.ColorMatrix = data
			}
		}

		return a
	}

	var nw, nh float64
	if p.opts.Measure != nil {
		nw, nh = p.opts.Measure(*first.Bitmap)
	}

	a.wl.Natural = &[2]float64{round3(nw), round3(nh)}
	a.wl.Bitmaps = &WindowBitmaps{Keys: []string{}, Index: make([]int, 0, room)}
	a.keyByWord = map[int]int{}

	return a
}

// transition keeps the whip, dip or wipe of the frame; the series is started when one appears, with
// null for the frames before.
func (a *layerAcc) transition(l *Layer) {
	wl := &a.wl
	n := len(wl.Opacity) // frames added so far, this one included

	if l.Transition == nil && wl.Transition == nil {
		return
	}

	for len(wl.Transition) < n-1 {
		wl.Transition = append(wl.Transition, nil)
	}

	wl.Transition = append(wl.Transition, l.Transition)
}

func (a *layerAcc) add(t int, l *Layer) {
	wl := &a.wl
	wl.Last = t
	wl.CX, wl.CY = append(wl.CX, l.CX), append(wl.CY, l.CY)
	wl.W, wl.H = append(wl.W, l.W), append(wl.H, l.H)
	wl.Rotation, wl.Opacity = append(wl.Rotation, l.Rotation), append(wl.Opacity, l.Opacity)
	wl.Scale = append(wl.Scale, l.Scale)
	a.transition(l)

	if a.maskKF {
		wl.Masks = append(wl.Masks, l.Mask)
	}

	if l.Kind == Group {
		a.addKids(t, l)

		return
	}

	if l.Kind == Media {
		if a.cropKF {
			wl.Crops = append(wl.Crops, [4]float64{l.Crop.X, l.Crop.Y, l.Crop.W, l.Crop.H})
		}

		sec := 0.0
		if a.asset.FrameRate.Num > 0 {
			sec = float64(l.SourceFrame) * float64(a.asset.FrameRate.Den) / float64(a.asset.FrameRate.Num)
		}

		wl.SourceSeconds = append(wl.SourceSeconds, round3(sec))
		a.addMatrix(l)

		if wl.SourceFrames != nil {
			wl.SourceFrames = append(wl.SourceFrames, l.SourceFrame)
		}

		return
	}

	// Only the active word changes the bitmap of an item from frame to frame.
	i, ok := a.keyByWord[l.Bitmap.ActiveWord]
	if !ok {
		i = len(wl.Bitmaps.Keys)
		wl.Bitmaps.Keys = append(wl.Bitmaps.Keys, l.Bitmap.Key())
		a.keyByWord[l.Bitmap.ActiveWord] = i
	}

	wl.Bitmaps.Index = append(wl.Bitmaps.Index, i)
}

// addKids keeps the nested layers of the frame of a group layer, each in its own accumulator.
func (a *layerAcc) addKids(t int, l *Layer) {
	for i := range l.Group.Layers {
		kl := &l.Group.Layers[i]

		ka := a.kids[kl.ItemID]
		if ka == nil {
			ka = newLayerAcc(a.p, t, a.to-t, kl)
			a.kids[kl.ItemID] = ka
		}

		ka.add(t, kl)
	}
}

// finish puts the nested layers of a group in its window layer, bottom first.
func (a *layerAcc) finish() {
	if a.wl.Group == nil {
		return
	}

	for _, id := range sortedLayers(a.kids) {
		a.kids[id].finish()
		a.wl.Group.Layers = append(a.wl.Group.Layers, a.kids[id].wl)
	}
}

// sortedLayers orders the layers bottom first: by track order descending (of each composition item on the way
// down, then of the item), then first frame, then id.
func sortedLayers(acc map[string]*layerAcc) []string {
	ids := make([]string, 0, len(acc))
	for id := range acc {
		ids = append(ids, id)
	}

	slices.SortFunc(ids, func(a, b string) int {
		return cmp.Or(
			-slices.Compare(acc[a].order, acc[b].order),
			cmp.Compare(acc[a].wl.First, acc[b].wl.First),
			cmp.Compare(a, b),
		)
	})

	return ids
}

// windowAudio lists the audible items that sound in [from, to) with their
// gain on each frame.
func windowAudio(doc *timeline.Document, from, to int) []WindowAudio {
	out := []WindowAudio{}
	fps := doc.Settings.FPS

	for _, it := range sources(doc) {
		cs, ce := max(from, it.Start), min(to, it.Start+it.Duration)
		if cs >= ce {
			continue
		}

		wa := WindowAudio{
			Item:    it.ID,
			Asset:   it.Asset,
			First:   cs,
			Last:    ce - 1,
			Denoise: round3(it.Denoise),
			Voice:   it.Voice,
		}

		wa.Processed = processedOf(doc, it)

		pos := int64(it.SourceIn)
		if it.Speed.Den > 0 {
			pos += int64(it.SourceOffset(cs - it.Start))
			wa.Speed = round3(float64(it.Speed.Num) / float64(it.Speed.Den))
			wa.Curve = curveOf(it, cs)
			wa.Reverse = it.Reverse
		}

		wa.SourceSeconds = round3(float64(pos) * float64(fps.Den) / float64(fps.Num))

		k := doc.TrackGain(it.Track)

		for t := cs; t < ce; t++ {
			wa.Gain = append(wa.Gain, round3(gainAt(it, float64(t-it.Start))*k))
		}

		out = append(out, wa)
	}

	return out
}

// addMatrix keeps the colour matrix of a frame when it differs from the first one: the series starts
// at the first difference, filled with the first matrix for the frames before.
func (a *layerAcc) addMatrix(l *Layer) {
	if !a.colourKF {
		return
	}

	wl := &a.wl
	m := json.RawMessage("null")

	if l.ColorMatrix != nil {
		if data, err := json.Marshal(l.ColorMatrix); err == nil {
			m = data
		}
	}

	if wl.ColorMatrices == nil {
		if string(m) == string(wl.ColorMatrix) {
			return
		}

		for len(wl.ColorMatrices) < len(wl.Opacity)-1 {
			wl.ColorMatrices = append(wl.ColorMatrices, wl.ColorMatrix)
		}
	}

	wl.ColorMatrices = append(wl.ColorMatrices, m)
}

// ownerTimeline is the id of the timeline that holds the item of a layer when it is a nested one: the planner of
// a composition has a prefix, the one of the timeline of the plan has none.
func ownerTimeline(owner *planner) string {
	if owner == nil || owner.prefix == "" {
		return ""
	}

	return owner.doc.ID
}
