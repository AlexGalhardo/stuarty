package command

import "github.com/AlexGalhardo/stuarty/core/timeline"

// Golden cases of audio.apply_voice_preset (SDD-002 C.91).

func init() {
	vp := func(name, args string, b func() *B) {
		gc("audio.apply_voice_preset", name, b, op("audio.apply_voice_preset", args))
	}

	clip := func() *B { return base().clip(itemA, vidTr, 0, 150, 0) }

	vp("ok_studio_on_clip", target(itemA, `"preset": "studio"`), clip)
	vp("ok_default_preset_is_studio", target(itemA, ""), clip)
	vp("ok_broadcast_on_track", target(audTr, `"preset": "broadcast"`), onTrack())
	vp("ok_room_rescue_on_audio_item", target(itemB, `"preset": "room_rescue"`), onAudio())
	vp("ok_replaces_the_chain", target(itemB, `"preset": "studio"`), onAudio(withChain(chainOf3()...)))
	vp("ok_same_preset_noop", target(itemB, `"preset": "studio"`),
		onAudio(withChain(timeline.VoicePresetChain("studio")...)))
	vp("err_validation_unknown_preset", target(itemB, `"preset": "radio"`), onAudio())
	vp("err_validation_no_target", `{"preset": "studio"}`, onAudio())
	vp("err_type_mismatch_text_item", target(itemC, `"preset": "studio"`),
		func() *B { return base().text(itemC, vidTr, 0, 60) })
	vp("err_not_found_track", target("trk_zzzzzzzzzz", `"preset": "studio"`), onAudio())
}
