package command

import (
	"fmt"
	"strings"

	"github.com/AlexGalhardo/stuarty/core/timeline"
)

func speed(n, d int) func(*timeline.Item) {
	return func(it *timeline.Item) { it.Speed = timeline.Rational{Num: n, Den: d} }
}

func keyframes(prop string, kfs ...timeline.Keyframe) func(*timeline.Item) {
	return func(it *timeline.Item) {
		if it.Keyframes == nil {
			it.Keyframes = timeline.Keyframes{}
		}
		it.Keyframes[prop] = kfs
	}
}

func fades(in int) func(*timeline.Item) {
	return func(it *timeline.Item) { it.FadeIn, it.FadeOut = in, 12 }
}

func transitions(in, out int) func(*timeline.Item) {
	return func(it *timeline.Item) {
		if in > 0 {
			it.TransitionIn = &timeline.Transition{Kind: "fade", Duration: in}
		}
		if out > 0 {
			it.TransitionOut = &timeline.Transition{Kind: "slide_left", Duration: out}
		}
	}
}

func image(it *timeline.Item) { it.Asset = imgAsset }

func ids(prefix string, n int) []string {
	out := make([]string, n)
	for i := range out {
		out[i] = fmt.Sprintf("%s_x%09d", prefix, i)
	}
	return out
}

func jsonList(s []string) string { return `["` + strings.Join(s, `", "`) + `"]` }

