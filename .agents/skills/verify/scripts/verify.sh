#!/usr/bin/env bash
# The verification loop of a change (skills/verify/SKILL.md). Run it from the repository or worktree root:
#
#   bash .agents/skills/verify/scripts/verify.sh [--quick|--full] [--base <ref>]
#
# --quick (default): commit messages, flaky patterns, and the lint, build and tests of what the change
#                    touches (Go packages, web/, site/ and the generated docs).
# --full:            --quick plus the full local gate (scripts/gate.sh), the end-to-end smoke test
#                    (scripts/smoke-e2e.sh) and the two scripted recipes (scripts/recipe-commercial.sh and
#                    scripts/recipe-long-video.sh). Required before opening or updating a pull request.
#
# The checks come from the default branch, never from the change under test: the scripts are read from
# <base> (default origin/main) with `git show` and run against the working tree. A change that edits a check
# (this skill, REVIEW.md, rules/, workflows, gate.sh, smoke-e2e.sh, CODEOWNERS) is reported as CHECKS_CHANGED
# and ends with exit 3: a human reviews it, an agent never approves its own checks.
# Logs: $TEMP/verify-<time>/<step>.log. Last line: VERIFY_OK, VERIFY_FAIL=<n> or VERIFY_NEEDS_HUMAN.
set -uo pipefail

mode=quick
base=origin/main
while [ $# -gt 0 ]; do
	case "$1" in
	--quick) mode=quick ;;
	--full) mode=full ;;
	--base) base="$2"; shift ;;
	*) echo "usage: verify.sh [--quick|--full] [--base <ref>]" >&2; exit 2 ;;
	esac
	shift
done

root="$(git rev-parse --show-toplevel)" || exit 2
cd "$root" || exit 2
git fetch -q origin main 2>/dev/null || echo "verify: could not fetch origin/main, using the local ref"

# Run the copy of this script that is on the default branch, so a change cannot loosen its own checks.
if [ -z "${VERIFY_FROM_BASE:-}" ]; then
	# Git Bash on Windows rewrites an argument shaped like <rev>:.<path> (MSYS path conversion), so the
	# <rev>:<path> arguments below turn conversion off for the git call alone (VV-229). The variable is
	# ignored elsewhere. Whether a file is on the base is asked with ls-tree, whose arguments are never
	# converted, so a check that is there but cannot be read stops the run instead of being replaced by the
	# working copy.
	base_has() {
		local out
		out="$(git ls-tree --name-only "$base" -- "$1")" || { echo "verify: could not list $1 on $base" >&2; exit 2; }
		[ -n "$out" ]
	}
	base_show() { MSYS_NO_PATHCONV=1 MSYS2_ARG_CONV_EXCL='*' git show "$base:$1"; }
	# A base that does not resolve is an error, never a reason to run the working copy.
	if ! git rev-parse --verify -q "$base^{commit}" >/dev/null; then
		echo "verify: the base $base is not a commit; cannot read the checks from it" >&2
		exit 2
	fi
	if base_has .agents/skills/verify/scripts/verify.sh; then
		checks="$(mktemp -d)" || exit 2
		for f in .agents/skills/verify/scripts/verify.sh .agents/skills/verify/scripts/check-flaky-patterns.sh \
			.agents/skills/ci-preflight/scripts/check-commits.sh scripts/gate.sh scripts/smoke-e2e.sh; do
			mkdir -p "$checks/$(dirname "$f")"
			if base_has "$f"; then
				base_show "$f" >"$checks/$f" || { echo "verify: could not read $f from $base" >&2; exit 2; }
			fi
		done
		VERIFY_FROM_BASE="$checks" exec bash "$checks/.agents/skills/verify/scripts/verify.sh" "--$mode" --base "$base"
	fi
	echo "verify: the checks are not on $base yet; running the working copy (first rollout only)"
	VERIFY_FROM_BASE="$root"
fi
checks="$VERIFY_FROM_BASE"
# A script missing from the base copy falls back to the working tree.
script() { if [ -f "$checks/$1" ]; then echo "$checks/$1"; else echo "$root/$1"; fi; }

L="${TEMP:-${TMPDIR:-/tmp}}/verify-$(date +%Y%m%d-%H%M%S)"
mkdir -p "$L"
fail=0
step() {
	local name="$1"
	shift
	if "$@" >"$L/$name.log" 2>&1; then
		echo "$name ok"
	else
		echo "${name}_fail (log: $L/$name.log)"
		grep -iE "error|fail|panic|DATA RACE|^---|:[0-9]+:" "$L/$name.log" | head -20
		fail=$((fail + 1))
	fi
}

mb="$(git merge-base "$base" HEAD)"
changed="$( (git diff --name-only "$mb"...HEAD; git diff --name-only HEAD; git ls-files --others --exclude-standard) | sort -u)"
echo "verify: $mode, base $base ($(git rev-parse --short "$mb")), $(echo "$changed" | grep -c .) file(s) changed"

