#!/usr/bin/env bash
# Checks the Debian package of the desktop application without installing it (the dependencies of the
# window, GTK and WebKitGTK, are large and a CI runner has no screen). SDD-009 section 10.4.
#
#   smoke-test.sh <vukem-video-desktop_..._linux_amd64.deb>
set -euo pipefail

[ $# -eq 1 ] || { echo "usage: smoke-test.sh <package>" >&2; exit 2; }
deb="$(cd "$(dirname "$1")" && pwd)/$(basename "$1")"

pass() { echo "ok: $*"; }
fail() { echo "smoke test failed: $*" >&2; exit 1; }

work="$(mktemp -d)"
trap 'rm -rf "$work"' EXIT
dpkg-deb -x "$deb" "$work"

[ "$(dpkg-deb -f "$deb" Package)" = vukem-video-desktop ] || fail "the package name is not vukem-video-desktop"
dpkg-deb -f "$deb" Depends | grep -q libwebkit2gtk-4.1-0 || fail "the package does not depend on WebKitGTK"
pass "the control file names the package and the window dependencies"

lib="$work/usr/lib/vukem-video-desktop"
[ -x "$lib/vukem-video-desktop" ] && [ -x "$lib/vukem-video" ] || fail "the application and the server are not both in $lib"
[ -L "$work/usr/bin/vukem-video-desktop" ] || fail "no /usr/bin/vukem-video-desktop link"
[ ! -e "$work/usr/bin/vukem-video" ] || fail "the package must not own /usr/bin/vukem-video (the server package does)"
pass "the application and the server sit in one folder, and /usr/bin/vukem-video is left to the server package"

entry="$work/usr/share/applications/vukem-video-desktop.desktop"
[ -f "$entry" ] || fail "no desktop entry"
[ -f "$work/usr/share/icons/hicolor/256x256/apps/vukem-video-desktop.png" ] || fail "no icon"
if command -v desktop-file-validate >/dev/null 2>&1; then
	desktop-file-validate "$entry" || fail "the desktop entry is not valid"
	pass "desktop-file-validate accepts the entry"
fi

VUKEM_VIDEO_HOME="$work/workspace" "$lib/vukem-video" version >/dev/null || fail "the packaged server does not run"
pass "the packaged server runs: vukem-video version"
echo "smoke test passed"
