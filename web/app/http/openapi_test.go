package httpapi_test

import (
	"encoding/json"
	"net/http"
	"slices"
	"strings"
	"testing"

	"github.com/AlexGalhardo/vukem-video/core/timeline"
	"github.com/AlexGalhardo/vukem-video/service/untrusted"
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

// SDD-007 section 8.2: the document marks the path of every field that carries
// text from media or from another actor with x-untrusted, one property per row
// of the provenance table.
func TestOpenAPIMarksUntrustedPaths(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	schemas := h.openapi()["components"].(map[string]any)["schemas"].(map[string]any)

	for key := range untrusted.Provenance {
		parts := strings.Split(key, ".") // service.Type.Field
		schema, _ := schemas[parts[1]].(map[string]any)

		props, _ := schema["properties"].(map[string]any)
		if schema == nil || props == nil {
			t.Errorf("%s: no schema %s", key, parts[1])
			continue
		}

		marked := false

		for _, p := range props {
			if m, ok := p.(map[string]any); ok && m[untrusted.Annotation] == true {
				marked = true
			}
		}

		if !marked {
			t.Errorf("%s: no property of %s carries %s", key, parts[1], untrusted.Annotation)
		}
	}

	name := schemas["ProjectSummary"].(map[string]any)["properties"].(map[string]any)["name"].(map[string]any)
	if name[untrusted.Annotation] != true {
		t.Errorf("ProjectSummary.name = %v, want x-untrusted", name)
	}

	id := schemas["ProjectSummary"].(map[string]any)["properties"].(map[string]any)["id"].(map[string]any)
	if _, has := id[untrusted.Annotation]; has {
		t.Error("ProjectSummary.id is marked x-untrusted")
	}
}

// VV-096: the document has no servers entry (it would be the framework default, not the address
// the server listens on) and declares the statuses the server sends: 400 for E_VALIDATION, never 422
// as a validation failure.
func TestOpenAPIDeclaresTheRealStatuses(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	doc := h.openapi()

	if servers, ok := doc["servers"]; ok {
		t.Errorf("servers = %v, want none", servers)
	}

	paths, _ := doc["paths"].(map[string]any)

	for path, item := range paths {
		for method, raw := range item.(map[string]any) {
			responses, _ := raw.(map[string]any)["responses"].(map[string]any)

			r422, _ := responses["422"].(map[string]any)

			if ref, _ := r422["$ref"].(string); strings.HasSuffix(ref, "/ValidationError") {
				t.Errorf("%s %s declares 422 as ValidationError", strings.ToUpper(method), path)
			}
		}
	}

	create, _ := paths["/api/v1/projects"].(map[string]any)["post"].(map[string]any)
	responses, _ := create["responses"].(map[string]any)

	if _, ok := responses["400"]; !ok {
		t.Errorf("POST /api/v1/projects responses = %v, want 400", responses)
	}

	if res := h.post("/api/v1/projects", `{"name":""}`); res.status != http.StatusBadRequest {
		t.Errorf("empty name = %d, want 400", res.status)
	}
}

// VV-103: the document gives the form of the ids, of the nonce and of the frame rate, and the API
// takes a numeric fps as the CLI does.
func TestOpenAPIGivesTheFormats(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	doc := h.openapi()
	schemas := doc["components"].(map[string]any)["schemas"].(map[string]any)

	asset, _ := schemas["importFileIn"].(map[string]any)["properties"].(map[string]any)["asset_id"].(map[string]any)
	if got, _ := asset["pattern"].(string); got != timeline.IDPattern(timeline.PrefixAsset) {
		t.Errorf("importFileIn.asset_id pattern = %q", got)
	}

	paths, _ := doc["paths"].(map[string]any)
	create, _ := paths["/api/v1/projects"].(map[string]any)["post"].(map[string]any)
	body, _ := create["requestBody"].(map[string]any)["content"].(map[string]any)["application/json"].(map[string]any)
	props, _ := body["schema"].(map[string]any)["properties"].(map[string]any)

	if fps, _ := props["fps"].(map[string]any); fps["oneOf"] == nil {
		t.Errorf("fps = %v, want oneOf number or text", fps)
	}

	if id, _ := props["id"].(map[string]any); id["pattern"] == nil {
		t.Errorf("project id = %v, want a pattern", id)
	}

	hs, _ := paths["/api/v1/handshake"].(map[string]any)["get"].(map[string]any)
	params, _ := hs["parameters"].([]any)

	if len(params) != 1 || params[0].(map[string]any)["schema"].(map[string]any)["pattern"] == nil {
		t.Errorf("nonce parameter = %v, want a pattern", params)
	}

	for _, fps := range []string{`60`, `"30000/1001"`} {
		res := h.post("/api/v1/projects", `{"name":"f","fps":`+fps+`}`)
		if res.status != http.StatusCreated {
			t.Errorf("fps %s = %d %.200s", fps, res.status, res.body)
		}
	}

	if res := h.post("/api/v1/projects", `{"name":"f","fps":true}`); res.status != http.StatusBadRequest {
		t.Errorf("fps true = %d, want 400", res.status)
	}
}

// VV-115: the heavy routes of the document are the ones that spend the heavy-call limit.
func TestOpenAPIMarksTheHeavyOperations(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	doc := h.openapi()
	paths, _ := doc["paths"].(map[string]any)
	marked := map[string]bool{}

	for _, item := range paths {
		for _, raw := range item.(map[string]any) {
			op, _ := raw.(map[string]any)
			desc, _ := op["description"].(string)

			if strings.Contains(desc, "Heavy call:") {
				id, _ := op["operationId"].(string)
				marked[id] = true
			}
		}
	}

	want := []string{
		"jobs.start", "media.import", "templates.import", "fonts.import",
		"projects.bundle.import", "projects.bundle.export", "projects.otio.import",
	}

	if len(marked) != len(want) {
		t.Errorf("marked = %v, want %v", marked, want)
	}

	for _, id := range want {
		if !marked[id] {
			t.Errorf("%s is not marked as a heavy call", id)
		}
	}
}
