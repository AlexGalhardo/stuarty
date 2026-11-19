package command

import (
	"fmt"
	"strings"

	"github.com/AlexGalhardo/stuarty/core/timeline"
)

const (
	oldCap = "itm_p7q8r9s0t1"
	capH   = "itm_h2j3k4m5n6"
)

type tword struct {
	text       string
	start, end int
}

// resolvedJSON is the `resolved` argument for words in asset frames.
func resolvedJSON(ws []tword) string {
	parts := make([]string, len(ws))
	for i, w := range ws {
		parts[i] = fmt.Sprintf(`{"text": %q, "start": %d, "end": %d}`, w.text, w.start, w.end)
	}
	return `"resolved": {"words": [` + strings.Join(parts, ", ") + `]}`
}

// transcript is the word list of the example of C.40.
var transcript = []tword{
	{"Intro", 20, 30},
	{"Hello", 62, 70},
	{"everyone,", 72, 86},
	{"welcome", 90, 100},
	{"to", 100, 104},
	{"the", 104, 108},
	{"channel", 110, 128},
	{"today", 170, 182},
}

// speech makes a caption of the given words, linked to the clip itemA.
func speech(c *timeline.Item, words ...timeline.Word) {
	c.Words = words
	last := words[len(words)-1]
	c.Duration = last.Start + last.Duration
	c.LinkedItem = itemA
}

// thirty is 30 words of 2 frames inside the visible range of the example clip.
func thirty() []tword {
	ws := make([]tword, 30)
	for i := range ws {
		ws[i] = tword{fmt.Sprintf("w%d", i), 60 + 3*i, 62 + 3*i}
	}
	return ws
}

