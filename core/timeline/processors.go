package timeline

import (
	"fmt"
	"maps"
	"math"
	"slices"
	"strconv"
)

// The audio processors of a clip, an audio item and a track (SDD-001 section 3.5, SDD-004 section 7.1).
const (
	ProcEQ             = "eq"
	ProcCompressor     = "compressor"
	ProcLimiter        = "limiter"
	ProcGate           = "gate"
	ProcDeEsser        = "deesser"
	ProcNoiseReduction = "noise_reduction"
)

// MaxProcessors is the longest chain of one item or one track.
const MaxProcessors = 12

// MinProfileMS is the shortest marked noise profile: one frame of the spectral method (1,024 samples at 48 kHz,
// rounded up to whole milliseconds).
const MinProfileMS = 22

// ProcessorKinds is the fixed list of processor kinds, in the order of SDD-004 section 7.1.
var ProcessorKinds = []string{ProcEQ, ProcCompressor, ProcLimiter, ProcGate, ProcDeEsser, ProcNoiseReduction}

// Processor is one entry of a processors chain. Params holds only the values that differ from the default of
// their parameter (Canonical form, SDD-001 section 3.5).
type Processor struct {
	Kind     string             `json:"kind"               enum:"eq,compressor,limiter,gate,deesser,noise_reduction"`
	Disabled bool               `json:"disabled,omitempty"`
	Params   map[string]float64 `json:"params,omitempty"`
}

// ParamSpec is one parameter of a processor kind: its name, unit, range and default. A parameter with OffZero is
// 0 (off) or at least MinOn.
type ParamSpec struct {
	Name    string  `json:"name"`
	Unit    string  `json:"unit"`
	Min     float64 `json:"min"`
	Max     float64 `json:"max"`
	Default float64 `json:"default"`
	OffZero bool    `json:"off_zero,omitempty"`
	MinOn   float64 `json:"min_on,omitempty"`
}

var bandDefaults = [6]float64{100, 250, 1000, 3000, 6000, 10000}

var processorParams = map[string][]ParamSpec{
	ProcEQ: eqParams(),
	ProcCompressor: {
		{Name: "threshold_db", Unit: "dBFS", Min: -60, Max: 0, Default: -18},
		{Name: "ratio", Unit: "ratio", Min: 1, Max: 20, Default: 3},
		{Name: "attack_ms", Unit: "ms", Min: 0.1, Max: 200, Default: 10},
		{Name: "release_ms", Unit: "ms", Min: 5, Max: 2000, Default: 100},
		{Name: "knee_db", Unit: "dB", Min: 0, Max: 24, Default: 6},
		{Name: "makeup_db", Unit: "dB", Min: -12, Max: 24, Default: 0},
	},
	ProcLimiter: {
		{Name: "ceiling_db", Unit: "dBFS", Min: -20, Max: 0, Default: -1},
		{Name: "release_ms", Unit: "ms", Min: 1, Max: 1000, Default: 50},
	},
	ProcGate: {
		{Name: "threshold_db", Unit: "dBFS", Min: -80, Max: 0, Default: -50},
		{Name: "range_db", Unit: "dB", Min: -80, Max: 0, Default: -40},
		{Name: "attack_ms", Unit: "ms", Min: 0.1, Max: 100, Default: 2},
		{Name: "hold_ms", Unit: "ms", Min: 0, Max: 1000, Default: 50},
		{Name: "release_ms", Unit: "ms", Min: 5, Max: 2000, Default: 150},
	},
	ProcDeEsser: {
		{Name: "frequency_hz", Unit: "Hz", Min: 2000, Max: 12000, Default: 6500},
		{Name: "threshold_db", Unit: "dBFS", Min: -60, Max: 0, Default: -30},
		{Name: "ratio", Unit: "ratio", Min: 1, Max: 20, Default: 4},
	},
	ProcNoiseReduction: {
		{Name: "strength", Unit: "fraction", Min: 0, Max: 1, Default: 0.5},
		{Name: "profile_start_ms", Unit: "ms", Min: 0, Max: 3600000, Default: 0},
		{Name: "profile_end_ms", Unit: "ms", Min: 0, Max: 3600000, Default: 0},
	},
}

