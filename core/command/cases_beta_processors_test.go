package command

import (
	"fmt"
	"strings"

	"github.com/AlexGalhardo/stuarty/core/timeline"
)

// Golden cases of audio.processor_add, audio.processor_set, audio.processor_move, audio.processor_remove and
// audio.apply_preset (SDD-002 C.78 to C.82).

func init() {
	processorAddCases()
	processorSetCases()
	processorMoveCases()
	processorRemoveCases()
	processorPresetCases()
}

func proc(kind string, kv ...any) timeline.Processor {
	p := timeline.Processor{Kind: kind}
	for i := 0; i < len(kv); i += 2 {
		if p.Params == nil {
			p.Params = map[string]float64{}
		}
		switch v := kv[i+1].(type) {
		case int:
			p.Params[kv[i].(string)] = float64(v)
		case float64:
			p.Params[kv[i].(string)] = v
		}
	}
	return p
}

func withChain(chain ...timeline.Processor) func(*timeline.Item) {
	return func(it *timeline.Item) { it.Processors = chain }
}

func trackChain(chain ...timeline.Processor) func(d *timeline.Document) {
	return func(d *timeline.Document) {
		t := d.Tracks[audTr]
		t.Processors = chain
		d.Tracks[audTr] = t
	}
}

func chainOf3() []timeline.Processor {
	return []timeline.Processor{
		proc(timeline.ProcGate, "threshold_db", -45),
		proc(timeline.ProcCompressor, "ratio", 4),
		proc(timeline.ProcLimiter),
	}
}

func onClip(mods ...func(*timeline.Item)) func() *B {
	return func() *B { return base().clip(itemA, vidTr, 0, 150, 0, mods...) }
}

func onAudio(mods ...func(*timeline.Item)) func() *B {
	return func() *B { return base().audio(itemB, 0, 90, 0, mods...) }
}

func onTrack(chain ...timeline.Processor) func() *B {
	return func() *B { return base().with(trackChain(chain...)) }
}

func target(id string, rest string) string {
	s := fmt.Sprintf(`{"item_id": %q`, id)
	if strings.HasPrefix(id, "trk_") {
		s = fmt.Sprintf(`{"track_id": %q`, id)
	}
	if rest == "" {
		return s + "}"
	}
	return s + ", " + rest + "}"
}

func processorAddCases() {
	add := func(name, args string, b func() *B) {
		gc("audio.processor_add", name, b, op("audio.processor_add", args))
	}
	add(
		"ok_add_compressor_to_clip",
		target(itemA, `"kind": "compressor", "params": {"threshold_db": -24, "ratio": 4}`),
		onClip(),
	)
	add("ok_add_eq_with_params_to_audio_item",
		target(itemB, `"kind": "eq", "params": {"hp_hz": 80, "b2_hz": 250, "b2_gain_db": -3}`), onAudio())
	add("ok_add_to_track", target(audTr, `"kind": "limiter", "params": {"ceiling_db": -2}`), onTrack())
	add("ok_add_at_index", target(itemB, `"kind": "eq", "index": 1`), onAudio(withChain(chainOf3()...)))
	add("ok_add_disabled", target(itemB, `"kind": "deesser", "enabled": false`), onAudio())
	add("ok_default_values_are_not_stored",
		target(itemB, `"kind": "compressor", "params": {"ratio": 3, "attack_ms": 5}`), onAudio())
	add(
		"ok_add_noise_reduction_marked_profile",
		target(
			itemB,
			`"kind": "noise_reduction", "params": {"strength": 0.7, "profile_start_ms": 100, "profile_end_ms": 600}`,
		),
		onAudio(),
	)
	add("err_validation_no_target", `{"kind": "limiter"}`, onAudio())
	add(
		"err_validation_both_targets",
		fmt.Sprintf(`{"item_id": %q, "track_id": %q, "kind": "limiter"}`, itemB, audTr),
		onAudio(),
	)
	add("err_validation_unknown_kind", target(itemB, `"kind": "reverb"`), onAudio())
	add("err_validation_unknown_param", target(itemB, `"kind": "gate", "params": {"ratio": 2}`), onAudio())
	add("err_validation_param_out_of_range", target(itemB, `"kind": "gate", "params": {"threshold_db": 5}`), onAudio())
	add(
		"err_validation_eq_high_pass_between_0_and_20",
		target(itemB, `"kind": "eq", "params": {"hp_hz": 10}`),
		onAudio(),
	)
	add("err_validation_profile_on_track",
		target(audTr, `"kind": "noise_reduction", "params": {"profile_start_ms": 0, "profile_end_ms": 500}`), onTrack())
	add(
		"err_validation_profile_range_inverted",
		target(
			itemB,
			`"kind": "noise_reduction", "params": {"profile_start_ms": 500, "profile_end_ms": 100}`,
		),
		onAudio(),
	)
	add(
		"err_validation_index_out_of_range",
		target(itemB, `"kind": "limiter", "index": 2`),
		onAudio(withChain(chainOf3()[0])),
	)
	add("err_not_found_item", target("itm_zzzzzzzzzz", `"kind": "limiter"`), onAudio())
	add("err_not_found_track", target("trk_zzzzzzzzzz", `"kind": "limiter"`), onAudio())
	add("err_type_mismatch_text_item", target(itemC, `"kind": "limiter"`),
		func() *B { return base().text(itemC, vidTr, 0, 60) })
	add("err_limit_13th_processor", target(itemB, `"kind": "limiter"`), func() *B {
		full := make([]timeline.Processor, timeline.MaxProcessors)
		for i := range full {
			full[i] = proc(timeline.ProcGate)
		}
		return onAudio(withChain(full...))()
	})
}

