package mcp

import (
	"encoding/json"
	"strings"
	"testing"
)

// VV-305: the description of job_start says that every field of a kind, connection_id and model of the
// generations included, goes inside args; the top level of the tool takes none of them.
func TestJobStartDescriptionSaysWhereTheGenerationFieldsGo(t *testing.T) {
	t.Parallel()

	defs, err := loadDefinitions()
	if err != nil {
		t.Fatal(err)
	}

	tool := defs["job_start"].tool

	for _, want := range []string{"fields go in args", "generations add connection_id, model:"} {
		if !strings.Contains(tool.Description, want) {
			t.Errorf("the description of job_start lacks %q: %s", want, tool.Description)
		}
	}

	if strings.Contains(tool.Description, "with connection_id, model:") {
		t.Errorf("the description still reads as if connection_id and model were top-level members")
	}

	var schema struct {
		Properties map[string]json.RawMessage `json:"properties"`
	}
	if err := json.Unmarshal(marshal(t, tool.InputSchema), &schema); err != nil || schema.Properties["args"] == nil {
		t.Fatalf("the input schema of job_start = %v, want properties with args", err)
	}

	for _, top := range []string{"connection_id", "model"} {
		if _, ok := schema.Properties[top]; ok {
			t.Errorf("job_start declares %s at the top level: the description says it goes in args", top)
		}
	}
}
