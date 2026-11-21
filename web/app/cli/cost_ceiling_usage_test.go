package cli

import (
	"strings"
	"testing"
)

// The usage line of cost:ceiling names every flag the command takes (VV-309).
func TestCostCeilingUsageNamesEveryFlag(t *testing.T) {
	t.Parallel()

	line := strings.Join(costCeilingCmd.usage, "\n")

	for _, f := range costCeilingCmd.flags {
		if !strings.Contains(line, "--"+f.Name) {
			t.Errorf("the usage %q does not name --%s", line, f.Name)
		}
	}
}
