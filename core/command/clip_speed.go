package command

import (
	"encoding/json"
	"strconv"
	"strings"

	"github.com/AlexGalhardo/vukem-video/core/timeline"
)

func init() {
	Register(Define(Spec{
		Name:    "clip.set_speed",
		Summary: "Changes the speed of a clip or audio item while keeping the same source range.",
		Description: "Sets the speed, from 0.25x to 4x, and rescales the duration, the fades, the transitions and the " +
			"keyframes by the same ratio. With ripple track the later items of the track follow the change in length. " +
			"Captions linked to the clip are listed, not changed.",
		Example: `{"item_id": "itm_m1n2p3q4r5", "speed": "1.5x", "ripple": "track"}`,
		Errors: []string{
			CodeValidation, CodeNotFound, CodeTypeMismatch, CodeOutOfSource, CodeInvalidTime, CodeOverlap,
		},
		Scope: ScopeEdit,
		Phase: Alpha,
	}, applyClipSetSpeed))
}

type clipSpeedArgs struct {
	ItemID string `json:"item_id"          desc:"A clip of a video asset, or an audio item."`
	Speed  any    `json:"speed"            desc:"\"1.5x\" (up to 3 decimals) or {\"num\": 3, \"den\": 2}; 1/4 to 4." schema:"speed"`
	Ripple string `json:"ripple,omitempty" desc:"none (default) or track: shift the later items of the track."                      enum:"none,track"`
}

// parseSpeed reads a speed argument and returns it in lowest terms.
func parseSpeed(v any) (timeline.Rational, *Error) {
	bad := func(format string, a ...any) *Error {
		return Errorf(CodeValidation, format, a...).
			WithHint(`Use a speed from "0.25x" to "4x".`).WithPath("args.speed")
	}
	var num, den int
	switch x := v.(type) {
	case string:
		body, ok := strings.CutSuffix(x, "x")
		whole, frac, _ := strings.Cut(body, ".")
		n, e1 := strconv.Atoi(whole + frac)
		if !ok || body == "" || len(frac) > 3 || e1 != nil || strings.ContainsAny(body, "+- ") ||
			whole == "" && frac == "" {
			return timeline.Rational{}, bad("args.speed %q is not a speed like \"1.5x\".", x)
		}
		num, den = n, 1
		for range frac {
			den *= 10
		}
	case map[string]any:
		n, ok1 := x["num"].(json.Number)
		d, ok2 := x["den"].(json.Number)
		var e1, e2 error
		if ok1 && ok2 {
			num, e1 = strconv.Atoi(string(n))
			den, e2 = strconv.Atoi(string(d))
		}
		if !ok1 || !ok2 || e1 != nil || e2 != nil || len(x) != 2 || num < 1 || den < 1 {
			return timeline.Rational{}, bad("args.speed is not a rational {\"num\", \"den\"} of positive integers.")
		}
	default:
		return timeline.Rational{}, bad("args.speed must be a string or an object, got %s.", kindOf(v))
	}
	g := int(gcd(int64(num), int64(den)))
	if g == 0 {
		return timeline.Rational{}, bad("args.speed is 0.")
	}
	num, den = num/g, den/g
	switch {
	case num*4 > den*16:
		return timeline.Rational{}, bad("args.speed %s is above the maximum 4x.", speedWord(num, den))
	case num*4 < den:
		return timeline.Rational{}, bad("args.speed %s is below the minimum 0.25x.", speedWord(num, den))
	}
	return timeline.Rational{Num: num, Den: den}, nil
}

func speedWord(num, den int) string {
	return strconv.FormatFloat(float64(num)/float64(den), 'f', -1, 64) + "x"
}

func applyClipSetSpeed(c *Context, a *clipSpeedArgs) error {
	next, err := parseSpeed(a.Speed)
	if err != nil {
		return err
	}
	if err := idForm("args.item_id", timeline.PrefixItem, a.ItemID); err != nil {
		return err
	}
	it, err := c.item(a.ItemID)
	if err != nil {
		return err
	}
	asset, ok := c.sourceAsset(&it)
	if !ok {
		return Errorf(CodeTypeMismatch, "Item %s is %s item and has no speed.", a.ItemID, article(it.Type)).
			WithHint("Change the duration with item.trim instead.").WithPath("args.item_id")
	}
	old := it.Speed
	oldDur, oldEnd := it.Duration, itemEnd(&it)
	newDur, clamped, err := c.speedDuration(&it, asset, next)
	if err != nil {
		return err
	}

	var adjusted []adjustment
	it.Speed, it.Duration = next, newDur
	adjusted = append(adjusted, rescaleForSpeed(&it, rescaler{
		int64(old.Num) * int64(next.Den), int64(old.Den) * int64(next.Num),
	})...)
	c.Doc.Items[a.ItemID] = it
	for _, fix := range []struct {
		reason string
		run    func(*timeline.Item)
	}{
		{"keyframe_cut", clampKeyframes},
		{"keyframe_collision", dropCollisions},
		{"fit", fitFix},
	} {
		before := snapshotFacts(c.Doc)
		cur := c.Doc.Items[a.ItemID]
		fix.run(&cur)
		c.Doc.Items[a.ItemID] = cur
		adjusted = append(adjusted, c.changesSince(before, fix.reason)...)
	}
	it = c.Doc.Items[a.ItemID]

	shift, err := c.rippleSpeed(&it, a.Ripple == "track", oldEnd, newDur-oldDur)
	if err != nil {
		return err
	}

	var linked []string
	for _, id := range c.sortedItems() {
		if o := c.Doc.Items[id]; o.Type == timeline.ItemCaption && o.LinkedItem == a.ItemID {
			linked = append(linked, id)
		}
	}
	if adjusted == nil {
		adjusted = []adjustment{}
	}
	if linked == nil {
		linked = []string{}
	}
	c.SetEffective("speed", map[string]any{"num": next.Num, "den": next.Den})
	c.SetEffective("duration", map[string]any{"before": oldDur, "after": newDur})
	c.SetEffective("shift", shift)
	c.SetEffective("clamped", clamped)
	c.SetEffective("adjusted", adjusted)
	c.SetEffective("linked_captions", linked)
	return nil
}

