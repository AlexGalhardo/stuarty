package httpapi_test

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/AlexGalhardo/stuarty/service"
	"github.com/AlexGalhardo/stuarty/web/app/events"
)

// SDD-003 section 6.8, the part of session C that does not need media or jobs:
// create, edit with and without an idempotency key, lint, history, undo.
func TestSessionCOverHTTP(t *testing.T) {
	t.Parallel()

	h := newHarness(t)

	created := h.do(req{
		method: http.MethodPost, path: "/api/v1/projects", body: projectJSON("Launch teaser"),
		headers: map[string]string{"Stuarty-Client": "curl-demo"},
	})
	if created.status != http.StatusCreated {
		t.Fatalf("create = %d %s", created.status, created.body)
	}

	body := created.json(t)
	id, _ := body["project_id"].(string)

	if loc := created.header.Get("Location"); loc != "/api/v1/projects/"+id {
		t.Errorf("Location = %q", loc)
	}

	if body["revision"] != float64(0) || body["created"] != true {
		t.Errorf("create body = %v", body)
	}

	if created.header.Get("Cache-Control") != "no-store" {
		t.Errorf("Cache-Control = %q on an API response", created.header.Get("Cache-Control"))
	}

	dry := h.post("/api/v1/projects?dry_run=true", projectJSON("dry run"))
	if dry.status != http.StatusOK || dry.json(t)["created"] != false {
		t.Errorf("dry run = %d %s", dry.status, dry.body)
	}

	edit := func(base int, ops, key string) resp {
		hd := map[string]string{"Stuarty-Client": "curl-demo"}
		if key != "" {
			hd["Idempotency-Key"] = key
		}

		return h.do(req{
			method: http.MethodPost, path: "/api/v1/projects/" + id + "/edit", headers: hd,
			body: fmt.Sprintf(`{"base_revision":%d,"operations":%s}`, base, ops),
		})
	}

	if added := h.post(
		"/api/v1/projects/"+id+"/items",
		`{"kind":"timeline","title":"Launch teaser"}`,
	); added.status != http.StatusCreated {
		t.Fatalf("add timeline = %d %s", added.status, added.body)
	}

	first := edit(0, `[{"op":"track.add","args":{"kind":"visual","name":"Video","id":"trk_f6g7h8j9k0"}},`+
		`{"op":"track.add","args":{"kind":"visual","name":"Captions","id":"trk_a1b2c3d4e5"}}]`, "")
	if first.status != http.StatusOK {
		t.Fatalf("edit = %d %s", first.status, first.body)
	}

	r1 := first.json(t)
	if r1["revision"] != float64(1) || r1["actor"] != "local:curl-demo" || r1["applied"] != true {
		t.Errorf("edit body = %v", r1)
	}

	if first.header.Get("X-Vukem-Revision") != "1" {
		t.Errorf("X-Vukem-Revision = %q", first.header.Get("X-Vukem-Revision"))
	}

	text := `[{"op":"text.add","args":{"track_id":"trk_a1b2c3d4e5","start":"0s","duration":"2s","text":"hello"}}]`

	keyed := edit(1, text, "caption-0001")
	if keyed.status != http.StatusOK || keyed.json(t)["replayed"] != false {
		t.Fatalf("keyed edit = %d %s", keyed.status, keyed.body)
	}

	again := edit(1, text, "caption-0001")
	if again.status != http.StatusOK || again.json(t)["replayed"] != true || again.json(t)["revision"] != float64(2) {
		t.Errorf("replay = %d %s; want 200 replayed at the same revision", again.status, again.body)
	}

	mismatch := edit(1, strings.Replace(text, "hello", "other", 1), "caption-0001")
	if env := mismatch.json(t); mismatch.status != http.StatusConflict || env["code"] != "E_IDEMPOTENCY_MISMATCH" {
		t.Errorf("same key, other operations = %d %s", mismatch.status, mismatch.body)
	}

	lint := h.get("/api/v1/projects/" + id + "/lint")
	if lint.status != http.StatusOK || lint.header.Get("X-Vukem-Revision") != "2" ||
		lint.json(t)["revision"] != float64(2) {
		t.Errorf("lint = %d %s", lint.status, lint.body)
	}

	hist := h.get("/api/v1/projects/" + id + "/history")
	if hist.status != http.StatusOK || len(hist.json(t)["entries"].([]any)) != 2 {
		t.Errorf("history = %d %s", hist.status, hist.body)
	}

	undo := h.do(req{
		method: http.MethodPost, path: "/api/v1/projects/" + id + "/undo",
		headers: map[string]string{"Stuarty-Client": "curl-demo"},
	})
	if undo.status != http.StatusOK || undo.json(t)["kind"] != "undo" || undo.json(t)["revision"] != float64(3) {
		t.Errorf("undo = %d %s", undo.status, undo.body)
	}

	nothing := h.post("/api/v1/projects/"+id+"/redo", `{"dry_run":true}`) // another actor: nothing to redo
	if nothing.status != http.StatusConflict || nothing.json(t)["code"] != "E_NOTHING_TO_REDO" {
		t.Errorf("redo by another actor = %d %s", nothing.status, nothing.body)
	}
}

