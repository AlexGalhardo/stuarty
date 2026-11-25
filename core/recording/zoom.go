package recording

import (
	"errors"
	"fmt"
	"slices"
)

// The limits of the automatic zoom, chosen so that no default can make a viewer ill (SDD-025 section 17.4).
// The limits of the options, in the unit of each: the level in thousandths of the rest scale.
const (
	MinLevel = 1100
	MaxLevel = 3000

	MinLeadMS = 0
	MaxLeadMS = 3000

	MinRampMS = 200
	MaxRampMS = 3000

	MinHoldMS = 500
	MaxHoldMS = 10000

	MinMergeMS = 0
	MaxMergeMS = 10000

	MinZooms = 1
	MaxZooms = 24
)

const (
	chainGapMS      = 1000 // zooms that would rest for less than this between them are chained
	minMoveMS       = 300  // the shortest pan from one focus to the next of a chained pair
	groupShare      = 600  // thousandths of the visible window that the clicks of a zoom may span
	groupShareScale = 1000

	defaultLevel, defaultLead = 1600, 400
	defaultRamp, defaultHold  = 700, 1200
	defaultMerge, defaultMax  = 2500, MaxZooms
)

// ErrZoomOptions is returned for an option out of its range.
var ErrZoomOptions = errors.New("recording: invalid zoom options")

// ZoomOptions are the options of the planner. A zero member is the default.
type ZoomOptions struct {
	// Level is how far the view zooms in, in thousandths: 1600 is 1.6x.
	Level   int
	LeadMS  int
	RampMS  int
	HoldMS  int
	MergeMS int
	// MaxZooms is the most zooms planned; the first in time are kept.
	MaxZooms int
}

// Validate checks every member against its range.
func (o ZoomOptions) Validate() error {
	for _, f := range []struct {
		name      string
		v, lo, hi int
	}{
		{"level", o.Level, MinLevel, MaxLevel},
		{"lead_ms", o.LeadMS, MinLeadMS, MaxLeadMS},
		{"ramp_ms", o.RampMS, MinRampMS, MaxRampMS},
		{"hold_ms", o.HoldMS, MinHoldMS, MaxHoldMS},
		{"merge_ms", o.MergeMS, MinMergeMS, MaxMergeMS},
		{"max_zooms", o.MaxZooms, MinZooms, MaxZooms},
	} {
		if f.v < f.lo || f.v > f.hi {
			return fmt.Errorf("%w: %s is %d, outside %d to %d", ErrZoomOptions, f.name, f.v, f.lo, f.hi)
		}
	}

	return nil
}

// DefaultZoomOptions are the options a caller gets when it gives none.
func DefaultZoomOptions() ZoomOptions {
	return ZoomOptions{
		Level: defaultLevel, LeadMS: defaultLead, RampMS: defaultRamp, HoldMS: defaultHold, MergeMS: defaultMerge,
		MaxZooms: defaultMax,
	}
}

// Click is a button press on the axis of the asset: milliseconds, and the position as a share of the source in Unit.
type Click struct {
	TMS  int64
	X, Y int
}

// Zoom is one move of the view: it eases in from StartMS to FullMS, holds the level until ReleaseMS and eases out
// until EndMS (SDD-025 section 17.4). Times are milliseconds on the axis of the asset.
type Zoom struct {
	StartMS, FullMS, ReleaseMS, EndMS int64
	// FX and FY are the focus, in Unit of the source.
	FX, FY int
	// Clicks is how many presses the zoom holds.
	Clicks int
	// FromPrev says the view comes from the focus of the previous zoom instead of from rest (StartMS is then the
	// release of that zoom); ToNext says the view goes on to the next zoom instead of returning to rest (EndMS is
	// then ReleaseMS).
	FromPrev, ToNext bool
}

type group struct {
	first, last int64
	minX, maxX  int
	minY, maxY  int
	clicks      int
}

// PlanZooms plans the zooms of a take from its clicks, which are in time order and inside [fromMS, toMS) of the
// asset. vw and vh are the share of the source the frame shows at scale 1, in Unit (View.Visible). The same
// clicks and options always give the same plan. It returns the zooms and how many groups did not fit MaxZooms.
func PlanZooms(clicks []Click, opts ZoomOptions, vw, vh int, fromMS, toMS int64) (zooms []Zoom, dropped int) {
	spanX := vw * ScaleOne / opts.Level * groupShare / groupShareScale
	spanY := vh * ScaleOne / opts.Level * groupShare / groupShareScale

	var groups []group

	for _, c := range clicks {
		if c.TMS < fromMS || c.TMS >= toMS {
			continue
		}

		if n := len(groups); n > 0 {
			g := &groups[n-1]
			if c.TMS-g.last <= int64(opts.MergeMS) &&
				max(g.maxX, c.X)-min(g.minX, c.X) <= spanX && max(g.maxY, c.Y)-min(g.minY, c.Y) <= spanY {
				g.last, g.clicks = c.TMS, g.clicks+1
				g.minX, g.maxX = min(g.minX, c.X), max(g.maxX, c.X)
				g.minY, g.maxY = min(g.minY, c.Y), max(g.maxY, c.Y)

				continue
			}
		}

		groups = append(groups, group{c.TMS, c.TMS, c.X, c.X, c.Y, c.Y, 1})
	}

	if len(groups) > opts.MaxZooms {
		dropped = len(groups) - opts.MaxZooms
		groups = slices.Clone(groups[:opts.MaxZooms])
	}

	ramp := int64(opts.RampMS)

	for i, g := range groups {
		z := Zoom{FX: (g.minX + g.maxX) / 2, FY: (g.minY + g.maxY) / 2, Clicks: g.clicks}

		z.FullMS = g.first - int64(opts.LeadMS)
		z.StartMS = z.FullMS - ramp

		if z.StartMS < fromMS {
			z.StartMS = fromMS
			z.FullMS = max(z.FullMS, z.StartMS+MinRampMS)
		}

		z.ReleaseMS = g.last + int64(opts.HoldMS)
		z.EndMS = z.ReleaseMS + ramp

		var prev *Zoom

		prevEnd := int64(0)

		if n := len(zooms); n > 0 && z.StartMS < zooms[n-1].EndMS+chainGapMS {
			prev = &zooms[n-1]
			prevEnd = prev.EndMS
			prev.ToNext, prev.EndMS = true, prev.ReleaseMS
			z.FromPrev, z.StartMS = true, prev.ReleaseMS
			z.FullMS = max(z.FullMS, z.StartMS+minMoveMS)
		}

		z.ReleaseMS = min(max(z.ReleaseMS, z.FullMS), toMS)
		z.EndMS = min(max(z.EndMS, z.ReleaseMS), toMS)

		if z.FullMS >= toMS {
			if prev != nil {
				prev.ToNext, prev.EndMS = false, prevEnd
			}

			dropped += len(groups) - i

			break
		}

		zooms = append(zooms, z)
	}

	return zooms, dropped
}
