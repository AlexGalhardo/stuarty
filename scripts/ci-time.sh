#!/usr/bin/env bash
# Wall time of the last N finished runs of the workflow CI on pull requests (NFR-042) and the
# largest tracked file: bash scripts/ci-time.sh [<runs>]
# The wall time of a run is the first job start to the last job end of its latest attempt, so
# the queue and the time a superseded run waited are not counted. A run that was cancelled
# (superseded by a newer push) never finished and is skipped. It prints one line per run with
# its slowest job, the median and the maximum, and exits 1 when the median is 15 minutes or
# more or a run took more than 20 minutes. Needs gh (authenticated) and git.
set -euo pipefail
n="${1:-10}"
repo="${GITHUB_REPOSITORY:-$(gh repo view --json nameWithOwner --jq .nameWithOwner)}"

ids="$(gh run list --workflow CI --event pull_request --status completed --limit $((n * 6)) \
	--json databaseId,conclusion --jq '[.[] | select(.conclusion != "cancelled")][:'"$n"'][] | .databaseId')"

durations=""
for id in $ids; do
	line="$(gh api "repos/$repo/actions/runs/$id/jobs?per_page=100&filter=latest" --jq '
		[.jobs[] | select(.started_at != null and .completed_at != null and .conclusion != "skipped")]
		| (map(.started_at | fromdateiso8601) | min) as $s
		| (map(.completed_at | fromdateiso8601) | max) as $e
		| (map({n: .name, d: ((.completed_at | fromdateiso8601) - (.started_at | fromdateiso8601))}) | max_by(.d)) as $j
		| "\($e - $s) \($j.d) \($j.n)"')"
	wall="${line%% *}"
	rest="${line#* }"
	slow="${rest%% *}"
	name="${rest#* }"
	awk -v id="$id" -v w="$wall" -v s="$slow" -v j="$name" \
		'BEGIN { printf "run %s: %.1f min, slowest job %.1f min (%s)\n", id, w / 60, s / 60, j }'
	durations="$durations$wall"$'\n'
done
durations="$(printf "%s" "$durations" | sort -n)"

count="$(printf "%s\n" "$durations" | grep -c . || true)"
if [ "$count" -eq 0 ]; then
	echo "ci-time: no finished pull request run of CI found" >&2
	exit 1
fi

median="$(printf "%s\n" "$durations" | awk '{ a[NR] = $1 } END { if (NR % 2) print a[(NR + 1) / 2]; else print (a[NR / 2] + a[NR / 2 + 1]) / 2 }')"
max="$(printf "%s\n" "$durations" | tail -n 1)"

awk -v c="$count" -v m="$median" -v x="$max" 'BEGIN { printf "ci-time: %s runs, median %.1f min, max %.1f min\n", c, m / 60, x / 60 }'

largest="$(git ls-files -z | xargs -0 stat -c '%s %n' 2>/dev/null | sort -n | tail -n 1 || true)"
if [ -n "$largest" ]; then
	echo "ci-time: largest tracked file ${largest%% *} bytes (${largest#* })"
fi

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
