package command

// Golden cases of item.trim with ripple (SDD-002 C.12).
func init() {
	const next, later, other = "itm_x000000002", "itm_x000000003", "itm_x000000004"
	scene := func() *B {
		return base().
			clip(itemA, vidTr, 30, 90, 30).
			clip(next, vidTr, 150, 60, 0).
			clip(later, vidTr, 300, 30, 0).
			shape(other, capTr, 160, 30)
	}
	tr := func(name, args string, b func() *B) { gc("item.trim", name, b, op("item.trim", args)) }
	a := `{"item_id": "` + itemA + `", `

	tr("ok_ripple_end_edge_shrink_closes_the_gap", a+`"edge": "end", "to": "100f", "ripple": "track"}`, scene)
	tr("ok_ripple_end_edge_extend_pushes_later_items", a+`"edge": "end", "to": "140f", "ripple": "track"}`, scene)
	tr("ok_ripple_start_edge_shrink_keeps_the_start", a+`"edge": "start", "to": "50f", "ripple": "track"}`, scene)
	tr("ok_ripple_start_edge_extend_pushes_later_items", a+`"edge": "start", "to": "10f", "ripple": "track"}`, scene)
	tr("ok_ripple_none_is_the_default", a+`"edge": "end", "to": "100f", "ripple": "none"}`, scene)
	tr("err_overlap_end_edge_extend_without_ripple", a+`"edge": "end", "to": "170f"}`, scene)
	tr("err_invalid_time_ripple_pushes_past_108000", a+`"edge": "end", "to": "140f", "ripple": "track"}`,
		func() *B { return scene().clip("itm_x000000005", vidTr, 107990, 10, 0) })
}
