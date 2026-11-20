package mcp

import (
	"testing"

	"github.com/AlexGalhardo/stuarty/web/contract/testenv"
)

// view with mode motion is the brief of a range (SDD-011 section 15.5): the structured content has the members
// the output schema of view requires, the frames are files and not image blocks, and an argument of another mode
// is refused.
func TestMotionBriefThroughView(t *testing.T) {
	t.Parallel()

	env := testenv.New(t, testenv.Options{Frames: true})
	cs := connect(t, env.Service, Options{}, "claude-code")
	project := textProject(t, cs) // one text item of 1 s at 30 fps

	res := ok(
		t,
		call(t, cs, "view", map[string]any{"project_id": project, "mode": "motion", "from": "0.2s", "until": "0.8s"}),
	)
	if len(imagesOf(res)) != 0 {
		t.Error("the brief carries no image block: its frames are files")
	}

	sc := structured(t, res)
	if sc["width"] != float64(1080) || sc["height"] != float64(1920) || sc["duration_frames"] != float64(18) ||
		sc["revision"] != float64(2) || sc["needs_alpha"] != false {
		t.Errorf("structured content = %v", sc)
	}

	frames, _ := sc["frames"].([]any)
	if len(frames) != 2 || sc["image_tokens"].(float64) < 1 {
		t.Errorf("frames %v, image tokens %v", sc["frames"], sc["image_tokens"])
	}

	for _, name := range []string{"at", "times", "count"} {
		args := map[string]any{"project_id": project, "mode": "motion", "until": "1s"}
		args[name] = map[string]any{"at": "0s", "times": []any{"0s", "1s"}, "count": 3}[name]

		if r := call(t, cs, "view", args); !r.IsError {
			t.Errorf("%s is accepted in mode motion", name)
		}
	}
}
