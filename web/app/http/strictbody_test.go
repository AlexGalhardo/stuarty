package httpapi_test

import (
	"net/http"
	"strings"
	"testing"
)

// VV-302: an unknown member of a JSON body is E_VALIDATION with its path, as SDD-003 section 1 item 7
// says and the MCP server already does; a typo of dryrun never applies the edit.
func TestUnknownBodyMembersAreRefused(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	id := h.newProject("a")

	cases := map[string]struct{ path, body, member string }{
		"edit dryrun typo": {
			"/api/v1/projects/" + id + "/edit",
			`{"base_revision":0,"dryrun":true,"operations":[{"op":"track.add","args":{"kind":"audio"}}]}`, "dryrun",
		},
		"create": {"/api/v1/projects", `{"name":"X","bogus_field":1}`, "bogus_field"},
		"nested operation": {
			"/api/v1/projects/" + id + "/edit",
			`{"base_revision":0,"operations":[{"op":"track.add","arg":{}}]}`,
			"operations[0].arg",
		},
		"text kind": {"/api/v1/projects/" + id + "/texts", `{"kind":"post","title":"S","text":"a b"}`, "kind"},
		"undo":      {"/api/v1/projects/" + id + "/undo", `{"dryrun":true}`, "dryrun"},
	}

	for name, tc := range cases {
		res := h.post(tc.path, tc.body)
		if res.status != http.StatusBadRequest && res.status != http.StatusUnprocessableEntity {
			t.Errorf("%s: status %d %s, want a validation error", name, res.status, res.body)
			continue
		}

		j := res.json(t)
		if j["code"] != "E_VALIDATION" || j["path"] != tc.member || !strings.Contains(string(res.body), tc.member) {
			t.Errorf("%s: %s, want E_VALIDATION with path %q", name, res.body, tc.member)
		}
	}

	// a wrongly named price field must not remove the price
	put := h.do(req{method: http.MethodPut, path: "/api/v1/providers/prices", body: `{"price_in_micros":1}`})
	if put.status != http.StatusBadRequest && put.status != http.StatusUnprocessableEntity {
		t.Errorf("prices with wrong names = %d %s", put.status, put.body)
	}

	// the typo must not have applied anything
	hist := h.get("/api/v1/projects/" + id + "/history")
	if strings.Contains(string(hist.body), "track.add") {
		t.Errorf("the refused edit was applied: %s", hist.body)
	}

	// query parameters stay free, and known members still work
	if res := h.post("/api/v1/projects?unknown=1", `{"name":"ok"}`); res.status != http.StatusCreated {
		t.Errorf("create with a query extra = %d %s", res.status, res.body)
	}
}
