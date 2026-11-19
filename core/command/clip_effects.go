package command

import (
	"fmt"
	"slices"

	"github.com/AlexGalhardo/stuarty/core/timeline"
)

func init() {
	Register(Define(Spec{
		Name:    "clip.set_effects",
		Summary: "Sets the visual effects of a clip: motion blur, glow, vignette, grain, RGB split and shake.",
		Description: "Replaces the whole list of effects, applied in order; an empty list removes them. Each kind appears at " +
			"most once and takes only its own parameters (omitted ones get the defaults): motion_blur amount 0.5 and " +
			"angle 0; glow amount 0.5, radius 12 and threshold 0.6; vignette amount 0.5 and softness 0.5; grain amount " +
			"0.3, size 1 and seed 0; rgb_split amount 0.3 and angle 0; shake amount 0.3, speed 8 and seed 0.",
		Example: `{"item_id": "itm_m1n2p3q4r5", "effects": [{"kind": "vignette", "amount": 0.6}, {"kind": "grain"}]}`,
		Errors:  []string{CodeValidation, CodeNotFound, CodeTypeMismatch},
		Scope:   ScopeEdit,
		Phase:   Beta,
	}, applyClipSetEffects))
}

type effectArgs struct {
	Kind      string   `json:"kind"                desc:"The effect."                                                enum:"motion_blur,glow,vignette,grain,rgb_split,shake"`
	Amount    *float64 `json:"amount,omitempty"    desc:"Strength, 0 to 1; all kinds."                                                                                      min:"0"    max:"1"`
	Angle     *float64 `json:"angle,omitempty"     desc:"Direction in degrees, -180 to 180; motion_blur, rgb_split."                                                        min:"-180" max:"180"`
	Radius    *float64 `json:"radius,omitempty"    desc:"Spread in frame pixels, 1 to 100; glow."                                                                           min:"1"    max:"100"`
	Threshold *float64 `json:"threshold,omitempty" desc:"Brightness where the glow starts, 0 to 1; glow."                                                                   min:"0"    max:"1"`
	Softness  *float64 `json:"softness,omitempty"  desc:"How far in the darkening starts, 0 to 1; vignette."                                                                min:"0"    max:"1"`
	Size      *int     `json:"size,omitempty"      desc:"Grain size in frame pixels, 1 to 8; grain."                                                                        min:"1"    max:"8"`
	Speed     *int     `json:"speed,omitempty"     desc:"Shakes per second, 1 to 30; shake."                                                                                min:"1"    max:"30"`
	Seed      *int     `json:"seed,omitempty"      desc:"Noise seed, 0 to 65535; grain, shake."                                                                             min:"0"    max:"65535"`
}

type clipEffectsArgs struct {
	ItemID  string       `json:"item_id" desc:"A clip."                                                             pattern:"^itm_[0123456789abcdefghjkmnpqrstvwxyz]{10}$"`
	Effects []effectArgs `json:"effects" desc:"The effects in order, up to 6, each kind once; [] removes them all."                                                        maxitems:"6"`
}

// effectDefaults are the values a parameter takes when it is left out (SDD-004 §3.13).
var effectDefaults = map[string]float64{
	"amount": 0.5, "angle": 0, "radius": 12, "threshold": 0.6, "softness": 0.5, "size": 1, "speed": 8, "seed": 0,
}

func (e *effectArgs) given() map[string]*float64 {
	asF := func(n *int) *float64 {
		if n == nil {
			return nil
		}
		f := float64(*n)
		return &f
	}
	return map[string]*float64{
		"amount": e.Amount, "angle": e.Angle, "radius": e.Radius, "threshold": e.Threshold,
		"softness": e.Softness, "size": asF(e.Size), "speed": asF(e.Speed), "seed": asF(e.Seed),
	}
}

// buildEffect checks one effect of the arguments and returns it with the defaults filled in and
// the decimals rounded to 3.
func buildEffect(i int, a *effectArgs) (timeline.Effect, *Error) {
	path := fmt.Sprintf("args.effects[%d]", i)
	params, ok := timeline.EffectParams[a.Kind]
	if !ok {
		return timeline.Effect{}, Errorf(CodeValidation, "%s.kind %q is not an effect.", path, a.Kind).
			WithHint("Use motion_blur, glow, vignette, grain, rgb_split or shake.").WithPath(path + ".kind")
	}
	given := a.given()
	out := timeline.Effect{Kind: a.Kind}
	for _, name := range []string{"amount", "angle", "radius", "threshold", "softness", "size", "speed", "seed"} {
		v := given[name]
		switch {
		case !slices.Contains(params, name):
			if v != nil {
				return out, Errorf(CodeValidation, "%s.%s does not apply to %s.", path, name, a.Kind).
					WithHint("%s takes %v.", a.Kind, params).WithPath(path + "." + name)
			}
			continue
		case v == nil:
			d := effectDefaults[name]
			v = &d
		}
		lo, hi := timeline.EffectRange(name)
		if *v < lo || *v > hi {
			return out, Errorf(CodeValidation, "%s.%s %v is outside %v to %v.", path, name, *v, lo, hi).
				WithHint("Use a %s from %v to %v.", name, lo, hi).WithPath(path + "." + name)
		}
		x := round3(*v)
		switch name {
		case "amount":
			out.Amount = &x
		case "angle":
			out.Angle = &x
		case "radius":
			out.Radius = &x
		case "threshold":
			out.Threshold = &x
		case "softness":
			out.Softness = &x
		case "size":
			n := int(x)
			out.Size = &n
		case "speed":
			n := int(x)
			out.Speed = &n
		case "seed":
			n := int(x)
			out.Seed = &n
		}
	}
	return out, nil
}

func applyClipSetEffects(c *Context, a *clipEffectsArgs) error {
	if len(a.Effects) > timeline.MaxEffects {
		return Errorf(
			CodeValidation,
			"args.effects has %d effects; the limit is %d.",
			len(a.Effects),
			timeline.MaxEffects,
		).
			WithHint("Send at most 6 effects.").
			WithPath("args.effects")
	}
	var effects []timeline.Effect
	seen := map[string]bool{}
	for i := range a.Effects {
		e, err := buildEffect(i, &a.Effects[i])
		if err != nil {
			return err
		}
		if seen[e.Kind] {
			return Errorf(CodeValidation, "args.effects[%d] repeats the effect %s.", i, e.Kind).
				WithHint("List each effect once.").WithPath(fmt.Sprintf("args.effects[%d].kind", i))
		}
		seen[e.Kind] = true
		effects = append(effects, e)
	}
	it, err := c.clipItem(a.ItemID, "effects")
	if err != nil {
		return err
	}
	it.Effects = effects
	c.Doc.Items[a.ItemID] = it
	c.SetEffective("effects", len(effects))
	return nil
}
