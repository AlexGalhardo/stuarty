package command

import (
	"fmt"
	"strings"

	"github.com/AlexGalhardo/vukem-video/core/timeline"
)

func withStyle(style string) func(*timeline.Item) {
	return func(it *timeline.Item) {
		b := (&B{d: &timeline.Document{Tracks: map[string]timeline.Track{"t": {}}}}).captionStyle("t", style)
		it.Style = b.d.Tracks["t"].CaptionStyle
	}
}

func wordList(n int) string {
	parts := make([]string, n)
	for i := range parts {
		parts[i] = fmt.Sprintf(`{"text": "w%d", "start": "%df", "duration": "1f"}`, i+1, i)
	}
	return "[" + strings.Join(parts, ", ") + "]"
}

// captionsWithWords fills the document with captions of 60 words each, one after
// the other on the caption track, and returns the number of words.
func captionsWithWords(b *B, captions int) *B {
	for i := range captions {
		b.caption(fmt.Sprintf("itm_x%09d", i), i*1000, 6)
		it := b.d.Items[fmt.Sprintf("itm_x%09d", i)]
		it.Words = make([]timeline.Word, 60)
		for j := range it.Words {
			it.Words[j] = timeline.Word{Text: "w", Start: j, Duration: 1}
		}
		it.Duration = 60
		b.d.Items[it.ID] = it
	}
	return b
}

