package httpapi

import (
	"encoding/json"
	"net/http"
	"reflect"
	"slices"
	"strconv"
	"strings"

	"github.com/AlexGalhardo/stuarty/core/timeline"
	"github.com/AlexGalhardo/stuarty/engine/transcribe"
	"github.com/AlexGalhardo/stuarty/service"
	"github.com/AlexGalhardo/stuarty/service/untrusted"

	"github.com/AlexGalhardo/vukem/openapi"
	"github.com/AlexGalhardo/vukem/web"
)

// Info is the base of the OpenAPI document: the problem members of the error
// envelope (SDD-003 section 6.7 item 3) and the bearer scheme.
func Info() openapi.Info {
	return openapi.Info{
		Title:       "Stuarty",
		Description: "The HTTP API of Stuarty. Every operation is the HTTP form of one service operation. This document needs the same Bearer token as the API.",
		PathPrefix:  BasePath,
		ProblemExtensions: map[string]openapi.Schema{
			"code":      {"type": "string", "description": "A stable error code, for example E_OVERLAP."},
			"message":   {"type": "string", "description": "What went wrong, in one or two sentences."},
			"hint":      {"type": "string", "description": "What to do next."},
			"retryable": {"type": "boolean", "description": "Whether a later retry can succeed."},
			"op_index":  {"type": "integer", "description": "The operation of the batch that failed."},
			"path":      {"type": "string", "description": "The member or argument at fault."},
			"details":   {"type": "object", "description": "Data specific to the code."},
		},
	}
}

// serveOpenAPI is GET /openapi.json: the document vukem builds from the typed
// handlers, completed with what its types cannot say (SDD-003 section 6.7).
func (h *apiHandler) serveOpenAPI(c *web.Ctx) error {
	doc, err := h.app.OpenAPI()
	if err != nil {
		return err
	}

	h.completeOpenAPI(doc)

	// The framework fills servers with its APP_URL default, which is not where this server
	// listens. Without servers, OpenAPI resolves the paths against the address the document
	// was fetched from.
	doc.Servers = nil

	return c.JSON(http.StatusOK, doc)
}

// completeOpenAPI adds the argument schema of every public command as
// components.schemas["args.<name>"], limits Operation.op to the command names and
// records the mapping in x-args-by-op. The registry is the only source of those
// schemas (ADR-0014).
func (h *apiHandler) completeOpenAPI(doc *openapi.Document) {
	if doc.Components.Schemas == nil {
		doc.Components.Schemas = map[string]openapi.Schema{}
	}

	names := make([]string, 0)
	byOp := map[string]any{}

	for _, cmd := range h.Registry.List() {
		if cmd.Internal {
			continue
		}

		raw, err := h.Registry.Schema(cmd.Name)
		if err != nil {
			continue
		}

		var schema openapi.Schema
		if json.Unmarshal(raw, &schema) != nil {
			continue
		}

		delete(schema, "$schema")

		doc.Components.Schemas["args."+cmd.Name] = schema
		byOp[cmd.Name] = map[string]any{"$ref": "#/components/schemas/args." + cmd.Name}
		names = append(names, cmd.Name)
	}

	slices.Sort(names)

	markUntrusted(doc)
	describeJobStartArgs(doc)
	describeErrorStatuses(doc)
	describeReturnedStatuses(doc)
	describeIDFormats(doc)
	describeHeavyOperations(doc)
	describeStepBodies(doc)

	op, ok := doc.Components.Schemas["Operation"]
	if !ok {
		return
	}

	if props, ok := op["properties"].(map[string]any); ok {
		if name, ok := props["op"].(map[string]any); ok {
			name["enum"] = names
		}
	}

	op["x-args-by-op"] = byOp
}

// describeErrorStatuses replaces the 422 the framework declares for a validation failure with the
// statuses the server sends (service.Error.HTTPStatus, SDD-000 section 8): E_VALIDATION is 400,
// a missing project, asset or track is 404, a refused state or revision is 409, and a domain
// rule (E_SLOT, E_UNSUPPORTED_MEDIA, E_TYPE_MISMATCH, ...) is 422.
func describeErrorStatuses(doc *openapi.Document) {
	problem := openapi.Response{Ref: "#/components/responses/Problem"}

	for path, item := range doc.Paths {
		for method, op := range item {
			if op == nil {
				continue
			}

			validation, ok := op.Responses["422"]
			if !ok {
				continue
			}

			delete(op.Responses, "422")

			op.Responses["400"] = validation

			if strings.Contains(path, "{") {
				op.Responses["404"] = problem
			}

			if method == "post" {
				op.Responses["409"] = problem
				op.Responses["422"] = problem
			}
		}
	}
}

