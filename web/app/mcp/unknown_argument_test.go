package mcp

import (
	"strings"
	"testing"
)

// An argument the schema does not have is named in the error, so an agent knows what to remove (VV-304).
func TestUnknownArgumentIsNamed(t *testing.T) {
	t.Parallel()

	svc := newService(t)
	cs := connect(t, svc, Options{}, "unknown-argument")

	for _, tc := range []struct {
		tool string
		args map[string]any
		name string
	}{
		{"project_list", map[string]any{"bogus": 1}, "bogus"},
		{"lint", map[string]any{"project_id": "prj_0000000000", "severty": "error"}, "severty"},
	} {
		res := call(t, cs, tc.tool, tc.args)
		text := textOf(t, res)

		if !res.IsError || !strings.Contains(text, "E_VALIDATION") {
			t.Fatalf("%s %v: %s, want E_VALIDATION", tc.tool, tc.args, text)
		}

		if !strings.Contains(text, `"`+tc.name+`"`) {
			t.Errorf("%s: %s, want the unknown argument %q named", tc.tool, text, tc.name)
		}
	}
}