func init() {
	// clip.add
	ca := func(name, args string, b func() *B) { gc("clip.add", name, b, op("clip.add", args)) }
	clipArgs := func(extra string) string {
		return `{"track_id": "` + vidTr + `", "asset_id": "` + vidAsset + `", "start": "0s", "id": "` + itemA + `"` + extra + `}`
	}
	imageArgs := func(extra string) string {
		return `{"track_id": "` + vidTr + `", "asset_id": "` + imgAsset + `", "id": "` + itemA + `"` + extra + `}`
	}
	ca("ok_video_default_duration_rest_of_asset", clipArgs(``), base)
	ca("ok_video_explicit_duration", clipArgs(`, "duration": "5s"`), base)
	ca("ok_video_source_in", clipArgs(`, "source_in": "2s", "duration": "5s"`), base)
	ca("ok_video_ends_exactly_at_asset_end", clipArgs(`, "source_in": "60f", "duration": "840f"`), base)
	ca("ok_image_default_5s", imageArgs(`, "start": "0s"`), base)
	ca("ok_image_explicit_duration", imageArgs(`, "start": "0s", "duration": "3s"`), base)
	ca("ok_fit_contain", clipArgs(`, "fit": "contain"`), base)
	ca("ok_fit_none", clipArgs(`, "fit": "none"`), base)
	ca(
		"ok_start_omitted_goes_to_the_end_of_the_track",
		`{"track_id": "`+vidTr+`", "asset_id": "`+vidAsset+`", "id": "`+itemA+`", "duration": "1s"}`,
		func() *B { return base().clip(itemB, vidTr, 10, 90, 0) },
	)
	ca(
		"ok_start_omitted_on_empty_track_is_0",
		`{"track_id": "`+vidTr+`", "asset_id": "`+vidAsset+`", "id": "`+itemA+`", "duration": "1s"}`,
		base,
	)
	ca("ok_start_frame_0_on_empty_track", clipArgs(`, "duration": "1s"`), base)
	ca(
		"ok_touching_neighbour",
		`{"track_id": "`+vidTr+`", "asset_id": "`+vidAsset+`", "start": "150f", "duration": "100f", "id": "`+itemB+`"}`,
		func() *B { return base().clip(itemA, vidTr, 0, 150, 0) },
	)
	ca("ok_ends_exactly_at_108000", imageArgs(`, "start": "107850f", "duration": "150f"`), base)
	ca("ok_default_duration_clamped_to_108000", imageArgs(`, "start": "107900f"`), base)
	ca(
		"ok_supplied_id",
		`{"track_id": "`+vidTr+`", "asset_id": "`+vidAsset+`", "start": "0s", "duration": "2s", "id": "itm_k000000001"}`,
		base,
	)
	ca(
		"ok_placeholder_asset_with_duration",
		`{"track_id": "`+vidTr+`", "asset_id": "ast_placeholder", "start": "0s", "duration": "5s", "id": "`+itemA+`"}`,
		func() *B { return base().template() },
	)
	ca(
		"err_validation_placeholder_asset_without_duration",
		`{"track_id": "`+vidTr+`", "asset_id": "ast_placeholder", "start": "0s", "id": "`+itemA+`"}`,
		func() *B { return base().template() },
	)
	ca("err_validation_source_in_on_image", imageArgs(`, "start": "0s", "source_in": "1s"`), base)
	ca("err_validation_bad_fit", clipArgs(`, "fit": "stretch"`), base)
	ca("err_invalid_time_malformed", `{"track_id": "`+vidTr+`", "asset_id": "`+vidAsset+`", "start": "12"}`, base)
	ca("err_invalid_time_zero_duration", clipArgs(`, "duration": "0s"`), base)
	ca("err_invalid_time_ends_after_108000", imageArgs(`, "start": "107950f", "duration": "100f"`), base)
	ca("err_not_found_track", `{"track_id": "trk_zzzzzzzzzz", "asset_id": "`+vidAsset+`", "start": "0s"}`, base)
	ca("err_not_found_asset", `{"track_id": "`+vidTr+`", "asset_id": "ast_zzzzzzzzzz", "start": "0s"}`, base)
	ca("err_type_mismatch_audio_track", `{"track_id": "`+audTr+`", "asset_id": "`+vidAsset+`", "start": "0s"}`, base)
	ca("err_type_mismatch_audio_asset", `{"track_id": "`+vidTr+`", "asset_id": "`+audAsset+`", "start": "0s"}`, base)
	ca("err_id_exists", clipArgs(`, "duration": "1s"`), func() *B { return base().clip(itemA, capTr, 0, 10, 0) })
	ca("err_limit_500_items", clipArgs(`, "duration": "1s"`), func() *B { return base().manyItems(capTr, 500) })
	ca("err_out_of_source_one_frame_too_long", clipArgs(`, "source_in": "60f", "duration": "841f"`), base)
	ca("err_out_of_source_source_in_at_asset_end", clipArgs(`, "source_in": "900f", "duration": "10f"`), base)
	ca(
		"err_overlap",
		`{"track_id": "`+vidTr+`", "asset_id": "`+vidAsset+`", "start": "100f", "duration": "150f", "id": "`+itemA+`"}`,
		func() *B { return base().clip(itemB, vidTr, 90, 90, 0) },
	)

	// text.add
	ta := func(name, args string, b func() *B) { gc("text.add", name, b, op("text.add", args)) }
	textArgs := func(extra string) string {
		return `{"track_id": "` + vidTr + `", "start": "0s", "duration": "2s", "text": "Hello", "id": "` + itemC + `"` + extra + `}`
	}
	ta("ok_minimal", textArgs(``), base)
	ta("ok_with_style", textArgs(`, "style": {"size": 96}`), base)
	ta("ok_style_value_equal_to_default_is_kept", textArgs(`, "style": {"size": 72, "color": "#ffffff"}`), base)
	ta("ok_with_x_y", textArgs(`, "x": 540, "y": 960.5`), base)
	ta(
		"ok_multiline_text",
		`{"track_id": "`+vidTr+`", "start": "0s", "duration": "2s", "text": "Hello\nworld", "id": "`+itemC+`"}`,
		base,
	)
	ta(
		"ok_text_2000_chars",
		`{"track_id": "`+vidTr+`", "start": "0s", "duration": "2s", "text": "`+strings.Repeat(
			"t",
			2000,
		)+`", "id": "`+itemC+`"}`,
		base,
	)
	ta(
		"ok_template_color_reference",
		textArgs(`, "style": {"color": "$accent"}`),
		func() *B { return base().template() },
	)
	ta(
		"ok_supplied_id",
		`{"track_id": "`+vidTr+`", "start": "0s", "duration": "2s", "text": "Hi", "id": "itm_k000000001"}`,
		base,
	)
	ta(
		"ok_ends_exactly_at_108000",
		`{"track_id": "`+vidTr+`", "start": "107940f", "duration": "60f", "text": "End", "id": "`+itemC+`"}`,
		base,
	)
	ta("err_validation_empty_text", `{"track_id": "`+vidTr+`", "start": "0s", "duration": "2s", "text": ""}`, base)
	ta(
		"err_validation_text_2001_chars",
		`{"track_id": "`+vidTr+`", "start": "0s", "duration": "2s", "text": "`+strings.Repeat("t", 2001)+`"}`,
		base,
	)
	ta("err_validation_style_null_value", textArgs(`, "style": {"size": null}`), base)
	ta("err_validation_unknown_style_key", textArgs(`, "style": {"zzz": 1}`), base)
	ta(
		"err_invalid_time_zero_duration",
		`{"track_id": "`+vidTr+`", "start": "0s", "duration": "0s", "text": "Hi"}`,
		base,
	)
	ta(
		"err_invalid_time_ends_after_108000",
		`{"track_id": "`+vidTr+`", "start": "107990f", "duration": "100f", "text": "Hi", "id": "itm_k000000001"}`,
		base,
	)
	ta("err_not_found_track", `{"track_id": "trk_zzzzzzzzzz", "start": "0s", "duration": "2s", "text": "Hi"}`, base)
	ta(
		"err_type_mismatch_audio_track",
		`{"track_id": "`+audTr+`", "start": "0s", "duration": "2s", "text": "Hi"}`,
		base,
	)
	ta("err_id_exists", textArgs(``), func() *B { return base().text(itemC, capTr, 0, 10) })
	ta("err_limit_500_items", textArgs(``), func() *B { return base().manyItems(capTr, 500) })
	ta("err_font_unknown_family", textArgs(`, "style": {"font_family": "Comic"}`), base)
	ta("err_font_unknown_weight", textArgs(`, "style": {"font_family": "JetBrains Mono", "font_weight": 900}`), base)
	ta("err_overlap", `{"track_id": "`+vidTr+`", "start": "30f", "duration": "60f", "text": "Hi", "id": "`+itemC+`"}`,
		func() *B { return base().text(itemA, vidTr, 0, 60) })

	// text.set
	ts := func(name, args string, b func() *B) { gc("text.set", name, b, op("text.set", args)) }
	setArgs := func(extra string) string { return `{"item_id": "` + itemC + `", ` + extra + `}` }
	styledText := func() *B { return base().text(itemC, vidTr, 0, 60, withStyle(`{"size": 96}`)) }
	ts("ok_text_only", setArgs(`"text": "Hello world"`), styledText)
	ts("ok_style_merge", setArgs(`"style": {"color": "#FFE600"}`), styledText)
	ts("ok_style_null_removes_key", setArgs(`"style": {"size": null}`), styledText)
	ts("ok_item_key_equal_to_default_is_kept", setArgs(`"style": {"align": "center"}`), styledText)
	ts("ok_reset_style_only", setArgs(`"reset_style": true`), styledText)
	ts("ok_reset_style_then_style", setArgs(`"reset_style": true, "style": {"color": "#FFE600"}`), styledText)
	ts("ok_text_and_style", setArgs(`"text": "Hello world", "style": {"color": "#FFE600"}`), styledText)
	ts("ok_same_values_noop", setArgs(`"text": "Hello", "style": {"size": 96}`), styledText)
	ts("err_validation_nothing_requested", setArgs(`"reset_style": false`), styledText)
	ts("err_validation_empty_text", setArgs(`"text": ""`), styledText)
	ts("err_validation_reset_style_false_alone", `{"item_id": "`+itemC+`", "reset_style": false}`, styledText)
	ts("err_not_found", `{"item_id": "itm_zzzzzzzzzz", "text": "x"}`, styledText)
	ts(
		"err_type_mismatch_caption_item",
		`{"item_id": "`+itemC+`", "text": "x"}`,
		func() *B { return base().caption(itemC, 0, 3) },
	)
	ts("err_font_unknown_family", setArgs(`"style": {"font_family": "Comic"}`), styledText)
	ts("err_font_unknown_weight", setArgs(`"style": {"font_family": "JetBrains Mono", "font_weight": 900}`), styledText)

	// caption.add
	cp := func(name, args string, b func() *B) { gc("caption.add", name, b, op("caption.add", args)) }
	capArgs := func(extra string) string {
		return `{"track_id": "` + capTr + `", "start": "12f", "id": "` + itemC + `"` + extra + `}`
	}
	word := func(t string, start, dur int) string {
		return fmt.Sprintf(`{"text": %q, "start": "%df", "duration": "%df"}`, t, start, dur)
	}
	cp(
		"ok_words_form",
		capArgs(`, "words": [`+word("Meet", 0, 9)+`, `+word("the", 9, 6)+`, `+word("new", 15, 12)+`]`),
		base,
	)
	cp(
		"ok_words_with_gaps_duration_is_last_end",
		capArgs(`, "words": [`+word("Meet", 0, 9)+`, `+word("you", 20, 10)+`]`),
		base,
	)
	cp(
		"ok_text_form_proportional_largest_remainder",
		capArgs(`, "text": "Meet the new editor.", "duration": "48f", "linked_item": "`+itemA+`"`),
		base,
	)
	cp("ok_text_form_tie_goes_to_lower_index", capArgs(`, "text": "aa bb", "duration": "5f"`), base)
	cp("ok_text_form_duration_equals_word_count", capArgs(`, "text": "a b c", "duration": "3f"`), base)
	cp("ok_text_form_single_word", capArgs(`, "text": "Hello", "duration": "30f"`), base)
	cp("ok_text_form_collapses_runs_of_whitespace", capArgs(`, "text": "a   b\n c", "duration": "30f"`), base)
	cp("ok_60_words", capArgs(`, "words": `+wordList(60)), base)
	cp(
		"ok_linked_item_that_does_not_exist",
		capArgs(`, "text": "Hi there", "duration": "20f", "linked_item": "itm_k000000009"`),
		base,
	)
	cp(
		"ok_supplied_id",
		`{"track_id": "`+capTr+`", "start": "0f", "text": "Hi", "duration": "10f", "id": "itm_k000000001"}`,
		base,
	)
	cp("ok_custom_position_x_y", capArgs(`, "text": "Hi there", "duration": "20f", "x": 540, "y": 1500`), func() *B {
		return base().captionStyle(capTr, `{"position": "custom"}`)
	})
	cp(
		"err_validation_both_words_and_text",
		capArgs(`, "words": [`+word("a", 0, 5)+`], "text": "a", "duration": "5f"`),
		base,
	)
	cp("err_validation_neither_words_nor_text", capArgs(``), base)
	cp("err_validation_duration_with_words", capArgs(`, "words": [`+word("a", 0, 5)+`], "duration": "5f"`), base)
	cp("err_validation_empty_words", capArgs(`, "words": []`), base)
	cp("err_validation_word_with_whitespace", capArgs(`, "words": [`+word("two words", 0, 5)+`]`), base)
	cp("err_validation_word_81_chars", capArgs(`, "words": [`+word(strings.Repeat("w", 81), 0, 5)+`]`), base)
	cp("err_validation_words_overlap", capArgs(`, "words": [`+word("a", 0, 10)+`, `+word("b", 5, 10)+`]`), base)
	cp("err_invalid_time_zero_word_duration", capArgs(`, "words": [`+word("a", 0, 0)+`]`), base)
	cp(
		"err_invalid_time_ends_after_108000",
		`{"track_id": "`+capTr+`", "start": "107990f", "text": "Hi", "duration": "100f", "id": "itm_k000000001"}`,
		base,
	)
	cp("err_not_found_track", `{"track_id": "trk_zzzzzzzzzz", "start": "0f", "text": "Hi", "duration": "10f"}`, base)
	cp(
		"err_type_mismatch_audio_track",
		`{"track_id": "`+audTr+`", "start": "0f", "text": "Hi", "duration": "10f"}`,
		base,
	)
	cp("err_invalid_state_duration_below_word_count", capArgs(`, "text": "a b c d", "duration": "3f"`), base)
	cp(
		"err_id_exists",
		capArgs(`, "text": "Hi", "duration": "10f"`),
		func() *B { return base().text(itemC, vidTr, 0, 10) },
	)
	cp("err_limit_61_words", capArgs(`, "words": `+wordList(61)), base)
	cp("err_limit_5000_words", capArgs(`, "words": `+wordList(21)), func() *B { return captionsWithWords(base(), 83) })
	cp(
		"err_limit_500_items",
		capArgs(`, "text": "Hi", "duration": "10f"`),
		func() *B { return base().manyItems(vidTr, 500) },
	)
	cp(
		"err_overlap",
		capArgs(`, "text": "Hi there", "duration": "48f"`),
		func() *B { return base().text(itemA, capTr, 0, 30) },
	)
}
