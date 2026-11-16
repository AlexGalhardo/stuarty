package otio

import (
	"math"
	"regexp"
	"slices"
	"sort"
	"unicode/utf8"

	"github.com/AlexGalhardo/vukem-video/core/timeline"
)

// The properties of an item that Export keeps in the vukem metadata and Import turns back
// into commands (SDD-001 section 9). Metadata comes from a file nobody vouches for, so each
// value is checked against the range of its command here and a bad one is reported as lost,
// never allowed to fail the whole batch.

// propOp is one command that restores properties of an item, without its item_id.
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

// itemMode is the kind of item whose properties are read: a clip keeps everything, an audio
// item volume, fades, speed and volume keyframes, a text item transform, keyframes and slot.
type itemMode int

const (
	modeClip itemMode = iota
	modeAudio
	modeText
)

// slotSpec is a template slot to mark and, when it was filled, to fill again.
type slotSpec struct {
	name, description string
	notRequired       bool
	policy            string
	filled            bool
}

// itemSpec is what the properties of an item ask for: the commands that follow its creation,
// the speed that must be set before anything is animated, and its slot.
type itemSpec struct {
	ops   []propOp
	speed *timeline.Rational
	slot  *slotSpec
}

var (
	slotNameRe  = regexp.MustCompile(`^[a-z][a-z0-9_]{0,39}$`)
	hexColourRe = regexp.MustCompile(`^#[0-9A-Fa-f]{6}([0-9A-Fa-f]{2})?$`)
)

const maxKeyframes = 100

// itemOps reads the properties of an item into the commands that restore them. start and dur
// are the frames the item occupies; used holds the slot names already taken in the file.
func itemOps(props map[string]any, mode itemMode, start, dur int, used map[string]bool, lose func(string)) itemSpec {
	p := &propReader{props: props, lose: lose}

	var spec itemSpec

	add := func(op string, args map[string]any) {
		if len(args) > 0 {
			spec.ops = append(spec.ops, propOp{op: op, args: args})
		}
	}

	set := func(args map[string]any, name string, lo, hi float64, open bool) {
		if v, ok := p.number(name, lo, hi, open); ok {
			args[name] = v
		}
	}

	if mode != modeAudio {
		transform := map[string]any{}
		set(transform, "x", -1e9, 1e9, false)
		set(transform, "y", -1e9, 1e9, false)
		set(transform, "scale", 0, 20, true)
		set(transform, "rotation", -1e9, 1e9, false)
		set(transform, "opacity", 0, 1, false)
		add("item.set_transform", transform)

		if b, ok := p.text("blend", "multiply", "screen", "overlay", "add"); ok {
			add("clip.set_blend", map[string]any{"mode": b})
		}

		if m := mask(p); m != nil {
			add("clip.set_mask", map[string]any{"mask": m})
		}

		flip := map[string]any{}
		if f, _ := props["flip_x"].(bool); f {
			flip["horizontal"] = true
		}

		if f, _ := props["flip_y"].(bool); f {
			flip["vertical"] = true
		}

		add("clip.set_flip", flip)
	}

	if mode == modeClip {
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

		add("clip.set_chroma_key", chromaKey(p))
	}

	if mode != modeText {
		sound := map[string]any{}
		set(sound, "volume", 0, 4, false)

		if f, ok := p.frames("fade_in"); ok {
			sound["fade_in"] = frameTime(f)
		}

		if f, ok := p.frames("fade_out"); ok {
			sound["fade_out"] = frameTime(f)
		}

		if m, _ := props["audio_muted"].(bool); m && mode == modeClip {
			sound["audio_muted"] = true
		}

		add("item.set_audio", sound)

		spec.speed = speed(p)
	}

	spec.ops = append(spec.ops, keyframes(p, mode, start, dur)...)
	spec.slot = slot(p, mode, used)

	return spec
}

// chromaKey reads the chroma_key object; all four members are needed.
func chromaKey(p *propReader) map[string]any {
	raw, present := p.props["chroma_key"]
	if !present {
		return nil
	}

	obj, _ := raw.(map[string]any)
	colour, _ := obj["color"].(string)
	sub := &propReader{props: obj, lose: func(string) {}}
	args := map[string]any{"color": colour}

	for _, name := range []string{"similarity", "smoothness", "spill"} {
		v, ok := sub.number(name, 0, 1, false)
		if !ok {
			sub.bad = true
		}

		args[name] = v
	}

	if obj == nil || !hexColourRe.MatchString(colour) || sub.bad {
		p.lose("vukem property chroma_key is not a valid value")

		return nil
	}

	return args
}

// mask reads the mask object; the shape, the four fractions and the feather are checked here, the
// rectangle inside the item by the command.
func mask(p *propReader) map[string]any {
	raw, present := p.props["mask"]
	if !present {
		return nil
	}

	obj, _ := raw.(map[string]any)
	sub := &propReader{props: obj, lose: func(string) {}}
	shape, shapeOK := sub.text("shape", "rect", "ellipse")
	args := map[string]any{"shape": shape}

	for _, f := range []struct {
		name   string
		lo, hi float64
	}{{"x", 0, 1}, {"y", 0, 1}, {"width", 0.001, 1}, {"height", 0.001, 1}} {
		v, ok := sub.number(f.name, f.lo, f.hi, false)
		if !ok {
			sub.bad = true
		}

		args[f.name] = v
	}

	feather, ok := sub.number("feather", 0, timeline.MaxMaskFeather, false)
	if _, given := obj["feather"]; given && (!ok || feather != math.Trunc(feather)) {
		sub.bad = true
	}

	args["feather"] = feather

	if invert, given := obj["invert"]; given {
		b, isBool := invert.(bool)
		if !isBool {
			sub.bad = true
		}

		args["invert"] = b
	}

	if obj == nil || !shapeOK || sub.bad {
		p.lose("vukem property mask is not a valid value")

		return nil
	}

	return args
}

