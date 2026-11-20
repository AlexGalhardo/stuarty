// Package perfgate separates the tests that judge a time on the wall clock from the tests that judge
// behaviour (SDD-008 section 10).
//
// An absolute budget (a p95 in milliseconds, a cold start) measures the machine as much as the code:
// under the parallel suite, or with other programs running, it fails for a reason that is not in the
// code. Such a test calls Require and runs only when STUARTY_PERF=1 is set, on the idle machine, by the
// orchestrator (SDD-008 section 13.1). The functional assertions of the same scenario run always, in a
// test of their own.
package perfgate

import (
	"os"
	"testing"
)

// Env is the variable that turns the absolute budgets on.
const Env = "STUARTY_PERF"

// Enabled says whether the absolute budgets run.
func Enabled() bool { return os.Getenv(Env) == "1" }

// Require skips the test unless the absolute budgets are on.
func Require(tb testing.TB) {
	tb.Helper()

	if !Enabled() {
		tb.Skipf("an absolute time budget: runs only on the idle machine with %s=1 (SDD-008 section 10)", Env)
	}
}
