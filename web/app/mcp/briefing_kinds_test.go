package mcp

import "testing"

// VV-399: project_item with action briefing asks the questions of the kind of the project when production_kind is
// left out, and the set the caller names wins. The adapter adds no rule: the service chooses.
func TestBriefingFollowsTheKindOfTheProject(t *testing.T) {
	t.Parallel()

	cs := connect(t, newService(t), Options{}, "claude-code")

	for kind, want := range map[string]string{"text": "text", "image": "image", "video": "short"} {
		args := projectArgs("Brief " + kind)
		args["kind"] = kind

		id, _ := structured(t, ok(t, call(t, cs, "project_create", args)))["project_id"].(string)

		res := structured(t, ok(t, call(t, cs, "project_item", map[string]any{
			"action": "briefing", "project_id": id,
		})))
		if got, _ := res["production_kind"].(string); got != want {
			t.Errorf("a %s project asks the %q set, got %q", kind, want, got)
		}
	}

	args := projectArgs("Brief named")
	args["kind"] = "text"

	id, _ := structured(t, ok(t, call(t, cs, "project_create", args)))["project_id"].(string)

	res := structured(t, ok(t, call(t, cs, "project_item", map[string]any{
		"action": "briefing", "project_id": id, "production_kind": "image",
	})))
	if got, _ := res["production_kind"].(string); got != "image" {
		t.Errorf("a named set lost to the kind of the project: %q", got)
	}
}
