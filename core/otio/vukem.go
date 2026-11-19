package otio

import (
	"encoding/json"
	"maps"
	"math"
	"regexp"
	"slices"
	"sort"
	"unicode/utf8"

	"github.com/AlexGalhardo/stuarty/core/timeline"
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
	ops     []propOp
	speed   *timeline.Rational
	curve   []timeline.SpeedPoint // set with speed, which is then the mean of the curve
	reverse bool
	slot    *slotSpec
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

	if d, _ := props["disabled"].(bool); d {
		spec.ops = append(spec.ops, propOp{op: "item.set_enabled", args: map[string]any{"enabled": false}})
	}

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
		add("clip.set_color", colourArgs(p, props, lose))

		add("clip.set_framing", framing(p))

		add("clip.set_chroma_key", chromaKey(p))
		add("clip.set_effects", effectsArgs(p))
	}

	if mode != modeAudio {
		filterOps(p, add)
		spec.ops = append(spec.ops, transitionOps(p, dur)...)
	}

	if mode != modeText {
		sound := map[string]any{}
		set(sound, "volume", 0, 4, false)
		set(sound, "denoise", 0, 1, false)

		if v, ok := p.text("voice", "clear", "warm", "deep", "bright", "radio"); ok {
			sound["voice"] = v
		}

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

		spec.speed, spec.curve = speedAndCurve(p)
		spec.reverse, _ = props["reverse"].(bool)
	}

	spec.ops = append(spec.ops, keyframes(p, mode, start, dur)...)
	spec.slot = slot(p, mode, used)

	return spec
}

// effectsArgs is the argument of clip.set_effects for the effects of the metadata, empty when none.
func effectsArgs(p *propReader) map[string]any {
	if fx := effects(p); fx != nil {
		return map[string]any{"effects": fx}
	}

	return nil
}

// effects reads the list of effects of a clip with the kinds and ranges of clip.set_effects; one that
// is not valid is dropped with the rest of the list, so that the batch still applies.
func effects(p *propReader) []any {
	raw, present := p.props["effects"]
	if !present {
		return nil
	}

	list, _ := raw.([]any)
	out := make([]any, 0, len(list))

	for _, e := range list {
		obj, _ := e.(map[string]any)
		kind, _ := obj["kind"].(string)

		params, known := timeline.EffectParams[kind]
		if !known || len(obj) > len(params)+1 {
			p.lose("vukem property effects is not a valid value")

			return nil
		}

		args := map[string]any{"kind": kind}
		sub := &propReader{props: obj, lose: func(string) {}}

		for _, name := range params {
			if _, has := obj[name]; !has {
				continue
			}

			lo, hi := timeline.EffectRange(name)

			v, ok := sub.number(name, lo, hi, false)
			if !ok || (name == "size" || name == "speed" || name == "seed") && v != math.Trunc(v) {
				p.lose("vukem property effects is not a valid value")

				return nil
			}

			args[name] = v
		}

		out = append(out, args)
	}

	if len(out) == 0 {
		return nil
	}

	return out
}

