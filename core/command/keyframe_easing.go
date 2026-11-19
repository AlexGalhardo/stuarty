package command

import (
	"fmt"
	"slices"
	"strings"

	"github.com/AlexGalhardo/stuarty/core/timeline"
)

func init() {
	Register(Define(Spec{
		Name:    "keyframe.set_easing",
		Summary: "Changes the easing of the segment that starts at a keyframe.",
		Description: "Gives the keyframe at the absolute position a named easing (easing), a custom cubic Bezier curve " +
			"(bezier: x1, y1, x2, y2; x1 and x2 from 0 to 1, y1 and y2 from -2 to 3) or a named Bezier preset " +
			"(preset: in, out, in_out, back, overshoot, stored as its four numbers). Exactly one of the three. " +
			"The value and the time of the keyframe do not change. Works for every animatable property, crop and mask included.",
		Example: `{"item_id": "itm_s6t7v8w9x0", "property": "scale", "at": "1s", "preset": "overshoot"}`,
		Errors: []string{
			CodeValidation, CodeInvalidTime, CodeNotFound, CodeTypeMismatch,
		},
		Scope: ScopeEdit,
	}, applyKeyframeSetEasing))
}

// bezierPresets are the named curves of keyframe.set_easing (SDD-002 C.75), in the order they are listed.
var bezierPresets = []struct {
	Name  string
	Curve [4]float64
}{
	{"in", [4]float64{0.42, 0, 1, 1}},
	{"out", [4]float64{0, 0, 0.58, 1}},
	{"in_out", [4]float64{0.42, 0, 0.58, 1}},
	{"back", [4]float64{0.36, 0, 0.66, -0.56}},
	{"overshoot", [4]float64{0.34, 1.56, 0.64, 1}},
}

func bezierPreset(name string) ([]float64, bool) {
	for _, p := range bezierPresets {
		if p.Name == name {
			return slices.Clone(p.Curve[:]), true
		}
	}

	return nil, false
}

func bezierPresetNames() []string {
	names := make([]string, len(bezierPresets))
	for i, p := range bezierPresets {
		names[i] = p.Name
	}

	return names
}

// checkBezier validates the four numbers of a curve and returns them rounded to 3 decimals.
func checkBezier(path string, b []float64) ([]float64, *Error) {
	if len(b) != 4 {
		return nil, Errorf(CodeValidation, "%s has %d numbers; a curve has 4: x1, y1, x2, y2.", path, len(b)).
			WithHint("Use for example [0.34, 1.56, 0.64, 1].").WithPath(path)
	}

	for i, v := range b {
		lo, hi := 0.0, 1.0
		if i%2 == 1 {
			lo, hi = timeline.BezierMinY, timeline.BezierMaxY
		}

		if !(v >= lo && v <= hi) { // also false for NaN
			return nil, Errorf(CodeValidation, "%s[%d] must be from %s to %s, got %s.",
				path, i, trimFloat(lo), trimFloat(hi), trimFloat(v)).
				WithHint("Keep x1 and x2 from 0 to 1; y1 and y2 may go from %s to %s.",
					trimFloat(timeline.BezierMinY), trimFloat(timeline.BezierMaxY)).
				WithPath(fmt.Sprintf("%s[%d]", path, i))
		}
	}

	out := make([]float64, 4)
	for i, v := range b {
		out[i] = round3(v)
	}

	return out, nil
}

// setEasing writes the easing of a keyframe: linear is stored absent and a curve only with "bezier".
func setEasing(k *timeline.Keyframe, easing string, curve []float64) {
	k.Easing, k.Bezier = easing, nil
	if easing == timeline.EasingBezier {
		k.Bezier = curve
	}
}

// keyframeEasingArgs checks the easing and bezier arguments of keyframe.set: a curve goes with the
// easing "bezier" and with nothing else. It returns the rounded curve.
func keyframeEasingArgs(easing string, bezier []float64) ([]float64, *Error) {
	switch {
	case easing == timeline.EasingBezier:
		if bezier == nil {
			return nil, invalidArg("args.bezier", "args.bezier is missing; the easing bezier needs four numbers.").
				WithHint("Add bezier, for example [0.34, 1.56, 0.64, 1], or use a named easing.")
		}

		return checkBezier("args.bezier", bezier)
	case bezier != nil:
		name := easing
		if name == "" {
			name = "linear"
		}

		return nil, invalidArg("args.bezier", "args.bezier goes with the easing \"bezier\", not with %q.", name).
			WithHint("Set easing to \"bezier\", or leave bezier out.")
	}

	return nil, nil
}

