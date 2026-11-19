package command

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/AlexGalhardo/stuarty/core/timeline"
)

// meetWords is the caption of the examples of C.34 to C.39: "Meet the new editor." at
// frame 12, 48 frames long.
func meetWords(mods ...func(*timeline.Item)) func() *B {
	return func() *B {
		return base().caption(itemC, 12, 4, func(it *timeline.Item) {
			it.Words = []timeline.Word{
				{Text: "Meet", Start: 0, Duration: 9},
				{Text: "the", Start: 9, Duration: 6},
				{Text: "new", Start: 15, Duration: 12},
				{Text: "editor.", Start: 27, Duration: 21},
			}
			it.Duration = 48
			for _, m := range mods {
				m(it)
			}
		})
	}
}

func styleOf(m map[string]any) *timeline.CaptionStyle {
	for k, v := range m {
		if n, ok := v.(int); ok {
			m[k] = json.Number(strconv.Itoa(n))
		}
	}
	s, err := timeline.StyleFromMap(m)
	if err != nil {
		panic(err)
	}
	return s
}

func wordArgs(n, dur int) string {
	parts := make([]string, n)
	for i := range n {
		parts[i] = fmt.Sprintf(`{"text": "w%d", "start": "%df", "duration": "%df"}`, i+1, i*dur, dur)
	}
	return "[" + strings.Join(parts, ", ") + "]"
}

func init() {
	sw := func(name, args string, b func() *B) { gc("caption.set_words", name, b, op("caption.set_words", args)) }
	args := func(rest string) string { return fmt.Sprintf(`{"item_id": %q, %s}`, itemC, rest) }
	hello := `"words": [{"text": "Hello", "start": "0f", "duration": "12f"}, {"text": "world", "start": "12f", "duration": "12f"}]`

	sw("ok_replace_shorter", args(hello), meetWords())
	sw("ok_replace_longer_free_space", args(`"words": `+wordArgs(6, 12)), meetWords())
	sw("ok_explicit_duration_longer_than_last_word", args(hello+`, "duration": "40f"`), meetWords())
	sw("ok_breaks_cleared", args(hello), meetWords(func(it *timeline.Item) { it.Breaks = []int{2} }))
	sw("ok_keyframes_cut_when_shorter", args(hello), meetWords(func(it *timeline.Item) {
		it.Keyframes = timeline.Keyframes{"opacity": {kf(0, 1), kf(10, 0.5), kf(40, 0.2)}}
	}))
	sw("ok_60_words", args(`"words": `+wordArgs(60, 1)), meetWords())
	sw("ok_keyframes_all_dropped_becomes_static_when_shorter", args(hello), meetWords(func(it *timeline.Item) {
		it.Keyframes = timeline.Keyframes{"opacity": {kf(30, 0.5), kf(47, 0.2)}}
	}))
	sw("err_validation_empty_words", args(`"words": []`), meetWords())
	sw("err_validation_words_overlap", args(
		`"words": [{"text": "a", "start": "0f", "duration": "10f"}, {"text": "b", "start": "5f", "duration": "10f"}]`),
		meetWords())
	sw("err_validation_word_with_whitespace", args(
		`"words": [{"text": "two words", "start": "0f", "duration": "10f"}]`), meetWords())
	sw("err_invalid_time_no_unit", args(
		`"words": [{"text": "a", "start": "0f", "duration": "5f"}, {"text": "b", "start": "5", "duration": "5f"}]`),
		meetWords())
	sw("err_invalid_time_zero_word_duration", args(
		`"words": [{"text": "a", "start": "0f", "duration": "0f"}]`), meetWords())
	sw("err_limit_61_words", args(`"words": `+wordArgs(61, 1)), meetWords())
	sw("err_not_found", `{"item_id": "itm_zzzzzzzzzz", `+hello+`}`, meetWords())
	sw("err_type_mismatch_text_item", args(hello), func() *B { return base().text(itemC, vidTr, 0, 60) })
	sw("err_invalid_state_duration_before_last_word_end", args(hello+`, "duration": "20f"`), meetWords())
	sw("err_invalid_state_fade_no_longer_fits", args(hello), meetWords(func(it *timeline.Item) {
		it.TransitionIn = &timeline.Transition{Kind: "fade", Duration: 20}
		it.TransitionOut = &timeline.Transition{Kind: "fade", Duration: 10}
	}))
	sw("err_limit_5000_words", args(`"words": `+wordArgs(30, 2)), func() *B {
		b := meetWords()()
		for i := range 83 {
			b.caption(fmt.Sprintf("itm_k%09d", i), 200+i*700, 60)
		}
		return b
	})
	sw("err_overlap_longer_caption", args(`"words": `+wordArgs(6, 12)), func() *B {
		return meetWords()().caption(itemB, 60, 3)
	})
}

