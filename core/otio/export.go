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
// first (OTIO stack order); text, caption and shape items, keyframes, colour and
// the other properties of the report are not written, and a gap keeps their time.
// A clip refers to its media by the asset name, as a relative target_url.
func Export(doc *timeline.Document) ([]byte, []Lost, error) {
	rate := fpsRate(doc.Settings.FPS)

	var lost []Lost

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
		}{{t.Hidden, "hidden track"}, {t.Muted, "muted track"}, {t.CaptionStyle != nil, "caption style"}} {
			if f.on {
				lost = append(lost, Lost{ID: id, Name: t.Name, Feature: f.feature})
			}
		}

		items := byTrack[id]
		sort.Slice(items, func(i, j int) bool { return items[i].Start < items[j].Start })

		kids := []any{}
		cursor := 0

		for i := range items {
			it := &items[i]
			if l := unsupported(it); l != "" {
				lost = append(lost, Lost{ID: it.ID, Name: it.Name, Feature: l})

				continue
			}

			a, ok := doc.Assets[it.Asset]
			if !ok {
				lost = append(lost, Lost{ID: it.ID, Name: it.Name, Feature: "placeholder or missing asset"})

				continue
			}

			if it.Start > cursor {
				kids = append(
					kids,
					object{"OTIO_SCHEMA": schemaGap, "name": "", "source_range": tr(0, it.Start-cursor, rate)},
				)
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

		kind := kindVid
		if t.Kind == timeline.TrackAudio {
			kind = kindAud
		}

		children = append(children, object{
			"OTIO_SCHEMA": schemaTrack, "name": t.Name, "kind": kind, "children": kids, "metadata": object{},
		})
	}

	root := object{
		"OTIO_SCHEMA": schemaTimeline, "name": doc.Name, "metadata": object{},
		"tracks": object{"OTIO_SCHEMA": schemaStack, "name": "tracks", "children": children, "metadata": object{}},
	}

	var buf bytes.Buffer

	enc := json.NewEncoder(&buf)
	enc.SetIndent("", "  ")

	if err := enc.Encode(root); err != nil {
		return nil, nil, fmt.Errorf("otio: %w", err)
	}

	return buf.Bytes(), lost, nil
}

func clip(it *timeline.Item, a *timeline.Asset, rate float64) object {
	src := it.SourceIn
	if a.Kind == timeline.AssetImage {
		src = 0
	}

	c := object{
		"OTIO_SCHEMA": schemaClip, "name": it.Name, "metadata": object{}, "effects": []any{},
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
		{it.Speed.Num != it.Speed.Den, "speed"},
		{it.Brightness != 0 || it.Contrast != 1 || it.Saturation != 1, "colour adjustment"},
		{(it.Filter != "none" && it.Filter != "") || it.Blur != 0, "filter and blur"},
		{it.Crop != nil || it.Fit != "cover", "framing"},
		{it.X != nil || it.Y != nil || it.Scale != 1 || it.Rotation != 0 || it.Opacity != 1, "transform and opacity"},
		{it.Volume != 1 || it.AudioMuted || it.FadeIn != 0 || it.FadeOut != 0, "audio settings"},
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
