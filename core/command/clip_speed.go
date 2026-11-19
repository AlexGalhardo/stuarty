package command

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/AlexGalhardo/stuarty/core/timeline"
)

func init() {
	Register(Define(Spec{
		Name:    "clip.set_speed",
		Summary: "Changes the speed of a clip or audio item, constant or along a curve, or plays it backwards.",
		Description: "Sets either speed, from 0.25x to 4x, or curve, 2 to 8 points {t, speed} (speed is text like \"0.5x\") with t the fraction of the " +
			"item from 0 to 1 (first 0, last 1) and a linear ramp between points; a speed clears the curve. Rescales the " +
			"duration, the fades, the transitions and the keyframes by the ratio of the old to the new mean speed. With " +
			"ripple track the later items of the track follow the change in length. Captions linked to the clip are " +
			"listed, not changed. reverse true plays the same source range backwards (false restores it), alone or " +
			"with a speed or a curve.",
		Example: `{"item_id": "itm_m1n2p3q4r5", "curve": [{"t": 0, "speed": "1x"}, {"t": 0.5, "speed": "3x"}, {"t": 1, "speed": "1x"}]}`,
		Errors: []string{
			CodeValidation, CodeNotFound, CodeTypeMismatch, CodeOutOfSource, CodeInvalidTime, CodeOverlap,
		},
		Scope: ScopeEdit,
	}, applyClipSetSpeed))
}

type clipSpeedArgs struct {
	ItemID  string           `json:"item_id"           desc:"A clip of a video asset, or an audio item."                                              pattern:"^itm_[0123456789abcdefghjkmnpqrstvwxyz]{10}$"`
	Speed   any              `json:"speed,omitempty"   desc:"\"1.5x\" (up to 3 decimals) or {\"num\": 3, \"den\": 2}; 1/4 to 4. Not with curve."                                                             schema:"speed"`
	Curve   []speedPointArgs `json:"curve,omitempty"   desc:"Speed ramp: 2 to 8 points, linear between them, first t 0 and last t 1. Not with speed."                                                                       minitems:"2" maxitems:"8"`
	Reverse *bool            `json:"reverse,omitempty" desc:"true plays the source range backwards; false plays it forwards."`
	Ripple  string           `json:"ripple,omitempty"  desc:"none (default) or track: shift the later items of the track."                                                                                                                            enum:"none,track"`
}

type speedPointArgs struct {
	T     float64 `json:"t"     desc:"Position in the item, 0 to 1, up to 3 decimals."                       min:"0" max:"1"`
	Speed any     `json:"speed" desc:"Text like \"0.5x\" (up to 3 decimals, 1/4 to 4), never a bare number."                 schema:"speed"`
}

// parseSpeed reads a speed argument and returns it in lowest terms.
func parseSpeed(v any) (timeline.Rational, *Error) { return parseSpeedAt(v, "args.speed") }

func parseSpeedAt(v any, path string) (timeline.Rational, *Error) {
	bad := func(format string, a ...any) *Error {
		return Errorf(CodeValidation, format, a...).
			WithHint(`Use a speed from "0.25x" to "4x".`).WithPath(path)
	}
	var num, den int
	switch x := v.(type) {
	case string:
		body, ok := strings.CutSuffix(x, "x")
		whole, frac, _ := strings.Cut(body, ".")
		n, e1 := strconv.Atoi(whole + frac)
		if !ok || body == "" || len(frac) > 3 || e1 != nil || strings.ContainsAny(body, "+- ") ||
			whole == "" && frac == "" {
			return timeline.Rational{}, bad("%s %q is not a speed like \"1.5x\".", path, x)
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
			return timeline.Rational{}, bad("%s is not a rational {\"num\", \"den\"} of positive integers.", path)
		}
	default:
		return timeline.Rational{}, bad("%s must be a string or an object, got %s.", path, kindOf(v))
	}
	g := int(gcd(int64(num), int64(den)))
	if g == 0 {
		return timeline.Rational{}, bad("%s is 0.", path)
	}
	num, den = num/g, den/g
	switch {
	case num*4 > den*16:
		return timeline.Rational{}, bad("%s %s is above the maximum 4x.", path, speedWord(num, den))
	case num*4 < den:
		return timeline.Rational{}, bad("%s %s is below the minimum 0.25x.", path, speedWord(num, den))
	}
	return timeline.Rational{Num: num, Den: den}, nil
}

