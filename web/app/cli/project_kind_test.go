package cli

import "testing"

// The kind and the workflow of a project on the command line (epic 78): project:create --kind, the workflow in
// project:show, and project:step to skip an optional step and to put it back.
func TestProjectKindAndWorkflowOnTheCommandLine(t *testing.T) {
	r := newRealRuntime(t)

	code, created := r.do(
		"project:create", "--name", "Blog post", "--description", ctxDescription, "--specific", ctxSpecific,
		"--measurable", ctxMeasurable, "--achievable", ctxAchievable, "--relevant", ctxRelevant,
		"--time-bound", ctxTimeBound, "--due", ctxDue, "--kind", "text",
	)
	if code != 0 || created["kind"] != "text" {
		t.Fatalf("project:create --kind text: exit %d, %v", code, created)
	}

	id, _ := created["project_id"].(string)

	code, shown := r.do("project:show", id)
	if code != 0 || shown["kind"] != "text" {
		t.Fatalf("project:show: exit %d, %v", code, shown)
	}

	workflow, _ := shown["workflow"].(map[string]any)
	steps, _ := workflow["steps"].([]any)

	if len(steps) != 5 || workflow["current"] != "briefing" || workflow["next"] != "writing" {
		t.Fatalf("the workflow of a text project: %v", workflow)
	}

	stateOf := func(w map[string]any, id string) any {
		list, _ := w["steps"].([]any)
		for _, s := range list {
			if st, _ := s.(map[string]any); st["id"] == id {
				return st["state"]
			}
		}

		return nil
	}

	// The publication of a copy is optional: it is skipped, shown skipped, and put back.
	code, skipped := r.do("project:step", id, "publication")
	if code != 0 || skipped["changed"] != true {
		t.Fatalf("project:step: exit %d, %v", code, skipped)
	}

	if w, _ := skipped["workflow"].(map[string]any); stateOf(w, "publication") != "skipped" {
		t.Fatalf("after the skip: %v", skipped["workflow"])
	}

	code, restored := r.do("project:step", id, "publication", "--restore")
	if code != 0 || restored["changed"] != true {
		t.Fatalf("project:step --restore: exit %d, %v", code, restored)
	}

	// A required step is refused, with the field named.
	code, refused := r.do("project:step", id, "briefing")
	if code == 0 {
		t.Fatalf("a required step was skipped: %v", refused)
	}

	if refused["code"] != "E_VALIDATION" || refused["path"] != "step" {
		t.Fatalf("the refusal = %v", refused)
	}

	// An unknown kind is refused before anything is made.
	code, bad := r.do(
		"project:create", "--name", "Odd kind", "--description", ctxDescription, "--specific", ctxSpecific,
		"--measurable", ctxMeasurable, "--achievable", ctxAchievable, "--relevant", ctxRelevant,
		"--time-bound", ctxTimeBound, "--due", ctxDue, "--kind", "audio",
	)
	if code == 0 || bad["code"] != "E_VALIDATION" || bad["path"] != "kind" {
		t.Fatalf("project:create --kind audio: exit %d, %v", code, bad)
	}

	// The list says the kind of each project.
	code, listed := r.do("project:list")
	if code != 0 {
		t.Fatalf("project:list: exit %d", code)
	}

	if p, _ := listed["projects"].([]any); len(p) != 1 || p[0].(map[string]any)["kind"] != "text" {
		t.Fatalf("project:list: %v", listed)
	}

	// One step, no more: the positional count is checked.
	if code, _ := r.do("project:step", id); code == 0 {
		t.Fatal("project:step without a step succeeded")
	}
}
