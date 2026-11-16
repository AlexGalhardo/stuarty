package command

// Golden cases of item.paste_attributes (TASK-0254).

import (
	"fmt"
	"strings"

	"github.com/AlexGalhardo/vukem-video/core/timeline"
)

func init() {
	f := func(v float64) *float64 { return &v }
	i := func(v int) *int { return &v }
	// A graded clip: every group has a value that is not the default.
	graded := func(it *timeline.Item) {
		it.Brightness, it.Contrast, it.Saturation = 0.2, 1.2, 1.3
		it.Temperature, it.Tint, it.Hue = 0.3, -0.1, 15
		it.Filter, it.FilterAmount, it.Blur = "sepia", 0.6, 4
		it.Fit, it.Crop = "contain", &timeline.Crop{Left: 0.1, Top: 0.05}
		it.X, it.Y, it.Scale, it.Rotation, it.Opacity = f(10), f(-20), 1.5, 5, 0.8
		it.Blend = "screen"
		it.Mask = &timeline.Mask{Shape: "ellipse", X: 0.1, Y: 0.1, Width: 0.8, Height: 0.8, Feather: 10}
		it.FlipX = true
		it.ChromaKey = &timeline.ChromaKey{Color: "#00FF00", Similarity: 0.4, Smoothness: 0.1, Spill: 0.2}
		it.Effects = []timeline.Effect{
			{Kind: "vignette", Amount: f(0.7), Softness: f(0.5)},
			{Kind: "grain", Amount: f(0.3), Size: i(2), Seed: i(7)},
		}
		it.Volume, it.Denoise, it.Voice = 0.5, 0.4, "warm"
	}
	// Source clip at frame 0, a second clip, a text, a shape and an audio item.
	doc := func(dst func(*timeline.Item)) func() *B {
		return func() *B {
			b := base().clip(itemA, vidTr, 0, 90, 0, graded).clip(itemB, vidTr, 100, 90, 0, dst).
				text(itemC, capTr, 0, 30).shape(itemD, capTr, 40, 30).audio(itemE, 0, 90, 0)
			return b
		}
	}
	plain := doc(func(*timeline.Item) {})
	ids := func(ids ...string) string {
		if len(ids) == 0 {
			return ""
		}
		return `"` + strings.Join(ids, `", "`) + `"`
	}
	args := func(to []string, attrs ...string) string {
		return fmt.Sprintf(
			`{"from_item_id": %q, "item_ids": [%s], "attributes": [%s]}`,
			itemA,
			ids(to...),
			ids(attrs...),
		)
	}
	pa := func(name, a string, b func() *B) {
		gc("item.paste_attributes", name, b, op("item.paste_attributes", a))
	}

	pa("ok_color_to_a_clip", args([]string{itemB}, "color"), plain)
	pa("ok_filter_and_blur_to_a_clip", args([]string{itemB}, "filter", "blur"), plain)
	pa("ok_framing_to_a_clip", args([]string{itemB}, "framing"), plain)
	pa("ok_transform_to_a_clip_and_a_text", args([]string{itemB, itemC}, "transform"), plain)
	pa("ok_look_to_a_text", args([]string{itemC}, "blend", "mask", "flip", "chroma_key", "effects"), plain)
	pa("ok_filter_and_blur_to_a_shape", args([]string{itemD}, "filter", "blur"), plain)
	pa("ok_audio_to_an_audio_item", args([]string{itemE}, "audio"), plain)
	pa("ok_skips_the_groups_the_target_lacks", args([]string{itemB, itemC, itemE}, "color", "blend"), plain)
	pa("ok_every_group", args([]string{itemB}, "color", "filter", "blur", "framing", "transform", "blend", "mask",
		"flip", "chroma_key", "effects", "audio"), plain)
	pa("ok_defaults_of_the_source_clear_the_target", args([]string{itemB}, "color", "mask", "effects"),
		func() *B {
			b := base().clip(itemA, vidTr, 0, 90, 0).clip(itemB, vidTr, 100, 90, 0, graded)
			return b
		})
	pa("ok_same_values_noop", args([]string{itemB}, "color"), doc(graded))
	pa("ok_keyframes_of_other_groups_do_not_block", args([]string{itemB}, "color"), doc(func(it *timeline.Item) {
		it.Keyframes = timeline.Keyframes{"x": {timeline.NewKeyframe(0, 0), timeline.NewKeyframe(30, 10)}}
	}))

	pa("err_validation_no_attributes", args([]string{itemB}), plain)
	pa("err_validation_unknown_attribute", args([]string{itemB}, "color", "sparkle"), plain)
	pa("err_validation_attribute_listed_twice", args([]string{itemB}, "color", "color"), plain)
	pa("err_validation_no_targets", args(nil, "color"), plain)
	pa("err_validation_target_is_the_source", args([]string{itemA}, "color"), plain)
	pa("err_validation_target_listed_twice", args([]string{itemB, itemB}, "color"), plain)
	pa("err_validation_bad_target_id", args([]string{"clip2"}, "color"), plain)
	pa(
		"err_not_found_source",
		fmt.Sprintf(`{"from_item_id": "itm_zzzzzzzzzz", "item_ids": [%q], "attributes": ["color"]}`, itemB),
		plain,
	)
	pa("err_not_found_target", args([]string{"itm_zzzzzzzzzz"}, "color"), plain)
	pa("err_type_mismatch_source_lacks_the_group",
		fmt.Sprintf(`{"from_item_id": %q, "item_ids": [%q], "attributes": ["color"]}`, itemC, itemB), plain)
	pa("err_type_mismatch_no_target_has_the_group", args([]string{itemC, itemD}, "color"), plain)
	pa("err_invalid_state_target_transform_has_keyframes", args([]string{itemB}, "transform"),
		doc(func(it *timeline.Item) {
			it.Keyframes = timeline.Keyframes{"x": {timeline.NewKeyframe(0, 0), timeline.NewKeyframe(30, 10)}}
		}))
	pa("err_invalid_state_target_volume_has_keyframes", args([]string{itemB}, "audio"),
		doc(func(it *timeline.Item) {
			it.Keyframes = timeline.Keyframes{"volume": {timeline.NewKeyframe(0, 1), timeline.NewKeyframe(30, 0)}}
		}))
}
