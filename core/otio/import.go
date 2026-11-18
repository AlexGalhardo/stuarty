package otio

import (
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"math"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/AlexGalhardo/vukem-video/core/command"
	"github.com/AlexGalhardo/vukem-video/core/timeline"
)

// ErrInvalid is returned for a file that is not an OTIO timeline this package reads.
var ErrInvalid = errors.New("otio: not a readable OpenTimelineIO timeline")

type entry struct {
	clip     bool
	text     bool
	shape    map[string]any // shape items only: the arguments of shape.add
	caption  *captionSpec   // caption items only
	ref      string
	name     string
	start    int // timeline frame, clips and texts only
	source   int
	duration int
	in, out  int // transition offsets in frames
	props    []propOp
	speed    *timeline.Rational
	curve    []timeline.SpeedPoint
	reverse  bool
	slot     *slotSpec
	markers  []timeline.Marker // clips only: the markers of the item, in frames of the item
	body     string            // texts only
	style    map[string]any    // texts only
}

type importTrack struct {
	name          string
	audio         bool
	hidden, muted bool
	magnet        bool
	locked, solo  bool
	volume        *float64
	captionStyle  map[string]any
	entries       []entry
}

// Import is a parsed OTIO file: what it asks for, not a document. Operations turns
// it into a batch for Apply, so that undo and every rule of the command core hold.
type Import struct {
	Name  string            `json:"name"`
	FPS   timeline.Rational `json:"fps"`   // the frame rate of the file; the caller creates the project with it
	Media []Media           `json:"media"` // every distinct reference, in file order
	Lost  []Lost            `json:"lost"`  // what the batch cannot carry

	// KnownFonts are the families of the fonts of the project the batch applies to; a text or caption
	// style may use them besides the bundled fonts. The caller sets it before Operations.
	KnownFonts []string `json:"-"`

	tracks  []importTrack
	markers []timeline.Marker
}

type reader struct {
	im    *Import
	fps   timeline.Rational
	seen  map[string]bool
	slots map[string]bool
}

// Parse reads an OTIO file. It never opens a media reference: each target_url is
// only classified (Candidate or Unresolved). Nesting deeper than MaxDepth is an error.
func Parse(data []byte) (*Import, error) {
	var root map[string]any
	if err := json.Unmarshal(data, &root); err != nil {
		return nil, fmt.Errorf("%w: %w", ErrInvalid, err)
	}

	if !schemaIs(root, "Timeline") {
		return nil, fmt.Errorf("%w: the root is not a Timeline", ErrInvalid)
	}

	stack, _ := root["tracks"].(map[string]any)
	if !schemaIs(stack, "Stack") {
		return nil, fmt.Errorf("%w: tracks is not a Stack", ErrInvalid)
	}

	name, _ := root["name"].(string)
	im := &Import{Name: name, Media: []Media{}, Lost: []Lost{}}
	r := &reader{im: im, seen: map[string]bool{}, slots: map[string]bool{}}

	if err := checkDepth(stack, 1); err != nil {
		return nil, err
	}

	r.fps = r.findRate(stack)
	im.FPS = r.fps

	r.timelineMarkers(root, stack)

	kids, _ := stack["children"].([]any)
	for _, k := range kids {
		t, _ := k.(map[string]any)
		if !schemaIs(t, "Track") {
			r.lose("nested composition or unknown track child")

			continue
		}

		r.track(t)
	}

	return im, nil
}

