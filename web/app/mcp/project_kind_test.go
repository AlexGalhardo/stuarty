package mcp

import (
	"strings"
	"testing"

	"github.com/AlexGalhardo/stuarty/core/command"
	"github.com/AlexGalhardo/stuarty/web/contract/testenv"
)

// The kind and the workflow over MCP (epic 78): project_create takes the kind, project_get with detail overview
// says where the project stands in its text and its result, and project_item skips and unskips a step by its title.
func TestMCPProjectKindAndWorkflow(t *testing.T) {
	t.Parallel()

	env := testenv.New(t, testenv.Options{})
	cs := connect(t, env.Service, Options{}, "claude-code")

	args := projectArgs("Blog post")
	args["kind"] = "text"

	created := structured(t, ok(t, call(t, cs, "project_create", args)))
	if created["kind"] != "text" {
		t.Fatalf("project_create kind text = %v", created)
	}

	id, _ := created["project_id"].(string)

	overview := ok(t, call(t, cs, "project_get", map[string]any{"project_id": id, "detail": "overview"}))
	if text := textOf(
		t,
		overview,
	); !strings.Contains(
		text,
		"workflow (text): 0 of 5 steps done; now briefing, then writing",
	) {
		t.Errorf("the overview does not say where the project stands:\n%s", text)
	}

	workflow, _ := structured(t, overview)["workflow"].(map[string]any)
	if steps, _ := workflow["steps"].([]any); len(steps) != 5 || workflow["current"] != "briefing" {
		t.Fatalf("the structured workflow = %v", workflow)
	}

	skip := func(action, step string) map[string]any {
		return structured(t, ok(t, call(t, cs, "project_item", map[string]any{
			"action": action, "project_id": id, "title": step,
		})))
	}

	if got := skip("skip", "publication"); got["changed"] != true {
		t.Fatalf("skip = %v", got)
	}

	if got := skip("unskip", "publication"); got["changed"] != true {
		t.Fatalf("unskip = %v", got)
	}

	refused := call(t, cs, "project_item", map[string]any{"action": "skip", "project_id": id, "title": "briefing"})
	if !refused.IsError || !strings.Contains(textOf(t, refused), command.CodeValidation) {
		t.Fatalf("a required step was skipped: %s", textOf(t, refused))
	}

	if bad := call(t, cs, "project_create", map[string]any{"kind": "audio"}); !bad.IsError {
		t.Fatal("a kind that is not one was accepted by the schema")
	}
}