func eqParams() []ParamSpec {
	out := []ParamSpec{
		{Name: "hp_hz", Unit: "Hz", Min: 0, Max: 1000, Default: 0, OffZero: true, MinOn: 20},
		{Name: "lp_hz", Unit: "Hz", Min: 0, Max: 20000, Default: 0, OffZero: true, MinOn: 1000},
	}
	for n := 1; n <= 6; n++ {
		p := "b" + strconv.Itoa(n)
		out = append(out,
			ParamSpec{Name: p + "_hz", Unit: "Hz", Min: 20, Max: 20000, Default: bandDefaults[n-1]},
			ParamSpec{Name: p + "_gain_db", Unit: "dB", Min: -24, Max: 24, Default: 0},
			ParamSpec{Name: p + "_q", Unit: "Q", Min: 0.1, Max: 18, Default: 1},
		)
	}
	return out
}

// ProcessorParams lists the parameters of a kind, or nil for an unknown kind.
func ProcessorParams(kind string) []ParamSpec { return slices.Clone(processorParams[kind]) }

// ValidProcessorKind reports whether kind is one of ProcessorKinds.
func ValidProcessorKind(kind string) bool { return slices.Contains(ProcessorKinds, kind) }

func paramSpec(kind, name string) (ParamSpec, bool) {
	for _, s := range processorParams[kind] {
		if s.Name == name {
			return s, true
		}
	}
	return ParamSpec{}, false
}

// Check returns why value is not allowed for the parameter, or "".
func (s ParamSpec) Check(value float64) string {
	if math.IsNaN(value) || math.IsInf(value, 0) {
		return s.Name + " must be a number"
	}
	if s.OffZero && value == 0 {
		return ""
	}
	lo := s.Min
	if s.OffZero {
		lo = s.MinOn
	}
	if value < lo || value > s.Max {
		if s.OffZero {
			return fmt.Sprintf("%s %s is out of range: 0 (off) or %s to %s %s",
				s.Name, num(value), num(lo), num(s.Max), s.Unit)
		}

		return fmt.Sprintf("%s %s is out of range: %s to %s %s", s.Name, num(value), num(lo), num(s.Max), s.Unit)
	}
	return ""
}

func num(v float64) string { return strconv.FormatFloat(v, 'f', -1, 64) }

// CheckProcessor returns the first problem of a processor as a message, or "": an unknown kind or parameter, a
// value out of its range, or a noise profile that is not both 0 or a range of at least MinProfileMS.
func CheckProcessor(p Processor) string {
	if !ValidProcessorKind(p.Kind) {
		return fmt.Sprintf("kind %q is not one of eq, compressor, limiter, gate, deesser or noise_reduction", p.Kind)
	}
	for _, name := range slices.Sorted(maps.Keys(p.Params)) {
		spec, ok := paramSpec(p.Kind, name)
		if !ok {
			return fmt.Sprintf("%s has no parameter %q", p.Kind, name)
		}
		if msg := spec.Check(p.Params[name]); msg != "" {
			return msg
		}
	}
	if p.Kind == ProcNoiseReduction {
		a, b := p.Value("profile_start_ms"), p.Value("profile_end_ms")
		if (a != 0 || b != 0) && b-a < MinProfileMS {
			return fmt.Sprintf("the noise profile must end at least %d ms after it starts", MinProfileMS)
		}
	}
	return ""
}

// Value is the value of a parameter: the stored one, or its default.
func (p Processor) Value(name string) float64 {
	if v, ok := p.Params[name]; ok {
		return v
	}
	if s, ok := paramSpec(p.Kind, name); ok {
		return s.Default
	}
	return 0
}

// CloneProcessors is a deep copy of a chain: no parameter map is shared with it. Nil stays nil.
func CloneProcessors(chain []Processor) []Processor {
	if chain == nil {
		return nil
	}
	out := make([]Processor, len(chain))
	for i, p := range chain {
		out[i] = Processor{Kind: p.Kind, Disabled: p.Disabled, Params: maps.Clone(p.Params)}
	}
	return out
}

// FullParams is every parameter of the kind with its value, stored or default.
func (p Processor) FullParams() map[string]float64 {
	out := map[string]float64{}
	for _, s := range processorParams[p.Kind] {
		out[s.Name] = p.Value(s.Name)
	}
	return out
}

// Normalized returns the processor with the values equal to their default removed and an empty map as nil, so two
// equal processors have the same encoding.
func (p Processor) Normalized() Processor {
	out := Processor{Kind: p.Kind, Disabled: p.Disabled}
	for name, v := range p.Params {
		if s, ok := paramSpec(p.Kind, name); ok && v == s.Default {
			continue
		}
		if out.Params == nil {
			out.Params = map[string]float64{}
		}
		out.Params[name] = v
	}
	return out
}