// timelineMarkers reads the markers of the document: the vukem metadata of the timeline when it
// has them, otherwise the native markers of the stack. A marker that no marker.add accepts (a
// frame outside the timeline, a label over 60 characters, a frame already marked, more than 200)
// is reported and left out.
func (r *reader) timelineMarkers(root, stack map[string]any) {
	type cand struct {
		frame int
		label string
		ok    bool
	}

	var cands []cand

	if v := vukemMeta(root); v != nil && v["markers"] != nil {
		list, _ := v["markers"].([]any)
		for _, e := range list {
			m, _ := e.(map[string]any)
			f, ok := m["frame"].(float64)
			label, hasLabel := m["label"].(string)
			ok = ok && f == math.Trunc(f) && (hasLabel || m["label"] == nil)
			cands = append(cands, cand{frame: int(f), label: label, ok: ok})
		}

		if _, isList := v["markers"].([]any); !isList {
			r.lose("vukem property markers is not a valid value")
		}
	} else {
		list, _ := stack["markers"].([]any)
		for _, e := range list {
			m, _ := e.(map[string]any)
			label, _ := m["name"].(string)
			f, ok := r.frames(asMap(m["marked_range"])["start_time"])
			cands = append(cands, cand{frame: f, label: label, ok: ok})
		}
	}

	seen := map[int]bool{}

	for _, c := range cands {
		switch {
		case !c.ok || c.frame < 0 || c.frame >= timeline.MaxFrames || utf8.RuneCountInString(c.label) > 60:
			r.lose("timeline marker that is not a valid value")
		case seen[c.frame]:
			r.lose("timeline marker at a frame that is already marked")
		case len(r.im.markers) >= timeline.MaxMarkers:
			r.lose("timeline markers over the limit of 200")
		default:
			seen[c.frame] = true
			r.im.markers = append(r.im.markers, timeline.Marker{Frame: c.frame, Label: c.label})
		}
	}
}

func asMap(v any) map[string]any {
	m, _ := v.(map[string]any)

	return m
}

func schemaIs(m map[string]any, kind string) bool {
	s, _ := m["OTIO_SCHEMA"].(string)

	return strings.HasPrefix(s, kind+".")
}

// checkDepth bounds the nesting of containers.
func checkDepth(m map[string]any, depth int) error {
	if depth > MaxDepth {
		return fmt.Errorf("%w: nesting deeper than %d", ErrInvalid, MaxDepth)
	}

	kids, _ := m["children"].([]any)
	for _, k := range kids {
		if c, ok := k.(map[string]any); ok {
			if err := checkDepth(c, depth+1); err != nil {
				return err
			}
		}
	}

	return nil
}

func (r *reader) lose(feature string) { r.im.Lost = append(r.im.Lost, Lost{Feature: feature}) }

// findRate returns the frame rate of the first time value of the file, 30 fps when
// there is none or it is not a rate the product supports.
func (r *reader) findRate(m map[string]any) timeline.Rational {
	def := timeline.Rational{Num: 30, Den: 1}

	kids, _ := m["children"].([]any)
	for _, k := range kids {
		c, _ := k.(map[string]any)

		if sr, ok := c["source_range"].(map[string]any); ok {
			if st, ok := sr["start_time"].(map[string]any); ok {
				if rate, ok := st["rate"].(float64); ok {
					if f, err := rateToFPS(rate); err == nil {
						return f
					}

					return def
				}
			}
		}

		if len(c) > 0 {
			if f := r.findRate(c); f != def {
				return f
			}
		}
	}

	return def
}

// frames converts an OTIO RationalTime to frames of the project rate.
func (r *reader) frames(v any) (int, bool) {
	m, _ := v.(map[string]any)
	value, ok1 := m["value"].(float64)
	rate, ok2 := m["rate"].(float64)

	if !ok1 || !ok2 || rate <= 0 || math.IsNaN(value) || math.IsInf(value, 0) {
		return 0, false
	}

	f := math.Round(value * fpsRate(r.fps) / rate)
	if math.Abs(f) > timeline.MaxFrames {
		return 0, false
	}

	return int(f), true
}

func (r *reader) rangeOf(v any) (start, dur int, ok bool) {
	m, _ := v.(map[string]any)

	s, ok1 := r.frames(m["start_time"])
	d, ok2 := r.frames(m["duration"])

	return s, d, ok1 && ok2
}

