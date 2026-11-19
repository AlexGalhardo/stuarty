package command

import (
	"fmt"
	"strings"

	"github.com/AlexGalhardo/stuarty/core/timeline"
)

// Golden cases of composition.create_from_selection and composition.expand (SDD-002 C.79, C.80).

const (
	tlNew  = "prj_n1n1n1n1n1"
	itemK  = "itm_k1k1k1k1k1"
	linkAB = "lnk_a1a1a1a1a1"
	trkN1  = "trk_n1n1n1n1n1"
	trkN2  = "trk_n2n2n2n2n2"
	itemX  = "itm_x1x1x1x1x1"
)

// compItem puts the composition item itemA of the nested timeline on the video track.
func compItem(nested string, start, dur int, mods ...func(*timeline.Item)) func(*timeline.Document) {
	return func(d *timeline.Document) {
		it := timeline.NewItem(itemA, timeline.ItemComposition, vidTr, start, dur)
		it.Timeline = nested

		for _, m := range mods {
			m(&it)
		}

		d.Items[itemA] = it
	}
}

func init() {
	sel := func(ids ...string) string {
		quoted := make([]string, len(ids))
		for i, id := range ids {
			quoted[i] = fmt.Sprintf("%q", id)
		}

		return `"item_ids": [` + strings.Join(quoted, ", ") + `]`
	}
	args := func(rest string) string {
		return "{" + rest + `, "timeline_id": "` + tlNew + `", "id": "` + itemK + `"}`
	}
	cr := func(name, op string, others []*timeline.Document, b func() *B) {
		gcTL("composition.create_from_selection", name, b, op3("composition.create_from_selection", op), others)
	}
	clipA := func() *B { return base().clip(itemA, vidTr, 30, 90, 0) }
	twoTracks := func() *B { return base().clip(itemA, vidTr, 0, 90, 0).text(itemB, capTr, 10, 60) }

	cr("ok_one_clip", args(sel(itemA)), nil, clipA)
	cr("ok_two_clips_on_two_tracks", args(sel(itemA, itemB)), nil, twoTracks)
	cr("ok_selection_not_at_zero_starts_the_nested_timeline_at_zero", args(sel(itemA, itemB)), nil,
		func() *B { return base().clip(itemA, vidTr, 100, 60, 0).text(itemB, capTr, 130, 60) })
	cr("ok_with_an_audio_item", args(sel(itemA, itemB)), nil,
		func() *B { return base().clip(itemA, vidTr, 0, 90, 0).audio(itemB, 0, 90, 0) })
	cr("ok_a_link_that_is_not_whole_is_removed", args(sel(itemA)), nil, func() *B {
		return base().clip(itemA, vidTr, 0, 90, 0, func(i *timeline.Item) { i.Link = linkAB }).
			audio(itemB, 0, 90, 0, func(i *timeline.Item) { i.Link = linkAB })
	})
	cr(
		"ok_supplied_name_and_ids",
		`{`+sel(itemA)+`, "name": "Intro", "timeline_id": "`+tlNew+`", "id": "`+itemK+`"}`,
		nil,
		clipA,
	)
	cr("ok_nests_a_composition", args(sel(itemA)), []*timeline.Document{nestedTL(tlC, 100)},
		func() *B { return base().with(compItem(tlC, 0, 100)) })
	cr("err_validation_no_items", `{"item_ids": [], "timeline_id": "`+tlNew+`"}`, nil, clipA)
	cr("err_validation_repeated_item", args(sel(itemA, itemA)), nil, clipA)
	cr("err_not_found_item", args(sel(itemA, "itm_zzzzzzzzzz")), nil, clipA)
	cr("err_invalid_state_track_locked", args(sel(itemA)), nil, func() *B {
		return clipA().with(func(d *timeline.Document) {
			tr := d.Tracks[vidTr]
			tr.Locked = true
			d.Tracks[vidTr] = tr
		})
	})
	cr("err_id_exists_timeline", args(sel(itemA)), []*timeline.Document{nestedTL(tlNew, 100)}, clipA)
	cr("err_id_exists_item", `{`+sel(itemA)+`, "timeline_id": "`+tlNew+`", "id": "`+itemA+`"}`, nil, clipA)
	cr("err_overlap_unselected_item_in_range", args(sel(itemB, itemA)), nil, func() *B {
		return base().text(itemB, capTr, 0, 30).clip(itemA, vidTr, 120, 30, 0).text(itemC, capTr, 60, 30)
	})
	cr("err_limit_depth_five", args(sel(itemA)), chain(tlB, tlC, tlD, tlE, tlF),
		func() *B { return base().with(compItem(tlB, 0, 100)) })

	ex := func(name, item string, others []*timeline.Document, b func() *B, generated ...string) {
		gcTL(
			"composition.expand",
			name,
			b,
			op("composition.expand", fmt.Sprintf(`{"item_id": %q}`, item)),
			others,
			generated...)
	}
	parent := func(start int, mods ...func(*timeline.Item)) func() *B {
		return func() *B { return base().with(compItem(tlB, start, 300, mods...)) }
	}
	twoTrackChild := func(mods ...func(*timeline.Document)) *timeline.Document {
		return nestedTL(tlB, 300, append([]func(*timeline.Document){func(d *timeline.Document) {
			it := timeline.NewItem(itemX, timeline.ItemText, capTr, 50, 100)
			it.Text = "Over"
			d.Items[itemX] = it
		}}, mods...)...)
	}
	oneChild := []*timeline.Document{nestedTL(tlB, 300)}

	ex("ok_one_visual_track", itemA, oneChild, parent(0), trkN1)
	ex("ok_two_visual_tracks_keep_their_order", itemA, []*timeline.Document{twoTrackChild()}, parent(0), trkN1, trkN2)
	ex("ok_with_an_audio_track", itemA, []*timeline.Document{nestedTL(tlB, 300, func(d *timeline.Document) {
		it := timeline.NewItem(itemX, timeline.ItemAudio, audTr, 0, 120)
		it.Asset = audAsset
		d.Items[itemX] = it
	})}, parent(0), trkN1, trkN2)
	ex("ok_later_start_shifts_the_items", itemA, oneChild, parent(100), trkN1)
	ex("err_validation_bad_item_id", "x", oneChild, parent(0))
	ex("err_not_found_item", "itm_zzzzzzzzzz", oneChild, parent(0))
	ex("err_not_found_timeline", itemA, []*timeline.Document{nestedTL(tlC, 300)}, parent(0))
	ex("err_type_mismatch_not_a_composition", itemB, oneChild, func() *B { return base().clip(itemB, vidTr, 0, 90, 0) })
	ex("err_invalid_state_opacity", itemA, oneChild, parent(0, func(i *timeline.Item) { i.Opacity = 0.5 }))
	ex(
		"err_invalid_state_window",
		itemA,
		oneChild,
		parent(0, func(i *timeline.Item) { i.SourceIn = 30; i.Duration = 270 }),
	)
	ex("err_invalid_state_other_size", itemA, []*timeline.Document{
		nestedTL(tlB, 300, func(d *timeline.Document) { d.Settings.Width, d.Settings.Height = 1920, 1080 }),
	}, parent(0))
	ex("err_invalid_state_track_locked", itemA, oneChild, func() *B {
		return parent(0)().with(func(d *timeline.Document) {
			tr := d.Tracks[vidTr]
			tr.Locked = true
			d.Tracks[vidTr] = tr
		})
	}, trkN1)
	ex("err_id_exists", itemA, oneChild, func() *B { return parent(0)().text(itemC, capTr, 0, 30) })
	ex("err_limit_500_items", itemA, []*timeline.Document{twoTrackChild()},
		func() *B { return parent(0)().manyItems(capTr, 499) })
}

// op3 is op for a case whose arguments are already the whole JSON object.
func op3(name, args string) string { return op(name, args) }
