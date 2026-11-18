package command

// Golden cases of item.link, item.unlink and item.extract_audio, and of the linked edits of
// item.move, item.trim and item.split (TASK-0277).

import (
	"fmt"

	"github.com/AlexGalhardo/vukem-video/core/timeline"
)

const (
	linkA = "lnk_a1b2c3d4e5"
	linkB = "lnk_f6g7h8j9k0"
	itemD = "itm_d4e5f6g7h8"
	itemE = "itm_e5f6g7h8j9"
)

func init() {
	linked := func(l string) func(*timeline.Item) { return func(it *timeline.Item) { it.Link = l } }
	ids2 := func(a, b string) string { return fmt.Sprintf(`{"item_ids": [%q, %q]}`, a, b) }
	// A clip at frame 30 for 90 frames and an audio item with the same range, on their tracks.
	pair := func(mods ...func(*timeline.Item)) func() *B {
		return func() *B {
			return base().clip(itemA, vidTr, 30, 90, 0, mods...).audio(itemB, 30, 90, 0, mods...)
		}
	}

	lk := func(name, args string, b func() *B) { gc("item.link", name, b, op("item.link", args)) }
	gcIDs("item.link", "ok_link_clip_and_audio", pair(), op("item.link", ids2(itemA, itemB)), linkA)
	gcIDs("item.link", "ok_link_three_items", func() *B {
		return base().clip(itemA, vidTr, 30, 90, 0).audio(itemB, 30, 90, 0).text(itemC, vidTr, 200, 20)
	}, op("item.link", fmt.Sprintf(`{"item_ids": [%q, %q, %q]}`, itemC, itemA, itemB)), linkA)
	lk("ok_link_with_given_id", fmt.Sprintf(`{"item_ids": [%q, %q], "id": %q}`, itemA, itemB, linkA), pair())
	gcIDs("item.link", "ok_link_with_generated_id", pair(), op("item.link", ids2(itemA, itemB)), linkA)
	gcIDs("item.link", "ok_link_text_and_shape", func() *B {
		return base().text(itemC, vidTr, 0, 30).shape(itemD, capTr, 0, 30)
	}, op("item.link", ids2(itemC, itemD)), linkA)
	lk("err_validation_one_item", fmt.Sprintf(`{"item_ids": [%q]}`, itemA), pair())
	lk("err_validation_listed_twice", ids2(itemA, itemA), pair())
	lk("err_validation_bad_link_id", fmt.Sprintf(`{"item_ids": [%q, %q], "id": "group1"}`, itemA, itemB), pair())
	lk("err_not_found_item", ids2(itemA, "itm_zzzzzzzzzz"), pair())
	lk("err_type_mismatch_caption", ids2(itemA, itemC), func() *B {
		return base().clip(itemA, vidTr, 0, 90, 0).caption(itemC, 200, 3)
	})
	lk("err_invalid_state_already_linked", ids2(itemA, itemB), func() *B {
		return base().clip(itemA, vidTr, 30, 90, 0, linked(linkB)).audio(itemB, 30, 90, 0)
	})
	lk("err_id_exists_link_in_use", fmt.Sprintf(`{"item_ids": [%q, %q], "id": %q}`, itemA, itemB, linkA), func() *B {
		return base().clip(itemA, vidTr, 30, 90, 0).audio(itemB, 30, 90, 0).
			text(itemC, vidTr, 200, 20, linked(linkA)).text(itemD, vidTr, 300, 20, linked(linkA))
	})

	ul := func(name, args string, b func() *B) { gc("item.unlink", name, b, op("item.unlink", args)) }
	three := func() *B {
		return base().clip(itemA, vidTr, 30, 90, 0, linked(linkA)).audio(itemB, 30, 90, 0, linked(linkA)).
			text(itemC, vidTr, 200, 20, linked(linkA))
	}
	ul("ok_unlink_one_of_three", fmt.Sprintf(`{"item_ids": [%q]}`, itemC), three)
	ul("ok_unlink_two_of_two_dissolves", ids2(itemA, itemB), pair(linked(linkA)))
	ul("ok_unlink_one_of_two_unlinks_the_other", fmt.Sprintf(`{"item_ids": [%q]}`, itemA), pair(linked(linkA)))
	ul("ok_unlink_whole_group", fmt.Sprintf(`{"item_ids": [%q, %q, %q]}`, itemA, itemB, itemC), three)
	ul("err_validation_listed_twice", ids2(itemA, itemA), pair(linked(linkA)))
	ul("err_not_found_item", fmt.Sprintf(`{"item_ids": [%q]}`, "itm_zzzzzzzzzz"), pair(linked(linkA)))
	ul("err_invalid_state_not_linked", fmt.Sprintf(`{"item_ids": [%q]}`, itemA), pair())

	ea := func(name, args string, b func() *B) {
		gc("item.extract_audio", name, b, op("item.extract_audio", args))
	}
	ex := func(rest string) string { return fmt.Sprintf(`{"item_id": %q, "track_id": %q%s}`, itemA, audTr, rest) }
	clipOnly := func(mods ...func(*timeline.Item)) func() *B {
		return func() *B { return base().clip(itemA, vidTr, 30, 90, 10, mods...) }
	}
	ea("ok_extract_audio", ex(fmt.Sprintf(`, "id": %q`, itemB)), clipOnly())
	gcIDs(
		"item.extract_audio",
		"ok_extract_audio_with_generated_id",
		clipOnly(),
		op("item.extract_audio", ex("")),
		itemB,
	)
	ea("ok_copies_audio_settings", ex(fmt.Sprintf(`, "id": %q`, itemB)), clipOnly(func(it *timeline.Item) {
		it.Speed = timeline.Rational{Num: 3, Den: 2}
		it.Volume, it.FadeIn, it.FadeOut, it.Denoise, it.Voice = 0.5, 6, 9, 0.4, "warm"
		it.Keyframes = timeline.Keyframes{"volume": {kf(0, 1), kf(40, 0.2)}, "scale": {kf(0, 1), kf(40, 1.5)}}
	}))
	ea("ok_copies_curve_and_reverse", ex(fmt.Sprintf(`, "id": %q`, itemB)), clipOnly(func(it *timeline.Item) {
		it.SpeedCurve = []timeline.SpeedPoint{
			{T: 0, Speed: timeline.Rational{Num: 1, Den: 1}}, {T: 1, Speed: timeline.Rational{Num: 2, Den: 1}},
		}
		it.Speed, it.Reverse = timeline.CurveMean(it.SpeedCurve), true
	}))
	ea("ok_joins_the_group_of_the_clip", ex(fmt.Sprintf(`, "id": %q`, itemB)), func() *B {
		return base().clip(itemA, vidTr, 30, 90, 10, linked(linkA)).text(itemC, vidTr, 200, 20, linked(linkA))
	})
	ea("err_validation_bad_new_id", ex(`, "id": "audio1"`), clipOnly())
	ea("err_not_found_item", fmt.Sprintf(`{"item_id": "itm_zzzzzzzzzz", "track_id": %q}`, audTr), clipOnly())
	ea("err_not_found_track", fmt.Sprintf(`{"item_id": %q, "track_id": "trk_zzzzzzzzzz"}`, itemA), clipOnly())
	ea("err_type_mismatch_not_a_clip", fmt.Sprintf(`{"item_id": %q, "track_id": %q}`, itemB, audTr),
		func() *B { return base().audio(itemB, 0, 60, 0) })
	ea("err_type_mismatch_visual_track", fmt.Sprintf(`{"item_id": %q, "track_id": %q}`, itemA, capTr), clipOnly())
	ea("err_type_mismatch_image_clip", ex(""), clipOnly(image))
	ea("err_type_mismatch_asset_without_audio", ex(""), func() *B {
		return base().with(func(d *timeline.Document) {
			a := d.Assets[vidAsset]
			a.HasAudio = false
			d.Assets[vidAsset] = a
		}).clip(itemA, vidTr, 30, 90, 10)
	})
	ea("err_invalid_state_already_muted", ex(""), clipOnly(func(it *timeline.Item) { it.AudioMuted = true }))
	ea("err_overlap_audio_track_busy", ex(fmt.Sprintf(`, "id": %q`, itemD)), func() *B {
		return base().clip(itemA, vidTr, 30, 90, 10).audio(itemB, 60, 30, 0)
	})
	gcIDs("item.extract_audio", "err_limit_500_items", func() *B {
		return base().manyItems(capTr, 499).clip(itemA, vidTr, 30, 90, 10)
	}, op("item.extract_audio", ex("")), itemB)
	ea("err_id_exists", ex(fmt.Sprintf(`, "id": %q`, itemA)), clipOnly())

	// The linked edits of the other commands.
	mv := func(name, args string, b func() *B) { gc("item.move", name, b, op("item.move", args)) }
	mvA := func(start string) string { return fmt.Sprintf(`{"item_id": %q, "start": %q}`, itemA, start) }
	mv("ok_linked_items_move_together", mvA("100f"), pair(linked(linkA)))
	mv("ok_linked_items_move_earlier_together", mvA("10f"), pair(linked(linkA)))
	mv("ok_linked_move_to_other_track_moves_only_the_item",
		fmt.Sprintf(`{"item_id": %q, "track_id": %q}`, itemA, capTr), pair(linked(linkA)))
	mv("ok_linked_peer_out_of_the_way_unaffected", mvA("100f"), func() *B {
		return base().clip(itemA, vidTr, 30, 90, 0, linked(linkA)).audio(itemB, 30, 90, 0, linked(linkA)).
			text(itemC, vidTr, 400, 20)
	})
	mv("err_invalid_time_linked_peer_before_frame_0", mvA("0f"), func() *B {
		return base().clip(itemA, vidTr, 30, 90, 0, linked(linkA)).audio(itemB, 10, 90, 0, linked(linkA))
	})
	mv("err_overlap_linked_peer", mvA("100f"), func() *B {
		return base().clip(itemA, vidTr, 30, 90, 0, linked(linkA)).audio(itemB, 30, 90, 0, linked(linkA)).
			audio(itemD, 120, 50, 0)
	})
	mv("err_invalid_state_linked_peer_on_a_locked_track", mvA("100f"), func() *B {
		return pair(linked(linkA))().with(func(d *timeline.Document) {
			t := d.Tracks[audTr]
			t.Locked = true
			d.Tracks[audTr] = t
		})
	})

	tr := func(name, args string, b func() *B) { gc("item.trim", name, b, op("item.trim", args)) }
	trimArgs := func(edge, to string) string {
		return fmt.Sprintf(`{"item_id": %q, "edge": %q, "to": %q}`, itemA, edge, to)
	}
	tr("ok_linked_trim_end_together", trimArgs("end", "100f"), pair(linked(linkA)))
	tr("ok_linked_trim_start_together", trimArgs("start", "50f"), pair(linked(linkA)))
	tr("ok_linked_trim_extend_start_together", trimArgs("start", "20f"), func() *B {
		return base().clip(itemA, vidTr, 30, 90, 20, linked(linkA)).audio(itemB, 30, 90, 20, linked(linkA))
	})
	tr("err_invalid_state_linked_peer_would_have_no_frame", trimArgs("end", "35f"), func() *B {
		return base().clip(itemA, vidTr, 30, 90, 0, linked(linkA)).audio(itemB, 30, 4, 0, linked(linkA))
	})
	tr("err_invalid_state_linked_peer_on_a_locked_track", trimArgs("end", "100f"), func() *B {
		return pair(linked(linkA))().with(func(d *timeline.Document) {
			t := d.Tracks[audTr]
			t.Locked = true
			d.Tracks[audTr] = t
		})
	})

	sp := func(name, args string, b func() *B, generated ...string) {
		gcIDs("item.split", name, b, op("item.split", args), generated...)
	}
	splitAt := func(f string, rest string) string {
		return fmt.Sprintf(`{"item_id": %q, "at": %q, "new_id": %q%s}`, itemA, f, itemC, rest)
	}
	sp("ok_linked_split_together", splitAt("60f", fmt.Sprintf(`, "new_ids": [%q]`, itemD)), pair(linked(linkA)))
	sp("ok_linked_split_with_generated_ids", splitAt("60f", ""), pair(linked(linkA)), itemD)
	sp(
		"ok_linked_split_cuts_only_the_items_that_span_the_frame",
		splitAt("100f", fmt.Sprintf(`, "new_ids": [%q]`, itemE)),
		func() *B {
			return base().clip(itemA, vidTr, 30, 90, 0, linked(linkA)).audio(itemB, 30, 90, 0, linked(linkA)).
				text(itemD, vidTr, 200, 20, linked(linkA))
		},
	)
	sp("ok_linked_split_where_no_peer_spans_leaves_the_right_part_unlinked", splitAt("100f", ""), func() *B {
		return base().clip(itemA, vidTr, 30, 90, 0, linked(linkA)).audio(itemB, 200, 20, 0, linked(linkA))
	})
	sp("err_invalid_state_linked_peer_speed_curve", splitAt("60f", fmt.Sprintf(`, "new_ids": [%q]`, itemD)), func() *B {
		return base().clip(itemA, vidTr, 30, 90, 0, linked(linkA)).audio(itemB, 30, 90, 0, linked(linkA),
			func(it *timeline.Item) {
				it.SpeedCurve = []timeline.SpeedPoint{
					{T: 0, Speed: timeline.Rational{Num: 1, Den: 1}}, {T: 1, Speed: timeline.Rational{Num: 2, Den: 1}},
				}
				it.Speed = timeline.CurveMean(it.SpeedCurve)
			})
	})
	sp("err_invalid_state_linked_peer_on_a_locked_track", splitAt("60f", fmt.Sprintf(`, "new_ids": [%q]`, itemD)),
		func() *B {
			return pair(linked(linkA))().with(func(d *timeline.Document) {
				t := d.Tracks[audTr]
				t.Locked = true
				d.Tracks[audTr] = t
			})
		})
}