func (r *reader) track(t map[string]any) {
	name, _ := t["name"].(string)
	name = clipRunes(name, 60)
	kind, _ := t["kind"].(string)

	tk := importTrack{name: name, audio: kind == kindAud}
	if kind != kindVid && kind != kindAud {
		r.lose("track of kind " + kind)

		return
	}

	if v := vukemMeta(t); v != nil {
		tk.hidden, _ = v["hidden"].(bool)
		tk.muted, _ = v["muted"].(bool)
		tk.magnet, _ = v["magnet"].(bool)
		tk.locked, _ = v["locked"].(bool)
		tk.solo, _ = v["solo"].(bool)
		tk.hidden = tk.hidden && !tk.audio

		if f, ok := v["volume"].(float64); ok && f >= 0 && f <= 4 {
			tk.volume = &f
		}

		if raw, present := v["caption_style"]; present && !tk.audio {
			tk.captionStyle = textStyle(raw, true, func(f string) { r.lose(f + " of track " + name) })
		}
	}

	cursor := 0
	kids, _ := t["children"].([]any)

	for _, k := range kids {
		c, _ := k.(map[string]any)

		switch {
		case schemaIs(c, "Gap"):
			if _, d, ok := r.rangeOf(c["source_range"]); ok && d > 0 {
				if e, ok := r.visualItem(c, cursor, d); ok {
					tk.entries = append(tk.entries, e)
				}

				cursor += d
			}
		case schemaIs(c, "Transition"):
			r.transition(&tk, c)
		case schemaIs(c, "Clip"):
			e, ok := r.clip(c, cursor, kind)
			if ok {
				cursor += e.duration
				tk.entries = append(tk.entries, e)
			}
		default:
			r.lose("track child that is not a clip, gap or transition")
		}
	}

	r.im.tracks = append(r.im.tracks, tk)
}

func (r *reader) transition(tk *importTrack, c map[string]any) {
	typ, _ := c["transition_type"].(string)
	in, ok1 := r.frames(c["in_offset"])
	out, ok2 := r.frames(c["out_offset"])

	if typ != dissolve || !ok1 || !ok2 {
		r.lose("transition " + typ)

		return
	}

	tk.entries = append(tk.entries, entry{in: in, out: out})
}

func (r *reader) clip(c map[string]any, cursor int, kind string) (entry, bool) {
	name, _ := c["name"].(string)
	name = clipRunes(name, 120)

	start, dur, ok := r.rangeOf(c["source_range"])
	if !ok || dur < 1 || start < 0 {
		r.lose("clip " + name + " without a usable source_range")

		return entry{}, false
	}

	if es, _ := c["effects"].([]any); len(es) > 0 {
		r.lose("effects of clip " + name)
	}

	ref := ""
	if mr, ok := c["media_reference"].(map[string]any); ok && schemaIs(mr, "ExternalReference") {
		ref, _ = mr["target_url"].(string)
	}

	if !r.seen[ref] {
		r.seen[ref] = true
		m := classify(ref)

		if ref == "" && c["media_reference"] != nil {
			m.Reason = "media reference without a target_url"
		}

		r.im.Media = append(r.im.Media, m)
	}

	e := entry{clip: true, ref: ref, name: name, start: cursor, source: start, duration: dur}
	e.markers = r.clipMarkers(c, name, start, dur)

	if v := vukemMeta(c); v != nil {
		props, _ := v["properties"].(map[string]any)
		mode := modeClip
		if kind == kindAud {
			mode = modeAudio
		}

		spec := itemOps(props, mode, cursor, dur, r.slots, func(f string) { r.lose(f + " of clip " + name) })
		e.props, e.speed, e.curve, e.reverse, e.slot = spec.ops, spec.speed, spec.curve, spec.reverse, spec.slot
	}

	return e, true
}

