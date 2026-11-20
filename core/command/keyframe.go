package command

import (
	"math"
	"slices"
	"strconv"
	"strings"

	"github.com/AlexGalhardo/stuarty/core/timeline"
)

func init() {
	Register(Define(Spec{
		Name:    "keyframe.set",
		Summary: "Adds or replaces a keyframe of one animated property of an item.",
		Description: "Sets the value of x, y, scale, rotation, opacity, (clips and audio) volume, (clips) brightness, contrast, saturation, temperature, tint or hue, " +
			"(items with a mask, set it first with clip.set_mask) mask_x, mask_y, mask_width, mask_height or mask_feather, or (clips) crop_left, crop_top, crop_right or crop_bottom at an absolute " +
			"position inside the item, from its start to its end inclusive. When the property had no keyframe " +
			"and at is after the start, its current static value becomes the first keyframe. The easing applies " +
			"from this keyframe to the next. At most 100 keyframes per property.",
		Example: `{"item_id": "itm_s6t7v8w9x0", "property": "scale", "at": "2s", "value": 1.2, "easing": "ease_out"}`,
		Errors: []string{
			CodeValidation, CodeInvalidTime, CodeNotFound, CodeTypeMismatch, CodeLimit, CodeInvalidState,
		},
		Scope: ScopeEdit,
	}, applyKeyframeSet))

	Register(Define(Spec{
		Name:    "keyframe.remove",
		Summary: "Removes the keyframe of a property at a frame.",
		Description: "Removes the keyframe at the absolute position. The last keyframe of a property writes its " +
			"value into the static field.",
		Example: `{"item_id": "itm_s6t7v8w9x0", "property": "scale", "at": "2s"}`,
		Errors:  []string{CodeValidation, CodeInvalidTime, CodeNotFound, CodeTypeMismatch},
		Scope:   ScopeEdit,
	}, applyKeyframeRemove))

	Register(Define(Spec{
		Name:    "keyframe.clear",
		Summary: "Removes all keyframes of one property, or of every property.",
		Description: "The value at the first frame of the item becomes the static value. Without property, every " +
			"animated property is cleared.",
		Example: `{"item_id": "itm_s6t7v8w9x0", "property": "opacity"}`,
		Errors:  []string{CodeValidation, CodeNotFound, CodeTypeMismatch},
		Scope:   ScopeEdit,
	}, applyKeyframeClear))
}

// propApplies is precondition 3: volume belongs to clips and audio items, the other
// properties to visual items.
func propApplies(it *timeline.Item, prop string) *Error {
	hasVolume := it.Type == timeline.ItemClip || it.Type == timeline.ItemAudio || it.Type == timeline.ItemComposition
	switch {
	case prop == "volume" && !hasVolume:
		return Errorf(CodeTypeMismatch, "Property volume does not apply to %s item %s.", it.Type, it.ID).
			WithHint("Use opacity, scale, x, y or rotation.").WithPath("args.property")
	case isColourProp(prop) && it.Type != timeline.ItemClip:
		return Errorf(
			CodeTypeMismatch,
			"Property %s applies to clips; item %s is %s item.",
			prop,
			it.ID,
			article(it.Type),
		).
			WithHint("Use opacity, scale, x, y or rotation.").
			WithPath("args.property")
	case isCropProp(prop) && it.Type != timeline.ItemClip:
		return Errorf(
			CodeTypeMismatch,
			"Property %s applies to clips; item %s is %s item.",
			prop,
			it.ID,
			article(it.Type),
		).
			WithHint("Use opacity, scale, x, y or rotation.").
			WithPath("args.property")
	case prop != "volume" && it.Type == timeline.ItemAudio:
		return Errorf(CodeTypeMismatch, "Property %s does not apply to audio item %s.", prop, it.ID).
			WithHint("Use property volume for audio items.").WithPath("args.property")
	}
	return nil
}

// staticValue is the value of a property when it has no keyframes (R10).
func (c *Context) staticValue(it *timeline.Item, prop string) float64 {
	switch prop {
	case "x":
		if it.X != nil {
			return *it.X
		}
		return float64(c.Doc.Settings.Width) / 2
	case "y":
		if it.Y != nil {
			return *it.Y
		}
		return float64(c.Doc.Settings.Height) / 2
	case "scale":
		return it.Scale
	case "rotation":
		return it.Rotation
	case "opacity":
		return it.Opacity
	case "volume":
		return it.Volume
	}
	if isMaskProp(prop) {
		return it.MaskValue(prop)
	}
	if isCropProp(prop) {
		return it.CropValue(prop)
	}
	return it.ColourValue(prop)
}

func isMaskProp(prop string) bool {
	_, _, ok := timeline.MaskRange(prop)
	return ok
}

func isCropProp(prop string) bool {
	_, _, ok := timeline.CropRange(prop)
	return ok
}

