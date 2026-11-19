package command

// Golden cases of animation.apply_preset and animation.list_presets (epic 31, RF-082).

import (
	"fmt"

	"github.com/AlexGalhardo/stuarty/core/timeline"
)

func init() {
	clip := func(mods ...func(*timeline.Item)) func() *B {
		return func() *B { return base().clip(itemA, vidTr, 0, 90, 0, mods...) }
	}
	withKeys := func(k timeline.Keyframes) func(*timeline.Item) {
		return func(it *timeline.Item) { it.Keyframes = k }
	}
	apply := func(name, rest string, b func() *B) {
		gc("animation.apply_preset", name, b,
			op("animation.apply_preset", fmt.Sprintf(`{"item_id": %q, %s}`, itemA, rest)))
	}

	apply("ok_fade_in", `"preset": "fade"`, clip())
	apply("ok_fade_out", `"preset": "fade", "edge": "out", "duration": "1s"`, clip())
	apply("ok_slide_in_from_left", `"preset": "slide"`, clip())
	apply("ok_slide_out_to_the_bottom", `"preset": "slide", "edge": "out", "direction": "down"`, clip())
	apply("ok_slide_with_distance", `"preset": "slide", "direction": "right", "amount": 120`, clip())
	apply("ok_scale_in", `"preset": "scale"`, clip())
	apply("ok_pop_in", `"preset": "pop"`, clip())
	apply("ok_pop_out", `"preset": "pop", "edge": "out", "duration": "10f"`, clip())
	apply("ok_push_in_the_whole_item", `"preset": "push_in"`, clip())
	apply("ok_pull_out_on_text", `"preset": "pull_out", "amount": 1.5`,
		func() *B { return base().text(itemA, vidTr, 0, 90) })
	apply("ok_in_then_out_gives_both_ends", `"preset": "fade", "edge": "out"`,
		clip(withKeys(timeline.Keyframes{"opacity": {kf(0, 0), kf(15, 1)}})))
	apply("ok_replaces_the_keyframes_inside_the_window", `"preset": "scale", "duration": "1s"`,
		clip(withKeys(timeline.Keyframes{"scale": {kf(0, 1), kf(10, 1.1), kf(20, 1.3), kf(80, 1.2)}})))
	apply("ok_keeps_the_keyframes_outside_the_window", `"preset": "fade", "edge": "out", "duration": "1s"`,
		clip(withKeys(timeline.Keyframes{"opacity": {kf(0, 1), kf(30, 0.5)}})))
	apply("ok_shape_item", `"preset": "pop"`, func() *B { return base().shape(itemA, vidTr, 0, 90) })

	apply("err_validation_unknown_preset", `"preset": "wobble"`, clip())
	apply("err_validation_direction_on_fade", `"preset": "fade", "direction": "left"`, clip())
	apply("err_validation_amount_on_pop", `"preset": "pop", "amount": 2`, clip())
	apply("err_validation_scale_above_20", `"preset": "scale", "amount": 6`,
		clip(func(it *timeline.Item) { it.Scale = 5 }))
	apply("err_invalid_time_duration_longer_than_the_item", `"preset": "fade", "duration": "5s"`, clip())
	apply("err_invalid_time_duration_zero", `"preset": "fade", "duration": "0f"`, clip())
	gc("animation.apply_preset", "err_not_found", clip(),
		op("animation.apply_preset", `{"item_id": "itm_zzzzzzzzzz", "preset": "fade"}`))
	gc("animation.apply_preset", "err_type_mismatch_audio_item",
		func() *B { return base().audio(itemA, 0, 60, 0) },
		op("animation.apply_preset", fmt.Sprintf(`{"item_id": %q, "preset": "fade"}`, itemA)))
	apply("err_limit_101_keyframes", `"preset": "scale", "edge": "out", "duration": "15f"`, func() *B {
		return base().clip(itemA, vidTr, 0, 200, 0, withKeys(timeline.Keyframes{"scale": keyRun(100)}))
	})

	gc("animation.list_presets", "ok_lists_the_presets", clip(), op("animation.list_presets", `{}`))
	gc("animation.list_presets", "err_validation_takes_no_arguments", clip(),
		op("animation.list_presets", `{"preset": "fade"}`))
}
