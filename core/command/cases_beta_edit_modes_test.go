package command

// Golden cases of the roll edit (item.trim with roll), the slide edit (item.move with slide) and
// the slip edit (item.slip), SDD-002 C.11, C.12 and C.59.
func init() {
	const next, prev, other = "itm_x000000002", "itm_x000000003", "itm_x000000004"
	// A at 100-250 (source 30), B right after it at 250-340, P right before at 40-100.
	scene := func() *B {
		return base().
			clip(prev, vidTr, 40, 60, 0).
			clip(itemA, vidTr, 100, 150, 30).
			clip(next, vidTr, 250, 90, 100).
			shape(other, capTr, 120, 30)
	}
	a := `{"item_id": "` + itemA + `", `

	tr := func(name, args string, b func() *B) { gc("item.trim", name, b, op("item.trim", args)) }
	tr("ok_roll_end_edge_earlier_grows_the_next_item", a+`"edge": "end", "to": "200f", "roll": true}`, scene)
	tr("ok_roll_end_edge_later_shrinks_the_next_item", a+`"edge": "end", "to": "290f", "roll": true}`, scene)
	tr("ok_roll_start_edge_later_grows_the_previous_item", a+`"edge": "start", "to": "130f", "roll": true}`, scene)
	tr("ok_roll_start_edge_earlier_shrinks_the_previous_item", a+`"edge": "start", "to": "70f", "roll": true}`, scene)
	tr("err_validation_roll_with_ripple", a+`"edge": "end", "to": "200f", "roll": true, "ripple": "track"}`, scene)
	tr("err_invalid_state_roll_without_a_touching_neighbour", a+`"edge": "end", "to": "200f", "roll": true}`,
		func() *B { return base().clip(itemA, vidTr, 100, 150, 30).clip(next, vidTr, 300, 90, 100) })
	tr("err_invalid_state_roll_leaves_no_frame_in_the_neighbour", a+`"edge": "end", "to": "340f", "roll": true}`, scene)
	tr("err_out_of_source_roll_neighbour_reads_before_zero", a+`"edge": "end", "to": "200f", "roll": true}`,
		func() *B { return base().clip(itemA, vidTr, 100, 150, 30).clip(next, vidTr, 250, 90, 10) })

	mv := func(name, args string, b func() *B) { gc("item.move", name, b, op("item.move", args)) }
	mv("ok_slide_later_trims_the_previous_and_next", a+`"start": "120f", "slide": true}`, scene)
	mv("ok_slide_earlier_trims_the_previous_and_next", a+`"start": "80f", "slide": true}`, scene)
	mv("ok_slide_without_neighbours_is_a_move", a+`"start": "120f", "slide": true}`,
		func() *B { return base().clip(itemA, vidTr, 100, 150, 30) })
	mv("err_validation_slide_without_start", a+`"track_id": "`+capTr+`", "slide": true}`, scene)
	mv("err_validation_slide_with_track_id", a+`"start": "120f", "track_id": "`+vidTr+`", "slide": true}`, scene)
	mv("err_invalid_state_slide_leaves_no_frame_in_the_next_item", a+`"start": "340f", "slide": true}`, scene)
	mv("err_out_of_source_slide_earlier_next_item_reads_before_zero", a+`"start": "70f", "slide": true}`,
		func() *B { return base().clip(itemA, vidTr, 100, 150, 30).clip(next, vidTr, 250, 90, 10) })
	mv("err_overlap_slide_into_a_non_touching_item", a+`"start": "120f", "slide": true}`,
		func() *B { return base().clip(itemA, vidTr, 100, 150, 30).clip(next, vidTr, 260, 90, 100) })
	mv("err_overlap_slide_extends_the_previous_over_another_item", a+`"start": "400f", "slide": true}`,
		func() *B {
			return base().clip(prev, vidTr, 40, 60, 0).clip(itemA, vidTr, 100, 30, 30).clip(next, vidTr, 300, 60, 100)
		})

	sl := func(name, args string, b func() *B) { gc("item.slip", name, b, op("item.slip", args)) }
	sl("ok_slip_later", a+`"source_in": "2s"}`, scene)
	sl("ok_slip_to_zero", a+`"source_in": "0f"}`, scene)
	sl("ok_slip_to_the_end_of_the_asset", a+`"source_in": "750f"}`, scene)
	sl("ok_slip_with_speed", a+`"source_in": "100f"}`,
		func() *B { return base().clip(itemA, vidTr, 100, 150, 30, speed(2, 1)) })
	sl("ok_slip_audio", `{"item_id": "`+itemB+`", "source_in": "60f"}`,
		func() *B { return base().audio(itemB, 0, 300, 0) })
	sl("ok_slip_same_source_in_noop", a+`"source_in": "30f"}`, scene)
	sl("err_validation_no_source_in", `{"item_id": "`+itemA+`"}`, scene)
	sl("err_invalid_time_malformed", a+`"source_in": "soon"}`, scene)
	sl("err_not_found_item", `{"item_id": "itm_zzzzzzzzzz", "source_in": "1s"}`, scene)
	sl("err_type_mismatch_image_clip", a+`"source_in": "1s"}`,
		func() *B { return base().clip(itemA, vidTr, 100, 90, 0, image) })
	sl("err_type_mismatch_text_item", a+`"source_in": "1s"}`,
		func() *B { return base().text(itemA, vidTr, 100, 90) })
	sl("err_out_of_source_past_the_asset", a+`"source_in": "751f"}`, scene)
}