// clampKeyframes moves a keyframe past the end of the item to the end.
func clampKeyframes(it *timeline.Item) {
	if len(it.Keyframes) == 0 {
		return
	}
	ks := cloneKeyframes(it.Keyframes)
	for _, kfs := range ks {
		for i := range kfs {
			kfs[i].T = min(kfs[i].T, it.Duration)
		}
	}
	it.Keyframes = ks
}

// dropCollisions reduces keyframes that share a t to the last of them.
func dropCollisions(it *timeline.Item) {
	if len(it.Keyframes) == 0 {
		return
	}
	ks := make(timeline.Keyframes, len(it.Keyframes))
	for prop, kfs := range it.Keyframes {
		var out []timeline.Keyframe
		for i, k := range kfs {
			if i+1 < len(kfs) && kfs[i+1].T == k.T {
				continue
			}
			out = append(out, k)
		}
		ks[prop] = out
	}
	it.Keyframes = ks
}

// speedDuration is the new duration of postcondition 1: the ceiling of the rescaled
// length, reduced to what the asset allows when rounding would read past its end.
func (c *Context) speedDuration(it *timeline.Item, asset timeline.Asset, next timeline.Rational) (int, any, *Error) {
	old := it.Speed
	newDur := int((int64(it.Duration)*int64(old.Num)*int64(next.Den) + int64(old.Den)*int64(next.Num) - 1) /
		(int64(old.Den) * int64(next.Num)))
	total, ok := timeline.AssetDuration(asset, c.Doc.Settings.FPS)
	if !ok || it.SourceIn+ceilDiv(newDur*next.Num, next.Den) <= total {
		return newDur, nil, nil
	}
	fit := maxDur(total, it.SourceIn, next)
	if fit < 1 {
		return 0, nil, Errorf(
			CodeOutOfSource,
			"At speed %s the clip needs %d source frames from frame %d; asset %s has %d.",
			speedWord(next.Num, next.Den), ceilDiv(next.Num, next.Den), it.SourceIn, it.Asset, total,
		).
			WithHint("Use item.trim to move source_in earlier, or choose a lower speed.").
			WithDetail("available", map[string]any{"asset_duration": total, "source_in": it.SourceIn, "max_duration": 0}).
			WithPath("args.speed")
	}
	return fit, map[string]any{"duration": map[string]any{"from": newDur, "to": fit}}, nil
}

// rescaleForSpeed scales the fades, the transitions and the keyframe times of the item
// by r; a transition below 1 frame becomes 1 and is reported.
func rescaleForSpeed(it *timeline.Item, r rescaler) []adjustment {
	var adjusted []adjustment
	it.FadeIn, it.FadeOut = r.of(it.FadeIn), r.of(it.FadeOut)
	for _, t := range []struct {
		field string
		p     **timeline.Transition
	}{{"transition_in.duration", &it.TransitionIn}, {"transition_out.duration", &it.TransitionOut}} {
		if *t.p == nil {
			continue
		}
		cp := **t.p
		cp.Duration = r.of(cp.Duration)
		if cp.Duration < 1 {
			adjusted = append(adjusted, adjustment{it.ID, t.field, "became_zero", cp.Duration, 1})
			cp.Duration = 1
		}
		*t.p = &cp
	}
	if len(it.Keyframes) > 0 {
		ks := cloneKeyframes(it.Keyframes)
		for _, kfs := range ks {
			for i := range kfs {
				kfs[i].T = r.of(kfs[i].T)
			}
		}
		it.Keyframes = ks
	}
	return adjusted
}

// rippleSpeed shifts the later items of the track when asked, then refuses what the
// change leaves wrong: an item past frame 108,000, or a longer item on its neighbour.
func (c *Context) rippleSpeed(it *timeline.Item, ripple bool, oldEnd, shift int) (int, *Error) {
	if !ripple {
		shift = 0
	}
	if ripple {
		for _, id := range c.trackItems(it.Track) {
			if o := c.Doc.Items[id]; id != it.ID && o.Start >= oldEnd {
				o.Start += shift
				c.Doc.Items[id] = o
			}
		}
	}
	for _, id := range c.sortedItems() {
		if o := c.Doc.Items[id]; itemEnd(&o) > timeline.MaxFrames {
			return 0, Errorf(
				CodeInvalidTime,
				"Item %s would end at frame %d; the limit is %d.",
				id,
				itemEnd(&o),
				timeline.MaxFrames,
			).
				WithHint("Use a higher speed, or shorten the clip first.").
				WithPath("args.speed")
		}
	}
	if !ripple {
		if err := c.checkOverlap(it.ID, it.Track, it.Start, it.Duration); err != nil {
			return 0, err
		}
	}
	return shift, nil
}