func init() {
	ft := func(name, args string, b func() *B, ids ...string) {
		if len(ids) > 0 {
			gcIDs("caption.from_transcript", name, b, op("caption.from_transcript", args), ids...)
			return
		}
		gc("caption.from_transcript", name, b, op("caption.from_transcript", args))
	}
	args := func(rest ...string) string {
		return fmt.Sprintf(`{"clip_item_id": %q, "track_id": %q, %s}`, itemA, capTr, strings.Join(rest, ", "))
	}
	ids3 := fmt.Sprintf(`"new_ids": [%q, %q, %q]`, itemB, itemC, capH)
	ids1 := fmt.Sprintf(`"new_ids": [%q]`, itemB)
	res := resolvedJSON(transcript)
	// The clip and the old caption of the example of C.40.
	example := func(mods ...func(*B)) func() *B {
		return func() *B {
			b := base().clip(itemA, vidTr, 90, 120, 60).caption(oldCap, 92, 1, func(it *timeline.Item) {
				speech(it, timeline.Word{Text: "Old", Start: 0, Duration: 30})
			})
			for _, m := range mods {
				m(b)
			}
			return b
		}
	}
	noOld := func(b *B) { delete(b.d.Items, oldCap) }
	clipAt := func(start, dur, srcIn int, mods ...func(*timeline.Item)) func(*B) {
		return func(b *B) {
			delete(b.d.Items, itemA)
			b.clip(itemA, vidTr, start, dur, srcIn, mods...)
		}
	}
	simple := func(ws ...tword) string { return resolvedJSON(ws) }
	hello := []tword{{"Hello", 62, 70}, {"world", 72, 86}}
	five := []tword{{"a", 62, 66}, {"b", 66, 70}, {"c", 70, 74}, {"d", 74, 78}, {"e", 78, 82}}

	ft("ok_worked_example_replace_true", args(ids3, res), example())
	ft("ok_replace_false_with_free_track", args(`"replace": false`, ids1, simple(hello...)), example(noOld))
	ft("ok_replace_removes_only_captions_linked_to_this_clip_on_this_track", args(ids1, simple(hello...)),
		example(func(b *B) {
			b.caption("itm_k000000001", 500, 2, func(it *timeline.Item) { it.LinkedItem = itemC })
			b.caption("itm_k000000002", 700, 2, func(it *timeline.Item) { it.LinkedItem = itemA })
			other := b.d.Items["itm_k000000002"]
			other.Track = vidTr
			b.d.Items["itm_k000000002"] = other
		}))
	ft("ok_max_words_split", args(`"max_words": 2`, ids3, simple(five...)), example(noOld))
	ft("ok_max_chars_split", args(`"max_chars": 10`, ids3, simple(
		tword{"Hello", 62, 66}, tword{"there", 66, 70}, tword{"general", 70, 74}, tword{"kenobi", 74, 78})),
		example(noOld), "itm_d5e6f7g8h9")
	ft("ok_max_gap_split_strict_greater_than", args(`"max_gap": "18f"`, ids3, simple(
		tword{"A", 62, 70}, tword{"B", 88, 96}, tword{"C", 115, 120})), example(noOld))
	ft("ok_max_gap_0", args(`"max_gap": "0f"`, ids3, simple(
		tword{"A", 62, 70}, tword{"B", 70, 76}, tword{"C", 77, 80})), example(noOld))
	ft("ok_single_word_longer_than_max_chars", args(`"max_chars": 5`, ids3, simple(
		tword{"Hi", 62, 66}, tword{"extraordinarily", 66, 80}, tword{"ok", 80, 84})), example(noOld))
	ft("ok_clip_with_speed_3_2_mapping_uses_ceil", args(ids3, res), example(noOld,
		clipAt(90, 80, 60, func(it *timeline.Item) { it.Speed = timeline.Rational{Num: 3, Den: 2} })))
	ft("ok_word_before_in_point_dropped", args(ids1, simple(tword{"Early", 10, 20}, tword{"Hello", 62, 70})),
		example(noOld))
	ft(
		"ok_word_crossing_out_point_kept_and_capped",
		args(ids1, simple(tword{"Hello", 62, 70}, tword{"today", 170, 190})),
		example(noOld),
		itemC,
	)
	ft("ok_word_starting_at_out_point_dropped", args(ids1, simple(tword{"Hello", 62, 70}, tword{"late", 180, 190})),
		example(noOld))
	ft("ok_clip_not_at_frame_0", args(ids1, simple(hello...)), example(noOld, clipAt(300, 120, 60)))
	ft("ok_new_ids_partial_rest_generated", args(ids1, res), example(), itemC, capH)
	ft("ok_new_ids_extra_ignored_and_reported",
		args(fmt.Sprintf(`"new_ids": [%q, %q, %q, "itm_d5e6f7g8h9"]`, itemB, itemC, capH), res), example())
	ft("ok_replace_true_new_ids_reuse_replaced_ids",
		args(fmt.Sprintf(`"new_ids": [%q, %q, %q]`, oldCap, itemC, capH), res), example())
	ft("err_validation_resolved_missing", args(ids3), example())
	ft("err_validation_words_overlap", args(simple(tword{"a", 62, 80}, tword{"b", 70, 90})), example())
	ft("err_validation_new_ids_duplicate", args(fmt.Sprintf(`"new_ids": [%q, %q]`, itemB, itemB), res), example())
	ft("err_validation_max_words_61", args(`"max_words": 61`, res), example())
	ft("err_invalid_time_max_gap_no_unit", args(`"max_gap": "0.6"`, res), example())
	ft(
		"err_not_found_clip",
		fmt.Sprintf(`{"clip_item_id": "itm_zzzzzzzzzz", "track_id": %q, %s}`, capTr, res),
		example(),
	)
	ft(
		"err_not_found_track",
		fmt.Sprintf(`{"clip_item_id": %q, "track_id": "trk_zzzzzzzzzz", %s}`, itemA, res),
		example(),
	)
	ft("err_type_mismatch_not_a_clip", args(res), func() *B { return base().text(itemA, vidTr, 0, 60) })
	ft("err_type_mismatch_asset_without_audio", args(res), example(func(b *B) {
		a := b.d.Assets[vidAsset]
		a.HasAudio = false
		b.d.Assets[vidAsset] = a
	}))
	ft(
		"err_type_mismatch_audio_track",
		fmt.Sprintf(`{"clip_item_id": %q, "track_id": %q, %s}`, itemA, audTr, res),
		example(),
	)
	ft("err_invalid_state_no_word_in_range", args(simple(tword{"Early", 10, 20}, tword{"Late", 190, 200})), example())
	ft("err_id_exists", args(fmt.Sprintf(`"new_ids": [%q, %q, %q]`, itemA, itemC, capH), res), example())
	ft("err_limit_5000_words", args(simple(thirty()...)), example(func(b *B) {
		for i := range 83 {
			b.caption(fmt.Sprintf("itm_k%09d", i), 1000+i*700, 60)
		}
	}), itemB, itemC, capH, "itm_d5e6f7g8h9", "itm_e6f7g8h9j0", "itm_f7g8h9j0k1")
	ft("err_limit_500_items", args(res), example(func(b *B) { b.manyItems(vidTr, 497) }), itemB, itemC, capH)
	ft("err_overlap_replace_false", args(`"replace": false`, ids3, res), example())
}

