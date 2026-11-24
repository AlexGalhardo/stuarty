package facetrack

import (
	"errors"
	"fmt"
)

// Version is the version of the face track artifact (SDD-024 section 6).
const Version = 1

// Unit is the fixed-point scale of a position: ten-thousandths of the source frame. Every value that decides
// a keyframe is an integer, so the result is the same on every system (SDD-024 section 1 rule 5).
const Unit = 10000

// Limits of a track read back from an artifact or sent by a helper's host.
const (
	MaxSamples      = 400_000
	MaxFacesPerShot = 16
	MaxSide         = 16384
)

// Face is one detected face in ten-thousandths of the source frame, with its score in thousandths.
type Face struct {
	X     int `json:"x"`
	Y     int `json:"y"`
	W     int `json:"w"`
	H     int `json:"h"`
	Score int `json:"s"`
}

// CenterX is the horizontal centre of the box.
func (f Face) CenterX() int { return f.X + f.W/2 }

// CenterY is the vertical centre of the box.
func (f Face) CenterY() int { return f.Y + f.H/2 }

// Sample is the faces seen at one instant of the asset, best first.
type Sample struct {
	// T is milliseconds on the time axis of the asset.
	T     int64  `json:"t"`
	Faces []Face `json:"faces"`
}

// Size is the size of the source frame in pixels.
type Size struct {
	Width  int `json:"width"`
	Height int `json:"height"`
}

// Track is the face track of a range of an asset: the artifact `facetrack` (SDD-024 section 6).
type Track struct {
	Version  int      `json:"version"`
	Model    string   `json:"model"`
	Provider string   `json:"provider"`
	Source   Size     `json:"source"`
	FPS      int      `json:"fps"`
	StartMS  int64    `json:"start_ms"`
	EndMS    int64    `json:"end_ms"`
	Samples  []Sample `json:"samples"`
}

// ErrTrack is the error of an invalid track.
var ErrTrack = errors.New("facetrack: invalid track")

func invalid(format string, a ...any) error {
	return fmt.Errorf("%w: %s", ErrTrack, fmt.Sprintf(format, a...))
}

// Validate checks a track that came from outside the process: the version, the sizes, the order of the
// samples and every box, so the rules below can assume them.
func (t *Track) Validate() error {
	if t.Version != Version {
		return invalid("version %d is not %d", t.Version, Version)
	}

	if t.Source.Width < 1 || t.Source.Width > MaxSide || t.Source.Height < 1 || t.Source.Height > MaxSide {
		return invalid("the source size %dx%d is outside 1 to %d", t.Source.Width, t.Source.Height, MaxSide)
	}

	if t.FPS < 1 || t.FPS > 60 {
		return invalid("fps %d is outside 1 to 60", t.FPS)
	}

	if t.StartMS < 0 || t.EndMS < t.StartMS {
		return invalid("the range %d to %d is not valid", t.StartMS, t.EndMS)
	}

	if len(t.Samples) > MaxSamples {
		return invalid("%d samples is over the limit %d", len(t.Samples), MaxSamples)
	}

	prev := int64(-1)

	for i := range t.Samples {
		s := &t.Samples[i]
		if s.T <= prev {
			return invalid("sample %d is not after the one before it", i)
		}

		prev = s.T

		if len(s.Faces) > MaxFacesPerShot {
			return invalid("sample %d has %d faces", i, len(s.Faces))
		}

		for j, f := range s.Faces {
			if f.W <= 0 || f.H <= 0 || f.X < 0 || f.Y < 0 || f.X+f.W > Unit || f.Y+f.H > Unit ||
				f.Score < 0 || f.Score > 1000 {
				return invalid("face %d of sample %d is outside the frame", j, i)
			}
		}
	}

	return nil
}
