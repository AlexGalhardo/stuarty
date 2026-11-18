package contract

import (
	"strings"
	"testing"

	"github.com/AlexGalhardo/vukem-video/web/app/cli"
)

// RF-131, T-28: no MCP tool and no CLI command can delete data for good. The
// only operation that does, TrashEmpty, has an HTTP route and nothing else.
func TestSecurity_T28_NoIrreversibleOperationOnAgentSurfaces(t *testing.T) {
	t.Parallel()

	tools := listedTools(t)
	commands, _ := cli.CommandNames()

	for _, name := range append(append([]string{}, tools...), commands...) {
		low := strings.ToLower(name)
		if strings.Contains(low, "empty") || strings.Contains(low, "purge") || strings.Contains(low, "delete") {
			t.Errorf("the agent surface %q looks like a permanent deletion", name)
		}
	}

	for _, r := range parity {
		if r.httpOnly && (r.tool != "" || r.command != "") {
			t.Errorf("%s is HTTP only but names a tool or a command", r.method)
		}

		if r.method == "TrashEmpty" && !r.httpOnly {
			t.Error("TrashEmpty must be HTTP only")
		}
	}
}