func init() {
	// item.move
	mv := func(name, args string, b func() *B) { gc("item.move", name, b, op("item.move", args)) }
	alone := func() *B { return base().clip(itemA, vidTr, 0, 150, 0) }
	mv("ok_move_in_time", `{"item_id": "`+itemA+`", "start": "5s"}`, alone)
	mv("ok_move_to_other_track_same_kind", `{"item_id": "`+itemA+`", "track_id": "`+capTr+`"}`, alone)
	mv("ok_move_time_and_track", `{"item_id": "`+itemA+`", "start": "60f", "track_id": "`+capTr+`"}`, alone)
	mv("ok_move_next_to_neighbour_touching", `{"item_id": "`+itemA+`", "start": "150f"}`, func() *B {
		return alone().clip(itemB, vidTr, 300, 100, 0)
	})
	mv(
		"ok_move_to_frame_0",
		`{"item_id": "`+itemA+`", "start": "0s"}`,
		func() *B { return base().clip(itemA, vidTr, 50, 150, 0) },
	)
	mv("ok_move_ending_at_frame_108000", `{"item_id": "`+itemA+`", "start": "107850f"}`, alone)
	mv("ok_keyframes_and_words_unchanged", `{"item_id": "`+itemA+`", "start": "300f"}`, func() *B {
		return base().clip(itemA, vidTr, 0, 150, 0, keyframes("scale", kf(0, 1), kf(30, 1.3, "ease_out")), fades(10), transitions(15, 15)).
			caption(itemC, 0, 3)
	})
	mv("ok_same_position_noop", `{"item_id": "`+itemA+`", "start": "0f"}`, alone)
	mv("err_validation_no_argument", `{"item_id": "`+itemA+`"}`, alone)
	mv("err_invalid_time_malformed", `{"item_id": "`+itemA+`", "start": "12"}`, alone)
	mv("err_invalid_time_ends_after_108000", `{"item_id": "`+itemA+`", "start": "100000s"}`, alone)
	mv("err_not_found_item", `{"item_id": "itm_zzzzzzzzzz", "start": "1s"}`, alone)
	mv("err_not_found_track", `{"item_id": "`+itemA+`", "track_id": "trk_zzzzzzzzzz"}`, alone)
	mv("err_type_mismatch_audio_track", `{"item_id": "`+itemA+`", "track_id": "`+audTr+`"}`, alone)
	mv(
		"err_overlap_in_time",
		`{"item_id": "`+itemA+`", "start": "100f"}`,
		func() *B { return alone().clip(itemB, vidTr, 200, 100, 0) },
	)
	mv("err_overlap_on_other_track", `{"item_id": "`+itemA+`", "track_id": "`+capTr+`"}`, func() *B {
		return alone().text(itemC, capTr, 100, 100)
	})

	// item.trim
	tr := func(name, args string, b func() *B) { gc("item.trim", name, b, op("item.trim", args)) }
	start := func(to string) string { return `{"item_id": "` + itemA + `", "edge": "start", "to": "` + to + `"}` }
	end := func(to string) string { return `{"item_id": "` + itemA + `", "edge": "end", "to": "` + to + `"}` }
	clip := func(mods ...func(*timeline.Item)) func() *B {
		return func() *B { return base().clip(itemA, vidTr, 90, 150, 60, mods...) }
	}
	spec := clip(
		speed(3, 2),
		fades(10),
		transitions(15, 0),
		keyframes("scale", kf(0, 1), kf(30, 1.3, "ease_out"), kf(100, 1)),
	)
	tr("ok_start_edge_shrink_clip_speed_1", start("100f"), clip())
	tr("ok_start_edge_shrink_clip_speed_3_2_floor", start("97f"), clip(speed(3, 2)))
	tr("ok_start_edge_extend_clip_source_decreases", start("80f"), clip())
	tr(
		"ok_start_edge_extend_to_source_in_exactly_0",
		start("70f"),
		func() *B { return base().clip(itemA, vidTr, 90, 150, 20) },
	)
	tr("ok_start_edge_keyframes_interpolated_at_0", start("97f"), spec)
	tr(
		"ok_start_edge_all_keyframes_dropped_becomes_static",
		start("100f"),
		clip(keyframes("opacity", kf(0, 1), kf(5, 0.5))),
	)
	captionAt := func(words int, mods ...func(*timeline.Item)) func() *B {
		return func() *B { return base().caption(itemC, 100, words, mods...) }
	}
	capItem := func(args string) string { return `{"item_id": "` + itemC + `", ` + args + `}` }
	tr("ok_start_edge_caption_words_removed_and_clamped", capItem(`"edge": "start", "to": "115f"`), captionAt(4))
	tr("ok_start_edge_image_clip_source_in_untouched", start("100f"), clip(image))
	tr(
		"ok_start_edge_text_item",
		capItem(`"edge": "start", "to": "10f"`),
		func() *B { return base().text(itemC, vidTr, 0, 60) },
	)
	tr("ok_start_edge_to_frame_0", start("0f"), func() *B { return base().clip(itemA, vidTr, 90, 150, 100) })
	tr(
		"ok_end_edge_shrink_keyframes_cut",
		end("150f"),
		clip(keyframes("scale", kf(0, 1), kf(30, 1.3, "ease_out"), kf(100, 1))),
	)
	tr("ok_end_edge_extend_within_source", end("300f"), clip())
	tr("ok_end_edge_extend_to_asset_end_exactly", end("930f"), clip())
	tr("ok_end_edge_extend_image_clip", end("400f"), clip(image))
	tr("ok_end_edge_caption_word_clamped", capItem(`"edge": "end", "to": "125f"`), captionAt(4))
	tr(
		"ok_end_edge_to_frame_108000",
		capItem(`"edge": "end", "to": "108000f"`),
		func() *B { return base().text(itemC, vidTr, 90, 150) },
	)
	tr("ok_edge_already_at_to_noop", end("240f"), clip())
	tr("err_validation_bad_edge", `{"item_id": "`+itemA+`", "edge": "left", "to": "100f"}`, clip())
	tr("err_invalid_time_malformed", start("12"), clip())
	tr("err_invalid_time_end_past_108000", end("2000000f"), clip())
	tr("err_not_found", `{"item_id": "itm_zzzzzzzzzz", "edge": "start", "to": "100f"}`, clip())
	tr("err_invalid_state_start_edge_at_item_end", start("240f"), clip())
	tr("err_invalid_state_end_edge_at_item_start", end("90f"), clip())
	oneWord := captionAt(1, func(it *timeline.Item) { it.Duration = 100 })
	tr("err_invalid_state_caption_no_word_left", capItem(`"edge": "start", "to": "120f"`), oneWord)
	tr("err_invalid_state_fade_no_longer_fits", end("105f"), clip(fades(10)))
	tr("err_invalid_state_transition_no_longer_fits", end("110f"), clip(transitions(15, 15)))
	tr("err_out_of_source_start_before_zero", start("60f"), func() *B { return base().clip(itemA, vidTr, 90, 150, 20) })
	tr("err_out_of_source_end_past_asset", end("931f"), clip())
	tr("err_overlap_start_edge_extends_into_neighbour", start("70f"), func() *B {
		return base().clip(itemB, vidTr, 0, 80, 0).clip(itemA, vidTr, 90, 150, 60)
	})
	tr("err_overlap_end_edge_extends_into_neighbour", end("320f"), func() *B {
		return base().clip(itemA, vidTr, 90, 150, 60).clip(itemB, vidTr, 300, 50, 0)
	})

	// item.split
	sp := func(name, args string, b func() *B) { gc("item.split", name, b, op("item.split", args)) }
	at := func(f string) string {
		return `{"item_id": "` + itemA + `", "at": "` + f + `", "new_id": "` + itemC + `"}`
	}
	c := func(mods ...func(*timeline.Item)) func() *B {
		return func() *B { return base().clip(itemA, vidTr, 60, 120, 30, mods...) }
	}
	capSplit := func(words []timeline.Word, breaks []int) func() *B {
		return func() *B {
			return base().caption(itemA, 60, 1, func(it *timeline.Item) {
				it.Words, it.Breaks, it.Duration = words, breaks, 60
			})
		}
	}
	w := func(start, dur int) timeline.Word {
		return timeline.Word{Text: fmt.Sprintf("w%d", start), Start: start, Duration: dur}
	}
	sp("ok_clip_speed_1", at("97f"), c())
	sp("ok_clip_speed_3_2_floor_source_in", at("97f"), c(speed(3, 2)))
	sp("ok_audio_item", at("97f"), func() *B { return base().audio(itemA, 60, 120, 30) })
	sp("ok_image_clip", at("97f"), c(image))
	sp("ok_text_item", at("97f"), func() *B { return base().text(itemA, vidTr, 60, 120) })
	sp("ok_shape_item", at("97f"), func() *B { return base().shape(itemA, vidTr, 60, 120) })
	sp("ok_at_first_frame_plus_1", at("61f"), c())
	sp("ok_at_last_frame", at("179f"), c())
	sp("ok_transition_longer_than_left_part_shortened", at("90f"), c(transitions(40, 0)))
	sp("ok_fade_in_stays_left_fade_out_goes_right", at("97f"), c(fades(12)))
	sp(
		"ok_slot_stays_left",
		at("97f"),
		c(func(it *timeline.Item) { s := timeline.NewSlot("main_video", "media"); it.Slot = &s }),
	)
	sp("ok_keyframes_cut_between_two_keyframes", at("97f"), c(keyframes("opacity", kf(0, 1), kf(60, 0.4))))
	sp("ok_keyframes_cut_exactly_on_a_keyframe", at("97f"), c(keyframes("opacity", kf(0, 1), kf(37, 0.5), kf(80, 0.2))))
	sp("ok_keyframes_all_before_cut", at("97f"), c(keyframes("opacity", kf(0, 1), kf(10, 0.5))))
	sp("ok_keyframes_all_after_cut", at("97f"), c(keyframes("opacity", kf(40, 0.5), kf(60, 0.8))))
	sp("ok_keyframes_100_before_cut_limit_kept", at("160f"), c(func(it *timeline.Item) {
		kfs := make([]timeline.Keyframe, 100)
		for i := range kfs {
			kfs[i] = kf(i, float64(i+1)/100)
		}
		keyframes("scale", kfs...)(it)
	}))
	capAt30 := `{"item_id": "` + itemA + `", "at": "90f", "new_id": "` + itemC + `"}`
	sp("ok_caption_words_split_by_first_frame", capAt30, func() *B {
		return base().caption(itemA, 60, 6)
	})
	sp(
		"ok_caption_word_crossing_cut_clamped_left",
		at("80f"),
		capSplit([]timeline.Word{w(0, 10), w(10, 15), w(25, 15)}, nil),
	)
	sp("ok_caption_breaks_divided", capAt30, func() *B {
		return base().caption(itemA, 60, 6, func(it *timeline.Item) { it.Breaks = []int{2, 4} })
	})
	sp("ok_new_id_supplied", `{"item_id": "`+itemA+`", "at": "97f", "new_id": "itm_k000000001"}`, c())
	sp("err_validation_bad_new_id", `{"item_id": "`+itemA+`", "at": "97f", "new_id": "clip-2"}`, c())
	sp("err_invalid_time_no_unit", `{"item_id": "`+itemA+`", "at": "97"}`, c())
	sp("err_not_found", `{"item_id": "itm_zzzzzzzzzz", "at": "97f"}`, c())
	sp("err_invalid_time_at_equals_start", at("60f"), c())
	sp("err_invalid_time_at_equals_end", at("180f"), c())
	sp("err_invalid_state_caption_right_part_no_word", at("90f"), capSplit([]timeline.Word{w(0, 10)}, nil))
	sp("err_invalid_state_caption_left_part_no_word", at("90f"), capSplit([]timeline.Word{w(40, 10)}, nil))
	sp(
		"err_id_exists",
		`{"item_id": "`+itemA+`", "at": "97f", "new_id": "`+itemB+`"}`,
		func() *B { return c()().clip(itemB, capTr, 0, 10, 0) },
	)
	sp("err_limit_500_items", at("97f"), func() *B { return c()().manyItems(capTr, 499) })

	// item.delete
	d := func(name, args string, b func() *B) { gc("item.delete", name, b, op("item.delete", args)) }
	three := func() *B {
		return base().text("itm_a1b2c3d4e5", vidTr, 0, 60).
			text("itm_f6g7h8j9k0", vidTr, 60, 30).
			text("itm_p7q8r9s0t1", vidTr, 100, 20)
	}
	del := func(list, ripple string) string {
		return `{"item_ids": ` + list + `, "ripple": "` + ripple + `"}`
	}
	d("ok_one_item_ripple_none", del(`["itm_f6g7h8j9k0"]`, "none"), three)
	d("ok_many_items_ripple_none", `{"item_ids": ["itm_a1b2c3d4e5", "itm_p7q8r9s0t1"]}`, three)
	d("ok_200_items", `{"item_ids": `+jsonList(ids("itm", 200))+`}`, func() *B { return base().manyItems(vidTr, 200) })
	d("ok_ripple_track_one_item", del(`["itm_a1b2c3d4e5"]`, "track"), three)
	d(
		"ok_ripple_track_two_items_between_shift_differs",
		del(`["itm_a1b2c3d4e5", "itm_p7q8r9s0t1"]`, "track"),
		func() *B {
			return base().text("itm_a1b2c3d4e5", vidTr, 0, 20).text("itm_b1b2c3d4e5", vidTr, 20, 10).
				text("itm_p7q8r9s0t1", vidTr, 30, 20).text("itm_d1b2c3d4e5", vidTr, 60, 10)
		},
	)
	d("ok_ripple_track_two_tracks_independent", del(`["itm_a1b2c3d4e5", "itm_f6g7h8j9k0"]`, "track"), func() *B {
		return base().text("itm_a1b2c3d4e5", vidTr, 0, 40).text("itm_b1b2c3d4e5", vidTr, 50, 10).
			text("itm_f6g7h8j9k0", capTr, 10, 20).text("itm_d1b2c3d4e5", capTr, 40, 10)
	})
	d("ok_ripple_all_moves_and_drops_timeline_markers", del(`["itm_a1b2c3d4e5"]`, "all"), func() *B {
		return base().text("itm_a1b2c3d4e5", vidTr, 20, 40).text("itm_b1b2c3d4e5", vidTr, 60, 30).
			with(func(d *timeline.Document) {
				d.Markers = []timeline.Marker{{Frame: 5}, {Frame: 30, Label: "cut"}, {Frame: 70, Label: "after"}}
			})
	})
	d("ok_ripple_track_leaves_timeline_markers", del(`["itm_a1b2c3d4e5"]`, "track"), func() *B {
		return base().text("itm_a1b2c3d4e5", vidTr, 20, 40).text("itm_b1b2c3d4e5", vidTr, 60, 30).
			with(func(d *timeline.Document) { d.Markers = []timeline.Marker{{Frame: 70}} })
	})
	d("ok_ripple_all_one_track_range", del(`["itm_a1b2c3d4e5"]`, "all"), func() *B {
		return base().text("itm_a1b2c3d4e5", vidTr, 0, 60).
			text("itm_b1b2c3d4e5", vidTr, 60, 30).
			text("itm_d1b2c3d4e5", capTr, 70, 10)
	})
	d(
		"ok_ripple_all_range_covered_by_items_on_two_tracks",
		del(`["itm_a1b2c3d4e5", "itm_f6g7h8j9k0"]`, "all"),
		func() *B {
			return base().text("itm_a1b2c3d4e5", vidTr, 0, 30).text("itm_f6g7h8j9k0", capTr, 30, 30).
				text("itm_b1b2c3d4e5", vidTr, 60, 20).text("itm_d1b2c3d4e5", capTr, 70, 20)
		},
	)
	d("ok_ripple_track_keeps_gaps", del(`["itm_a1b2c3d4e5"]`, "track"), func() *B {
		return base().text("itm_a1b2c3d4e5", vidTr, 0, 20).
			text("itm_b1b2c3d4e5", vidTr, 50, 10).
			text("itm_d1b2c3d4e5", vidTr, 100, 10)
	})
	d("err_validation_empty_list", `{"item_ids": []}`, three)
	d("err_validation_201_ids", `{"item_ids": `+jsonList(ids("itm", 201))+`}`, three)
	d("err_validation_duplicate_ids", `{"item_ids": ["itm_a1b2c3d4e5", "itm_a1b2c3d4e5"]}`, three)
	d(
		"err_not_found_first_missing_reported",
		`{"item_ids": ["itm_zzzzzzzzzz", "itm_a1b2c3d4e5", "itm_yyyyyyyyyy"]}`,
		three,
	)
	d("err_invalid_state_ripple_all_not_contiguous", del(`["itm_a1b2c3d4e5", "itm_p7q8r9s0t1"]`, "all"), func() *B {
		return base().text("itm_a1b2c3d4e5", vidTr, 0, 60).text("itm_p7q8r9s0t1", vidTr, 90, 30)
	})
	d("err_overlap_ripple_all_straddling_item", del(`["itm_a1b2c3d4e5"]`, "all"), func() *B {
		return base().text("itm_a1b2c3d4e5", vidTr, 0, 60).
			text("itm_b1b2c3d4e5", capTr, 20, 50).
			text("itm_d1b2c3d4e5", capTr, 80, 20)
	})
}