// notFoundOperations are the operations without a path id that still answer 404: the file they read
// is missing, or the project named in the body does not exist (VV-139).
var notFoundOperations = []string{"projects.bundle.import", "templates.import", "templates.save", "fonts.import"}

// networkOnlyPrefixes are the operations of the keys and the users, which answer 404 in local mode.
var networkOnlyPrefixes = []string{"keys.", "users."}

// describeReturnedStatuses declares the statuses the framework cannot derive from the handlers
// (VV-139): 403 on every route that needs a scope, 429 on every route that the rate limiters count,
// 404 where a file or a project in the body can be missing, the 200 of a bundle export that has
// already finished (202 when it is still running), and the 404 of the keys and users routes in
// local mode.
func describeReturnedStatuses(doc *openapi.Document) {
	problem := openapi.Response{Ref: "#/components/responses/Problem"}

	for _, item := range doc.Paths {
		for _, op := range item {
			if op == nil {
				continue
			}

			ep, ok := endpointByID(op.OperationID)
			if !ok {
				continue
			}

			if ep.Scope != "" {
				op.Responses["403"] = problem
			}

			if !ep.Uncounted {
				op.Responses["429"] = problem
			}

			if slices.Contains(notFoundOperations, ep.ID) {
				op.Responses["404"] = problem
			}

			// A batch with a refused file answers 200 with the body of the 202 (SDD-003 section 2.6, VV-193).
			if ep.ID == "media.import" {
				op.Responses["200"] = op.Responses["202"]
			}

			if ep.ID == "projects.bundle.export" {
				op.Responses["200"] = op.Responses["202"]
			}

			describeDryRunAndRevision(op, ep.ID, problem)

			if slices.ContainsFunc(networkOnlyPrefixes, func(p string) bool { return strings.HasPrefix(ep.ID, p) }) {
				op.Responses["404"] = problem
				op.Description = strings.TrimSpace(op.Description + " Network mode only: local mode answers 404.")
			}
		}
	}
}

// describeDryRunAndRevision declares the 200 of a dry run of jobs.start, which carries the body of the job it
// would have queued, and the 409 E_REVISION_CONFLICT of a read pinned with ?rev= (VV-209).
func describeDryRunAndRevision(op *openapi.Operation, id string, problem openapi.Response) {
	if id == "jobs.start" {
		op.Responses["200"] = op.Responses["202"]
	}

	if slices.ContainsFunc(op.Parameters, func(p openapi.Parameter) bool { return p.Name == "rev" }) {
		op.Responses["409"] = problem
	}
}

// describeStepBodies restores the boolean type of dry_run. The field is read untyped so that text is
// refused, and the document still says a caller sends true or false. The description on the field stays.
func describeStepBodies(doc *openapi.Document) {
	for _, item := range doc.Paths {
		for _, op := range item {
			if op == nil {
				continue
			}

			if op.RequestBody != nil {
				if media, ok := op.RequestBody.Content["application/json"]; ok && media.Schema != nil {
					props, _ := media.Schema["properties"].(map[string]any)
					if prop, ok := props["dry_run"].(map[string]any); ok {
						prop["type"] = "boolean"
					}
				}
			}

			for _, p := range op.Parameters {
				if p.Name == "dry_run" && p.Schema != nil {
					p.Schema["type"] = "boolean"
				}
			}
		}
	}
}

// heavyOperations are the operations that spend the heavy-call limit; service.heavyLimit is called by
// exactly these (TestHeavyLimitCallSites in service keeps the count).
var heavyOperations = []string{
	"jobs.start", "media.import", "templates.import", "fonts.import",
	"projects.bundle.import", "projects.bundle.export", "projects.otio.import",
}

// describeHeavyOperations says on each heavy operation that it shares the heavy-call limit, so
// the document marks the same routes as the documentation pages (VV-115).
func describeHeavyOperations(doc *openapi.Document) {
	note := "Heavy call: shares the limit of " + strconv.Itoa(service.HeavyPerMinute) +
		" a minute per credential (E_RATE_LIMITED with Retry-After)."

	for _, item := range doc.Paths {
		for _, op := range item {
			if op != nil && slices.Contains(heavyOperations, op.OperationID) {
				op.Description = strings.TrimSpace(op.Description + " " + note)
			}
		}
	}
}

