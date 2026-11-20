package audio

import (
	"fmt"

	"github.com/AlexGalhardo/stuarty/core/plan"
	"github.com/AlexGalhardo/stuarty/core/timeline"
)

// Chain is an ordered list of processors run one after the other over stereo frames (SDD-004 section 7.1).
type Chain struct {
	stages []stage
	lat    int
}

// NewChain builds the stages of specs, in order. profile is the measured noise profile of a noise_reduction with a
// marked range, or nil for the estimated one.
func NewChain(specs []plan.ProcessorSpec, profile noiseProfile) (*Chain, error) {
	c := &Chain{}

	for _, s := range specs {
		var st stage

		switch s.Kind {
		case timeline.ProcEQ:
			st = newEqualiser(s.Params)
		case timeline.ProcCompressor:
			st = newCompressor(s.Params)
		case timeline.ProcLimiter:
			st = newLimiter(s.Params)
		case timeline.ProcGate:
			st = newGate(s.Params)
		case timeline.ProcDeEsser:
			st = newDeesser(s.Params)
		case timeline.ProcNoiseReduction:
			st = newNoiseReduction(s.Params, profile)
		default:
			return nil, fmt.Errorf("audio: %q is not a processor", s.Kind)
		}

		c.stages = append(c.stages, st)
		c.lat += st.latency()
	}

	return c, nil
}

// Latency is how many frames the chain delays its output: the sum of its stages.
func (c *Chain) Latency() int { return c.lat }

// Process runs the chain in place over the frames l[i], r[i]; the output is delayed by Latency frames.
func (c *Chain) Process(l, r []float64) {
	for _, s := range c.stages {
		s.process(l, r)
	}
}

// MarkedProfile is the range, in milliseconds of the asset, that the noise reduction of the chain was told to take its
// profile from; both are 0 when the chain has none or estimates it.
func MarkedProfile(specs []plan.ProcessorSpec) (startMS, endMS float64) {
	for _, s := range specs {
		if s.Kind == timeline.ProcNoiseReduction {
			if a, b := s.Params["profile_start_ms"], s.Params["profile_end_ms"]; b > a {
				return a, b
			}
		}
	}

	return 0, 0
}

// runner feeds a chain from a source of frames and gives back the output aligned with the input: the chain delays
// its output by its latency, so the first call reads that many frames more than it returns and drops the first ones.
type runner struct {
	chain  *Chain
	pull   func(l, r []float64) error // fills the next len(l) input frames
	primed bool
}

// next fills l and r with the next len(l) processed frames.
func (u *runner) next(l, r []float64) error {
	lat := u.chain.Latency()
	if u.primed || lat == 0 {
		u.primed = true

		if err := u.pull(l, r); err != nil {
			return err
		}

		u.chain.Process(l, r)

		return nil
	}

	u.primed = true
	n := len(l)
	tl, tr := make([]float64, n+lat), make([]float64, n+lat)

	if err := u.pull(tl, tr); err != nil {
		return err
	}

	u.chain.Process(tl, tr)
	copy(l, tl[lat:])
	copy(r, tr[lat:])

	return nil
}
