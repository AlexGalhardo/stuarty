package mcp

import (
	"strings"
	"testing"
)

// VV-289: the briefing of project_item takes the kind of production as production_kind, and kind stays the kind
// of an item, so neither is read as the other: a kind of production sent as kind, or production_kind sent to an
// action that is not the briefing, is refused instead of falling back to the short video.
func TestBriefingTakesTheKindOfProductionApartFromTheKindOfAnItem(t *testing.T) {
	t.Parallel()

	cs := connect(t, newService(t), Options{}, "claude-code")
	project := createProject(t, cs, "Ad")

	questions := func(res map[string]any) string {
		list, _ := res["questions"].([]any)
		if len(list) == 0 {
			t.Fatalf("no questions: %v", res)
		}

		first, _ := list[0].(map[string]any)
		id, _ := first["id"].(string)

		return id
	}

	short := structured(t, ok(t, call(t, cs, "project_item", map[string]any{
		"action": "briefing", "project_id": project,
	})))
	if got := questions(short); got == "product" {
		t.Errorf("with no production_kind the briefing is the short video, got the first question %q", got)
	}

	ad := structured(t, ok(t, call(t, cs, "project_item", map[string]any{
		"action": "briefing", "project_id": project, "production_kind": "commercial",
	})))
	if got := questions(ad); got != "product" {
		t.Errorf("the commercial briefing starts with %q, want product", got)
	}

	for _, tc := range []struct {
		args map[string]any
		path string
	}{
		{map[string]any{"action": "briefing", "project_id": project, "kind": "commercial"}, "kind"},
		{map[string]any{"action": "briefing", "project_id": project, "kind": "text"}, "kind"},
		{map[string]any{"action": "add", "project_id": project, "kind": "text", "production_kind": "long"}, "production_kind"},
		{map[string]any{"action": "briefing", "project_id": project, "production_kind": "podcast"}, "production_kind"},
	} {
		res := call(t, cs, "project_item", tc.args)
		if !res.IsError {
			t.Errorf("%v was accepted", tc.args)

			continue
		}

		if text := textOf(t, res); !strings.Contains(text, "E_VALIDATION") || !strings.Contains(text, tc.path) {
			t.Errorf("%v: %s, want E_VALIDATION on %s", tc.args, text, tc.path)
		}
	}
}