// speed reads {num, den}: whole numbers whose ratio is 1/4 to 4.
func speed(p *propReader) *timeline.Rational {
	raw, present := p.props["speed"]
	if !present {
		return nil
	}

	obj, _ := raw.(map[string]any)
	sub := &propReader{props: obj, lose: func(string) {}}
	num, ok1 := sub.number("num", 1, 1e6, false)
	den, ok2 := sub.number("den", 1, 1e6, false)

	if !ok1 || !ok2 || num != math.Trunc(num) || den != math.Trunc(den) || num*4 > den*16 || num*4 < den {
		p.lose("vukem property speed is not a valid value")

		return nil
	}

	return &timeline.Rational{Num: int(num), Den: int(den)}
}

var keyframeEasings = []string{"linear", "ease_in", "ease_out", "ease_in_out", "hold"}

// keyframeValueOK is the range of a keyframe value per property (keyframe.set).
func keyframeValueOK(prop string, v float64) bool {
	switch {
	case math.IsNaN(v) || math.IsInf(v, 0) || math.Abs(v) > 1e9:
		return false
	case prop == "scale":
		return v > 0 && v <= 20
	case prop == "opacity":
		return v >= 0 && v <= 1
	case prop == "volume":
		return v >= 0 && v <= 4
	}

	return true
}

type keyframeAt struct {
	t      int
	value  float64
	easing string
}

// keyframes turns the keyframes object into keyframe.set commands at absolute frames. A property
// whose first keyframe is after the item start would get an extra keyframe at the start from
// keyframe.set, so that one is removed again.
func keyframes(p *propReader, mode itemMode, start, dur int) []propOp {
	raw, present := p.props["keyframes"]
	if !present {
		return nil
	}

	obj, ok := raw.(map[string]any)
	if !ok {
		p.lose("vukem property keyframes is not a valid value")

		return nil
	}

	names := make([]string, 0, len(obj))
	for name := range obj {
		names = append(names, name)
	}

	sort.Strings(names)

	var ops []propOp

	for _, name := range names {
		list, ok := keyframeList(name, obj[name], mode, dur)
		if !ok {
			p.lose("vukem property keyframes." + name + " is not a valid value")

			continue
		}

		for i, k := range list {
			ops = append(ops, propOp{op: "keyframe.set", args: map[string]any{
				"property": name, "at": frameTime(start + k.t), "value": k.value, "easing": k.easing,
			}})

			if i == 0 && k.t > 0 {
				ops = append(ops, propOp{op: "keyframe.remove", args: map[string]any{
					"property": name, "at": frameTime(start),
				}})
			}
		}
	}

	return ops
}

func keyframeList(name string, raw any, mode itemMode, dur int) ([]keyframeAt, bool) {
	list, _ := raw.([]any)
	if len(list) == 0 || len(list) > maxKeyframes {
		return nil, false
	}

	switch {
	case name == "volume" && mode == modeText, name != "volume" && mode == modeAudio:
		return nil, false
	case !slices.Contains([]string{"x", "y", "scale", "rotation", "opacity", "volume"}, name):
		return nil, false
	}

	out := make([]keyframeAt, 0, len(list))

	for _, e := range list {
		m, _ := e.(map[string]any)
		t, ok1 := m["t"].(float64)
		v, ok2 := m["value"].(float64)
		easing, ok3 := m["easing"].(string)

		if !ok1 || !ok2 || !ok3 || t != math.Trunc(t) || t < 0 || t > float64(dur) ||
			!slices.Contains(keyframeEasings, easing) || !keyframeValueOK(name, v) ||
			(len(out) > 0 && int(t) <= out[len(out)-1].t) {
			return nil, false
		}

		out = append(out, keyframeAt{t: int(t), value: v, easing: easing})
	}

	return out, true
}

// slot reads the slot object of an item; its kind follows from the item type.
func slot(p *propReader, mode itemMode, used map[string]bool) *slotSpec {
	raw, present := p.props["slot"]
	if !present {
		return nil
	}

	obj, _ := raw.(map[string]any)
	name, _ := obj["name"].(string)
	description, _ := obj["description"].(string)
	policy, hasPolicy := obj["duration_policy"].(string)
	required, hasRequired := obj["required"].(bool)
	filled, _ := obj["filled"].(bool)

	if obj == nil || !slotNameRe.MatchString(name) || used[name] || utf8.RuneCountInString(description) > 200 ||
		(hasPolicy && (mode == modeText || !slices.Contains([]string{"fit", "stretch_timeline"}, policy))) ||
		(hasRequired && required) {
		p.lose("vukem property slot is not a valid value")

		return nil
	}

	used[name] = true

	if !hasPolicy {
		policy = "fit"
	}

	return &slotSpec{name: name, description: description, notRequired: hasRequired, policy: policy, filled: filled}
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
