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
patterns='t\.Skip(f|Now)?\(|\b(test|it|describe)\.(skip|fixme)\b|\.only\(|\bretries\s*:|\bretry\(|--retries|-count=([2-9]|[1-9][0-9])|time\.Sleep\(|waitForTimeout\(|setTimeout\('

bad=0
# Added lines with their new line number, from the committed range and from the working tree.
for range in "$mb...HEAD" ""; do
	if [ -n "$range" ]; then diff_cmd=(git diff -U0 --no-color "$range"); else diff_cmd=(git diff -U0 --no-color HEAD); fi
	while IFS=$'\t' read -r file line text; do
		[ -n "$file" ] || continue
		echo "$file" | grep -qE "$tests" || continue
		echo "$text" | grep -qE "$patterns" || continue
		if echo "$text" | grep -q "verify:env-skip [^ ]" && echo "$text" | grep -qE 't\.Skip|\.skip'; then
			continue
		fi
		echo "$file:$line: $text"
		bad=$((bad + 1))
	done < <("${diff_cmd[@]}" | awk '
		/^\+\+\+ b\// { file = substr($0, 7); next }
		/^@@/ { split($3, a, ","); n = substr(a[1], 2) + 0; next }
		/^\+/ && !/^\+\+\+/ { printf "%s\t%d\t%s\n", file, n, substr($0, 2); n++; next }
	')
done

if [ "$bad" -gt 0 ]; then
	echo "FLAKY_FAIL=$bad: fix the cause; never skip, retry or sleep (rules/ci.md). An environment skip needs 'verify:env-skip <reason>' on the line."
	exit 1
fi
echo FLAKY_OK
