#!/usr/bin/env bash
# Prints the own FFmpeg block of .config/ffmpeg.env (D-017, ADR-0011) from the assets of the release
# that .github/workflows/ffmpeg-build.yml published, or checks that the file holds exactly that block:
#   bash scripts/ffmpeg-pin.sh print [<dir>]
#   bash scripts/ffmpeg-pin.sh check [<dir>]
# <dir> holds the ffmpeg-*.tar.xz, ffmpeg-*.zip and ffmpeg-*.sha256 files of the release; without it
# the assets of FFMPEG_BUILD_TAG (.config/ffmpeg-build/sources.env) are downloaded with `gh release
# download` into a temporary folder. Every digest is computed here from the downloaded bytes (the
# archives, and the ffmpeg and ffprobe inside them), never copied from release notes. Needs: gh (no <dir>), tar, xz, unzip or PowerShell.
set -euo pipefail

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
die() { printf 'ffmpeg-pin: error: %s\n' "$*" >&2; exit 1; }

mode="${1:-}"
case "$mode" in print | check) ;; *) die "usage: ffmpeg-pin.sh print|check [<dir>]" ;; esac

# shellcheck source=../.config/ffmpeg-build/sources.env
. "$root/.config/ffmpeg-build/sources.env"

sha256_of() {
	if command -v sha256sum >/dev/null 2>&1; then sha256sum <"$1" | cut -d ' ' -f 1
	else shasum -a 256 <"$1" | cut -d ' ' -f 1; fi
}

# `check` validates the own block of .config/ffmpeg.env, which exists only when FFMPEG_SOURCE is "own": while the
# BtbN build is pinned, FFMPEG_BUILD_TAG may name a release that is built but not adopted yet (or not published).
if [ "$mode" = check ]; then
	pinned_source="$(grep -m1 '^FFMPEG_SOURCE=' "$root/.config/ffmpeg.env" | tr -d '"\r' | cut -d '=' -f 2)"
	if [ "${pinned_source:-btbn}" != own ]; then
		printf 'ffmpeg-pin: .config/ffmpeg.env pins %s, not the own build: nothing to check against %s\n' "${pinned_source:-btbn}" "$FFMPEG_BUILD_TAG"
		exit 0
	fi
fi

work="$(mktemp -d)"
trap 'rm -rf "$work"' EXIT
dir="${2:-}"
if [ -z "$dir" ]; then
	command -v gh >/dev/null 2>&1 || die "gh is required to download the release (or pass the folder of the assets)"
	dir="$work/assets"
	mkdir -p "$dir"
	gh release download "$FFMPEG_BUILD_TAG" --repo "${REPO:-AlexGalhardo/vukem-video}" --dir "$dir" --pattern 'ffmpeg-*' >&2
fi

unpack() { # unpack <archive> <dest>
	mkdir -p "$2"
	case "$1" in
	*.tar.xz) xz -dc <"$1" | tar -xf - -C "$2" ;;
	*.zip)
		if command -v unzip >/dev/null 2>&1; then unzip -q "$1" -d "$2"
		else powershell.exe -NoProfile -NonInteractive -Command \
			"Expand-Archive -LiteralPath '$(cygpath -w "$1")' -DestinationPath '$(cygpath -w "$2")' -Force" >&2; fi
		;;
	*) die "unknown archive type: $1" ;;
	esac
}

lines=()
add() { lines+=("$1=\"$2\""); }
add FFMPEG_OWN_TAG "$FFMPEG_BUILD_TAG"
add FFMPEG_OWN_VERSION "$FFMPEG_BUILD_VERSION"
add FFMPEG_OWN_SOURCE_URL "$FFMPEG_SRC_URL"
add FFMPEG_OWN_SOURCE_SHA256 "$FFMPEG_SRC_SHA256"

platforms=(linux-amd64 linux-arm64 windows-amd64 darwin-arm64 darwin-amd64)
assets=() sums=() bins=()
for p in "${platforms[@]}"; do
	key="$(printf '%s' "$p" | tr 'a-z-' 'A-Z_')"
	exe=""
	ext=tar.xz
	case "$p" in windows-*) exe=.exe; ext=zip ;; esac
	asset="ffmpeg-${FFMPEG_BUILD_VERSION}-${p}.${ext}"
	[ -f "$dir/$asset" ] || die "missing $dir/$asset"
	sum="$(sha256_of "$dir/$asset")"
	listed="$(cut -d ' ' -f 1 "$dir/$asset.sha256")"
	[ "$sum" = "$listed" ] || die "$asset: the file hashes to $sum but $asset.sha256 says $listed"
	unpack "$dir/$asset" "$work/x-$p"
	bin="$work/x-$p/ffmpeg-${FFMPEG_BUILD_VERSION}-${p}/bin"
	assets+=("FFMPEG_OWN_ASSET_$key $asset")
	sums+=("FFMPEG_OWN_SHA256_$key $sum")
	bins+=("FFMPEG_OWN_BIN_SHA256_$key $(sha256_of "$bin/ffmpeg$exe")")
	bins+=("FFPROBE_OWN_BIN_SHA256_$key $(sha256_of "$bin/ffprobe$exe")")
done
for kv in "${assets[@]}" "${sums[@]}" "${bins[@]}"; do add "${kv%% *}" "${kv#* }"; done

if [ "$mode" = print ]; then
	printf '%s\n' "${lines[@]}"
	exit 0
fi

env_file="$root/.config/ffmpeg.env"
bad=0
for l in "${lines[@]}"; do
	if ! grep -qxF -e "$l" -e "$l"$'\r' "$env_file"; then
		printf 'ffmpeg-pin: not in .config/ffmpeg.env: %s\n' "$l" >&2
		bad=1
	fi
done
[ "$bad" = 0 ] || exit 1
printf 'ffmpeg-pin: .config/ffmpeg.env matches the release %s (%s lines)\n' "$FFMPEG_BUILD_TAG" "${#lines[@]}"
