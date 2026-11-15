package command

import (
	"fmt"

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
}
