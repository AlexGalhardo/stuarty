#!/usr/bin/env bash
# Regression guard of the benchmarks (TASK-0136, SDD-004 section 12, SDD-009 section 13):
#   bash scripts/bench-gate.sh <pr|nightly> <limits file> <go test -bench output>
# Each line of the limits file is "<profile> <Benchmark name> <unit> <min|max> <limit>": the
# benchmark must report <unit> at least (min) or at most (max) <limit>. The profile `pr`
# checks the lines tagged `pr`, `nightly` checks every line. A limit with no result in the
# output fails too: a benchmark that was renamed or skipped must not leave its guard silent.
# Prints one line per limit and exits 1 on any regression.
set -euo pipefail
profile="${1:?profile (pr or nightly)}"
limits="${2:?limits file}"
output="${3:?benchmark output}"

awk -v profile="$profile" '
	FNR == NR {
		if ($0 ~ /^[[:space:]]*(#|$)/ || ($1 != profile && profile != "nightly")) next
		n++; name[n] = $2; unit[n] = $3; dir[n] = $4; lim[n] = $5
		next
	}
	/^Benchmark/ {
		base = $1; sub(/-[0-9]+$/, "", base)
		for (i = 1; i <= n; i++) {
			if (name[i] != base) continue
			for (f = 3; f < NF; f++) if ($(f + 1) == unit[i]) { val[i] = $f; seen[i] = 1 }
		}
	}
	END {
		bad = 0
		for (i = 1; i <= n; i++) {
			if (!seen[i]) { printf "FAIL %s: no %s in the output\n", name[i], unit[i]; bad = 1; continue }
			ok = (dir[i] == "min") ? (val[i] + 0 >= lim[i] + 0) : (val[i] + 0 <= lim[i] + 0)
			printf "%s %s: %s %s, %s %s\n", ok ? "ok  " : "FAIL", name[i], val[i], unit[i], dir[i], lim[i]
			if (!ok) bad = 1
		}
		if (n == 0) { print "FAIL: no limit for the profile " profile; bad = 1 }
		exit bad
	}
' "$limits" "$output"
