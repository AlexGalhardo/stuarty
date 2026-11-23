package command

import (
	"fmt"
	"strings"

	"github.com/AlexGalhardo/stuarty/core/timeline"
)

func init() {
	dr := func(name, args string, b func() *B, ids ...string) {
		if len(ids) > 0 {
			gcIDs("transcript.delete_range", name, b, op("transcript.delete_range", args), ids...)
			return
		}
		gc("transcript.delete_range", name, b, op("transcript.delete_range", args))
	}
	words := resolvedJSON([]tword{
		{"hello", 62, 70}, {"um", 72, 80}, {"world", 90, 110}, {"again", 340, 358},
	})
	args := func(from, to int, rest ...string) string {
		head := fmt.Sprintf(`{"asset_id": %q, "from_word": %d, "to_word": %d`, vidAsset, from, to)
		return strings.Join(append(append([]string{head}, rest...), words), ", ") + "}"
	}
	newID := fmt.Sprintf(`"new_ids": [%q]`, itemC)
	// The worked example of C.87: start 30, 300 frames from source frame 60; a text item follows on the track.
	example := func(mods ...func(*timeline.Item)) func() *B {
		return func() *B {
			return base().clip(itemA, vidTr, 30, 300, 60, mods...).text(itemText, vidTr, 330, 40)
		}
	}
	// The sound of the same recording, linked to the clip on the audio track.
	withSound := func(b *B) {
		b.audio(itemB, 30, 300, 60, func(it *timeline.Item) { it.Asset = vidAsset })
	}
	plus := func(b func() *B, mod func(*B)) func() *B {
		return func() *B { x := b(); mod(x); return x }
	}

	dr("ok_middle_word", args(1, 1, newID), example())
	dr("ok_first_words_of_the_clip", args(0, 1), example(), "itm_d5e6f7g8h9")
	dr("ok_last_words_of_the_clip", args(3, 3), example(), itemC)
	dr("ok_whole_clip_deleted", args(0, 3), func() *B {
		return base().clip(itemA, vidTr, 30, 40, 62).text(itemText, vidTr, 70, 40)
	})
	dr("ok_changed_speed", args(1, 1, newID), func() *B {
		return base().clip(itemA, vidTr, 30, 200, 60, func(it *timeline.Item) {
			it.Speed = timeline.Rational{Num: 3, Den: 2}
		}).text(itemText, vidTr, 230, 40)
	})
	dr("ok_linked_audio_cut_with_the_video", args(1, 1, fmt.Sprintf(`"new_ids": [%q, %q]`, itemC, "itm_d5e6f7g8h9")),
		plus(example(), withSound))
	dr("ok_clip_used_twice", args(1, 1, fmt.Sprintf(`"new_ids": [%q, %q]`, itemC, "itm_d5e6f7g8h9")),
		plus(example(), func(b *B) { b.clip(itemB, vidTr, 500, 100, 40) }))
	dr("ok_already_cut_clip", args(2, 2), func() *B {
		return base().clip(itemA, vidTr, 30, 12, 60).clip(itemB, vidTr, 42, 280, 80).text(itemText, vidTr, 322, 40)
	}, itemC)
	dr("ok_captions_follow_the_piece", args(1, 1, newID), plus(example(), func(b *B) {
		b.caption(oldCap, 100, 2, func(it *timeline.Item) { it.LinkedItem = itemA })
		b.caption(capH, 200, 2, func(it *timeline.Item) { it.LinkedItem = itemA })
	}))
	dr("err_validation_range_reversed", args(2, 1), example())
	dr("err_validation_index_beyond_transcript", args(1, 9), example())
	dr(
		"err_validation_resolved_missing",
		fmt.Sprintf(`{"asset_id": %q, "from_word": 0, "to_word": 1}`, vidAsset),
		example(),
	)
	dr("err_not_found_asset", fmt.Sprintf(`{"asset_id": "ast_zzzzzzzzzz", "from_word": 0, "to_word": 1, %s}`, words),
		example())
	dr("err_type_mismatch_no_audio", args(0, 1), plus(example(), func(b *B) {
		a := b.d.Assets[vidAsset]
		a.HasAudio = false
		b.d.Assets[vidAsset] = a
	}))
	dr("err_invalid_state_no_item_shows_the_range", args(0, 0), func() *B {
		return base().clip(itemA, vidTr, 30, 40, 100)
	})
	dr("err_invalid_state_speed_curve", args(1, 1), example(func(it *timeline.Item) {
		it.SpeedCurve = []timeline.SpeedPoint{
			{T: 0, Speed: timeline.Rational{Num: 1, Den: 1}}, {T: 1, Speed: timeline.Rational{Num: 2, Den: 1}},
		}
	}))
	dr("err_invalid_state_track_locked", args(1, 1, newID), plus(example(), func(b *B) {
		tr := b.d.Tracks[vidTr]
		tr.Locked = true
		b.d.Tracks[vidTr] = tr
	}))
	dr("err_id_exists", args(1, 1, fmt.Sprintf(`"new_ids": [%q]`, itemA)), example())
	dr("err_limit_500_items", args(1, 1, newID), plus(example(), func(b *B) { b.manyItems(vidTr, 498) }))
}
