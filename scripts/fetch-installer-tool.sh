#!/usr/bin/env bash
# Downloads one installer build tool of .config/installers/tools.env by its exact URL, verifies the
# SHA-256 and prepares it. Prints the path to use on the last line of stdout; messages go to stderr.
#
#   fetch-installer-tool.sh inno         installs Inno Setup silently, prints the path of ISCC.exe
#   fetch-installer-tool.sh appimagetool downloads appimagetool, prints the path of the executable
#
# Environment: INSTALLER_TOOLS_DIR (default $RUNNER_TEMP, $TEMP or $TMPDIR, then /installer-tools under it).
set -euo pipefail

log() { printf '%s\n' "$*" >&2; }
die() { log "fetch-installer-tool: error: $*"; exit 1; }

[ $# -eq 1 ] || die "usage: fetch-installer-tool.sh inno|appimagetool"
script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=../.config/installers/tools.env
. "$script_dir/../.config/installers/tools.env"

case "$1" in
inno) url="$INNO_SETUP_URL" sha="$INNO_SETUP_SHA256" ;;
appimagetool) url="$APPIMAGETOOL_URL" sha="$APPIMAGETOOL_SHA256" ;;
*) die "unknown tool: $1" ;;
esac
[[ "$sha" =~ ^[0-9a-f]{64}$ ]] || die "the digest of $1 is not 64 lowercase hex characters"

base="${INSTALLER_TOOLS_DIR:-${RUNNER_TEMP:-${TEMP:-${TMPDIR:-/tmp}}}/installer-tools}"
mkdir -p "$base"
file="$base/$(basename "$url")"

sha256_of() {
	if command -v sha256sum >/dev/null 2>&1; then
		sha256sum <"$1" | cut -d ' ' -f 1
	else
		shasum -a 256 <"$1" | cut -d ' ' -f 1
	fi
}

if [ ! -f "$file" ] || [ "$(sha256_of "$file")" != "$sha" ]; then
	log "fetch-installer-tool: downloading $url"
	curl --fail --silent --show-error --location --retry 3 --output "$file" "$url" || die "download failed: $url"
	actual="$(sha256_of "$file")"
	if [ "$actual" != "$sha" ]; then
		rm -f "$file"
		die "SHA-256 mismatch for $(basename "$url"): expected $sha, got $actual. The download was deleted."
	fi
fi
log "fetch-installer-tool: SHA-256 verified ($sha)"

case "$1" in
inno)
	dest="$base/inno-setup-$INNO_SETUP_VERSION"
	if [ ! -f "$dest/ISCC.exe" ]; then
		win() { if command -v cygpath >/dev/null 2>&1; then cygpath -w "$1"; else printf '%s' "$1"; fi; }
		# Git Bash would turn /VERYSILENT into a path; MSYS2_ARG_CONV_EXCL keeps the switches as they are.
		MSYS2_ARG_CONV_EXCL='*' "$file" /VERYSILENT /SUPPRESSMSGBOXES /NORESTART /CURRENTUSER "/DIR=$(win "$dest")" >&2
	fi
	[ -f "$dest/ISCC.exe" ] || die "Inno Setup was not installed in $dest"
	printf '%s\n' "$dest/ISCC.exe"
	;;
appimagetool)
	chmod +x "$file"
	printf '%s\n' "$file"
	;;
esac