func TestProjectsListGetAndConditionalRead(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	id := h.newProject("a")

	list := h.get("/api/v1/projects?limit=5")
	if list.status != http.StatusOK || len(list.json(t)["projects"].([]any)) != 1 {
		t.Fatalf("list = %d %s", list.status, list.body)
	}

	get := h.get("/api/v1/projects/" + id)
	if get.status != http.StatusOK || get.header.Get("ETag") != `"r0-concise-active"` ||
		get.header.Get("X-Vukem-Revision") != "0" {
		t.Fatalf("get = %d etag %q %s", get.status, get.header.Get("ETag"), get.body)
	}

	cached := h.do(
		req{path: "/api/v1/projects/" + id, headers: map[string]string{"If-None-Match": `"r0-concise-active"`}},
	)
	if cached.status != http.StatusNotModified || len(cached.body) != 0 {
		t.Errorf("conditional get = %d %q", cached.status, cached.body)
	}

	missing := h.get("/api/v1/projects/prj_zzzzzzzzzz")
	if env := missing.json(t); missing.status != http.StatusNotFound || env["code"] != "E_NOT_FOUND" {
		t.Errorf("unknown project = %d %s", missing.status, missing.body)
	}

	trashed := h.post("/api/v1/projects/"+id+"/trash", "")
	if trashed.status != http.StatusOK || trashed.json(t)["state"] != "trashed" {
		t.Errorf("trash = %d %s", trashed.status, trashed.body)
	}
}

// Trash and restore change the state of a project but not its revision, so the
// ETag carries the state and a stale cached copy is never confirmed.
func TestProjectETagChangesWhenTheProjectIsTrashed(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	id := h.newProject("etag")
	path := "/api/v1/projects/" + id

	etag := h.get(path).header.Get("ETag")

	if trashed := h.post(path+"/trash", ""); trashed.status != http.StatusOK {
		t.Fatalf("trash = %d %s", trashed.status, trashed.body)
	}

	got := h.do(req{path: path, headers: map[string]string{"If-None-Match": etag}})
	if got.status != http.StatusOK || got.json(t)["state"] != "trashed" {
		t.Errorf("get after trash = %d %s, want 200 and state trashed", got.status, got.body)
	}
}

// dryService answers every job start as a queued job, as a dry run does, and
// records whether the call was a dry run.
type dryService struct {
	service.Service
	dry *[]bool
}

func (d dryService) JobStart(_ context.Context, req service.JobStartRequest) (*service.Job, error) {
	*d.dry = append(*d.dry, req.DryRun)

	return &service.Job{JobID: "job_0000000001", Kind: req.Kind, State: service.JobQueued}, nil
}

