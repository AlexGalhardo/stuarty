package facetrack

import (
	"errors"
	"fmt"
	"sort"
)

// Options are the knobs of the framing rules (SDD-024 section 7). Every value is an integer.
type Options struct {
	// SmoothingMS is the time constant of the exponential smoothing of the frame position.
	SmoothingMS int `json:"smoothing_ms,omitempty"`
	// DeadZone is how far the face may drift from the centre of the frame before the frame follows, in
	// ten-thousandths of the visible window.
	DeadZone int `json:"dead_zone,omitempty"`
	// MaxPan is the fastest the frame may move, in ten-thousandths of the source frame per second.
	MaxPan int `json:"max_pan,omitempty"`
	// HoldMS is how long the last position is held when no face is seen, before the frame eases to the centre.
	HoldMS int `json:"hold_ms,omitempty"`
	// KeyframeMS is the least time between two keyframes.
	KeyframeMS int `json:"keyframe_ms,omitempty"`
}

// DefaultOptions are the defaults of SDD-024 section 7.
func DefaultOptions() Options {
	return Options{SmoothingMS: 400, DeadZone: 800, MaxPan: 6000, HoldMS: 1500, KeyframeMS: 200}
}

// WithDefaults fills every member that is zero.
func (o Options) WithDefaults() Options {
	d := DefaultOptions()

	if o.SmoothingMS == 0 {
		o.SmoothingMS = d.SmoothingMS
	}

	if o.DeadZone == 0 {
		o.DeadZone = d.DeadZone
	}

	if o.MaxPan == 0 {
		o.MaxPan = d.MaxPan
	}

	if o.HoldMS == 0 {
		o.HoldMS = d.HoldMS
	}

	if o.KeyframeMS == 0 {
		o.KeyframeMS = d.KeyframeMS
	}

	return o
}

// ErrOptions is the error of an option out of its range.
var ErrOptions = errors.New("facetrack: invalid options")

// Validate checks the ranges of options that already have their defaults.
func (o Options) Validate() error {
	check := func(name string, v, lo, hi int) error {
		if v < lo || v > hi {
			return fmt.Errorf("%w: %s %d is outside %d to %d", ErrOptions, name, v, lo, hi)
		}

		return nil
	}

	for _, err := range []error{
		check("smoothing_ms", o.SmoothingMS, 0, 5000),
		check("dead_zone", o.DeadZone, 0, 5000),
		check("max_pan", o.MaxPan, 100, 100_000),
		check("hold_ms", o.HoldMS, 0, 30_000),
		check("keyframe_ms", o.KeyframeMS, 50, 5000),
	} {
		if err != nil {
			return err
		}
	}

	return nil
}

// Window is the part of the source frame a vertical frame shows, as the four crop sides of a clip, in
// ten-thousandths of the source frame. Left plus right, and top plus bottom, are at most Unit.
type Window struct {
	Left, Right, Top, Bottom int
}

// Visible is the share of the source width and height a frame of FW x FH shows when it covers the frame
// (fit cover): the whole of one axis and a part of the other.
func Visible(src Size, fw, fh int) (vw, vh int) {
	vw = min(Unit, src.Height*fw*Unit/(src.Width*fh))
	vh = min(Unit, src.Width*fh*Unit/(src.Height*fw))

	return max(vw, 1), max(vh, 1)
}

// WindowAt is the window of size vw x vh centred on (cx, cy) and kept inside the source frame.
func WindowAt(vw, vh, cx, cy int) Window {
	left := max(0, min(cx-vw/2, Unit-vw))
	top := max(0, min(cy-vh/2, Unit-vh))

	return Window{Left: left, Right: Unit - vw - left, Top: top, Bottom: Unit - vh - top}
}

// Keyframe is the centre of the window at an instant of the asset, in milliseconds and ten-thousandths of
// the source frame.
type Keyframe struct {
	T int64 `json:"t"`
	X int   `json:"x"`
	Y int   `json:"y"`
}

func area(f Face) int64 { return int64(f.W) * int64(f.H) }

func weight(f Face) int64 { return area(f) * int64(f.Score) }

func abs(v int) int {
	if v < 0 {
		return -v
	}

	return v
}

// subject follows one face through the samples (SDD-024 section 7 rule 1).
type subject struct {
	have       bool
	cx, cy     int
	challenger int
}

// subjectRadius is how far, in ten-thousandths of the source width, a face may be from the last position of
// the subject and still be the same person.
const subjectRadius = 2500

// switchAfter is the number of consecutive samples a clearly stronger face must hold before it takes over.
const switchAfter = 3

