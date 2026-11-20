package mcp

import (
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

	raw := string(marshal(t, tool.InputSchema))
	for _, top := range []string{`"connection_id"`, `"model"`} {
		if strings.Contains(strings.SplitN(raw, `"args"`, 2)[0], top) {
			t.Errorf("job_start declares %s at the top level: the description says it goes in args", top)
		}
	}
}
