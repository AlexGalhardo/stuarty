package command

import "fmt"

// Golden cases of item.crossfade between two neighbours of one track (SDD-002 C.20, VV-042).
func init() {
	cf := func(name, args string, b func() *B) { gc("item.crossfade", name, b, op("item.crossfade", args)) }
	args := func(dur, track string) string {
		return fmt.Sprintf(
			`{"from_item_id": %q, "to_item_id": %q, "duration": %q, "track_id": %q}`,
			itemA,
			itemB,
			dur,
			track,
		)
	}
	// A is 0-150 and B is 150-300 on the same track; B reads its asset from source frame 60.
	pair := func() *B { return base().clip(itemA, vidTr, 0, 150, 0).clip(itemB, vidTr, 150, 150, 60) }

	cf("ok_neighbours_use_the_source_before_the_cut", args("1s", capTr), pair)
	cf("ok_neighbours_image_clip_has_no_source_to_check", args("1s", capTr), func() *B {
		return base().clip(itemA, vidTr, 0, 150, 0).clip(itemB, vidTr, 150, 150, 0, image)
	})
	cf("ok_neighbours_text_items", args("1s", capTr), func() *B {
		return base().text(itemA, vidTr, 0, 150).text(itemB, vidTr, 150, 150)
	})
	cf("ok_neighbours_whole_source_handle_used", args("60f", capTr), pair)
	cf("err_invalid_state_neighbours_do_not_touch", args("1s", capTr), func() *B {
		return base().clip(itemA, vidTr, 0, 150, 0).clip(itemB, vidTr, 160, 150, 60)
	})
	cf("err_invalid_state_neighbours_start_before_frame_0", args("1s", capTr), func() *B {
		return base().clip(itemA, vidTr, 0, 20, 0).clip(itemB, vidTr, 20, 150, 60)
	})
	cf("err_out_of_source_neighbour_has_no_handle", args("1s", capTr), func() *B {
		return base().clip(itemA, vidTr, 0, 150, 0).clip(itemB, vidTr, 150, 150, 10)
	})
	cf("err_overlap_target_track_is_taken", args("1s", capTr), func() *B { return pair().shape(itemC, capTr, 100, 60) })
	cf("err_type_mismatch_target_is_an_audio_track", args("1s", audTr), pair)
	cf("err_type_mismatch_target_is_the_same_track", args("1s", vidTr), pair)
	cf("err_not_found_target_track", args("1s", "trk_zzzzzzzzzz"), pair)
	cf("err_validation_track_id_with_items_on_two_tracks", args("1s", capTr), func() *B {
		return base().clip(itemA, vidTr, 0, 150, 0).clip(itemB, capTr, 120, 150, 0)
	})
}
