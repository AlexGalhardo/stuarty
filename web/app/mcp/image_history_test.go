package mcp

import (
	"strings"
	"testing"

	"github.com/AlexGalhardo/stuarty/core/command"
)

// TestImageHistoryRevertDiffAndCheckpoints is the flow of epic 50 on the tool with domain image: name the state,
// let an agent work, read what changed, preview and go back, and take the revert back with undo.
func TestImageHistoryRevertDiffAndCheckpoints(t *testing.T) {
	t.Parallel()

	cs := connect(t, newService(t), Options{}, "claude-code")
	img, _ := structured(t, ok(t, call(t, cs, "image", map[string]any{
		"action": "create", "preset": "video_thumbnail", "name": "Cover",
	})))["image_id"].(string)

	history := func(args map[string]any) map[string]any {
		t.Helper()

		args["domain"], args["image_id"] = "image", img

		return structured(t, ok(t, call(t, cs, "history", args)))
	}

	ok(t, call(t, cs, "edit", map[string]any{
		"domain": "image", "image_id": img, "base_revision": 1,
		"operations": []any{map[string]any{"op": "layer.add_text", "args": map[string]any{"text": "HOW IT WORKS"}}},
	}))

	// r1 is the blank image, r2 the text. Name r2, then let the agent change it.
	if sc := history(
		map[string]any{"action": "checkpoint", "name": "good title", "note": "keep"},
	); sc["name"] != "good title" ||
		sc["revision"] != float64(2) {
		t.Fatalf("checkpoint: %v", sc)
	}

	ok(t, call(t, cs, "edit", map[string]any{
		"domain":        "image",
		"image_id":      img,
		"base_revision": 2,
		"operations": []any{
			map[string]any{"op": "layer.add_shape", "args": map[string]any{"shape": "rect", "w": 50, "h": 50}},
		},
	}))

	if cps, _ := history(map[string]any{"action": "checkpoints"})["checkpoints"].([]any); len(cps) != 2 {
		t.Fatalf("checkpoints = %v (the named one and the automatic one)", cps)
	}

	diff := ok(t, call(t, cs, "history", map[string]any{
		"domain": "image", "image_id": img, "action": "diff", "name": "good title",
	}))
	if sc := structured(t, diff); sc["count"] != float64(1) || sc["from"] != float64(2) || sc["to"] != float64(3) {
		t.Fatalf("diff: %v", sc)
	}

	if text := textOf(
		t,
		diff,
	); !strings.Contains(text, "r2 -> r3 (1 changes)") ||
		!strings.Contains(text, "added shape") {
		t.Errorf("diff text:\n%s", text)
	}

	if sc := history(
		map[string]any{"action": "revert", "name": "good title", "dry_run": true},
	); sc["applied"] != false ||
		sc["dry_run"] != true ||
		sc["kind"] != "revert" {
		t.Fatalf("dry run: %v", sc)
	}

	if sc := history(map[string]any{"action": "revert", "revision": 2}); sc["applied"] != true ||
		sc["revision"] != float64(4) || sc["target"] != float64(2) {
		t.Fatalf("revert: %v", sc)
	}

	if sc := history(map[string]any{"action": "undo"}); sc["revision"] != float64(5) || sc["kind"] != "undo" {
		t.Fatalf("undo of the revert: %v", sc)
	}

	if sc := history(
		map[string]any{"action": "rename_checkpoint", "name": "good title", "new_name": "title"},
	); sc["name"] != "title" {
		t.Fatalf("rename: %v", sc)
	}

	if sc := history(map[string]any{"action": "remove_checkpoint", "name": "title"}); sc["name"] != "title" {
		t.Fatalf("remove: %v", sc)
	}
}

// A member the action does not use is refused in the image domain, and a dry_run that an undo ignored would undo
// for real.
func TestImageHistoryRefusesWhatTheActionDoesNotUse(t *testing.T) {
	t.Parallel()

	cs := connect(t, newService(t), Options{}, "claude-code")
	img, _ := structured(t, ok(t, call(t, cs, "image", map[string]any{
		"action": "create", "preset": "video_thumbnail",
	})))["image_id"].(string)

	for name, tc := range map[string]struct {
		args map[string]any
		path string
	}{
		"undo with dry_run":        {map[string]any{"action": "undo", "dry_run": true}, "dry_run"},
		"redo with a revision":     {map[string]any{"action": "redo", "revision": 1}, "revision"},
		"revert with until":        {map[string]any{"action": "revert", "revision": 1, "until": 2}, "until"},
		"diff with dry_run":        {map[string]any{"action": "diff", "revision": 1, "dry_run": true}, "dry_run"},
		"checkpoints with a name":  {map[string]any{"action": "checkpoints", "name": "x"}, "name"},
		"checkpoint with new_name": {map[string]any{"action": "checkpoint", "name": "x", "new_name": "y"}, "new_name"},
		"revert with project_id":   {map[string]any{"action": "revert", "revision": 1, "project_id": "prj_7k2m9x4q1b"}, "project_id"},
		"list":                     {map[string]any{"action": "list"}, "action"},
	} {
		tc.args["domain"], tc.args["image_id"] = "image", img
		res := call(t, cs, "history", tc.args)
		text := textOf(t, res)

		if !res.IsError || !strings.Contains(text, "error "+command.CodeValidation+":") ||
			!strings.Contains(text, `"path":"`+tc.path+`"`) {
			t.Errorf("%s: %q", name, text)
		}
	}
}