// filterOps reads the filter and the blur of any visual item.
func filterOps(p *propReader, add func(string, map[string]any)) {
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

// speedAndCurve reads the constant speed and the curve of an item; with a curve the speed is its mean.
func speedAndCurve(p *propReader) (*timeline.Rational, []timeline.SpeedPoint) {
	if curve := speedCurve(p); curve != nil {
		mean := timeline.CurveMean(curve)

		return &mean, curve
	}

	return speed(p), nil
}

// speedCurve reads the points {t, speed: {num, den}} of a speed curve and checks them with
// timeline.CurveError, so that clip.set_speed accepts what comes back.
func speedCurve(p *propReader) []timeline.SpeedPoint {
	raw, present := p.props["speed_curve"]
	if !present {
		return nil
	}

	list, _ := raw.([]any)
	pts := make([]timeline.SpeedPoint, 0, len(list))

	for _, e := range list {
		obj, _ := e.(map[string]any)
		sp, _ := obj["speed"].(map[string]any)
		sub := &propReader{props: sp, lose: func(string) {}}
		pt := &propReader{props: obj, lose: func(string) {}}
		t, ok0 := pt.number("t", 0, 1, false)
		num, ok1 := sub.number("num", 1, 1e6, false)
		den, ok2 := sub.number("den", 1, 1e6, false)

		if !ok0 || !ok1 || !ok2 || num != math.Trunc(num) || den != math.Trunc(den) {
			p.lose("vukem property speed_curve is not a valid value")

			return nil
		}

		pts = append(pts, timeline.SpeedPoint{T: t, Speed: timeline.Rational{Num: int(num), Den: int(den)}})
	}

	if timeline.CurveError(pts) != "" {
		p.lose("vukem property speed_curve is not a valid value")

		return nil
	}

	return pts
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

	for _, rng := range []func(string) (float64, float64, bool){
		timeline.ColourRange, timeline.MaskRange, timeline.CropRange,
	} {
		if lo, hi, ok := rng(prop); ok {
			return v >= lo && v <= hi
		}
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
	case !slices.Contains([]string{"x", "y", "scale", "rotation", "opacity", "volume"}, name) && !isColourKey(name, mode) &&
		!isMaskCropKey(name, mode):
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

// gradeArgs reads the hsl, wheels and curves properties. The three are decoded into the types of the
// document and checked with timeline.GradeProblem, so a value the command would refuse is reported
// here and left out, and the rest of the batch still applies.
func gradeArgs(p *propReader) map[string]any {
	out := map[string]any{}

	var (
		hsl    []timeline.HslBand
		wheels *timeline.Wheels
		curves *timeline.Curves
	)

	for _, f := range []struct {
		name string
		dst  any
	}{{"hsl", &hsl}, {"wheels", &wheels}, {"curves", &curves}} {
		raw, present := p.props[f.name]
		if !present {
			continue
		}

		b, err := json.Marshal(raw)
		if err == nil {
			err = json.Unmarshal(b, f.dst)
		}

		if err != nil {
			p.lose("vukem property " + f.name + " is not a valid value")

			continue
		}

		if _, msg := timeline.GradeProblem(hsl, wheels, curves); msg != "" {
			p.lose("vukem property " + f.name + " is not a valid value")

			hsl, wheels, curves = nil, nil, nil

			continue
		}

		if f.name == "hsl" && len(hsl) == 0 {
			continue
		}

		out[f.name] = raw
	}

	return out
}

// colourArgs is the argument of clip.set_color for the colour properties. A freeze frame is not a
// command of its own (item.freeze_frame splits a clip) and a lookup table names an asset the file does
// not carry: both are reported, never dropped silently.
func colourArgs(p *propReader, props map[string]any, lose func(string)) map[string]any {
	if f, _ := props["freeze"].(bool); f {
		lose("freeze frame")
	}

	if _, ok := props["lut"]; ok {
		lose("lookup table (the .cube asset is not part of the file)")
	}

	colour := map[string]any{}

	for _, f := range []struct {
		name   string
		lo, hi float64
	}{
		{"brightness", -1, 1},
		{"contrast", 0, 2},
		{"saturation", 0, 2},
		{"temperature", -1, 1},
		{"tint", -1, 1},
		{"hue", -180, 180},
	} {
		if v, ok := p.number(f.name, f.lo, f.hi, false); ok {
			colour[f.name] = v
		}
	}

	maps.Copy(colour, gradeArgs(p))

	return colour
}

// isColourKey is true for the colour properties a clip can animate.
func isColourKey(name string, mode itemMode) bool {
	_, _, ok := timeline.ColourRange(name)

	return ok && mode == modeClip
}

// isMaskCropKey is true for the mask properties a visual item can animate and the crop sides a clip can.
func isMaskCropKey(name string, mode itemMode) bool {
	if _, _, ok := timeline.MaskRange(name); ok {
		return mode != modeAudio
	}

	_, _, ok := timeline.CropRange(name)

	return ok && mode == modeClip
}
