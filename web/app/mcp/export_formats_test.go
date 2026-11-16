package mcp

import (
	"encoding/json"
	"slices"
	"testing"

	"github.com/AlexGalhardo/vukem-video/service"
)

// TestJobStartFormatsMatchTheService fails when the hand-written job_start definition drifts
// from service.ExportFormats (AGENTS.md rule 3: names come from one place).
func TestJobStartFormatsMatchTheService(t *testing.T) {
	raw, err := definitionFiles.ReadFile("definitions/job_start.json")
	if err != nil {
		t.Fatal(err)
	}

	var found [][]string

	var walk func(v any)
	walk = func(v any) {
		switch x := v.(type) {
		case map[string]any:
			for k, child := range x {
				if k == "format" {
					if m, ok := child.(map[string]any); ok {
						if enum, ok := m["enum"].([]any); ok {
							var names []string
							for _, e := range enum {
								s, _ := e.(string)
								names = append(names, s)
							}
							found = append(found, names)
						}
					}
				}
				walk(child)
			}
		case []any:
			for _, child := range x {
				walk(child)
			}
		}
	}

	var doc any
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	walk(doc)

	if len(found) == 0 {
		t.Fatal("job_start.json has no format enum")
	}

	for _, names := range found {
		if !slices.Equal(names, service.ExportFormats) {
			t.Errorf("job_start.json format enum = %v, want service.ExportFormats %v", names, service.ExportFormats)
		}
	}
}
