package command

import (
	"fmt"
	"strings"

	"github.com/AlexGalhardo/stuarty/core/timeline"
)

func init() {
	named := func(name string) func(*timeline.Item) { return func(it *timeline.Item) { it.Name = name } }
	one := func() *B { return base().clip(itemA, vidTr, 0, 150, 0) }

	rn := func(name, args string, b func() *B) { gc("item.rename", name, b, op("item.rename", args)) }
	rn("ok_set_name", fmt.Sprintf(`{"item_id": %q, "name": "Intro shot"}`, itemA), one)
	rn("ok_clear_name", fmt.Sprintf(`{"item_id": %q, "name": ""}`, itemA),
		func() *B { return base().clip(itemA, vidTr, 0, 150, 0, named("Old")) })
	rn("ok_name_120_chars", fmt.Sprintf(`{"item_id": %q, "name": %q}`, itemA, strings.Repeat("n", 120)), one)
	rn("ok_same_name_noop", fmt.Sprintf(`{"item_id": %q, "name": "Same"}`, itemA),
		func() *B { return base().clip(itemA, vidTr, 0, 150, 0, named("Same")) })
	rn("err_validation_121_chars", fmt.Sprintf(`{"item_id": %q, "name": %q}`, itemA, strings.Repeat("n", 121)), one)
	rn("err_not_found", `{"item_id": "itm_zzzzzzzzzz", "name": "x"}`, one)

	tf := func(name, args string, b func() *B) {
		gc("item.set_transform", name, b, op("item.set_transform", args))
	}
	id := func(rest string) string { return fmt.Sprintf(`{"item_id": %q, %s}`, itemC, rest) }
	text := func(mods ...func(*timeline.Item)) func() *B {
		return func() *B { return base().text(itemC, vidTr, 0, 60, mods...) }
	}
	tf("ok_set_x_y", id(`"x": 540, "y": 960`), text())
	tf("ok_set_scale", id(`"scale": 1.5`), text())
	tf("ok_set_rotation_negative", id(`"rotation": -12.5`), text())
	tf("ok_set_opacity_0_and_1", id(`"opacity": 0`), text(func(it *timeline.Item) { it.Opacity = 0.4 }))
	tf("ok_scale_1_removes_field", id(`"scale": 1`), text(func(it *timeline.Item) { it.Scale = 2 }))
	tf("ok_value_rounded_to_3_decimals", id(`"x": 10.12345, "scale": 1.23456`), text())
	tf("ok_caption_item", fmt.Sprintf(`{"item_id": %q, "y": 1500}`, itemA),
		func() *B { return base().caption(itemA, 0, 3) })
	tf("ok_shape_item", fmt.Sprintf(`{"item_id": %q, "rotation": 45, "opacity": 0.5}`, itemB),
		func() *B { return base().shape(itemB, vidTr, 0, 30) })
	tf("ok_same_values_noop", id(`"scale": 2, "opacity": 1`), text(func(it *timeline.Item) { it.Scale = 2 }))
	tf("err_validation_no_property", fmt.Sprintf(`{"item_id": %q}`, itemC), text())
	tf("err_validation_scale_0", id(`"scale": 0`), text())
	tf("err_validation_scale_20_001", id(`"scale": 20.001`), text())
	tf("err_validation_opacity_above_1", id(`"opacity": 1.5`), text())
	tf("err_not_found", `{"item_id": "itm_zzzzzzzzzz", "x": 1}`, text())
	tf("err_type_mismatch_audio_item", fmt.Sprintf(`{"item_id": %q, "x": 1}`, itemB),
		func() *B { return base().audio(itemB, 0, 60, 0) })
	tf("err_invalid_state_property_has_keyframes", id(`"scale": 1.2, "x": 5`), text(func(it *timeline.Item) {
		it.Keyframes = timeline.Keyframes{"scale": {kf(0, 1), kf(30, 2)}}
	}))

	st := func(name, args string, b func() *B) {
		gc("item.set_transition", name, b, op("item.set_transition", args))
	}
	tr := func(kind string, d int) *timeline.Transition { return &timeline.Transition{Kind: kind, Duration: d} }
	cl := func(mods ...func(*timeline.Item)) func() *B {
		return func() *B { return base().clip(itemA, vidTr, 0, 150, 0, mods...) }
	}
	arg := func(rest string) string { return fmt.Sprintf(`{"item_id": %q, %s}`, itemA, rest) }
	st("ok_set_in_default_duration", arg(`"edge": "in", "kind": "fade"`), cl())
	st("ok_set_out_explicit_duration", arg(`"edge": "out", "kind": "slide_left", "duration": "1s"`), cl())
	st("ok_replace_existing_kind", arg(`"edge": "in", "kind": "slide_up"`),
		cl(func(it *timeline.Item) { it.TransitionIn = tr("slide_left", 20) }))
	st("ok_remove_existing", arg(`"edge": "out", "kind": null`),
		cl(func(it *timeline.Item) { it.TransitionOut = tr("fade", 20) }))
	st("ok_remove_absent_noop", arg(`"edge": "out", "kind": null`), cl())
	st("ok_each_kind", arg(`"edge": "in", "kind": "slide_down", "duration": "10f"`), cl())
	st("ok_duration_1_frame", arg(`"edge": "in", "kind": "fade", "duration": "1f"`), cl())
	st("ok_sum_equals_item_duration", arg(`"edge": "out", "kind": "fade", "duration": "90f"`),
		cl(func(it *timeline.Item) { it.TransitionIn = tr("fade", 60) }))
	st("ok_text_caption_shape_items", fmt.Sprintf(`{"item_id": %q, "edge": "in", "kind": "fade"}`, itemB),
		func() *B { return base().shape(itemB, vidTr, 0, 60) })
	st("err_validation_unknown_kind", arg(`"edge": "in", "kind": "bounce"`), cl())
	st("ok_zoom", arg(`"edge": "in", "kind": "zoom", "duration": "12f"`), cl())
	st("ok_spin", arg(`"edge": "in", "kind": "spin", "duration": "12f"`), cl())
	st("ok_shrink", arg(`"edge": "in", "kind": "shrink", "duration": "12f"`), cl())
	st("ok_grow", arg(`"edge": "in", "kind": "grow", "duration": "12f"`), cl())
	st("ok_whip_with_direction", arg(`"edge": "out", "kind": "whip", "direction": "up", "duration": "9f"`), cl())
	st("ok_whip_default_direction_left_not_stored", arg(`"edge": "in", "kind": "whip", "direction": "left"`), cl())
	st("ok_dip_with_colour", arg(`"edge": "in", "kind": "dip", "color": "#FF8800", "duration": "15f"`), cl())
	st("ok_dip_black_not_stored", arg(`"edge": "out", "kind": "dip", "color": "#000000"`), cl())
	st("ok_wipe_with_direction", arg(`"edge": "in", "kind": "wipe", "direction": "right", "duration": "20f"`), cl())
	st("ok_replace_colour_dip_with_fade_drops_parameters", arg(`"edge": "in", "kind": "fade"`),
		cl(func(it *timeline.Item) {
			it.TransitionIn = &timeline.Transition{Kind: "dip", Duration: 20, Color: "#FF8800"}
		}))
	st("err_validation_color_with_fade", arg(`"edge": "in", "kind": "fade", "color": "#FF8800"`), cl())
	st("err_validation_color_not_6_digit_hex", arg(`"edge": "in", "kind": "dip", "color": "#FF8800CC"`), cl())
	st("err_validation_direction_with_fade", arg(`"edge": "in", "kind": "fade", "direction": "up"`), cl())
	st("err_validation_direction_unknown", arg(`"edge": "in", "kind": "wipe", "direction": "diagonal"`), cl())
	st("err_validation_duration_with_null_kind", arg(`"edge": "in", "kind": null, "duration": "1s"`), cl())
	st("err_invalid_time_zero_duration", arg(`"edge": "in", "kind": "fade", "duration": "0s"`), cl())
	st("err_not_found", `{"item_id": "itm_zzzzzzzzzz", "edge": "in", "kind": "fade"}`, cl())
	st("err_type_mismatch_audio_item", fmt.Sprintf(`{"item_id": %q, "edge": "in", "kind": "fade"}`, itemB),
		func() *B { return base().audio(itemB, 0, 60, 0) })
	st("err_invalid_state_sum_exceeds_duration", arg(`"edge": "in", "kind": "fade", "duration": "60f"`),
		cl(func(it *timeline.Item) { it.TransitionOut = tr("fade", 100) }))

	sa := func(name, args string, b func() *B) { gc("item.set_audio", name, b, op("item.set_audio", args)) }
	sa("ok_volume_only", arg(`"volume": 0.8`), cl())
	sa("ok_volume_0_and_4", arg(`"volume": 4`), cl(func(it *timeline.Item) { it.Volume = 0 }))
	sa("ok_fades", arg(`"fade_in": "1s", "fade_out": "0.5s"`), cl())
	sa("ok_fade_0_removes_field", arg(`"fade_in": "0s"`), cl(func(it *timeline.Item) { it.FadeIn = 30 }))
	sa("ok_fades_sum_equals_duration", arg(`"fade_in": "90f", "fade_out": "60f"`), cl())
	sa("ok_audio_muted_on_clip", arg(`"audio_muted": true`), cl())
	sa("ok_audio_item", fmt.Sprintf(`{"item_id": %q, "volume": 2, "fade_out": "1s"}`, itemB),
		func() *B { return base().audio(itemB, 0, 60, 0) })
	sa("ok_same_values_noop", arg(`"volume": 1, "audio_muted": false`), cl())
	sa("ok_fades_only_when_volume_has_keyframes", arg(`"fade_in": "1s"`),
		cl(func(it *timeline.Item) { it.Keyframes = timeline.Keyframes{"volume": {kf(0, 1), kf(30, 0.5)}} }))
	sa("ok_denoise", arg(`"denoise": 0.6`), cl())
	sa("ok_denoise_boundaries_0_and_1", arg(`"denoise": 1`), cl(func(it *timeline.Item) { it.Denoise = 0.4 }))
	sa("ok_denoise_0_removes_field", arg(`"denoise": 0`), cl(func(it *timeline.Item) { it.Denoise = 0.4 }))
	sa("ok_denoise_on_audio_item", fmt.Sprintf(`{"item_id": %q, "denoise": 0.8}`, itemB),
		func() *B { return base().audio(itemB, 0, 60, 0) })
	sa("ok_denoise_rounded_to_3_decimals", arg(`"denoise": 0.12345`), cl())
	sa("ok_denoise_when_volume_has_keyframes", arg(`"denoise": 0.5`),
		cl(func(it *timeline.Item) { it.Keyframes = timeline.Keyframes{"volume": {kf(0, 1), kf(30, 0.5)}} }))
	sa("ok_voice", arg(`"voice": "warm"`), cl())
	sa("ok_voice_changes_the_preset", arg(`"voice": "radio"`), cl(func(it *timeline.Item) { it.Voice = "deep" }))
	sa("ok_voice_none_removes_field", arg(`"voice": "none"`), cl(func(it *timeline.Item) { it.Voice = "deep" }))
	sa("ok_voice_on_audio_item", fmt.Sprintf(`{"item_id": %q, "voice": "clear"}`, itemB),
		func() *B { return base().audio(itemB, 0, 60, 0) })
	sa("ok_voice_with_volume_and_denoise", arg(`"volume": 0.9, "denoise": 0.3, "voice": "bright"`), cl())
	sa("ok_voice_same_value_noop", arg(`"voice": "warm"`), cl(func(it *timeline.Item) { it.Voice = "warm" }))
	sa("err_validation_unknown_voice", arg(`"voice": "robot"`), cl())
	sa("err_type_mismatch_voice_on_text_item", fmt.Sprintf(`{"item_id": %q, "voice": "warm"}`, itemC), text())
	sa("err_validation_denoise_above_1", arg(`"denoise": 1.2`), cl())
	sa("err_validation_denoise_negative", arg(`"denoise": -0.1`), cl())
	sa("err_validation_no_argument", fmt.Sprintf(`{"item_id": %q}`, itemA), cl())
	sa("err_validation_volume_4_001", arg(`"volume": 4.001`), cl())
	sa("err_invalid_time_no_unit", arg(`"fade_in": "2"`), cl())
	sa("err_not_found", `{"item_id": "itm_zzzzzzzzzz", "volume": 1}`, cl())
	sa("err_type_mismatch_image_clip", arg(`"volume": 1`),
		cl(func(it *timeline.Item) { it.Asset, it.SourceIn = imgAsset, 0 }))
	sa("err_type_mismatch_text_item", fmt.Sprintf(`{"item_id": %q, "volume": 1}`, itemC), text())
	sa("err_type_mismatch_audio_muted_on_audio_item", fmt.Sprintf(`{"item_id": %q, "audio_muted": true}`, itemB),
		func() *B { return base().audio(itemB, 0, 60, 0) })
	sa("err_invalid_state_volume_has_keyframes", arg(`"volume": 2`),
		cl(func(it *timeline.Item) { it.Keyframes = timeline.Keyframes{"volume": {kf(0, 1), kf(30, 0.5)}} }))
	sa("err_invalid_state_fades_exceed_duration", arg(`"fade_in": "100f"`),
		cl(func(it *timeline.Item) { it.FadeOut = 60 }))
}
