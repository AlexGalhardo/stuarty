package main

import (
	"bytes"
	"os"
	"strings"
	"testing"

	"github.com/AlexGalhardo/stuarty/web/app/mcp"
)

// The tool fails on a measure over its limit and prints every number it measured next to its limit.
func TestReportFailsOverTheBudget(t *testing.T) {
	t.Parallel()

	var out, errOut bytes.Buffer

	if code := report(&out, &errOut, []measure{{"a", "bytes", 10, 10}, {"b", "bytes", 11, 10}}); code != 1 {
		t.Errorf("exit %d, want 1 for a measure over its limit", code)
	}

	if !strings.Contains(out.String(), "OVER") || !strings.Contains(errOut.String(), "1 measure(s) over") {
		t.Errorf("stdout %q, stderr %q", out.String(), errOut.String())
	}

	out.Reset()
	errOut.Reset()

	if code := report(&out, &errOut, []measure{{"a", "bytes", 10, 10}}); code != 0 || errOut.Len() != 0 {
		t.Errorf("exit %d, stderr %q, want 0 for a measure at its limit", code, errOut.String())
	}
}

// The real surface is inside the budget, and the measure sees what a client receives: every tool of the list.
func TestTheSurfaceIsInsideTheBudget(t *testing.T) {
	t.Parallel()

	rows, err := measureSurface(t.Context())
	if err != nil {
		t.Fatal(err)
	}

	seen := map[string]bool{}

	for _, m := range rows {
		seen[m.what] = true

		if m.measured <= 0 {
			t.Errorf("%s measured %d", m.what, m.measured)
		}

		if m.over() {
			t.Errorf("%s is %d %s, at most %d", m.what, m.measured, m.unit, m.limit)
		}
	}

	for _, want := range []string{"tools/list, minified", "tool count", "first discovery call: command_help", "domain description: video"} {
		if !seen[want] {
			t.Errorf("the measure %q is missing", want)
		}
	}
}

// The limits written in the code are the ones written in the specification: a number changed in one place and
// not in the other fails here.
func TestLimitsAreInTheSpecification(t *testing.T) {
	t.Parallel()

	for file, wants := range map[string][]string{
		"../../../docs/sdds/003-agent-interfaces.md":  {"22,400 bytes", "1,700 bytes", "7,040 characters"},
		"../../../docs/prds/002-agent-operability.md": {"7,000 tokens", "22,400 bytes", "2,200 tokens"},
		"../../../docs/sdds/008-test-strategy.md":     {"7,000 tokens"},
	} {
		raw, err := os.ReadFile(file)
		if err != nil {
			t.Fatalf("the specification is not next to the code: %v", err)
		}

		text := strings.ReplaceAll(string(raw), "\r\n", "\n")

		for _, want := range wants {
			if !strings.Contains(text, want) {
				t.Errorf("%s does not say %q", file, want)
			}
		}
	}

	if mcp.MaxToolListBytes != 22_400 || mcp.MaxToolBytes != 1_700 || mcp.MaxDiscoveryChars != 7_040 ||
		mcp.MaxTools != 20 {
		t.Error("the limits in web/app/mcp/budget.go changed: change the specification and this test together")
	}
}