// clipMarkers reads the markers of a clip as the markers of the item: the vukem export writes each
// one at the source frame of the item plus its own frame, so the frame is the position minus the
// start of the source range. A marker that no marker.add accepts (outside the item, a label over
// 60 characters, a frame already marked, more than 200) is reported and left out.
func (r *reader) clipMarkers(c map[string]any, name string, sourceStart, dur int) []timeline.Marker {
	list, _ := c["markers"].([]any)

	var out []timeline.Marker

	seen := map[int]bool{}

	for _, e := range list {
		m, _ := e.(map[string]any)
		label, _ := m["name"].(string)
		at, ok := r.frames(asMap(m["marked_range"])["start_time"])
		frame := at - sourceStart

		switch {
		case !ok || frame < 0 || frame >= dur || utf8.RuneCountInString(label) > 60:
			r.lose("marker of clip " + name + " that is not a valid value")
		case seen[frame]:
			r.lose("marker of clip " + name + " at a frame that is already marked")
		case len(out) >= timeline.MaxMarkers:
			r.lose("markers of clip " + name + " over the limit of 200")
		default:
			seen[frame] = true
			out = append(out, timeline.Marker{Frame: frame, Label: label})
		}
	}

	return out
}

// visualItem reads a Gap that carries a text, shape or caption item of the vukem metadata; a Gap
// without one, or with one that is not valid, is only a gap.
func (r *reader) visualItem(c map[string]any, cursor, dur int) (entry, bool) {
	v := vukemMeta(c)

	switch {
	case v == nil:
		return entry{}, false
	case v["text"] != nil:
		return r.textItem(c, v, cursor, dur)
	case v["shape"] != nil:
		return r.shapeItem(c, v, cursor, dur)
	case v["caption"] != nil:
		return r.captionItem(c, v, cursor, dur)
	}

	return entry{}, false
}

func (r *reader) shapeItem(c, v map[string]any, cursor, dur int) (entry, bool) {
	name, _ := c["name"].(string)
	name = clipRunes(name, 120)
	lose := func(f string) { r.lose(f + " of shape " + name) }

	args := shapeSpec(v["shape"], lose)
	if args == nil {
		return entry{}, false
	}

	e := entry{shape: args, name: name, start: cursor, duration: dur}
	props, _ := v["properties"].(map[string]any)
	spec := itemOps(props, modeText, cursor, dur, r.slots, lose)
	e.props = spec.ops

	return e, true
}

func (r *reader) captionItem(c, v map[string]any, cursor, dur int) (entry, bool) {
	name, _ := c["name"].(string)
	name = clipRunes(name, 120)
	lose := func(f string) { r.lose(f + " of caption " + name) }

	spec := readCaption(v["caption"], dur, lose)
	if spec == nil {
		return entry{}, false
	}

	e := entry{caption: spec, name: name, start: cursor, duration: dur}
	props, _ := v["properties"].(map[string]any)
	ops := itemOps(props, modeText, cursor, dur, r.slots, lose)
	e.props = ops.ops

	return e, true
}

// textItem reads the text item of a Gap's metadata.
func (r *reader) textItem(c, v map[string]any, cursor, dur int) (entry, bool) {
	name, _ := c["name"].(string)
	name = clipRunes(name, 120)
	lose := func(f string) { r.lose(f + " of text " + name) }

	body, _ := v["text"].(string)
	if n := utf8.RuneCountInString(body); n < 1 || n > 2000 {
		lose("vukem property text is not a valid value")

		return entry{}, false
	}

	e := entry{text: true, name: name, start: cursor, duration: dur, body: body}

	if raw, present := v["style"]; present {
		e.style = textStyle(raw, false, lose)
	}

	props, _ := v["properties"].(map[string]any)
	spec := itemOps(props, modeText, cursor, dur, r.slots, lose)
	e.props, e.slot = spec.ops, spec.slot

	return e, true
}

// textStyle checks a style object like text.add does, except for the project fonts, which are
// not known here: a family that is not bundled is dropped with its weight, never left to fail
// the batch.
func textStyle(raw any, caption bool, lose func(string)) map[string]any {
	style, _ := raw.(map[string]any)

	data, err := json.Marshal(style)
	if err != nil || style == nil {
		lose("vukem property style is not a valid value")

		return nil
	}

	typed, err := timeline.DecodeRaw(data)
	if err != nil {
		lose("vukem property style is not a valid value")

		return nil
	}

	cs, err := timeline.StyleFromMap(typed)
	if err != nil || len(timeline.StyleViolations(cs)) > 0 && !caption ||
		caption && len(timeline.CaptionStyleViolations(cs)) > 0 {
		lose("vukem property style is not a valid value")

		return nil
	}

	return style
}

