package command

import (
	"fmt"
	"strings"

	"github.com/AlexGalhardo/stuarty/core/timeline"
)

func init() {
	dup := func(name, args string, b func() *B) { gc("item.duplicate", name, b, op("item.duplicate", args)) }
	da := func(rest string) string {
		if !strings.Contains(rest, "new_id") {
			rest += `, "new_id": "` + itemC + `"`
		}
		return fmt.Sprintf(`{"item_id": %q, %s}`, itemA, rest)
	}
	srcText := func(mods ...func(*timeline.Item)) func() *B {
		return func() *B { return base().text(itemA, vidTr, 0, 60, mods...) }
	}
	dup("ok_same_track", da(`"start": "3s"`), srcText())
	dup("ok_other_track", da(fmt.Sprintf(`"start": "10f", "track_id": %q`, capTr)), srcText())
	dup("ok_slot_not_copied", da(`"start": "3s"`), func() *B {
		return base().template().text(itemA, vidTr, 0, 60, func(it *timeline.Item) {
			s := timeline.NewSlot("title", "text")
			it.Slot = &s
		})
	})
	dup("ok_keyframes_copied", da(`"start": "100f"`), srcText(func(it *timeline.Item) {
		it.Keyframes = timeline.Keyframes{"scale": {kf(0, 1), kf(30, 2, "ease_out")}}
		it.Name = "Title"
	}))
	dup("ok_caption_words_copied", da(`"start": "200f"`), func() *B { return base().caption(itemA, 0, 3) })
	dup("ok_copy_touching_source", da(`"start": "60f"`), srcText())
	dup("ok_copy_ending_at_108000", da(`"start": "107940f"`), srcText())
	dup("ok_new_id_supplied", da(`"start": "3s", "new_id": "`+itemC+`"`), srcText())
	dup("err_validation_bad_new_id", `{"item_id": "`+itemA+`", "start": "3s", "new_id": "bad"}`, srcText())
	dup("err_invalid_time_end_past_108000", da(`"start": "107970f"`), srcText())
	dup("err_not_found_item", `{"item_id": "itm_zzzzzzzzzz", "start": "3s"}`, srcText())
	dup("err_not_found_track", da(`"start": "3s", "track_id": "trk_zzzzzzzzzz"`), srcText())
	dup("err_type_mismatch_track_kind", da(fmt.Sprintf(`"start": "3s", "track_id": %q`, audTr)), srcText())
	dup("err_id_exists", da(`"start": "3s", "new_id": "`+itemB+`"`),
		func() *B { return base().text(itemA, vidTr, 0, 60).text(itemB, capTr, 500, 10) })
	dup("err_limit_500_items", da(`"start": "200f"`), func() *B {
		return base().text(itemA, vidTr, 0, 60).manyItems(vidTr, 499)
	})
	dup("err_overlap_with_source", da(`"start": "30f"`), srcText())
	dup("err_overlap_with_other_item", da(`"start": "70f"`),
		func() *B { return base().text(itemA, vidTr, 0, 60).text(itemB, vidTr, 100, 30) })

	cf := func(name, args string, b func() *B) { gc("item.crossfade", name, b, op("item.crossfade", args)) }
	pair := func(a, b func(*timeline.Item)) func() *B {
		return func() *B {
			return base().clip(itemA, vidTr, 0, 150, 0, a).clip(itemB, capTr, 120, 150, 0, b)
		}
	}
	none := func(*timeline.Item) {}
	cfArgs := func(d string) string {
		return fmt.Sprintf(`{"from_item_id": %q, "to_item_id": %q, "duration": %q}`, itemA, itemB, d)
	}
	cf("ok_overlap_equals_duration", cfArgs("1s"), pair(none, none))
	cf("ok_overlap_larger_than_duration", cfArgs("20f"), pair(none, none))
	cf("ok_replaces_existing_transitions_and_fades", cfArgs("1s"), pair(
		func(it *timeline.Item) {
			it.TransitionOut = &timeline.Transition{Kind: "slide_left", Duration: 10}
			it.FadeOut = 5
		},
		func(it *timeline.Item) {
			it.TransitionIn = &timeline.Transition{Kind: "slide_up", Duration: 10}
			it.FadeIn = 5
		}))
	cf("ok_text_items_no_audio_fades", cfArgs("1s"), func() *B {
		return base().text(itemA, vidTr, 0, 150).text(itemB, capTr, 120, 150)
	})
	cf("ok_to_shorter_than_from_overlap_equals_to_duration", cfArgs("40f"), func() *B {
		return base().clip(itemA, vidTr, 0, 150, 0).clip(itemB, capTr, 100, 40, 0)
	})
	cf("ok_same_result_twice_noop", cfArgs("1s"), pair(
		func(it *timeline.Item) {
			it.TransitionOut = &timeline.Transition{Kind: "fade", Duration: 30}
			it.FadeOut = 30
		},
		func(it *timeline.Item) {
			it.TransitionIn = &timeline.Transition{Kind: "fade", Duration: 30}
			it.FadeIn = 30
		}))
	cf(
		"err_validation_missing_duration",
		fmt.Sprintf(`{"from_item_id": %q, "to_item_id": %q}`, itemA, itemB),
		pair(none, none),
	)
	cf("err_invalid_time_zero_duration", cfArgs("0f"), pair(none, none))
	cf(
		"err_not_found_from",
		fmt.Sprintf(`{"from_item_id": "itm_zzzzzzzzzz", "to_item_id": %q, "duration": "1s"}`, itemB),
		pair(none, none),
	)
	cf(
		"err_not_found_to",
		fmt.Sprintf(`{"from_item_id": %q, "to_item_id": "itm_zzzzzzzzzz", "duration": "1s"}`, itemA),
		pair(none, none),
	)
	cf("err_type_mismatch_audio_item", cfArgs("1s"), func() *B {
		return base().clip(itemA, vidTr, 0, 150, 0).audio(itemB, 120, 150, 0)
	})
	cf("err_invalid_state_same_track", cfArgs("1s"), func() *B {
		return base().clip(itemA, vidTr, 0, 50, 0).clip(itemB, vidTr, 60, 50, 0)
	})
	cf("err_invalid_state_overlap_too_small", cfArgs("30f"), func() *B {
		return base().clip(itemA, vidTr, 0, 140, 0).clip(itemB, capTr, 120, 150, 0)
	})
	cf("err_invalid_state_from_starts_after_to_starts", cfArgs("1s"), func() *B {
		return base().clip(itemA, vidTr, 200, 150, 0).clip(itemB, capTr, 120, 150, 0)
	})
	cf("err_invalid_state_from_and_to_start_together", cfArgs("1s"), func() *B {
		return base().clip(itemA, vidTr, 0, 150, 0).clip(itemB, capTr, 0, 150, 0)
	})
	cf("err_invalid_state_to_ends_inside_the_fade", cfArgs("30f"), func() *B {
		return base().clip(itemA, vidTr, 0, 150, 0).clip(itemB, capTr, 100, 20, 0)
	})
	cf("err_invalid_state_to_ends_before_from_starts", cfArgs("1s"), func() *B {
		return base().clip(itemA, vidTr, 100, 150, 0).clip(itemB, capTr, 20, 30, 0)
	})
	cf("err_invalid_state_existing_transition_leaves_no_room", cfArgs("60f"), func() *B {
		return base().clip(itemA, vidTr, 0, 150, 0, func(it *timeline.Item) {
			it.TransitionIn = &timeline.Transition{Kind: "fade", Duration: 100}
		}).clip(itemB, capTr, 80, 150, 0)
	})

	cg := func(name, args string, b func() *B) {
		gc("timeline.close_gap", name, b, op("timeline.close_gap", args))
	}
	gap := func() *B {
		return base().text(itemA, vidTr, 0, 60).text(itemB, vidTr, 100, 30).text(itemC, vidTr, 150, 20)
	}
	cgArgs := func(rest string) string { return fmt.Sprintf(`{"track_id": %q, %s}`, vidTr, rest) }
	cg("ok_gap_between_two_items", cgArgs(`"at": "70f"`), gap)
	cg("ok_gap_at_start_of_track", cgArgs(`"at": "10f"`),
		func() *B { return base().text(itemA, vidTr, 40, 60).text(itemB, vidTr, 120, 30) })
	cg("ok_gap_first_frame", cgArgs(`"at": "60f"`), gap)
	cg("ok_gap_last_frame", cgArgs(`"at": "99f"`), gap)
	other := func() *B { return gap().text("itm_x000000001", capTr, 200, 10).text("itm_x000000002", capTr, 0, 20) }
	cg("ok_ripple_all_moves_other_tracks", cgArgs(`"at": "70f", "ripple": "all"`), other)
	cg("ok_ripple_all_moves_and_drops_timeline_markers", cgArgs(`"at": "70f", "ripple": "all"`), func() *B {
		return gap().with(func(d *timeline.Document) {
			d.Markers = []timeline.Marker{{Frame: 50}, {Frame: 80, Label: "in the gap"}, {Frame: 120, Label: "after"}}
		})
	})
	cg("ok_ripple_track_leaves_timeline_markers", cgArgs(`"at": "70f", "ripple": "track"`), func() *B {
		return gap().with(func(d *timeline.Document) { d.Markers = []timeline.Marker{{Frame: 120}} })
	})
	cg("ok_ripple_track_leaves_other_tracks", cgArgs(`"at": "70f", "ripple": "track"`), other)
	cg("err_validation_bad_ripple", cgArgs(`"at": "70f", "ripple": "none"`), gap)
	cg("err_invalid_time_no_unit", cgArgs(`"at": "70"`), gap)
	cg("err_not_found_track", `{"track_id": "trk_zzzzzzzzzz", "at": "70f"}`, gap)
	cg("err_invalid_state_at_inside_item", cgArgs(`"at": "40f"`), gap)
	cg("err_invalid_state_at_after_last_item", cgArgs(`"at": "175f"`), gap)
	cg("err_invalid_state_item_touching_no_gap", cgArgs(`"at": "60f"`),
		func() *B { return base().text(itemA, vidTr, 0, 60).text(itemB, vidTr, 60, 30) })
	cg("err_overlap_ripple_all_other_track", cgArgs(`"at": "70f", "ripple": "all"`), func() *B {
		return base().text(itemA, vidTr, 0, 60).text(itemB, vidTr, 100, 30).
			text("itm_x000000001", capTr, 30, 50).text("itm_x000000002", capTr, 100, 30)
	})
}
