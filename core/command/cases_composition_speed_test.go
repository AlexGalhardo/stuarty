package command

import "github.com/AlexGalhardo/stuarty/core/timeline"

// Golden cases of clip.set_speed on a composition item (SDD-002 C.25, epic 53): a constant speed only.

func init() {
	cs := func(mods ...func(*timeline.Item)) func() *B {
		return func() *B {
			b := base()
			compItem(tlB, 30, 90, mods...)(b.d)

			return b
		}
	}
	set := func(name, args string, b func() *B) {
		gc("clip.set_speed", name, b, op("clip.set_speed", args))
	}

	set("ok_composition_double_speed_halves_the_duration", `{"item_id": "`+itemA+`", "speed": "2x"}`, cs())
	set("ok_composition_half_speed_doubles_the_duration", `{"item_id": "`+itemA+`", "speed": "0.5x"}`, cs())
	set("ok_composition_speed_rescales_fades_and_keyframes", `{"item_id": "`+itemA+`", "speed": "1.5x"}`,
		cs(func(i *timeline.Item) {
			i.FadeIn, i.FadeOut = 10, 8
			i.Keyframes = timeline.Keyframes{"scale": {kf(0, 1), kf(60, 1.2)}}
		}))
	set("ok_composition_speed_back_to_one", `{"item_id": "`+itemA+`", "speed": "1x"}`,
		cs(func(i *timeline.Item) { i.Speed, i.Duration = timeline.Rational{Num: 2, Den: 1}, 45 }))
	set("err_validation_composition_curve",
		`{"item_id": "`+itemA+`", "curve": [{"t": 0, "speed": "1x"}, {"t": 1, "speed": "2x"}]}`, cs())
	set("err_validation_composition_reverse", `{"item_id": "`+itemA+`", "reverse": true}`, cs())
}
