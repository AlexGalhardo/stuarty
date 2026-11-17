#!/usr/bin/env bash
# Refuses a change that hides a flaky or failing test instead of fixing it (rules/ci.md, REVIEW.md):
#
#   bash .agents/skills/verify/scripts/check-flaky-patterns.sh [<base ref>]
#
# It reads only the lines the change ADDS (git diff <merge-base>...HEAD plus the working tree) in test
# files and in the Playwright and Vitest configs, and fails on:
#   - a skipped or focused test: t.Skip, t.Skipf, test.skip, it.skip, describe.skip, test.fixme, .only(
#   - a retry: retries:, retry(, --retries, -count=N with N > 1 used to rerun
#   - a sleep in a test: time.Sleep(, waitForTimeout(, setTimeout( , new Promise(r => setTimeout
# A skip that depends on the environment (an OS, a privilege, a missing tool) is allowed only when the same
# line carries the marker `verify:env-skip <reason>`; it is never allowed for a test that fails sometimes.
# Prints FLAKY_OK, or every offending line as file:line and exits 1.
set -euo pipefail

base="${1:-origin/main}"
mb="$(git merge-base "$base" HEAD 2>/dev/null || echo "$base")"

tests='(_test\.go|\.test\.tsx?|\.spec\.tsx?|/e2e/.*\.tsx?|playwright\.config\.ts|vitest\.config\.ts)$'
# A timeout limit (test.setTimeout, page.setDefaultTimeout) is not a sleep, and retries: 0 is not a retry.
patterns='t\.Skip(f|Now)?\(|\b(test|it|describe)\.(skip|fixme)\b|\.only\(|\bretries\s*:\s*([1-9]|process)|\bretry\(|--retries|-count=([2-9]|[1-9][0-9])|time\.Sleep\(|waitForTimeout\(|(^|[^.A-Za-z_])setTimeout\('

# One pass: the added lines (committed range and working tree) of test files, as file:line: text.
added() {
	{ git diff -U0 --no-color "$mb...HEAD"; git diff -U0 --no-color HEAD; } | TESTS="$tests" awk '
		/^\+\+\+ / { file = substr($0, 7); keep = (file ~ ENVIRON["TESTS"]); next }
		/^@@/ { split($3, a, ","); n = substr(a[1], 2) + 0; next }
		/^\+/ { if (keep) printf "%s:%d: %s\n", file, n, substr($0, 2); n++; next }
	'
}
hits="$(added | grep -E -- "$patterns" || true)"
# The env-skip marker excuses only a skip, never a retry or a sleep on the same line.
hits="$(printf '%s\n' "$hits" | grep -v '^$' | awk '!(/verify:env-skip [^ ]/ && /(t\.Skip|\.skip)/)' || true)"
bad=0
[ -z "$hits" ] || { printf '%s\n' "$hits"; bad="$(printf '%s\n' "$hits" | grep -c .)"; }

if [ "$bad" -gt 0 ]; then
	echo "FLAKY_FAIL=$bad: fix the cause; never skip, retry or sleep (rules/ci.md). An environment skip needs 'verify:env-skip <reason>' on the line."
	exit 1
fi
echo FLAKY_OK
