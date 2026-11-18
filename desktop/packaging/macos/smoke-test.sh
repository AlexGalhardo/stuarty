#!/usr/bin/env bash
# Mounts the macOS disk image of the desktop application and checks its content (SDD-009 section 10.4).
# The window is not started: a CI runner has no screen to show it.
#
#   smoke-test.sh <vukem-video-desktop_..._darwin_arm64.dmg>
set -euo pipefail

[ $# -eq 1 ] || { echo "usage: smoke-test.sh <disk image>" >&2; exit 2; }
dmg="$(cd "$(dirname "$1")" && pwd)/$(basename "$1")"

pass() { echo "ok: $*"; }
fail() { echo "smoke test failed: $*" >&2; exit 1; }

mnt="$(mktemp -d)"
cleanup() { hdiutil detach -quiet "$mnt" 2>/dev/null || true; rmdir "$mnt" 2>/dev/null || true; }
trap cleanup EXIT
hdiutil attach -quiet -nobrowse -readonly -mountpoint "$mnt" "$dmg"

app="$mnt/Vukem Video.app"
[ -x "$app/Contents/MacOS/vukem-video-desktop" ] || fail "no desktop application in $app"
[ -x "$app/Contents/MacOS/vukem-video" ] || fail "no server next to the application"
plutil -lint "$app/Contents/Info.plist" >/dev/null || fail "Info.plist is not valid"
[ "$(plutil -extract CFBundleExecutable raw "$app/Contents/Info.plist")" = vukem-video-desktop ] || fail "CFBundleExecutable is not the desktop application"
[ -f "$app/Contents/Resources/vukem-video.icns" ] || fail "no icon"
[ -L "$mnt/Applications" ] || fail "no Applications link"
pass "the application, the server next to it, the icon and the Applications link are in the image"

workspace="$(mktemp -d)"
trap 'rm -rf "$workspace"; cleanup' EXIT
VUKEM_VIDEO_HOME="$workspace" "$app/Contents/MacOS/vukem-video" version >/dev/null || fail "the packaged server does not run"
pass "the packaged server runs: vukem-video version"
echo "smoke test passed"
