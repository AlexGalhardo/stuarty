package command

import (
	"encoding/json"
	"fmt"
	"reflect"
	"slices"
	"strconv"
	"strings"

	"github.com/AlexGalhardo/stuarty/core/timeline"
)

func init() {
	Register(Define(Spec{
		Name:    "settings.set_fps",
		Summary: "Changes the project frame rate and rescales every time quantity.",
		Description: "Rescales every time quantity q of the project to round(q * new / old), then repairs what rounding " +
			"can break: durations that became 0, the source range, overlaps, keyframes, caption words, fades and " +
			"transitions. effective.adjusted lists every item changed by a repair. A rate is 1 to 120 fps with a " +
			"denominator of 1 or 1001.",
		Example: `{"fps": "24"}`,
		Errors:  []string{CodeValidation, CodeInvalidTime},
		Scope:   ScopeEdit,
	}, applySettingsSetFPS))
}

type setFPSArgs struct {
	FPS any `json:"fps" schema:"fps" desc:"\"30\", \"30000/1001\" or {\"num\": 30000, \"den\": 1001}; den is 1 or 1001, 1 to 120 fps."`
}

// parseFPS reads the three forms of an fps argument.
func parseFPS(v any) (timeline.Rational, *Error) {
	bad := func(shown string) *Error {
		return Errorf(
			CodeValidation,
			"args.fps %s is not a valid frame rate; den must be 1 or 1001 and the rate 1 to 120 fps.",
			shown,
		).
			WithHint(`Use "30", "24" or "30000/1001" for 29.97 fps.`).WithPath("args.fps")
	}
	var num, den int
	switch x := v.(type) {
	case string:
		n, d, hasDen := strings.Cut(x, "/")
		var e1, e2 error
		num, e1 = strconv.Atoi(n)
		den = 1
		if hasDen {
			den, e2 = strconv.Atoi(d)
		}
		if e1 != nil || e2 != nil {
			return timeline.Rational{}, bad(strconv.Quote(x))
		}
	case map[string]any:
		n, ok1 := x["num"].(json.Number)
		d, ok2 := x["den"].(json.Number)
		var e1, e2 error
		if ok1 && ok2 {
			num, e1 = strconv.Atoi(string(n))
			den, e2 = strconv.Atoi(string(d))
		}
		if !ok1 || !ok2 || e1 != nil || e2 != nil || len(x) != 2 {
			return timeline.Rational{}, bad("object")
		}
	default:
		return timeline.Rational{}, bad(kindOf(v))
	}
	if (den != 1 && den != 1001) || num < den || num > 120*den {
		return timeline.Rational{}, bad(fmt.Sprintf("%d/%d", num, den))
	}
	return timeline.Rational{Num: num, Den: den}, nil
}

func fpsString(f timeline.Rational) string {
	if f.Den == 1 {
		return strconv.Itoa(f.Num)
	}
	return fmt.Sprintf("%d/%d", f.Num, f.Den)
}

func gcd(a, b int64) int64 {
	for b != 0 {
		a, b = b, a%b
	}
	return a
}

// rescaler is round(q * rn / rd), halves up, in integer arithmetic.
type rescaler struct{ rn, rd int64 }

func (r rescaler) of(q int) int {
	return int((2*int64(q)*r.rn + r.rd) / (2 * r.rd))
}

// adjustment is one entry of effective.adjusted: the value of a field after the
// rescale and after the fix-up that changed it.
type adjustment struct {
	ID       string `json:"id"`
	Field    string `json:"field"`
	Reason   string `json:"reason"`
	Rescaled any    `json:"rescaled"`
	Final    any    `json:"final"`
}

