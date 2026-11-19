#!/usr/bin/env bash
# Installs the macOS package, runs the launcher of the app, runs the uninstall script, and checks
# what must hold (SDD-009 section 10.4). Needs sudo (a macOS runner has it).
#
#   smoke-test.sh <stuarty_..._darwin_arm64.pkg>
set -euo pipefail

[ $# -eq 1 ] || { echo "usage: smoke-test.sh <package>" >&2; exit 2; }
pkg="$(cd "$(dirname "$1")" && pwd)/$(basename "$1")"

pass() { echo "ok: $*"; }
fail() { echo "smoke test failed: $*" >&2; exit 1; }

workspace="$(mktemp -d)"
echo "keep me" >"$workspace/mine.txt"
export STUARTY_HOME="$workspace" STUARTY_NO_BROWSER=1
cleanup() { pkill -x stuarty 2>/dev/null || true; rm -rf "$workspace"; }
trap cleanup EXIT

sudo installer -pkg "$pkg" -target /
pass "installer accepts the package"
[ -x /usr/local/bin/stuarty ] || fail "no /usr/local/bin/stuarty"
app="/Applications/Stuarty.app"
[ -x "$app/Contents/MacOS/stuarty-launcher" ] || fail "no launcher in $app"
plutil -lint "$app/Contents/Info.plist" >/dev/null || fail "Info.plist is not valid"
[ -f "$app/Contents/Resources/stuarty.icns" ] || fail "no icon in $app"
[ -x /usr/local/share/stuarty/uninstall.sh ] || fail "no uninstall script"
pass "the program, the launcher app and the uninstall script are installed"

"$app/Contents/MacOS/stuarty-launcher"
for _ in $(seq 1 60); do
	if curl -fsS -m 2 http://127.0.0.1:7878/up 2>/dev/null | grep -q '"status":"ok"'; then up=1; break; fi
	sleep 0.5
done
[ "${up:-0}" = 1 ] || fail "the launcher did not start the local server"
pass "the launcher started the local server: GET /up answers ok"

sudo /usr/local/share/stuarty/uninstall.sh
[ ! -e /usr/local/bin/stuarty ] && [ ! -e "$app" ] && [ ! -e /usr/local/share/stuarty ] || fail "the uninstall left files behind"
[ -f "$workspace/mine.txt" ] && [ -f "$workspace/app.key" ] || fail "the workspace was not kept"
pass "the uninstall removes the program and keeps the workspace"
echo "smoke test passed"
