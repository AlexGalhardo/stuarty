package recording

import (
	"errors"
	"fmt"
	"slices"
)

// TrackKind is one of the four sources a recording can hold (SDD-025 section 16.1).
type TrackKind string

// The track kinds, in the order a recording lists them.
const (
	TrackScreen     TrackKind = "screen"
	TrackCamera     TrackKind = "camera"
	TrackMicrophone TrackKind = "microphone"
	TrackSystem     TrackKind = "system"
)

// TrackKinds is the closed list of kinds, in listing order.
var TrackKinds = []TrackKind{TrackScreen, TrackCamera, TrackMicrophone, TrackSystem}

// Valid reports whether k is one of the four kinds.
func (k TrackKind) Valid() bool { return slices.Contains(TrackKinds, k) }

// File is the name of the file of a track in the folder of its recording.
func (k TrackKind) File() string {
	switch k {
	case TrackScreen:
		return "capture.mp4"
	case TrackCamera:
		return "camera.mp4"
	case TrackMicrophone:
		return "microphone.wav"
	case TrackSystem:
		return "system.wav"
	}

	return ""
}

// Audio reports whether the track is a sound track (it is placed on an audio track).
func (k TrackKind) Audio() bool { return k == TrackMicrophone || k == TrackSystem }

// Label is the name of the timeline track a kind is placed on.
func (k TrackKind) Label() string {
	switch k {
	case TrackScreen:
		return "Recording"
	case TrackCamera:
		return "Camera"
	case TrackMicrophone:
		return "Microphone"
	case TrackSystem:
		return "System audio"
	}

	return ""
}

// EventsFile is the name of the pointer event file of a recording.
const EventsFile = "events.jsonl"

// MaxDriftMS is the largest distance between two tracks the product accepts (NFR-068, SDD-025 section 16.2).
const MaxDriftMS = 40

// Track is one source of a recording as the helper reported it when it finished.
type Track struct {
	Kind       TrackKind `json:"kind"`
	File       string    `json:"file"`
	StartMS    int64     `json:"start_ms"`
	DurationMS int64     `json:"duration_ms"`
	Bytes      int64     `json:"bytes"`
	// Lost is a track whose device went away during the recording: it is shorter than the others.
	Lost      bool   `json:"lost,omitempty"`
	AssetHash string `json:"asset_hash,omitempty"`
	AssetID   string `json:"asset_id,omitempty"`
}

// Pause is an interval of the helper's wall clock, in milliseconds since the origin, that was removed from every
// file (SDD-025 section 16.4).
type Pause struct {
	FromMS int64 `json:"from_ms"`
	ToMS   int64 `json:"to_ms"`
}

// ErrTracks is returned when a list of tracks or pauses breaks a rule.
var ErrTracks = errors.New("recording: invalid tracks")

// ValidateTracks checks what a helper reported: known kinds, each once, the screen present, the file the kind
// names, and non-negative numbers.
func ValidateTracks(tracks []Track) error {
	seen := map[TrackKind]bool{}

	for _, t := range tracks {
		switch {
		case !t.Kind.Valid():
			return fmt.Errorf("%w: unknown kind %q", ErrTracks, t.Kind)
		case seen[t.Kind]:
			return fmt.Errorf("%w: %q twice", ErrTracks, t.Kind)
		case t.File != t.Kind.File():
			return fmt.Errorf("%w: %q has the file %q", ErrTracks, t.Kind, t.File)
		case t.StartMS < 0 || t.DurationMS < 0 || t.Bytes < 0:
			return fmt.Errorf("%w: %q has a negative number", ErrTracks, t.Kind)
		}

		seen[t.Kind] = true
	}

	if !seen[TrackScreen] {
		return fmt.Errorf("%w: no screen track", ErrTracks)
	}

	return nil
}

// ValidatePauses checks that the pauses are in order, do not overlap and each has a positive length.
func ValidatePauses(pauses []Pause) error {
	var end int64

	for i, p := range pauses {
		if p.FromMS < end || p.ToMS <= p.FromMS {
			return fmt.Errorf("%w: pause %d is %d to %d", ErrTracks, i, p.FromMS, p.ToMS)
		}

		end = p.ToMS
	}

	return nil
}

// PausedMS is the total length of the pauses.
func PausedMS(pauses []Pause) int64 {
	var n int64

	for _, p := range pauses {
		n += p.ToMS - p.FromMS
	}

	return n
}

// ActiveMS converts a moment of the wall clock to active time (the pauses removed). ok is false for a moment
// inside a pause, which no file holds.
func ActiveMS(wallMS int64, pauses []Pause) (active int64, ok bool) {
	var removed int64

	for _, p := range pauses {
		switch {
		case wallMS < p.FromMS:
			return wallMS - removed, true
		case wallMS < p.ToMS:
			return 0, false
		}

		removed += p.ToMS - p.FromMS
	}

	return wallMS - removed, true
}

// Drift is the distance between the longest and the shortest of the tracks that did not lose their device: the
// number NFR-068 bounds. A recording with fewer than two such tracks has none.
func Drift(tracks []Track) int64 {
	var lo, hi int64

	n := 0

	for _, t := range tracks {
		if t.Lost {
			continue
		}

		end := t.StartMS + t.DurationMS
		if n == 0 || end < lo {
			lo = end
		}

		if n == 0 || end > hi {
			hi = end
		}

		n++
	}

	if n < 2 {
		return 0
	}

	return hi - lo
}

// PlaceStarts is where each track starts on the timeline relative to the first one: the start the helper
// reported minus the smallest start, so tracks that share the origin all start together (SDD-025 section 16.9).
func PlaceStarts(tracks []Track) map[TrackKind]int64 {
	out := make(map[TrackKind]int64, len(tracks))

	if len(tracks) == 0 {
		return out
	}

	base := tracks[0].StartMS
	for _, t := range tracks {
		base = min(base, t.StartMS)
	}

	for _, t := range tracks {
		out[t.Kind] = t.StartMS - base
	}

	return out
}

// TracksAsked is the tracks a start request asks for, in listing order. The screen is always there.
func TracksAsked(s StartSpec) []TrackKind {
	out := []TrackKind{TrackScreen}

	if s.Camera != "" {
		out = append(out, TrackCamera)
	}

	if s.Microphone != "" {
		out = append(out, TrackMicrophone)
	}

	if s.SystemAudio {
		out = append(out, TrackSystem)
	}

	return out
}