// itemFacts lists the fields of an item that a fix-up can change, by name.
func itemFacts(it *timeline.Item) map[string]any {
	f := map[string]any{
		"start": it.Start, "duration": it.Duration, "source_in": it.SourceIn,
		"fade_in": it.FadeIn, "fade_out": it.FadeOut, "transition_in.duration": 0, "transition_out.duration": 0,
	}
	if it.TransitionIn != nil {
		f["transition_in.duration"] = it.TransitionIn.Duration
	}
	if it.TransitionOut != nil {
		f["transition_out.duration"] = it.TransitionOut.Duration
	}
	for prop, kfs := range it.Keyframes {
		ts := make([]int, len(kfs))
		for i, k := range kfs {
			ts[i] = k.T
		}
		f["keyframes."+prop] = ts
	}
	if it.Type == timeline.ItemCaption {
		ws := make([][2]int, len(it.Words))
		for i, w := range it.Words {
			ws[i] = [2]int{w.Start, w.Duration}
		}
		f["words"] = ws
	}
	return f
}

func sortedFactNames(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	return keys
}

func snapshotFacts(doc *timeline.Document) map[string]map[string]any {
	out := make(map[string]map[string]any, len(doc.Items))
	for id := range doc.Items {
		it := doc.Items[id]
		out[id] = itemFacts(&it)
	}
	return out
}

// changesSince lists, by item id and field name, what differs from the snapshot.
func (c *Context) changesSince(before map[string]map[string]any, reason string) []adjustment {
	var out []adjustment
	for _, id := range c.sortedItems() {
		it := c.Doc.Items[id]
		after, old := itemFacts(&it), before[id]
		for _, k := range sortedFactNames(after) {
			if o, had := old[k]; !had || !reflect.DeepEqual(o, after[k]) {
				out = append(out, adjustment{id, k, reason, o, after[k]})
			}
		}
		for _, k := range sortedFactNames(old) {
			if _, still := after[k]; !still {
				out = append(out, adjustment{id, k, reason, old[k], nil})
			}
		}
	}
	return out
}

func (c *Context) sortedItems() []string {
	ids := make([]string, 0, len(c.Doc.Items))
	for id := range c.Doc.Items {
		ids = append(ids, id)
	}
	slices.Sort(ids)
	return ids
}

func applySettingsSetFPS(c *Context, a *setFPSArgs) error {
	next, err := parseFPS(a.FPS)
	if err != nil {
		return err
	}
	old := c.Doc.Settings.FPS
	rn, rd := int64(next.Num)*int64(old.Den), int64(next.Den)*int64(old.Num)
	g := gcd(rn, rd)
	c.SetEffective("fps", map[string]any{"num": next.Num, "den": next.Den})
	c.SetEffective("ratio", map[string]any{"num": rn / g, "den": rd / g})

	var adjusted []adjustment
	c.rescaleTimes(rescaler{rn, rd}, &adjusted)
	if err := c.checkEnds(old, next); err != nil {
		return err
	}
	c.Doc.Settings.FPS = next

	run := func(reason string, fix func()) {
		before := snapshotFacts(c.Doc)
		fix()
		adjusted = append(adjusted, c.changesSince(before, reason)...)
	}
	run("source_range", c.fixSourceRanges)
	run("overlap", c.fixOverlaps)
	if err := c.checkEnds(old, next); err != nil {
		return err
	}
	run("keyframe_collision", c.fixKeyframeCollisions)
	run("keyframe_cut", c.fixKeyframeCuts)
	c.fixMarkers()
	run("word_overlap", c.fixWordOverlaps)
	run("word_clamp", c.fixWordClamps)
	run("fit", c.fixFit)

	if adjusted == nil {
		adjusted = []adjustment{}
	}
	c.SetEffective("adjusted", adjusted)
	return nil
}

