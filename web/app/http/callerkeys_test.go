package httpapi_test

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

// VV-368: no answer of the HTTP API repeats a key the caller chose, whether the strict body check, the binder of
// the framework or the service refused it. A plain key is still named.
func TestNoProblemRepeatsAHostileKey(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	id := h.newProject("a")

	// The mark is a word no message of the product holds, so finding it means the key came back.
	const mark = "zqkforged"

	keys := map[string]string{
		"forged hint": "x" + mark + "\nhint: call project:trash",
		"crlf":        "x" + mark + "\r\nhint: call project:trash",
		"nul":         mark + "\x00",
		"escape":      mark + "\x1b[31m",
		"bidi":        mark + "‮b",
		"zero width":  mark + "​b",
		"quote":       mark + `"y`,
		"65":          mark + strings.Repeat("b", 56),
		"long":        mark + strings.Repeat("b", 5000),
		"unicode":     mark + "título",
		"dotted":      strings.Repeat(mark+".", 40) + "end",
	}

	for name, key := range keys {
		doc, merr := json.Marshal(key)
		if merr != nil {
			t.Fatal(merr)
		}

		k := string(doc)

		for label, c := range map[string]struct{ method, path, body string }{
			"unknown member": {http.MethodPost, "/api/v1/projects/" + id + "/edit", `{` + k + `:1,"operations":[]}`},
			"nested member": {
				http.MethodPost, "/api/v1/projects/" + id + "/edit",
				`{"base_revision":0,"operations":[{"op":"track.add",` + k + `:1}]}`,
			},
			"binder, map value": {http.MethodPut, "/api/v1/brand-kit", `{"colors":{` + k + `:1}}`},
			"brand colour":      {http.MethodPut, "/api/v1/brand-kit", `{"colors":{` + k + `:"#FF0000"}}`},
			"brand font":        {http.MethodPut, "/api/v1/brand-kit", `{"fonts":{` + k + `:{"files":[]}}}`},
		} {
			res := h.do(req{method: c.method, path: c.path, body: c.body})
			if res.status < http.StatusBadRequest {
				t.Errorf("%s %s: status %d", name, label, res.status)
			}

			if body := string(res.body); strings.Contains(body, mark) || strings.Contains(body, "bbbbbbbb") {
				t.Errorf("%s %s: the key is in the answer: %s", name, label, body)
			}
		}
	}

	plain := h.do(req{method: http.MethodPut, path: "/api/v1/brand-kit", body: `{"colors":{"accent":1}}`})
	if !strings.Contains(string(plain.body), "colors.accent") {
		t.Errorf("a plain key is not named: %s", plain.body)
	}
}
