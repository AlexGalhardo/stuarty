package command

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/AlexGalhardo/vukem-video/core/timeline"
)

// TestKeyframeHelpers reproduces the worked values of R11 to R13.
func TestKeyframeHelpers(t *testing.T) {
	item := func() timeline.Item {
		it := timeline.NewItem("itm_k000000001", timeline.ItemText, vidTr, 0, 100)
		it.Keyframes = timeline.Keyframes{"scale": {kf(0, 1), kf(60, 3)}}
		return it
	}
	// R13: cut at 30, the cut keyframe carries the interpolated value on both parts.
	it := item()
	l, r := kSplit(&it, 30)
	if got := l.Keyframes["scale"]; len(got) != 2 || got[1].T != 30 || got[1].Value != 2 {
		t.Errorf("left %+v", got)
	}
	if got := r.Keyframes["scale"]; len(got) != 2 || got[0].T != 0 || got[0].Value != 2 || got[1].T != 30 ||
		got[1].Value != 3 {
		t.Errorf("right %+v", got)
	}
	// R11: losing the first 30 frames starts the keyframes at the interpolated value.
	it = item()
	kShift(&it, 30)
	if got := it.Keyframes["scale"]; len(got) != 2 || got[0].T != 0 || got[0].Value != 2 || got[1].T != 30 {
		t.Errorf("shift %+v", got)
	}
	// R12: cutting at 30 adds a keyframe with the value there.
	it = item()
	kCut(&it, 30)
	if got := it.Keyframes["scale"]; len(got) != 2 || got[1].T != 30 || got[1].Value != 2 {
		t.Errorf("cut %+v", got)
	}
}

func TestSetStaticWritesEveryProperty(t *testing.T) {
	it := timeline.NewItem("itm_k000000001", timeline.ItemClip, vidTr, 0, 10)
	for _, p := range []string{"x", "y", "scale", "rotation", "opacity", "volume", "other"} {
		setStatic(&it, p, 1.23456)
	}
	if *it.X != 1.235 || *it.Y != 1.235 || it.Scale != 1.235 || it.Rotation != 1.235 || it.Opacity != 1.235 ||
		it.Volume != 1.235 {
		t.Errorf("%+v", it)
	}
	if easingOrLinear("") != "linear" || easingOrLinear("hold") != "hold" {
		t.Error("easingOrLinear")
	}
}

func TestKShiftGrowingAtTheFrontShiftsEveryKeyframe(t *testing.T) {
	it := timeline.NewItem("itm_k000000001", timeline.ItemText, vidTr, 10, 50)
	it.Keyframes = timeline.Keyframes{"x": {kf(0, 1), kf(20, 2)}, "opacity": {kf(5, 0.5)}}
	kShift(&it, -10)
	if it.Keyframes["x"][0].T != 10 || it.Keyframes["x"][1].T != 30 || it.Keyframes["opacity"][0].T != 15 {
		t.Errorf("%+v", it.Keyframes)
	}
}

func TestKShiftWithNothingLeftEmptiesKeyframes(t *testing.T) {
	it := timeline.NewItem("itm_k000000001", timeline.ItemText, vidTr, 10, 50)
	it.Keyframes = timeline.Keyframes{"x": {kf(0, 1), kf(5, 3)}}
	kShift(&it, 10)
	if it.Keyframes != nil || it.X == nil || *it.X != 3 {
		t.Errorf("%+v %v", it.Keyframes, it.X)
	}
}

func TestKShiftKeepsTheEasingOfTheLastDroppedKeyframe(t *testing.T) {
	it := timeline.NewItem("itm_k000000001", timeline.ItemText, vidTr, 10, 50)
	it.Keyframes = timeline.Keyframes{"scale": {kf(0, 1, "ease_in"), kf(10, 2), kf(40, 3)}}
	kShift(&it, 4)
	got := it.Keyframes["scale"]
	if len(got) != 3 || got[0].T != 0 || got[0].Easing != "ease_in" || got[1].T != 6 {
		t.Errorf("%+v", got)
	}
}

