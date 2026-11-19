package command

import (
	"cmp"
	"math"
	"slices"

	"github.com/AlexGalhardo/stuarty/core/timeline"
)

func init() {
	Register(Define(Spec{
		Name:    "animation.apply_preset",
		Summary: "Animates the start or the end of a visual item with a preset: fade, slide, scale, pop, push in or pull out.",
		Description: "Writes ordinary keyframes (with a Bezier easing where the preset needs a curve) on the first " +
			"duration frames (edge in) or the last ones (edge out) of a clip, text, caption or shape item; edit them " +
			"afterwards with keyframe.set and keyframe.set_easing. Keyframes of the same properties inside that window " +
			"are replaced, the rest are kept. duration is at most the item's duration, default 0.5s (the whole item " +
			"for push_in and pull_out). direction is for slide; amount is the slide distance in pixels, the scale " +
			"factor of scale, or the zoom factor of push_in and pull_out. animation.list_presets describes them.",
		Example: `{"item_id": "itm_s6t7v8w9x0", "preset": "slide", "edge": "in", "direction": "left", "duration": "0.5s"}`,
		Errors: []string{
			CodeValidation, CodeInvalidTime, CodeNotFound, CodeTypeMismatch, CodeLimit,
		},
		Scope: ScopeEdit,
	}, applyAnimationPreset))

	Register(Define(Spec{
		Name:    "animation.list_presets",
		Summary: "Describes the animation presets and the named Bezier curves.",
		Description: "Changes nothing. The result lists, for each preset of animation.apply_preset, what it " +
			"animates and the range and default of its parameters, and the four numbers of each named curve of " +
			"keyframe.set_easing.",
		Example: `{}`,
		Errors:  []string{CodeValidation},
		Scope:   ScopeEdit,
	}, applyAnimationListPresets))
}

// amountSpec is the range and default of the amount argument of a preset.
type amountSpec struct {
	Min, Max, Default float64
	Unit              string
}

// animationPreset is one row of the table of SDD-002 C.76.
type animationPreset struct {
	ID        string
	Summary   string
	Direction bool        // takes direction (slide)
	Amount    *amountSpec // nil: takes no amount
	WholeItem bool        // the default duration is the whole item
	// InEase and OutEase are the Bezier presets of the first key of the window; "" is linear.
	InEase, OutEase string
}

var animationPresets = []animationPreset{
	{ID: "fade", Summary: "Opacity from 0 to its value (in) or to 0 (out)."},
	{
		ID: "slide", Summary: "Moves in from a side of the frame (in) or out to it (out).", Direction: true,
		Amount: &amountSpec{Min: 1, Max: 10000, Unit: "project pixels"},
		InEase: "out", OutEase: "in",
	},
	{
		ID: "scale", Summary: "Scale from a factor of its value to its value (in) or the reverse (out).",
		Amount: &amountSpec{Min: 0.01, Max: 20, Default: 0.5, Unit: "factor"}, InEase: "out", OutEase: "in",
	},
	{
		ID: "pop", Summary: "Pops in from nothing with an overshoot (in), or shrinks away (out).",
		InEase: "overshoot", OutEase: "back",
	},
	{
		ID: "push_in", Summary: "A steady zoom in: scale grows by the factor over the window.", WholeItem: true,
		Amount: &amountSpec{Min: 1.01, Max: 4, Default: 1.2, Unit: "factor"}, InEase: "in_out", OutEase: "in_out",
	},
	{
		ID: "pull_out", Summary: "A steady zoom out: scale shrinks by the factor over the window.", WholeItem: true,
		Amount: &amountSpec{Min: 1.01, Max: 4, Default: 1.2, Unit: "factor"}, InEase: "in_out", OutEase: "in_out",
	},
}

func animationPresetByID(id string) (animationPreset, bool) {
	for _, p := range animationPresets {
		if p.ID == id {
			return p, true
		}
	}

	return animationPreset{}, false
}

const defaultPresetDuration = "0.5s"

type animationPresetArgs struct {
	ItemID    string   `json:"item_id"             desc:"A clip, text, caption or shape item."                                                                            pattern:"^itm_[0123456789abcdefghjkmnpqrstvwxyz]{10}$"`
	Preset    string   `json:"preset"              desc:"The animation."                                                                                                                                                         enum:"fade,slide,scale,pop,push_in,pull_out"`
	Edge      string   `json:"edge,omitempty"      desc:"in animates the start of the item, out its end. Default in."                                                                                                            enum:"in,out"`
	Duration  Time     `json:"duration,omitempty"  desc:"A length, at most the item's duration. Default 0.5s; the whole item for push_in and pull_out."`
	Direction string   `json:"direction,omitempty" desc:"slide only: the side it comes from or leaves to. Default left."                                                                                                         enum:"left,right,up,down"`
	Amount    *float64 `json:"amount,omitempty"    desc:"slide: distance in pixels; scale: factor at the far end; push_in and pull_out: zoom factor. Per preset default."`
}