func processorSetCases() {
	set := func(name, args string, b func() *B) {
		gc("audio.processor_set", name, b, op("audio.processor_set", args))
	}
	set("ok_set_param", target(itemB, `"index": 1, "params": {"ratio": 6}`), onAudio(withChain(chainOf3()...)))
	set("ok_set_disabled", target(itemB, `"index": 0, "enabled": false`), onAudio(withChain(chainOf3()...)))
	set("ok_set_back_to_default_drops_the_param", target(itemB, `"index": 1, "params": {"ratio": 3}`),
		onAudio(withChain(chainOf3()...)))
	set(
		"ok_set_on_track",
		target(audTr, `"index": 0, "params": {"ceiling_db": -3}`),
		onTrack(proc(timeline.ProcLimiter)),
	)
	set(
		"ok_set_same_value_noop",
		target(itemB, `"index": 1, "params": {"ratio": 4}`),
		onAudio(withChain(chainOf3()...)),
	)
	set("err_validation_nothing_to_change", target(itemB, `"index": 1`), onAudio(withChain(chainOf3()...)))
	set("err_validation_param_not_of_the_kind", target(itemB, `"index": 1, "params": {"ceiling_db": -3}`),
		onAudio(withChain(chainOf3()...)))
	set("err_validation_param_out_of_range", target(itemB, `"index": 1, "params": {"ratio": 40}`),
		onAudio(withChain(chainOf3()...)))
	set(
		"err_validation_index_out_of_range",
		target(itemB, `"index": 3, "enabled": false`),
		onAudio(withChain(chainOf3()...)),
	)
	set("err_not_found_item", target("itm_zzzzzzzzzz", `"index": 0, "enabled": false`), onAudio())
	set("err_type_mismatch_text_item", target(itemC, `"index": 0, "enabled": false`),
		func() *B { return base().text(itemC, vidTr, 0, 60) })
}

func processorMoveCases() {
	mv := func(name, args string, b func() *B) {
		gc("audio.processor_move", name, b, op("audio.processor_move", args))
	}
	mv("ok_move_first_to_last", target(itemB, `"index": 0, "to": 2`), onAudio(withChain(chainOf3()...)))
	mv("ok_move_last_to_first", target(itemB, `"index": 2, "to": 0`), onAudio(withChain(chainOf3()...)))
	mv("ok_move_to_the_same_place_noop", target(itemB, `"index": 1, "to": 1`), onAudio(withChain(chainOf3()...)))
	mv("ok_move_on_track", target(audTr, `"index": 0, "to": 1`), onTrack(chainOf3()[:2]...))
	mv("err_validation_to_out_of_range", target(itemB, `"index": 0, "to": 3`), onAudio(withChain(chainOf3()...)))
	mv("err_validation_index_out_of_range", target(itemB, `"index": 3, "to": 0`), onAudio(withChain(chainOf3()...)))
	mv("err_not_found_item", target("itm_zzzzzzzzzz", `"index": 0, "to": 1`), onAudio())
	mv(
		"err_type_mismatch_text_item",
		target(itemC, `"index": 0, "to": 1`),
		func() *B { return base().text(itemC, vidTr, 0, 60) },
	)
}

func processorRemoveCases() {
	rm := func(name, args string, b func() *B) {
		gc("audio.processor_remove", name, b, op("audio.processor_remove", args))
	}
	rm("ok_remove_middle", target(itemB, `"index": 1`), onAudio(withChain(chainOf3()...)))
	rm("ok_remove_last_leaves_no_field", target(itemB, `"index": 0`), onAudio(withChain(proc(timeline.ProcLimiter))))
	rm("ok_remove_on_track", target(audTr, `"index": 0`), onTrack(chainOf3()...))
	rm("err_validation_index_out_of_range", target(itemB, `"index": 0`), onAudio())
	rm("err_not_found_item", target("itm_zzzzzzzzzz", `"index": 0`), onAudio())
	rm(
		"err_type_mismatch_text_item",
		target(itemC, `"index": 0`),
		func() *B { return base().text(itemC, vidTr, 0, 60) },
	)
}

func processorPresetCases() {
	pr := func(name, args string, b func() *B) {
		gc("audio.apply_preset", name, b, op("audio.apply_preset", args))
	}
	pr("ok_voice_on_clip", target(itemA, `"preset": "voice"`), onClip())
	pr("ok_voice_with_music_on_track", target(audTr, `"preset": "voice_with_music"`), onTrack())
	pr("ok_podcast_on_audio_item", target(itemB, `"preset": "podcast"`), onAudio())
	pr("ok_clean_noisy_room", target(itemB, `"preset": "clean_noisy_room"`), onAudio())
	pr("ok_replaces_the_chain", target(itemB, `"preset": "voice"`), onAudio(withChain(chainOf3()...)))
	pr("ok_same_preset_noop", target(itemB, `"preset": "voice"`), onAudio(withChain(timeline.PresetChain("voice")...)))
	pr("err_validation_unknown_preset", target(itemB, `"preset": "radio"`), onAudio())
	pr("err_validation_no_target", `{"preset": "voice"}`, onAudio())
	pr("err_type_mismatch_text_item", target(itemC, `"preset": "voice"`),
		func() *B { return base().text(itemC, vidTr, 0, 60) })
	pr("err_not_found_track", target("trk_zzzzzzzzzz", `"preset": "voice"`), onAudio())
}