// A dry run answers 200 whether it is asked in the query or in the body.
func TestJobStartDryRunAnswersOKInQueryAndBody(t *testing.T) {
	t.Parallel()

	var seen []bool

	h := newHarnessWrapped(t, events.Options{}, func(s service.Service) service.Service {
		return dryService{Service: s, dry: &seen}
	})
	id := h.newProject("dry")

	for name, res := range map[string]resp{
		"query": h.post("/api/v1/projects/"+id+"/jobs?dry_run=true", `{"kind":"export"}`),
		"body":  h.post("/api/v1/projects/"+id+"/jobs", `{"kind":"export","dry_run":true}`),
	} {
		if res.status != http.StatusOK {
			t.Errorf("dry run in the %s = %d %s, want 200", name, res.status, res.body)
		}
	}

	if len(seen) != 2 || !seen[0] || !seen[1] {
		t.Errorf("the service saw dry flags %v, want [true true]", seen)
	}

	if res := h.post("/api/v1/projects/"+id+"/jobs", `{"kind":"export"}`); res.status != http.StatusAccepted {
		t.Errorf("a queued job = %d, want 202", res.status)
	}
}

func TestCommandsListAndGet(t *testing.T) {
	t.Parallel()

	h := newHarness(t)

	list := h.get("/api/v1/commands")
	if list.status != http.StatusOK || !strings.Contains(string(list.body), `"item.move"`) ||
		strings.Contains(string(list.body), `"asset.add"`) {
		t.Errorf("list = %d %.200s", list.status, list.body)
	}

	one := h.get("/api/v1/commands/item.move")
	if one.status != http.StatusOK || one.json(t)["command"] == nil {
		t.Errorf("get = %d %.200s", one.status, one.body)
	}

	for _, name := range []string{"asset.add", "no.such"} {
		bad := h.get("/api/v1/commands/" + name)
		if env := bad.json(t); bad.status != http.StatusBadRequest || env["code"] != "E_UNKNOWN_COMMAND" {
			t.Errorf("get %s = %d %s", name, bad.status, bad.body)
		}
	}
}

// The error envelope is the same on every status (SDD-003 section 6.6).
func TestErrorEnvelope(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	id := h.newProject("e")

	h.post(
		"/api/v1/projects/"+id+"/edit",
		`{"base_revision":0,"operations":[{"op":"track.add","args":{"kind":"visual","name":"V","id":"trk_f6g7h8j9k0"}}]}`,
	)

	cases := map[string]struct {
		r      req
		status int
		code   string
	}{
		"service error": {req{
			method: http.MethodPost, path: "/api/v1/projects/" + id + "/edit",
			body: `{"base_revision":1,"operations":[{"op":"nope.nothing","args":{}}]}`,
		}, 400, "E_UNKNOWN_COMMAND"},
		"operation name with spaces": {req{
			method: http.MethodPost, path: "/api/v1/projects/" + id + "/edit",
			body: `{"base_revision":1,"operations":[{"op":" track.add ","args":{}}]}`,
		}, 400, "E_UNKNOWN_COMMAND"},
		"unknown route": {req{path: "/api/v1/nothing-here"}, 404, "E_NOT_FOUND"},
		"wrong method":  {req{method: http.MethodDelete, path: "/api/v1/projects"}, 405, "E_METHOD_NOT_ALLOWED"},
		"no credential": {req{path: "/api/v1/projects", noAuth: true}, 401, "E_UNAUTHORIZED"},
		"bad token": {req{
			path: "/api/v1/projects", noAuth: true,
			headers: map[string]string{"Authorization": "Bearer wrong"},
		}, 401, "E_UNAUTHORIZED"},
		"bad label": {
			req{path: "/api/v1/projects", headers: map[string]string{"Stuarty-Client": "ui"}},
			400,
			"E_VALIDATION",
		},
		"bad json": {
			req{method: http.MethodPost, path: "/api/v1/projects", body: `{"name":`},
			400,
			"E_VALIDATION",
		},
		"missing member": {
			req{method: http.MethodPost, path: "/api/v1/projects", body: `{}`},
			400,
			"E_VALIDATION",
		},
		"bad limit":           {req{path: "/api/v1/projects?limit=abc"}, 400, "E_VALIDATION"},
		"bad assets limit":    {req{path: "/api/v1/assets?limit=abc"}, 400, "E_VALIDATION"},
		"bad assets reusable": {req{path: "/api/v1/assets?reusable=maybe"}, 400, "E_VALIDATION"},
		"bad assets origin":   {req{path: "/api/v1/assets?origin=stolen"}, 400, "E_VALIDATION"},
		"bad asset hash":      {req{path: "/api/v1/assets/nonsense"}, 400, "E_VALIDATION"},
		"unknown asset":       {req{path: "/api/v1/assets/sha256:" + strings.Repeat("0", 64)}, 404, "E_NOT_FOUND"},
		"unknown asset uses": {
			req{path: "/api/v1/assets/sha256:" + strings.Repeat("0", 64) + "/uses"},
			404,
			"E_NOT_FOUND",
		},
		"tag an unknown asset": {req{
			method: http.MethodPost, path: "/api/v1/assets/sha256:" + strings.Repeat("0", 64) + "/tags",
			body: `{"add":["x"]}`,
		}, 404, "E_NOT_FOUND"},
		"set an unknown asset": {req{
			method: http.MethodPut, path: "/api/v1/assets/sha256:" + strings.Repeat("0", 64), body: `{"reusable":true}`,
		}, 404, "E_NOT_FOUND"},
		"too large": {
			req{
				method: http.MethodPost,
				path:   "/api/v1/projects/" + id + "/edit",
				body: `{"base_revision":1,"operations":[{"op":"text.add","args":{"text":"` + strings.Repeat(
					"x",
					2<<20,
				) + `"}}]}`,
			},
			413,
			"E_PAYLOAD_TOO_LARGE",
		},
		"pending operation": {
			req{method: http.MethodGet, path: "/api/v1/projects/" + id + "/frame"},
			500,
			"E_INTERNAL",
		},
	}

	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			res := h.do(c.r)

			ct := res.header.Get("Content-Type")
			if res.status != c.status || !strings.HasPrefix(ct, "application/problem+json") {
				t.Fatalf("status %d (want %d), content type %q\n%s", res.status, c.status, ct, res.body)
			}

			env := res.json(t)
			if env["code"] != c.code || env["title"] != c.code || env["type"] != "urn:stuarty:error:"+c.code ||
				env["status"] != float64(c.status) || env["message"] == "" || env["message"] != env["detail"] ||
				env["retryable"] == nil || env["instance"] == nil {
				t.Errorf("envelope = %s", res.body)
			}
		})
	}
}