func applyAnimationPreset(c *Context, a *animationPresetArgs) error {
	if err := idForm("args.item_id", timeline.PrefixItem, a.ItemID); err != nil {
		return err
	}

	preset, ok := animationPresetByID(a.Preset)
	if !ok {
		return invalidArg("args.preset", "args.preset is %q, which is not one of the accepted values.", a.Preset).
			WithHint("Use animation.list_presets to see the presets.")
	}

	if a.Direction != "" && !preset.Direction {
		return invalidArg("args.direction", "args.direction applies to slide only, not to %s.", a.Preset).
			WithHint("Leave direction out.")
	}

	if a.Amount != nil && preset.Amount == nil {
		return invalidArg("args.amount", "args.amount does not apply to %s.", a.Preset).WithHint("Leave amount out.")
	}

	if a.Amount != nil && !(*a.Amount >= preset.Amount.Min && *a.Amount <= preset.Amount.Max) {
		return invalidArg("args.amount", "args.amount must be from %s to %s for %s, got %s.",
			trimFloat(preset.Amount.Min), trimFloat(preset.Amount.Max), a.Preset, trimFloat(*a.Amount)).
			WithHint("Use animation.list_presets to see the range.")
	}

	it, err := c.item(a.ItemID)
	if err != nil {
		return err
	}

	if !isVisual(it.Type) {
		return Errorf(CodeTypeMismatch, "Animation presets apply to visual items; item %s is an audio item.", a.ItemID).
			WithHint("Use keyframe.set with volume for audio items.").WithPath("args.item_id")
	}

	d, err := c.presetDuration(&it, &preset, a)
	if err != nil {
		return err
	}

	edge := cmp.Or(a.Edge, "in")
	w0, w1 := 0, d

	if edge == "out" {
		w0, w1 = it.Duration-d, it.Duration
	}

	steps := c.presetSteps(&preset, a)

	ks := cloneKeyframes(it.Keyframes)
	if ks == nil {
		ks = timeline.Keyframes{}
	}

	written, props := 0, []string{}

	for _, st := range steps {
		list, err := writeWindow(&it, c, st, edge, w0, w1)
		if err != nil {
			return err
		}

		if len(list) > maxKeyframes {
			return Errorf(CodeLimit, "Property %s of %s would have %d keyframes; the limit is %d.",
				st.prop, a.ItemID, len(list), maxKeyframes).
				WithHint("Remove keyframes with keyframe.clear first.")
		}

		ks[st.prop] = list
		written += 2

		props = append(props, st.prop)
	}

	it.Keyframes = ks
	c.Doc.Items[a.ItemID] = it
	c.SetEffective("properties", props)
	c.SetEffective("edge", edge)
	c.SetEffectiveTime("duration", d, string(a.Duration))
	c.SetEffective("keyframes", written)

	return nil
}

// presetDuration is the length of the window, from 1 frame up to the duration of the item.
func (c *Context) presetDuration(it *timeline.Item, p *animationPreset, a *animationPresetArgs) (int, *Error) {
	if a.Duration == "" && p.WholeItem {
		return it.Duration, nil
	}

	d, err := c.length("args.duration", cmp.Or(string(a.Duration), defaultPresetDuration), false)
	if err != nil {
		return 0, err
	}

	if d > it.Duration {
		return 0, Errorf(CodeInvalidTime, "args.duration %q is %d frames; item %s lasts %d.",
			string(a.Duration), d, it.ID, it.Duration).
			WithHint("Use a duration of at most \"%df\".", it.Duration).WithPath("args.duration").
			WithDetail("range", []int{1, it.Duration})
	}

	return d, nil
}

// presetStep is what a preset does to one property: the value at the far end of the window and the
// easing of its first key, given the value `at` the window edge where it meets the rest of the item.
type presetStep struct {
	prop string
	// far is the far value for an in/out preset, computed from the value at the inner edge; for the
	// zoom presets it is computed from the value at the start of the window.
	far    func(v float64) float64
	zoom   bool // push_in and pull_out: the value at the start of the window, then far(start)
	easing string
}

func (c *Context) presetSteps(p *animationPreset, a *animationPresetArgs) []presetStep {
	amount := 0.0
	if a.Amount != nil {
		amount = *a.Amount
	} else if p.Amount != nil {
		amount = p.Amount.Default
	}

	ease := p.InEase
	if a.Edge == "out" {
		ease = p.OutEase
	}

	scaleStep := func(far func(float64) float64, zoom bool) []presetStep {
		return []presetStep{{prop: "scale", far: far, zoom: zoom, easing: ease}}
	}

	switch p.ID {
	case "fade":
		return []presetStep{{prop: "opacity", far: func(float64) float64 { return 0 }}}
	case "slide":
		return c.slideStep(a, amount, ease)
	case "scale":
		return scaleStep(func(v float64) float64 { return v * amount }, false)
	case "pop":
		return scaleStep(func(float64) float64 { return timeline.MinScale }, false)
	case "push_in":
		return scaleStep(func(v float64) float64 { return v * amount }, true)
	default: // pull_out
		return scaleStep(func(v float64) float64 { return v / amount }, true)
	}
}

