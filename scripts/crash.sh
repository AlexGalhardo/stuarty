#!/usr/bin/env bash
# Runs every kill point of the product (SDD-008 section 13.3):
#   bash scripts/crash.sh [count, default 1]
# The commit path, the bundle export and import, the migration and the emptying of the trash are killed by the
# tests of web/app/servicetest (build tag crashtest: crashpoint.Hit ends the child with exit code 137), and a real
# `serve` is killed in the middle of concurrent commits; the vault is killed by internal/tools/crashharness, which
# also checks that every crash point named in the code has such a test. Exits 1 when any of them fails.
set -uo pipefail

count="${1:-1}"
failed=0

run() {
	echo "== $*"
	"$@" || failed=1
}

run go test -tags crashtest -count="$count" -timeout 60m ./web/app/servicetest -run 'TestCrash|TestKill'
run go test -count="$count" -timeout 10m ./internal/tools/crashharness

exit "$failed"
