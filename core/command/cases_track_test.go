package command

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/AlexGalhardo/stuarty/core/timeline"
)

// styled gives the caption track a caption style, from a JSON object.
func styled(style string) func() *B {
	return func() *B { return base().captionStyle(capTr, style) }
}

func (b *B) captionStyle(track, style string) *B {
	var m map[string]any
	dec := json.NewDecoder(strings.NewReader(style))
	dec.UseNumber()
	if err := dec.Decode(&m); err != nil {
		panic(err)
	}
	s, err := timeline.StyleFromMap(m)
	if err != nil {
		panic(err)
	}
	t := b.d.Tracks[track]
	t.CaptionStyle = s
	b.d.Tracks[track] = t
	return b
}

func tracksUpTo(n int) *B {
	b := base()
	for i := len(b.d.Tracks); i < n; i++ {
		id := fmt.Sprintf("trk_x%09d", i)
		b.d.Tracks[id] = timeline.Track{ID: id, Kind: timeline.TrackVisual, Order: fmt.Sprintf("c%02d", i)}
	}
	return b
}

func init() {
	// track.add
	a := func(name, args string, b func() *B) { gc("track.add", name, b, op("track.add", args)) }
	a("ok_default_top_on_empty_document", `{"kind": "visual", "id": "trk_k000000001"}`, func() *B {
		return base().with(func(d *timeline.Document) { d.Tracks = map[string]timeline.Track{} })
	})
	a("ok_default_top", `{"kind": "visual", "id": "trk_k000000001"}`, base)
	a("ok_audio_explicit_top", `{"kind": "audio", "position": "top", "id": "trk_k000000001"}`, base)
	a("ok_position_bottom", `{"kind": "visual", "position": "bottom", "id": "trk_k000000001"}`, base)
	a("ok_above", `{"kind": "visual", "above": "`+vidTr+`", "id": "trk_k000000001"}`, base)
	a("ok_below", `{"kind": "visual", "below": "`+capTr+`", "id": "trk_k000000001"}`, base)
	a("ok_audio_kind", `{"kind": "audio", "name": "Voice", "id": "trk_k000000001"}`, base)
	a("ok_name_60_chars", `{"kind": "visual", "name": "`+strings.Repeat("n", 60)+`", "id": "trk_k000000001"}`, base)
	a("ok_supplied_id", `{"kind": "visual", "id": "trk_k000000002"}`, base)
	a("ok_49_tracks_then_add", `{"kind": "visual", "id": "trk_k000000001"}`, func() *B { return tracksUpTo(49) })
	a("err_validation_two_placements", `{"kind": "visual", "position": "top", "above": "`+vidTr+`"}`, base)
	a("err_validation_name_61_chars", `{"kind": "visual", "name": "`+strings.Repeat("n", 61)+`"}`, base)
	a("err_not_found_above", `{"kind": "visual", "above": "trk_zzzzzzzzzz"}`, base)
	a("err_id_exists", `{"kind": "visual", "id": "`+vidTr+`"}`, base)
	a("err_limit_50_tracks", `{"kind": "visual", "id": "trk_k000000001"}`, func() *B { return tracksUpTo(50) })

	// track.update
	u := func(name, args string, b func() *B) { gc("track.update", name, b, op("track.update", args)) }
	hidden := func() *B {
		return base().with(func(d *timeline.Document) { t := d.Tracks[vidTr]; t.Hidden = true; d.Tracks[vidTr] = t })
	}
	u("ok_rename", `{"track_id": "`+vidTr+`", "name": "Main"}`, base)
	u("ok_clear_name", `{"track_id": "`+vidTr+`", "name": ""}`, base)
	u("ok_hide_visual", `{"track_id": "`+vidTr+`", "hidden": true}`, base)
	u("ok_unhide_removes_field", `{"track_id": "`+vidTr+`", "hidden": false}`, hidden)
	u("ok_mute_audio_track", `{"track_id": "`+audTr+`", "muted": true}`, base)
	u("ok_mute_visual_track", `{"track_id": "`+vidTr+`", "muted": true}`, base)
	u("ok_same_values_noop", `{"track_id": "`+vidTr+`", "hidden": false, "name": "Video"}`, base)
	u("ok_name_60_chars", `{"track_id": "`+vidTr+`", "name": "`+strings.Repeat("n", 60)+`"}`, base)
	locked := func() *B {
		return base().with(func(d *timeline.Document) { t := d.Tracks[vidTr]; t.Locked = true; d.Tracks[vidTr] = t })
	}
	u("ok_lock", `{"track_id": "`+vidTr+`", "locked": true}`, base)
	u("ok_unlock_removes_field", `{"track_id": "`+vidTr+`", "locked": false}`, locked)
	u("ok_solo_audio_track", `{"track_id": "`+audTr+`", "solo": true}`, base)
	u("ok_volume", `{"track_id": "`+audTr+`", "volume": 0.5}`, base)
	u("ok_volume_1_removes_field", `{"track_id": "`+audTr+`", "volume": 1}`, func() *B {
		return base().with(func(d *timeline.Document) { t := d.Tracks[audTr]; v := 0.5; t.Volume = &v; d.Tracks[audTr] = t })
	})
	u("ok_edit_a_locked_track_itself", `{"track_id": "`+vidTr+`", "name": "Main"}`, locked)
	u("err_validation_volume_above_4", `{"track_id": "`+audTr+`", "volume": 4.5}`, base)
	u("err_validation_no_field", `{"track_id": "`+vidTr+`"}`, base)
	u("err_validation_name_61_chars", `{"track_id": "`+vidTr+`", "name": "`+strings.Repeat("n", 61)+`"}`, base)
	u("err_not_found", `{"track_id": "trk_zzzzzzzzzz", "name": "x"}`, base)
	u("err_type_mismatch_hidden_on_audio", `{"track_id": "`+audTr+`", "hidden": true}`, base)

	// track.move
	m := func(name, args string, b func() *B) { gc("track.move", name, b, op("track.move", args)) }
	m("ok_to_top", `{"track_id": "`+audTr+`", "position": "top"}`, base)
	m("ok_to_bottom", `{"track_id": "`+capTr+`", "position": "bottom"}`, base)
	m("ok_above", `{"track_id": "`+audTr+`", "above": "`+vidTr+`"}`, base)
	m("ok_below", `{"track_id": "`+audTr+`", "below": "`+capTr+`"}`, base)
	m("ok_already_in_place_noop", `{"track_id": "`+capTr+`", "position": "top"}`, base)
	m("err_validation_no_placement", `{"track_id": "`+vidTr+`"}`, base)
	m("err_validation_two_placements", `{"track_id": "`+vidTr+`", "position": "top", "below": "`+capTr+`"}`, base)
	m("err_not_found_track", `{"track_id": "trk_zzzzzzzzzz", "position": "top"}`, base)
	m("err_not_found_reference", `{"track_id": "`+vidTr+`", "above": "trk_zzzzzzzzzz"}`, base)
	m("err_invalid_state_relative_to_itself", `{"track_id": "`+vidTr+`", "above": "`+vidTr+`"}`, base)

	// track.remove
	rm := func(name, args string, b func() *B) { gc("track.remove", name, b, op("track.remove", args)) }
	withItems := func() *B {
		return base().text(itemA, vidTr, 0, 60).text(itemB, vidTr, 100, 30)
	}
	rm("ok_empty_track", `{"track_id": "`+audTr+`"}`, base)
	rm("ok_with_items_removes_items", `{"track_id": "`+vidTr+`", "with_items": true}`, withItems)
	rm("ok_with_items_false_explicit_on_empty_track", `{"track_id": "`+audTr+`", "with_items": false}`, base)
	rm("ok_dangling_linked_item_left_alone", `{"track_id": "`+vidTr+`", "with_items": true}`, func() *B {
		return base().clip(itemA, vidTr, 0, 150, 0).
			caption(itemC, 0, 3, func(it *timeline.Item) { it.LinkedItem = itemA })
	})
	rm("err_not_found", `{"track_id": "trk_zzzzzzzzzz"}`, base)
	rm("err_not_empty", `{"track_id": "`+vidTr+`"}`, withItems)

	// track.set_caption_style
	cs := func(name, args string, b func() *B) {
		gc("track.set_caption_style", name, b, op("track.set_caption_style", args))
	}
	cs(
		"ok_merge_into_existing",
		`{"track_id": "`+capTr+`", "style": {"size": 72, "position": "middle"}}`,
		styled(`{"highlight": "none"}`),
	)
	cs("ok_merge_into_none", `{"track_id": "`+capTr+`", "style": {"size": 72}}`, base)
	cs(
		"ok_replace",
		`{"track_id": "`+capTr+`", "style": {"size": 64}, "replace": true}`,
		styled(`{"highlight": "none", "color": "#FFFFFF"}`),
	)
	cs(
		"ok_null_value_removes_key",
		`{"track_id": "`+capTr+`", "style": {"size": null}}`,
		styled(`{"size": 72, "color": "#FFFFFF"}`),
	)
	cs("ok_key_equal_to_default_is_kept", `{"track_id": "`+capTr+`", "style": {"size": 64, "align": "center"}}`, base)
	cs(
		"ok_replace_with_empty_style_removes_caption_style",
		`{"track_id": "`+capTr+`", "style": {}, "replace": true}`,
		styled(`{"size": 72}`),
	)
	cs(
		"ok_template_color_reference",
		`{"track_id": "`+capTr+`", "style": {"color": "$accent"}}`,
		func() *B { return base().template() },
	)
	cs("ok_same_style_noop", `{"track_id": "`+capTr+`", "style": {"size": 72}}`, styled(`{"size": 72}`))
	cs("err_validation_unknown_key", `{"track_id": "`+capTr+`", "style": {"zzz": 1}}`, base)
	cs("err_validation_size_below_8", `{"track_id": "`+capTr+`", "style": {"size": 4}}`, base)
	cs("err_validation_highlight_word_in_prototype", `{"track_id": "`+capTr+`", "style": {"highlight": "word"}}`, base)
	cs("err_validation_unknown_color_reference", `{"track_id": "`+capTr+`", "style": {"color": "$brand"}}`, base)
	cs("err_not_found_track", `{"track_id": "trk_zzzzzzzzzz", "style": {"size": 72}}`, base)
	cs("err_type_mismatch_audio_track", `{"track_id": "`+audTr+`", "style": {"size": 72}}`, base)
	cs("err_font_unknown_family", `{"track_id": "`+capTr+`", "style": {"font_family": "Comic"}}`, base)
	cs(
		"err_font_unknown_weight",
		`{"track_id": "`+capTr+`", "style": {"font_family": "JetBrains Mono", "font_weight": 900}}`,
		base,
	)
}
