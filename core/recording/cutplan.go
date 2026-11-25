package recording

import (
	"errors"
	"fmt"
	"slices"
	"strings"
)

// Clip is a clip of the timeline that shows one track of a recording, as the planner needs it. Every number is
// in frames of the timeline; SourceIn is where in the asset the clip starts (SDD-001, speed 1).
type Clip struct {
	ID       string
	Track    string
	Kind     TrackKind
	Start    int
	Duration int
	SourceIn int
	// Slow is true when the clip does not play at speed 1: its frames are not those of the source.
	Slow bool
}

// StepOp is the command of a step.
type StepOp string

// The two commands a plan uses; no other command is needed (SDD-025 section 19.5).
const (
	StepSplit  StepOp = "item.split"
	StepDelete StepOp = "item.delete"
)

// Step is one command of the batch. A split names the item, the timeline frame and the id of the new right
// part; a delete names the item and closes the gap on its track (ripple `track`).
type Step struct {
	Op     StepOp
	ItemID string
	At     int
	NewID  string
}

// ErrSlowClip is returned for a clip that does not play at speed 1.
var ErrSlowClip = errors.New("recording: a clip with a changed speed cannot be cut by a range of the take")

// ErrNothingToCut is returned when no accepted range falls on any clip.
var ErrNothingToCut = errors.New("recording: no accepted range is on the timeline")

// MSToFrames is a time in milliseconds as frames of a timeline at num/den frames per second, rounded to the
// nearest frame.
func MSToFrames(ms int64, num, den int) int {
	n, d := int64(max(num, 1)), int64(max(den, 1))

	return int((ms*n + 500*d) / (1000 * d))
}

// PlanCuts is the batch that cuts the accepted ranges out of the clips of a recording. It is a pure function of
// its arguments: the ranges are on the clock of the recording, tracks gives the start of each track on that
// clock (the planner moves a range into the source of each clip with it), and newID makes the ids of the pieces.
//
// The ranges are taken last first, so that the closing of a gap never moves a place a later step still needs, and
// the clips are simulated, so the second range sees what the first one did. A clip that holds only part of a
// range loses that part; a clip a range does not touch moves earlier with its track as the gaps close. The
// ranges must lie inside the span every track shares (SharedSpan), which ValidateRanges checks when a proposal
// is made, so each track loses the same time and they stay aligned.
func PlanCuts(
	ranges []CutRange,
	clips []Clip,
	tracks []Track,
	fpsNum, fpsDen int,
	newID func() string,
) ([]Step, error) {
	start := map[TrackKind]int64{}
	for _, t := range tracks {
		start[t.Kind] = t.StartMS
	}

	sim := slices.Clone(clips)
	for i := range sim {
		if sim[i].Slow {
			return nil, fmt.Errorf("%w: %s", ErrSlowClip, sim[i].ID)
		}
	}

	rs := slices.Clone(ranges)
	slices.SortFunc(rs, func(a, b CutRange) int { return int(b.FromMS - a.FromMS) })

	var steps []Step

	for _, r := range rs {
		ids := make([]string, len(sim))
		for i := range sim {
			ids[i] = sim[i].ID
		}

		pos := func(id string) int { return slices.IndexFunc(sim, func(c Clip) bool { return c.ID == id }) }

		// Later clips first on every track: closing a gap moves only what is after it.
		slices.SortStableFunc(ids, func(a, b string) int {
			ca, cb := sim[pos(a)], sim[pos(b)]
			if c := strings.Compare(ca.Track, cb.Track); c != 0 {
				return c
			}

			return cb.Start - ca.Start
		})

		for _, id := range ids {
			if i := pos(id); i >= 0 {
				steps = cutClip(steps, &sim, i, r, start[sim[i].Kind], fpsNum, fpsDen, newID)
			}
		}
	}

	if len(steps) == 0 {
		return nil, ErrNothingToCut
	}

	return steps, nil
}

// cutClip cuts the part of range r that clip i shows, appends the steps and updates the simulation.
func cutClip(
	steps []Step,
	simp *[]Clip,
	i int,
	r CutRange,
	trackStart int64,
	num, den int,
	newID func() string,
) []Step {
	sim := *simp
	c := sim[i]
	a, b := MSToFrames(r.FromMS-trackStart, num, den), MSToFrames(r.ToMS-trackStart, num, den)
	srcEnd := c.SourceIn + c.Duration
	lo, hi := max(a, c.SourceIn), min(b, srcEnd)

	if hi <= lo {
		return steps
	}

	tlLo, tlHi := c.Start+(lo-c.SourceIn), c.Start+(hi-c.SourceIn)
	removed := hi - lo
	end := c.Start + c.Duration

	switch {
	case lo == c.SourceIn && hi == srcEnd:
		steps = append(steps, Step{Op: StepDelete, ItemID: c.ID})
		sim = slices.Delete(sim, i, i+1)
	case lo == c.SourceIn:
		n := newID()
		steps = append(steps, Step{Op: StepSplit, ItemID: c.ID, At: tlHi, NewID: n}, Step{Op: StepDelete, ItemID: c.ID})
		sim[i] = Clip{ID: n, Track: c.Track, Kind: c.Kind, Start: c.Start, Duration: srcEnd - hi, SourceIn: hi}
	case hi == srcEnd:
		n := newID()
		steps = append(steps, Step{Op: StepSplit, ItemID: c.ID, At: tlLo, NewID: n}, Step{Op: StepDelete, ItemID: n})
		sim[i].Duration = lo - c.SourceIn
	default:
		right, middle := newID(), newID()
		steps = append(steps,
			Step{Op: StepSplit, ItemID: c.ID, At: tlHi, NewID: right},
			Step{Op: StepSplit, ItemID: c.ID, At: tlLo, NewID: middle},
			Step{Op: StepDelete, ItemID: middle})
		sim[i].Duration = lo - c.SourceIn
		sim = append(
			sim,
			Clip{ID: right, Track: c.Track, Kind: c.Kind, Start: tlLo, Duration: srcEnd - hi, SourceIn: hi},
		)
	}

	for j := range sim {
		if sim[j].Track == c.Track && sim[j].Start >= end && sim[j].ID != c.ID {
			sim[j].Start -= removed
		}
	}

	*simp = sim

	return steps
}
