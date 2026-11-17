package command

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/AlexGalhardo/vukem-video/core/timeline"
)

// Golden cases of audio.duck and audio.mark_beats (SDD-002 C.51, C.52) and the
// marker rules M1 to M4 of item.split, item.trim, clip.set_speed and settings.set_fps.

const musicM = itemB

func marks(frames ...int) []timeline.Marker {
	out := make([]timeline.Marker, len(frames))
	for i, f := range frames {
		out[i] = timeline.Marker{Frame: f}
	}
	return out
}

func rangesJSON(pairs ...[2]int) string {
	parts := make([]string, len(pairs))
	for i, p := range pairs {
		parts[i] = fmt.Sprintf(`{"start": %d, "end": %d}`, p[0], p[1])
	}
	return "[" + strings.Join(parts, ", ") + "]"
}

func init() {
	duckCases()
	beatCases()
	markerRuleCases()
}

func duckCases() {
	dk := func(name, args string, b func() *B) { gc("audio.duck", name, b, op("audio.duck", args)) }
	// The speech is the clip itemA: 300 frames from frame 0 of the video asset; the music is
	// itemB, 300 frames on the audio track.
	scene := func(mods ...func(*timeline.Item)) func() *B {
		return func() *B {
			return base().clip(itemA, vidTr, 0, 300, 0).audio(musicM, 0, 300, 0, mods...)
		}
	}
	da := func(rest string) string {
		return strings.TrimSuffix(
			fmt.Sprintf(`{"item_id": %q, "speech_item_ids": [%q], %s`, musicM, itemA, rest),
			", ",
		) + "}"
	}
	speech := func(pairs ...[2]int) string {
		return fmt.Sprintf(`"resolved": {"speech": [{"item_id": %q, "ranges": %s}]}`, itemA, rangesJSON(pairs...))
	}

	dk("ok_one_stretch_defaults", da(speech([2]int{60, 120})), scene())
	dk(
		"ok_gain_attack_release_given",
		da(`"gain_db": -20, "attack": "0.5s", "release": "1s", `+speech([2]int{90, 150})),
		scene(),
	)
	dk("ok_zero_attack_and_release", da(`"attack": "0f", "release": "0f", `+speech([2]int{60, 120})), scene())
	dk("ok_two_stretches_apart", da(speech([2]int{30, 60}, [2]int{200, 240})), scene())
	dk("ok_close_stretches_held_low", da(speech([2]int{60, 90}, [2]int{100, 130})), scene())
	dk("ok_speech_at_start", da(speech([2]int{0, 40})), scene())
	dk("ok_speech_to_the_end", da(speech([2]int{250, 300})), scene())
	dk("ok_music_volume_scaled", da(speech([2]int{60, 120})), scene(func(it *timeline.Item) { it.Volume = 0.5 }))
	dk("ok_no_speech_leaves_document", da(speech()), scene())
	dk("ok_speech_outside_the_music_ignored", da(speech([2]int{60, 120})), func() *B {
		return base().clip(itemA, vidTr, 400, 300, 0).audio(musicM, 0, 300, 0)
	})
	dk("ok_speech_item_trimmed_and_sped_up", da(speech([2]int{10, 50}, [2]int{80, 200})), func() *B {
		return base().clip(itemA, vidTr, 100, 60, 40, func(it *timeline.Item) {
			it.Speed = timeline.Rational{Num: 2, Den: 1}
		}).audio(musicM, 0, 300, 0)
	})
	dk("ok_two_speech_items", fmt.Sprintf(
		`{"item_id": %q, "speech_item_ids": [%q, %q], "resolved": {"speech": [`+
			`{"item_id": %q, "ranges": %s}, {"item_id": %q, "ranges": %s}]}}`,
		musicM, itemC, itemA, itemA, rangesJSON([2]int{20, 50}), itemC, rangesJSON([2]int{0, 30})), func() *B {
		return base().clip(itemA, vidTr, 0, 100, 0).clip(itemC, vidTr, 150, 100, 0).audio(musicM, 0, 300, 0)
	})
	dk("ok_clip_with_audio_as_music_source", da(speech([2]int{60, 120})), func() *B {
		return base().clip(itemA, vidTr, 0, 300, 0).audio(musicM, 0, 300, 0, func(it *timeline.Item) {
			it.Asset = vidAsset
		})
	})

	dk("err_validation_resolved_missing", da(""), scene())
	dk("err_validation_speech_entry_missing", da(`"resolved": {"speech": []}`), scene())
	dk("err_validation_speech_entry_for_other_item", da(
		fmt.Sprintf(`"resolved": {"speech": [{"item_id": %q, "ranges": []}]}`, itemC)), scene())
	dk("err_validation_ranges_overlap", da(speech([2]int{60, 120}, [2]int{100, 140})), scene())
	dk("err_validation_gain_not_below_0", da(`"gain_db": 0, `+speech([2]int{60, 120})), scene())
	dk("err_validation_speech_is_the_music", fmt.Sprintf(
		`{"item_id": %q, "speech_item_ids": [%q], "resolved": {"speech": [{"item_id": %q, "ranges": []}]}}`,
		musicM, musicM, musicM), scene())
	dk("err_invalid_time_attack", da(`"attack": "later", `+speech([2]int{60, 120})), scene())
	dk(
		"err_not_found",
		`{"item_id": "itm_zzzzzzzzzz", "speech_item_ids": [`+`"`+itemA+`"], `+speech([2]int{60, 120})+`}`,
		scene(),
	)
	dk("err_not_found_speech_item", da(fmt.Sprintf(`"resolved": {"speech": [{"item_id": %q, "ranges": []}]}`, itemA)),
		func() *B { return base().audio(musicM, 0, 300, 0) })
	dk("err_type_mismatch_music_is_a_clip", fmt.Sprintf(
		`{"item_id": %q, "speech_item_ids": [%q], %s}`, itemA, itemC,
		fmt.Sprintf(`"resolved": {"speech": [{"item_id": %q, "ranges": []}]}`, itemC)), func() *B {
		return base().clip(itemA, vidTr, 0, 300, 0).audio(itemC, 0, 300, 0)
	})
	dk("err_type_mismatch_speech_is_a_text", da(speech()), func() *B {
		return base().text(itemA, vidTr, 0, 300).audio(musicM, 0, 300, 0)
	})
	dk("err_type_mismatch_speech_asset_without_audio", da(speech()), func() *B {
		return base().clip(itemA, vidTr, 0, 300, 0, func(it *timeline.Item) { it.Asset = imgAsset }).
			audio(musicM, 0, 300, 0)
	})
	dk("err_invalid_state_music_already_animated", da(speech([2]int{60, 120})),
		scene(func(it *timeline.Item) {
			it.Keyframes = timeline.Keyframes{"volume": {kf(0, 1), kf(100, 0.5)}}
		}))
	dk("err_limit_over_100_keyframes", da(`"attack": "0f", "release": "0f", `+speech(manyRanges(40)...)),
		func() *B {
			return base().clip(itemA, vidTr, 0, 900, 0).audio(musicM, 0, 900, 0)
		})
}

