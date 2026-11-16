package otio

import (
	"math"
	"slices"
)

// The properties of a clip that Export keeps in the vukem metadata and Import turns back
// into commands (SDD-001 section 9). Metadata comes from a file nobody vouches for, so each
// value is checked against the range of its command here and a bad one is reported as lost,
// never allowed to fail the whole batch.

// propOp is one command that restores properties of a clip, without its item_id.
type propOp struct {
	op   string
	args map[string]any
}

type propReader struct {
	props map[string]any
	lose  func(feature string)
	bad   bool
}

// vukemMeta returns the vukem object of the metadata of an OTIO object, or nil.
func vukemMeta(m map[string]any) map[string]any {
	md, _ := m["metadata"].(map[string]any)
	v, _ := md[metaNamespace].(map[string]any)

	return v
}

// number reads a finite number within [lo, hi] (lo excluded when open is true); a present
// value that is not one is lost.
func (p *propReader) number(name string, lo, hi float64, open bool) (float64, bool) {
	raw, present := p.props[name]
	if !present {
		return 0, false
	}

	v, ok := raw.(float64)
	if !ok || math.IsNaN(v) || math.IsInf(v, 0) || v > hi || v < lo || (open && v == lo) {
		p.lose("vukem property " + name + " is not a valid value")
		p.bad = true

		return 0, false
	}

	return v, true
}

func (p *propReader) frames(name string) (int, bool) {
	v, ok := p.number(name, 0, 1e9, false)
	if !ok || v != math.Trunc(v) {
		if ok {
			p.lose("vukem property " + name + " is not a whole number of frames")
		}

		return 0, false
	}

	return int(v), true
}

func (p *propReader) text(name string, allowed ...string) (string, bool) {
	raw, present := p.props[name]
	if !present {
		return "", false
	}

	s, ok := raw.(string)
	if !ok || !slices.Contains(allowed, s) {
		p.lose("vukem property " + name + " is not a valid value")

		return "", false
	}

	return s, true
}

// clipOps reads the properties of a clip into the commands that restore them. audio says
// the clip is an audio item, which keeps volume and fades only.
func clipOps(props map[string]any, audio bool, lose func(string)) []propOp {
	p := &propReader{props: props, lose: lose}

	var ops []propOp

	add := func(op string, args map[string]any) {
		if len(args) > 0 {
			ops = append(ops, propOp{op: op, args: args})
		}
	}

	set := func(args map[string]any, name string, lo, hi float64, open bool) {
		if v, ok := p.number(name, lo, hi, open); ok {
			args[name] = v
		}
	}

	if !audio {
		transform := map[string]any{}
		set(transform, "x", -1e9, 1e9, false)
		set(transform, "y", -1e9, 1e9, false)
		set(transform, "scale", 0, 20, true)
		set(transform, "rotation", -1e9, 1e9, false)
		set(transform, "opacity", 0, 1, false)
		add("item.set_transform", transform)

		colour := map[string]any{}
		set(colour, "brightness", -1, 1, false)
		set(colour, "contrast", 0, 2, false)
		set(colour, "saturation", 0, 2, false)
		add("clip.set_color", colour)

		add("clip.set_framing", framing(p))

		filter := map[string]any{}
		if f, ok := p.text("filter", "grayscale", "sepia", "invert", "warm", "cool"); ok {
			filter["filter"] = f
		}

		if a, ok := p.number("filter_amount", 0, 1, false); ok {
			filter["amount"] = a
		}

		add("clip.set_filter", filter)

		if r, ok := p.number("blur", 0, 100, false); ok {
			add("clip.set_blur", map[string]any{"radius": r})
		}
	}

	sound := map[string]any{}
	set(sound, "volume", 0, 4, false)

	if f, ok := p.frames("fade_in"); ok {
		sound["fade_in"] = frameTime(f)
	}

	if f, ok := p.frames("fade_out"); ok {
		sound["fade_out"] = frameTime(f)
	}

	if m, _ := props["audio_muted"].(bool); m && !audio {
		sound["audio_muted"] = true
	}

	add("item.set_audio", sound)

	return ops
}

func framing(p *propReader) map[string]any {
	args := map[string]any{}

	if f, ok := p.text("fit", "contain", "none"); ok {
		args["fit"] = f
	}

	if raw, present := p.props["crop"]; present {
		sides, _ := raw.(map[string]any)
		crop := map[string]any{}
		sub := &propReader{props: sides, lose: p.lose}

		for _, side := range []string{"left", "top", "right", "bottom"} {
			if v, ok := sub.number(side, 0, 1, false); ok {
				crop[side] = v
			}
		}

		if sides == nil || sub.bad {
			p.lose("vukem property crop is not a valid value")
		} else if len(crop) > 0 {
			args["crop"] = crop
		}
	}

	return args
}
