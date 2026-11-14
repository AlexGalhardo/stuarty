#!/usr/bin/env bash
# The binaries that the installers carry must be the binaries of the release (SDD-009 section 10.4).
# GoReleaser builds them twice in a release: once for the installers workflow, once for the archives.
# They are reproducible (-trimpath, a fixed module timestamp), and this script proves it.
#
#   installer-binaries.sh flatten <dist/artifacts.json> <output directory>
#       copies each binary of a GoReleaser run to <output>/<goos>_<goarch>/ and writes
#       <output>/binaries.sha256 (sha256sum format, paths relative to <output>)
#   installer-binaries.sh check <binaries.sha256> <dist/artifacts.json>
#       fails when a binary of the later run differs from the one recorded by flatten
set -euo pipefail

die() { echo "installer-binaries: error: $*" >&2; exit 1; }
command -v jq >/dev/null 2>&1 || die "jq is required"

sha256_of() {
	if command -v sha256sum >/dev/null 2>&1; then
		sha256sum <"$1" | cut -d ' ' -f 1
	else
		shasum -a 256 <"$1" | cut -d ' ' -f 1
	fi
}

# binaries <artifacts.json> prints "<goos> <goarch> <path>" for each binary, paths as GoReleaser wrote them.
binaries() {
	jq -r '.[] | select(.type == "Binary") | [.goos, .goarch, .path] | @tsv' "$1" | tr -d '\r'
}

case "${1:-}" in
flatten)
	[ $# -eq 3 ] || die "usage: installer-binaries.sh flatten <artifacts.json> <output directory>"
	out="$3"
	mkdir -p "$out"
	: >"$out/binaries.sha256"
	while IFS=$'\t' read -r goos goarch path; do
		[ -n "$goos" ] || continue
		dir="${goos}_${goarch}"
		mkdir -p "$out/$dir"
		cp "$path" "$out/$dir/"
		printf '%s  %s\n' "$(sha256_of "$out/$dir/$(basename "$path")")" "$dir/$(basename "$path")" >>"$out/binaries.sha256"
	done < <(binaries "$2")
	[ -s "$out/binaries.sha256" ] || die "no binary found in $2"
	sort -k2 -o "$out/binaries.sha256" "$out/binaries.sha256"
	;;
check)
	[ $# -eq 3 ] || die "usage: installer-binaries.sh check <binaries.sha256> <artifacts.json>"
	status=0
	while IFS=$'\t' read -r goos goarch path; do
		[ -n "$goos" ] || continue
		key="${goos}_${goarch}/$(basename "$path")"
		want="$(awk -v k="$key" '$2 == k { print $1 }' "$2")"
		[ -n "$want" ] || { echo "installer-binaries: $key is not in $2" >&2; status=1; continue; }
		got="$(sha256_of "$path")"
		if [ "$got" != "$want" ]; then
			echo "installer-binaries: $key differs: installers carry $want, the release has $got" >&2
			status=1
		fi
	done < <(binaries "$3")
	[ "$status" -eq 0 ] || die "the installers do not carry the binaries of the release"
	echo "installer-binaries: every binary of the release is the one the installers carry"
	;;
*) die "usage: installer-binaries.sh flatten|check ..." ;;
esac
