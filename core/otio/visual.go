package otio

import (
	"math"
	"slices"
	"unicode/utf8"

	"github.com/AlexGalhardo/stuarty/core/timeline"
)

// The shape, caption and transition metadata of VV-025: what clips and text items already keep
// in the vukem namespace, for the other visual items.

var transitionKinds = []string{
	"fade",
	"slide_left",
	"slide_right",
	"slide_up",
	"slide_down",
	"zoom",
	"whip",
	"dip",
	"wipe",
	"spin",
	"shrink",
	"grow",
}

// transitionProps keeps the transitions of an item in its vukem properties. A clip keeps its fades
// as the native Transition objects of OTIO and so writes only the other kinds here; every other
// visual item has no native form and writes all of them.
func transitionProps(it *timeline.Item, props object, nativeFade bool) {
	for _, t := range []struct {
		name string
		val  *timeline.Transition
	}{{"transition_in", it.TransitionIn}, {"transition_out", it.TransitionOut}} {
		if t.val == nil || nativeFade && t.val.Kind == "fade" {
			continue
		}

		o := object{"kind": t.val.Kind, "duration": t.val.Duration}
		if t.val.Color != "" {
			o["color"] = string(t.val.Color)
		}

		if t.val.Direction != "" {
			o["direction"] = t.val.Direction
		}

		props[t.name] = o
	}
}

// visualProps keeps the blur and the filter of a text, caption or shape item (a clip keeps its own).
func visualProps(it *timeline.Item, props object) {
	if it.Blur != 0 {
		props["blur"] = it.Blur
	}

	if it.Filter != "" && it.Filter != "none" {
		props["filter"] = it.Filter
		props["filter_amount"] = it.FilterAmount
	}
}

// shapeMetadata keeps a shape item in the vukem namespace of the gap that stands for it.
func shapeMetadata(it *timeline.Item) object {
	sh := object{"shape": it.Shape, "width": it.Width, "height": it.Height}
	if it.Radius != 0 {
		sh["radius"] = it.Radius
	}

	if it.Fill != "" {
		sh["fill"] = string(it.Fill)
	}

	if it.StrokeColor != "" {
		sh["stroke_color"] = string(it.StrokeColor)
	}

	if it.StrokeWidth != 0 {
		sh["stroke_width"] = it.StrokeWidth
	}

	return object{metaNamespace: object{"shape": sh, "properties": visualItemProps(it)}}
}

// captionMetadata keeps a caption item: its words, line breaks, style override and the clip it follows.
func captionMetadata(it *timeline.Item) object {
	words := make([]any, len(it.Words))
	for i, w := range it.Words {
		words[i] = object{"text": w.Text, "start": w.Start, "duration": w.Duration}
	}

	c := object{"words": words}
	if len(it.Breaks) > 0 {
		c["breaks"] = it.Breaks
	}

	if m := timeline.StyleToMap(it.Style); len(m) > 0 {
		c["style"] = m
	}

	return object{metaNamespace: object{"caption": c, "properties": visualItemProps(it)}}
}

// visualItemProps are the properties a text-like item keeps: position, scale, rotation, opacity,
// the shared ones, blend, mask, flip, transitions, blur and filter.
func visualItemProps(it *timeline.Item) object {
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

	return props
}

// transitionOps reads transition_in and transition_out into item.set_transition commands.
func transitionOps(p *propReader, dur int) []propOp {
	var out []propOp

	for _, e := range []struct{ name, edge string }{{"transition_in", "in"}, {"transition_out", "out"}} {
		raw, present := p.props[e.name]
		if !present {
			continue
		}

		obj, _ := raw.(map[string]any)
		kind, _ := obj["kind"].(string)
		sub := &propReader{props: obj, lose: func(string) {}}
		d, ok := sub.number("duration", 1, float64(dur), false)

		if !slices.Contains(transitionKinds, kind) || !ok || d != math.Trunc(d) {
			p.lose("vukem property " + e.name + " is not a valid value")

			continue
		}

		args := map[string]any{"edge": e.edge, "kind": kind, "duration": frameTime(int(d))}

		if c, ok := obj["color"].(string); ok && hexColourRe.MatchString(c) && kind == "dip" {
			args["color"] = c
		}

		if dir, ok := sub.text("direction", "left", "right", "up", "down"); ok && (kind == "whip" || kind == "wipe") {
			args["direction"] = dir
		}

		out = append(out, propOp{op: "item.set_transition", args: args})
	}

	return out
}

// shapeSpec reads the shape object of a gap's metadata like shape.add checks it; it returns nil when
// the object is not valid.
func shapeSpec(raw any, lose func(string)) map[string]any {
	obj, _ := raw.(map[string]any)
	sub := &propReader{props: obj, lose: lose}
	args := map[string]any{}

	kind, ok := sub.text("shape", "rect", "rounded_rect", "ellipse")
	w, okW := sub.number("width", 1e-9, 1e6, false)
	h, okH := sub.number("height", 1e-9, 1e6, false)

	if !ok || !okW || !okH {
		lose("vukem property shape is not a valid value")

		return nil
	}

	args["shape"], args["width"], args["height"] = kind, w, h

	for _, name := range []string{"stroke_width", "radius"} {
		if _, has := obj[name]; has {
			if v, ok := sub.number(name, 0, 1e6, false); ok {
				args[name] = v
			} else {
				return nil
			}
		}
	}

	for _, name := range []string{"fill", "stroke_color"} {
		if raw, has := obj[name]; has {
			s, _ := raw.(string)
			if !hexColourRe.MatchString(s) {
				lose("vukem property shape " + name + " is not a valid value")

				return nil
			}

			args[name] = s
		}
	}

	return args
}

// captionSpec is a caption of the metadata: words with frames relative to the item, the line breaks
// and the style override.
type captionSpec struct {
	words  []map[string]any
	breaks []int
	style  map[string]any
	last   int // the frame after the last word
}

// readCaption reads the caption object of a gap's metadata like caption.add checks it.
func readCaption(raw any, dur int, lose func(string)) *captionSpec {
	obj, _ := raw.(map[string]any)
	list, _ := obj["words"].([]any)

	if len(list) < 1 || len(list) > 60 {
		lose("vukem property caption is not a valid value")

		return nil
	}

	spec := &captionSpec{}

	for _, e := range list {
		w, _ := e.(map[string]any)
		text, _ := w["text"].(string)
		sub := &propReader{props: w, lose: func(string) {}}
		start, ok1 := sub.number("start", 0, float64(dur), false)
		length, ok2 := sub.number("duration", 1, float64(dur), false)

		if n := utf8.RuneCountInString(text); n < 1 || n > 80 || !ok1 || !ok2 || start != math.Trunc(start) ||
			length != math.Trunc(length) || int(start+length) > dur {
			lose("vukem property caption is not a valid value")

			return nil
		}

		spec.words = append(spec.words, map[string]any{
			"text": text, "start": frameTime(int(start)), "duration": frameTime(int(length)),
		})
		spec.last = max(spec.last, int(start+length))
	}

	prev := 0

	for _, b := range asList(obj["breaks"]) {
		f, ok := b.(float64)
		if !ok || f != math.Trunc(f) || int(f) <= prev || int(f) >= len(list) {
			lose("vukem property caption breaks is not a valid value")

			spec.breaks = nil

			break
		}

		spec.breaks, prev = append(spec.breaks, int(f)), int(f)
	}

	if raw, present := obj["style"]; present {
		spec.style = textStyle(raw, true, lose)
	}

	return spec
}

func asList(v any) []any {
	l, _ := v.([]any)

	return l
}