// rescaleTimes is step a: every time quantity becomes round(q * r); a duration that
// became 0 becomes 1 and is reported as became_zero.
func (c *Context) rescaleTimes(r rescaler, adjusted *[]adjustment) {
	for _, id := range c.sortedItems() {
		it := c.Doc.Items[id]
		zero := func(field string, v *int) {
			if *v == 0 {
				*adjusted = append(*adjusted, adjustment{id, field, "became_zero", 0, 1})
				*v = 1
			}
		}
		it.Start, it.Duration, it.SourceIn = r.of(it.Start), r.of(it.Duration), r.of(it.SourceIn)
		it.FadeIn, it.FadeOut = r.of(it.FadeIn), r.of(it.FadeOut)
		zero("duration", &it.Duration)
		mRescale(&it, r.of)
		for _, t := range []struct {
			field string
			p     **timeline.Transition
		}{{"transition_in.duration", &it.TransitionIn}, {"transition_out.duration", &it.TransitionOut}} {
			if *t.p != nil {
				cp := **t.p
				cp.Duration = r.of(cp.Duration)
				zero(t.field, &cp.Duration)
				*t.p = &cp
			}
		}
		if len(it.Keyframes) > 0 {
			ks := make(timeline.Keyframes, len(it.Keyframes))
			for prop, kfs := range it.Keyframes {
				out := slices.Clone(kfs)
				for i := range out {
					out[i].T = r.of(out[i].T)
				}
				ks[prop] = out
			}
			it.Keyframes = ks
		}
		if len(it.Words) > 0 {
			ws := slices.Clone(it.Words)
			for i := range ws {
				// The two ends are rounded, not the start and the length: words that touch keep
				// touching and a rounding error never adds up from one word to the next (VV-058).
				start, end := r.of(ws[i].Start), r.of(ws[i].Start+ws[i].Duration)
				ws[i].Start, ws[i].Duration = start, end-start
				zero(fmt.Sprintf("words[%d].duration", i), &ws[i].Duration)
			}
			it.Words = ws
		}
		c.Doc.Items[id] = it
	}
}

// checkEnds is the R7 check after the steps a and c: nothing is applied when an item
// ends past frame 108,000.
func (c *Context) checkEnds(old, next timeline.Rational) *Error {
	for _, id := range c.sortedItems() {
		it := c.Doc.Items[id]
		end := it.Start + it.Duration
		if end <= timeline.MaxFrames {
			continue
		}
		latest := int64(timeline.MaxFrames) * int64(old.Num) * int64(next.Den) / (int64(old.Den) * int64(next.Num))
		return Errorf(
			CodeInvalidTime,
			"Item %s would end at frame %d at %s fps; the limit is %d.",
			id,
			end,
			fpsString(next),
			timeline.MaxFrames,
		).
			WithHint("Shorten the item so that it ends at or before frame %d at %s fps first, or keep %s fps.",
				latest, fpsString(old), fpsString(old)).
			WithPath("args.fps")
	}
	return nil
}

// fixSourceRanges is step b: the source range at the new rate (R9).
func (c *Context) fixSourceRanges() {
	for _, id := range c.sortedItems() {
		it := c.Doc.Items[id]
		a, ok := c.sourceAsset(&it)
		if !ok || it.Freeze {
			continue
		}
		total, ok := timeline.AssetDuration(a, c.Doc.Settings.FPS)
		if !ok {
			continue
		}
		sp := it.Speed
		if it.SourceIn+ceilDiv(it.Duration*sp.Num, sp.Den) <= total {
			continue
		}
		it.Duration = maxDur(total, it.SourceIn, sp)
		if it.Duration < 1 {
			it.SourceIn = max(0, total-ceilDiv(sp.Num, sp.Den))
			it.Duration = max(maxDur(total, it.SourceIn, sp), 1)
		}
		c.Doc.Items[id] = it
	}
}

// fixOverlaps is step c: one pass per track over the items sorted by start and id.
func (c *Context) fixOverlaps() {
	tracks := map[string]bool{}
	for _, id := range c.sortedItems() {
		tracks[c.Doc.Items[id].Track] = true
	}
	for track := range tracks {
		var prev *timeline.Item
		for _, id := range c.trackItems(track) {
			x := c.Doc.Items[id]
			if prev != nil && prev.Start+prev.Duration > x.Start {
				if x.Start > prev.Start {
					prev.Duration = x.Start - prev.Start
				} else {
					s := prev.Start + 1 - x.Start
					prev.Duration = 1
					x.Start += s
					x.Duration = max(1, x.Duration-s)
				}
				c.Doc.Items[prev.ID] = *prev
				c.Doc.Items[id] = x
			}
			cp := x
			prev = &cp
		}
	}
}

