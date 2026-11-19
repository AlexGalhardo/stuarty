package command

import "github.com/AlexGalhardo/stuarty/core/timeline"

func init() {
	f := func(name, args string, b func() *B) { gc("settings.set_fps", name, b, op("settings.set_fps", args)) }

	mixed := func() *B {
		return base().
			text(itemA, vidTr, 0, 60).
			clip(itemB, vidTr, 100, 100, 0).
			text(itemC, capTr, 30, 30)
	}
	f("ok_30_to_24_string_form", `{"fps": "24"}`, mixed)
	f("ok_to_ntsc_30000_1001_string_form", `{"fps": "30000/1001"}`, mixed)
	f("ok_object_form", `{"fps": {"num": 25, "den": 1}}`, mixed)
	f("ok_same_rate_noop", `{"fps": "30"}`, mixed)
	f("ok_duration_became_zero", `{"fps": "12"}`, func() *B {
		return base().text(itemA, vidTr, 0, 10).text(itemB, vidTr, 15, 1)
	})

	// The example of SDD-002 C.3: 30 to 12 fps.
	f("ok_overlap_shortens_earlier_item", `{"fps": "12"}`, func() *B {
		return base().text(itemA, vidTr, 4, 4).text(itemB, vidTr, 8, 5).text(itemC, vidTr, 15, 1)
	})
	f("ok_overlap_equal_starts_shifts_later_item", `{"fps": "12"}`, func() *B {
		return base().text(itemA, vidTr, 4, 1).text(itemB, vidTr, 5, 1)
	})
	f("ok_equal_start_after_rescale", `{"fps": "12"}`, func() *B {
		return base().text(itemA, vidTr, 4, 1).text(itemB, vidTr, 5, 1).text(itemC, vidTr, 6, 1)
	})
	nearEnd := func(extra bool) func() *B {
		return func() *B {
			b := base().clip(itemA, vidTr, 0, 15, 885)
			if extra {
				b.text(itemB, vidTr, 15, 10)
			}
			return b
		}
	}
	f("ok_overlap_after_source_range_shortening", `{"fps": "25"}`, nearEnd(true))
	f("ok_source_range_repaired_at_asset_end", `{"fps": "25"}`, nearEnd(false))
	f("ok_keyframes_rescaled_and_collision_reduced", `{"fps": "12"}`, func() *B {
		return base().text(itemA, vidTr, 0, 30, func(it *timeline.Item) {
			it.Keyframes = timeline.Keyframes{"scale": {kf(0, 1), kf(4, 2, "ease_in"), kf(5, 3), kf(20, 4)}}
		})
	})
	f("ok_caption_words_rescaled_and_repaired", `{"fps": "12"}`, func() *B {
		return base().caption(itemA, 0, 0, func(it *timeline.Item) {
			it.Duration = 30
			it.Words = []timeline.Word{
				{
					Text:     "A",
					Start:    0,
					Duration: 1,
				},
				{Text: "B", Start: 1, Duration: 1},
				{Text: "C", Start: 2, Duration: 10},
			}
		})
	})
	f("ok_caption_keeps_one_word", `{"fps": "1"}`, func() *B {
		return base().caption(itemA, 0, 3)
	})
	f("ok_fades_and_transitions_fit_fix", `{"fps": "1"}`, func() *B {
		return base().clip(itemA, vidTr, 0, 40, 0, func(it *timeline.Item) {
			it.FadeIn, it.FadeOut = 15, 15
			it.TransitionIn = &timeline.Transition{Kind: "fade", Duration: 20}
			it.TransitionOut = &timeline.Transition{Kind: "fade", Duration: 15}
		})
	})
	f("ok_boundary_120_fps", `{"fps": "120"}`, func() *B {
		return base().text(itemA, vidTr, 1000, 100).clip(itemB, vidTr, 2000, 100, 100)
	})
	f("ok_boundary_1_fps", `{"fps": "1"}`, func() *B {
		return base().text(itemA, vidTr, 0, 30).text(itemB, vidTr, 30, 30).text(itemC, vidTr, 60, 30)
	})
	f("err_validation_decimal_string", `{"fps": "29.97"}`, mixed)
	f("err_validation_den_not_1_or_1001", `{"fps": {"num": 25, "den": 2}}`, mixed)
	f("err_validation_121_fps", `{"fps": "121"}`, mixed)
	f("err_invalid_time_item_passes_108000", `{"fps": "60"}`, func() *B {
		return base().text(itemA, vidTr, 100000, 5000)
	})

	r := func(name, args string, b func() *B) { gc("asset.remove", name, b, op("asset.remove", args)) }
	used := func() *B { return base().clip(itemA, vidTr, 0, 30, 0).audio(itemB, 0, 30, 0) }
	r("ok_unused_asset", `{"asset_id": "`+imgAsset+`"}`, used)
	r("ok_asset_used_only_by_deleted_item", `{"asset_id": "`+audAsset+`"}`, func() *B {
		return base().clip(itemA, vidTr, 0, 30, 0)
	})
	r("err_not_found", `{"asset_id": "ast_zzzzzzzzzz"}`, used)
	r("err_validation_placeholder_id", `{"asset_id": "ast_placeholder"}`, used)
	r("err_in_use_one_item", `{"asset_id": "`+vidAsset+`"}`, used)
	r("err_in_use_several_items_sorted", `{"asset_id": "`+vidAsset+`"}`, func() *B {
		return base().clip(itemC, vidTr, 0, 30, 0).clip(itemA, vidTr, 40, 30, 0).clip(itemB, capTr, 0, 30, 0)
	})
}
