package command

// Golden cases of the Bezier easing: keyframe.set_easing and the bezier argument of keyframe.set (epic 31).

import (
	"fmt"

	"github.com/AlexGalhardo/stuarty/core/timeline"
)

// kfb is a keyframe whose segment follows a cubic Bezier curve.
func kfb(t int, v float64, curve ...float64) timeline.Keyframe {
	k := timeline.NewKeyframe(t, v)
	k.Easing, k.Bezier = timeline.EasingBezier, curve

	return k
}

func init() {
	clip := func(mods ...func(*timeline.Item)) func() *B {
		return func() *B { return base().clip(itemA, vidTr, 0, 90, 0, mods...) }
	}
	scaleKeys := func(it *timeline.Item) {
		it.Keyframes = timeline.Keyframes{"scale": {kf(0, 1), kf(60, 2)}}
	}
	curved := func(it *timeline.Item) {
		it.Keyframes = timeline.Keyframes{"scale": {kfb(0, 1, 0.1, 0.9, 0.2, 1), kf(60, 2)}}
	}
	cropKeys := func(it *timeline.Item) {
		it.Keyframes = timeline.Keyframes{"crop_left": {kf(0, 0), kf(60, 0.3)}}
	}
	maskWidthKeys := func(it *timeline.Item) {
		it.Mask = &timeline.Mask{Shape: "ellipse", X: 0.25, Y: 0.25, Width: 0.1, Height: 0.5, Feather: 4}
		it.Keyframes = timeline.Keyframes{"mask_width": {kf(0, 0.1), kf(60, 0.5)}}
	}
	easing := func(name, rest string, b func() *B) {
		gc("keyframe.set_easing", name, b,
			op("keyframe.set_easing", fmt.Sprintf(`{"item_id": %q, "property": "scale", "at": "0f", %s}`, itemA, rest)))
	}
	on := func(name, prop, at, rest string, b func() *B) {
		gc(
			"keyframe.set_easing",
			name,
			b,
			op(
				"keyframe.set_easing",
				fmt.Sprintf(`{"item_id": %q, "property": %q, "at": %q, %s}`, itemA, prop, at, rest),
			),
		)
	}

	easing("ok_custom_bezier", `"bezier": [0.1, 0.9, 0.2, 1]`, clip(scaleKeys))
	easing("ok_preset_overshoot", `"preset": "overshoot"`, clip(scaleKeys))
	on("ok_preset_back_on_crop", "crop_left", "0f", `"preset": "back"`, clip(cropKeys))
	on("ok_bezier_on_mask_width", "mask_width", "0f", `"bezier": [0.34, 1.56, 0.64, 1]`, clip(maskWidthKeys))
	easing("ok_named_easing_clears_the_bezier", `"easing": "ease_in"`, clip(curved))
	easing("ok_linear_is_stored_absent", `"easing": "linear"`, clip(curved))
	on("ok_last_keyframe_of_the_property", "scale", "60f", `"preset": "in_out"`, clip(scaleKeys))
	easing("ok_bezier_is_rounded_to_3_decimals", `"bezier": [0.12345, 0.98765, 0.2, 1]`, clip(scaleKeys))

	gc("keyframe.set_easing", "err_validation_none_of_the_three", clip(scaleKeys),
		op("keyframe.set_easing", fmt.Sprintf(`{"item_id": %q, "property": "scale", "at": "0f"}`, itemA)))
	easing("err_validation_easing_and_preset", `"easing": "hold", "preset": "back"`, clip(scaleKeys))
	easing("err_validation_bezier_x_above_1", `"bezier": [1.5, 0, 0.5, 1]`, clip(scaleKeys))
	easing("err_validation_bezier_y_below_range", `"bezier": [0.5, -2.5, 0.5, 1]`, clip(scaleKeys))
	easing("err_validation_bezier_wrong_length", `"bezier": [0.5, 0, 0.5]`, clip(scaleKeys))
	easing("err_validation_unknown_preset", `"preset": "bounce"`, clip(scaleKeys))
	easing("err_validation_easing_bezier_is_not_a_name", `"easing": "bezier"`, clip(scaleKeys))
	gc("keyframe.set_easing", "err_not_found", clip(scaleKeys), op("keyframe.set_easing",
		`{"item_id": "itm_zzzzzzzzzz", "property": "scale", "at": "0f", "preset": "out"}`))
	on("err_not_found_no_keyframe_at", "scale", "30f", `"preset": "out"`, clip(scaleKeys))
	on("err_invalid_time_outside_the_item", "scale", "200f", `"preset": "out"`, clip(scaleKeys))
	gc(
		"keyframe.set_easing",
		"err_type_mismatch_crop_on_text",
		func() *B { return base().text(itemA, vidTr, 0, 90) },
		op(
			"keyframe.set_easing",
			fmt.Sprintf(`{"item_id": %q, "property": "crop_left", "at": "0f", "preset": "out"}`, itemA),
		),
	)

	set := func(name, rest string, b func() *B) {
		gc("keyframe.set", name, b, op("keyframe.set", fmt.Sprintf(`{"item_id": %q, %s}`, itemA, rest)))
	}
	scale := `"property": "scale", "at": "30f", "value": 1.5, `

	set("ok_bezier_easing", scale+`"easing": "bezier", "bezier": [0.34, 1.56, 0.64, 1]`, clip(scaleKeys))
	set("err_validation_bezier_without_the_easing", scale+`"bezier": [0.34, 1.56, 0.64, 1]`, clip())
	set("err_validation_bezier_easing_without_numbers", scale+`"easing": "bezier"`, clip())
	set("err_validation_bezier_x_above_1", scale+`"easing": "bezier", "bezier": [0.5, 0, 1.2, 1]`, clip())
}