func isColourProp(prop string) bool {
	_, _, ok := timeline.ColourRange(prop)
	return ok
}

func checkKeyframeValue(prop string, v float64) *Error {
	bad := func(rule string) *Error {
		return Errorf(CodeValidation, "args.value must be %s for %s, got %s.", rule, prop, trimFloat(v)).
			WithHint("Use a value in range.").WithPath("args.value")
	}
	switch {
	case math.IsNaN(v) || math.IsInf(v, 0):
		return bad("a finite number")
	case math.Abs(v) > maxMagnitude:
		return bad("between -1000000000 and 1000000000")
	case prop == "scale" && (v <= 0 || v > maxScale):
		return bad("above 0 and at most 20")
	case prop == "opacity" && (v < 0 || v > 1):
		return bad("from 0 to 1")
	case prop == "volume" && (v < 0 || v > 4):
		return bad("from 0 to 4")
	}
	for _, rng := range []func(string) (float64, float64, bool){
		timeline.ColourRange, timeline.MaskRange, timeline.CropRange,
	} {
		if lo, hi, ok := rng(prop); ok && (v < lo || v > hi) {
			return bad("from " + trimFloat(lo) + " to " + trimFloat(hi))
		}
	}
	return nil
}

// keyframeT converts an absolute position to t, checking that it lies inside the item
// (inclusive, decision 12).
func (c *Context) keyframeT(it *timeline.Item, at Time) (int, *Error) {
	frame, err := c.ParseTime("args.at", string(at))
	if err != nil {
		return 0, err
	}
	end := itemEnd(it)
	if frame < it.Start || frame > end {
		return 0, Errorf(
			CodeInvalidTime,
			"args.at %q is outside item %s; the allowed frames are %d to %d inclusive (a keyframe may sit at start + duration).",
			string(at),
			it.ID,
			it.Start,
			end,
		).
			WithHint("Use an at from \"%df\" to \"%df\".", it.Start, end).
			WithDetail("range", []int{it.Start, end}).WithPath("args.at")
	}
	return frame - it.Start, nil
}

type keyframeSetArgs struct {
	ItemID   string    `json:"item_id"          pattern:"^itm_[0123456789abcdefghjkmnpqrstvwxyz]{10}$"`
	Property string    `json:"property"                                                                enum:"x,y,scale,rotation,opacity,volume,brightness,contrast,saturation,temperature,tint,hue,mask_x,mask_y,mask_width,mask_height,mask_feather,crop_left,crop_top,crop_right,crop_bottom"`
	At       Time      `json:"at"                                                                                                                                                                                                                                                               desc:"Position on the project timeline, from the item start to its end inclusive."`
	Value    float64   `json:"value"                                                                                                                                                                                                                                                            desc:"Valid for the property: scale above 0 up to 20, opacity 0 to 1, volume 0 to 4."`
	Easing   string    `json:"easing,omitempty"                                                        enum:"linear,ease_in,ease_out,ease_in_out,hold,bezier"                                                                                                                                   desc:"From this keyframe to the next. Default linear. bezier needs the bezier argument."`
	Bezier   []float64 `json:"bezier,omitempty"                                                                                                                                                                                                                                                 desc:"x1, y1, x2, y2 of the curve of easing bezier: x1 and x2 from 0 to 1, y1 and y2 from -2 to 3." min:"4" max:"4"`
}

