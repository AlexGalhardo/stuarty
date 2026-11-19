package command

import (
	"fmt"

	"github.com/AlexGalhardo/stuarty/core/timeline"
)

func init() {
	au := func(name, args string, b func() *B) { gc("audio.add", name, b, op("audio.add", args)) }
	aa := func(asset, rest string) string {
		return fmt.Sprintf(`{"track_id": %q, "asset_id": %q, "id": %q, %s}`, audTr, asset, itemC, rest)
	}
	au("ok_audio_asset_default_duration", aa(audAsset, `"start": "1s"`), base)
	au(
		"ok_audio_asset_explicit_duration_and_source_in",
		aa(audAsset, `"start": "1s", "source_in": "1s", "duration": "10s"`),
		base,
	)
	au("ok_start_omitted_goes_to_the_end_of_the_track",
		fmt.Sprintf(`{"track_id": %q, "asset_id": %q, "id": %q, "duration": "1s"}`, audTr, audAsset, itemC),
		func() *B { return base().audio(itemB, 5, 40, 0) })
	au("ok_video_with_audio_asset", aa(vidAsset, `"start": "0s"`), base)
	au("ok_ends_exactly_at_asset_end", aa(audAsset, `"start": "0s", "source_in": "30f", "duration": "870f"`), base)
	au("ok_supplied_id", aa(audAsset, `"start": "5s"`), base)
	au("ok_placeholder_asset_with_duration", aa("ast_placeholder", `"start": "0s", "duration": "5s"`),
		func() *B { return base().template() })
	au("err_validation_bad_asset_id", aa("ast_", `"start": "0s"`), base)
	au("err_invalid_time_ends_after_108000", aa(audAsset, `"start": "107600f", "duration": "500f"`), base)
	au(
		"err_not_found_track",
		fmt.Sprintf(`{"track_id": "trk_zzzzzzzzzz", "asset_id": %q, "start": "0s", "id": %q}`, audAsset, itemC),
		base,
	)
	au("err_not_found_asset", aa("ast_zzzzzzzzzz", `"start": "0s"`), base)
	au("err_type_mismatch_visual_track",
		fmt.Sprintf(`{"track_id": %q, "asset_id": %q, "start": "0s", "id": %q}`, vidTr, audAsset, itemC), base)
	au("err_type_mismatch_image_asset", aa(imgAsset, `"start": "0s"`), base)
	au("err_type_mismatch_video_without_audio", aa(vidAsset, `"start": "0s"`), func() *B {
		return base().with(func(d *timeline.Document) {
			a := d.Assets[vidAsset]
			a.HasAudio, a.SampleCount = false, 0
			d.Assets[vidAsset] = a
		})
	})
	au("err_id_exists", aa(audAsset, `"start": "0s"`), func() *B { return base().text(itemC, vidTr, 0, 10) })
	au("err_limit_500_items", aa(audAsset, `"start": "0s"`), func() *B { return base().manyItems(vidTr, 500) })
	au("err_out_of_source", aa(audAsset, `"start": "0s", "source_in": "30f", "duration": "900f"`), base)
	au("err_overlap", aa(audAsset, `"start": "30f", "duration": "900f"`),
		func() *B { return base().audio(itemB, 0, 100, 0) })

	fr := func(name, args string, b func() *B) { gc("clip.set_framing", name, b, op("clip.set_framing", args)) }
	fa := func(rest string) string { return fmt.Sprintf(`{"item_id": %q, %s}`, itemA, rest) }
	plain := func(mods ...func(*timeline.Item)) func() *B {
		return func() *B { return base().clip(itemA, vidTr, 0, 150, 0, mods...) }
	}
	cropped := func(it *timeline.Item) { it.Crop = &timeline.Crop{Left: 0.2, Top: 0.1} }
	fr("ok_fit_only", fa(`"fit": "contain"`), plain())
	fr("ok_crop_only", fa(`"crop": {"left": 0.1, "right": 0.1}`), plain())
	fr("ok_crop_merges_with_existing_sides", fa(`"crop": {"right": 0.3}`), plain(cropped))
	fr("ok_crop_zero_removes_field", fa(`"crop": {"left": 0, "top": 0}`), plain(cropped))
	fr("ok_fit_cover_removes_field", fa(`"fit": "cover"`), plain(func(it *timeline.Item) { it.Fit = "none" }))
	fr("ok_crop_sides_sum_just_below_1", fa(`"crop": {"left": 0.5, "right": 0.499}`), plain())
	fr("ok_image_clip", fa(`"fit": "contain"`), plain(func(it *timeline.Item) { it.Asset, it.SourceIn = imgAsset, 0 }))
	fr("ok_same_values_noop", fa(`"fit": "contain", "crop": {"left": 0.2}`),
		plain(cropped, func(it *timeline.Item) { it.Fit = "contain" }))
	fr("err_validation_no_argument", fmt.Sprintf(`{"item_id": %q}`, itemA), plain())
	fr("err_validation_side_above_1", fa(`"crop": {"left": 1.2}`), plain())
	fr("err_validation_horizontal_sum_1", fa(`"crop": {"left": 0.6, "right": 0.4}`), plain())
	fr("err_validation_merged_vertical_sum_1", fa(`"crop": {"bottom": 0.9}`), plain(cropped))
	fr("err_not_found", `{"item_id": "itm_zzzzzzzzzz", "fit": "none"}`, plain())
	fr("err_type_mismatch_text_item", fmt.Sprintf(`{"item_id": %q, "fit": "none"}`, itemC),
		func() *B { return base().text(itemC, vidTr, 0, 60) })

	co := func(name, args string, b func() *B) { gc("clip.set_color", name, b, op("clip.set_color", args)) }
	co("ok_brightness_only", fa(`"brightness": 0.1`), plain())
	co("ok_all_three", fa(`"brightness": -0.2, "contrast": 1.5, "saturation": 0.5`), plain())
	co("ok_boundaries_minus1_1_0_2", fa(`"brightness": -1, "contrast": 2, "saturation": 0`), plain())
	co("ok_default_values_remove_fields", fa(`"brightness": 0, "contrast": 1, "saturation": 1`),
		plain(func(it *timeline.Item) { it.Brightness, it.Contrast, it.Saturation = 0.3, 1.4, 0.6 }))
	co("ok_rounded_to_3_decimals", fa(`"brightness": 0.12345, "saturation": 1.23456`), plain())
	co("ok_same_values_noop", fa(`"brightness": 0.1`), plain(func(it *timeline.Item) { it.Brightness = 0.1 }))
	co("ok_temperature_tint_hue", fa(`"temperature": 0.3, "tint": -0.2, "hue": 45`), plain())
	co("ok_white_balance_boundaries", fa(`"temperature": -1, "tint": 1, "hue": -180`), plain())
	co("ok_hue_180", fa(`"hue": 180`), plain())
	co("ok_new_defaults_remove_fields", fa(`"temperature": 0, "tint": 0, "hue": 0`),
		plain(func(it *timeline.Item) { it.Temperature, it.Tint, it.Hue = 0.4, 0.1, -30 }))
	co("ok_new_rounded_to_3_decimals", fa(`"temperature": 0.12345, "hue": 12.34567`), plain())
	co("err_validation_temperature_above_1", fa(`"temperature": 1.5`), plain())
	co("err_validation_tint_below_minus_1", fa(`"tint": -1.2`), plain())
	co("err_validation_hue_above_180", fa(`"hue": 200`), plain())
	co("err_validation_no_argument", fmt.Sprintf(`{"item_id": %q}`, itemA), plain())
	co("err_validation_brightness_above_1", fa(`"brightness": 1.5`), plain())
	co("err_validation_saturation_negative", fa(`"saturation": -0.1`), plain())
	co("err_not_found", `{"item_id": "itm_zzzzzzzzzz", "contrast": 1.2}`, plain())
	co("err_type_mismatch_audio_item", fmt.Sprintf(`{"item_id": %q, "contrast": 1.2}`, itemB),
		func() *B { return base().audio(itemB, 0, 60, 0) })
}
