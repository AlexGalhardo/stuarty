package timeline

// VoicePresets are the studio voice chains of audio.apply_voice_preset (SDD-002 C.91), in the order of SDD-000
// section 17. The first is the default.
var VoicePresets = []string{"studio", "broadcast", "room_rescue"}

// VoicePresetChain is the chain of a studio voice preset, or nil for an unknown name. Every call returns a new
// slice. Each chain ends in the limiter, so that the one gain of the export (SDD-004 section 7) brings the voice
// to the loudness target of the project without clipping.
func VoicePresetChain(name string) []Processor {
	type values = map[string]float64

	pr := func(kind string, params values) Processor {
		return Processor{Kind: kind, Params: params}.Normalized()
	}
	deess := pr(ProcDeEsser, nil)
	limit := pr(ProcLimiter, nil)

	switch name {
	case "studio":
		return []Processor{
			pr(ProcNoiseReduction, values{"strength": 0.4}),
			pr(ProcEQ, values{"hp_hz": 80, "b2_hz": 250, "b2_gain_db": -1.5, "b4_hz": 3500, "b4_gain_db": 2}),
			deess,
			pr(ProcCompressor, values{"threshold_db": -20, "ratio": 3, "makeup_db": 3}),
			limit,
		}
	case "broadcast":
		return []Processor{
			pr(ProcNoiseReduction, values{"strength": 0.5}),
			pr(ProcGate, values{"threshold_db": -50, "range_db": -18}),
			pr(ProcEQ, values{"hp_hz": 90, "b2_hz": 200, "b2_gain_db": -2.5, "b4_hz": 4000, "b4_gain_db": 3}),
			deess,
			pr(ProcCompressor, values{"threshold_db": -24, "ratio": 4, "makeup_db": 4}),
			limit,
		}
	case "room_rescue":
		return []Processor{
			pr(ProcNoiseReduction, values{"strength": 0.9}),
			pr(ProcGate, values{"threshold_db": -42, "range_db": -15}),
			pr(ProcEQ, values{"hp_hz": 120, "b2_hz": 300, "b2_gain_db": -3, "b4_hz": 3000, "b4_gain_db": 2}),
			deess,
			pr(ProcCompressor, values{"threshold_db": -20, "ratio": 3, "makeup_db": 3}),
			limit,
		}
	}

	return nil
}
