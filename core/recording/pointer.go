package recording

import (
	"errors"
	"fmt"
	"io"
)

// The bounds of the pointer data a command receives (SDD-025 section 17.2).
const (
	// MaxTrackEvents is the most events a PointerTrack holds.
	MaxTrackEvents = 30_000
	// MinTrackStepMS is the least time between two positions of a thinned track.
	MinTrackStepMS = 50
)

// PointerTrack is the part of the pointer events a clip plays, as the service hands it to the commands that finish
// a take: the size of the captured source and the events, in order, with the times on the axis of the asset
// (milliseconds) and the positions in pixels of the source.
type PointerTrack struct {
	Width  int            `json:"width"  desc:"Width in pixels of the captured source."`
	Height int            `json:"height" desc:"Height in pixels of the captured source."`
	Events []PointerEvent `json:"events" desc:"The pointer events in the source range of the clip, in time order."`
}

// ErrTrack is returned for a track that fails Validate.
var ErrTrack = errors.New("recording: invalid pointer track")

// Validate checks a track that came from outside the core, so the rules below can assume it.
func (t *PointerTrack) Validate() error {
	if t.Width < 1 || t.Width > MaxSide || t.Height < 1 || t.Height > MaxSide {
		return fmt.Errorf("%w: the source size %dx%d is outside 1 to %d", ErrTrack, t.Width, t.Height, MaxSide)
	}

	if len(t.Events) > MaxTrackEvents {
		return fmt.Errorf("%w: %d events, at most %d", ErrTrack, len(t.Events), MaxTrackEvents)
	}

	var last int64

	for i, e := range t.Events {
		switch e.Kind {
		case EventMove, EventDown, EventUp:
		default:
			return fmt.Errorf("%w: event %d has the kind %q", ErrTrack, i, e.Kind)
		}

		if e.TMS < last || e.TMS > int64(MaxSecondsCeiling)*1000 {
			return fmt.Errorf("%w: event %d is out of time order", ErrTrack, i)
		}

		if !inRange(e.X, t.Width) || !inRange(e.Y, t.Height) {
			return fmt.Errorf("%w: event %d is outside the source", ErrTrack, i)
		}

		last = e.TMS
	}

	return nil
}

// Clicks are the button presses of the track, with the position as a share of the source in Unit.
func (t *PointerTrack) Clicks() []Click {
	var out []Click

	for _, e := range t.Events {
		if e.Kind == EventDown {
			out = append(out, Click{TMS: e.TMS, X: shareOf(e.X, t.Width), Y: shareOf(e.Y, t.Height)})
		}
	}

	return out
}

// shareOf is the middle of pixel v of a side of size pixels, in Unit.
func shareOf(v, size int) int { return (2*v + 1) * Unit / (2 * size) }

// TrackQuery selects the part of an event file a clip plays.
type TrackQuery struct {
	// FromMS and ToMS are the half-open range of the axis of the asset.
	FromMS, ToMS int64
	// ClicksOnly keeps the button presses and nothing else.
	ClicksOnly bool
}

// ReadTrack reads an event file and returns the part a query selects. With positions (ClicksOnly false) the moves
// are thinned to one per MinTrackStepMS at least, the step growing with the range so that the track never holds
// more than MaxTrackEvents; the last position before FromMS is kept, moved to FromMS, so the pointer has a place
// at the start. A file that cannot be read at all is ErrEvents.
func ReadTrack(r io.Reader, q TrackQuery) (PointerTrack, error) {
	if q.FromMS < 0 || q.ToMS <= q.FromMS {
		return PointerTrack{}, fmt.Errorf("%w: the range %d to %d is empty", ErrEvents, q.FromMS, q.ToMS)
	}

	// Moves take at most half of the room; the edges (presses and releases) the rest.
	step := max(int64(MinTrackStepMS), (q.ToMS-q.FromMS)*2/MaxTrackEvents+1)

	var (
		out      PointerTrack
		before   *PointerEvent
		lastMove = q.FromMS - step
		moves    int
	)

	w, h, _, err := scanEvents(r, func(ev PointerEvent) bool {
		if ev.TMS >= q.ToMS {
			return false
		}

		if ev.TMS < q.FromMS {
			if !q.ClicksOnly {
				e := ev
				before = &e
			}

			return true
		}

		switch {
		case q.ClicksOnly && ev.Kind != EventDown:
			return true
		case ev.Kind == EventMove:
			if ev.TMS-lastMove < step || moves >= MaxTrackEvents/2 {
				return true
			}

			lastMove = ev.TMS
			moves++
		case len(out.Events)-moves >= MaxTrackEvents/2:
			return true
		}

		out.Events = append(out.Events, ev)

		return true
	})
	if err != nil {
		return PointerTrack{}, err
	}

	out.Width, out.Height = w, h

	if before != nil {
		first := *before
		first.TMS, first.Kind, first.Button = q.FromMS, EventMove, ""

		out.Events = append([]PointerEvent{first}, out.Events...)
	}

	return out, nil
}

// PathPoint is a position of the smoothed pointer at an instant of the axis of the asset, in milli-pixels of the
// source.
type PathPoint struct {
	TMS    int64
	XM, YM int64
}

// smoothTickMS is the step of the filter: finer than any keyframe spacing, coarse enough to be cheap.
const smoothTickMS = 10

// SmoothPath runs the pointer through an exponential filter of time constant tauMS (0 follows the raw path) and
// samples it every stepMS from fromMS to toMS, the last sample at toMS. Positions hold between events. It is
// integer arithmetic only: the same events and options give the same path on every system. The events are in
// time order and are not empty.
func SmoothPath(events []PointerEvent, fromMS, toMS int64, stepMS, tauMS int) []PathPoint {
	if len(events) == 0 || toMS < fromMS || stepMS < 1 {
		return nil
	}

	next, cur := 0, events[0]
	at := func(t int64) (x, y int64) {
		for next < len(events) && events[next].TMS <= t {
			cur = events[next]
			next++
		}

		return int64(cur.X) * milli, int64(cur.Y) * milli
	}

	px, py := at(fromMS)
	out := []PathPoint{{TMS: fromMS, XM: px, YM: py}}
	if toMS == fromMS {
		return out
	}

	due := fromMS + int64(stepMS)

	for t := fromMS + smoothTickMS; t < toMS+smoothTickMS; t += smoothTickMS {
		tx, ty := at(t)

		d := int64(tauMS) + smoothTickMS
		px += (tx - px) * smoothTickMS / d
		py += (ty - py) * smoothTickMS / d

		if t >= toMS {
			out = append(out, PathPoint{TMS: toMS, XM: px, YM: py})

			break
		}

		if t >= due {
			out = append(out, PathPoint{TMS: t, XM: px, YM: py})
			due += int64(stepMS)
		}
	}

	return out
}