func init() {
	rt := func(name, args string, b func() *B) {
		gc("caption.retime_from_clip", name, b, op("caption.retime_from_clip", args))
	}
	args := func(ws ...tword) string {
		return fmt.Sprintf(`{"clip_item_id": %q, %s}`, itemA, resolvedJSON(ws))
	}
	w := func(t string, s, d int) timeline.Word { return timeline.Word{Text: t, Start: s, Duration: d} }
	// The captions of the result of C.40: 4, 2 and 1 words, "welcome" edited to "Welcome".
	built := func(clip func(*B), mods ...func(*B)) func() *B {
		return func() *B {
			b := base()
			clip(b)
			b.caption(itemB, 92, 1, func(it *timeline.Item) {
				speech(it, w("Hello", 0, 8), w("everyone,", 10, 14), w("Welcome", 28, 10), w("to", 38, 4))
			})
			b.caption(itemC, 134, 1, func(it *timeline.Item) { speech(it, w("the", 0, 4), w("channel", 6, 18)) })
			b.caption(capH, 200, 1, func(it *timeline.Item) { speech(it, w("today", 0, 10)) })
			for _, m := range mods {
				m(b)
			}
			return b
		}
	}
	moved := func(start, dur, srcIn int, mods ...func(*timeline.Item)) func(*B) {
		return func(b *B) { b.clip(itemA, vidTr, start, dur, srcIn, mods...) }
	}
	at := func(dur, srcIn int, mods ...func(*timeline.Item)) func(*B) { return moved(90, dur, srcIn, mods...) }
	speed32 := func(it *timeline.Item) { it.Speed = timeline.Rational{Num: 3, Den: 2} }
	edit := func(id string, f func(*timeline.Item)) func(*B) {
		return func(b *B) {
			it := b.d.Items[id]
			f(&it)
			b.d.Items[id] = it
		}
	}
	noAudio := func(b *B) {
		a := b.d.Assets[vidAsset]
		a.HasAudio = false
		b.d.Assets[vidAsset] = a
	}

	rt("ok_worked_example_speed_3_2", args(transcript...), built(at(80, 60, speed32)))
	rt("ok_clip_moved_captions_follow", args(transcript...), built(moved(300, 120, 60)))
	rt("ok_clip_trimmed_at_start_first_words_dropped_edited_texts_kept_when_counts_match", args(transcript...),
		func() *B {
			b := base()
			at(80, 100)(b)
			b.caption(itemB, 92, 1, func(it *timeline.Item) { speech(it, w("To", 0, 4), w("The", 6, 4)) })
			b.caption(itemC, 134, 1, func(it *timeline.Item) { speech(it, w("Channel", 0, 18), w("Today", 30, 10)) })
			return b
		})
	rt("ok_edited_texts_kept_when_counts_match", args(transcript...), built(moved(100, 120, 60)))
	rt("ok_caption_receives_fewer_words_takes_transcript_texts_and_is_listed_in_text_reset", args(transcript...),
		built(at(50, 60)))
	rt("ok_caption_receiving_no_word_removed_and_listed_in_removed", args(transcript...), built(at(40, 60)))
	rt("ok_removed_caption_restored_by_undo", args(transcript...), built(at(60, 60)))
	rt("ok_leftover_transcript_words_counted_not_added", args(transcript...), func() *B {
		b := base()
		at(120, 60)(b)
		b.caption(itemB, 92, 1, func(it *timeline.Item) { speech(it, w("Hello", 0, 8), w("everyone,", 10, 14)) })
		return b
	})
	rt("ok_captions_on_two_tracks_each_start_from_first_word", args(transcript...), built(at(120, 60),
		func(b *B) {
			b.d.Tracks["trk_p7q8r9s0t1"] = timeline.Track{
				ID: "trk_p7q8r9s0t1", Kind: timeline.TrackVisual, Name: "More", Order: "a3",
			}
			b.caption("itm_k000000001", 1000, 1, func(it *timeline.Item) {
				speech(it, w("Hello", 0, 8), w("everyone,", 10, 14))
				it.Track = "trk_p7q8r9s0t1"
			})
		}))
	rt("ok_breaks_beyond_new_word_count_dropped", args(transcript...),
		built(at(40, 60), edit(itemB, func(it *timeline.Item) { it.Breaks = []int{3} })))
	rt("ok_keyframes_cut_when_caption_shorter", args(transcript...),
		built(at(80, 60, speed32), edit(itemB, func(it *timeline.Item) {
			it.Keyframes = timeline.Keyframes{"opacity": {kf(0, 1), kf(40, 0.5)}}
		})))
	rt("ok_unchanged_clip_noop", args(transcript...), func() *B {
		b := base()
		at(120, 60)(b)
		b.caption(itemB, 92, 1, func(it *timeline.Item) {
			speech(it, w("Hello", 0, 8), w("everyone,", 10, 14), w("welcome", 28, 10), w("to", 38, 4))
		})
		b.caption(itemC, 134, 1, func(it *timeline.Item) { speech(it, w("the", 0, 4), w("channel", 6, 18)) })
		b.caption(capH, 200, 1, func(it *timeline.Item) { speech(it, w("today", 0, 10)) })
		return b
	})
	rt("err_validation_resolved_missing", fmt.Sprintf(`{"clip_item_id": %q}`, itemA), built(at(80, 60)))
	rt("err_not_found_clip", `{"clip_item_id": "itm_zzzzzzzzzz", `+resolvedJSON(transcript)+`}`, built(at(80, 60)))
	rt("err_type_mismatch_not_a_clip", args(transcript...), func() *B { return base().text(itemA, vidTr, 0, 60) })
	rt("err_type_mismatch_asset_without_audio", args(transcript...), built(at(80, 60), noAudio))
	rt("err_invalid_state_no_linked_caption", args(transcript...), func() *B {
		b := base()
		at(80, 60)(b)
		return b
	})
	rt("err_invalid_state_no_visible_word", args(tword{"Early", 10, 20}), built(at(80, 60)))
	rt("err_invalid_state_transition_no_longer_fits", args(transcript...),
		built(at(80, 60, speed32), edit(itemB, func(it *timeline.Item) {
			it.TransitionIn = &timeline.Transition{Kind: "fade", Duration: 15}
			it.TransitionOut = &timeline.Transition{Kind: "fade", Duration: 15}
		})))
	rt("err_overlap_with_other_item", args(transcript...), built(moved(300, 120, 60), func(b *B) {
		b.text(oldCap, capTr, 310, 20)
	}))
}
