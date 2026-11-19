package command

import (
	"cmp"
	"maps"
	"math"
	"slices"

	"github.com/AlexGalhardo/stuarty/core/timeline"
)

func init() {
	Register(Define(Spec{
		Name:    "text.apply_preset",
		Summary: "Animates one edge of a visual item with a named preset.",
		Description: "Writes the keyframes (or, for slide_left and slide_right, the transition) of the preset on the " +
			"in or out edge, 0.4 seconds long unless duration is given. Keyframes of the same properties inside " +
			"that edge are replaced; the rest of the animation is kept.",
		Example: `{"item_id": "itm_s6t7v8w9x0", "preset": "fade_up", "edge": "in"}`,
		Errors: []string{
			CodeValidation, CodeInvalidTime, CodeNotFound, CodeTypeMismatch, CodeInvalidState, CodeLimit,
		},
		Scope: ScopeEdit,
	}, applyTextPreset))
}

type textPresetArgs struct {
	ItemID   string `json:"item_id"            desc:"A clip, text, caption or shape item."                     pattern:"^itm_[0123456789abcdefghjkmnpqrstvwxyz]{10}$"`
	Preset   string `json:"preset"             desc:"The animation."                                                                                                  enum:"fade_up,fade_down,pop,zoom_in,slide_left,slide_right"`
	Edge     string `json:"edge"               desc:"in or out."                                                                                                      enum:"in,out"`
	Duration Time   `json:"duration,omitempty" desc:"A length, at most the item's duration. Default \"0.4s\"."`
}

const (
	maxScale     = 20
	maxKeyframes = 100
)

// presetKey is one keyframe of a preset before it is placed on the item.
type presetKey struct {
	t      int
	value  float64
	easing string
}

// presetPlan is what the table of C.30 needs: the preset and edge, its length n,
// the item duration d, the height step h, and per property the base value b_p and
// the easing the last "in" keyframe inherits.
type presetPlan struct {
	preset string
	in     bool
	n, d   int
	h      float64
	base   map[string]float64
	tail   map[string]string
}

// writes returns the keyframes of the table for each property the preset writes.
func (p presetPlan) writes() map[string][]presetKey {
	q := int(math.Round(0.7 * float64(p.n)))
	capScale := func(v float64) float64 { return math.Min(round3(v), maxScale) }
	// ramp moves from the value far at the outer end to the base (in), or back (out).
	ramp := func(prop string, far float64) []presetKey {
		if p.in {
			return []presetKey{{0, far, "ease_out"}, {p.n, p.base[prop], p.tail[prop]}}
		}
		return []presetKey{{p.d - p.n, p.base[prop], "ease_in"}, {p.d, far, ""}}
	}
	out := map[string][]presetKey{}
	if p.preset == "slide_left" || p.preset == "slide_right" {
		return out
	}
	out["opacity"] = ramp("opacity", 0)
	switch p.preset {
	case "fade_up", "fade_down":
		dir := -1.0
		if (p.preset == "fade_up") == p.in {
			dir = 1
		}
		out["y"] = ramp("y", p.base["y"]+dir*p.h)
	case "zoom_in":
		far := 1.2 * p.base["scale"]
		if p.in {
			far = 0.8 * p.base["scale"]
		}
		out["scale"] = ramp("scale", capScale(far))
	case "pop":
		b := p.base["scale"]
		if p.in {
			keys := []presetKey{{0, capScale(0.6 * b), "ease_out"}}
			if p.n > 1 {
				keys = append(keys, presetKey{q, capScale(1.1 * b), "ease_in_out"})
			}
			out["scale"] = append(keys, presetKey{p.n, b, p.tail["scale"]})
		} else {
			keys := []presetKey{{p.d - p.n, b, "ease_in_out"}}
			if p.n > 1 {
				keys = append(keys, presetKey{p.d - q, capScale(1.1 * b), "ease_in"})
			}
			out["scale"] = append(keys, presetKey{p.d, capScale(0.6 * b), ""})
		}
	default:
	}
	return out
}

