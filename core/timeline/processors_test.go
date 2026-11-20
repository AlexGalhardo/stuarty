package timeline

import (
	"slices"
	"testing"
)

func TestEveryPresetIsAValidChain(t *testing.T) {
	t.Parallel()

	for _, name := range Presets {
		chain := PresetChain(name)
		if len(chain) == 0 {
			t.Fatalf("preset %s is empty", name)
		}

		if i, msg := checkChain(chain, true); msg != "" {
			t.Errorf("preset %s, processor %d: %s", name, i, msg)
		}

		if notLast, repeated := ChainWarnings(chain); len(notLast)+len(repeated) > 0 {
			t.Errorf("preset %s triggers the chain lint: %v %v", name, notLast, repeated)
		}
	}

	if PresetChain("radio") != nil {
		t.Error("an unknown preset has a chain")
	}
}

func TestCheckProcessor(t *testing.T) {
	t.Parallel()

	cases := map[string]struct {
		p  Processor
		ok bool
	}{
		"defaults":          {Processor{Kind: ProcCompressor}, true},
		"unknown kind":      {Processor{Kind: "reverb"}, false},
		"unknown param":     {Processor{Kind: ProcGate, Params: map[string]float64{"ratio": 2}}, false},
		"above range":       {Processor{Kind: ProcGate, Params: map[string]float64{"threshold_db": 1}}, false},
		"below range":       {Processor{Kind: ProcCompressor, Params: map[string]float64{"ratio": 0.5}}, false},
		"edge of range":     {Processor{Kind: ProcCompressor, Params: map[string]float64{"ratio": 20}}, true},
		"high-pass off":     {Processor{Kind: ProcEQ, Params: map[string]float64{"hp_hz": 0}}, true},
		"high-pass too low": {Processor{Kind: ProcEQ, Params: map[string]float64{"hp_hz": 10}}, false},
		"high-pass on":      {Processor{Kind: ProcEQ, Params: map[string]float64{"hp_hz": 80}}, true},
		"profile one-sided": {
			Processor{Kind: ProcNoiseReduction, Params: map[string]float64{"profile_end_ms": 10}},
			false,
		},
		"profile too short": {
			Processor{
				Kind:   ProcNoiseReduction,
				Params: map[string]float64{"profile_start_ms": 100, "profile_end_ms": 110},
			},
			false,
		},
		"profile ok": {
			Processor{
				Kind:   ProcNoiseReduction,
				Params: map[string]float64{"profile_start_ms": 100, "profile_end_ms": 600},
			},
			true,
		},
		"strength above one": {Processor{Kind: ProcNoiseReduction, Params: map[string]float64{"strength": 1.5}}, false},
	}

	for name, c := range cases {
		if msg := CheckProcessor(c.p); (msg == "") != c.ok {
			t.Errorf("%s: %q, want ok=%v", name, msg, c.ok)
		}
	}
}

func TestNormalizedDropsDefaults(t *testing.T) {
	t.Parallel()

	p := Processor{Kind: ProcCompressor, Params: map[string]float64{"ratio": 3, "attack_ms": 5}}.Normalized()
	if len(p.Params) != 1 || p.Params["attack_ms"] != 5 {
		t.Fatalf("params %v", p.Params)
	}

	if q := (Processor{Kind: ProcLimiter, Params: map[string]float64{"ceiling_db": -1}}).Normalized(); q.Params != nil {
		t.Errorf("an all-default processor keeps %v", q.Params)
	}

	full := p.FullParams()
	if full["ratio"] != 3 || full["attack_ms"] != 5 || len(full) != len(ProcessorParams(ProcCompressor)) {
		t.Errorf("full params %v", full)
	}
}

func TestChainWarnings(t *testing.T) {
	t.Parallel()

	mk := func(kinds ...string) []Processor {
		var out []Processor
		for _, k := range kinds {
			out = append(out, Processor{Kind: k})
		}

		return out
	}

	notLast, repeated := ChainWarnings(mk(ProcLimiter, ProcGate, ProcNoiseReduction, ProcNoiseReduction))
	if !slices.Equal(notLast, []int{0}) || !slices.Equal(repeated, []int{3}) {
		t.Errorf("got %v %v", notLast, repeated)
	}

	chain := mk(ProcLimiter, ProcGate)
	chain[1].Disabled = true

	if notLast, _ := ChainWarnings(chain); len(notLast) != 0 {
		t.Errorf("a disabled processor after the limiter warns: %v", notLast)
	}
}

func TestCloneProcessorsSharesNothing(t *testing.T) {
	t.Parallel()

	a := []Processor{{Kind: ProcGate, Params: map[string]float64{"threshold_db": -30}}}
	b := CloneProcessors(a)
	b[0].Params["threshold_db"] = -10

	if a[0].Params["threshold_db"] != -30 {
		t.Error("the clone shares the parameter map")
	}
}
