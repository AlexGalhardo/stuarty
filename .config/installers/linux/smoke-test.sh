#!/usr/bin/env bash
# Installs a Linux package, runs what its shortcut runs, removes it, and checks what must hold
# (SDD-009 section 10.4): the files are in place, the shortcut command ("vukem-video open") starts the
# local server on 127.0.0.1 (VUKEM_VIDEO_NO_BROWSER keeps the browser closed), the removal leaves the
# workspace alone.
#
#   smoke-test.sh deb <vukem-video_..._linux_amd64.deb>      needs sudo and dpkg (an Ubuntu or Debian runner)
#   smoke-test.sh appimage <vukem-video_..._linux_amd64.AppImage>
set -euo pipefail

[ $# -eq 2 ] || { echo "usage: smoke-test.sh deb|appimage <file>" >&2; exit 2; }
kind="$1" file="$(readlink -f "$2")"

pass() { echo "ok: $*"; }
fail() { echo "smoke test failed: $*" >&2; exit 1; }

workspace="$(mktemp -d)"
echo "keep me" >"$workspace/mine.txt"
export VUKEM_VIDEO_HOME="$workspace" VUKEM_VIDEO_NO_BROWSER=1
cleanup() { pkill -x vukem-video 2>/dev/null || true; rm -rf "$workspace"; }
trap cleanup EXIT

wait_up() {
	for _ in $(seq 1 60); do
		if curl -fsS -m 2 http://127.0.0.1:7878/up 2>/dev/null | grep -q '"status":"ok"'; then return 0; fi
		sleep 0.5
	done
	return 1
}

case "$kind" in
deb)
	sudo dpkg -i "$file"
	pass "dpkg installs the package"
	command -v vukem-video >/dev/null || fail "vukem-video is not on the PATH"
	pass "vukem-video is on the PATH"
	desktop=/usr/share/applications/vukem-video.desktop
	[ -f "$desktop" ] || fail "no desktop entry at $desktop"
	[ -f /usr/share/icons/hicolor/256x256/apps/vukem-video.png ] || fail "no icon"
	pass "the desktop entry and the icon are installed"
	if command -v desktop-file-validate >/dev/null 2>&1; then
		desktop-file-validate "$desktop" || fail "the desktop entry is not valid"
		pass "desktop-file-validate accepts the entry"
	fi
	exec_line="$(sed -n 's/^Exec=//p' "$desktop" | head -n 1)"
	[ "$exec_line" = "vukem-video open" ] || fail "the entry runs '$exec_line', not 'vukem-video open'"
	# Run the command of the entry, as a launcher does.
	bash -c "$exec_line"
	wait_up || fail "the shortcut command did not start the local server"
	pass "the shortcut command started the local server: GET /up answers ok"
	pkill -x vukem-video || true
	sudo dpkg -r vukem-video
	command -v vukem-video >/dev/null && fail "vukem-video is still installed"
	[ ! -e "$desktop" ] || fail "the desktop entry is still there"
	[ -f "$workspace/mine.txt" ] && [ -f "$workspace/app.key" ] || fail "the workspace was not kept"
	pass "dpkg -r removes the program and keeps the workspace"
	;;
appimage)
	chmod +x "$file"
	# Runners have no FUSE: the AppImage extracts itself and runs.
	APPIMAGE_EXTRACT_AND_RUN=1 "$file"
	wait_up || fail "running the AppImage with no argument did not start the local server"
	pass "the AppImage with no argument started the local server: GET /up answers ok"
	sleep 2
	curl -fsS -m 2 http://127.0.0.1:7878/up | grep -q '"status":"ok"' || fail "the server did not outlive the AppImage"
	pass "the server outlives the AppImage process"
	[ -f "$workspace/app.key" ] || fail "no workspace was made"
	;;
*) fail "unknown kind $kind" ;;
esac
echo "smoke test passed"
