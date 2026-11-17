package otio

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"

	"github.com/AlexGalhardo/vukem-video/core/timeline"
)

type object = map[string]any

func rt(frames int, rate float64) object {
	return object{"OTIO_SCHEMA": schemaRational, "rate": rate, "value": frames}
}

func tr(start, dur int, rate float64) object {
	return object{"OTIO_SCHEMA": schemaTimeRange, "start_time": rt(start, rate), "duration": rt(dur, rate)}
}

// Export writes doc as an OpenTimelineIO file and reports what it could not carry.
// The output is deterministic: object keys are sorted. Tracks are written bottom
// first (OTIO stack order); a text item is a gap whose metadata holds it, caption and shape
// items are not written, and a gap keeps their time.
// A clip refers to its media by the asset name, as a relative target_url.
func Export(doc *timeline.Document) ([]byte, []Lost, error) {
	rate := fpsRate(doc.Settings.FPS)

	var lost []Lost

	if doc.Template != nil {
		lost = append(lost, Lost{Name: doc.Name, Feature: "template information of the project"})
	}

	ids := make([]string, 0, len(doc.Tracks))
	for id := range doc.Tracks {
		ids = append(ids, id)
	}

	sort.Slice(ids, func(i, j int) bool { return doc.Tracks[ids[i]].Order > doc.Tracks[ids[j]].Order })

	byTrack := map[string][]timeline.Item{}
	for k := range doc.Items {
		it := doc.Items[k]
		byTrack[it.Track] = append(byTrack[it.Track], it)
	}

	children := make([]any, 0, len(ids))

	for _, id := range ids {
		t := doc.Tracks[id]
		for _, f := range []struct {
			on      bool
			feature string
		}{{t.Hidden, "hidden track"}, {t.Muted, "muted track"}, {t.Locked, "locked track"}, {t.Solo, "solo track"}, {t.Magnet, "track magnet"}, {t.Volume != nil, "track volume"}, {t.CaptionStyle != nil, "caption style"}} {
			if f.on {
				lost = append(lost, Lost{ID: id, Name: t.Name, Feature: f.feature})
			}
		}

		items := byTrack[id]
		sort.Slice(items, func(i, j int) bool { return items[i].Start < items[j].Start })

		kids, itemLost := trackChildren(doc, items, rate)
		lost = append(lost, itemLost...)

		kind := kindVid
		if t.Kind == timeline.TrackAudio {
			kind = kindAud
		}

		children = append(children, object{
			"OTIO_SCHEMA": schemaTrack, "name": t.Name, "kind": kind, "children": kids, "metadata": trackMetadata(&t),
		})
	}

	stack := object{"OTIO_SCHEMA": schemaStack, "name": "tracks", "children": children, "metadata": object{}}
	if len(doc.Markers) > 0 {
		stack["markers"] = timelineMarkers(doc.Markers, rate)
	}

	root := object{
		"OTIO_SCHEMA": schemaTimeline, "name": doc.Name, "metadata": timelineMetadata(doc), "tracks": stack,
	}

	var buf bytes.Buffer

	enc := json.NewEncoder(&buf)
	enc.SetIndent("", "  ")

	if err := enc.Encode(root); err != nil {
		return nil, nil, fmt.Errorf("otio: %w", err)
	}

	return buf.Bytes(), lost, nil
}