// The view routes pin the revision: a rev that is not the current one is
// E_INVALID_STATE with the current revision, before anything is rendered; a
// build with no renderer answers E_INTERNAL.
func TestViewRoutesPinTheRevision(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	id := h.newProject("view")

	for _, route := range []string{"frame?at=0s&rev=9", "sheet?rev=9"} {
		res := h.get("/api/v1/projects/" + id + "/" + route)
		if res.status != http.StatusConflict || res.json(t)["code"] != "E_INVALID_STATE" ||
			res.json(t)["details"].(map[string]any)["current_revision"] != float64(0) {
			t.Errorf("%s = %d %s", route, res.status, res.body)
		}
	}

	if res := h.get("/api/v1/projects/" + id + "/frame?at=0s"); res.status != http.StatusInternalServerError {
		t.Errorf("no renderer = %d, want 500", res.status)
	}
}

// to is a deprecated alias of until on /plan and /sheet until 1.0: it works
// alone, may repeat until, and a different value is E_VALIDATION on to.
func TestUntilAliasOnPlanAndSheet(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	id := h.newProject("alias")
	base := "/api/v1/projects/" + id

	if res := h.get(base + "/plan?from=0&to=30"); res.status != http.StatusOK {
		t.Errorf("plan with to = %d %s", res.status, res.body)
	}

	if res := h.get(base + "/plan?from=0&until=30&to=30"); res.status != http.StatusOK {
		t.Errorf("plan with equal until and to = %d %s", res.status, res.body)
	}

	for _, route := range []string{"/plan?from=0&until=30&to=40", "/sheet?until=1s&to=2s"} {
		res := h.get(base + route)
		if res.status != http.StatusBadRequest || res.json(t)["code"] != "E_VALIDATION" {
			t.Errorf("%s = %d %s", route, res.status, res.body)
		}
	}
}

