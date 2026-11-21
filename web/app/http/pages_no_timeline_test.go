package httpapi_test

import (
	"net/http"
	"testing"
)

// A project with no timeline has no document for the editor to open: its address leads to the page of the project,
// where a video is added, and not to an editor that can only fail.
func TestEditorOfAProjectWithNoTimelineOpensItsPage(t *testing.T) {
	h := pagesHarness(t)
	ck := cookie(h)

	made := h.post("/api/v1/projects", projectJSON("No timeline"))
	if made.status != http.StatusCreated {
		t.Fatalf("create = %d %s", made.status, made.body)
	}

	id, _ := made.json(t)["project_id"].(string)

	res := h.do(req{path: "/editor/" + id, noAuth: true, headers: map[string]string{"Cookie": ck}})
	if res.status < 300 || res.status >= 400 || res.header.Get("Location") != "/projects/"+id {
		t.Fatalf("/editor = %d, location %q, want a redirect to the project", res.status, res.header.Get("Location"))
	}
}
