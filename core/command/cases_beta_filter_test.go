package command

import (
	"fmt"

	"github.com/AlexGalhardo/vukem-video/core/timeline"
)

func init() {
	fa := func(rest string) string { return fmt.Sprintf(`{"item_id": %q, %s}`, itemA, rest) }
	plain := func(mods ...func(*timeline.Item)) func() *B {
		return func() *B { return base().clip(itemA, vidTr, 0, 150, 0, mods...) }
	}
	audioItem := func() *B { return base().audio(itemB, 0, 60, 0) }
	image := func(it *timeline.Item) { it.Asset, it.SourceIn = imgAsset, 0 }

	fi := func(name, args string, b func() *B) { gc("clip.set_filter", name, b, op("clip.set_filter", args)) }
	fi("ok_filter_only", fa(`"filter": "sepia"`), plain())
	fi("ok_amount_only", fa(`"amount": 0.4`), plain(func(it *timeline.Item) { it.Filter = "invert" }))
	fi("ok_filter_and_amount", fa(`"filter": "grayscale", "amount": 0.6`), plain())
	fi("ok_each_filter", fa(`"filter": "cool"`), plain(func(it *timeline.Item) { it.Filter = "warm" }))
	fi(
		"ok_boundaries_amount_0_1",
		fa(`"filter": "warm", "amount": 0`),
		plain(func(it *timeline.Item) { it.FilterAmount = 1 }),
	)
	fi("ok_default_values_remove_fields", fa(`"filter": "none", "amount": 1`),
		plain(func(it *timeline.Item) { it.Filter, it.FilterAmount = "sepia", 0.3 }))
	fi("ok_none_keeps_amount", fa(`"filter": "none"`),
		plain(func(it *timeline.Item) { it.Filter, it.FilterAmount = "sepia", 0.3 }))
	fi("ok_rounded_to_3_decimals", fa(`"filter": "sepia", "amount": 0.12345`), plain())
	fi("ok_image_clip", fa(`"filter": "invert"`), plain(image))
	fi("ok_same_values_noop", fa(`"filter": "sepia", "amount": 0.5`),
		plain(func(it *timeline.Item) { it.Filter, it.FilterAmount = "sepia", 0.5 }))
	fi("ok_text_item", fmt.Sprintf(`{"item_id": %q, "filter": "sepia"}`, itemC),
		func() *B { return base().text(itemC, vidTr, 0, 60) })
	fi("ok_shape_item", fmt.Sprintf(`{"item_id": %q, "filter": "grayscale", "amount": 0.4}`, itemC),
		func() *B { return base().shape(itemC, vidTr, 0, 60) })
	fi("err_validation_no_argument", fmt.Sprintf(`{"item_id": %q}`, itemA), plain())
	fi("err_validation_unknown_filter", fa(`"filter": "sharpen"`), plain())
	fi("err_validation_amount_above_1", fa(`"amount": 1.5`), plain())
	fi("err_validation_amount_negative", fa(`"amount": -0.1`), plain())
	fi("err_not_found", `{"item_id": "itm_zzzzzzzzzz", "filter": "sepia"}`, plain())
	fi("err_type_mismatch_audio_item", fmt.Sprintf(`{"item_id": %q, "filter": "sepia"}`, itemB), audioItem)

	bl := func(name, args string, b func() *B) { gc("clip.set_blur", name, b, op("clip.set_blur", args)) }
	bl("ok_radius", fa(`"radius": 12.5`), plain())
	bl("ok_boundaries_0_100", fa(`"radius": 100`), plain(func(it *timeline.Item) { it.Blur = 5 }))
	bl("ok_zero_removes_field", fa(`"radius": 0`), plain(func(it *timeline.Item) { it.Blur = 5 }))
	bl("ok_rounded_to_3_decimals", fa(`"radius": 3.14159`), plain())
	bl("ok_image_clip", fa(`"radius": 8`), plain(image))
	bl("ok_same_value_noop", fa(`"radius": 5`), plain(func(it *timeline.Item) { it.Blur = 5 }))
	bl("ok_text_item", fmt.Sprintf(`{"item_id": %q, "radius": 6}`, itemC),
		func() *B { return base().text(itemC, vidTr, 0, 60) })
	bl("ok_caption_item", fmt.Sprintf(`{"item_id": %q, "radius": 6}`, itemC),
		func() *B { return base().caption(itemC, 0, 3) })
	bl("err_validation_no_argument", fmt.Sprintf(`{"item_id": %q}`, itemA), plain())
	bl("err_validation_radius_above_100", fa(`"radius": 120`), plain())
	bl("err_validation_radius_negative", fa(`"radius": -1`), plain())
	bl("err_not_found", `{"item_id": "itm_zzzzzzzzzz", "radius": 4}`, plain())
	bl("err_type_mismatch_audio_item", fmt.Sprintf(`{"item_id": %q, "radius": 4}`, itemB), audioItem)

	ck := func(name, args string, b func() *B) {
		gc("clip.set_chroma_key", name, b, op("clip.set_chroma_key", args))
	}
	keyed := func(it *timeline.Item) {
		it.ChromaKey = &timeline.ChromaKey{Color: "#00FF00", Similarity: 0.5, Smoothness: 0.2, Spill: 0.4}
	}
	ck("ok_defaults", fa(`"color": "#00FF00"`), plain())
	ck("ok_all_values", fa(`"color": "#1FDB3A", "similarity": 0.35, "smoothness": 0.05, "spill": 0.25`), plain())
	ck("ok_keeps_current_values", fa(`"color": "#0000FF"`), plain(keyed))
	ck("ok_one_value", fa(`"color": "#00FF00", "spill": 0.9`), plain(keyed))
	ck("ok_null_removes_key", fa(`"color": null`), plain(keyed))
	ck("ok_null_without_key_noop", fa(`"color": null`), plain())
	ck("ok_boundaries_0_1", fa(`"color": "#00FF00", "similarity": 1, "smoothness": 0, "spill": 1`), plain())
	ck("ok_rounded_to_3_decimals", fa(`"color": "#00FF00", "similarity": 0.12345`), plain())
	ck("ok_image_clip", fa(`"color": "#00FF00"`), plain(image))
	ck("err_validation_no_color", fmt.Sprintf(`{"item_id": %q}`, itemA), plain())
	ck("err_validation_bad_color", fa(`"color": "green"`), plain())
	ck("err_validation_similarity_above_1", fa(`"color": "#00FF00", "similarity": 1.5`), plain())
	ck("err_validation_spill_negative", fa(`"color": "#00FF00", "spill": -0.1`), plain())
	ck("err_validation_value_with_null", fa(`"color": null, "spill": 0.1`), plain(keyed))
	ck("err_not_found", `{"item_id": "itm_zzzzzzzzzz", "color": "#00FF00"}`, plain())
	ck("err_type_mismatch_audio_item", fmt.Sprintf(`{"item_id": %q, "color": "#00FF00"}`, itemB), audioItem)

	en := func(name, args string, b func() *B) {
		gc("item.set_enabled", name, b, op("item.set_enabled", args))
	}
	off := func(it *timeline.Item) { it.Disabled = true }
	en("ok_disable_clip", fa(`"enabled": false`), plain())
	en("ok_enable_clip", fa(`"enabled": true`), plain(off))
	en("ok_disable_audio_item", fmt.Sprintf(`{"item_id": %q, "enabled": false}`, itemB), audioItem)
	en("ok_disable_text_item", fmt.Sprintf(`{"item_id": %q, "enabled": false}`, itemC),
		func() *B { return base().text(itemC, vidTr, 0, 60) })
	en("ok_enable_without_field_noop", fa(`"enabled": true`), plain())
	en("ok_disable_twice_noop", fa(`"enabled": false`), plain(off))
	en("err_validation_no_enabled", fmt.Sprintf(`{"item_id": %q}`, itemA), plain())
	en("err_not_found", `{"item_id": "itm_zzzzzzzzzz", "enabled": false}`, plain())

	fl := func(name, args string, b func() *B) {
		gc("clip.set_flip", name, b, op("clip.set_flip", args))
	}
	flipped := func(it *timeline.Item) { it.FlipX = true }
	fl("ok_horizontal", fa(`"horizontal": true`), plain())
	fl("ok_vertical", fa(`"vertical": true`), plain())
	fl("ok_both", fa(`"horizontal": true, "vertical": true`), plain())
	fl("ok_keeps_the_side_not_given", fa(`"vertical": true`), plain(flipped))
	fl("ok_false_clears_the_field", fa(`"horizontal": false`), plain(flipped))
	fl("ok_false_without_field_noop", fa(`"horizontal": false`), plain())
	fl("ok_same_value_noop", fa(`"horizontal": true`), plain(flipped))
	fl("ok_image_clip", fa(`"horizontal": true`), plain(image))
	fl("err_validation_no_argument", fmt.Sprintf(`{"item_id": %q}`, itemA), plain())
	fl("err_not_found", `{"item_id": "itm_zzzzzzzzzz", "horizontal": true}`, plain())
	fl("err_type_mismatch_audio_item", fmt.Sprintf(`{"item_id": %q, "horizontal": true}`, itemB), audioItem)

	bm := func(name, args string, b func() *B) {
		gc("clip.set_blend", name, b, op("clip.set_blend", args))
	}
	blended := func(it *timeline.Item) { it.Blend = "multiply" }
	bm("ok_multiply", fa(`"mode": "multiply"`), plain())
	bm("ok_screen", fa(`"mode": "screen"`), plain())
	bm("ok_overlay", fa(`"mode": "overlay"`), plain())
	bm("ok_add", fa(`"mode": "add"`), plain())
	bm("ok_replaces_mode", fa(`"mode": "add"`), plain(blended))
	bm("ok_normal_removes_field", fa(`"mode": "normal"`), plain(blended))
	bm("ok_normal_without_field_noop", fa(`"mode": "normal"`), plain())
	bm("ok_same_mode_noop", fa(`"mode": "multiply"`), plain(blended))
	bm("ok_image_clip", fa(`"mode": "screen"`), plain(image))
	bm("err_validation_no_mode", fmt.Sprintf(`{"item_id": %q}`, itemA), plain())
	bm("err_validation_unknown_mode", fa(`"mode": "dodge"`), plain())
	bm("err_not_found", `{"item_id": "itm_zzzzzzzzzz", "mode": "screen"}`, plain())
	bm("err_type_mismatch_audio_item", fmt.Sprintf(`{"item_id": %q, "mode": "screen"}`, itemB), audioItem)

	mk := func(name, args string, b func() *B) {
		gc("clip.set_mask", name, b, op("clip.set_mask", args))
	}
	mask := func(body string) string { return fa(`"mask": ` + body) }
	masked := func(it *timeline.Item) {
		it.Mask = &timeline.Mask{Shape: "rect", X: 0, Y: 0, Width: 0.5, Height: 0.5, Feather: 4}
	}
	mk("ok_ellipse", mask(`{"shape": "ellipse", "x": 0.1, "y": 0.1, "width": 0.8, "height": 0.8}`), plain())
	mk(
		"ok_rect_feather_invert",
		mask(`{"shape": "rect", "x": 0, "y": 0.25, "width": 1, "height": 0.5, "feather": 30, "invert": true}`),
		plain(),
	)
	mk("ok_replaces_mask", mask(`{"shape": "ellipse", "x": 0.2, "y": 0.2, "width": 0.6, "height": 0.6}`), plain(masked))
	mk("ok_null_removes_mask", mask(`null`), plain(masked))
	mk("ok_null_without_mask_noop", mask(`null`), plain())
	mk(
		"ok_same_mask_noop",
		mask(`{"shape": "rect", "x": 0, "y": 0, "width": 0.5, "height": 0.5, "feather": 4}`),
		plain(masked),
	)
	mk("ok_boundaries", mask(`{"shape": "rect", "x": 0, "y": 0, "width": 1, "height": 1, "feather": 200}`), plain())
	mk(
		"ok_rounded_to_3_decimals",
		mask(`{"shape": "rect", "x": 0.12345, "y": 0, "width": 0.5, "height": 0.5}`),
		plain(),
	)
	mk("ok_image_clip", mask(`{"shape": "ellipse", "x": 0, "y": 0, "width": 1, "height": 1}`), plain(image))
	mk("err_validation_no_mask", fmt.Sprintf(`{"item_id": %q}`, itemA), plain())
	mk("err_validation_unknown_shape", mask(`{"shape": "star", "x": 0, "y": 0, "width": 1, "height": 1}`), plain())
	mk("err_validation_outside_item", mask(`{"shape": "rect", "x": 0.6, "y": 0, "width": 0.6, "height": 1}`), plain())
	mk("err_validation_width_zero", mask(`{"shape": "rect", "x": 0, "y": 0, "width": 0, "height": 1}`), plain())
	mk(
		"err_validation_feather_above_200",
		mask(`{"shape": "rect", "x": 0, "y": 0, "width": 1, "height": 1, "feather": 201}`),
		plain(),
	)
	mk("err_validation_missing_height", mask(`{"shape": "rect", "x": 0, "y": 0, "width": 1}`), plain())
	mk("err_not_found", `{"item_id": "itm_zzzzzzzzzz", "mask": null}`, plain())
	mk("err_type_mismatch_audio_item", fmt.Sprintf(`{"item_id": %q, "mask": null}`, itemB), audioItem)
}
