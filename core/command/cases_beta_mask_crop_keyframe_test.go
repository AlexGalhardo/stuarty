package command

// Golden cases of the mask and crop properties of keyframe.set and their guards (VV-120).

import (
	"fmt"

	"github.com/AlexGalhardo/stuarty/core/timeline"
)

func init() {
	clip := func(mods ...func(*timeline.Item)) func() *B {
		return func() *B { return base().clip(itemA, vidTr, 0, 90, 0, mods...) }
	}
	masked := func(it *timeline.Item) {
		it.Mask = &timeline.Mask{Shape: "rect", X: 0, Y: 0, Width: 0.5, Height: 0.5, Feather: 4}
	}
	maskKeys := func(it *timeline.Item) {
		masked(it)
		it.Keyframes = timeline.Keyframes{"mask_x": {kf(0, 0), kf(60, 0.5)}}
	}
	cropKeys := func(it *timeline.Item) {
		it.Keyframes = timeline.Keyframes{"crop_left": {kf(0, 0), kf(60, 0.3)}}
	}
	set := func(name, rest string, b func() *B) {
		gc("keyframe.set", name, b, op("keyframe.set", fmt.Sprintf(`{"item_id": %q, %s}`, itemA, rest)))
	}

	set("ok_mask_x_first_keyframe_keeps_static", `"property": "mask_x", "at": "30f", "value": 0.4`, clip(masked))
	set("ok_mask_width_at_its_limit", `"property": "mask_width", "at": "60f", "value": 1`, clip(maskKeys))
	set("ok_mask_feather", `"property": "mask_feather", "at": "30f", "value": 40`, clip(masked))
	set("err_invalid_state_mask_missing", `"property": "mask_x", "at": "30f", "value": 0.4`, clip())
	set("err_validation_mask_width_zero", `"property": "mask_width", "at": "30f", "value": 0`, clip(masked))
	set(
		"err_validation_mask_feather_above_range",
		`"property": "mask_feather", "at": "30f", "value": 201`,
		clip(masked),
	)
	set("ok_crop_left", `"property": "crop_left", "at": "30f", "value": 0.2`, clip())
	set("ok_crop_bottom_on_cropped_clip", `"property": "crop_bottom", "at": "60f", "value": 0.5`, clip(cropKeys))
	set("err_validation_crop_above_range", `"property": "crop_top", "at": "30f", "value": 1.2`, clip())
	set("err_type_mismatch_crop_on_text", `"property": "crop_left", "at": "30f", "value": 0.2`,
		func() *B { return base().text(itemA, vidTr, 0, 90) })

	gc("keyframe.remove", "ok_mask_last_keyframe_writes_static", clip(func(it *timeline.Item) {
		masked(it)
		it.Keyframes = timeline.Keyframes{"mask_x": {kf(30, 0.4)}}
	}), op("keyframe.remove", fmt.Sprintf(`{"item_id": %q, "property": "mask_x", "at": "30f"}`, itemA)))
	gc("keyframe.clear", "ok_crop_returns_to_static", clip(cropKeys),
		op("keyframe.clear", fmt.Sprintf(`{"item_id": %q, "property": "crop_left"}`, itemA)))
	gc("clip.set_mask", "ok_replace_clears_mask_keyframes", clip(maskKeys), op("clip.set_mask", fmt.Sprintf(
		`{"item_id": %q, "mask": {"shape": "ellipse", "x": 0, "y": 0, "width": 1, "height": 1}}`, itemA)))
	gc("clip.set_mask", "ok_null_clears_mask_keyframes", clip(maskKeys),
		op("clip.set_mask", fmt.Sprintf(`{"item_id": %q, "mask": null}`, itemA)))
	gc("clip.set_framing", "err_invalid_state_crop_side_has_keyframes", clip(cropKeys),
		op("clip.set_framing", fmt.Sprintf(`{"item_id": %q, "crop": {"left": 0.1}}`, itemA)))
	gc("clip.set_framing", "ok_other_crop_side_is_free", clip(cropKeys),
		op("clip.set_framing", fmt.Sprintf(`{"item_id": %q, "crop": {"right": 0.1}}`, itemA)))
}