// manyRanges returns n stretches of 4 frames, 20 frames apart.
func manyRanges(n int) [][2]int {
	out := make([][2]int, n)
	for i := range n {
		out[i] = [2]int{i * 20, i*20 + 4}
	}
	return out
}

func beatCases() {
	mb := func(name, args string, b func() *B) { gc("audio.mark_beats", name, b, op("audio.mark_beats", args)) }
	// Music at frame 30 for 120 frames, from asset frame 60.
	scene := func(mods ...func(*timeline.Item)) func() *B {
		return func() *B { return base().audio(musicM, 30, 120, 60, mods...) }
	}
	ma := func(rest string) string { return fmt.Sprintf(`{"item_id": %q, %s}`, musicM, rest) }
	beats := func(frames ...int) string {
		parts := make([]string, len(frames))
		for i, f := range frames {
			parts[i] = strconv.Itoa(f)
		}
		return `"resolved": {"beats": [` + strings.Join(parts, ", ") + `]}`
	}
	grid := beats(0, 15, 30, 45, 60, 75, 90, 105, 120, 135, 150, 165, 180, 195, 210)

	mb("ok_every_beat_of_the_visible_part", ma(grid), scene())
	mb("ok_every_second_beat", ma(`"every": 2, `+grid), scene())
	mb("ok_label_given", ma(`"label": "beat", `+grid), scene())
	mb("ok_replaces_existing_markers", ma(grid), scene(func(it *timeline.Item) { it.Markers = marks(3, 100) }))
	mb("ok_keeps_existing_markers", ma(`"replace": false, `+grid), scene(func(it *timeline.Item) {
		it.Markers = []timeline.Marker{{Frame: 3, Label: "intro"}, {Frame: 15, Label: "kept"}}
	}))
	mb("ok_speed_2_beats_on_the_same_frame_once", ma(beats(60, 61, 62, 63, 70)), scene(func(it *timeline.Item) {
		it.Speed = timeline.Rational{Num: 2, Den: 1}
	}))
	mb("ok_beat_on_the_last_visible_frame", ma(beats(60, 179)), scene())
	mb("ok_video_asset_audio", ma(grid), scene(func(it *timeline.Item) { it.Asset = vidAsset }))

	mb("ok_manual_tempo_with_offset", ma(`"bpm": 120, "offset": "0.25s"`), scene())
	mb("ok_manual_tempo_when_the_grid_is_empty", ma(`"bpm": 90, `+beats()), scene())

	mb("err_validation_resolved_missing", ma(`"every": 2`), scene())
	mb("err_validation_beats_not_increasing", ma(beats(60, 90, 90)), scene())
	mb("err_validation_beat_negative", ma(beats(-1, 90)), scene())
	mb("err_validation_offset_without_bpm", ma(`"offset": "1s", `+grid), scene())
	mb("err_validation_bpm_10", ma(`"bpm": 10`), scene())
	mb("err_validation_every_0", ma(`"every": 0, `+grid), scene())
	mb("err_validation_every_65", ma(`"every": 65, `+grid), scene())
	mb("err_validation_label_over_60_characters", ma(`"label": "`+strings.Repeat("a", 61)+`", `+grid), scene())
	mb("err_not_found", `{"item_id": "itm_zzzzzzzzzz", `+grid+`}`, scene())
	mb("err_type_mismatch_clip", fmt.Sprintf(`{"item_id": %q, %s}`, itemA, grid),
		func() *B { return base().clip(itemA, vidTr, 0, 120, 0) })
	mb("err_type_mismatch_asset_without_audio", ma(grid), scene(func(it *timeline.Item) { it.Asset = imgAsset }))
	mb("err_invalid_state_empty_beat_grid", ma(beats()), scene())
	mb("err_invalid_state_no_beat_in_the_visible_part", ma(beats(0, 20, 200, 260)), scene())
	mb(
		"err_limit_over_200_markers",
		ma(`"replace": false, `+beats(consecutive(180, 279)...)),
		scene(func(it *timeline.Item) {
			it.Markers = marks(consecutive(0, 120)[:120]...)
			it.Duration = 400
		}),
	)
}

