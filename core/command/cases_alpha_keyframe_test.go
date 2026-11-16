package command

import (
	"fmt"

	"github.com/AlexGalhardo/vukem-video/core/timeline"
)

func init() {
	// The text item of the examples of C.42 to C.44: 90 frames from frame 30.
	item := func(mods ...func(*timeline.Item)) func() *B {
		return func() *B { return base().text(itemC, vidTr, 30, 90, mods...) }
	}
	withKeys := func(k timeline.Keyframes) func(*timeline.Item) {
		return func(it *timeline.Item) { it.Keyframes = k }
	}
	set := func(name, rest string, b func() *B) {
		gc("keyframe.set", name, b, op("keyframe.set", fmt.Sprintf(`{"item_id": %q, %s}`, itemC, rest)))
	}
	scaleAt := func(at, value string) string {
		return fmt.Sprintf(`"property": "scale", "at": %q, "value": %s`, at, value)
	}
	scaleKeys := withKeys(timeline.Keyframes{"scale": {kf(0, 1), kf(60, 2)}})

	set("ok_first_keyframe_at_item_start", scaleAt("30f", "1.2"), item())
	set(
		"ok_first_keyframe_later_adds_static_value_at_0",
		`"property": "scale", "at": "2s", "value": 1.2, "easing": "ease_out"`,
		item(),
	)
	set("ok_insert_between_existing", scaleAt("60f", "1.5"), item(scaleKeys))
	set("ok_replace_existing_resets_easing_when_omitted", scaleAt("2s", "1.4"),
		item(withKeys(timeline.Keyframes{"scale": {kf(0, 1), kf(30, 1.2, "ease_out")}})))
	set("ok_replace_with_easing", scaleAt("2s", "1.4")+`, "easing": "hold"`,
		item(withKeys(timeline.Keyframes{"scale": {kf(0, 1), kf(30, 1.2)}})))
	set("ok_at_last_frame_of_item_t_equals_duration", scaleAt("120f", "1.5"), item())
	set("ok_each_property", `"property": "rotation", "at": "60f", "value": 45.5`, item())
	set("ok_volume_on_audio_item", `"property": "volume", "at": "10f", "value": 0.5`,
		func() *B { return base().audio(itemC, 0, 60, 0) })
	set("ok_x_static_absent_uses_frame_centre", `"property": "x", "at": "2s", "value": 700`, item())
	set("ok_99th_to_100th_keyframe", scaleAt("150f", "1.5"), func() *B {
		return base().text(itemC, vidTr, 30, 200, withKeys(timeline.Keyframes{"scale": keyRun(99)}))
	})
	set("err_validation_bad_property", `"property": "zoom", "at": "2s", "value": 1`, item())
	set("err_validation_bad_easing", scaleAt("2s", "1.2")+`, "easing": "bounce"`, item())
	set("err_validation_scale_0", scaleAt("2s", "0"), item())
	set("err_validation_opacity_1_5", `"property": "opacity", "at": "2s", "value": 1.5`, item())
	set("err_invalid_time_no_unit", scaleAt("60", "1.2"), item())
	gc("keyframe.set", "err_not_found", item(), op("keyframe.set",
		`{"item_id": "itm_zzzzzzzzzz", "property": "scale", "at": "2s", "value": 1.2}`))
	set("err_type_mismatch_opacity_on_audio", `"property": "opacity", "at": "10f", "value": 0.5`,
		func() *B { return base().audio(itemC, 0, 60, 0) })
	set("err_type_mismatch_volume_on_image", `"property": "volume", "at": "10f", "value": 0.5`,
		func() *B {
			return base().clip(itemC, vidTr, 0, 60, 0, func(it *timeline.Item) { it.Asset, it.SourceIn = imgAsset, 0 })
		})
	set("err_type_mismatch_volume_on_text", `"property": "volume", "at": "2s", "value": 1`, item())
	set("err_invalid_time_at_before_item", scaleAt("10f", "1.2"), item())
	set("err_invalid_time_at_after_item_end", scaleAt("300f", "1.2"), item())
	set("err_invalid_time_at_one_past_start_plus_duration", scaleAt("121f", "1.2"), item())
	set("err_limit_101_keyframes", scaleAt("150f", "1.5"), func() *B {
		return base().text(itemC, vidTr, 30, 200, withKeys(timeline.Keyframes{"scale": keyRun(100)}))
	})

	rem := func(name, rest string, b func() *B) {
		gc("keyframe.remove", name, b, op("keyframe.remove", fmt.Sprintf(`{"item_id": %q, %s}`, itemC, rest)))
	}
	three := withKeys(timeline.Keyframes{"scale": {kf(0, 1), kf(30, 1.2, "ease_out"), kf(60, 1.5)}})
	only := func(prop string, v float64) func(*timeline.Item) {
		return withKeys(timeline.Keyframes{prop: {kf(0, v)}})
	}
	rem("ok_remove_middle_keyframe", `"property": "scale", "at": "2s"`, item(three))
	rem("ok_remove_first_keyframe_others_remain", `"property": "scale", "at": "30f"`, item(three))
	rem(
		"ok_remove_last_remaining_keyframe_writes_static_scale",
		`"property": "scale", "at": "30f"`,
		item(only("scale", 1.2)),
	)
	rem("ok_remove_last_remaining_keyframe_writes_static_x", `"property": "x", "at": "30f"`, item(only("x", 400)))
	rem("ok_remove_last_remaining_keyframe_value_equal_to_default_omits_field", `"property": "scale", "at": "30f"`,
		item(only("scale", 1)))
	rem("ok_other_properties_keep_keyframes", `"property": "scale", "at": "30f"`,
		item(withKeys(timeline.Keyframes{"scale": {kf(0, 1.2)}, "opacity": {kf(0, 0), kf(12, 1)}})))
	rem("err_validation_bad_property", `"property": "zoom", "at": "2s"`, item(three))
	rem("err_invalid_time_no_unit", `"property": "scale", "at": "60"`, item(three))
	rem(
		"err_invalid_time_at_outside_item_message_states_inclusive_range",
		`"property": "scale", "at": "300f"`,
		item(three),
	)
	gc("keyframe.remove", "err_not_found_item", item(three),
		op("keyframe.remove", `{"item_id": "itm_zzzzzzzzzz", "property": "scale", "at": "2s"}`))
	rem("err_not_found_no_keyframe_at_frame", `"property": "scale", "at": "120f"`,
		item(withKeys(timeline.Keyframes{"scale": {kf(0, 1), kf(30, 1.2)}})))
	rem("err_not_found_property_has_no_keyframes", `"property": "opacity", "at": "2s"`, item(three))
	rem("err_type_mismatch_volume_on_text", `"property": "volume", "at": "2s"`, item(three))

	clr := func(name, rest string, b func() *B) {
		gc("keyframe.clear", name, b, op("keyframe.clear", fmt.Sprintf(`{"item_id": %q%s}`, itemC, rest)))
	}
	prop := func(p string) string { return fmt.Sprintf(`, "property": %q`, p) }
	opacityKeys := withKeys(timeline.Keyframes{"opacity": {kf(0, 0), kf(12, 1)}})
	clr("ok_clear_one_property", prop("opacity"), item(opacityKeys))
	clr("ok_clear_all_properties", "", item(withKeys(timeline.Keyframes{
		"opacity": {kf(0, 0), kf(12, 1)}, "scale": {kf(0, 1.5), kf(30, 1)}, "x": {kf(0, 300), kf(30, 700)},
	})))
	clr("ok_first_keyframe_not_at_frame_0_its_value_still_used", prop("scale"),
		item(withKeys(timeline.Keyframes{"scale": {kf(10, 1.5), kf(40, 2)}})))
	clr("ok_value_equal_to_default_omits_static_field", prop("scale"),
		item(withKeys(timeline.Keyframes{"scale": {kf(0, 1), kf(10, 2)}})))
	clr("ok_x_written_explicitly", prop("x"), item(withKeys(timeline.Keyframes{"x": {kf(0, 540), kf(10, 100)}})))
	clr("ok_property_without_keyframes_noop", prop("scale"), item(opacityKeys))
	clr("ok_item_without_keyframes_noop", "", item())
	clr("err_validation_bad_property", prop("zoom"), item(opacityKeys))
	gc("keyframe.clear", "err_not_found", item(), op("keyframe.clear", `{"item_id": "itm_zzzzzzzzzz"}`))
	clr("err_type_mismatch_volume_on_text", prop("volume"), item(opacityKeys))
}

// keyRun is n linear keyframes of a growing value, one frame apart from frame 0.
func keyRun(n int) []timeline.Keyframe {
	ks := make([]timeline.Keyframe, n)
	for i := range ks {
		ks[i] = kf(i, 1+float64(i)/100)
	}
	return ks
}