// Operations returns the batch that builds what the file describes, bottom track
// first. assets maps a Media.Ref to the id of an asset the caller has imported
// from that candidate; a clip whose reference is not in assets is left out and
// reported. newID supplies the identifiers (SDD-001 §2 item 6). The batch applies
// to a project that has the file's FPS and those assets.
func (im *Import) Operations(assets map[string]string, newID func(prefix string) string) ([]command.Operation, []Lost) {
	b := &batch{assets: assets, newID: newID, known: map[string]bool{}}
	for _, f := range im.KnownFonts {
		b.known[f] = true
	}

	for _, m := range im.markers {
		args := map[string]any{"at": frameTime(m.Frame)}
		if m.Label != "" {
			args["label"] = m.Label
		}

		b.push("marker.add", args)
	}

	for i := range im.tracks {
		b.track(&im.tracks[i])
	}

	return append(b.ops, b.pending...), b.lost
}

// placed is a clip the batch has added.
type placed struct {
	id     string
	visual bool
}

type batch struct {
	assets  map[string]string
	known   map[string]bool // project font families
	newID   func(prefix string) string
	ops     []command.Operation
	pending []command.Operation // transitions, after every clip exists
	lost    []Lost
}

func (b *batch) push(op string, args any) {
	b.ops = append(b.ops, command.Operation{Op: op, Args: mustJSON(args)})
}

// knownStyle drops the font family and weight of a style when the family is neither bundled nor a
// font of the project the batch applies to (Import.KnownFonts); the rest of the style is kept, so
// that the batch never fails on a font.
func (b *batch) knownStyle(style map[string]any, what string) map[string]any {
	data, err := json.Marshal(style)
	if err != nil || len(style) == 0 {
		return style
	}

	typed, err := timeline.DecodeRaw(data)
	if err != nil {
		return style
	}

	cs, err := timeline.StyleFromMap(typed)
	if err != nil {
		return style
	}

	family := "Inter"
	if cs.FontFamily != nil {
		family = *cs.FontFamily
	}

	weights, bundled := timeline.BundledWeights[family]
	if b.known[family] || bundled && (cs.FontWeight == nil || slices.Contains(weights, *cs.FontWeight)) {
		return style
	}

	b.lost = append(b.lost, Lost{Name: what, Feature: "style font " + family + " is not a bundled or project font"})

	style = maps.Clone(style)
	delete(style, "font_family")
	delete(style, "font_weight")

	return style
}

func (b *batch) track(tk *importTrack) {
	trackID := b.newID(timeline.PrefixTrack)
	kind := "visual"

	if tk.audio {
		kind = "audio"
	}

	b.push("track.add", map[string]any{"kind": kind, "name": tk.name, "id": trackID, "position": "top"})

	if len(tk.captionStyle) > 0 {
		b.push("track.set_caption_style", map[string]any{
			"track_id": trackID, "style": b.knownStyle(tk.captionStyle, "track "+tk.name), "replace": true,
		})
	}

	if tk.hidden || tk.muted || tk.magnet || tk.solo || tk.volume != nil {
		update := map[string]any{"track_id": trackID}
		if tk.solo {
			update["solo"] = true
		}

		if tk.volume != nil {
			update["volume"] = *tk.volume
		}

		if tk.hidden {
			update["hidden"] = true
		}

		if tk.muted {
			update["muted"] = true
		}

		if tk.magnet {
			update["magnet"] = true
		}

		b.push("track.update", update)
	}

	ids := make([]*placed, len(tk.entries))

	for i := range tk.entries {
		switch e := &tk.entries[i]; {
		case e.clip:
			ids[i] = b.clip(trackID, tk.audio, e)
		case e.text:
			ids[i] = b.text(trackID, e)
		case e.shape != nil:
			ids[i] = b.shape(trackID, e)
		case e.caption != nil:
			ids[i] = b.caption(trackID, e)
		}
	}

	for i := range tk.entries {
		e := &tk.entries[i]
		if e.clip || e.text || e.shape != nil || e.caption != nil {
			continue
		}

		b.transition(ids, i+1, "in", e.out)
		b.transition(ids, i-1, "out", e.in)
	}

	// A locked track refuses every edit of its items, so it is locked after the last one.
	if tk.locked {
		b.pending = append(b.pending, command.Operation{
			Op: "track.update", Args: mustJSON(map[string]any{"track_id": trackID, "locked": true}),
		})
	}
}

