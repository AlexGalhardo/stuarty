package contract

import (
	"encoding/json"
	"strings"
	"testing"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/AlexGalhardo/stuarty/service"
	"github.com/AlexGalhardo/stuarty/web/contract/testenv"
)

// the three operations of a person (ADR-0051, SDD-025 section 9) with arguments an agent would send.
var recordingPersonOnly = []step{
	{
		Call: "RecordingStart",
		Args: json.RawMessage(`{"project_id":"prj_0000000001","source":{"kind":"display","id":"d1"},"fps":30}`),
	},
	{Call: "RecordingConfirm", Args: json.RawMessage(`{"recording_id":"rec_0000000001"}`)},
	{Call: "RecordingDismiss", Args: json.RawMessage(`{"recording_id":"rec_0000000001"}`)},
}

// SDD-007 T-118: an agent cannot start a capture, add a recording to the project or set it aside, on any surface:
// the local token that an agent holds is refused by the service with one stable code, over HTTP, in the CLI and in
// process, before the project, the disk or the helper is looked at, and the MCP server has no tool that does it.
func TestSecurity_T118_NoSurfaceLetsAnAgentStartACapture(t *testing.T) {
	for name, newDriver := range map[string]func(*testing.T, testenv.Options) driver{
		"in-process": newDirectWith, "http": newHTTPWith, "cli": newCLIWith,
	} {
		t.Run(name, func(t *testing.T) {
			d := newDriver(t, testenv.Options{})

			for _, s := range recordingPersonOnly {
				result, _, fail := d.do(t, s)

				if fail == nil || fail.Code != service.CodePrivilegedSession {
					t.Errorf("%s %s with the local token: result %s, error %+v; want E_PRIVILEGED_SESSION",
						d.name(), s.Call, result, fail)

					continue
				}

				if fail.Hint == "" {
					t.Errorf("%s %s: the refusal has no hint", d.name(), s.Call)
				}
			}
		})
	}

	t.Run("mcp", func(t *testing.T) {
		d := newMCP(t).(*mcpDriver)

		for _, s := range recordingPersonOnly {
			if tool, _, err := toolCall(s); err == nil {
				t.Errorf("%s is reachable through the tool %s", s.Call, tool)
			}
		}

		list, err := d.cs.ListTools(t.Context(), nil)
		if err != nil {
			t.Fatal(err)
		}

		for _, tool := range list.Tools {
			name := strings.ToLower(tool.Name)

			for _, word := range []string{"record", "capture", "screen"} {
				if strings.Contains(name, word) {
					t.Errorf("the tool %s is a surface of the screen capture", tool.Name)
				}
			}
		}

		// An agent that guesses a tool is refused by the protocol, with a stable error.
		for _, guess := range []struct {
			name string
			args map[string]any
		}{
			{"recording", map[string]any{"action": "start", "project_id": "prj_0000000001"}},
			{"recording_start", map[string]any{"project_id": "prj_0000000001"}},
			{"capture_start", map[string]any{"source": "d1"}},
		} {
			res, err := d.cs.CallTool(t.Context(), &sdk.CallToolParams{Name: guess.name, Arguments: guess.args})
			if err == nil && (res == nil || !res.IsError) {
				t.Errorf("the guessed tool %s answered %+v, want a refusal", guess.name, res)
			}
		}

		// job_cancel is the one way an agent touches a capture, and it only stops it: a recording id that does not
		// exist is not found, and the tool cannot start anything.
		res, err := d.cs.CallTool(t.Context(), &sdk.CallToolParams{
			Name: "job_cancel", Arguments: map[string]any{"job_id": "rec_0000000001"},
		})
		if err == nil && (res == nil || !res.IsError) {
			t.Errorf("job_cancel of a recording that does not exist answered %+v", res)
		}
	})
}
