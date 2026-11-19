#!/usr/bin/env bash
# Checks the Debian package of the desktop application without installing it (the dependencies of the
# window, GTK and WebKitGTK, are large and a CI runner has no screen). SDD-009 section 10.4.
#
#   smoke-test.sh <stuarty-desktop_..._linux_amd64.deb>
set -euo pipefail

[ $# -eq 1 ] || { echo "usage: smoke-test.sh <package>" >&2; exit 2; }
deb="$(cd "$(dirname "$1")" && pwd)/$(basename "$1")"

pass() { echo "ok: $*"; }
fail() { echo "smoke test failed: $*" >&2; exit 1; }

work="$(mktemp -d)"
trap 'rm -rf "$work"' EXIT
dpkg-deb -x "$deb" "$work"

[ "$(dpkg-deb -f "$deb" Package)" = stuarty-desktop ] || fail "the package name is not stuarty-desktop"
dpkg-deb -f "$deb" Depends | grep -q libwebkit2gtk-4.1-0 || fail "the package does not depend on WebKitGTK"
pass "the control file names the package and the window dependencies"

lib="$work/usr/lib/stuarty-desktop"
[ -x "$lib/stuarty-desktop" ] && [ -x "$lib/stuarty" ] || fail "the application and the server are not both in $lib"
[ -L "$work/usr/bin/stuarty-desktop" ] || fail "no /usr/bin/stuarty-desktop link"
[ ! -e "$work/usr/bin/stuarty" ] || fail "the package must not own /usr/bin/stuarty (the server package does)"
pass "the application and the server sit in one folder, and /usr/bin/stuarty is left to the server package"

entry="$work/usr/share/applications/stuarty-desktop.desktop"
[ -f "$entry" ] || fail "no desktop entry"
[ -f "$work/usr/share/icons/hicolor/256x256/apps/stuarty-desktop.png" ] || fail "no icon"
if command -v desktop-file-validate >/dev/null 2>&1; then
	desktop-file-validate "$entry" || fail "the desktop entry is not valid"
	pass "desktop-file-validate accepts the entry"
fi

STUARTY_HOME="$work/workspace" "$lib/stuarty" version >/dev/null || fail "the packaged server does not run"
pass "the packaged server runs: stuarty version"
echo "smoke test passed"
