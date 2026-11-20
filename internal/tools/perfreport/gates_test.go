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
