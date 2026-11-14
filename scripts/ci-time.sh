#!/usr/bin/env bash
# Median and maximum wall time of the last N runs of the workflow CI on pull
# requests (NFR-042): bash scripts/ci-time.sh [<runs>]
# Needs gh (authenticated). It exits 1 when the median is 15 minutes or
# more, or when a run took more than 20 minutes.
set -euo pipefail
n="${1:-10}"

durations="$(gh run list --workflow CI --event pull_request --status completed --limit "$n" \
	--json startedAt,updatedAt --jq ".[] | ((.updatedAt | fromdateiso8601) - (.startedAt | fromdateiso8601))" | sort -n)"

count="$(printf "%s\n" "$durations" | grep -c . || true)"
if [ "$count" -eq 0 ]; then
	echo "ci-time: no completed pull request run of CI found" >&2
	exit 1
fi

median="$(printf "%s\n" "$durations" | awk "{ a[NR] = \$1 } END { if (NR % 2) print a[(NR + 1) / 2]; else print (a[NR / 2] + a[NR / 2 + 1]) / 2 }")"
max="$(printf "%s\n" "$durations" | tail -n 1)"

awk -v c="$count" -v m="$median" -v x="$max" 'BEGIN { printf "ci-time: %s runs, median %.1f min, max %.1f min\n", c, m / 60, x / 60 }'

if [ "$count" -lt "$n" ]; then
	echo "ci-time: only $count runs, the budget is checked over $n" >&2
fi
if awk "BEGIN { exit !($median >= 900) }"; then
	echo "ci-time: the median is 15 minutes or more (NFR-042)" >&2
	exit 1
fi
if awk "BEGIN { exit !($max > 1200) }"; then
	echo "ci-time: a run took more than 20 minutes (NFR-042)" >&2
	exit 1
fi
