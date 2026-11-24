package transcript

import "math"

// Silence is a silent stretch of the asset on its own time axis, in frames, half-open.
type Silence struct{ Start, End int }

// EdgeKind says what the snap did to one edge of a cut.
type EdgeKind string

const (
	// EdgeInside means the edge was already in a silence and stays.
	EdgeInside EdgeKind = "inside"
	// EdgeMoved means the edge moved to the nearest silence.
	EdgeMoved EdgeKind = "moved"
	// EdgeNone means no silence qualified and the edge stays at the word time.
	EdgeNone EdgeKind = "none"
)

// Edge is the final position of one edge of the cut and what happened to it.
type Edge struct {
	Frame int      // the edge in asset frames
	Kind  EdgeKind // what the snap did
	Moved int      // frames added to the cut at this edge; 0 unless Kind is EdgeMoved
}

// Snapped is the cut [Start.Frame, End.Frame) in asset frames. It always contains the words the person chose.
type Snapped struct{ Start, End Edge }

// SnapEdges moves the edges of the cut of words[from..to] to the nearest silence within window frames (SDD-023
// section 3.1). It is pure and deterministic. The start edge only moves earlier, to the end of a silence that
// lies between the previous word and the first chosen word; the end edge only moves later, to the start of a
// silence between the last chosen word and the next one. So the result contains the chosen words and never takes
// speech the transcript shows from a neighbour; a silence the transcript contradicts is not trusted. Silences must
// be sorted and not overlapping. from and to must index words with from <= to; otherwise the result is empty.
func SnapEdges(silences []Silence, window int, words []Word, from, to int) Snapped {
	if from < 0 || to < from || to >= len(words) {
		return Snapped{}
	}
	start, end := words[from].Start, words[to].End
	lo, hi := 0, math.MaxInt
	if prev := from - 1; prev >= 0 && prev < len(words) {
		lo = words[prev].End
	}
	if next := to + 1; next < len(words) {
		hi = words[next].Start
	}
	return Snapped{Start: snapStart(silences, window, start, lo), End: snapEnd(silences, window, end, hi)}
}

func inside(silences []Silence, edge int) bool {
	for _, s := range silences {
		if s.Start <= edge && edge <= s.End {
			return true
		}
	}
	return false
}

func snapStart(silences []Silence, window, edge, lo int) Edge {
	if inside(silences, edge) {
		return Edge{Frame: edge, Kind: EdgeInside}
	}
	best := -1
	for _, s := range silences {
		if s.End >= lo && s.End < edge && edge-s.End <= window {
			best = max(best, s.End)
		}
	}
	if best < 0 {
		return Edge{Frame: edge, Kind: EdgeNone}
	}
	return Edge{Frame: best, Kind: EdgeMoved, Moved: edge - best}
}

func snapEnd(silences []Silence, window, edge, hi int) Edge {
	if inside(silences, edge) {
		return Edge{Frame: edge, Kind: EdgeInside}
	}
	best := math.MaxInt
	for _, s := range silences {
		if s.Start > edge && s.Start <= hi && s.Start-edge <= window {
			best = min(best, s.Start)
		}
	}
	if best == math.MaxInt {
		return Edge{Frame: edge, Kind: EdgeNone}
	}
	return Edge{Frame: best, Kind: EdgeMoved, Moved: best - edge}
}