// trackChildren writes the items of one track, sorted by start, as OTIO children and reports what
// they lose.
func trackChildren(doc *timeline.Document, items []timeline.Item, rate float64) ([]any, []Lost) {
	var lost []Lost

	kids := []any{}
	cursor := 0

	for i := range items {
		it := &items[i]
		if l := unsupported(it); l != "" {
			lost = append(lost, Lost{ID: it.ID, Name: it.Name, Feature: l})

			meta := gapMetadata(it)
			if meta == nil {
				continue
			}

			// A text, caption or shape item is a gap that carries the item in the vukem metadata: other
			// tools see the time, Vukem gets the item back.
			if it.Start > cursor {
				kids = append(kids, gap(it.Start-cursor, "", nil, rate))
			}

			lost = append(lost, lostTextProperties(it)...)
			kids = append(kids, gap(it.Duration, it.Name, meta, rate))
			cursor = it.Start + it.Duration

			continue
		}

		a, ok := doc.Assets[it.Asset]
		if !ok {
			lost = append(lost, Lost{ID: it.ID, Name: it.Name, Feature: "placeholder or missing asset"})

			continue
		}

		if it.Start > cursor {
			kids = append(kids, gap(it.Start-cursor, "", nil, rate))
		}

		lost = append(lost, lostProperties(it)...)

		if d, ok := fade(it.TransitionIn); ok {
			kids = append(kids, transition(0, d, rate))
		}

		kids = append(kids, clip(it, &a, rate))

		if d, ok := fade(it.TransitionOut); ok {
			kids = append(kids, transition(d, 0, rate))
		}

		cursor = it.Start + it.Duration
	}

	return kids, lost
}

func gap(dur int, name string, meta object, rate float64) object {
	g := object{"OTIO_SCHEMA": schemaGap, "name": name, "source_range": tr(0, dur, rate)}
	if meta != nil {
		g["metadata"] = meta
	}

	return g
}

func clip(it *timeline.Item, a *timeline.Asset, rate float64) object {
	src := it.SourceIn
	if a.Kind == timeline.AssetImage {
		src = 0
	}

	c := object{
		"OTIO_SCHEMA": schemaClip, "name": it.Name, "metadata": clipMetadata(it), "effects": []any{},
		"source_range": tr(src, it.Duration, rate),
		"media_reference": object{
			"OTIO_SCHEMA": schemaExternal, "name": "", "target_url": a.Name, "metadata": object{},
		},
	}

	markers := make([]any, 0, len(it.Markers))
	for _, m := range it.Markers {
		markers = append(markers, object{
			"OTIO_SCHEMA": schemaMarker, "name": m.Label, "color": "GREEN", "metadata": object{},
			"marked_range": tr(src+m.Frame, 0, rate),
		})
	}

	c["markers"] = markers

	return c
}

// timelineMetadata keeps the markers of the document, in frames from the start of the timeline,
// in the vukem namespace. The same markers are also written as the native markers of the stack, so
// that other tools see them; Import reads the vukem ones first because they are exact.
func timelineMetadata(doc *timeline.Document) object {
	if len(doc.Markers) == 0 {
		return object{}
	}

	list := make([]any, 0, len(doc.Markers))
	for _, m := range doc.Markers {
		e := object{"frame": m.Frame}
		if m.Label != "" {
			e["label"] = m.Label
		}

		list = append(list, e)
	}

	return object{metaNamespace: object{"markers": list}}
}

func timelineMarkers(marks []timeline.Marker, rate float64) []any {
	out := make([]any, 0, len(marks))
	for _, m := range marks {
		out = append(out, object{
			"OTIO_SCHEMA": schemaMarker, "name": m.Label, "color": "GREEN", "metadata": object{},
			"marked_range": tr(m.Frame, 0, rate),
		})
	}

	return out
}

// trackMetadata keeps what OTIO has no field for in the vukem namespace, which other tools
// ignore and Import reads back: hidden, muted, magnet, locked, solo and volume. Ids are not kept: an import makes new ones.
func trackMetadata(t *timeline.Track) object {
	v := object{}
	if t.Hidden {
		v["hidden"] = true
	}

	if t.Muted {
		v["muted"] = true
	}

	if t.Magnet {
		v["magnet"] = true
	}

	if t.Locked {
		v["locked"] = true
	}

	if t.Solo {
		v["solo"] = true
	}

	if t.Volume != nil {
		v["volume"] = *t.Volume
	}

	if m := timeline.StyleToMap(t.CaptionStyle); len(m) > 0 {
		v["caption_style"] = m
	}

	if len(v) == 0 {
		return object{}
	}

	return object{metaNamespace: v}
}

