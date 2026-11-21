package mcp

import (
	"fmt"
	"strings"
	"testing"
)

// TestHistoryRevertDiffAndCheckpoints is the flow of epic 36 on the tool: name the state, let work happen, read
// what changed, preview and go back, and take the revert back with undo.
func TestHistoryRevertDiffAndCheckpoints(t *testing.T) {
	t.Parallel()

	cs := connect(t, newService(t), Options{}, "claude-code")
	project := createProject(t, cs, "Cut")

	track := ""
	base := 0

	for i, text := range []string{"", "one", "two", "three"} {
		op := map[string]any{"op": "track.add", "args": map[string]any{"kind": "visual"}}
		if i > 0 {
			op = map[string]any{"op": "text.add", "args": map[string]any{
				"track_id": track, "start": fmt.Sprintf("%ds", i*2), "duration": "1s", "text": text,
			}}
		}

		res := structured(t, ok(t, call(t, cs, "edit", map[string]any{
			"project_id": project, "base_revision": base, "operations": []any{op},
		})))

		if i == 0 {
			track, _ = res["created"].([]any)[0].(string)
		}

		base = int(res["revision"].(float64))
	}

	// r1 is the track, r2 to r4 the texts. Name r3, then the whole line of work after it can be undone in one step.
	made := ok(t, call(t, cs, "history", map[string]any{
		"project_id": project, "action": "checkpoint", "name": "two texts", "revision": 3, "note": "keep these",
	}))
	if sc := structured(t, made); sc["name"] != "two texts" || sc["revision"] != float64(3) {
		t.Fatalf("checkpoint: %v", sc)
	}

	if text := textOf(t, made); !strings.Contains(text, `created checkpoint "two texts" at r3`) {
		t.Errorf("checkpoint text: %s", text)
	}

	listed := ok(t, call(t, cs, "history", map[string]any{"project_id": project, "action": "checkpoints"}))
	if cps, _ := structured(t, listed)["checkpoints"].([]any); len(cps) != 2 {
		t.Fatalf("checkpoints = %v (the named one and the automatic one)", structured(t, listed))
	}

	diff := ok(t, call(t, cs, "history", map[string]any{"project_id": project, "action": "diff", "name": "two texts"}))
	if sc := structured(t, diff); sc["count"] != float64(1) || sc["from"] != float64(3) || sc["to"] != float64(4) {
		t.Fatalf("diff: %v", sc)
	}

	if text := textOf(t, diff); !strings.Contains(text, "r3 -> r4 (1 changes)") || !strings.Contains(text, `"three"`) {
		t.Errorf("diff text:\n%s", text)
	}

	dry := ok(t, call(t, cs, "history", map[string]any{
		"project_id": project, "action": "revert", "name": "two texts", "dry_run": true,
	}))
	if sc := structured(t, dry); sc["applied"] != false || sc["dry_run"] != true || sc["kind"] != "revert" {
		t.Fatalf("dry run: %v", sc)
	}

	done := ok(t, call(t, cs, "history", map[string]any{"project_id": project, "action": "revert", "revision": 3}))
	if sc := structured(t, done); sc["applied"] != true || sc["revision"] != float64(5) || sc["target"] != float64(3) {
		t.Fatalf("revert: %v", sc)
	}

	undone := ok(t, call(t, cs, "history", map[string]any{"project_id": project, "action": "undo"}))
	if sc := structured(t, undone); sc["revision"] != float64(6) || sc["kind"] != "undo" {
		t.Fatalf("undo of the revert: %v", sc)
	}

	back := ok(t, call(t, cs, "history", map[string]any{"project_id": project, "action": "diff", "revision": 4}))
	if sc := structured(t, back); sc["count"] != float64(0) {
		t.Fatalf("after revert and undo the document equals r4: %v", sc)
	}

	renamed := ok(t, call(t, cs, "history", map[string]any{
		"project_id": project, "action": "rename_checkpoint", "name": "two texts", "new_name": "intro",
	}))
	if sc := structured(t, renamed); sc["name"] != "intro" {
		t.Fatalf("rename: %v", sc)
	}

	removed := ok(
		t,
		call(t, cs, "history", map[string]any{"project_id": project, "action": "remove_checkpoint", "name": "intro"}),
	)
	if !strings.Contains(textOf(t, removed), `removed checkpoint "intro"`) {
		t.Errorf("remove text: %s", textOf(t, removed))
	}
}

// A member that a new action does not use is E_VALIDATION naming it, and the old actions stay as lenient as before.
func TestHistoryRefusesMembersOfOtherActions(t *testing.T) {
	t.Parallel()

	cs := connect(t, newService(t), Options{}, "claude-code")
	project := createProject(t, cs, "Cut")

	for name, args := range map[string]map[string]any{
		"revert with limit":        {"project_id": project, "action": "revert", "revision": 1, "limit": 3},
		"diff with dry_run":        {"project_id": project, "action": "diff", "revision": 1, "dry_run": true},
		"checkpoints with name":    {"project_id": project, "action": "checkpoints", "name": "x"},
		"checkpoint with new_name": {"project_id": project, "action": "checkpoint", "name": "x", "new_name": "y"},
		"remove with note":         {"project_id": project, "action": "remove_checkpoint", "name": "x", "note": "n"},
		"list with revision":       {"project_id": project, "revision": 1},
		"revert with neither":      {"project_id": project, "action": "revert"},
		"diff with neither":        {"project_id": project, "action": "diff"},
	} {
		res := call(t, cs, "history", args)
		if !res.IsError || !strings.Contains(textOf(t, res), "E_VALIDATION") {
			t.Errorf("%s: %s", name, textOf(t, res))
		}
	}
}

// The members epic 36 added are of the video domain: with domain=image they are refused before anything runs, so an
// undo sent with dry_run=true never undoes an image step for real.
func TestHistoryImageRefusesTheRevertMembers(t *testing.T) {
	t.Parallel()

	cs := connect(t, newService(t), Options{}, "claude-code")
	img, _ := structured(t, ok(t, call(t, cs, "image", map[string]any{
		"action": "create", "preset": "video_thumbnail", "name": "Cover",
	})))["image_id"].(string)

	ok(t, call(t, cs, "edit", map[string]any{
		"domain": "image", "image_id": img, "base_revision": 1,
		"operations": []any{map[string]any{"op": "layer.add_fill", "args": map[string]any{"color": "#203040"}}},
	}))

	for _, member := range []string{"dry_run", "revision", "until", "name", "new_name", "note"} {
		var value any = "x"

		switch member {
		case "dry_run":
			value = true
		case "revision", "until":
			value = 1
		}

		res := call(
			t,
			cs,
			"history",
			map[string]any{"domain": "image", "image_id": img, "action": "undo", member: value},
		)
		if text := textOf(t, res); !res.IsError || !strings.Contains(text, `"path":"`+member+`"`) {
			t.Errorf("history domain=image undo with %s: %s", member, text)
		}
	}

	shown := textOf(t, ok(t, call(t, cs, "image", map[string]any{"action": "show", "image_id": img})))
	if !strings.Contains(shown, "fill") {
		t.Errorf("a refused call undid the image step: %s", shown)
	}
}