func TestPlanRouteAnswersTheWindowOfSDD004(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	id := h.newProject("plan")

	res := h.get("/api/v1/projects/" + id + "/plan?from=0&until=30")
	body := res.json(t)

	ok := res.status == http.StatusOK && body["revision"] == float64(0) && body["end_frame"] == float64(0)
	ok = ok && body["layers"] != nil && res.header.Get("X-Vukem-Revision") == "0"

	if !ok {
		t.Errorf("plan = %d %s", res.status, res.body)
	}

	if res := h.get("/api/v1/projects/" + id + "/plan?from=0&until=30&rev=4"); res.status != http.StatusConflict {
		t.Errorf("a stale rev = %d, want 409", res.status)
	}

	if res := h.get("/api/v1/projects/" + id + "/plan?from=0&until=900"); res.status != http.StatusBadRequest {
		t.Errorf("a window of 900 frames = %d, want 400", res.status)
	}

	if res := h.get(
		"/api/v1/projects/" + id + "/bitmaps/" + strings.Repeat("a", 64),
	); res.status != http.StatusNotFound {
		t.Errorf("an unknown bitmap = %d, want 404", res.status)
	}
}

// frameService answers ViewFrame without a renderer.
type frameService struct{ service.Service }

// A pinned request goes to the real service, which owns the pin check.
func (f frameService) ViewFrame(ctx context.Context, req service.ViewFrameRequest) (*service.ViewResult, error) {
	if req.Pin != 0 {
		return f.Service.ViewFrame(ctx, req)
	}

	return &service.ViewResult{
		ProjectID: req.ProjectID, Mode: "frame", Width: 2, Height: 2, MIME: "image/jpeg",
		Image: []byte{0xff, 0xd8, 0xff, 0xd9}, Cells: []service.ViewCell{{}}, Text: "frame",
	}, nil
}

// The editor's truth-on-pause <img> sends Accept: image/*; the guard must not
// turn that into the JSON of the ViewResult.
func TestFrameRouteAnswersTheJPEGToAnImageAccept(t *testing.T) {
	t.Parallel()

	h := newHarnessWrapped(t, events.Options{}, func(s service.Service) service.Service { return frameService{s} })
	id := h.newProject("view")
	path := "/api/v1/projects/" + id + "/frame?at=0s"

	res := h.do(req{path: path, headers: map[string]string{"Accept": "image/*"}})
	if res.status != http.StatusOK || res.header.Get("Content-Type") != "image/jpeg" ||
		!bytes.HasPrefix(res.body, []byte{0xff, 0xd8}) {
		t.Errorf("Accept image/*: %d %s %q", res.status, res.header.Get("Content-Type"), res.body)
	}

	res = h.do(req{path: path, headers: map[string]string{"Accept": "application/json"}})
	if res.status != http.StatusOK || !strings.HasPrefix(res.header.Get("Content-Type"), "application/json") {
		t.Errorf("Accept application/json: %d %s", res.status, res.header.Get("Content-Type"))
	}

	// An error stays problem+json whatever the client prefers.
	bad := h.do(req{
		path: "/api/v1/projects/" + id + "/frame?at=0s&rev=9", headers: map[string]string{"Accept": "image/*"},
	})
	if bad.status != http.StatusConflict || bad.json(t)["code"] != "E_INVALID_STATE" {
		t.Errorf("an error to an image client = %d %s", bad.status, bad.body)
	}
}

// VV-086: a field the route does not know is refused, not dropped (a 1920x1080 project was silently made 9:16).
func TestProjectCreateRefusesAnUnknownField(t *testing.T) {
	t.Parallel()

	h := newHarness(t)

	res := h.post("/api/v1/projects", `{"name":"wide","width":1920,"height":1080}`)
	if res.status < 400 || res.status >= 500 || !strings.Contains(string(res.body), "width") {
		t.Fatalf("an unknown field = %d %s, want a 4xx that names the field", res.status, res.body)
	}

	if res := h.post(
		"/api/v1/projects",
		strings.TrimSuffix(projectJSON("ok project"), "}")+`,"preset":"16:9"}`,
	); res.status != http.StatusCreated {
		t.Fatalf("a known body = %d %s", res.status, res.body)
	}
}

