package command

// Golden cases of item.freeze_frame and of the other commands on a freeze frame (TASK-0254).

import (
	"fmt"

	"github.com/AlexGalhardo/stuarty/core/timeline"
)

const (
	freezeID = "itm_f1g2h3j4k5"
	rightID  = "itm_m2n3p4q5r6"
)

func init() {
	// A 90 frame clip from source frame 10, a clip after it on the same track, a text on the caption track.
	scene := func(mods ...func(*timeline.Item)) func() *B {
		return func() *B {
			return base().clip(itemA, vidTr, 0, 90, 10, mods...).
				clip(itemB, vidTr, 100, 20, 0).
				text(itemC, capTr, 50, 30)
		}
	}
	fz := func(name, args string, b func() *B) {
		gc("item.freeze_frame", name, b, op("item.freeze_frame", args))
	}
	args := func(at, duration string) string {
		if duration == "" {
			return fmt.Sprintf(`{"item_id": %q, "at": %q, "freeze_id": %q, "new_id": %q}`, itemA, at, freezeID, rightID)
		}
		return fmt.Sprintf(`{"item_id": %q, "at": %q, "duration": %q, "freeze_id": %q, "new_id": %q}`,
			itemA, at, duration, freezeID, rightID)
	}

	fz("ok_holds_the_frame_for_the_given_duration", args("30f", "2s"), scene())
	fz("ok_default_duration_is_3_seconds", args("30f", ""), scene())
	fz("ok_later_items_of_the_track_move_other_tracks_do_not", args("30f", "20f"), scene())
	fz("ok_reversed_clip_holds_the_frame_that_is_on_screen", args("30f", "20f"), scene(func(it *timeline.Item) {
		it.Reverse = true
	}))
	fz("ok_sped_up_clip_holds_the_frame_that_is_on_screen", args("20f", "20f"), scene(func(it *timeline.Item) {
		it.Speed, it.Duration = timeline.Rational{Num: 2, Den: 1}, 45
	}))
	fz("ok_keyframes_and_fades_stay_with_the_parts", args("30f", "20f"), scene(func(it *timeline.Item) {
		it.Keyframes = timeline.Keyframes{"x": {timeline.NewKeyframe(0, 0), timeline.NewKeyframe(89, 10)}}
		it.FadeIn, it.FadeOut = 5, 8
		it.TransitionOut = &timeline.Transition{Kind: "fade", Duration: 10}
	}))
	fz("ok_last_frame_of_the_clip", args("89f", "10f"), scene())
	fz("ok_magnet_track_stays_packed", args("30f", "20f"), func() *B {
		return scene()().with(func(d *timeline.Document) { t := d.Tracks[vidTr]; t.Magnet = true; d.Tracks[vidTr] = t })
	})

	fz(
		"err_validation_bad_freeze_id",
		fmt.Sprintf(`{"item_id": %q, "at": "30f", "freeze_id": "freeze"}`, itemA),
		scene(),
	)
	fz("err_validation_same_ids", fmt.Sprintf(`{"item_id": %q, "at": "30f", "freeze_id": %q, "new_id": %q}`,
		itemA, freezeID, freezeID), scene())
	fz("err_invalid_time_zero_duration", args("30f", "0f"), scene())
	fz("err_invalid_time_at_the_start_of_the_clip", args("0f", "1s"), scene())
	fz("err_invalid_time_at_the_end_of_the_clip", args("90f", "1s"), scene())
	fz("err_invalid_time_malformed", args("30", "1s"), scene())
	fz("err_invalid_time_track_would_pass_108000", args("107950f", "3s"), func() *B {
		return base().clip(itemA, vidTr, 107900, 90, 10)
	})
	fz("err_not_found", `{"item_id": "itm_zzzzzzzzzz", "at": "30f"}`, scene())
	fz("err_type_mismatch_audio_item", fmt.Sprintf(`{"item_id": %q, "at": "30f"}`, itemE), func() *B {
		return scene()().audio(itemE, 0, 90, 0)
	})
	fz("err_type_mismatch_image_clip", args("30f", "1s"), func() *B {
		return base().clip(itemA, vidTr, 0, 90, 0, func(it *timeline.Item) { it.Asset = imgAsset })
	})
	fz(
		"err_invalid_state_linked_item",
		args("30f", "1s"),
		scene(func(it *timeline.Item) { it.Link = "lnk_a1b2c3d4e5" }),
	)
	fz("err_invalid_state_already_a_freeze_frame", args("30f", "1s"), scene(func(it *timeline.Item) {
		it.Freeze, it.SourceIn = true, 10
	}))
	fz("err_invalid_state_speed_curve", args("30f", "1s"), scene(func(it *timeline.Item) {
		it.Duration = 60
		it.SpeedCurve = []timeline.SpeedPoint{
			{T: 0, Speed: timeline.Rational{Num: 1, Den: 1}}, {T: 1, Speed: timeline.Rational{Num: 1, Den: 1}},
		}
	}))
	fz("err_id_exists", fmt.Sprintf(`{"item_id": %q, "at": "30f", "freeze_id": %q}`, itemA, itemB), scene())
	fz("err_limit_500_items", args("30f", "1s"), func() *B { return scene()().manyItems(capTr, 497) })

	frozen := func() *B {
		return base().clip(itemA, vidTr, 0, 60, 33, func(it *timeline.Item) { it.Freeze, it.AudioMuted = true, true })
	}
	gc("item.split", "ok_freeze_frame_parts_hold_the_same_frame", frozen,
		op("item.split", fmt.Sprintf(`{"item_id": %q, "at": "20f", "new_id": %q}`, itemA, itemB)))
	gc("item.trim", "ok_freeze_frame_start_keeps_the_held_frame", frozen,
		op("item.trim", fmt.Sprintf(`{"item_id": %q, "edge": "start", "to": "10f"}`, itemA)))
	gc("item.trim", "ok_freeze_frame_end_grows_past_the_clip_length", frozen,
		op("item.trim", fmt.Sprintf(`{"item_id": %q, "edge": "end", "to": "900f"}`, itemA)))
	gc("clip.set_speed", "err_type_mismatch_freeze_frame", frozen,
		op("clip.set_speed", fmt.Sprintf(`{"item_id": %q, "speed": "2x"}`, itemA)))
}