type keyframeSetEasingArgs struct {
	ItemID   string    `json:"item_id"          pattern:"^itm_[0123456789abcdefghjkmnpqrstvwxyz]{10}$"`
	Property string    `json:"property"                                                                enum:"x,y,scale,rotation,opacity,volume,brightness,contrast,saturation,temperature,tint,hue,mask_x,mask_y,mask_width,mask_height,mask_feather,crop_left,crop_top,crop_right,crop_bottom"`
	At       Time      `json:"at"                                                                                                                                                                                                                                                               desc:"Position of an existing keyframe on the project timeline."`
	Easing   string    `json:"easing,omitempty"                                                        enum:"linear,ease_in,ease_out,ease_in_out,hold"                                                                                                                                          desc:"A named easing. Exactly one of easing, bezier and preset."`
	Bezier   []float64 `json:"bezier,omitempty"                                                                                                                                                                                                                                                 desc:"x1, y1, x2, y2 of a cubic Bezier curve: x1 and x2 from 0 to 1, y1 and y2 from -2 to 3." min:"4" max:"4"`
	Preset   string    `json:"preset,omitempty"                                                        enum:"in,out,in_out,back,overshoot"                                                                                                                                                      desc:"A named Bezier curve, stored as its four numbers."`
}

func applyKeyframeSetEasing(c *Context, a *keyframeSetEasingArgs) error {
	if err := idForm("args.item_id", timeline.PrefixItem, a.ItemID); err != nil {
		return err
	}

	given := 0

	for _, g := range []bool{a.Easing != "", a.Bezier != nil, a.Preset != ""} {
		if g {
			given++
		}
	}

	if given != 1 {
		return Errorf(CodeValidation, "args needs exactly one of easing, bezier and preset; %d were given.", given).
			WithHint("Use a preset such as \"overshoot\", a bezier such as [0.34, 1.56, 0.64, 1], or an easing such as \"ease_out\".").
			WithPath("args")
	}

	easing, curve := a.Easing, []float64(nil)

	switch {
	case a.Bezier != nil:
		b, err := checkBezier("args.bezier", a.Bezier)
		if err != nil {
			return err
		}

		easing, curve = timeline.EasingBezier, b
	case a.Preset != "":
		b, ok := bezierPreset(a.Preset)
		if !ok {
			return invalidArg("args.preset", "args.preset is %q, which is not one of the accepted values.", a.Preset).
				WithHint("Use one of: %s.", strings.Join(bezierPresetNames(), ", "))
		}

		easing, curve = timeline.EasingBezier, b
	}

	it, err := c.item(a.ItemID)
	if err != nil {
		return err
	}

	if _, err := c.ParseTime("args.at", string(a.At)); err != nil {
		return err
	}

	if err := propApplies(&it, a.Property); err != nil {
		return err
	}

	t, err := c.keyframeT(&it, a.At)
	if err != nil {
		return err
	}

	list := it.Keyframes[a.Property]

	i := slices.IndexFunc(list, func(k timeline.Keyframe) bool { return k.T == t })
	if i < 0 {
		missing := &keyframeRemoveArgs{ItemID: a.ItemID, Property: a.Property, At: a.At}

		return c.missingKeyframe(&it, missing, list, it.Start+t)
	}

	ks := cloneKeyframes(it.Keyframes)
	setEasing(&ks[a.Property][i], easing, curve)
	it.Keyframes = ks
	c.Doc.Items[a.ItemID] = it
	c.SetEffectiveTime("at", it.Start+t, string(a.At))
	c.SetEffective("t", t)
	c.SetEffective("easing", easing)

	if curve != nil {
		c.SetEffective("bezier", curve)
	}

	return nil
}