// clip adds one clip; it returns nil when the file's reference has no asset.
func (b *batch) clip(trackID string, audio bool, e *entry) *placed {
	asset, ok := b.assets[e.ref]
	if !ok {
		b.lost = append(b.lost, Lost{Name: e.name, Feature: "clip without an imported asset for " + e.ref})

		return nil
	}

	id := b.newID(timeline.PrefixItem)
	op := "clip.add"

	if audio {
		op = "audio.add"
	}

	// A sped clip is added with the source length that set_speed turns back into e.duration, and
	// trimmed to it when no length does (set_speed rounds up).
	length, trim := e.duration, false
	if e.speed != nil {
		length, trim = sourceLength(e.duration, *e.speed)
	}

	b.push(op, map[string]any{
		"track_id": trackID, "asset_id": asset, "id": id,
		"start": frameTime(e.start), "source_in": frameTime(e.source), "duration": frameTime(length),
	})

	if e.speed != nil {
		args := map[string]any{"item_id": id, "speed": map[string]any{"num": e.speed.Num, "den": e.speed.Den}}

		if len(e.curve) > 0 {
			pts := make([]any, len(e.curve))
			for i, p := range e.curve {
				pts[i] = map[string]any{"t": p.T, "speed": map[string]any{"num": p.Speed.Num, "den": p.Speed.Den}}
			}

			args = map[string]any{"item_id": id, "curve": pts}
		}

		b.push("clip.set_speed", args)

		if trim {
			b.push("item.trim", map[string]any{"item_id": id, "edge": "end", "to": frameTime(e.start + e.duration)})
		}
	}

	// Reverse comes last: trimming the end of a reversed clip moves its source_in.
	if e.reverse {
		b.push("clip.set_speed", map[string]any{"item_id": id, "reverse": true})
	}

	for _, m := range e.markers {
		args := map[string]any{"item_id": id, "at": frameTime(e.start + m.Frame)}
		if m.Label != "" {
			args["label"] = m.Label
		}

		b.push("marker.add", args)
	}

	b.after(id, e, asset)

	return &placed{id: id, visual: !audio}
}

// text adds one text item and what its metadata asks for.
func (b *batch) text(trackID string, e *entry) *placed {
	id := b.newID(timeline.PrefixItem)
	args := map[string]any{
		"track_id": trackID, "id": id, "start": frameTime(e.start), "duration": frameTime(e.duration), "text": e.body,
	}

	if len(e.style) > 0 {
		args["style"] = b.knownStyle(e.style, e.name)
	}

	b.push("text.add", args)
	b.after(id, e, "")

	return &placed{id: id, visual: true}
}

// shape adds one shape item and what its metadata asks for.
func (b *batch) shape(trackID string, e *entry) *placed {
	id := b.newID(timeline.PrefixItem)
	args := map[string]any{
		"track_id": trackID, "id": id, "start": frameTime(e.start), "duration": frameTime(e.duration),
	}
	maps.Copy(args, e.shape)

	b.push("shape.add", args)
	b.after(id, e, "")

	return &placed{id: id, visual: true}
}

