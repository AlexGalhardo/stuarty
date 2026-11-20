package command

import (
	"fmt"

	"github.com/AlexGalhardo/stuarty/core/timeline"
)

// Golden cases of the composition commands (SDD-002 C.84 to C.86). The production a case runs in is its
// timelines.json: the other timelines the command may read.

const (
	tlB = "prj_b1b1b1b1b1"
	tlC = "prj_c2c2c2c2c2"
	tlD = "prj_d3d3d3d3d3"
	tlE = "prj_e4e4e4e4e4"
	tlF = "prj_f5f5f5f5f5"
	tlG = "prj_g6g6g6g6g6"
)

// nestedTL is a timeline of the production: a text item of the given length on the video track, plus any
// compositions the mods add.
func nestedTL(id string, length int, mods ...func(*timeline.Document)) *timeline.Document {
	b := newCaseDoc()
	b.d.ID, b.d.Name = id, "Timeline "+id[4:6]
	if length > 0 {
		b.text(itemC, vidTr, 0, length)
	}
	for _, m := range mods {
		m(b.d)
	}
	return b.d
}

// holds puts a composition of each timeline, one after the other, on the caption track.
func holds(children ...string) func(*timeline.Document) {
	return func(d *timeline.Document) {
		for i, c := range children {
			it := timeline.NewItem(
				fmt.Sprintf("itm_c%d%d%d%d%d%d%d%d%d", i, i, i, i, i, i, i, i, i),
				timeline.ItemComposition,
				capTr,
				i*10,
				10,
			)
			it.Timeline = c
			d.Items[it.ID] = it
		}
	}
}

// chain makes timelines where each holds the next and the last holds nothing: ids[0] has depth len(ids)-1.
func chain(ids ...string) []*timeline.Document {
	docs := make([]*timeline.Document, len(ids))
	for i, id := range ids {
		if i+1 < len(ids) {
			docs[i] = nestedTL(id, 300, holds(ids[i+1]))
		} else {
			docs[i] = nestedTL(id, 300)
		}
	}
	return docs
}

// propGraph is the production of the random sequences: the document and one nested timeline of 300 frames.
func propGraph(doc *timeline.Document) timeline.Graph {
	return caseGraph{doc.ID: doc, tlB: nestedTL(tlB, 300)}
}

func init() {
	add := func(rest string) string {
		return fmt.Sprintf(`{"track_id": %q, "timeline_id": %q, %s}`, vidTr, tlB, rest)
	}
	one := []*timeline.Document{nestedTL(tlB, 300)}
	ac := func(name, args string, others []*timeline.Document, b func() *B) {
		gcTL("clip.add_composition", name, b, op("clip.add_composition", args), others, itemA)
	}
	empty := base
	withClip := func() *B { return base().clip(itemC, vidTr, 0, 90, 0) }

	ac("ok_default_duration_rest_of_nested", add(`"start": "0s"`), one, empty)
	ac("ok_explicit_duration", add(`"start": "0s", "duration": "5s"`), one, empty)
	ac("ok_source_in", add(`"start": "0s", "source_in": "2s"`), one, empty)
	ac(
		"ok_other_frame_rate_converts_the_length",
		add(`"start": "0s"`),
		[]*timeline.Document{
			nestedTL(tlB, 240, func(d *timeline.Document) { d.Settings.FPS = timeline.Rational{Num: 24, Den: 1} }),
		},
		empty,
	)
	ac("ok_start_omitted_goes_to_the_end_of_the_track", add(`"duration": "2s"`), one, withClip)
	ac("ok_fit_contain", add(`"start": "0s", "fit": "contain"`), one, empty)
	ac("ok_supplied_id", add(fmt.Sprintf(`"start": "0s", "id": %q`, itemB)), one, empty)
	ac("ok_nested_two_levels", add(`"start": "0s"`),
		[]*timeline.Document{nestedTL(tlB, 300, holds(tlC)), nestedTL(tlC, 100)}, empty)
	ac("ok_depth_exactly_four", add(`"start": "0s"`), chain(tlB, tlC, tlD, tlE)[0:4], empty)
	ac("err_validation_bad_fit", add(`"fit": "stretch"`), one, empty)
	ac("err_invalid_time_malformed", add(`"start": "45"`), one, empty)
	ac("err_invalid_time_zero_duration", add(`"start": "0s", "duration": "0f"`), one, empty)
	ac("err_not_found_track", fmt.Sprintf(`{"track_id": "trk_zzzzzzzzzz", "timeline_id": %q}`, tlB), one, empty)
	ac("err_not_found_timeline", fmt.Sprintf(`{"track_id": %q, "timeline_id": %q}`, vidTr, tlG), one, empty)
	gc("clip.add_composition", "err_not_found_no_production", empty,
		op("clip.add_composition", add(`"start": "0s"`)))
	ac("err_type_mismatch_audio_track", fmt.Sprintf(`{"track_id": %q, "timeline_id": %q}`, audTr, tlB), one, empty)
	ac("err_invalid_state_itself", fmt.Sprintf(`{"track_id": %q, "timeline_id": %q}`, vidTr, prjID), one, empty)
	ac("err_invalid_state_cycle_direct", add(`"start": "0s"`),
		[]*timeline.Document{nestedTL(tlB, 300, holds(prjID))}, empty)
	ac("err_invalid_state_cycle_indirect", add(`"start": "0s"`),
		[]*timeline.Document{nestedTL(tlB, 300, holds(tlC)), nestedTL(tlC, 300, holds(prjID))}, empty)
	ac("err_invalid_state_empty_timeline", add(`"start": "0s"`),
		[]*timeline.Document{nestedTL(tlB, 0)}, empty)
	ac("err_limit_depth_five", add(`"start": "0s"`), chain(tlB, tlC, tlD, tlE, tlF), empty)
	ac("err_limit_depth_through_the_timelines_above", fmt.Sprintf(`{"track_id": %q, "timeline_id": %q}`, vidTr, tlG),
		append(chain(tlE, tlD, tlC, tlB, prjID)[0:4], nestedTL(tlG, 300)),
		empty)
	ac("err_limit_500_items", add(`"start": "0s"`), one, func() *B { return base().manyItems(vidTr, 500) })
	ac("err_id_exists", add(fmt.Sprintf(`"start": "300f", "id": %q`, itemC)), one, withClip)
	ac("err_out_of_source_one_frame_too_long", add(`"start": "0s", "duration": "301f"`), one, empty)
	ac("err_out_of_source_source_in_at_the_end", add(`"start": "0s", "source_in": "300f"`), one, empty)
	ac("err_overlap", add(`"start": "30f"`), one, withClip)
}