func applyKeyframeSet(c *Context, a *keyframeSetArgs) error {
	if err := idForm("args.item_id", timeline.PrefixItem, a.ItemID); err != nil {
		return err
	}
	if err := checkKeyframeValue(a.Property, a.Value); err != nil {
		return err
	}
	curve, err := keyframeEasingArgs(a.Easing, a.Bezier)
	if err != nil {
		return err
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
	if a.Property == "volume" {
		if err := c.stillImageNoAudio(&it, "has no volume to animate"); err != nil {
			return err.WithPath("args.property")
		}
	}
	if isMaskProp(a.Property) && it.Mask == nil {
		return Errorf(CodeInvalidState, "Item %s has no mask to animate.", a.ItemID).
			WithHint("Set one with clip.set_mask first.").WithPath("args.property")
	}
	t, err := c.keyframeT(&it, a.At)
	if err != nil {
		return err
	}
	ks := cloneKeyframes(it.Keyframes)
	if ks == nil {
		ks = timeline.Keyframes{}
	}
	list := ks[a.Property]
	auto := len(list) == 0 && t > 0
	if auto {
		list = []timeline.Keyframe{timeline.NewKeyframe(0, round3(c.staticValue(&it, a.Property)))}
	}
	nk := timeline.NewKeyframe(t, round3(a.Value))
	if a.Easing != "" {
		setEasing(&nk, a.Easing, curve)
	}
	i, found := slices.BinarySearchFunc(list, t, func(k timeline.Keyframe, t int) int { return k.T - t })
	if found {
		list[i] = nk
	} else {
		list = slices.Insert(list, i, nk)
	}
	if len(list) > maxKeyframes {
		return Errorf(CodeLimit, "Property %s of %s would have %d keyframes; the limit is %d.",
			a.Property, a.ItemID, len(list), maxKeyframes).
			WithHint("Remove keyframes with keyframe.remove first.")
	}
	ks[a.Property] = list
	it.Keyframes = ks
	c.Doc.Items[a.ItemID] = it
	c.SetEffectiveTime("at", it.Start+t, string(a.At))
	c.SetEffective("t", t)
	c.SetEffective("auto_first_keyframe", auto)
	c.SetEffective("replaced", found)
	return nil
}

// dropKeyframes removes a property's keyframes and writes value into its static field.
func dropKeyframes(it *timeline.Item, prop string, value float64) {
	ks := cloneKeyframes(it.Keyframes)
	delete(ks, prop)
	it.Keyframes = ks
	if len(ks) == 0 {
		it.Keyframes = nil
	}
	setStatic(it, prop, value)
}

type keyframeRemoveArgs struct {
	ItemID   string `json:"item_id"  pattern:"^itm_[0123456789abcdefghjkmnpqrstvwxyz]{10}$"`
	Property string `json:"property"                                                        enum:"x,y,scale,rotation,opacity,volume,brightness,contrast,saturation,temperature,tint,hue,mask_x,mask_y,mask_width,mask_height,mask_feather,crop_left,crop_top,crop_right,crop_bottom"`
	At       Time   `json:"at"                                                                                                                                                                                                                                                       desc:"Position of the keyframe on the project timeline, from the item start to its end inclusive."`
}

func applyKeyframeRemove(c *Context, a *keyframeRemoveArgs) error {
	if err := idForm("args.item_id", timeline.PrefixItem, a.ItemID); err != nil {
		return err
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
		return c.missingKeyframe(&it, a, list, it.Start+t)
	}
	removed := list[i]
	if len(list) == 1 {
		dropKeyframes(&it, a.Property, removed.Value)
	} else {
		ks := cloneKeyframes(it.Keyframes)
		ks[a.Property] = slices.Delete(slices.Clone(list), i, i+1)
		it.Keyframes = ks
	}
	c.Doc.Items[a.ItemID] = it
	c.SetEffectiveTime("at", it.Start+t, string(a.At))
	c.SetEffective("remaining", len(list)-1)
	return nil
}

func (c *Context) missingKeyframe(
	it *timeline.Item,
	a *keyframeRemoveArgs,
	list []timeline.Keyframe,
	frame int,
) *Error {
	e := Errorf(CodeNotFound, "Property %s of %s has no keyframes.", a.Property, a.ItemID).
		WithHint("Add one with keyframe.set.").WithPath("args.property")
	if len(list) > 0 {
		frames := make([]string, len(list))
		nearest := list[0].T + it.Start
		for i, k := range list {
			frames[i] = strconv.Itoa(it.Start + k.T)
			if abs(it.Start+k.T-frame) < abs(nearest-frame) {
				nearest = it.Start + k.T
			}
		}
		e = Errorf(CodeNotFound, "Property %s of %s has no keyframe at frame %d; keyframes are at frames %s.",
			a.Property, a.ItemID, frame, strings.Join(frames, ", ")).
			WithHint("Use at \"%df\" (%s), or list the keyframes with project_get.", nearest, c.seconds(nearest)).
			WithPath("args.at")
	}
	return e.WithDetail("kind", "keyframe").WithDetail("id", a.ItemID)
}

func abs(v int) int { return max(v, -v) }

type keyframeClearArgs struct {
	ItemID   string `json:"item_id"            pattern:"^itm_[0123456789abcdefghjkmnpqrstvwxyz]{10}$"`
	Property string `json:"property,omitempty"                                                        desc:"One property; default every animated property." enum:"x,y,scale,rotation,opacity,volume,brightness,contrast,saturation,temperature,tint,hue,mask_x,mask_y,mask_width,mask_height,mask_feather,crop_left,crop_top,crop_right,crop_bottom"`
}

func applyKeyframeClear(c *Context, a *keyframeClearArgs) error {
	if err := idForm("args.item_id", timeline.PrefixItem, a.ItemID); err != nil {
		return err
	}
	it, err := c.item(a.ItemID)
	if err != nil {
		return err
	}
	props := sortedProps(it.Keyframes)
	if a.Property != "" {
		if err := propApplies(&it, a.Property); err != nil {
			return err
		}
		props = []string{a.Property}
	}
	cleared := []string{}
	for _, p := range props {
		if list := it.Keyframes[p]; len(list) > 0 {
			dropKeyframes(&it, p, timeline.ValueAtProp(p, list, 0, 0))
			cleared = append(cleared, p)
		}
	}
	c.Doc.Items[a.ItemID] = it
	c.SetEffective("cleared", cleared)
	return nil
}
