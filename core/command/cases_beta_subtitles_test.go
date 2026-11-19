package command

// Golden cases of caption.import_subtitles (TASK-0254).

import "fmt"

func init() {
	const srt = "1\n00:00:01,000 --> 00:00:02,500\nHello <i>there</i>\nfriend\n\n2\n00:00:03,000 --> 00:00:04,000\nBye\n"
	ids := []string{"itm_d4d4d4d4d4", "itm_e5e5e5e5e5"}
	plain := base
	arg := func(rest string, text string) string {
		return fmt.Sprintf(`{"track_id": %q, "text": %q%s}`, vidTr, text, rest)
	}
	imp := func(name, rest, text string, b func() *B, newIDs ...string) {
		gcIDs("caption.import_subtitles", name, b, op("caption.import_subtitles", arg(rest, text)), newIDs...)
	}

	imp("ok_srt_two_cues", "", srt, plain, ids...)
	imp(
		"ok_vtt_detected_by_its_header",
		"",
		"WEBVTT\n\nintro\n00:01.000 --> 00:02.500 align:start\n<c.yellow>Hi</c> you\n",
		plain,
		ids[0],
	)
	imp("ok_format_given", `, "format": "srt"`, "1\n00:00:00,000 --> 00:00:01,000\nOne\n", plain, ids[0])
	imp("ok_start_moves_every_cue", `, "start": "2s"`, srt, plain, ids...)
	imp("err_validation_bad_time_line", "", "1\n00:00:01 --> 00:00:02,000\nx\n", plain, ids[0])
	imp("err_validation_no_cue_with_text", "", "1\n00:00:01,000 --> 00:00:02,000\n\n", plain, ids[0])
	gc("caption.import_subtitles", "err_validation_bad_new_id", plain,
		op("caption.import_subtitles", arg(`, "new_ids": ["bad-id"]`, srt)))
	gc("caption.import_subtitles", "err_not_found_track", plain, op("caption.import_subtitles",
		`{"track_id": "trk_zzzzzzzzzz", "text": "1\n00:00:01,000 --> 00:00:02,000\nx\n"}`))
	gc("caption.import_subtitles", "err_type_mismatch_audio_track", plain, op("caption.import_subtitles",
		fmt.Sprintf(`{"track_id": %q, "text": "1\n00:00:01,000 --> 00:00:02,000\nx\n"}`, audTr)))
	imp("err_invalid_time_cue_past_the_end", `, "start": "3599s"`, srt, plain, ids...)
	imp("err_overlap_item_on_the_track", "", srt, func() *B { return base().clip(itemA, vidTr, 0, 90, 0) }, ids...)
	imp("err_id_exists", "", srt, func() *B { return base().clip(itemA, vidTr, 200, 30, 0) }, itemA, ids[1])
	imp("err_invalid_state_cue_shorter_than_its_words", "",
		"1\n00:00:01,000 --> 00:00:01,034\none two three\n", plain, ids[0])
	imp("err_limit_500_items", "", srt, func() *B { return base().manyItems(vidTr, 499) }, ids...)
}
