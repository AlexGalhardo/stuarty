package httpapi_test

import (
	"net/http"
	"testing"
)

// VV-306: the OpenTimelineIO export of a project with two timelines takes the timeline_id of the request, as every
// other per-timeline route does (SDD-003 section 1.7); without it the refusal still names the choices.
func TestOtioExportTakesTheTimelineOfAProductionWithTwo(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	project := h.newProject("two")

	added := h.post("/api/v1/projects/"+project+"/items", `{"kind":"timeline","title":"Wide","preset":"16:9"}`)
	if added.status != http.StatusCreated && added.status != http.StatusOK {
		t.Fatalf("second timeline = %d %s", added.status, added.body)
	}

	item, _ := added.json(t)["item"].(map[string]any)
	second, _ := item["timeline_id"].(string)

	if second == "" {
		t.Fatalf("no timeline id in %s", added.body)
	}

	if res := h.get("/api/v1/projects/" + project + "/otio"); res.status != http.StatusBadRequest {
		t.Errorf("without timeline_id = %d %s, want 400", res.status, res.body)
	}

	for _, id := range []string{project, second} {
		if res := h.get("/api/v1/projects/" + project + "/otio?timeline_id=" + id); res.status != http.StatusOK {
			t.Errorf("timeline_id=%s = %d %s, want 200", id, res.status, res.body)
		}
	}
}
