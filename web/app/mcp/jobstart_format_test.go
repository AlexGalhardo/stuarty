package mcp

import (
	"strings"
	"testing"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

// TestJobStartFormatIsCheckedBySchema: job_start args name the export format, and a format outside
// mp4, gif, wav and aac is refused before a job exists (SDD-003 section 2.8).
func TestJobStartFormatIsCheckedBySchema(t *testing.T) {
	t.Parallel()

	cs := connect(t, newService(t), Options{}, "claude-code")

	res, err := cs.CallTool(t.Context(), &sdk.CallToolParams{Name: "job_start", Arguments: map[string]any{
		"project_id": "prj_7k2m9x4q1b", "kind": "export", "args": map[string]any{"format": "avi"},
	}})
	msg := ""
	if err != nil {
		msg = err.Error()
	} else if res != nil && res.IsError {
		msg = textOf(t, res)
	}

	t.Log(msg)

	if !strings.Contains(msg, "format") {
		t.Fatalf("a format outside the enum was not refused by the schema: %q", msg)
	}
}
