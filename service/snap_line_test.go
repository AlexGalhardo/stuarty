package service

import (
	"testing"

	"github.com/AlexGalhardo/stuarty/core/command"
)

func TestSnapLineSaysWhereEachEdgeWent(t *testing.T) {
	edge := func(result string, moved int) map[string]any {
		return map[string]any{"edge": 0, "result": result, "moved": moved}
	}
	cases := []struct {
		name string
		eff  map[string]any
		want string
	}{
		{"no snap", map[string]any{}, ""},
		{
			"one frame and several",
			map[string]any{"snap": map[string]any{
				"analysis": command.SilenceRead, "start": edge("moved", 1), "end": edge("moved", 3),
			}},
			"Cut edges: start edge moved 1 frame into a silence; end edge moved 3 frames into a silence.",
		},
		{
			"inside and none",
			map[string]any{"snap": map[string]any{
				"analysis": command.SilenceRead, "start": edge("inside", 0), "end": edge("none", 0),
			}},
			"Cut edges: start edge was already in a silence; end edge found no silence nearby.",
		},
		{
			"no analysis",
			map[string]any{"snap": map[string]any{
				"analysis": command.SilenceMissing, "start": edge("none", 0), "end": edge("none", 0),
			}},
			"Cut edges not snapped: no silence analysis was available, so the cut follows the word times.",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := snapLine(c.eff); got != c.want {
				t.Errorf("snapLine = %q, want %q", got, c.want)
			}
		})
	}
}