func init() {
	ew := func(name, args string, b func() *B) { gc("caption.edit_word", name, b, op("caption.edit_word", args)) }
	args := func(idx int, text string) string {
		return fmt.Sprintf(`{"item_id": %q, "index": %d, "text": %q}`, itemC, idx, text)
	}
	breaks := func(bs ...int) func() *B {
		return meetWords(func(it *timeline.Item) { it.Breaks = bs })
	}

	ew("ok_replace_text", args(3, "editor"), meetWords())
	ew("ok_replace_with_same_text_noop", args(3, "editor."), meetWords())
	ew("ok_replace_80_chars", args(0, strings.Repeat("a", 80)), meetWords())
	ew("ok_remove_middle_word_leaves_gap", args(1, ""), meetWords())
	ew("ok_remove_first_word", args(0, ""), meetWords())
	ew("ok_remove_last_of_several_words", args(3, ""), meetWords())
	ew("ok_remove_adjusts_breaks", args(1, ""), breaks(1, 2, 3))
	ew("err_validation_negative_index", args(-1, "x"), meetWords())
	ew("err_validation_text_with_whitespace", args(0, "new editor"), meetWords())
	ew("err_validation_text_81_chars", args(0, strings.Repeat("a", 81)), meetWords())
	ew("err_not_found_item", `{"item_id": "itm_zzzzzzzzzz", "index": 0, "text": "x"}`, meetWords())
	ew("err_not_found_index_out_of_range", args(7, "x"), meetWords())
	ew("err_type_mismatch_text_item", args(0, "x"), func() *B { return base().text(itemC, vidTr, 0, 60) })
	ew("err_type_mismatch_checked_before_index", args(99, "x"), func() *B { return base().text(itemC, vidTr, 0, 60) })
	ew("err_invalid_state_remove_only_word", args(0, ""), func() *B { return base().caption(itemC, 0, 1) })
}

func init() {
	sb := func(name, args string, b func() *B) {
		gc("caption.set_breaks", name, b, op("caption.set_breaks", args))
	}
	args := func(rest string) string { return fmt.Sprintf(`{"item_id": %q, "breaks": %s}`, itemC, rest) }
	with := func(bs ...int) func() *B { return meetWords(func(it *timeline.Item) { it.Breaks = bs }) }

	sb("ok_one_break", args("[2]"), meetWords())
	sb("ok_several_breaks", args("[1, 3]"), meetWords())
	sb("ok_break_at_last_allowed_index", args("[3]"), meetWords())
	sb("ok_clear_with_empty_array", args("[]"), with(2))
	sb("ok_same_breaks_noop", args("[2]"), with(2))
	sb("err_validation_index_0", args("[0]"), meetWords())
	sb("err_validation_index_equals_word_count", args("[2, 4]"), meetWords())
	sb("err_validation_not_increasing", args("[3, 2]"), meetWords())
	sb("err_validation_duplicate", args("[2, 2]"), meetWords())
	sb("err_not_found", `{"item_id": "itm_zzzzzzzzzz", "breaks": [1]}`, meetWords())
	sb("err_type_mismatch_text_item", args("[1]"), func() *B { return base().text(itemC, vidTr, 0, 60) })
}

func init() {
	ss := func(name, args string, b func() *B) { gc("caption.set_style", name, b, op("caption.set_style", args)) }
	args := func(rest string) string { return fmt.Sprintf(`{"item_id": %q, "style": %s}`, itemC, rest) }
	styled := func(m map[string]any) func() *B {
		return meetWords(func(it *timeline.Item) { it.Style = styleOf(m) })
	}

	ss("ok_set_override", args(`{"size": 80, "highlight": "word"}`), meetWords())
	ss("ok_merge_keeps_the_other_keys", args(`{"size": 80}`), styled(map[string]any{"size": 60, "uppercase": true}))
	ss("ok_null_removes_one_key", args(`{"uppercase": null}`), styled(map[string]any{"size": 60, "uppercase": true}))
	ss("ok_replace_existing_override", fmt.Sprintf(`{"item_id": %q, "style": {"size": 80}, "replace": true}`, itemC),
		styled(map[string]any{"size": 60, "uppercase": true}))
	ss("ok_key_equal_to_default_overrides_track_style", args(`{"highlight": "none"}`), meetWords())
	ss("ok_clear_with_null", args(`null`), styled(map[string]any{"size": 60}))
	ss("ok_clear_with_empty_object", args(`{}`), styled(map[string]any{"size": 60}))
	ss("ok_clear_when_none_noop", args(`null`), meetWords())
	ss("ok_template_color_reference", args(`{"color": "$accent"}`),
		func() *B { return meetWords()().template() })
	ss("err_validation_bad_highlight", args(`{"highlight": "all"}`), meetWords())
	ss(
		"err_validation_null_value_inside",
		fmt.Sprintf(`{"item_id": %q, "style": {"size": null}, "replace": true}`, itemC),
		meetWords(),
	)
	ss("err_validation_unknown_color_reference", args(`{"color": "$nope"}`),
		func() *B { return meetWords()().template() })
	ss("err_not_found", `{"item_id": "itm_zzzzzzzzzz", "style": {"size": 80}}`, meetWords())
	ss("err_type_mismatch_text_item", args(`{"size": 80}`), func() *B { return base().text(itemC, vidTr, 0, 60) })
	ss("err_font_unknown_family", args(`{"font_family": "Comic"}`), meetWords())
}
