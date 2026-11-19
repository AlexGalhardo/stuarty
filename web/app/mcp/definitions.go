package mcp

import (
	"embed"
	"encoding/json"
	"fmt"
	"strings"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/santhosh-tekuri/jsonschema/v6"
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
	"font_import", "library",
}

// definition is a tool definition and its compiled input schema.
type definition struct {
	tool  *sdk.Tool
	input *jsonschema.Schema
}

// loadDefinitions reads every definition in order. The definitions are the same on every build: the operations
// of a domain are not in them but in command_help (SDD-003 section 4.8).
func loadDefinitions() (map[string]*definition, error) {
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

		compiled, err := compileInput(&t)
		if err != nil {
			return nil, fmt.Errorf("mcp: input schema of %s: %w", name, err)
		}

		out[name] = &definition{tool: &t, input: compiled}
	}

	return out, nil
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
