package main

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"
)

const gateOut = `=== RUN   TestViewFrameColdP95
--- PASS: TestViewFrameColdP95 (4.10s)
=== RUN   TestScaleProjectViewTimeBudgets
--- FAIL: TestScaleProjectViewTimeBudgets (9.00s)
`

func TestGateRowsAreJudgedByWhatRan(t *testing.T) {
	t.Parallel()

	g := &Gate{"NFR-004", "./x", []string{"TestViewFrameColdP95", "TestScaleProjectViewTimeBudgets", "TestGone"}}
	rows := GateRows(g, gateOut+"--- SKIP: TestOther (0.00s)\n")

	want := []string{"pass", "fail", "missing"}
	for i, w := range want {
		if rows[i].Result != w {
			t.Errorf("%s = %s, want %s", rows[i].Test, rows[i].Result, w)
		}
	}

	skipped := GateRows(&Gate{"NFR-004", "./x", []string{"TestOther"}}, "--- SKIP: TestOther (0.00s)\n")
	if skipped[0].Result != "skip" || skipped[0].Pass() {
		t.Errorf("a skipped budget counts as judged: %+v", skipped[0])
	}
}

func TestGatedRunFailsOnASkippedOrFailedBudget(t *testing.T) {
	old := gateRunner
	t.Cleanup(func() { gateRunner = old })

	var env []string

	gateRunner = func(_ context.Context, _ string, tests []string) (string, error) {
		env = tests

		return "--- SKIP: " + tests[0] + " (0.00s)\n", nil
	}

	var out, errb bytes.Buffer

	code := run(context.Background(), []string{"-gated", "-only", "NFR-001"}, &out, &errb, fake(sample),
		func() time.Time { return time.Date(2026, 11, 20, 0, 0, 0, 0, time.UTC) })
	if code != 1 || !strings.Contains(errb.String(), "TestTransitionEmptyLintTimeBudget: skip") {
		t.Fatalf("a skipped gate passed: %d %s", code, errb.String())
	}

	if len(env) == 0 || !strings.Contains(out.String(), "Budgets in tests") {
		t.Errorf("the report has no table of the gates:\n%s", out.String())
	}

	gateRunner = func(_ context.Context, _ string, tests []string) (string, error) {
		return "--- PASS: " + tests[0] + " (0.10s)\n", nil
	}
	errb.Reset()

	code = run(context.Background(), []string{"-gated", "-only", "NFR-001"}, &out, &errb, fake(sample),
		func() time.Time { return time.Date(2026, 11, 20, 0, 0, 0, 0, time.UTC) })
	if code != 0 {
		t.Fatalf("a passing gate failed: %d %s", code, errb.String())
	}
}

// A requirement whose only budgets live in tests (NFR-002, NFR-004) is measured alone with -gated, so a failed
// budget can be measured again without the benchmarks; without -gated it is still no requirement of the table.
func TestGatedRunTakesARequirementWithOnlyBudgetsInTests(t *testing.T) {
	old := gateRunner
	t.Cleanup(func() { gateRunner = old })

	var ran []string

	gateRunner = func(_ context.Context, _ string, tests []string) (string, error) {
		ran = append(ran, tests...)

		return "--- PASS: " + tests[0] + " (0.10s)\n", nil
	}

	noBench := func(context.Context, string, string, string) (string, error) {
		t.Error("a benchmark ran for a requirement that has none")

		return "", nil
	}
	day := func() time.Time { return time.Date(2026, 11, 20, 0, 0, 0, 0, time.UTC) }

	var out, errb bytes.Buffer

	if code := run(context.Background(), []string{"-gated", "-only", "NFR-002"}, &out, &errb, noBench, day); code != 0 {
		t.Fatalf("the gate of NFR-002 alone: %d %s", code, errb.String())
	}

	if len(ran) != 1 || ran[0] != "TestEventsSlowReaderDoesNotSlowCommits" || !strings.Contains(out.String(), "NFR-002") {
		t.Errorf("ran %v, report:\n%s", ran, out.String())
	}

	if code := run(context.Background(), []string{"-only", "NFR-002"}, &out, &errb, noBench, day); code != 2 {
		t.Errorf("-only NFR-002 without -gated: %d, want 2", code)
	}
}