func TestKCut(t *testing.T) {
	cut := func(l int, kfs ...timeline.Keyframe) *timeline.Item {
		it := timeline.NewItem("itm_k000000001", timeline.ItemText, vidTr, 0, 100)
		it.Keyframes = timeline.Keyframes{"opacity": kfs}
		kCut(&it, l)
		return &it
	}
	if it := cut(100, kf(0, 1), kf(50, 0)); len(it.Keyframes["opacity"]) != 2 {
		t.Errorf("nothing to cut: %+v", it.Keyframes)
	}
	if it := cut(50, kf(0, 1), kf(50, 0.5), kf(80, 0)); len(it.Keyframes["opacity"]) != 2 {
		t.Errorf("a keyframe at the cut is kept and none is added: %+v", it.Keyframes)
	}
	if it := cut(
		70,
		kf(0, 1),
		kf(50, 0.5),
		kf(90, 0),
	); len(it.Keyframes["opacity"]) != 3 ||
		it.Keyframes["opacity"][2].T != 70 {
		t.Errorf("the cut keyframe is added: %+v", it.Keyframes)
	}
	if it := cut(10, kf(20, 0.4), kf(30, 0.8)); it.Keyframes != nil || it.Opacity != 0.4 {
		t.Errorf("all dropped becomes static: %+v %v", it.Keyframes, it.Opacity)
	}
}

func TestKSplitCases(t *testing.T) {
	split := func(tc int, kfs ...timeline.Keyframe) (l, r timeline.Item) {
		it := timeline.NewItem("itm_k000000001", timeline.ItemText, vidTr, 0, 100)
		it.Keyframes = timeline.Keyframes{"scale": kfs}
		return kSplit(&it, tc)
	}
	// The cut keyframe is eased like the keyframe before it when the right part needs one.
	l, r := split(50, kf(0, 1, "ease_out"), kf(80, 2))
	if len(l.Keyframes["scale"]) != 2 || l.Keyframes["scale"][1].T != 50 ||
		r.Keyframes["scale"][0].T != 0 || r.Keyframes["scale"][0].Easing != "ease_out" || r.Keyframes["scale"][1].T != 30 {
		t.Errorf("%+v / %+v", l.Keyframes, r.Keyframes)
	}
	// A keyframe exactly on the cut goes to both parts, with its easing on the right.
	l, r = split(50, kf(0, 1), kf(50, 2, "hold"), kf(80, 3))
	if len(l.Keyframes["scale"]) != 2 || r.Keyframes["scale"][0].Easing != "hold" || len(r.Keyframes["scale"]) != 2 {
		t.Errorf("%+v / %+v", l.Keyframes, r.Keyframes)
	}
	// Only the keyframe on the cut: the left part keeps it, the right gets the static value.
	l, r = split(50, kf(50, 2))
	if len(l.Keyframes["scale"]) != 1 || r.Keyframes != nil || r.Scale != 2 {
		t.Errorf("%+v / %+v", l.Keyframes, r.Keyframes)
	}
	// No keyframe before and none on the cut: the left part is static.
	l, r = split(50, kf(60, 2), kf(80, 4))
	if l.Keyframes != nil || l.Scale != 2 || len(r.Keyframes["scale"]) != 2 {
		t.Errorf("%+v / %+v", l.Keyframes, r.Keyframes)
	}
}

func TestStyleValidation(t *testing.T) {
	c := &Context{Doc: baseDoc(t), effective: map[string]any{}}
	cases := []struct {
		style   string
		caption bool
		want    string
	}{
		{`{"font_family": ""}`, false, "non-empty string"},
		{`{"uppercase": "yes"}`, false, "must be a boolean"},
		{`{"align": 5}`, false, "must be one of"},
		{`{"size": "big"}`, false, "must be a number"},
		{`{"font_weight": 750}`, false, "multiple of 100"},
		{`{"max_width": 0}`, false, "greater than 0"},
		{`{"background_padding": -1}`, false, "at least 0"},
		{`{"shadow_dx": 1, "size": 7}`, false, "between 8 and 1000"},
		{`{"color": 5}`, false, "colour string"},
		{`{"color": "red"}`, false, "is not a colour"},
		{`{"position": "top"}`, false, "not a style key"},
		{`{"highlight": "word"}`, true, `must be "none"`},
		{`{"position": "left"}`, true, "must be one of"},
	}
	for _, tc := range cases {
		var m map[string]any
		if err := decodeInto(tc.style, &m); err != nil {
			t.Fatal(err)
		}
		e := c.validateStyle(m, tc.caption, false)
		if e == nil || e.Code != CodeValidation || !contains(e.Message+e.Hint, tc.want) || CheckError(e) != nil {
			t.Errorf("%s: %+v", tc.style, e)
		}
	}
	if e := c.validateStyle(map[string]any{"size": nil}, false, true); e != nil {
		t.Errorf("null is allowed on a merge: %v", e)
	}
	if e := c.validateStyle(map[string]any{"size": nil}, false, false); e == nil {
		t.Error("null is not allowed on create")
	}
}

func decodeInto(src string, v any) error {
	dec := json.NewDecoder(strings.NewReader(src))
	dec.UseNumber()
	return dec.Decode(v)
}

func contains(s, sub string) bool { return strings.Contains(s, sub) }
