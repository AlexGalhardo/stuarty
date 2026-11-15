package httpapi_test

import (
	"encoding/json"
	"net/http"
	"slices"
	"strings"
	"testing"

	httpapi "github.com/AlexGalhardo/vukem-video/web/app/http"
)

func (h *harness) openapi() map[string]any {
	h.t.Helper()

	res := h.get("/openapi.json")
	if res.status != http.StatusOK {
		h.t.Fatalf("openapi = %d %.300s", res.status, res.body)
	}

	return res.json(h.t)
}

// refs walks a JSON value and returns every $ref in it.
func refs(v any, out *[]string) {
	switch x := v.(type) {
	case map[string]any:
		for k, val := range x {
			if s, ok := val.(string); ok && k == "$ref" {
				*out = append(*out, s)
			} else {
				refs(val, out)
			}
		}
	case []any:
		for _, e := range x {
			refs(e, out)
		}
	}
}

func TestRoutesMatchOpenAPI(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	doc := h.openapi()

	if v, _ := doc["openapi"].(string); !strings.HasPrefix(v, "3.1") {
		t.Errorf("openapi = %q, want 3.1.x", v)
	}

	paths, _ := doc["paths"].(map[string]any)
	inDoc := map[string]string{} // "GET /api/v1/x" to operationId

	for path, item := range paths {
		for method, op := range item.(map[string]any) {
			id, _ := op.(map[string]any)["operationId"].(string)
			inDoc[strings.ToUpper(method)+" "+path] = id
		}
	}

	for _, r := range h.vapp.Router.Routes() {
		if !strings.HasPrefix(r.Path, httpapi.BasePath+"/") {
			continue
		}

		key := r.Method + " " + r.Path
		id, ok := inDoc[key]

		if !ok {
			t.Errorf("the router serves %s and the document does not list it", key)
			continue
		}

		if id == "" {
			t.Errorf("%s has no operationId", key)
		}

		delete(inDoc, key)
	}

	for key := range inDoc {
		t.Errorf("the document lists %s, which the router does not serve", key)
	}

	// Every $ref resolves.
	schemas := doc["components"].(map[string]any)["schemas"].(map[string]any)

	var all []string
	refs(doc, &all)

	for _, ref := range all {
		name, ok := strings.CutPrefix(ref, "#/components/schemas/")
		if !ok {
			continue
		}

		if _, found := schemas[name]; !found {
			t.Errorf("$ref %s does not resolve", ref)
		}
	}
}

func TestOpenAPICarriesTheCommandArguments(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	doc := h.openapi()
	schemas := doc["components"].(map[string]any)["schemas"].(map[string]any)

	for _, name := range []string{"item.move", "track.add", "text.add"} {
		if _, ok := schemas["args."+name]; !ok {
			t.Errorf("no args schema for %s", name)
		}
	}

	if _, ok := schemas["args.asset.add"]; ok {
		t.Error("the internal command asset.add is in the document")
	}

	op, _ := schemas["Operation"].(map[string]any)
	if op == nil {
		t.Fatal("no Operation schema")
	}

	enum, _ := op["properties"].(map[string]any)["op"].(map[string]any)["enum"].([]any)
	names := make([]string, len(enum))

	for i, e := range enum {
		names[i], _ = e.(string)
	}

	if !slices.Contains(names, "item.move") || slices.Contains(names, "asset.add") || !slices.IsSorted(names) {
		t.Errorf("Operation.op enum = %v", names)
	}

	byOp, _ := op["x-args-by-op"].(map[string]any)
	if len(byOp) != len(names) {
		t.Errorf("x-args-by-op has %d entries for %d commands", len(byOp), len(names))
	}

	problem, _ := schemas["Problem"].(map[string]any)
	props, _ := problem["properties"].(map[string]any)

	for _, member := range []string{"code", "message", "hint", "retryable", "op_index", "path", "details"} {
		if _, ok := props[member]; !ok {
			t.Errorf("the Problem schema lacks %s", member)
		}
	}
}

// What the server sends matches what the document says (SDD-003 section 6.7 item 6).
func TestResponsesMatchTheDocument(t *testing.T) {
	t.Parallel()

	h := newHarness(t)

	doc, err := h.vapp.OpenAPI()
	if err != nil {
		t.Fatal(err)
	}

	check := func(method, path string, res resp) {
		t.Helper()

		if err := doc.ValidateResponse(method, path, res.status, res.body); err != nil {
			t.Errorf("%s %s = %d does not match the document: %v\n%.300s", method, path, res.status, err, res.body)
		}
	}

	created := h.post("/api/v1/projects", `{"name":"doc"}`)
	check("POST", "/api/v1/projects", created)

	id, _ := created.json(t)["project_id"].(string)

	edit := h.post(
		"/api/v1/projects/"+id+"/edit",
		`{"base_revision":0,"operations":[{"op":"track.add","args":{"kind":"visual","name":"V"}}]}`,
	)
	check("POST", "/api/v1/projects/"+id+"/edit", edit)

	for _, p := range []string{
		"/api/v1/projects", "/api/v1/projects/" + id, "/api/v1/projects/" + id + "?detail=document",
		"/api/v1/projects/" + id + "/history", "/api/v1/projects/" + id + "/lint", "/api/v1/commands", "/api/v1/commands/item.move",
	} {
		check("GET", strings.SplitN(p, "?", 2)[0], h.get(p))
	}

	check("POST", "/api/v1/projects/"+id+"/undo", h.post("/api/v1/projects/"+id+"/undo", ""))
	check("GET", "/api/v1/commands/nope", h.get("/api/v1/commands/nope"))
	check("GET", "/api/v1/projects/"+id+"/media", h.get("/api/v1/projects/"+id+"/media"))

	if _, err := json.Marshal(doc); err != nil {
		t.Error(err)
	}
}