// pick returns the centre the frame should aim at for a sample, and whether any face is followed.
func (s *subject) pick(faces []Face) (cx, cy int, ok bool) {
	if len(faces) == 0 {
		return 0, 0, false
	}

	best := 0

	for i := range faces {
		if weight(faces[i]) > weight(faces[best]) {
			best = i
		}
	}

	if !s.have {
		s.have, s.cx, s.cy, s.challenger = true, faces[best].CenterX(), faces[best].CenterY(), 0

		return s.cx, s.cy, true
	}

	cur, dist := -1, subjectRadius+1

	for i := range faces {
		if d := abs(faces[i].CenterX() - s.cx); d <= subjectRadius && d < dist {
			cur, dist = i, d
		}
	}

	switch {
	case cur < 0:
		// The subject is gone: take the strongest face at once.
		cur = best
		s.challenger = 0
	case cur != best && weight(faces[best])*2 >= weight(faces[cur])*3:
		s.challenger++
		if s.challenger >= switchAfter {
			cur, s.challenger = best, 0
		}
	default:
		s.challenger = 0
	}

	s.cx, s.cy = faces[cur].CenterX(), faces[cur].CenterY()

	return s.cx, s.cy, true
}

// follow moves cur towards target for dt milliseconds: beyond the dead zone, by the share of the distance
// the smoothing allows, and never faster than maxStep.
func follow(cur, target, dt, tau, dead, maxStep int) int {
	diff := target - cur
	if abs(diff) <= dead {
		return cur
	}

	excess := abs(diff) - dead
	// alpha is dt/(dt+tau) in thousandths.
	alpha := dt * 1000 / (dt + tau)
	step := min(excess*alpha/1000, maxStep)

	if diff < 0 {
		step = -step
	}

	return cur + step
}

// Plan turns a track into the keyframes of the window centre for the frame vw x vh (from Visible) over
// [from, to] milliseconds of the asset. It is deterministic: the same track and options give the same
// keyframes, with integers only. o must have its defaults and be valid.
func Plan(t *Track, o Options, vw, vh int, from, to int64) []Keyframe {
	var (
		sub        subject
		kfs        []Keyframe
		curX, curY = Unit / 2, Unit / 2
		tgtX, tgtY = Unit / 2, Unit / 2
		lastSeen   = int64(-1)
		prevT      int64
		started    bool
	)

	samples := t.Samples[sort.Search(len(t.Samples), func(i int) bool { return int64(t.Samples[i].T) >= from }):]

	var prev Keyframe

	// emit keeps a keyframe when the centre changed and enough time passed. A centre that did not change is
	// not kept, but the last such instant is kept just before the next change, so the frame holds still until
	// then instead of drifting between two distant keyframes.
	emit := func(at int64, x, y int, force bool) {
		k := Keyframe{T: at, X: x, Y: y}
		before := prev
		prev = k

		if n := len(kfs); n > 0 && !force {
			last := kfs[n-1]
			if at-last.T < int64(o.KeyframeMS) || last.X == x && last.Y == y {
				return
			}

			if before.T-last.T >= int64(o.KeyframeMS) && before.X == last.X && before.Y == last.Y {
				kfs = append(kfs, before)
			}
		}

		kfs = append(kfs, k)
	}

	clampX := func(v int) int { return max(vw/2, min(v, Unit-vw/2)) }
	clampY := func(v int) int { return max(vh/2, min(v, Unit-vh/2)) }

	step := func(at int64, faces []Face) {
		if cx, cy, ok := sub.pick(faces); ok {
			tgtX, tgtY, lastSeen = cx, cy, at
		} else if lastSeen >= 0 && at-lastSeen > int64(o.HoldMS) {
			tgtX, tgtY = Unit/2, Unit/2
		}

		if !started {
			curX, curY, started = clampX(tgtX), clampY(tgtY), true
			emit(at, curX, curY, true)
			prevT = at

			return
		}

		dt := int(at - prevT)
		maxStep := o.MaxPan * dt / 1000
		curX = clampX(follow(curX, tgtX, dt, o.SmoothingMS, o.DeadZone*vw/Unit, maxStep))
		curY = clampY(follow(curY, tgtY, dt, o.SmoothingMS, o.DeadZone*vh/Unit, maxStep))
		prevT = at
		emit(at, curX, curY, false)
	}

	for i := range samples {
		if int64(samples[i].T) > to {
			break
		}

		step(int64(samples[i].T), samples[i].Faces)
	}

	if !started {
		return []Keyframe{{T: from, X: Unit / 2, Y: Unit / 2}, {T: max(to, from+1), X: Unit / 2, Y: Unit / 2}}
	}

	if last := kfs[len(kfs)-1]; last.T < to {
		kfs = append(kfs, Keyframe{T: to, X: last.X, Y: last.Y})
	}

	return kfs
}

// At is the centre at an instant, by linear interpolation between the keyframes in integers; before the
// first keyframe and after the last it is the nearest one.
func At(kfs []Keyframe, t int64) (x, y int) {
	if len(kfs) == 0 {
		return Unit / 2, Unit / 2
	}

	if t <= kfs[0].T {
		return kfs[0].X, kfs[0].Y
	}

	for i := 1; i < len(kfs); i++ {
		if t <= kfs[i].T {
			a, b := kfs[i-1], kfs[i]
			span := b.T - a.T

			return a.X + int(int64(b.X-a.X)*(t-a.T)/span), a.Y + int(int64(b.Y-a.Y)*(t-a.T)/span)
		}
	}

	last := kfs[len(kfs)-1]

	return last.X, last.Y
}