// clipMetadata keeps the properties of a clip that OTIO has no field for, in the vukem
// namespace, under the names and values of the document. Only values that differ from the
// default are written. Import turns them back into commands.
// playbackMetadata writes the speed, the speed curve, reverse and the effects of an item.
func playbackMetadata(it *timeline.Item, props object) {
	if it.Speed.Den != 0 && it.Speed.Num != it.Speed.Den {
		props["speed"] = object{"num": it.Speed.Num, "den": it.Speed.Den}
	}

	if it.Reverse {
		props["reverse"] = true
	}

	if it.Freeze {
		props["freeze"] = true
	}

	if len(it.Hsl) > 0 || it.Wheels != nil || it.Curves != nil {
		gradeProps(it, props)
	}

	if l := it.Lut; l != nil {
		props["lut"] = object{"asset": l.Asset, "intensity": l.Intensity}
	}

	if len(it.Effects) > 0 {
		list := make([]any, len(it.Effects))

		for i, e := range it.Effects {
			o := object{"kind": e.Kind}

			for _, name := range timeline.EffectParams[e.Kind] {
				if v, ok := e.Param(name); ok {
					o[name] = v
				}
			}

			list[i] = o
		}

		props["effects"] = list
	}

	if len(it.SpeedCurve) > 0 {
		pts := make([]any, len(it.SpeedCurve))
		for i, p := range it.SpeedCurve {
			pts[i] = object{"t": p.T, "speed": object{"num": p.Speed.Num, "den": p.Speed.Den}}
		}

		props["speed_curve"] = pts
	}
}

func clipMetadata(it *timeline.Item) object {
	props := object{}
	num := func(name string, v, def float64) {
		if v != def {
			props[name] = v
		}
	}

	playbackMetadata(it, props)

	if k := it.ChromaKey; k != nil {
		props["chroma_key"] = object{
			"color":      string(k.Color),
			"similarity": k.Similarity,
			"smoothness": k.Smoothness,
			"spill":      k.Spill,
		}
	}

	sharedProps(it, props)

	if it.Blend != "" {
		props["blend"] = it.Blend
	}

	maskProps(it, props)

	if it.FlipX {
		props["flip_x"] = true
	}

	if it.FlipY {
		props["flip_y"] = true
	}

	if it.X != nil {
		props["x"] = *it.X
	}

	if it.Y != nil {
		props["y"] = *it.Y
	}

	num("scale", it.Scale, 1)
	num("rotation", it.Rotation, 0)
	num("opacity", it.Opacity, 1)
	num("volume", it.Volume, 1)
	num("brightness", it.Brightness, 0)
	num("contrast", it.Contrast, 1)
	num("saturation", it.Saturation, 1)
	num("temperature", it.Temperature, 0)
	num("tint", it.Tint, 0)
	num("hue", it.Hue, 0)
	num("blur", it.Blur, 0)
	num("denoise", it.Denoise, 0)

	if it.Voice != "" {
		props["voice"] = it.Voice
	}

	if it.AudioMuted {
		props["audio_muted"] = true
	}

	if it.FadeIn != 0 {
		props["fade_in"] = it.FadeIn
	}

	if it.FadeOut != 0 {
		props["fade_out"] = it.FadeOut
	}

	if it.Fit != "" && it.Fit != "cover" {
		props["fit"] = it.Fit
	}

	if c := it.Crop; c != nil {
		props["crop"] = object{"left": c.Left, "top": c.Top, "right": c.Right, "bottom": c.Bottom}
	}

	if it.Filter != "" && it.Filter != "none" {
		props["filter"] = it.Filter
		props["filter_amount"] = it.FilterAmount
	}

	transitionProps(it, props, true)

	if len(props) == 0 {
		return object{}
	}

	return object{metaNamespace: object{"properties": props}}
}