// fixKeyframeCollisions is step d, first part: keyframes that share a t are reduced
// to the last of them.
func (c *Context) fixKeyframeCollisions() {
	for _, id := range c.sortedItems() {
		it := c.Doc.Items[id]
		changed := false
		ks := make(timeline.Keyframes, len(it.Keyframes))
		for prop, kfs := range it.Keyframes {
			var out []timeline.Keyframe
			for i, k := range kfs {
				if i+1 < len(kfs) && kfs[i+1].T == k.T {
					changed = true
					continue
				}
				out = append(out, k)
			}
			ks[prop] = out
		}
		if changed {
			it.Keyframes = ks
			c.Doc.Items[id] = it
		}
	}
}

// fixKeyframeCuts is step d, second part: R12 with L = duration when keyframes lie
// beyond it.
func (c *Context) fixKeyframeCuts() {
	for _, id := range c.sortedItems() {
		it := c.Doc.Items[id]
		beyond := false
		for _, kfs := range it.Keyframes {
			beyond = beyond || kfs[len(kfs)-1].T > it.Duration
		}
		if beyond {
			it.Keyframes = cloneKeyframes(it.Keyframes)
			kCut(&it, it.Duration)
			c.Doc.Items[id] = it
		}
	}
}

func cloneKeyframes(k timeline.Keyframes) timeline.Keyframes {
	out := make(timeline.Keyframes, len(k))
	for prop, kfs := range k {
		out[prop] = slices.Clone(kfs)
	}
	return out
}

// fixWordOverlaps is step e, first part: a word starts no earlier than the end of
// the one before it.
func (c *Context) fixWordOverlaps() {
	for _, id := range c.sortedItems() {
		it := c.Doc.Items[id]
		if len(it.Words) == 0 {
			continue
		}
		ws := slices.Clone(it.Words)
		end := 0
		for i := range ws {
			ws[i].Start = max(ws[i].Start, end)
			end = ws[i].Start + ws[i].Duration
		}
		it.Words = ws
		c.Doc.Items[id] = it
	}
}

// fixWordClamps is step e, second part: a word that ends after the item is shortened
// to end there, a word with no frame left is removed, and a caption keeps at least
// one word.
func (c *Context) fixWordClamps() {
	for _, id := range c.sortedItems() {
		it := c.Doc.Items[id]
		if len(it.Words) == 0 {
			continue
		}
		var kept []timeline.Word
		for _, w := range it.Words {
			if w.Start >= it.Duration {
				continue
			}
			w.Duration = min(w.Duration, it.Duration-w.Start)
			kept = append(kept, w)
		}
		if len(kept) == 0 {
			kept = []timeline.Word{{Text: it.Words[0].Text, Start: 0, Duration: 1}}
		}
		it.Words = kept
		it.Breaks = slices.DeleteFunc(slices.Clone(it.Breaks), func(b int) bool { return b >= len(kept) })
		c.Doc.Items[id] = it
	}
}

// fixFit is step f: the fit fix of R14 against the final duration.
func (c *Context) fixFit() {
	for _, id := range c.sortedItems() {
		it := c.Doc.Items[id]
		fitFix(&it)
		c.Doc.Items[id] = it
	}
}

// fixMarkers drops the markers that lie past the final duration of their item; like
// the marker rescale it is not reported in adjusted (markers do not alter the
// result, SDD-001 section 3.4).
func (c *Context) fixMarkers() {
	for _, id := range c.sortedItems() {
		if it := c.Doc.Items[id]; len(it.Markers) > 0 {
			mCut(&it, it.Duration)
			c.Doc.Items[id] = it
		}
	}
}