// describeIDFormats adds the "pattern" of the ids and of the handshake nonce to the schemas of the
// requests of this package, which are plain strings for the framework (VV-103). The id forms come
// from timeline.IDPattern, the one the validator uses.
func describeIDFormats(doc *openapi.Document) {
	pattern := func(schema openapi.Schema, prop, re string) {
		props, _ := schema["properties"].(map[string]any)

		if p, ok := props[prop].(map[string]any); ok {
			p["pattern"] = re
		}
	}

	pattern(doc.Components.Schemas["importFileIn"], "asset_id", timeline.IDPattern(timeline.PrefixAsset))

	for path, item := range doc.Paths {
		post, get := item["post"], item["get"]

		if post != nil && post.RequestBody != nil && strings.HasSuffix(path, "/projects") {
			schema := post.RequestBody.Content["application/json"].Schema

			pattern(schema, "id", timeline.IDPattern(timeline.PrefixProject))

			if props, _ := schema["properties"].(map[string]any); props != nil {
				if fps, ok := props["fps"].(map[string]any); ok {
					delete(fps, "type")
					fps["oneOf"] = []any{
						map[string]any{"type": "integer", "minimum": 1, "maximum": 999999},
						map[string]any{"type": "string", "pattern": `^[0-9]{1,6}(/(1|1001))?$`},
					}
				}
			}
		}

		if get != nil && strings.HasSuffix(path, "/handshake") {
			for _, p := range get.Parameters {
				if p.Name == "nonce" {
					p.Schema["pattern"] = "^[0-9a-f]{32}$"
				}
			}
		}
	}
}

// describeJobStartArgs lists the members of args in the body of POST /projects/{id}/jobs, with the
// values of an export format (SDD-003 section 2.8). The frame rate and the size of an export are
// project settings, not arguments (SDD-004 section 8.1), so they are not listed. The service
// refuses a member that is not here with E_VALIDATION.
func describeJobStartArgs(doc *openapi.Document) {
	for path, item := range doc.Paths {
		post := item["post"]
		if !strings.HasSuffix(path, "/projects/{id}/jobs") || post == nil || post.RequestBody == nil {
			continue
		}

		media, ok := post.RequestBody.Content["application/json"]
		if !ok {
			continue
		}

		props, _ := media.Schema["properties"].(map[string]any)
		args, _ := props["args"].(map[string]any)

		if args == nil {
			continue
		}

		delete(args, "additionalProperties")

		args["properties"] = map[string]any{
			"start": map[string]any{
				"type":        "string",
				"description": "export: the first time to render, such as 1.5s or 45f.",
			},
			"end": map[string]any{"type": "string", "description": "export: the end time, not included."},
			"format": map[string]any{
				"type":        "string",
				"enum":        service.ExportFormats,
				"default":     "mp4",
				"description": "export: the file to write; wav and aac are audio only; png is one frame at `at`.",
			},
			"at": map[string]any{
				"type":        "string",
				"description": "export png: the time of the frame, such as 1.5s.",
			},
			"asset_id": map[string]any{"type": "string", "description": "transcribe: the asset to transcribe."},
			"language": map[string]any{"type": "string", "description": "transcribe: the spoken language."},
			"model": map[string]any{
				"type":        "string",
				"enum":        transcribe.Models,
				"description": "transcribe: an installed whisper model; default is the workspace model.",
			},
		}
		args["additionalProperties"] = false
	}
}

// markUntrusted sets the annotation x-untrusted on every property of a result
// schema whose field carries text from media or from another actor (SDD-007
// section 8.2). The Go field tag is the one source, the same one the
// provenance test checks.
func markUntrusted(doc *openapi.Document) {
	seen := map[reflect.Type]bool{}

	var walk func(t reflect.Type)

	walk = func(t reflect.Type) {
		for t.Kind() == reflect.Pointer || t.Kind() == reflect.Slice || t.Kind() == reflect.Map || t.Kind() == reflect.Array {
			t = t.Elem()
		}

		if t.Kind() != reflect.Struct || seen[t] || t.PkgPath() == "" {
			return
		}

		seen[t] = true

		props, _ := doc.Components.Schemas[t.Name()]["properties"].(map[string]any)

		for f := range t.Fields() {
			if f.Tag.Get("untrusted") == "true" {
				name, _, _ := strings.Cut(f.Tag.Get("json"), ",")

				if prop, ok := props[name].(map[string]any); ok {
					prop[untrusted.Annotation] = true
				}
			}

			if f.Anonymous || f.IsExported() {
				walk(f.Type)
			}
		}
	}

	for m := range reflect.TypeFor[service.Service]().Methods() {
		for out := range m.Type.Outs() {
			walk(out)
		}
	}
}
