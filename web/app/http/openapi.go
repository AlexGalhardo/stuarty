package httpapi

import (
	"encoding/json"
	"net/http"
	"slices"

	"github.com/AlexGalhardo/vukem/openapi"
	"github.com/AlexGalhardo/vukem/web"
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
