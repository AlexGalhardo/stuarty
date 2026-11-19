package audio

import (
	"testing"

	"github.com/AlexGalhardo/stuarty/core/plan"
	"github.com/AlexGalhardo/stuarty/core/timeline"
)

// BenchmarkChainPresets runs ten seconds of the noisy stereo fixture through the chain of each preset and reports how
// many times faster than real time it is (SDD-004 section 7.1, the performance line of the epic 34 test strategy).
func BenchmarkChainPresets(b *testing.B) {
	const seconds = 10

	sig, _ := speechLike(seconds)

	for _, name := range timeline.Presets {
		b.Run(name, func(b *testing.B) {
			var specs []plan.ProcessorSpec
			for _, p := range timeline.PresetChain(name) {
				specs = append(specs, plan.ProcessorSpec{Kind: p.Kind, Params: p.FullParams()})
			}

			l, r := make([]float64, len(sig)), make([]float64, len(sig))

			for b.Loop() {
				copy(l, sig)
				copy(r, sig)

				chain, err := NewChain(specs, nil)
				if err != nil {
					b.Fatal(err)
				}

				chain.Process(l, r)
			}

			b.ReportMetric(seconds/(b.Elapsed().Seconds()/float64(b.N)), "x_realtime")
		})
	}
}