func (c *Context) slideStep(a *animationPresetArgs, amount float64, ease string) []presetStep {
	dir := cmp.Or(a.Direction, "left")
	prop, sign, size := "x", -1.0, float64(c.Doc.Settings.Width)

	switch dir {
	case "right":
		sign = 1
	case "up":
		prop, size = "y", float64(c.Doc.Settings.Height)
	case "down":
		prop, sign, size = "y", 1, float64(c.Doc.Settings.Height)
	}

	dist := amount
	if a.Amount == nil {
		dist = math.Round(size / 4)
	}

	return []presetStep{{prop: prop, far: func(v float64) float64 { return v + sign*dist }, easing: ease}}
}

// valueAtFrame is the value of a property at frame t of the item: its keyframes, or its static value.
func (c *Context) valueAtFrame(it *timeline.Item, prop string, t int) float64 {
	return timeline.ValueAtProp(prop, it.Keyframes[prop], t, c.staticValue(it, prop))
}

// writeWindow returns the keyframes of one property after the preset: those outside [w0, w1] stay, and
// the window holds two, the start and the end (SDD-002 C.76 postcondition 1).
func writeWindow(it *timeline.Item, c *Context, st presetStep, edge string, w0, w1 int) ([]timeline.Keyframe, *Error) {
	old := it.Keyframes[st.prop]

	var startV, endV float64

	switch {
	case st.zoom:
		startV = c.valueAtFrame(it, st.prop, w0)
		endV = st.far(startV)
	case edge == "in":
		endV = c.valueAtFrame(it, st.prop, w1)
		startV = st.far(endV)
	default:
		startV = c.valueAtFrame(it, st.prop, w0)
		endV = st.far(startV)
	}

	startV, endV = round3(startV), round3(endV)

	if st.prop == "scale" {
		for _, v := range []float64{startV, endV} {
			if v < timeline.MinScale || v > maxScale {
				return nil, Errorf(CodeValidation, "The preset would make a scale of %s for %s; the range is %s to %d.",
					trimFloat(v), it.ID, trimFloat(timeline.MinScale), maxScale).
					WithHint("Use a smaller amount.").WithPath("args.amount")
			}
		}
	}

	start := timeline.NewKeyframe(w0, startV)
	end := timeline.NewKeyframe(w1, endV)

	if st.easing != "" {
		curve, _ := bezierPreset(st.easing)
		setEasing(&start, timeline.EasingBezier, curve)
	}

	// The end key of an in window continues the segment it cuts: it takes the easing of the key that
	// governed that frame. The end key of an out window is the last of the item and has no segment.
	if edge == "in" {
		if gov := governing(old, w1); gov != nil {
			inheritEasing(&end, *gov)
		}
	}

	list := make([]timeline.Keyframe, 0, len(old)+2)

	for _, k := range old {
		if k.T < w0 {
			list = append(list, k)
		}
	}

	list = append(list, start, end)

	for _, k := range old {
		if k.T > w1 {
			list = append(list, k)
		}
	}

	return list, nil
}

// governing is the keyframe whose easing is in force at frame t: the last one at or before it.
func governing(keys []timeline.Keyframe, t int) *timeline.Keyframe {
	var g *timeline.Keyframe

	for i := range keys {
		if keys[i].T <= t {
			g = &keys[i]
		}
	}

	return g
}

type listPresetsArgs struct{}

func applyAnimationListPresets(c *Context, _ *listPresetsArgs) error {
	presets := make([]map[string]any, 0, len(animationPresets))

	for _, p := range animationPresets {
		params := map[string]any{
			"duration": map[string]any{
				"unit":    "frames",
				"min":     1,
				"max":     "the item's duration",
				"default": defaultPresetDuration,
			},
		}

		if p.WholeItem {
			params["duration"] = map[string]any{
				"unit":    "frames",
				"min":     1,
				"max":     "the item's duration",
				"default": "the whole item",
			}
		}

		if p.Direction {
			params["direction"] = map[string]any{"values": []string{"left", "right", "up", "down"}, "default": "left"}
		}

		if p.Amount != nil {
			amount := map[string]any{
				"min":     p.Amount.Min,
				"max":     p.Amount.Max,
				"default": p.Amount.Default,
				"unit":    p.Amount.Unit,
			}
			if p.ID == "slide" {
				amount["default"] = "a quarter of the width (left, right) or of the height (up, down)"
			}

			params["amount"] = amount
		}

		props := []string{"scale"}

		switch p.ID {
		case "fade":
			props = []string{"opacity"}
		case "slide":
			props = []string{"x", "y"}
		}

		presets = append(presets, map[string]any{
			"id": p.ID, "summary": p.Summary, "properties": props, "edges": []string{"in", "out"}, "parameters": params,
		})
	}

	curves := map[string][]float64{}

	for _, b := range bezierPresets {
		curves[b.Name] = slices.Clone(b.Curve[:])
	}

	c.SetEffective("presets", presets)
	c.SetEffective("bezier_presets", curves)

	return nil
}
