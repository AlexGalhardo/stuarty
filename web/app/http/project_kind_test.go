package httpapi_test

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/AlexGalhardo/stuarty/service/testproject"
)

func kindProjectJSON(name, kind string) string {
	raw, err := json.Marshal(map[string]any{
		"name": testproject.ValidName(name), "description": testproject.Description, "goal": testproject.Goal,
		"kind": kind,
	})
	if err != nil {
		panic(err)
	}

	return string(raw)
}

// The kind and the workflow over HTTP (epic 78): POST /projects takes the kind, GET /projects/{id} returns the
// workflow, the list returns the kind, and POST /projects/{id}/steps/{step}/skip skips and restores a step.
func TestProjectKindAndWorkflowOverHTTP(t *testing.T) {
	t.Parallel()

	h := newHarness(t)

	created := h.post("/api/v1/projects", kindProjectJSON("Blog post", "text"))
	if created.status != http.StatusCreated || created.json(t)["kind"] != "text" {
		t.Fatalf("create = %d %s", created.status, created.body)
	}

	id, _ := created.json(t)["project_id"].(string)

	got := h.get("/api/v1/projects/" + id)
	if got.status != http.StatusOK {
		t.Fatalf("get = %d %s", got.status, got.body)
	}

	doc := got.json(t)
	workflow, _ := doc["workflow"].(map[string]any)
	steps, _ := workflow["steps"].([]any)

	if doc["kind"] != "text" || workflow["kind"] != "text" || len(steps) != 5 || workflow["current"] != "briefing" {
		t.Fatalf("workflow = %v", doc["workflow"])
	}

	for _, s := range steps {
		st, _ := s.(map[string]any)
		if st["actor"] == "" || st["tool"] == "" || st["state"] == "" {
			t.Errorf("a step names no actor, tool or state: %v", st)
		}
	}

	if bad := h.post("/api/v1/projects", kindProjectJSON("Odd kind", "audio")); bad.status != http.StatusBadRequest ||
		bad.json(t)["path"] != "kind" {
		t.Fatalf("an unknown kind = %d %s", bad.status, bad.body)
	}

	skip := "/api/v1/projects/" + id + "/steps/publication/skip"

	if res := h.post(skip, `{}`); res.status != http.StatusOK || res.json(t)["changed"] != true {
		t.Fatalf("skip = %d %s", res.status, res.body)
	}

	if res := h.post(skip, `{"restore": true}`); res.status != http.StatusOK || res.json(t)["changed"] != true {
		t.Fatalf("restore = %d %s", res.status, res.body)
	}

	if res := h.post(skip+"?dry_run=true", `{}`); res.status != http.StatusOK || res.json(t)["changed"] != false {
		t.Fatalf("dry run = %d %s", res.status, res.body)
	}

	required := h.post("/api/v1/projects/"+id+"/steps/briefing/skip", `{}`)
	if required.status != http.StatusBadRequest || required.json(t)["code"] != "E_VALIDATION" ||
		required.json(t)["path"] != "step" {
		t.Fatalf("a required step = %d %s", required.status, required.body)
	}

	list := h.get("/api/v1/projects")
	items, _ := list.json(t)["projects"].([]any)

	if len(items) != 1 || items[0].(map[string]any)["kind"] != "text" {
		t.Fatalf("list = %s", list.body)
	}
}
