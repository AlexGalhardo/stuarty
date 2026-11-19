package command

import (
	"slices"

	"github.com/AlexGalhardo/stuarty/core/timeline"
)

// Keyframe rules R10 to R13 of SDD-002: the value of an animated property, and how
// keyframes follow a trim or a split.

func sortedProps(k timeline.Keyframes) []string {
	props := make([]string, 0, len(k))
	for p := range k {
		props = append(props, p)
	}
	slices.Sort(props)
	return props
}

// setStatic writes the static field of an animated property.
func setStatic(it *timeline.Item, prop string, v float64) {
	v = round3(v)
	switch prop {
	case "x":
		it.X = &v
	case "y":
		it.Y = &v
	case "scale":
		it.Scale = v
	case "rotation":
		it.Rotation = v
	case "opacity":
		it.Opacity = v
	case "volume":
		it.Volume = v
	case "brightness":
		it.Brightness = v
	case "contrast":
		it.Contrast = v
	case "saturation":
		it.Saturation = v
	case "temperature":
		it.Temperature = v
	case "tint":
		it.Tint = v
	case "hue":
		it.Hue = v
	default:
		if isMaskProp(prop) {
			it.SetMaskValue(prop, v)
		} else if isCropProp(prop) {
			it.SetCropValue(prop, v)
		}
	}
}

func easingOrLinear(e string) string {
	if e == "" {
		return "linear"
	}
	return e
}

// kShift is R11: the item loses its first d frames (d < 0: it gains -d).
func kShift(it *timeline.Item, d int) {
	mShift(it, d)
	for _, prop := range sortedProps(it.Keyframes) {
		old := it.Keyframes[prop]
		v0 := 0.0
		if d > 0 {
			v0 = timeline.ValueAt(old, d, 0)
		}
		var kept []timeline.Keyframe
		dropped, lastEasing := false, "linear"
		for _, k := range old {
			k.T -= d
			if k.T < 0 {
				dropped, lastEasing = true, easingOrLinear(k.Easing)
				continue
			}
			kept = append(kept, k)
		}
		switch {
		case len(kept) == 0:
			delete(it.Keyframes, prop)
			setStatic(it, prop, v0)
		case dropped && kept[0].T > 0:
			first := timeline.NewKeyframe(0, v0)
			first.Easing = lastEasing
			it.Keyframes[prop] = append([]timeline.Keyframe{first}, kept...)
		default:
			it.Keyframes[prop] = kept
		}
	}
	if len(it.Keyframes) == 0 {
		it.Keyframes = nil
	}
}

// kCut is R12: the item is cut to l frames at the back.
func kCut(it *timeline.Item, l int) {
	mCut(it, l)
	for _, prop := range sortedProps(it.Keyframes) {
		old := it.Keyframes[prop]
		var kept []timeline.Keyframe
		for _, k := range old {
			if k.T <= l {
				kept = append(kept, k)
			}
		}
		switch {
		case len(kept) == len(old):
		case len(kept) == 0:
			delete(it.Keyframes, prop)
			setStatic(it, prop, timeline.ValueAt(old, l, 0))
		default:
			if kept[len(kept)-1].T < l {
				kept = append(kept, timeline.NewKeyframe(l, timeline.ValueAt(old, l, 0)))
			}
			it.Keyframes[prop] = kept
		}
	}
	if len(it.Keyframes) == 0 {
		it.Keyframes = nil
	}
}

// kSplit is R13: it divides the keyframes of it at relative frame tc and returns
// the keyframes and static values of the left and the right part as two items
// carrying only those fields.
func kSplit(it *timeline.Item, tc int) (left, right timeline.Item) {
	left, right = timeline.CloneItem(*it), timeline.CloneItem(*it)
	left.Keyframes, right.Keyframes = timeline.Keyframes{}, timeline.Keyframes{}
	mSplit(&left, &right, tc)
	for _, prop := range sortedProps(it.Keyframes) {
		kfs := it.Keyframes[prop]
		var before, after []timeline.Keyframe
		var at *timeline.Keyframe
		for i := range kfs {
			switch k := kfs[i]; {
			case k.T < tc:
				before = append(before, k)
			case k.T == tc:
				at = &kfs[i]
			default:
				after = append(after, k)
			}
		}
		v := timeline.ValueAt(kfs, tc, 0)

		l := slices.Clone(before)
		if at != nil {
			l = append(l, *at)
		} else if len(after) > 0 && len(before) > 0 {
			l = append(l, timeline.NewKeyframe(tc, v))
		}
		if len(l) == 0 {
			setStatic(&left, prop, v)
		} else {
			left.Keyframes[prop] = l
		}

		if len(after) == 0 {
			setStatic(&right, prop, v)
			continue
		}
		var r []timeline.Keyframe
		if len(before) > 0 || at != nil {
			first := timeline.NewKeyframe(0, v)
			switch {
			case at != nil:
				first.Easing = easingOrLinear(at.Easing)
			default:
				first.Easing = easingOrLinear(before[len(before)-1].Easing)
			}
			r = append(r, first)
		}
		for _, k := range after {
			k.T -= tc
			r = append(r, k)
		}
		right.Keyframes[prop] = r
	}
	for _, p := range []*timeline.Item{&left, &right} {
		if len(p.Keyframes) == 0 {
			p.Keyframes = nil
		}
	}
	return left, right
}
