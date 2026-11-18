package mcp

import (
	"embed"
	"encoding/json"
	"fmt"
	"slices"
	"strings"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/santhosh-tekuri/jsonschema/v6"

	"github.com/AlexGalhardo/vukem-video/core/command"
)

// The definitions are the objects of SDD-003 section 4.2, one file per tool,
// copied from that section; TestDefinitionsMatchTheSDD fails when they drift.
// They are static: nothing in a call changes what tools/list returns (ADR-0020).
//
//go:embed definitions/*.json
var definitionFiles embed.FS

// order is the order of tools/list: the table of SDD-003 section 4.1, so prompt
// caches hit. These are the tools whose Service operations exist in the build.
var order = []string{
	"project_list", "project_create", "project_get", "project_trash", "edit", "command_help", "history",
	"media_import", "media_inspect", "view", "lint", "job_start", "job_get", "job_cancel", "template",
	"font_import",
}

// definition is a tool definition and its compiled input schema.
type definition struct {
	tool  *sdk.Tool
	input *jsonschema.Schema
}

// loadDefinitions reads every definition in order. The op enum of edit is cut to
// the commands the registry has, the phase filter of SDD-003 section 4.1.
func loadDefinitions(reg *command.Registry) (map[string]*definition, error) {
	out := make(map[string]*definition, len(order))

	for _, name := range order {
		raw, err := definitionFiles.ReadFile("definitions/" + name + ".json")
		if err != nil {
			return nil, fmt.Errorf("mcp: definition of %s: %w", name, err)
		}

		var t sdk.Tool
		if err := json.Unmarshal(raw, &t); err != nil {
			return nil, fmt.Errorf("mcp: definition of %s: %w", name, err)
		}

		if t.Name != name {
			return nil, fmt.Errorf("mcp: definitions/%s.json defines %q", name, t.Name)
		}

		if name == "edit" {
			filterOps(&t, reg)
		}

		compiled, err := compileInput(&t)
		if err != nil {
			return nil, fmt.Errorf("mcp: input schema of %s: %w", name, err)
		}

		out[name] = &definition{tool: &t, input: compiled}
	}

	return out, nil
}

// filterOps keeps, in the order of the definition, the operations the build has.
func filterOps(t *sdk.Tool, reg *command.Registry) {
	have := reg.Names()

	schema, _ := t.InputSchema.(map[string]any)
	props, _ := schema["properties"].(map[string]any)
	ops, _ := props["operations"].(map[string]any)
	items, _ := ops["items"].(map[string]any)
	itemProps, _ := items["properties"].(map[string]any)
	op, _ := itemProps["op"].(map[string]any)
	enum, _ := op["enum"].([]any)

	kept := make([]any, 0, len(enum))

	for _, e := range enum {
		if name, ok := e.(string); ok && slices.Contains(have, name) {
			kept = append(kept, e)
		}
	}

	op["enum"] = kept
}

func compileInput(t *sdk.Tool) (*jsonschema.Schema, error) {
	raw, err := json.Marshal(t.InputSchema)
	if err != nil {
		return nil, err
	}

	doc, err := jsonschema.UnmarshalJSON(strings.NewReader(string(raw)))
	if err != nil {
		return nil, err
	}

	c := jsonschema.NewCompiler()
	if err := c.AddResource("tool.json", doc); err != nil {
		return nil, err
	}

	return c.Compile("tool.json")
}
