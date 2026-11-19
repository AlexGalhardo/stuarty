package audio

import (
	"testing"

	"github.com/AlexGalhardo/stuarty/core/plan"
	"github.com/AlexGalhardo/stuarty/core/timeline"
)

// Golden audio of each preset (the test strategy of epic 34): half a second of the noisy fixture, the noise and the
// start of a burst, with a different noise on each channel, through the whole chain of the preset; compared with
// testdata/preset_<name>.wav within the tolerance of SDD-004 section 7.1.
func TestPresetGoldens(t *testing.T) {
	t.Parallel()

	sig, _ := speechLike(1)
	left := sig[12000:36000]
	right := add(left, noise(23, nrNoiseRMS, len(left)))

	for _, name := range timeline.Presets {
		var specs []plan.ProcessorSpec
		for _, p := range timeline.PresetChain(name) {
			specs = append(specs, plan.ProcessorSpec{Kind: p.Kind, Params: p.FullParams()})
		}

		l, r := throughStereo(t, left, right, 0, specs...)
		golden(t, "preset_"+name, l, r)
	}
}
