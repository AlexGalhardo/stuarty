package command

import (
	"fmt"
	"strings"

	"github.com/AlexGalhardo/vukem-video/core/timeline"
)

func init() {
	sp := func(name, args string, b func() *B) { gc("clip.set_speed", name, b, op("clip.set_speed", args)) }
	sa := func(rest string) string { return fmt.Sprintf(`{"item_id": %q, %s}`, itemA, rest) }
	rate := func(n, d int) func(*timeline.Item) {
		return func(it *timeline.Item) { it.Speed = timeline.Rational{Num: n, Den: d} }
	}
	// A clip at frame 30 for 90 frames, with a text item after it.
	withText := func(mods ...func(*timeline.Item)) func() *B {
		return func() *B {
			return base().clip(itemA, vidTr, 30, 90, 0, mods...).text(itemC, vidTr, 120, 20)
		}
	}
	alone := func(start, dur, srcIn int, mods ...func(*timeline.Item)) func() *B {
		return func() *B { return base().clip(itemA, vidTr, start, dur, srcIn, mods...) }
	}
	example := func(it *timeline.Item) {
		it.FadeIn, it.FadeOut = 10, 8
		it.TransitionIn = &timeline.Transition{Kind: "fade", Duration: 15}
		it.Keyframes = timeline.Keyframes{"scale": {kf(0, 1), kf(7, 1.2), kf(90, 1)}}
	}

	sp("ok_speed_up_string_form", sa(`"speed": "2x"`), alone(30, 90, 0))
	sp("ok_slow_down_string_form", sa(`"speed": "0.5x"`), alone(30, 90, 0))
	sp("ok_object_form_reduced", sa(`"speed": {"num": 6, "den": 4}`), alone(30, 90, 0))
	sp("ok_back_to_speed_1_removes_field", sa(`"speed": "1x"`), alone(30, 60, 0, rate(3, 2)))
	sp("ok_ripple_track_shortening", sa(`"speed": "1.5x", "ripple": "track"`), withText(example))
	sp("ok_ripple_track_lengthening", sa(`"speed": "0.5x", "ripple": "track"`), withText())
	sp("ok_ripple_none_leaves_gap", sa(`"speed": "1.5x"`), withText())
	sp("ok_duration_ceiling", sa(`"speed": "1.5x"`), alone(0, 10, 0))
	sp("ok_keyframes_rescaled_and_collision_reduced", sa(`"speed": "4x"`), alone(0, 90, 0, func(it *timeline.Item) {
		it.Keyframes = timeline.Keyframes{"scale": {kf(0, 1), kf(4, 1.5), kf(5, 2), kf(90, 1)}}
	}))
	sp("ok_fade_becomes_zero_removed", sa(`"speed": "4x"`), alone(0, 40, 0, func(it *timeline.Item) { it.FadeIn = 1 }))
	sp("ok_transition_min_1_and_fit_fix", sa(`"speed": "4x"`), alone(0, 4, 0, func(it *timeline.Item) {
		it.FadeIn, it.FadeOut = 2, 2
		it.TransitionIn = &timeline.Transition{Kind: "fade", Duration: 2}
		it.TransitionOut = &timeline.Transition{Kind: "fade", Duration: 2}
	}))
	sp("ok_clamped_at_asset_end", sa(`"speed": "0.7x"`), alone(0, 50, 850))
	sp("ok_audio_item", fmt.Sprintf(`{"item_id": %q, "speed": "2x"}`, itemB),
		func() *B { return base().audio(itemB, 0, 60, 0) })
	sp("ok_boundary_speed_quarter_and_4", sa(`"speed": "0.25x"`), alone(0, 10, 0))
	sp("ok_linked_captions_listed_not_changed", sa(`"speed": "2x"`), func() *B {
		return base().clip(itemA, vidTr, 0, 90, 0).
			caption(itemC, 200, 3, func(it *timeline.Item) { it.LinkedItem = itemA })
	})
	sp("ok_same_speed_noop", sa(`"speed": "1.5x"`), alone(0, 60, 0, rate(3, 2)))
	sp("err_validation_speed_above_4", sa(`"speed": "5x"`), alone(0, 90, 0))
	sp("err_validation_speed_below_quarter", sa(`"speed": "0.249x"`), alone(0, 90, 0))
	sp("err_validation_speed_without_x", sa(`"speed": "2"`), alone(0, 90, 0))
	sp("err_not_found", `{"item_id": "itm_zzzzzzzzzz", "speed": "2x"}`, alone(0, 90, 0))
	sp("err_type_mismatch_text_item", fmt.Sprintf(`{"item_id": %q, "speed": "2x"}`, itemC),
		func() *B { return base().text(itemC, vidTr, 0, 60) })
	sp(
		"err_type_mismatch_image_clip",
		sa(`"speed": "2x"`),
		alone(0, 60, 0, func(it *timeline.Item) { it.Asset = imgAsset }),
	)
	sp("err_out_of_source_no_frame_fits", sa(`"speed": "4x"`), alone(0, 1, 899))
	sp("err_invalid_time_ripple_pushes_item_past_108000", sa(`"speed": "0.5x", "ripple": "track"`), func() *B {
		return base().clip(itemA, vidTr, 100000, 100, 0).text(itemC, vidTr, 107990, 10)
	})
	sp("err_overlap_lengthening_without_ripple", sa(`"speed": "0.5x"`), withText())

	// Speed curve.
	ramp := `"curve": [{"t": 0, "speed": "1x"}, {"t": 0.5, "speed": "3x"}, {"t": 1, "speed": "1x"}]`
	curved := func(pts ...timeline.SpeedPoint) func(*timeline.Item) {
		return func(it *timeline.Item) { it.SpeedCurve, it.Speed = pts, timeline.CurveMean(pts) }
	}
	pt := func(t float64, n int) timeline.SpeedPoint {
		return timeline.SpeedPoint{T: t, Speed: timeline.Rational{Num: n, Den: 1}}
	}
	sp("ok_curve_ramp_up_and_down", sa(ramp), alone(30, 90, 0))
	sp(
		"ok_curve_two_points_is_a_ramp",
		sa(`"curve": [{"t": 0, "speed": "0.5x"}, {"t": 1, "speed": "2x"}]`),
		alone(0, 60, 0),
	)
	sp(
		"ok_curve_eight_points",
		sa(`"curve": [{"t": 0, "speed": "1x"}, {"t": 0.1, "speed": "2x"}, {"t": 0.2, "speed": "0.5x"}, `+
			`{"t": 0.3, "speed": "4x"}, {"t": 0.4, "speed": "1x"}, {"t": 0.5, "speed": "0.25x"}, {"t": 0.75, "speed": "3x"}, `+
			`{"t": 1, "speed": "1x"}]`),
		alone(0, 120, 0),
	)
	sp("ok_curve_ripple_track_with_keyframes_and_transition", sa(ramp+`, "ripple": "track"`), withText(example))
	sp("ok_curve_over_constant_speed", sa(ramp), alone(0, 60, 0, rate(3, 2)))
	sp("ok_curve_replaces_curve", sa(`"curve": [{"t": 0, "speed": "2x"}, {"t": 1, "speed": "2x"}]`),
		alone(0, 60, 0, curved(pt(0, 1), pt(1, 3))))
	sp("ok_speed_clears_curve", sa(`"speed": "1.5x"`), alone(0, 60, 0, curved(pt(0, 1), pt(1, 3))))
	sp(
		"ok_curve_clamped_at_asset_end",
		sa(`"curve": [{"t": 0, "speed": "1x"}, {"t": 1, "speed": "0.5x"}]`),
		alone(0, 50, 850),
	)
	sp("ok_curve_audio_item", fmt.Sprintf(`{"item_id": %q, %s}`, itemB, ramp),
		func() *B { return base().audio(itemB, 0, 60, 0) })
	// Reverse.
	reversed := func(it *timeline.Item) { it.Reverse = true }
	sp("ok_reverse_alone", sa(`"reverse": true`), alone(30, 60, 10))
	sp("ok_reverse_with_speed", sa(`"speed": "2x", "reverse": true`), alone(30, 90, 10))
	sp("ok_reverse_with_curve", sa(ramp+`, "reverse": true`), alone(30, 90, 10))
	sp("ok_reverse_false_removes_field", sa(`"reverse": false`), alone(30, 60, 10, reversed))
	sp("ok_speed_keeps_reverse", sa(`"speed": "0.5x"`), alone(30, 60, 10, reversed))
	sp("ok_reverse_keeps_curve", sa(`"reverse": true`), alone(30, 60, 10, curved(pt(0, 1), pt(1, 3))))
	sp("ok_reverse_audio_item", fmt.Sprintf(`{"item_id": %q, "reverse": true}`, itemB),
		func() *B { return base().audio(itemB, 0, 60, 0) })
	sp("ok_reverse_same_value_noop", sa(`"reverse": true`), alone(30, 60, 10, reversed))
	sp("err_validation_reverse_not_boolean", sa(`"reverse": "yes"`), alone(30, 60, 10))
	sp("err_type_mismatch_reverse_on_image_clip", sa(`"reverse": true`),
		alone(0, 60, 0, func(it *timeline.Item) { it.Asset = imgAsset }))
	sp("err_out_of_source_reverse_speed_no_frame_fits", sa(`"speed": "4x", "reverse": true`), alone(0, 1, 899))
	sp("err_validation_curve_one_point", sa(`"curve": [{"t": 0, "speed": "1x"}]`), alone(0, 60, 0))
	sp(
		"err_validation_curve_nine_points",
		sa(`"curve": [`+strings.Repeat(`{"t": 0, "speed": "1x"}, `, 8)+`{"t": 1, "speed": "1x"}]`),
		alone(0, 60, 0),
	)
	sp(
		"err_validation_curve_first_t_not_0",
		sa(`"curve": [{"t": 0.1, "speed": "1x"}, {"t": 1, "speed": "2x"}]`),
		alone(0, 60, 0),
	)
	sp(
		"err_validation_curve_last_t_not_1",
		sa(`"curve": [{"t": 0, "speed": "1x"}, {"t": 0.9, "speed": "2x"}]`),
		alone(0, 60, 0),
	)
	sp(
		"err_validation_curve_t_not_increasing",
		sa(
			`"curve": [{"t": 0, "speed": "1x"}, {"t": 0.5, "speed": "2x"}, {"t": 0.5, "speed": "2x"}, {"t": 1, "speed": "1x"}]`,
		),
		alone(0, 60, 0),
	)
	sp(
		"err_validation_curve_t_4_decimals",
		sa(`"curve": [{"t": 0, "speed": "1x"}, {"t": 0.5004, "speed": "2x"}, {"t": 1, "speed": "1x"}]`),
		alone(0, 60, 0),
	)
	sp(
		"err_validation_curve_speed_above_4",
		sa(`"curve": [{"t": 0, "speed": "1x"}, {"t": 1, "speed": "5x"}]`),
		alone(0, 60, 0),
	)
	sp("err_validation_curve_speed_rational_not_thousandths",
		sa(`"curve": [{"t": 0, "speed": "1x"}, {"t": 1, "speed": {"num": 1, "den": 3}}]`), alone(0, 60, 0))
	sp("err_validation_speed_and_curve", sa(`"speed": "2x", `+ramp), alone(0, 60, 0))
	sp("err_validation_no_speed_no_curve", fmt.Sprintf(`{"item_id": %q}`, itemA), alone(0, 60, 0))
	sp(
		"err_type_mismatch_curve_on_image_clip",
		sa(ramp),
		alone(0, 60, 0, func(it *timeline.Item) { it.Asset = imgAsset }),
	)
	sp(
		"err_out_of_source_curve_no_frame_fits",
		sa(`"curve": [{"t": 0, "speed": "4x"}, {"t": 1, "speed": "4x"}]`),
		alone(0, 1, 899),
	)
}
