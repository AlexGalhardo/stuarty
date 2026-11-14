#!/usr/bin/env bash
# Installs the pinned LGPL FFmpeg of .config/ffmpeg.env (ADR-0011, SDD-009 11.2 and 13.3).
#
# Downloads the BtbN asset for this OS and architecture by exact URL, verifies its
# SHA-256, unpacks it into a temporary folder and prints the bin folder on the last
# line of stdout. Everything else goes to stderr. When GITHUB_PATH is set (GitHub
# Actions) the bin folder is appended to it.
#
# macOS has no BtbN build: Homebrew ffmpeg is installed FOR TESTS ONLY and is never
# distributed (risk R6).
#
# Environment: FFMPEG_ENV_FILE (default .config/ffmpeg.env of the repository),
# FFMPEG_INSTALL_DIR (default $RUNNER_TEMP, $TEMP or $TMPDIR, then /ffmpeg under it).
set -euo pipefail

log() { printf '%s\n' "$*" >&2; }
die() { log "install-ffmpeg: error: $*"; exit 1; }

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
env_file="${FFMPEG_ENV_FILE:-$script_dir/../.config/ffmpeg.env}"
[ -f "$env_file" ] || die "pin file not found: $env_file"
# shellcheck source=../.config/ffmpeg.env
. "$env_file"

for v in FFMPEG_RELEASE_TAG FFMPEG_VERSION; do
	[ -n "${!v:-}" ] || die "$v is missing in $env_file"
done

os="$(uname -s)"
arch="$(uname -m)"

if [ "$os" = "Darwin" ]; then
	log "install-ffmpeg: macOS has no BtbN LGPL build (risk R6)."
	log "install-ffmpeg: installing Homebrew ffmpeg FOR TESTS ONLY. It is never distributed."
	command -v brew >/dev/null 2>&1 || die "Homebrew is required on macOS (https://brew.sh)"
	brew list ffmpeg >/dev/null 2>&1 || brew install ffmpeg >&2
	bin_dir="$(dirname "$(command -v ffmpeg)")"
	ffmpeg -version | head -n 1 >&2
	if [ -n "${GITHUB_PATH:-}" ]; then printf '%s\n' "$bin_dir" >>"$GITHUB_PATH"; fi
	printf '%s\n' "$bin_dir"
	exit 0
fi

case "$os" in
Linux)
	case "$arch" in
	x86_64 | amd64) key=LINUX_AMD64 ;;
	aarch64 | arm64) key=LINUX_ARM64 ;;
	*) die "unsupported Linux architecture: $arch" ;;
	esac
	;;
MINGW* | MSYS* | CYGWIN*)
	case "$arch" in
	x86_64 | amd64) key=WINDOWS_AMD64 ;;
	*) die "unsupported Windows architecture: $arch" ;;
	esac
	;;
*) die "unsupported operating system: $os" ;;
esac

asset_var="FFMPEG_ASSET_$key"
sha_var="FFMPEG_SHA256_$key"
asset="${!asset_var:-}"
expected="${!sha_var:-}"
[ -n "$asset" ] || die "$asset_var is missing in $env_file"
[[ "$expected" =~ ^[0-9a-f]{64}$ ]] || die "$sha_var is not 64 lowercase hex characters in $env_file"

sha256_of() {
	if command -v sha256sum >/dev/null 2>&1; then
		# stdin: with a Windows path sha256sum prefixes the digest with a backslash.
		sha256sum <"$1" | cut -d ' ' -f 1
	else
		shasum -a 256 <"$1" | cut -d ' ' -f 1
	fi
}

base="${FFMPEG_INSTALL_DIR:-${RUNNER_TEMP:-${TEMP:-${TMPDIR:-/tmp}}}/ffmpeg}"
dl_dir="$base/download/$FFMPEG_RELEASE_TAG"
out_dir="$base/$FFMPEG_RELEASE_TAG-$key"
mkdir -p "$dl_dir"
archive="$dl_dir/$asset"
url="https://github.com/BtbN/FFmpeg-Builds/releases/download/$FFMPEG_RELEASE_TAG/$asset"

if [ -f "$archive" ] && [ "$(sha256_of "$archive")" = "$expected" ]; then
	log "install-ffmpeg: reusing the verified download $archive"
else
	log "install-ffmpeg: downloading $url"
	curl --fail --silent --show-error --location --retry 3 --output "$archive" "$url" \
		|| die "download failed: $url"
	actual="$(sha256_of "$archive")"
	if [ "$actual" != "$expected" ]; then
		rm -f "$archive"
		die "SHA-256 mismatch for $asset: expected $expected, got $actual. The download was deleted and nothing was unpacked."
	fi
	log "install-ffmpeg: SHA-256 verified ($expected)"
fi

rm -rf "$out_dir"
mkdir -p "$out_dir"
case "$asset" in
*.tar.xz) tar -xJf "$archive" -C "$out_dir" ;;
*.zip)
	# Git Bash tar is GNU tar and cannot read zip.
	powershell.exe -NoProfile -NonInteractive -Command \
		"Expand-Archive -LiteralPath '$(cygpath -w "$archive")' -DestinationPath '$(cygpath -w "$out_dir")' -Force" >&2 \
		|| die "could not unpack $archive"
	;;
*) die "unknown archive type: $asset" ;;
esac

root="$out_dir/${asset%.tar.xz}"
root="${root%.zip}"
bin_dir="$root/bin"
[ -d "$bin_dir" ] || die "unexpected archive layout: $bin_dir not found"
exe=""
[ "$key" = "WINDOWS_AMD64" ] && exe=".exe"
for tool in ffmpeg ffprobe; do
	[ -f "$bin_dir/$tool$exe" ] || die "$tool$exe is missing in $bin_dir"
done
[ -f "$root/LICENSE.txt" ] || die "LICENSE.txt is missing in $root"

banner="$("$bin_dir/ffmpeg$exe" -version)"
case "$banner" in
*--enable-gpl* | *--enable-nonfree*) die "the build is not LGPL: its configure line has --enable-gpl or --enable-nonfree" ;;
esac
printf '%s\n' "$banner" | head -n 1 >&2

if [ -n "${GITHUB_PATH:-}" ]; then printf '%s\n' "$bin_dir" >>"$GITHUB_PATH"; fi
printf '%s\n' "$bin_dir"
