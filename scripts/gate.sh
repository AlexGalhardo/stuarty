#!/usr/bin/env bash
# The full local release gate, used while the pull-request workflows are paused.
# Run it from the repository root (or a worktree root): bash scripts/gate.sh [<tag>]
# (or `task gate -- <tag>`). Each step logs to $TEMP/gate-<tag>/<step>.log; a failing step prints
# `<step>_fail` and its first error lines, the script keeps going and ends with GATE_FAIL=<n>.
# Set VUKEM_VIDEO_TEST_FFMPEG_DIR to an LGPL FFmpeg bin directory so the export tests do not skip.
set -uo pipefail

tag="${1:-local}"
L="${TEMP:-${TMPDIR:-/tmp}}/gate-$tag"
mkdir -p "$L"
fail=0

if [ -z "${VUKEM_VIDEO_TEST_FFMPEG_DIR:-}" ]; then
	echo "ffmpeg: VUKEM_VIDEO_TEST_FFMPEG_DIR unset, export tests may skip"
fi
export VUKEM_VIDEO_MIN_FREE_BYTES=1

step() {
	local name="$1"
	shift
	if "$@" >"$L/$name.log" 2>&1; then
		echo "$name ok"
	else
		echo "${name}_fail"
		grep -iE "error|fail|panic|DATA RACE|^---" "$L/$name.log" | head -20
		fail=$((fail + 1))
	fi
}

for os in linux darwin windows; do
	step "lint_$os" env GOOS=$os golangci-lint run --allow-serial-runners --timeout 25m ./...
done
step tidy go mod tidy -diff
step notices go run ./internal/tools/notices -check
step threat go run ./internal/tools/threatcheck
step ffpins go run ./internal/tools/ffmpegpins -check
step agentdocs go run ./web/cmd/gen-agent-docs --check
if command -v cargo >/dev/null; then
	step rust bash -c "cd renderer && cargo fmt --check && cargo clippy --locked --all-targets -- -D warnings && cargo test --locked && cargo deny check && cd .. && bash scripts/build-renderer.sh verify"
else
	echo "rust: cargo not installed, skipped"
fi
step test go test ./... -count=1 -timeout 60m
step race go test -race -short -count=1 -timeout 30m ./core/... ./service/... ./web/app/...
step cover bash -c "go test -short -count=1 -coverprofile='$L/coverage.out' ./... && go run ./internal/tools/covergate '$L/coverage.out'"

export VUKEM_VIDEO_E2E_HOME="${TEMP:-${TMPDIR:-/tmp}}/gate-e2e-$tag"
export VUKEM_VIDEO_E2E_PORT=3471
step sec go test -run TestSecurity_ ./... -count=1
step web bash -c "cd web && bun run lint && bun run typecheck && bun run check:i18n && bun run check:thin-client && bun run test && bun run check:contrast && bun run check:hooks"
step md bunx markdownlint-cli2@0.24.1 "**/*.md"
if command -v betterleaks >/dev/null; then
	step leaks betterleaks git --log-opts="origin/main..HEAD"
else
	echo "leaks: betterleaks not installed, skipped"
fi
step site bash -c "cd site && npm run check:last-updated && npm run check:mdx && npm run test:scripts && npm run build && npm run check:built && npm run reference:check && npm run check:command-mentions"
step commits bash .agents/skills/ci-preflight/scripts/check-commits.sh .

echo "GATE_FAIL=$fail"
[ "$fail" -eq 0 ]
