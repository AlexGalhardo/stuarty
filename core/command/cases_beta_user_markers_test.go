package command

import (
	"fmt"
	"strings"

	"github.com/AlexGalhardo/vukem-video/core/timeline"
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
}
