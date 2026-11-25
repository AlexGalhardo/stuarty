#!/usr/bin/env bash
# Epic 98 task 3: a fresh Windows install exports with no FFmpeg supplied by the person. Where Inno Setup is not
# installed, the files the installer places (the [Files] section of stuarty-desktop.iss) are copied into a new folder,
# and the reference export of scripts/first-export-timing.sh --binary runs through the installed stuarty.exe, in an
# isolated workspace, with a PATH that holds no FFmpeg:
#
#   desktop/packaging/windows/installed-export-check.sh <stuarty.exe> <folder made by scripts/stage-ffmpeg-installer.sh>
#
# On a machine with the installer, run smoke-test.ps1 -Installer <setup.exe> -FFmpegDir <that folder> instead.
# It prints the installed digests and the build the server resolved. It owns everything it starts and removes it.
set -euo pipefail

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/../../.." && pwd)"
die() { printf 'installed-export-check: FAILED: %s\n' "$*" >&2; exit 1; }
[ $# -eq 2 ] || die "usage: installed-export-check.sh <stuarty.exe> <ffmpeg stage folder>"
server="$1" stage="$2"
[ -f "$server" ] || die "no such file: $server"
[ -f "$stage/ffmpeg.exe" ] && [ -f "$stage/ffprobe.exe" ] || die "the stage folder has no ffmpeg.exe and ffprobe.exe"

work="$(mktemp -d)"
app="$work/Stuarty Desktop"
cleanup() {
	local code=$?
	[ -z "${pid:-}" ] || kill "$pid" >/dev/null 2>&1 || true
	rm -rf "$work"
	exit "$code"
}
trap cleanup EXIT

mkdir -p "$app/licenses/ffmpeg-build"
cp "$server" "$app/stuarty.exe"
cp "$stage/ffmpeg.exe" "$stage/ffprobe.exe" "$app/"
cp -R "$stage/licenses/." "$app/licenses/ffmpeg-build/"
cp "$root/THIRD_PARTY_NOTICES.md" "$root/LICENSE.md" "$app/"
cp -R "$root/licenses/." "$app/licenses/"

echo "== installed files"
(cd "$app" && sha256sum stuarty.exe ffmpeg.exe ffprobe.exe)
(cd "$stage" && sha256sum -c --quiet ffmpeg-stage.sha256) || die "the staged files do not match their digests"
for f in ffmpeg.exe ffprobe.exe; do
	[ "$(sha256sum <"$app/$f" | cut -d ' ' -f 1)" = "$(sha256sum <"$stage/$f" | cut -d ' ' -f 1)" ] || die "installed $f differs from the staged one"
done
[ -f "$app/licenses/ffmpeg-build/ffmpeg/COPYING.LGPLv3" ] || die "the FFmpeg licence texts are not installed"
"$app/ffmpeg.exe" -hide_banner -version | head -n 1 | grep -q 'stuarty' || die "the installed ffmpeg has no stuarty marker"

# The harness makes its stand-in clip with a command named ffmpeg: a script without .exe, which the server's
# LookPath on Windows does not see. Every other directory of the PATH that holds an FFmpeg is dropped.
shim="$work/shim"
mkdir -p "$shim"
printf '#!/usr/bin/env bash\nexec "%s/ffmpeg.exe" "$@"\n' "$app" >"$shim/ffmpeg"
chmod +x "$shim/ffmpeg"
clean=""
IFS=':' read -ra dirs <<<"$PATH"
for d in "${dirs[@]}"; do
	if [ -e "$d/ffmpeg" ] || [ -e "$d/ffmpeg.exe" ] || [ -e "$d/ffprobe.exe" ]; then continue; fi
	clean="$clean:$d"
done
export PATH="$shim$clean"
unset STUARTY_FFMPEG STUARTY_FFPROBE STUARTY_TEST_FFMPEG_DIR
command -v ffmpeg.exe >/dev/null 2>&1 && die "an ffmpeg.exe is still in the PATH"
echo "== the installed program resolves its own FFmpeg"
mkdir -p "$work/doctor-home"
doctor="$(STUARTY_HOME="$(cygpath -w "$work/doctor-home" 2>/dev/null || echo "$work/doctor-home")" "$app/stuarty.exe" doctor --full 2>&1)" || true
printf '%s\n' "$doctor" | grep -E '^ok ffmpeg\.binary: bundled, ' || die "doctor did not resolve the bundled FFmpeg: $doctor"
echo "== no FFmpeg in the PATH for the server; export through the installed program"
bash "$root/scripts/first-export-timing.sh" --binary "$app/stuarty.exe"