// consecutive returns the frames from..to inclusive.
func consecutive(from, to int) []int {
	out := make([]int, 0, to-from+1)
	for f := from; f <= to; f++ {
		out = append(out, f)
	}
	return out
}

func markerRuleCases() {
	withMarks := func(frames ...int) func(*timeline.Item) {
		return func(it *timeline.Item) { it.Markers = marks(frames...) }
	}
	music := func(mods ...func(*timeline.Item)) func() *B {
		return func() *B { return base().audio(itemA, 60, 120, 30, mods...) }
	}
	gc("item.split", "ok_markers_divided_at_the_cut", music(withMarks(0, 20, 59, 60, 119)),
		`{"op": "item.split", "args": {"item_id": "`+itemA+`", "at": "120f", "new_id": "`+itemC+`"}}`)
	gc("item.trim", "ok_start_edge_shrink_drops_and_shifts_markers", music(withMarks(0, 20, 59, 60, 119)),
		`{"op": "item.trim", "args": {"item_id": "`+itemA+`", "edge": "start", "to": "100f"}}`)
	gc("item.trim", "ok_start_edge_extend_shifts_markers", music(withMarks(0, 20)),
		`{"op": "item.trim", "args": {"item_id": "`+itemA+`", "edge": "start", "to": "50f"}}`)
	gc("item.trim", "ok_end_edge_shrink_drops_markers_past_the_end", music(withMarks(0, 20, 59, 60, 119)),
		`{"op": "item.trim", "args": {"item_id": "`+itemA+`", "edge": "end", "to": "120f"}}`)
	gc("clip.set_speed", "ok_markers_rescaled_and_collisions_dropped", func() *B {
		return base().clip(itemA, vidTr, 30, 90, 0, withMarks(0, 1, 45, 89))
	}, `{"op": "clip.set_speed", "args": {"item_id": "`+itemA+`", "speed": "4x"}}`)
	gc("settings.set_fps", "ok_markers_rescaled", func() *B {
		return base().audio(itemA, 0, 60, 0, withMarks(0, 30, 59))
	}, `{"op": "settings.set_fps", "args": {"fps": "24"}}`)
}
