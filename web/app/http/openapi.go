package httpapi

import (
	"encoding/json"
	"net/http"
	"reflect"
	"slices"
	"strings"

	"github.com/AlexGalhardo/vukem/openapi"
	"github.com/AlexGalhardo/vukem/web"

	"github.com/AlexGalhardo/vukem-video/engine/transcribe"
	"github.com/AlexGalhardo/vukem-video/service"
	"github.com/AlexGalhardo/vukem-video/service/untrusted"
)

// Info is the base of the OpenAPI document: the problem members of the error
// envelope (SDD-003 section 6.7 item 3) and the bearer scheme.
func Info() openapi.Info {
	return openapi.Info{
		Title:       "Vukem Video",
		Description: "The HTTP API of Vukem Video. Every operation is the HTTP form of one service operation.",
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
				"description": "export: the file to write; wav and aac are audio only.",
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