// VV-104: a limit or a count below 1 is a validation error on every route, not a quiet 200.
func TestLimitAndCountBelowOneAreRefused(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	id := h.newProject("p")

	for _, path := range []string{
		"/api/v1/projects?limit=0", "/api/v1/projects?limit=-1", "/api/v1/projects/" + id + "/history?limit=-1",
		"/api/v1/projects/" + id + "/media?limit=0", "/api/v1/audit?limit=-3", "/api/v1/templates?limit=-5",
		"/api/v1/projects/" + id + "/sheet?count=0",
	} {
		if res := h.get(path); res.status != http.StatusBadRequest || res.json(t)["code"] != "E_VALIDATION" {
			t.Errorf("GET %s = %d %s, want 400 E_VALIDATION", path, res.status, res.body)
		}
	}

	if res := h.get("/api/v1/projects?limit=1"); res.status != http.StatusOK {
		t.Errorf("limit=1 = %d", res.status)
	}
}

// VV-194: a plan asked with at= says that until is the missing parameter.
func TestPlanWithoutUntilNamesTheMissingParameter(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	id := h.newProject("p")

	res := h.get("/api/v1/projects/" + id + "/plan?at=1s")
	if res.status != http.StatusBadRequest || !strings.Contains(string(res.body), "until is required") {
		t.Errorf("plan = %d %s", res.status, res.body)
	}
}

// VV-107: the listing names the id of a project the way every other answer does.
func TestProjectListCarriesProjectID(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	id := h.newProject("p")

	res := h.get("/api/v1/projects")
	if res.status != http.StatusOK || !strings.Contains(string(res.body), `"project_id":"`+id+`"`) {
		t.Fatalf("list = %d %s", res.status, res.body)
	}
}

// VV-210: dry_run in a body is a JSON boolean; a string is refused as a number is.
func TestUndoRedoDryRunTakesOnlyJSONBooleans(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	id := h.newProject("a")

	for _, route := range []string{"undo", "redo"} {
		for _, body := range []string{`{"dry_run":"yes"}`, `{"dry_run":"true"}`, `{"dry_run":7}`} {
			res := h.post("/api/v1/projects/"+id+"/"+route, body)
			if res.status != http.StatusBadRequest && res.status != http.StatusUnprocessableEntity {
				t.Errorf("%s %s = %d %s, want a validation error", route, body, res.status, res.body)
			}
		}
	}
}

func TestUndoDryRunQueryAliasStillWorks(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	id := h.newProject("a")

	res := h.post("/api/v1/projects/"+id+"/undo?dry_run=true", `{}`)
	if res.status != http.StatusConflict || res.json(t)["code"] != "E_NOTHING_TO_UNDO" {
		t.Errorf("undo ?dry_run=true = %d %s", res.status, res.body)
	}
}

// VV-210: the body of undo keeps the normal limit of the route; nothing reads it twice.
func TestUndoOversizedBodyKeepsTheRouteLimit(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	id := h.newProject("a")

	res := h.post("/api/v1/projects/"+id+"/undo", `{"dry_run":false,"pad":"`+strings.Repeat("x", 11<<20)+`"}`)
	if res.status != http.StatusRequestEntityTooLarge {
		t.Errorf("oversized undo body = %d %.200s, want 413", res.status, res.body)
	}
}

// A validation error may quote a short excerpt of the value. It must not send
// the value back whole (SDD-003 section 1.2 rule 9), and the refusal must not
// create a project.
func TestValidationErrorDoesNotEchoTheSubmittedValue(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	huge := strings.Repeat("p", 100_000)

	for _, c := range []struct {
		name string
		r    req
	}{
		{"preset", req{method: http.MethodPost, path: "/api/v1/projects", body: `{"name":"n","preset":"` + huge + `"}`}},
		{"state", req{path: "/api/v1/projects?state=" + huge[:4000]}},
	} {
		res := h.do(c.r)
		body := string(res.body)
		if res.status != http.StatusBadRequest || !strings.Contains(body, `"code":"E_VALIDATION"`) {
			t.Errorf("%s = %d %s, want 400 E_VALIDATION", c.name, res.status, body[:min(len(body), 200)])
		}

		if strings.Contains(body, "E_INTERNAL") || strings.Contains(body, huge[:300]) || len(body) > 4000 {
			t.Errorf("%s echoed the value: %d bytes", c.name, len(body))
		}
	}

	list := h.get("/api/v1/projects")
	if list.status != http.StatusOK || strings.Contains(string(list.body), `"name":"n"`) {
		t.Errorf("a refused create left a project: %d %s", list.status, list.body)
	}
}