// caption adds one caption item, trims it to the length of the item in the file when the words end
// before it, and sets its line breaks and style override.
func (b *batch) caption(trackID string, e *entry) *placed {
	id := b.newID(timeline.PrefixItem)
	words := make([]any, len(e.caption.words))

	for i, w := range e.caption.words {
		words[i] = w
	}

	b.push("caption.add", map[string]any{
		"track_id": trackID, "id": id, "start": frameTime(e.start), "words": words,
	})

	if e.caption.last < e.duration {
		b.push("item.trim", map[string]any{"item_id": id, "edge": "end", "to": frameTime(e.start + e.duration)})
	}

	if len(e.caption.breaks) > 0 {
		b.push("caption.set_breaks", map[string]any{"item_id": id, "breaks": e.caption.breaks})
	}

	if len(e.caption.style) > 0 {
		b.push("caption.set_style", map[string]any{
			"item_id": id, "style": b.knownStyle(e.caption.style, e.name), "replace": true,
		})
	}

	b.after(id, e, "")

	return &placed{id: id, visual: true}
}

// after pushes the name, the properties and the slot of an item that was just added. A filled
// slot is filled again with what the item holds, unless that would move other items.
func (b *batch) after(id string, e *entry, asset string) {
	if e.name != "" {
		b.push("item.rename", map[string]any{"item_id": id, "name": e.name})
	}

	for _, p := range e.props {
		args := map[string]any{"item_id": id}
		maps.Copy(args, p.args)
		b.push(p.op, args)
	}

	s := e.slot
	if s == nil {
		return
	}

	mark := map[string]any{"item_id": id, "name": s.name}
	if s.description != "" {
		mark["description"] = s.description
	}

	if s.notRequired {
		mark["required"] = false
	}

	if e.clip && s.policy != "fit" {
		mark["duration_policy"] = s.policy
	}

	b.push("slot.mark", mark)

	switch {
	case !s.filled:
	case e.text:
		b.push("slot.fill", map[string]any{"name": s.name, "text": e.body})
	case s.policy == "fit":
		b.push("slot.fill", map[string]any{"name": s.name, "asset_id": asset, "source_in": frameTime(e.source)})
	default:
		b.lost = append(b.lost, Lost{Name: e.name, Feature: "filled state of slot " + s.name})
	}
}

// sourceLength is the length in frames a clip is added with so that set_speed (which rounds the
// new duration up) gives duration; trim is true when no length does and the clip must be trimmed.
func sourceLength(duration int, speed timeline.Rational) (length int, trim bool) {
	n, d := int64(speed.Num), int64(speed.Den)
	ceilDur := func(l int64) int64 { return (l*d + n - 1) / n }

	if l := (int64(duration)-1)*n/d + 1; ceilDur(l) == int64(duration) {
		return int(l), false
	}

	l := max((int64(duration)*n+d/2)/d, 1)

	return int(l), ceilDur(l) != int64(duration)
}

// transition sets a fade on the clip at idx, when there is one.
func (b *batch) transition(ids []*placed, idx int, edge string, dur int) {
	if dur < 1 || idx < 0 || idx >= len(ids) || ids[idx] == nil {
		return
	}

	if !ids[idx].visual {
		b.lost = append(b.lost, Lost{Feature: "transition on an audio clip"})

		return
	}

	b.pending = append(b.pending, command.Operation{Op: "item.set_transition", Args: mustJSON(map[string]any{
		"item_id": ids[idx].id, "edge": edge, "kind": "fade", "duration": frameTime(dur),
	})})
}

func frameTime(n int) string { return fmt.Sprintf("%df", n) }

func mustJSON(v any) json.RawMessage {
	raw, err := json.Marshal(v)
	if err != nil {
		panic("otio: " + err.Error())
	}

	return raw
}

// clipRunes cuts s to at most n characters, the length limits of the names in
// SDD-001 (track 60, item 120).
func clipRunes(s string, n int) string {
	if r := []rune(s); len(r) > n {
		return string(r[:n])
	}

	return s
}
