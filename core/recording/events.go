package recording

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
)

// The bounds of the pointer event file (SDD-025 section 16.6).
const (
	// MaxEventLine is the longest line the file may hold.
	MaxEventLine = 200
	// MaxEventLines is the most lines the file may hold, the header included.
	MaxEventLines = 2_000_000
	// MaxEventsLimit is the most events one RecordingEvents answer holds; DefaultEventsLimit is its default.
	MaxEventsLimit     = 5000
	DefaultEventsLimit = 1000
)

// EventKind is the kind of a line of the event file. There is no kind for a key, and none can be added without
// changing this list and its security test.
type EventKind string

// The kinds of a line.
const (
	EventHeader EventKind = "header"
	EventMove   EventKind = "move"
	EventDown   EventKind = "down"
	EventUp     EventKind = "up"
)

// Buttons of the pointer.
const (
	ButtonLeft   = "left"
	ButtonRight  = "right"
	ButtonMiddle = "middle"
)

// PointerEvent is a position or a button edge, in active milliseconds since the origin of the recording and in
// pixels of the captured source.
type PointerEvent struct {
	TMS    int64     `json:"t_ms"`
	Kind   EventKind `json:"kind"`
	Button string    `json:"button,omitempty"`
	X      int       `json:"x"`
	Y      int       `json:"y"`
}

// EventsQuery selects events of a file.
type EventsQuery struct {
	FromMS int64
	// Clicks keeps only the button edges, which is what a zoom proposal reads.
	Clicks bool
	Limit  int
}

// EventsResult is the part of the file a query selected.
type EventsResult struct {
	Width, Height int
	// Total is how many events of the kind asked for the file holds, from FromMS on.
	Total     int
	Events    []PointerEvent
	Truncated bool
	// Discarded counts lines that failed validation; a file the helper wrote has none.
	Discarded int
}

// ErrEvents is returned for a file that cannot be read as an event file at all.
var ErrEvents = errors.New("recording: invalid event file")

// NormalizeQuery fills the default limit and refuses a value out of its bounds.
func NormalizeQuery(q EventsQuery) (EventsQuery, error) {
	if q.Limit == 0 {
		q.Limit = DefaultEventsLimit
	}

	if q.Limit < 1 || q.Limit > MaxEventsLimit {
		return q, fmt.Errorf("limit must be 1 to %d", MaxEventsLimit)
	}

	if q.FromMS < 0 {
		return q, errors.New("from_ms must not be negative")
	}

	return q, nil
}

// rawEvent is a line as it is decoded; every member is a pointer so a missing one is told from a zero. Unknown
// members make the decode fail, so a line that carries anything else is never accepted.
type rawEvent struct {
	K      string `json:"k"`
	V      *int   `json:"v"`
	Width  *int   `json:"width"`
	Height *int   `json:"height"`
	Clock  string `json:"clock"`
	T      *int64 `json:"t"`
	B      string `json:"b"`
	X      *int   `json:"x"`
	Y      *int   `json:"y"`
}

// header is a decoded first line.
type header struct{ width, height int }

var errLine = errors.New("recording: invalid event line")

// inRange reports whether a coordinate is a pixel of a side of `size` pixels: 0 to size-1.
func inRange(v int, size int) bool { return v >= 0 && v < size }

// parseEvent decodes and validates one line. For a header line it returns the header and no event.
func parseEvent(line []byte, hdr *header) (ev PointerEvent, h *header, err error) {
	if len(line) > MaxEventLine {
		return ev, nil, errLine
	}

	var raw rawEvent

	dec := json.NewDecoder(bytes.NewReader(line))
	dec.DisallowUnknownFields()

	if derr := dec.Decode(&raw); derr != nil {
		return ev, nil, errLine
	}

	if dec.More() {
		return ev, nil, errLine
	}

	switch EventKind(raw.K) {
	case EventHeader:
		if raw.V == nil || *raw.V != 1 || raw.Width == nil || raw.Height == nil || raw.Clock != "active_ms" ||
			raw.T != nil || raw.X != nil || raw.Y != nil || raw.B != "" ||
			*raw.Width < 1 || *raw.Width > MaxSide || *raw.Height < 1 || *raw.Height > MaxSide {
			return ev, nil, errLine
		}

		return ev, &header{*raw.Width, *raw.Height}, nil
	case EventMove, EventDown, EventUp:
		if raw.T == nil || raw.X == nil || raw.Y == nil || raw.V != nil || raw.Width != nil ||
			raw.Height != nil || raw.Clock != "" || *raw.T < 0 || *raw.T > int64(MaxSecondsCeiling)*1000 {
			return ev, nil, errLine
		}

		maxX, maxY := MaxSide, MaxSide
		if hdr != nil {
			maxX, maxY = hdr.width, hdr.height
		}

		if !inRange(*raw.X, maxX) || !inRange(*raw.Y, maxY) {
			return ev, nil, errLine
		}

		isButton := EventKind(raw.K) != EventMove
		validButton := raw.B == ButtonLeft || raw.B == ButtonRight || raw.B == ButtonMiddle

		if isButton != (raw.B != "") || (isButton && !validButton) {
			return ev, nil, errLine
		}

		return PointerEvent{TMS: *raw.T, Kind: EventKind(raw.K), Button: raw.B, X: *raw.X, Y: *raw.Y}, nil, nil
	}

	return ev, nil, errLine
}

// ReadEvents reads an event file and returns what the query selects. The header must be the first line; a file
// without one is ErrEvents. A line that fails validation is discarded and counted, never returned.
func ReadEvents(r io.Reader, q EventsQuery) (EventsResult, error) {
	q, err := NormalizeQuery(q)
	if err != nil {
		return EventsResult{}, fmt.Errorf("%w: %w", ErrEvents, err)
	}

	sc := bufio.NewScanner(r)
	// The capacity is the bound too: a longer line is an error, not a token.
	sc.Buffer(make([]byte, 0, MaxEventLine+1), MaxEventLine+1)

	var (
		res   EventsResult
		hdr   *header
		lines int
		last  int64
	)

	for sc.Scan() {
		lines++
		if lines > MaxEventLines {
			return res, fmt.Errorf("%w: more than %d lines", ErrEvents, MaxEventLines)
		}

		ev, h, perr := parseEvent(sc.Bytes(), hdr)

		switch {
		case lines == 1:
			if perr != nil || h == nil {
				return res, fmt.Errorf("%w: no header", ErrEvents)
			}

			hdr = h
			res.Width, res.Height = h.width, h.height

			continue
		case perr != nil || h != nil || ev.TMS < last:
			res.Discarded++

			continue
		}

		last = ev.TMS

		if ev.TMS < q.FromMS || (q.Clicks && ev.Kind == EventMove) {
			continue
		}

		res.Total++

		if len(res.Events) < q.Limit {
			res.Events = append(res.Events, ev)
		} else {
			res.Truncated = true
		}
	}

	if serr := sc.Err(); serr != nil {
		// A line longer than the bound: the rest of the file cannot be trusted.
		return res, fmt.Errorf("%w: %w", ErrEvents, serr)
	}

	if lines == 0 {
		return res, fmt.Errorf("%w: empty", ErrEvents)
	}

	return res, nil
}
