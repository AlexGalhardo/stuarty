#!/usr/bin/env bash
# Tests scripts/stage-ffmpeg-installer.sh (epic 98): a wrong digest or a missing input is refused and leaves
# nothing staged. With STAGE_TEST_ARCHIVE=<the windows zip> and STAGE_TEST_SHA256=<its digest> it also checks that
# the right archive stages ffmpeg.exe, ffprobe.exe and the licence texts with the digests of ffmpeg-stage.sha256.
set -euo pipefail

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
script="$root/scripts/stage-ffmpeg-installer.sh"
work="$(mktemp -d)"
trap 'rm -rf "$work"' EXIT
fail() { printf 'stage-ffmpeg-installer-test: FAIL: %s\n' "$*" >&2; exit 1; }
ok() { printf 'ok: %s\n' "$*"; }

printf 'not an archive' >"$work/bad.zip"
zero="$(printf '0%.0s' $(seq 64))"

if FFMPEG_ARCHIVE="$work/bad.zip" FFMPEG_ARCHIVE_SHA256="$zero" bash "$script" "$work/out" 2>/dev/null; then fail "a wrong archive digest was accepted"; fi
[ ! -e "$work/out" ] || fail "a refused archive left a staged folder"
ok "a wrong archive digest is refused and nothing is staged"

if FFMPEG_ARCHIVE="$work/bad.zip" bash "$script" "$work/out" 2>/dev/null; then fail "a local archive with no digest was accepted"; fi
ok "a local archive needs its digest"

if FFMPEG_ARCHIVE="$work/missing.zip" FFMPEG_ARCHIVE_SHA256="$zero" bash "$script" "$work/out" 2>/dev/null; then fail "a missing archive was accepted"; fi
ok "a missing archive is refused"

if [ -n "${STAGE_TEST_ARCHIVE:-}" ]; then
	good="${STAGE_TEST_SHA256:?set STAGE_TEST_SHA256 with STAGE_TEST_ARCHIVE}"
	if FFMPEG_ARCHIVE="$STAGE_TEST_ARCHIVE" FFMPEG_ARCHIVE_SHA256="$good" FFMPEG_BIN_SHA256="$zero" bash "$script" "$work/out" 2>/dev/null; then
		fail "a wrong ffmpeg.exe digest was accepted"
	fi
	ok "a wrong ffmpeg.exe digest is refused"
	FFMPEG_ARCHIVE="$STAGE_TEST_ARCHIVE" FFMPEG_ARCHIVE_SHA256="$good" bash "$script" "$work/out" >/dev/null 2>&1 || fail "the right archive was refused"
	for f in ffmpeg.exe ffprobe.exe licenses/ffmpeg/COPYING.LGPLv3 licenses/SOURCE.txt licenses/configure.txt ffmpeg-stage.sha256; do
		[ -f "$work/out/$f" ] || fail "not staged: $f"
	done
	(cd "$work/out" && sha256sum -c --quiet ffmpeg-stage.sha256) || fail "the staged files differ from ffmpeg-stage.sha256"
	ok "the right archive stages the binaries and the licence texts with their digests"
else
	printf 'skip: set STAGE_TEST_ARCHIVE and STAGE_TEST_SHA256 for the positive case\n'
fi
