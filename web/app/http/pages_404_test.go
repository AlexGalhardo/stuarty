package httpapi_test

import (
	"net/http"
	"strings"
	"testing"
)

// VV-068: a page that does not exist, and a project in the trash, answer the
// Inertia error page (layout, message and a way back), not a bare status line;
// the API keeps its problem+json answer.
func TestMissingPagesAnswerTheErrorPage(t *testing.T) {
	h := pagesHarness(t)
	ck := cookie(h)

	trashed := h.newProject("Gone")
	if res := h.post("/api/v1/projects/"+trashed+"/trash", ""); res.status != http.StatusOK {
		t.Fatalf("trash = %d %s", res.status, res.body)
	}

	html := map[string]string{"Cookie": ck, "Accept": "text/html"}

	for path, want := range map[string]string{
		"/nope":                  `"status":404`,
		"/editor/xyz":            `"status":404`,
		"/editor/prj_0000000000": `"status":404`,
		"/editor/" + trashed:     `"message":"This project is in the trash."`,
	} {
		res := h.do(req{path: path, noAuth: true, headers: html})
		if res.status != http.StatusNotFound || !strings.Contains(string(res.body), `"component":"Error"`) ||
			!strings.Contains(string(res.body), want) {
			t.Errorf("%s = %d, want 404 with the error page and %s\n%.400s", path, res.status, want, res.body)
		}
	}

	// A route that does not match runs no middleware, yet the page must read in
	// words, so it carries the catalogue of the default locale.
	nope := h.do(req{path: "/nope", noAuth: true, headers: html})
	if !strings.Contains(string(nope.body), `"ui.error.back"`) {
		t.Errorf("the 404 page carries no catalogue: %.600s", nope.body)
	}

	api := h.get("/api/v1/nothing-here")
	if api.status != http.StatusNotFound || api.json(t)["code"] != "E_NOT_FOUND" {
		t.Errorf("an unknown API route = %d %s, want problem+json", api.status, api.body)
	}
}
