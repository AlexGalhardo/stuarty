package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strings"
)

// Gate is an absolute time budget that lives in a Go test and runs only with STUARTY_PERF=1, on the idle
// machine (SDD-008 sections 10 and 13.1). Tests lists the names the run must find and pass.
type Gate struct {
	NFR     string
	Package string
	Tests   []string
}

// Gates is the list of the tests that judge a time on the wall clock.
var Gates = []Gate{
	{"NFR-004", "./engine/frame", []string{"TestViewFrameColdP95", "TestScaleProjectViewTimeBudgets"}},
	{"NFR-002", "./web/app/http", []string{"TestEventsSlowReaderDoesNotSlowCommits"}},
	{"NFR-001", "./core/lint", []string{"TestTransitionEmptyLintTimeBudget"}},
}

// GateRunner runs the tests of one gate with STUARTY_PERF=1 and returns the verbose output of go test.
type GateRunner func(ctx context.Context, pkg string, tests []string) (string, error)

// gateRunner is replaced by the tests.
var gateRunner GateRunner = goTestGate

func goTestGate(ctx context.Context, pkg string, tests []string) (string, error) {
	cmd := exec.CommandContext( //nolint:gosec // fixed program; the selection comes from the table in this package
		ctx, "go", "test", "-count=1", "-p", "1", "-v", "-run", "^("+strings.Join(tests, "|")+")$", pkg,
	)
	cmd.Env = append(os.Environ(), "STUARTY_PERF=1")
	out, err := cmd.CombinedOutput()

	return string(out), err
}

// GateRow is the outcome of one test of a gate: pass, fail, skip or missing (the test was not found).
type GateRow struct {
	Gate   *Gate
	Test   string
	Result string
}

// Pass says whether the budget was judged and met: a skipped or missing test is not a pass.
func (r GateRow) Pass() bool { return r.Result == "pass" }

var resultRe = regexp.MustCompile(`(?m)^--- (PASS|FAIL|SKIP): (\w+)`)

// GateRows reads the outcome of each test of the gate out of the verbose output of go test.
func GateRows(g *Gate, out string) []GateRow {
	seen := map[string]string{}
	for _, m := range resultRe.FindAllStringSubmatch(out, -1) {
		seen[m[2]] = strings.ToLower(m[1])
	}

	rows := make([]GateRow, 0, len(g.Tests))

	for _, name := range g.Tests {
		res, ok := seen[name]
		if !ok {
			res = "missing"
		}

		rows = append(rows, GateRow{Gate: g, Test: name, Result: res})
	}

	return rows
}

// runGates runs every gate of the requirement (every gate when only is empty). A test that fails to
// compile or run still yields rows: the budget was not judged.
func runGates(ctx context.Context, only string) []GateRow {
	var rows []GateRow

	for i := range Gates {
		g := &Gates[i]
		if only != "" && g.NFR != only {
			continue
		}

		out, _ := gateRunner(ctx, g.Package, g.Tests)
		rows = append(rows, GateRows(g, out)...)
	}

	return rows
}

// gatesMarkdown is the table of the gates for the report.
func gatesMarkdown(rows []GateRow) string {
	if len(rows) == 0 {
		return ""
	}

	var b strings.Builder

	b.WriteString("\n## Budgets in tests (`STUARTY_PERF=1`)\n\n")
	b.WriteString("| Requirement | Test | Result |\n| --- | --- | --- |\n")

	for _, r := range rows {
		fmt.Fprintf(&b, "| %s | `%s` (%s) | %s |\n", r.Gate.NFR, r.Test, r.Gate.Package, r.Result)
	}

	return b.String()
}