protected='^(\.agents/skills/verify/|\.agents/skills/ci-preflight/|\.agents/rules/|REVIEW\.md$|\.github/workflows/|\.github/CODEOWNERS$|scripts/gate\.sh$|scripts/smoke-e2e\.sh$|web/e2e/playwright\.config\.ts$|web/vitest\.config\.ts$)'
touched_checks="$(echo "$changed" | grep -E "$protected" || true)"
if [ -n "$touched_checks" ]; then
	echo "CHECKS_CHANGED: this change edits the checks themselves; a human must review it:"
	echo "$touched_checks" | sed 's/^/  /'
fi

step commits bash "$(script .agents/skills/ci-preflight/scripts/check-commits.sh)"
step flaky bash "$(script .agents/skills/verify/scripts/check-flaky-patterns.sh)" "$base"

export STUARTY_MIN_FREE_BYTES=1
go_files="$(echo "$changed" | grep -E '\.go$' | while read -r f; do [ -f "$f" ] && echo "$f"; done)"
if [ -z "$go_files" ] && echo "$changed" | grep -qE '(^|/)go\.(mod|sum)$'; then
	# Only module files changed (a go.mod or go.sum added, edited or deleted): there is no package to vet or
	# test, but the root module and every changed module that still exists must build (VV-230). A module whose
	# go.mod was deleted is never entered; whatever is left of it now builds with the root module.
	gomods="$( (echo .; echo "$changed" | grep -E '(^|/)go\.(mod|sum)$' | while read -r f; do
		if [ -f "$(dirname "$f")/go.mod" ]; then dirname "$f"; fi
	done) | sort -u)"
	for m in $gomods; do
		tag=""
		[ "$m" = . ] || tag="-$(echo "$m" | tr '/' '-')"
		step "go-build$tag" go -C "$m" build ./...
	done
elif [ -n "$go_files" ]; then
	step gofmt bash -c "out=\$(gofmt -l $(echo "$go_files" | tr '\n' ' ')); [ -z \"\$out\" ] || { echo \"\$out\"; exit 1; }"
	# A nested module (desktop/shell/app has its own go.mod) is built, vetted, tested and linted inside it.
	modof() {
		local d="$1"
		while [ "$d" != . ] && [ ! -f "$d/go.mod" ]; do d="$(dirname "$d")"; done
		echo "$d"
	}
	mods="$(echo "$go_files" | while read -r f; do modof "$(dirname "$f")"; done | sort -u)"
	for m in $mods; do
		pkgs=""
		for d in $(echo "$go_files" | while read -r f; do dirname "$f"; done | sort -u); do
			[ "$(modof "$d")" = "$m" ] || continue
			if [ "$m" = . ]; then pkgs="$pkgs ./$d"; elif [ "$d" = "$m" ]; then pkgs="$pkgs ."; else pkgs="$pkgs ./${d#"$m"/}"; fi
		done
		tag=""
		[ "$m" = . ] || tag="-$(echo "$m" | tr '/' '-')"
		step "go-build$tag" go -C "$m" build ./...
		step "go-vet$tag" go -C "$m" vet $pkgs
		step "go-test$tag" go -C "$m" test -count=1 $pkgs
		if command -v golangci-lint >/dev/null 2>&1; then step "golangci$tag" bash -c "cd '$m' && golangci-lint run $pkgs"; fi
	done
fi
if echo "$changed" | grep -qE '^(web/app/|core/schema/|web/app/agentdocs/)'; then
	step agent-docs go run ./web/cmd/gen-agent-docs -check
fi
if echo "$changed" | grep -qE '^web/(resources|scripts|e2e|package\.json|biome\.json)'; then
	step web-lint bash -c "cd web && bun run lint"
	step web-typecheck bash -c "cd web && bun run typecheck"
	step web-thin-client bash -c "cd web && bun run check:thin-client"
	step web-i18n bash -c "cd web && bun run check:i18n"
	step web-unit bash -c "cd web && bunx vitest run"
fi
if echo "$changed" | grep -qE '^(site/|web/app/|core/schema/)'; then
	step site-last-updated bash -c "cd site && bun run --silent check:last-updated"
	step site-reference bash -c "cd site && bun run --silent reference:check"
fi
if [ "$mode" = full ]; then
	step gate bash "$(script scripts/gate.sh)" verify
	grep -q "GATE_FAIL=0" "$L/gate.log" || { echo "gate: $(grep GATE_FAIL "$L/gate.log")"; }
	step smoke-e2e bash "$(script scripts/smoke-e2e.sh)"
	# The recipes of epic 39: a commercial in three aspect ratios and a long video with its short (SDD-018 section 17).
	step recipe-commercial bash "$root/scripts/recipe-commercial.sh"
	step recipe-long-video bash "$root/scripts/recipe-long-video.sh"
fi

echo "logs: $L"
if [ "$fail" -gt 0 ]; then echo "VERIFY_FAIL=$fail"; exit 1; fi
if [ -n "$touched_checks" ]; then echo VERIFY_NEEDS_HUMAN; exit 3; fi
echo VERIFY_OK
