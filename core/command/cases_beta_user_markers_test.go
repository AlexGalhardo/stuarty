package command

import (
	"fmt"
	"strings"

	"github.com/AlexGalhardo/stuarty/core/timeline"
)

// Golden cases of marker.add, marker.set and marker.remove (SDD-002 C.56 to C.58).
func init() {
	with := func(marks ...timeline.Marker) func() *B {
		return func() *B {
			b := base()
			b.d.Markers = marks

			return b
		}
	}
	full := func() *B {
		b := base()
		for i := range timeline.MaxMarkers {
			b.d.Markers = append(b.d.Markers, timeline.Marker{Frame: i})
		}

		return b
	}
	two := with(timeline.Marker{Frame: 30, Label: "intro"}, timeline.Marker{Frame: 90})

	ad := func(name, args string, b func() *B) { gc("marker.add", name, b, op("marker.add", args)) }
	ad("ok_first_marker", `{"at": "1s", "label": "drop"}`, with())
	ad("ok_without_label", `{"at": "30f"}`, with())
	ad("ok_between_existing_keeps_order", `{"at": "2s"}`, two)
	ad("ok_frame_0", `{"at": "0f"}`, two)
	ad("err_invalid_state_frame_taken", `{"at": "1s"}`, two)
	ad("err_invalid_time_no_unit", `{"at": "30"}`, with())
	ad(
		"err_validation_label_over_60_characters",
		fmt.Sprintf(`{"at": "1s", "label": %q}`, strings.Repeat("a", 61)),
		with(),
	)
	ad("err_limit_over_200_markers", `{"at": "200f"}`, full)

	st := func(name, args string, b func() *B) { gc("marker.set", name, b, op("marker.set", args)) }
	st("ok_move", `{"at": "1s", "to": "2s"}`, two)
	st("ok_relabel", `{"at": "30f", "label": "start"}`, two)
	st("ok_clear_label", `{"at": "30f", "label": ""}`, two)
	st("ok_move_past_the_next_keeps_order", `{"at": "30f", "to": "120f"}`, two)
	st("ok_same_values_noop", `{"at": "30f", "to": "30f", "label": "intro"}`, two)
	st("err_validation_nothing_to_change", `{"at": "30f"}`, two)
	st("err_not_found_no_marker_there", `{"at": "40f", "label": "x"}`, two)
	st("err_invalid_state_target_taken", `{"at": "30f", "to": "90f"}`, two)
	st("err_invalid_time_no_unit", `{"at": "30f", "to": "45"}`, two)

	rm := func(name, args string, b func() *B) { gc("marker.remove", name, b, op("marker.remove", args)) }
	rm("ok_remove_one", `{"at": "30f"}`, two)
	rm("ok_remove_the_last_one", `{"at": "30f"}`, with(timeline.Marker{Frame: 30}))
	rm("ok_remove_all", `{"all": true}`, two)
	rm("ok_remove_all_none_noop", `{"all": true}`, with())
	rm("err_validation_at_and_all", `{"at": "30f", "all": true}`, two)
	rm("err_validation_neither", `{}`, two)
	rm("err_not_found_no_marker_there", `{"at": "40f"}`, two)
	rm("err_invalid_time_no_unit", `{"at": "30"}`, two)

	// The markers of one item, in timeline positions inside the item (the item starts at frame 30).
	mk := func(marks ...timeline.Marker) func() *B {
		return func() *B {
			return base().clip(itemA, vidTr, 30, 90, 0, func(it *timeline.Item) { it.Markers = marks })
		}
	}
	itemFull := func() *B {
		return base().audio(itemB, 0, 300, 0, func(it *timeline.Item) {
			for i := range timeline.MaxMarkers {
				it.Markers = append(it.Markers, timeline.Marker{Frame: i})
			}
		})
	}
	onItem := func(rest string) string { return fmt.Sprintf(`{"item_id": %q, %s}`, itemA, rest) }
	items := mk(timeline.Marker{Frame: 0, Label: "beat"}, timeline.Marker{Frame: 30})

	ad("ok_item_first_marker", onItem(`"at": "2s", "label": "hit"`), mk())
	ad("ok_item_marker_at_the_first_frame", onItem(`"at": "30f"`), mk(timeline.Marker{Frame: 30}))
	ad("ok_item_marker_keeps_order", onItem(`"at": "50f"`), items)
	ad("ok_item_marker_on_the_last_frame", onItem(`"at": "119f"`), items)
	ad("err_validation_item_marker_before_the_item", onItem(`"at": "29f"`), items)
	ad("err_validation_item_marker_after_the_item", onItem(`"at": "120f"`), items)
	ad("err_invalid_state_item_frame_taken", onItem(`"at": "60f"`), items)
	ad("err_not_found_item", `{"item_id": "itm_zzzzzzzzzz", "at": "60f"}`, items)
	ad("err_validation_item_id_form", `{"item_id": "clip1", "at": "60f"}`, items)
	gc("marker.add", "err_limit_over_200_markers_on_an_item", itemFull,
		op("marker.add", fmt.Sprintf(`{"item_id": %q, "at": "250f"}`, itemB)))

	st("ok_item_move", onItem(`"at": "60f", "to": "90f"`), items)
	st("ok_item_relabel", onItem(`"at": "30f", "label": "start"`), items)
	st("err_not_found_item_marker_there", onItem(`"at": "40f", "label": "x"`), items)
	st("err_invalid_state_item_target_taken", onItem(`"at": "30f", "to": "60f"`), items)
	st("err_validation_item_target_outside", onItem(`"at": "30f", "to": "200f"`), items)

	rm("ok_item_remove_one", onItem(`"at": "30f"`), items)
	rm("ok_item_remove_all", onItem(`"all": true`), items)
	rm("ok_item_remove_all_leaves_the_timeline_markers", onItem(`"all": true`), func() *B {
		b := items()
		b.d.Markers = []timeline.Marker{{Frame: 90}}

		return b
	})
	rm("err_not_found_item_marker_there", onItem(`"at": "40f"`), items)
}