// presetBase is the b_p of a property: its value where the preset meets the rest of
// the animation, from the keyframes or the static value (R10).
func (c *Context) presetBase(it *timeline.Item, prop string, at int) float64 {
	static := 1.0
	switch prop {
	case "y":
		static = float64(c.Doc.Settings.Height) / 2
		if it.Y != nil {
			static = *it.Y
		}
	case "scale":
		static = it.Scale
	case "opacity":
		static = it.Opacity
	}
	return timeline.ValueAtProp(prop, it.Keyframes[prop], at, static)
}

// easingAt is the easing in force at frame t: that of the last keyframe at or before
// it; linear is stored as no easing.
func easingAt(keys []timeline.Keyframe, t int) string {
	e := ""
	for _, k := range keys {
		if k.T <= t {
			e = k.Easing
		}
	}
	if e == "linear" {
		return ""
	}
	return e
}

func applyTextPreset(c *Context, a *textPresetArgs) error {
	if err := idForm("args.item_id", timeline.PrefixItem, a.ItemID); err != nil {
		return err
	}
	n, err := c.length("args.duration", cmp.Or(string(a.Duration), "0.4s"), false)
	if err != nil {
		return err
	}
	it, err := c.item(a.ItemID)
	if err != nil {
		return err
	}
	if !isVisual(it.Type) {
		return Errorf(CodeTypeMismatch, "Item %s is an audio item; presets animate visual items.", a.ItemID).
			WithHint("Use item.set_audio fades for audio.").WithPath("args.item_id")
	}
	d := it.Duration
	if n > d {
		return Errorf(
			CodeInvalidState,
			"Preset duration %d frames is longer than item %s (%d frames).",
			n,
			a.ItemID,
			d,
		).
			WithHint("Use duration \"%df\" or shorter.", d).
			WithPath("args.duration")
	}
	c.SetEffective("preset", a.Preset)
	c.SetEffective("edge", a.Edge)
	c.SetEffectiveTime("duration", n, string(a.Duration))

	in := a.Edge == "in"
	if a.Preset == "slide_left" || a.Preset == "slide_right" {
		tr := &timeline.Transition{Kind: a.Preset, Duration: n}
		if in {
			it.TransitionIn = tr
		} else {
			it.TransitionOut = tr
		}
		if err := checkFit(&it, "The preset"); err != nil {
			return err.WithPath("args.duration")
		}
		c.Doc.Items[a.ItemID] = it
		c.SetEffective("written", []string{})
		c.SetEffective("base", map[string]float64{})
		return nil
	}

	at := n
	if !in {
		at = d - n
	}
	plan := presetPlan{
		preset: a.Preset, in: in, n: n, d: d, h: round3(0.05 * float64(c.Doc.Settings.Height)),
		base: map[string]float64{}, tail: map[string]string{},
	}
	for _, prop := range []string{"y", "scale", "opacity"} {
		plan.base[prop] = c.presetBase(&it, prop, at)
		plan.tail[prop] = easingAt(it.Keyframes[prop], at)
	}
	writes := plan.writes()
	written := slices.Sorted(maps.Keys(writes))
	lo, hi := 0, n
	if !in {
		lo, hi = d-n, d
	}
	ks := cloneKeyframes(it.Keyframes)
	if ks == nil {
		ks = timeline.Keyframes{}
	}
	bases := map[string]float64{}
	for _, prop := range written {
		var merged []timeline.Keyframe
		for _, k := range ks[prop] {
			if k.T < lo || k.T > hi {
				merged = append(merged, k)
			}
		}
		for _, k := range writes[prop] {
			nk := timeline.NewKeyframe(k.t, round3(k.value))
			if k.easing != "" {
				nk.Easing = k.easing
			}
			merged = append(merged, nk)
		}
		slices.SortStableFunc(merged, func(x, y timeline.Keyframe) int { return x.T - y.T })
		if len(merged) > maxKeyframes {
			return Errorf(CodeLimit, "Property %s of %s would have %d keyframes; the limit is %d.",
				prop, a.ItemID, len(merged), maxKeyframes).
				WithHint("Remove keyframes with keyframe.clear first.")
		}
		ks[prop] = merged
		bases[prop] = plan.base[prop]
	}
	it.Keyframes = ks
	c.Doc.Items[a.ItemID] = it
	c.SetEffective("written", written)
	c.SetEffective("base", bases)
	return nil
}
