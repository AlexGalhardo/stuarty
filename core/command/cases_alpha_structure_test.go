package command

import (
	"fmt"

	"github.com/AlexGalhardo/vukem-video/core/timeline"
)

func init() {
	cs := func(name, args string, b func() *B) { gc("caption.split", name, b, op("caption.split", args)) }
	args := func(k int) string {
		return fmt.Sprintf(`{"item_id": %q, "before_word": %d, "new_id": %q}`, itemC, k, itemB)
	}
	linked := func(it *timeline.Item) { it.LinkedItem = itemA }

	cs("ok_split_in_the_middle", args(2), meetWords(linked))
	cs("ok_before_word_1", args(1), meetWords())
	cs("ok_before_last_word", args(3), meetWords())
	cs("ok_gap_before_cut_word_stays_in_left_duration", args(1), meetWords(func(it *timeline.Item) {
		it.Words[1].Start = 15
		it.Words[2].Start = 21
		it.Words[3].Start = 33
		it.Duration = 60
	}))
	cs("ok_breaks_divided", args(2), meetWords(func(it *timeline.Item) { it.Breaks = []int{1, 3} }))
	cs("ok_style_and_linked_item_copied_to_right", args(2), meetWords(linked, func(it *timeline.Item) {
		it.Style = styleOf(map[string]any{"size": 60})
	}))
	cs("ok_transition_in_left_transition_out_right", args(2), meetWords(func(it *timeline.Item) {
		it.TransitionIn = &timeline.Transition{Kind: "fade", Duration: 5}
		it.TransitionOut = &timeline.Transition{Kind: "fade", Duration: 8}
	}))
	cs("ok_keyframes_divided", args(2), meetWords(func(it *timeline.Item) {
		it.Keyframes = timeline.Keyframes{"opacity": {kf(0, 1), kf(20, 0.5), kf(40, 0.2)}}
	}))
	cs("ok_keyframes_all_before_cut", args(2), meetWords(func(it *timeline.Item) {
		it.Keyframes = timeline.Keyframes{"opacity": {kf(0, 1), kf(10, 0.5)}}
	}))
	cs("ok_keyframes_all_after_cut", args(2), meetWords(func(it *timeline.Item) {
		it.Keyframes = timeline.Keyframes{"opacity": {kf(20, 0.5), kf(40, 1)}}
	}))
	cs("ok_slot_stays_left", args(2), meetWords(func(it *timeline.Item) {
		it.Slot = &timeline.Slot{Name: "title", Kind: "caption", Required: true, DurationPolicy: "fit"}
	}))
	cs("err_validation_before_word_0", args(0), meetWords())
	cs(
		"err_not_found",
		fmt.Sprintf(`{"item_id": "itm_zzzzzzzzzz", "before_word": 1, "new_id": %q}`, itemB),
		meetWords(),
	)
	cs("err_not_found_before_word_equals_word_count", args(4), meetWords())
	cs("err_type_mismatch_text_item", args(1), func() *B { return base().text(itemC, vidTr, 0, 60) })
	cs("err_id_exists", fmt.Sprintf(`{"item_id": %q, "before_word": 1, "new_id": %q}`, itemC, itemA),
		func() *B { return meetWords()().clip(itemA, vidTr, 0, 60, 0) })
	cs("err_limit_500_items", args(1), func() *B { return meetWords()().manyItems(vidTr, timeline.MaxItems-1) })
}

func init() {
	cm := func(name, args string, b func() *B) { gc("caption.merge", name, b, op("caption.merge", args)) }
	ids := fmt.Sprintf(`{"item_ids": [%q, %q]}`, itemC, itemB)
	// Two captions of C.37: "Meet the" and "new editor." on the captions track.
	pair := func(secondStart int, first, second func(*timeline.Item)) func() *B {
		return func() *B {
			b := base().caption(itemC, 12, 2, func(it *timeline.Item) {
				it.Words = []timeline.Word{{Text: "Meet", Start: 0, Duration: 9}, {Text: "the", Start: 9, Duration: 6}}
				it.Duration = 15
				if first != nil {
					first(it)
				}
			})
			return b.caption(itemB, secondStart, 2, func(it *timeline.Item) {
				it.Words = []timeline.Word{
					{Text: "new", Start: 0, Duration: 12},
					{Text: "editor.", Start: 12, Duration: 21},
				}
				it.Duration = 33
				if second != nil {
					second(it)
				}
			})
		}
	}
	adjacent := pair(27, nil, nil)

	cm("ok_adjacent_captions", ids, adjacent)
	cm("ok_ids_given_in_reverse_order", fmt.Sprintf(`{"item_ids": [%q, %q]}`, itemB, itemC), adjacent)
	cm("ok_gap_between_captions_spans_both", ids, pair(40, nil, nil))
	cm("ok_breaks_combined", ids, pair(27, func(it *timeline.Item) { it.Breaks = []int{1} },
		func(it *timeline.Item) { it.Breaks = []int{1} }))
	cm("ok_style_of_second_dropped_and_reported", ids, pair(27, nil, func(it *timeline.Item) {
		it.Style = styleOf(map[string]any{"size": 60})
		it.Name = "second"
	}))
	cm("ok_linked_item_taken_from_second_when_first_has_none", ids,
		pair(27, nil, func(it *timeline.Item) { it.LinkedItem = itemA }))
	cm("ok_transition_out_taken_from_second", ids, pair(27, nil, func(it *timeline.Item) {
		it.TransitionOut = &timeline.Transition{Kind: "fade", Duration: 8}
	}))
	cm("ok_merged_exactly_60_words", ids, func() *B { return base().caption(itemC, 0, 30).caption(itemB, 300, 30) })
	cm("err_validation_three_ids", fmt.Sprintf(`{"item_ids": [%q, %q, %q]}`, itemC, itemB, itemA), adjacent)
	cm("err_validation_same_id_twice", fmt.Sprintf(`{"item_ids": [%q, %q]}`, itemC, itemC), adjacent)
	cm("err_not_found", fmt.Sprintf(`{"item_ids": [%q, "itm_zzzzzzzzzz"]}`, itemC), adjacent)
	cm("err_type_mismatch_text_item", ids, func() *B {
		return base().caption(itemC, 12, 2).text(itemB, vidTr, 27, 30)
	})
	cm("err_invalid_state_different_tracks", ids, pair(27, nil, func(it *timeline.Item) { it.Track = vidTr }))
	cm("err_invalid_state_item_between", ids, func() *B {
		return pair(60, nil, nil)().text("itm_p7q8r9s0t1", capTr, 40, 20)
	})
	cm("err_invalid_state_keyframes_present", ids, pair(27, func(it *timeline.Item) {
		it.Keyframes = timeline.Keyframes{"opacity": {kf(0, 1), kf(10, 0.5)}}
	}, nil))
	cm("err_limit_61_words", ids, func() *B { return base().caption(itemC, 0, 31).caption(itemB, 310, 30) })
}