// maskProps keeps the mask of an item in its vukem properties.
func maskProps(it *timeline.Item, props object) {
	m := it.Mask
	if m == nil {
		return
	}

	mask := object{
		"shape": m.Shape, "x": m.X, "y": m.Y, "width": m.Width, "height": m.Height, "feather": m.Feather,
	}
	if m.Invert {
		mask["invert"] = true
	}

	props["mask"] = mask
}

// sharedProps writes the properties that clips and text items keep: keyframes and the slot.
func sharedProps(it *timeline.Item, props object) {
	if it.Disabled {
		props["disabled"] = true
	}

	if len(it.Keyframes) > 0 {
		kfs := object{}

		for name, list := range it.Keyframes {
			out := make([]any, 0, len(list))
			for _, k := range list {
				out = append(out, object{"t": k.T, "value": k.Value, "easing": k.Easing})
			}

			kfs[name] = out
		}

		props["keyframes"] = kfs
	}

	if s := it.Slot; s != nil {
		slot := object{"name": s.Name}
		if s.Description != "" {
			slot["description"] = s.Description
		}

		if !s.Required {
			slot["required"] = false
		}

		if s.DurationPolicy != "fit" {
			slot["duration_policy"] = s.DurationPolicy
		}

		if s.Filled {
			slot["filled"] = true
		}

		props["slot"] = slot
	}
}

// textMetadata keeps a text item in the vukem namespace of the gap that stands for it: the text,
// its style and the properties of a text item.
func textMetadata(it *timeline.Item) object {
	v := object{"text": it.Text}
	if m := timeline.StyleToMap(it.Style); len(m) > 0 {
		v["style"] = m
	}

	props := object{}
	if it.X != nil {
		props["x"] = *it.X
	}

	if it.Y != nil {
		props["y"] = *it.Y
	}

	for _, f := range []struct {
		name     string
		val, def float64
	}{{"scale", it.Scale, 1}, {"rotation", it.Rotation, 0}, {"opacity", it.Opacity, 1}} {
		if f.val != f.def {
			props[f.name] = f.val
		}
	}

	sharedProps(it, props)

	if it.Blend != "" {
		props["blend"] = it.Blend
	}

	maskProps(it, props)

	if it.FlipX {
		props["flip_x"] = true
	}

	if it.FlipY {
		props["flip_y"] = true
	}

	transitionProps(it, props, false)
	visualProps(it, props)

	if len(props) > 0 {
		v["properties"] = props
	}

	return object{metaNamespace: v}
}

// gapMetadata is the vukem metadata of the gap that stands for a text, caption or shape item, nil
// for any other type.
func gapMetadata(it *timeline.Item) object {
	switch it.Type {
	case timeline.ItemText:
		return textMetadata(it)
	case timeline.ItemCaption:
		return captionMetadata(it)
	case timeline.ItemShape:
		return shapeMetadata(it)
	case timeline.ItemClip, timeline.ItemAudio:
	}

	return nil
}

// lostTextProperties lists what a text, caption or shape item has that its metadata does not carry.
func lostTextProperties(it *timeline.Item) []Lost {
	var out []Lost

	for _, f := range []struct {
		on      bool
		feature string
	}{
		{len(it.Markers) > 0, "markers of a " + string(it.Type) + " item"},
		{it.Link != "", "link of a " + string(it.Type) + " item"},
		{it.LinkedItem != "", "caption link to a clip"},
	} {
		if f.on {
			out = append(out, Lost{ID: it.ID, Name: it.Name, Feature: f.feature})
		}
	}

	return out
}

func transition(in, out int, rate float64) object {
	return object{
		"OTIO_SCHEMA": schemaTransition, "name": "", "transition_type": dissolve, "metadata": object{},
		"in_offset": rt(in, rate), "out_offset": rt(out, rate),
	}
}