// dry_run is a JSON boolean. A string such as "no" is not false: accepting it
// applies the write (trash, or an edit) and advances the project.
func TestDryRunTextDoesNotApplyTheWrite(t *testing.T) {
	t.Parallel()

	h := newHarness(t)

	refused := func(path, body string) {
		t.Helper()

		res := h.post(path, body)
		if res.status != http.StatusBadRequest || res.json(t)["code"] != "E_VALIDATION" {
			t.Errorf("POST %s %s = %d %s, want 400 E_VALIDATION", path, body, res.status, res.body)
		}
	}

	id := h.newProject("trash")
	for _, body := range []string{`{"dry_run":"no"}`, `{"dry_run":"yes"}`, `{"dry_run":"true"}`, `{"dry_run":7}`} {
		refused("/api/v1/projects/"+id+"/trash", body)
	}

	refused("/api/v1/projects/"+id+"/trash?dry_run=yes", `{}`)
	refused("/api/v1/projects/"+id+"/restore", `{"dry_run":"no"}`)

	if res := h.post("/api/v1/projects/"+id+"/trash?dry_run=true", `{}`); res.status != http.StatusOK {
		t.Errorf("trash ?dry_run=true = %d %s", res.status, res.body)
	}

	if res := h.post("/api/v1/projects/"+id+"/trash", `{"dry_run":true}`); res.status != http.StatusOK {
		t.Errorf("trash dry_run true = %d %s", res.status, res.body)
	}

	if got := h.get("/api/v1/projects/" + id); got.json(t)["state"] != "active" {
		t.Fatalf("dry_run text changed the project: %s", got.body)
	}

	edit := h.newProject("edit")
	refused("/api/v1/projects/"+edit+"/edit",
		`{"base_revision":0,"dry_run":"no","operations":[{"op":"track.add","args":{"kind":"visual"}}]}`)

	if got := h.get("/api/v1/projects/" + edit); got.json(t)["revision"] != float64(0) {
		t.Fatalf("dry_run text advanced the revision: %s", got.body)
	}

	applied := h.post("/api/v1/projects/"+edit+"/edit",
		`{"base_revision":0,"operations":[{"op":"track.add","args":{"kind":"visual"}}]}`)
	if applied.status != http.StatusOK || applied.json(t)["applied"] != true {
		t.Fatalf("a valid edit = %d %s", applied.status, applied.body)
	}

	if got := h.get("/api/v1/projects/" + edit); got.json(t)["revision"] != float64(1) {
		t.Fatalf("a valid edit did not advance the revision: %s", got.body)
	}

	doc := h.openapi()
	paths, _ := doc["paths"].(map[string]any)
	for _, path := range []string{"/api/v1/projects/{id}/trash", "/api/v1/projects/{id}/edit", "/api/v1/projects/{id}/undo"} {
		item, _ := paths[path].(map[string]any)
		op, _ := item["post"].(map[string]any)
		body, _ := op["requestBody"].(map[string]any)
		content, _ := body["content"].(map[string]any)
		jsonBody, _ := content["application/json"].(map[string]any)
		schema, _ := jsonBody["schema"].(map[string]any)
		props, _ := schema["properties"].(map[string]any)
		dry, _ := props["dry_run"].(map[string]any)
		if dry["type"] != "boolean" {
			t.Errorf("%s dry_run schema = %v, want boolean", path, dry)
		}
	}
}

// VV-317: the fit check is its own route, so a lint that asks for it is refused and not answered as a plain lint.
func TestLintRefusesTheFitCheckQuery(t *testing.T) {
	t.Parallel()

	h := newHarness(t)

	for _, q := range []string{"preset=tiktok", "start=1", "end=2"} {
		res := h.get("/api/v1/projects/prj_0000000000/lint?" + q)
		env := res.json(t)

		if res.status != http.StatusBadRequest || env["code"] != "E_VALIDATION" ||
			!strings.Contains(string(res.body), "export-check") {
			t.Errorf("%s = %d %s, want E_VALIDATION naming export-check", q, res.status, res.body)
		}
	}
}
