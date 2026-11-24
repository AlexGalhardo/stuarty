#!/usr/bin/env bash
# Fetches the pinned ONNX Runtime library and the YuNet model of .config/aihelper.env into a folder, for
# development and for the opt-in tests of engine/aihost (STUARTY_TEST_AIHELPER_DIR). It is the scripted twin
# of `stuarty tools:install face`; the build of aihelper/ never calls it and downloads nothing.
#
# usage: install-aihelper-runtime.sh <folder> [--dry-run]
#   --dry-run  prints what would be downloaded and checked, and makes no request.
# Windows x64 only until epic 43 builds the helper elsewhere. Every file is checked against its SHA-256 and
# its size before it is kept; a wrong file is deleted and the script fails.
set -euo pipefail

log() { printf '%s\n' "$*" >&2; }
die() { log "install-aihelper-runtime: error: $*"; exit 1; }

dest="${1:?usage: install-aihelper-runtime.sh <folder> [--dry-run]}"
dry="${2:-}"

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=../.config/aihelper.env
. "$script_dir/../.config/aihelper.env"

sha256_of() {
	if command -v sha256sum >/dev/null 2>&1; then sha256sum "$1" | cut -d' ' -f1; else shasum -a 256 "$1" | cut -d' ' -f1; fi
}

fetch() { # url bytes sha256 outfile
	local url="$1" bytes="$2" want="$3" out="$4" part="$4.part"
	log "install-aihelper-runtime: $url ($bytes bytes, sha256 $want)"
	[ "$dry" = "--dry-run" ] && return 0
	curl --fail --silent --show-error --location --proto '=https' --max-redirs 5 --max-filesize "$bytes" -o "$part" "$url"
	local size got
	size="$(wc -c <"$part" | tr -d ' ')"
	got="$(sha256_of "$part")"
	if [ "$size" != "$bytes" ] || [ "$got" != "$want" ]; then
		rm -f "$part"
		die "$out is not the pinned file (size $size, sha256 $got)"
	fi
	mv "$part" "$out"
}

mkdir -p "$dest"
fetch "$ORT_URL" "$ORT_BYTES" "$ORT_SHA256" "$dest/onnxruntime.nupkg"
fetch "$YUNET_URL" "$YUNET_BYTES" "$YUNET_SHA256" "$dest/yunet.onnx"

if [ "$dry" != "--dry-run" ]; then
	# The members are taken by their exact names, never by what the archive offers.
	unzip -j -o -q "$dest/onnxruntime.nupkg" \
		"runtimes/win-x64/native/onnxruntime.dll" "runtimes/win-x64/native/onnxruntime_providers_shared.dll" -d "$dest"
	rm -f "$dest/onnxruntime.nupkg"
	log "install-aihelper-runtime: ONNX Runtime $ORT_VERSION and the YuNet model are in $dest"
fi
