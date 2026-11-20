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

// VV-157: a browser that opens a page of the local editor with no session gets a
// 401 page that says how to get in, and the JSON answer carries the same hint;
// neither carries the token or a launch code.
func TestUnauthenticatedPageSaysHowToSignIn(t *testing.T) {
	h := pagesHarness(t)

	page := h.do(req{path: "/projects", noAuth: true, headers: map[string]string{"Accept": "text/html"}})
	body := string(page.body)

	if page.status != http.StatusUnauthorized || !strings.Contains(page.header.Get("Content-Type"), "text/html") ||
		!strings.Contains(body, "stuarty open") || !strings.Contains(body, "https://stuarty.com/docs/") {
		t.Errorf("/projects as a browser = %d %s\n%.600s", page.status, page.header.Get("Content-Type"), body)
	}

	if strings.Contains(body, canary) || strings.Contains(body, "/auth/launch") {
		t.Errorf("the 401 page reveals a credential: %.600s", body)
	}

	api := h.do(req{path: "/api/v1/projects", noAuth: true})
	if api.status != http.StatusUnauthorized || !strings.Contains(string(api.body), "stuarty open") {
		t.Errorf("/api/v1/projects = %d %s, want the hint", api.status, api.body)
	}
}
