#!/usr/bin/env bash
# Helpers shared by the tests of deploy/. Sourced, never run.
# The tests use the stand-ins of deploy/test/stub/ for docker and curl, so they need no daemon,
# no image and no network. They never touch a real workspace: every folder is a mktemp folder.

TEST_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_DIR="$(cd "$TEST_DIR/../.." && pwd)"
REAL_DOCKER="$(command -v docker 2>/dev/null || true)"

pass() { printf 'ok   %s\n' "$*"; }
fail() { printf 'FAIL %s\n' "$*" >&2; exit 1; }

assert() { # assert DESCRIPTION COMMAND...
	local what="$1"
	shift
	if "$@" >/dev/null 2>&1; then pass "$what"; else fail "$what"; fi
}

# new_work makes a scratch folder with a copy of deploy/ in it (never its .env, secrets or tests'
# state) and sets WORK, DEP and STUB_STATE. The stand-ins come first on the PATH.
new_work() {
	WORK="$(mktemp -d)"
	DEP="$WORK/repo/deploy"
	mkdir -p "$DEP" "$WORK/state"
	cp "$REPO_DIR"/deploy/{compose.yml,Caddyfile,.env.example,lib.sh,install.sh,update.sh,backup.sh,restore.sh} "$DEP/" 2>/dev/null || true
	STUB_STATE="$WORK/state"
	export STUB_STATE
	PATH="$TEST_DIR/stub:$PATH"
	export PATH
	trap 'rm -rf "$WORK"' EXIT
}

# build_binary prints the path of a stuarty binary: $STUARTY_TEST_BIN, or one built from this checkout.
build_binary() {
	if [ -n "${STUARTY_TEST_BIN:-}" ]; then
		printf '%s' "$STUARTY_TEST_BIN"
		return
	fi
	local out="$WORK/stuarty"
	case "$(uname -s)" in MINGW* | MSYS* | CYGWIN*) out="$out.exe" ;; esac
	(cd "$REPO_DIR" && go build -o "$out" ./web/cmd/app) >&2 || fail "go build of the binary"
	printf '%s' "$out"
}

on_windows() { case "$(uname -s)" in MINGW* | MSYS* | CYGWIN*) return 0 ;; esac; return 1; }

mode_of() { stat -c %a "$1" 2>/dev/null || stat -f %Lp "$1"; }

tree_sums() { # a stable listing of the content and the mode of every file under $1
	(cd "$1" && find . -type f | LC_ALL=C sort | while read -r f; do printf '%s %s\n' "$(sha256sum "$f" | cut -d' ' -f1)" "$f"; done)
}
