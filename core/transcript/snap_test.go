package transcript

import "testing"

func TestSnapEdges(t *testing.T) {
	// 30 fps: "a" [10,20), "um" [30,40), "b" [50,60), "c" [60,70) contiguous with b.
	words := []Word{{"a", 10, 20}, {"um", 30, 40}, {"b", 50, 60}, {"c", 60, 70}}
	cases := []struct {
		name       string
		silences   []Silence
		window     int
		from, to   int
		start, end Edge
	}{
		{
			name:     "edges inside silences stay",
			silences: []Silence{{22, 32}, {38, 48}},
			window:   9, from: 1, to: 1,
			start: Edge{30, EdgeInside, 0}, end: Edge{40, EdgeInside, 0},
		},
		{
			name:     "an edge near a silence moves to it",
			silences: []Silence{{20, 27}, {43, 49}},
			window:   9, from: 1, to: 1,
			start: Edge{27, EdgeMoved, 3}, end: Edge{43, EdgeMoved, 3},
		},
		{
			name:     "no silence in the window",
			silences: []Silence{{20, 24}, {49, 50}},
			window:   5, from: 1, to: 1,
			start: Edge{30, EdgeNone, 0}, end: Edge{40, EdgeNone, 0},
		},
		{
			name:     "no silences at all",
			silences: nil,
			window:   9, from: 1, to: 1,
			start: Edge{30, EdgeNone, 0}, end: Edge{40, EdgeNone, 0},
		},
		{
			name:     "two candidates: the nearer wins",
			silences: []Silence{{20, 22}, {24, 27}, {42, 43}, {46, 48}},
			window:   9, from: 1, to: 1,
			start: Edge{27, EdgeMoved, 3}, end: Edge{42, EdgeMoved, 2},
		},
		{
			name:     "a silence beyond the window is ignored even when another is in it",
			silences: []Silence{{11, 14}, {20, 21}},
			window:   9, from: 1, to: 1,
			start: Edge{21, EdgeMoved, 9}, end: Edge{40, EdgeNone, 0},
		},
		{
			name: "never later than the first chosen word: a silence inside the word is ignored",
			// {33,36} lies inside the chosen word "um" [30,40): the start must not move later to reach it.
			silences: []Silence{{33, 36}},
			window:   9, from: 1, to: 1,
			start: Edge{30, EdgeNone, 0}, end: Edge{40, EdgeNone, 0},
		},
		{
			name: "never into the neighbour before: a silence inside the previous word is not trusted",
			// {12,16} lies inside "a" [10,20).
			silences: []Silence{{12, 16}},
			window:   30, from: 1, to: 1,
			start: Edge{30, EdgeNone, 0}, end: Edge{40, EdgeNone, 0},
		},
		{
			name: "a silence that overlaps the neighbour stops at the border of the chosen words",
			// {15,26} overlaps "a" [10,20) but ends in the gap: the edge goes to 26, not into "a".
			silences: []Silence{{15, 26}},
			window:   9, from: 1, to: 1,
			start: Edge{26, EdgeMoved, 4}, end: Edge{40, EdgeNone, 0},
		},
		{
			name: "never into the next word: a silence inside the next word is not trusted",
			// {52,56} lies inside "b" [50,60).
			silences: []Silence{{52, 56}},
			window:   30, from: 1, to: 1,
			start: Edge{30, EdgeNone, 0}, end: Edge{40, EdgeNone, 0},
		},
		{
			name: "a silence that overlaps the next word moves the end only to its own start",
			// {45,55} starts in the gap and overlaps "b" [50,60): the end edge goes to 45.
			silences: []Silence{{45, 55}},
			window:   9, from: 1, to: 1,
			start: Edge{30, EdgeNone, 0}, end: Edge{45, EdgeMoved, 5},
		},
		{
			name:     "contiguous words give an edge nothing to move through",
			silences: []Silence{{55, 58}},
			window:   30, from: 2, to: 2,
			start: Edge{50, EdgeNone, 0}, end: Edge{60, EdgeNone, 0},
		},
		{
			name:     "adjacent silences: the nearest start counts",
			silences: []Silence{{20, 24}, {24, 28}, {41, 44}, {44, 47}},
			window:   9, from: 1, to: 1,
			start: Edge{28, EdgeMoved, 2}, end: Edge{41, EdgeMoved, 1},
		},
		{
			name:     "the first word of the asset: the start edge may reach frame 0",
			silences: []Silence{{0, 6}},
			window:   9, from: 0, to: 0,
			start: Edge{6, EdgeMoved, 4}, end: Edge{20, EdgeNone, 0},
		},
		{
			name:     "the last word of the asset: nothing bounds the end edge but the window",
			silences: []Silence{{74, 90}},
			window:   9, from: 3, to: 3,
			start: Edge{60, EdgeNone, 0}, end: Edge{74, EdgeMoved, 4},
		},
		{
			name:     "a range of several words uses its outer edges",
			silences: []Silence{{20, 27}, {43, 49}},
			window:   9, from: 1, to: 2,
			start: Edge{27, EdgeMoved, 3}, end: Edge{60, EdgeNone, 0},
		},
		{
			name:     "an edge at the border of a silence is inside it",
			silences: []Silence{{20, 30}, {40, 45}},
			window:   9, from: 1, to: 1,
			start: Edge{30, EdgeInside, 0}, end: Edge{40, EdgeInside, 0},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := SnapEdges(c.silences, c.window, words, c.from, c.to)
			if got.Start != c.start || got.End != c.end {
				t.Errorf("SnapEdges = %+v, want start %+v end %+v", got, c.start, c.end)
			}
			if got.Start.Frame > words[c.from].Start || got.End.Frame < words[c.to].End {
				t.Errorf("the snapped cut %+v does not contain the chosen words", got)
			}
		})
	}
}

func TestSnapEdgesNeverShrinksTheCut(t *testing.T) {
	words := []Word{{"a", 0, 10}, {"b", 14, 30}, {"c", 33, 50}, {"d", 55, 60}}
	for from := range words {
		for to := from; to < len(words); to++ {
			for s0 := 0; s0 < 62; s0 += 3 {
				for l := 1; l < 12; l += 4 {
					sil := []Silence{{s0, s0 + l}}
					got := SnapEdges(sil, 20, words, from, to)
					if got.Start.Frame > words[from].Start || got.End.Frame < words[to].End {
						t.Fatalf("from %d to %d silence %v: %+v shrinks the cut", from, to, sil, got)
					}
					lo, hi := 0, 1<<30
					if from > 0 {
						lo = words[from-1].End
					}
					if to+1 < len(words) {
						hi = words[to+1].Start
					}
					if got.Start.Frame < lo || got.End.Frame > hi {
						t.Fatalf("from %d to %d silence %v: %+v reaches a neighbour", from, to, sil, got)
					}
				}
			}
		}
	}
}