func fade(t *timeline.Transition) (int, bool) {
	if t == nil || t.Kind != "fade" {
		return 0, false
	}

	return t.Duration, true
}

// unsupported names why an item cannot be written at all, or "".
func unsupported(it *timeline.Item) string {
	switch it.Type {
	case timeline.ItemText, timeline.ItemCaption, timeline.ItemShape:
		return string(it.Type) + " item"
	case timeline.ItemClip, timeline.ItemAudio:
		return ""
	}

	return "item type"
}

// lostProperties lists the properties of a written clip that OTIO does not carry.
func lostProperties(it *timeline.Item) []Lost {
	var out []Lost

	add := func(feature string) { out = append(out, Lost{ID: it.ID, Name: it.Name, Feature: feature}) }

	for _, f := range []struct {
		on      bool
		feature string
	}{
		{len(it.Keyframes) > 0, "keyframes"},
		{it.Speed.Num != it.Speed.Den || len(it.SpeedCurve) > 0, "speed"},
		{it.Reverse, "reverse"},
		{len(it.Effects) > 0, "effects"},
		{it.Link != "", "link"},
		{it.Brightness != 0 || it.Contrast != 1 || it.Saturation != 1 || it.Temperature != 0 || it.Tint != 0 || it.Hue != 0, "colour adjustment"},
		{(it.Filter != "none" && it.Filter != "") || it.Blur != 0, "filter and blur"},
		{it.ChromaKey != nil, "chroma key"},
		{it.Shadow != nil, "drop shadow"},
		{it.Outline != nil, "outline"},
		{it.Lut != nil, "lookup table"},
		{len(it.Hsl) > 0 || it.Wheels != nil || it.Curves != nil, "advanced colour"},
		{it.Freeze, "freeze frame"},
		{it.Crop != nil || it.Fit != "cover", "framing"},
		{it.X != nil || it.Y != nil || it.Scale != 1 || it.Rotation != 0 || it.Opacity != 1, "transform and opacity"},
		{it.Volume != 1 || it.AudioMuted || it.FadeIn != 0 || it.FadeOut != 0 || it.Denoise != 0 || it.Voice != "", "audio settings"},
		{it.Slot != nil, "template slot"},
	} {
		if f.on {
			add(f.feature)
		}
	}

	for _, t := range []*timeline.Transition{it.TransitionIn, it.TransitionOut} {
		if t != nil && t.Kind != "fade" {
			add("transition " + t.Kind)
		}
	}

	return out
}

// gradeProps keeps the HSL bands, the wheels and the curves of a clip as the vukem properties hsl,
// wheels and curves, in the shape of the document.
func gradeProps(it *timeline.Item, props object) {
	if len(it.Hsl) > 0 {
		list := make([]any, len(it.Hsl))
		for i, b := range it.Hsl {
			list[i] = object{"range": b.Range, "hue": b.Hue, "saturation": b.Saturation, "lightness": b.Lightness}
		}

		props["hsl"] = list
	}

	if w := it.Wheels; w != nil {
		out := object{}

		for name, wh := range map[string]*timeline.Wheel{"shadows": w.Shadows, "midtones": w.Midtones, "highlights": w.Highlights} {
			if wh != nil {
				out[name] = object{"hue": wh.Hue, "amount": wh.Amount, "luma": wh.Luma}
			}
		}

		props["wheels"] = out
	}

	if c := it.Curves; c != nil {
		out := object{}

		for _, ch := range c.Channels() {
			if len(ch.Points) == 0 {
				continue
			}

			pts := make([]any, len(ch.Points))
			for i, p := range ch.Points {
				pts[i] = object{"x": p.X, "y": p.Y}
			}

			out[ch.Name] = pts
		}

		props["curves"] = out
	}
}
