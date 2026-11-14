package command

import (
	"strings"
	"testing"

	"github.com/AlexGalhardo/vukem-video/core/timeline"
)

// Error messages of the item property commands quote identifiers and numbers, never
// the label a person gave an item.
func TestErrorsQuoteIdsNotNames(t *testing.T) {
	const label = "Secret label"
	named := func(it *timeline.Item) { it.Name = label }
	d := base().
		clip(itemA, vidTr, 0, 150, 0, named, func(it *timeline.Item) {
			it.TransitionOut = &timeline.Transition{Kind: "fade", Duration: 100}
			it.FadeOut = 60
			it.Keyframes = timeline.Keyframes{"scale": {kf(0, 1), kf(30, 2)}, "volume": {kf(0, 1)}}
		}).
		text(itemC, vidTr, 200, 60, named).
		audio(itemB, 0, 60, 0).d
	d.Items[itemB] = func() timeline.Item { it := d.Items[itemB]; it.Name = label; return it }()

	for _, o := range []Operation{
		op2("item.set_transform", `{"item_id": "`+itemB+`", "x": 1}`),
		op2("item.set_transform", `{"item_id": "`+itemA+`", "scale": 2}`),
		op2("item.set_transition", `{"item_id": "`+itemB+`", "edge": "in", "kind": "fade"}`),
		op2("item.set_transition", `{"item_id": "`+itemA+`", "edge": "in", "kind": "fade", "duration": "60f"}`),
		op2("item.set_audio", `{"item_id": "`+itemC+`", "volume": 1}`),
		op2("item.set_audio", `{"item_id": "`+itemB+`", "audio_muted": true}`),
		op2("item.set_audio", `{"item_id": "`+itemA+`", "volume": 2}`),
		op2("item.set_audio", `{"item_id": "`+itemA+`", "fade_in": "100f"}`),
	} {
		_, err := Default().Apply(d, o, Env{})
		if err == nil {
			t.Errorf("%s %s succeeded", o.Op, o.Args)
			continue
		}
		e := AsError(err)
		if strings.Contains(e.Message+e.Hint, label) {
			t.Errorf("%s quotes a name: %s %s", o.Op, e.Message, e.Hint)
		}
	}
}

func TestSetTransformRoundsToZeroScale(t *testing.T) {
	d := base().text(itemC, vidTr, 0, 60).d
	_, err := Default().Apply(d, op2("item.set_transform", `{"item_id": "`+itemC+`", "scale": 0.0001}`), Env{})
	if e := AsError(err); err == nil || e.Code != CodeValidation {
		t.Errorf("%v", err)
	}
	if _, err := Default().Apply(d, op2("item.set_transform", `{"item_id": "`+itemC+`", "x": 1e999}`), Env{}); err == nil {
		t.Error("an out-of-range number was accepted")
	}
}