func speedWord(num, den int) string {
	return strconv.FormatFloat(float64(num)/float64(den), 'f', -1, 64) + "x"
}

// parseCurve reads the points of a speed curve and checks them against the limits of
// timeline.CurveError; the speeds of a curve are multiples of 0.001.
func parseCurve(in []speedPointArgs) ([]timeline.SpeedPoint, *Error) {
	pts := make([]timeline.SpeedPoint, len(in))
	for i, p := range in {
		path := fmt.Sprintf("args.curve[%d]", i)
		sp, err := parseSpeedAt(p.Speed, path+".speed")
		if err != nil {
			return nil, err
		}
		pts[i] = timeline.SpeedPoint{T: p.T, Speed: sp}
	}
	if msg := timeline.CurveError(pts); msg != "" {
		return nil, Errorf(CodeValidation, "args.curve is not a speed curve: %s.", msg).
			WithHint(`Use 2 to 8 points like {"t": 0, "speed": "1x"} ... {"t": 1, "speed": "2x"}.`).
			WithPath("args.curve")
	}
	for i := range pts {
		pts[i].T = round3(pts[i].T)
	}
	return pts, nil
}

func applyClipSetSpeed(c *Context, a *clipSpeedArgs) error {
	if err := atLeastOne(
		[]string{"speed", "curve", "reverse"},
		a.Speed != nil,
		len(a.Curve) > 0,
		a.Reverse != nil,
	); err != nil {
		return err
	}
	if a.Speed != nil && len(a.Curve) > 0 {
		return Errorf(CodeValidation, "Give either speed or curve, not both.").
			WithHint("A speed clears the curve; a curve sets the speed to its mean.").WithPath("args.curve")
	}
	var next timeline.Rational
	var curve []timeline.SpeedPoint
	var err *Error
	given := a.Speed != nil || len(a.Curve) > 0
	switch {
	case len(a.Curve) > 0:
		if curve, err = parseCurve(a.Curve); err == nil {
			next = timeline.CurveMean(curve)
		}
	case a.Speed != nil:
		next, err = parseSpeed(a.Speed)
	}
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
	if it.Freeze {
		return Errorf(CodeTypeMismatch, "Item %s is a freeze frame, which has no speed.", a.ItemID).
			WithHint("Change its duration with item.trim, or slip the held frame with item.slip.").
			WithPath("args.item_id")
	}
	if !given {
		next, curve = it.Speed, it.SpeedCurve
	}
	old := it.Speed
	oldDur, oldEnd := it.Duration, itemEnd(&it)
	newDur, clamped, err := c.speedDuration(&it, asset, next)
	if err != nil {
		return err
	}

	var adjusted []adjustment
	it.Speed, it.SpeedCurve, it.Duration = next, curve, newDur
	if a.Reverse != nil {
		it.Reverse = *a.Reverse
	}
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
	if a.Reverse != nil {
		c.SetEffective("reverse", it.Reverse)
	}
	if len(curve) > 0 {
		c.SetEffective("curve_points", len(curve))
	}
	c.SetEffective("duration", map[string]any{"before": oldDur, "after": newDur})
	c.SetEffective("result_duration", newDur)
	if gap := c.speedGap(&it, a.Ripple == "track", oldEnd); gap != nil {
		c.SetEffective("gap", gap)
	}
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
	mRescale(it, r.of)
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

// speedGap describes the gap that a shorter item leaves before the next item of its track
// when ripple is off (VV-163); nil when there is none.
func (c *Context) speedGap(it *timeline.Item, ripple bool, oldEnd int) map[string]any {
	if ripple || itemEnd(it) >= oldEnd {
		return nil
	}
	next := -1
	for _, id := range c.trackItems(it.Track) {
		if o := c.Doc.Items[id]; id != it.ID && o.Start >= oldEnd && (next < 0 || o.Start < next) {
			next = o.Start
		}
	}
	if next < 0 {
		return nil
	}
	return map[string]any{
		"start":  itemEnd(it),
		"frames": next - itemEnd(it),
		"hint":   `The item got shorter and left a gap before the next item; use ripple "track" to close it, or timeline.close_gap.`,
	}
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
			return 0, err.WithDetail("result_duration", it.Duration).
				WithHint("At this speed the item is %d frames long (a curve counts at its mean speed); use ripple \"track\" to move the later items, or shorten the item first.", it.Duration)
		}
	}
	return shift, nil
}
