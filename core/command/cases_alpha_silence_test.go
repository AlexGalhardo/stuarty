package command

import (
	"fmt"
	"strings"

	"github.com/AlexGalhardo/vukem-video/core/timeline"
)

const itemText = "itm_v2w3x4y5z6"

func init() {
	rs := func(name, args string, b func() *B, ids ...string) {
		if len(ids) > 0 {
			gcIDs("clip.remove_silences", name, b, op("clip.remove_silences", args), ids...)
			return
		}
		gc("clip.remove_silences", name, b, op("clip.remove_silences", args))
	}
	silences := func(pairs ...[2]int) string {
		parts := make([]string, len(pairs))
		for i, p := range pairs {
			parts[i] = fmt.Sprintf(`{"start": %d, "end": %d}`, p[0], p[1])
		}
		return `"resolved": {"silences": [` + strings.Join(parts, ", ") + `]}`
	}
	args := func(rest ...string) string {
		return fmt.Sprintf(`{"item_id": %q, %s}`, itemA, strings.Join(rest, ", "))
	}
	newIDs := `"new_ids": ["itm_s6t7v8w9x0", "itm_c3d4e5f6g7"]`
	// The worked example of C.27: start 30, 300 frames from source frame 60.
	example := func(mods ...func(*timeline.Item)) func() *B {
		return func() *B {
			return base().
				clip(itemA, vidTr, 30, 300, 60, append([]func(*timeline.Item){func(it *timeline.Item) {
					it.FadeIn, it.FadeOut = 6, 9
				}}, mods...)...).
				text(itemText, vidTr, 330, 40)
		}
	}
	three := silences([2]int{100, 160}, [2]int{200, 215}, [2]int{250, 256}, [2]int{340, 360})

	rs("ok_three_pieces_ripple_track", args(newIDs, three), example())
	rs("ok_three_pieces_ripple_none", args(newIDs, `"ripple": "none"`, three), example())
	rs("ok_short_silence_ignored", args(newIDs, silences([2]int{100, 160}, [2]int{250, 256})), example())
	rs("ok_silence_at_start_edge_no_padding_on_outer_side", args(silences([2]int{60, 100})), example())
	rs("ok_silence_at_end_edge_no_padding_on_outer_side", args(silences([2]int{340, 360})), example())
	rs("ok_padding_0", args(newIDs, `"padding": "0f"`, silences([2]int{100, 160})), example())
	rs("ok_padding_larger_than_half_silence_removes_nothing",
		args(`"padding": "10f"`, silences([2]int{100, 116})), example())
	rs("ok_min_silence_custom", args(newIDs, `"min_silence": "5f"`, silences([2]int{100, 160}, [2]int{250, 256})),
		example())
	rs("ok_speed_3_2_piece_durations_floor", args(newIDs, silences([2]int{100, 130})), func() *B {
		return base().clip(itemA, vidTr, 30, 90, 60, func(it *timeline.Item) {
			it.Speed = timeline.Rational{Num: 3, Den: 2}
		})
	})
	rs(
		"ok_single_piece_keeps_both_fades_and_transitions",
		args(silences([2]int{60, 100})),
		example(func(it *timeline.Item) {
			it.TransitionIn = &timeline.Transition{Kind: "fade", Duration: 10}
			it.TransitionOut = &timeline.Transition{Kind: "slide_left", Duration: 12}
		}),
	)
	rs("ok_keyframes_divided_among_pieces", args(newIDs, three), example(func(it *timeline.Item) {
		it.Keyframes = timeline.Keyframes{"scale": {kf(0, 1), kf(150, 2), kf(300, 1)}}
	}))
	rs("ok_slot_stays_on_first_piece", args(newIDs, three), example(func(it *timeline.Item) {
		it.Slot = &timeline.Slot{Name: "hero", Kind: "media", Required: true, DurationPolicy: "fit"}
	}))
	rs("ok_new_ids_partial_rest_generated", args(`"new_ids": ["itm_s6t7v8w9x0"]`, three), example(), "itm_c3d4e5f6g7")
	rs(
		"ok_new_ids_extra_ignored_and_reported",
		args(
			`"new_ids": ["itm_s6t7v8w9x0", "itm_c3d4e5f6g7", "itm_d5e6f7g8h9"]`,
			silences([2]int{100, 160}),
		),
		example(),
	)
	rs("ok_no_silence_noop", args(newIDs, silences([2]int{250, 256})), example())

	rs("err_validation_resolved_missing", args(newIDs), example())
	rs("err_validation_silences_overlap", args(silences([2]int{100, 160}, [2]int{150, 170})), example())
	rs("err_validation_new_ids_duplicate",
		args(`"new_ids": ["itm_s6t7v8w9x0", "itm_s6t7v8w9x0"]`, three), example())
	rs("err_invalid_time_min_silence_zero", args(`"min_silence": "0s"`, three), example())
	rs("err_not_found", `{"item_id": "itm_zzzzzzzzzz", `+three+`}`, example())
	rs("err_type_mismatch_not_a_clip", fmt.Sprintf(`{"item_id": %q, %s}`, itemText, three), example())
	rs("err_type_mismatch_asset_without_audio", args(three), func() *B {
		return example()().with(func(d *timeline.Document) {
			a := d.Assets[vidAsset]
			a.HasAudio = false
			d.Assets[vidAsset] = a
		})
	})
	rs("err_id_exists", args(`"new_ids": ["`+itemText+`", "itm_c3d4e5f6g7"]`, three), example())
	rs("err_invalid_state_all_silence", args(silences([2]int{0, 900})), example())
	rs("err_limit_over_500_items", args(newIDs, three), func() *B {
		return example()().manyItems(vidTr, 498)
	})
}