// checkChain checks a whole chain of an item or a track; the message is "" when it is valid.
func checkChain(chain []Processor, item bool) (index int, msg string) {
	if len(chain) > MaxProcessors {
		return len(chain) - 1, fmt.Sprintf("a chain has at most %d processors", MaxProcessors)
	}
	for i, p := range chain {
		if m := CheckProcessor(p); m != "" {
			return i, m
		}
		if !item && p.Kind == ProcNoiseReduction &&
			(p.Value("profile_start_ms") != 0 || p.Value("profile_end_ms") != 0) {
			return i, "a marked noise profile belongs to an item: a track has no single source to mark"
		}
	}
	return 0, ""
}

func (v *validator) processors(path, key string, chain []Processor, item bool) {
	if len(chain) == 0 {
		return
	}
	if i, msg := checkChain(chain, item); msg != "" {
		v.add(VShape, path+".processors["+strconv.Itoa(i)+"]", key, "%s", msg)
	}
}

// Presets are the processor chains of audio.apply_preset (SDD-002 C.83).
var Presets = []string{"voice", "voice_with_music", "podcast", "clean_noisy_room"}

// PresetChain is the chain of a preset, or nil for an unknown name. Every call returns a new slice.
func PresetChain(name string) []Processor {
	type values = map[string]float64

	pr := func(kind string, params values) Processor {
		return Processor{Kind: kind, Params: params}.Normalized()
	}
	deess := pr(ProcDeEsser, nil)
	limit := pr(ProcLimiter, nil)

	switch name {
	case "voice":
		return []Processor{
			pr(ProcNoiseReduction, values{"strength": 0.5}),
			pr(ProcEQ, values{"hp_hz": 80, "b2_hz": 250, "b2_gain_db": -2, "b4_hz": 3000, "b4_gain_db": 2}),
			deess,
			pr(ProcCompressor, values{"threshold_db": -20, "ratio": 3, "makeup_db": 3}),
			limit,
		}
	case "voice_with_music":
		return []Processor{
			pr(ProcEQ, values{"hp_hz": 100, "b2_hz": 250, "b2_gain_db": -2, "b4_hz": 3500, "b4_gain_db": 2.5}),
			deess,
			pr(ProcCompressor, values{"threshold_db": -22, "ratio": 2.5, "makeup_db": 2}),
			limit,
		}
	case "podcast":
		return []Processor{
			pr(ProcNoiseReduction, values{"strength": 0.4}),
			pr(ProcGate, values{"threshold_db": -55, "range_db": -20}),
			pr(ProcEQ, values{"hp_hz": 70, "b2_hz": 200, "b2_gain_db": -2, "b4_hz": 4000, "b4_gain_db": 2}),
			deess,
			pr(ProcCompressor, values{"threshold_db": -22, "ratio": 3.5, "makeup_db": 4}),
			limit,
		}
	case "clean_noisy_room":
		return []Processor{
			pr(ProcNoiseReduction, values{"strength": 0.8}),
			pr(ProcGate, values{"threshold_db": -45, "range_db": -15}),
			pr(ProcEQ, values{"hp_hz": 100, "b2_hz": 300, "b2_gain_db": -3, "b4_hz": 3000, "b4_gain_db": 2}),
			deess,
			pr(ProcCompressor, values{"threshold_db": -20, "ratio": 3, "makeup_db": 3}),
			limit,
		}
	}

	return nil
}

// ChainWarnings are the lint findings of a chain: the index of the enabled limiter that is followed by another enabled
// processor, and the index of every enabled noise reduction after the first.
func ChainWarnings(chain []Processor) (limiterNotLast, noiseRepeated []int) {
	last := -1
	for i, p := range chain {
		if !p.Disabled {
			last = i
		}
	}
	seenNR := false
	for i, p := range chain {
		if p.Disabled {
			continue
		}
		if p.Kind == ProcLimiter && i != last {
			limiterNotLast = append(limiterNotLast, i)
		}
		if p.Kind == ProcNoiseReduction {
			if seenNR {
				noiseRepeated = append(noiseRepeated, i)
			}
			seenNR = true
		}
	}
	return limiterNotLast, noiseRepeated
}
