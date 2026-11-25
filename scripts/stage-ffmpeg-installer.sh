#!/usr/bin/env bash
# Stages the own FFmpeg build for the Windows installer (epic 98, SDD-009 section 10.4, SDD-011 section 8.1):
#   bash scripts/stage-ffmpeg-installer.sh <output dir>
# It leaves in <output dir>: ffmpeg.exe, ffprobe.exe and licenses/ (the licence texts, SOURCE.txt and
# configure.txt of the build), plus ffmpeg-stage.sha256, and prints the folder to pass to Inno Setup as
# /DFFmpegDir=<output dir>. The installer never downloads anything: this runs at build time.
#
# Where the archive comes from (every digest is computed here from the bytes, then compared):
#   FFMPEG_ARCHIVE=<local .zip>  with FFMPEG_ARCHIVE_SHA256=<64 hex>, and optionally FFMPEG_BIN_SHA256 and
#                                FFPROBE_BIN_SHA256, for a build input before the assets are published;
#   (nothing set)                the FFMPEG_OWN_* block of .config/ffmpeg.env (FFMPEG_SOURCE="own"), downloaded
#                                from the release FFMPEG_OWN_TAG of REPO (default AlexGalhardo/stuarty).
set -euo pipefail

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
die() { printf 'stage-ffmpeg-installer: error: %s\n' "$*" >&2; exit 1; }
sha256_of() {
	if command -v sha256sum >/dev/null 2>&1; then sha256sum <"$1" | cut -d ' ' -f 1
	else shasum -a 256 <"$1" | cut -d ' ' -f 1; fi
}
hex64() { [[ "$1" =~ ^[0-9a-f]{64}$ ]]; }

[ $# -eq 1 ] || die "usage: stage-ffmpeg-installer.sh <output dir>"
out="$1"
work="$(mktemp -d)"
trap 'rm -rf "$work"' EXIT

archive="" want_archive="" want_ffmpeg="" want_ffprobe="" version=""
if [ -n "${FFMPEG_ARCHIVE:-}" ]; then
	archive="$FFMPEG_ARCHIVE"
	want_archive="${FFMPEG_ARCHIVE_SHA256:-}"
	want_ffmpeg="${FFMPEG_BIN_SHA256:-}"
	want_ffprobe="${FFPROBE_BIN_SHA256:-}"
	[ -f "$archive" ] || die "FFMPEG_ARCHIVE is not a file: $archive"
	hex64 "$want_archive" || die "FFMPEG_ARCHIVE needs FFMPEG_ARCHIVE_SHA256, 64 lowercase hex characters"
else
	# shellcheck source=../.config/ffmpeg.env
	. "$root/.config/ffmpeg.env"
	[ "${FFMPEG_SOURCE:-}" = own ] || die ".config/ffmpeg.env does not pin the own build yet: set FFMPEG_ARCHIVE and FFMPEG_ARCHIVE_SHA256 for a local archive"
	asset="${FFMPEG_OWN_ASSET_WINDOWS_AMD64:?}"
	want_archive="${FFMPEG_OWN_SHA256_WINDOWS_AMD64:?}"
	want_ffmpeg="${FFMPEG_OWN_BIN_SHA256_WINDOWS_AMD64:?}"
	want_ffprobe="${FFPROBE_OWN_BIN_SHA256_WINDOWS_AMD64:?}"
	for d in "$want_archive" "$want_ffmpeg" "$want_ffprobe"; do hex64 "$d" || die "a digest in .config/ffmpeg.env is not 64 lowercase hex characters"; done
	archive="$work/$asset"
	url="https://github.com/${REPO:-AlexGalhardo/stuarty}/releases/download/${FFMPEG_OWN_TAG:?}/$asset"
	printf 'stage-ffmpeg-installer: downloading %s\n' "$url" >&2
	curl --fail --silent --show-error --location --retry 3 --output "$archive" "$url" || die "download failed: $url"
fi

got="$(sha256_of "$archive")"
[ "$got" = "$want_archive" ] || die "the archive hashes to $got, expected $want_archive"

x="$work/x"
mkdir -p "$x"
if command -v unzip >/dev/null 2>&1; then unzip -q "$archive" -d "$x"
else powershell.exe -NoProfile -NonInteractive -Command \
	"Expand-Archive -LiteralPath '$(cygpath -w "$archive")' -DestinationPath '$(cygpath -w "$x")' -Force" >&2; fi

top=""
for d in "$x"/*/; do
	case "$d" in *windows-amd64/) top="${d%/}" ;; esac
done
[ -n "$top" ] || die "the archive has no ffmpeg-*-windows-amd64 folder"
for f in bin/ffmpeg.exe bin/ffprobe.exe LICENSE.txt SOURCE.txt configure.txt licenses/ffmpeg/COPYING.LGPLv3; do
	[ -f "$top/$f" ] || die "the archive lacks $f"
done

got_ffmpeg="$(sha256_of "$top/bin/ffmpeg.exe")"
got_ffprobe="$(sha256_of "$top/bin/ffprobe.exe")"
[ -z "$want_ffmpeg" ] || [ "$got_ffmpeg" = "$want_ffmpeg" ] || die "ffmpeg.exe hashes to $got_ffmpeg, expected $want_ffmpeg"
[ -z "$want_ffprobe" ] || [ "$got_ffprobe" = "$want_ffprobe" ] || die "ffprobe.exe hashes to $got_ffprobe, expected $want_ffprobe"

# The own build is the one the engine tells apart by this marker (SDD-011 8.2); a GPL or nonfree one is refused (NFR-043).
grep -aq -e '--extra-version=stuarty' "$top/bin/ffmpeg.exe" || die "ffmpeg.exe does not carry the stuarty marker"
if grep -aq -e '--enable-gpl' -e '--enable-nonfree' "$top/bin/ffmpeg.exe" "$top/bin/ffprobe.exe"; then die "a GPL or nonfree flag is in the build"; fi

rm -rf "$out"
mkdir -p "$out/licenses"
cp "$top/bin/ffmpeg.exe" "$top/bin/ffprobe.exe" "$out/"
cp -R "$top/licenses/." "$out/licenses/"
cp "$top/LICENSE.txt" "$top/SOURCE.txt" "$top/configure.txt" "$out/licenses/"
{
	printf '%s  ffmpeg.exe\n' "$got_ffmpeg"
	printf '%s  ffprobe.exe\n' "$got_ffprobe"
	(cd "$out/licenses" && find . -type f | LC_ALL=C sort | while read -r f; do printf '%s  licenses/%s\n' "$(sha256_of "$f")" "${f#./}"; done)
} >"$out/ffmpeg-stage.sha256"
printf 'stage-ffmpeg-installer: staged %s (archive %s)\n' "$out" "$got" >&2
printf '%s\n' "$out"
